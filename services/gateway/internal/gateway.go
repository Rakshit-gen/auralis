// Package internal implements the Auralis API gateway: authentication at the
// edge, identity forwarding, per-caller rate limiting, and reverse proxying to
// the domain services.
package internal

import (
	"context"
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

// Route maps a public path prefix to a backend and its auth requirement.
type Route struct {
	Prefix      string // e.g. "/api/auth"
	Backend     string
	StripPrefix string // what to remove before proxying, e.g. "/api"
	Public      bool   // no access token required
	PublicGET   bool   // GET is public, other methods require auth
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
	for name, base := range cfg.Backends {
		u, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		b := &Backend{Name: name, URL: base}
		b.proxy = httputil.NewSingleHostReverseProxy(u)
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
		{Prefix: "/api/auth", Backend: "auth", StripPrefix: "/api", Public: true},
		{Prefix: "/api/catalog", Backend: "content", StripPrefix: "/api/catalog", PublicGET: true},
		{Prefix: "/api/me", Backend: "user", StripPrefix: "/api"},
		{Prefix: "/api/users", Backend: "user", StripPrefix: "/api"},
		{Prefix: "/api/creator", Backend: "content", StripPrefix: "/api/creator"},
		{Prefix: "/api/admin/content", Backend: "content", StripPrefix: "/api/admin/content"},
		{Prefix: "/api/admin/users", Backend: "auth", StripPrefix: "/api/admin/users"},
		{Prefix: "/api/admin/entitlements", Backend: "user", StripPrefix: "/api/admin/entitlements"},
		{Prefix: "/api/admin/analytics", Backend: "analytics", StripPrefix: "/api/admin"},
		{Prefix: "/api/admin/jobs", Backend: "ai-media", StripPrefix: "/api/admin"},
		{Prefix: "/api/playback", Backend: "playback", StripPrefix: "/api"},
		{Prefix: "/api/recommendations", Backend: "recommendation", StripPrefix: "/api"},
		{Prefix: "/api/generate", Backend: "ai-media", StripPrefix: "/api"},
		{Prefix: "/api/ai", Backend: "ai-media", StripPrefix: "/api"},
		{Prefix: "/api/analytics", Backend: "analytics", StripPrefix: "/api"},
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

	// Strip any client-supplied identity headers before we set our own.
	for _, h := range []string{"X-Auralis-User", "X-Auralis-Roles", "X-Auralis-Identity-Sig"} {
		r.Header.Del(h)
	}
	if authed {
		for k, v := range authn.IdentityHeaders(g.identitySecret, identity, reqID) {
			r.Header.Set(k, v)
		}
	}

	// Rewrite the path for the backend.
	if route.StripPrefix != "" && strings.HasPrefix(r.URL.Path, route.StripPrefix) {
		r.URL.Path = r.URL.Path[len(route.StripPrefix):]
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
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
