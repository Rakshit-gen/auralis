// Package internal implements the Auralis API gateway: authentication at the
// edge, identity forwarding, per-caller rate limiting, and reverse proxying to
// the domain services.
package internal

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/telemetry"
	"github.com/google/uuid"
)

// Backend is one downstream service.
type Backend struct {
	Name  string
	URL   string
	proxy *httputil.ReverseProxy
}

// Route maps a public path prefix to a backend service. The forwarded path is
// TargetPrefix followed by whatever came after Prefix, so
// (Prefix "/api/catalog", TargetPrefix "") turns /api/catalog/shows into /shows.
type Route struct {
	Prefix       string // public path prefix, e.g. "/api/auth"
	Backend      string
	TargetPrefix string // prefix on the backend, e.g. "/auth" or ""
	Public       bool   // no access token required
	PublicGET    bool   // GET is public, other methods require auth
}

// Gateway holds configuration and wiring.
type Gateway struct {
	verifier       *authn.Verifier
	identitySecret string
	backends       map[string]*Backend
	routes         []Route
	limiter        Limiter
	rateLimit      int
	rateWindow     time.Duration
}

// Limiter decides whether a request may proceed. Allow returns the remaining
// budget for response headers.
type Limiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ok bool, remaining int, err error)
}

// Config for New.
type Config struct {
	JWTSecret      string
	JWTIssuer      string
	JWTAudience    string
	IdentitySecret string
	Backends       map[string]string // name -> base URL
	Limiter        Limiter
	RateLimit      int
	RateWindow     time.Duration
}

// New builds a Gateway.
func New(cfg Config) (*Gateway, error) {
	g := &Gateway{
		verifier:       authn.NewVerifier(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience),
		identitySecret: cfg.IdentitySecret,
		backends:       map[string]*Backend{},
		limiter:        cfg.Limiter,
	}
	// One shared transport across all backends. The default transport keeps only
	// two idle connections per host, which forces a new TCP handshake for almost
	// every proxied request under load and exhausts local ephemeral ports; a
	// larger idle pool lets the gateway reuse upstream connections.
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	for name, base := range cfg.Backends {
		u, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		b := &Backend{Name: name, URL: base}
		b.proxy = httputil.NewSingleHostReverseProxy(u)
		// NewSingleHostReverseProxy rewrites the URL host but leaves the Host
		// header as the caller sent it. Platforms that route by Host (Render,
		// most PaaS edges) would bounce the request back to the gateway, so
		// pin the Host header to the backend too.
		baseDirector := b.proxy.Director
		b.proxy.Director = func(r *http.Request) {
			baseDirector(r)
			r.Host = u.Host
		}
		b.proxy.Transport = transport
		b.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logging.L(r.Context()).Error("backend proxy error", "backend", name, "error", err.Error())
			errcodes.Write(w, r.Header.Get("X-Auralis-Request-Id"),
				errcodes.New(http.StatusBadGateway, errcodes.Unavailable, name+" is unavailable"))
		}
		g.backends[name] = b
	}
	g.routes = defaultRoutes()
	g.rateLimit, g.rateWindow = cfg.RateLimit, cfg.RateWindow
	if g.rateLimit == 0 {
		g.rateLimit = 240
	}
	if g.rateWindow == 0 {
		g.rateWindow = time.Minute
	}
	return g, nil
}

func defaultRoutes() []Route {
	return []Route{
		// Longest / most specific prefixes first.
		{Prefix: "/api/admin/users", Backend: "auth", TargetPrefix: "/auth/admin/users"},
		{Prefix: "/api/admin/entitlements", Backend: "user", TargetPrefix: "/admin"},
		{Prefix: "/api/auth", Backend: "auth", TargetPrefix: "/auth", Public: true},
		{Prefix: "/api/catalog", Backend: "content", TargetPrefix: "", PublicGET: true},
		{Prefix: "/api/content", Backend: "content", TargetPrefix: ""},
		{Prefix: "/api/me", Backend: "user", TargetPrefix: "/me"},
		{Prefix: "/api/playback", Backend: "playback", TargetPrefix: "/playback"},
		{Prefix: "/api/recommendations", Backend: "recommendation", TargetPrefix: "/recommendations", PublicGET: true},
		{Prefix: "/api/generate", Backend: "ai-media", TargetPrefix: "/generate"},
		{Prefix: "/api/ai", Backend: "ai-media", TargetPrefix: "/ai"},
		{Prefix: "/api/analytics", Backend: "analytics", TargetPrefix: "/analytics"},
	}
}

// ServeHTTP routes one request.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := g.match(r.URL.Path)
	if !ok {
		errcodes.Write(w, r.Header.Get("X-Auralis-Request-Id"), errcodes.Missing("no route for this path"))
		return
	}
	backend := g.backends[route.Backend]
	if backend == nil {
		errcodes.Write(w, r.Header.Get("X-Auralis-Request-Id"),
			errcodes.New(http.StatusBadGateway, errcodes.Unavailable, route.Backend+" is not configured"))
		return
	}

	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = uuid.NewString()
	}
	r.Header.Set("X-Auralis-Request-Id", reqID)
	corrID := r.Header.Get("X-Correlation-Id")
	if corrID == "" {
		corrID = reqID
		r.Header.Set("X-Correlation-Id", corrID)
	}
	ctx := logging.WithFields(r.Context(), logging.Fields{RequestID: reqID, CorrelationID: corrID, Endpoint: route.Prefix})
	r = r.WithContext(ctx)

	// Authenticate if a token is present; enforce when the route requires it.
	var identity authn.Identity
	authed := false
	if tok := bearer(r); tok != "" {
		claims, err := g.verifier.Parse(tok)
		if err != nil {
			errcodes.Write(w, reqID, err)
			return
		}
		identity = authn.Identity{UserID: claims.Subject, Roles: claims.Roles}
		authed = true
	}

	requiresAuth := !route.Public && !(route.PublicGET && r.Method == http.MethodGet)
	if requiresAuth && !authed {
		errcodes.Write(w, reqID, errcodes.Unauthed("authentication required"))
		return
	}

	// Rate limit: per user when known, else per client IP.
	limitKey := "ip:" + clientIP(r)
	if authed {
		limitKey = "user:" + identity.UserID
	}
	if g.limiter != nil {
		allow, remaining, err := g.limiter.Allow(ctx, limitKey, g.rateLimit, g.rateWindow)
		if err != nil {
			logging.L(ctx).Warn("rate limiter error, allowing request", "error", err.Error())
		} else {
			w.Header().Set("X-RateLimit-Remaining", itoa(remaining))
			if !allow {
				telemetry.Count("gateway", "rate_limit", "blocked")
				w.Header().Set("Retry-After", "1")
				errcodes.Write(w, reqID, errcodes.New(http.StatusTooManyRequests, errcodes.RateLimited,
					"rate limit exceeded"))
				return
			}
		}
	}

	// Strip any client-supplied trust headers before we set our own. The
	// service token in particular must never be forwarded from a client: it is
	// a shared secret that grants unauthenticated service-to-service access, so
	// a client that learned it could otherwise reach every /internal/* route.
	for _, h := range []string{"X-Auralis-User", "X-Auralis-Roles", "X-Auralis-Identity-Sig", "X-Auralis-Service-Token"} {
		r.Header.Del(h)
	}
	if authed {
		for k, v := range authn.IdentityHeaders(g.identitySecret, identity, reqID) {
			r.Header.Set(k, v)
		}
	}

	// Rewrite the path for the backend: replace the gateway prefix with the
	// backend's target prefix.
	rest := strings.TrimPrefix(r.URL.Path, route.Prefix)
	r.URL.Path = route.TargetPrefix + rest
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	start := time.Now()
	backend.proxy.ServeHTTP(w, r)
	telemetry.ObserveHTTP("gateway", r.Method, route.Prefix, 0, time.Since(start))
}

func (g *Gateway) match(path string) (Route, bool) {
	for _, rt := range g.routes {
		if path == rt.Prefix || strings.HasPrefix(path, rt.Prefix+"/") {
			return rt, true
		}
	}
	return Route{}, false
}
