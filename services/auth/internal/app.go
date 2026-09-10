package internal

import (
	"context"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/auralis/platform/authn"
	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/auralis/platform/kafkax"
	"github.com/auralis/platform/logging"
	"github.com/auralis/platform/outbox"
	"github.com/auralis/platform/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	service      = "auth"
	maxFailed    = 8
	failWindow   = 15 * time.Minute
	bcryptCost   = 12
	displayMaxCh = 60
)

// App holds the auth service dependencies and HTTP handlers.
type App struct {
	Store  *Store
	Tokens TokenConfig
}

// NewApp builds an App.
func NewApp(store *Store, tokens TokenConfig) *App {
	return &App{Store: store, Tokens: tokens}
}

// BootstrapAdmin ensures an admin account exists. It is idempotent: an existing
// account is promoted to ADMIN (password untouched); a new one is created with
// USER, CREATOR, and ADMIN roles and a user.registered event is enqueued so the
// user service provisions its profile. Called once at startup when
// AUTH_BOOTSTRAP_ADMIN_EMAIL is configured.
func (a *App) BootstrapAdmin(ctx context.Context, email, password, displayName string) error {
	norm := normalizeEmail(email)
	if norm == "" {
		return errcodes.BadRequest("AUTH_BOOTSTRAP_ADMIN_EMAIL is not a valid address")
	}
	existing, err := a.Store.UserByEmailNorm(ctx, norm)
	if err == nil {
		if !existing.HasAdmin() {
			tx, err := a.Store.Pool().Begin(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback(ctx)
			if _, e := a.Store.SetRoles(ctx, tx, existing.ID, appendRole(existing.Roles, authn.RoleAdmin)); e != nil {
				return e
			}
			return tx.Commit(ctx)
		}
		return nil
	}
	if err != ErrNotFound {
		return err
	}
	if len(password) < 10 {
		return errcodes.BadRequest("AUTH_BOOTSTRAP_ADMIN_PASSWORD must be at least 10 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return err
	}
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	user, err := a.Store.CreateUser(ctx, tx, email, norm, string(hash), displayName,
		[]string{authn.RoleUser, authn.RoleCreator, authn.RoleAdmin})
	if err != nil {
		return err
	}
	env, _ := envelope.New("user.registered", 1, service, "", "", map[string]any{
		"user_id": user.ID, "email": user.Email, "display_name": user.DisplayName,
		"roles": user.Roles, "registered_at": user.CreatedAt,
	})
	if err := outbox.Enqueue(ctx, tx, kafkax.TopicUserEvents, user.ID, env); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Routes registers auth endpoints. All are public at the gateway; the service
// authorizes admin actions itself using forwarded identity headers.
func (a *App) Routes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", a.register)
		r.Post("/login", a.login)
		r.Post("/refresh", a.refresh)
		r.Post("/logout", a.logout)
		r.Get("/me", a.me)
		r.Post("/password", a.changePassword)

		r.Group(func(r chi.Router) {
			r.Use(authn.RequireRoles(authn.RoleAdmin))
			r.Get("/admin/users", a.listUsers)
			r.Post("/admin/users/{id}/roles", a.setRoles)
			r.Post("/admin/users/{id}/status", a.setStatus)
		})
	})
}

// --- request/response types ---

type registerReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

type passwordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type publicUser struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Roles       []string  `json:"roles"`
	Status      string    `json:"status"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

func toPublic(u User) publicUser {
	return publicUser{u.ID, u.Email, u.Roles, u.Status, u.DisplayName, u.CreatedAt}
}

// --- handlers ---

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	email := strings.TrimSpace(req.Email)
	norm := normalizeEmail(email)
	if _, err := mail.ParseAddress(email); err != nil || norm == "" {
		httpx.Error(w, r, errcodes.BadRequest("a valid email address is required"))
		return
	}
	if err := checkPassword(req.Password); err != nil {
		httpx.Error(w, r, err)
		return
	}
	display := strings.TrimSpace(req.DisplayName)
	if display == "" {
		display = strings.SplitN(email, "@", 2)[0]
	}
	if len([]rune(display)) > displayMaxCh {
		httpx.Error(w, r, errcodes.BadRequest("display name is too long"))
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not hash password"))
		return
	}

	ctx := r.Context()
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("database unavailable"))
		return
	}
	defer tx.Rollback(ctx)

	user, err := a.Store.CreateUser(ctx, tx, email, norm, string(hash), display, []string{authn.RoleUser})
	if err != nil {
		if strings.Contains(err.Error(), "users_email_norm_key") {
			httpx.Error(w, r, errcodes.Conflicting("an account with this email already exists"))
			return
		}
		httpx.Error(w, r, errcodes.Unexpected("could not create account"))
		return
	}

	env, _ := envelope.New("user.registered", 1, service,
		logging.FromContext(ctx).CorrelationID, "", map[string]any{
			"user_id": user.ID, "email": user.Email, "display_name": user.DisplayName,
			"roles": user.Roles, "registered_at": user.CreatedAt,
		})
	if err := outbox.Enqueue(ctx, tx, kafkax.TopicUserEvents, user.ID, env); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not record registration event"))
		return
	}
	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not complete registration"))
		return
	}

	tokens, err := a.mintForLogin(ctx, user, r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	telemetry.Count(service, "register", "success")
	httpx.JSON(w, http.StatusCreated, map[string]any{"user": toPublic(user), "tokens": tokens})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	ctx := r.Context()
	norm := normalizeEmail(req.Email)
	ip := clientIP(r)

	// An unparseable address can never match an account. Reject it up front so
	// we neither query for the empty string nor write empty-email rows into
	// login_attempts (which the per-email throttle keys on).
	if norm == "" {
		telemetry.Count(service, "login", "failure")
		httpx.Error(w, r, errcodes.Unauthed("email or password is incorrect"))
		return
	}

	if failed, _ := a.Store.RecentFailedLogins(ctx, norm, failWindow); failed >= maxFailed {
		telemetry.Count(service, "login", "throttled")
		httpx.Error(w, r, errcodes.New(http.StatusTooManyRequests, errcodes.RateLimited,
			"too many failed attempts, try again later"))
		return
	}

	user, err := a.Store.UserByEmailNorm(ctx, norm)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		_ = a.Store.RecordLoginAttempt(ctx, norm, ip, false)
		telemetry.Count(service, "login", "failure")
		httpx.Error(w, r, errcodes.Unauthed("email or password is incorrect"))
		return
	}
	if user.Status != "active" {
		httpx.Error(w, r, errcodes.Denied("this account is "+user.Status))
		return
	}

	_ = a.Store.RecordLoginAttempt(ctx, norm, ip, true)
	tokens, err := a.mintForLogin(ctx, user, r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	telemetry.Count(service, "login", "success")
	httpx.JSON(w, http.StatusOK, map[string]any{"user": toPublic(user), "tokens": tokens})
}

func (a *App) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		httpx.Error(w, r, errcodes.BadRequest("refresh_token is required"))
		return
	}
	ctx := r.Context()
	row, err := a.Store.RefreshByHash(ctx, hashToken(req.RefreshToken))
	if err != nil {
		httpx.Error(w, r, errcodes.Unauthed("invalid refresh token"))
		return
	}

	// Reuse of an already-rotated or revoked token is treated as theft: revoke
	// the whole family so neither the attacker nor the victim can continue.
	if row.UsedAt != nil || row.RevokedAt != nil {
		_ = a.Store.RevokeFamily(ctx, row.FamilyID)
		telemetry.Count(service, "refresh", "reuse_detected")
		httpx.Error(w, r, errcodes.Unauthed("refresh token has already been used"))
		return
	}
	if time.Now().After(row.ExpiresAt) {
		httpx.Error(w, r, errcodes.Unauthed("refresh token has expired"))
		return
	}

	user, err := a.Store.UserByID(ctx, row.UserID)
	if err != nil || user.Status != "active" {
		httpx.Error(w, r, errcodes.Unauthed("account is not active"))
		return
	}

	rawRefresh, newHash := newOpaqueRefresh()
	expires := time.Now().Add(a.Tokens.RefreshTTL)
	if _, err := a.Store.RotateRefresh(ctx, row.ID, user.ID, row.FamilyID, newHash, expires, r.UserAgent(), clientIP(r)); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not rotate refresh token"))
		return
	}
	access, exp, err := a.Tokens.signAccess(user.ID, user.Roles)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not sign access token"))
		return
	}
	telemetry.Count(service, "refresh", "success")
	httpx.JSON(w, http.StatusOK, map[string]any{"tokens": IssuedTokens{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(a.Tokens.AccessTTL.Seconds()),
		ExpiresAt:    exp,
	}})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.RefreshToken != "" {
		if row, err := a.Store.RefreshByHash(r.Context(), hashToken(req.RefreshToken)); err == nil {
			_ = a.Store.RevokeToken(r.Context(), row.ID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// accountLookupErr answers a single-account read: 404 only when the row is
// genuinely absent, 500 for anything else. Collapsing every error to "account
// not found" hid a database outage (and a malformed id) behind a 404 that never
// paged, even though the id here comes from a gateway-signed token and the row
// almost always exists.
func accountLookupErr(w http.ResponseWriter, r *http.Request, err error) {
	if err == ErrNotFound {
		httpx.Error(w, r, errcodes.Missing("account not found"))
		return
	}
	httpx.Error(w, r, errcodes.Unexpected("account lookup failed"))
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	id, err := authn.MustIdentity(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	user, err := a.Store.UserByID(r.Context(), id.UserID)
	if err != nil {
		accountLookupErr(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toPublic(user))
}

func (a *App) changePassword(w http.ResponseWriter, r *http.Request) {
	id, err := authn.MustIdentity(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req passwordReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := checkPassword(req.NewPassword); err != nil {
		httpx.Error(w, r, err)
		return
	}
	ctx := r.Context()
	user, err := a.Store.UserByID(ctx, id.UserID)
	if err != nil {
		accountLookupErr(w, r, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		httpx.Error(w, r, errcodes.Unauthed("current password is incorrect"))
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not hash password"))
		return
	}
	if err := a.Store.UpdatePassword(ctx, user.ID, string(hash)); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not update password"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) listUsers(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 1_000_000)
	users, err := a.Store.ListUsers(r.Context(), limit, offset)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not list users"))
		return
	}
	out := make([]publicUser, 0, len(users))
	for _, u := range users {
		out = append(out, toPublic(u))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": out, "limit": limit, "offset": offset})
}

func (a *App) setRoles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Roles []string `json:"roles"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	roles, err := validRoles(req.Roles)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	a.mutateUser(w, r, "user.roles_changed", func(ctx context.Context, tx pgx.Tx) (User, map[string]any, error) {
		u, e := a.Store.SetRoles(ctx, tx, chi.URLParam(r, "id"), roles)
		return u, map[string]any{"user_id": u.ID, "roles": roles}, e
	})
}

func (a *App) setStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.Status != "active" && req.Status != "suspended" {
		httpx.Error(w, r, errcodes.BadRequest("status must be active or suspended"))
		return
	}
	a.mutateUser(w, r, "user.status_changed", func(ctx context.Context, tx pgx.Tx) (User, map[string]any, error) {
		u, e := a.Store.SetStatus(ctx, tx, chi.URLParam(r, "id"), req.Status)
		return u, map[string]any{"user_id": u.ID, "status": req.Status}, e
	})
}

// mutateUser applies an admin mutation and enqueues its outbox event in a single
// transaction, so the state change and the event the user service consumes
// either both land or neither does.
func (a *App) mutateUser(w http.ResponseWriter, r *http.Request, eventType string, fn func(context.Context, pgx.Tx) (User, map[string]any, error)) {
	ctx := r.Context()
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("database unavailable"))
		return
	}
	defer tx.Rollback(ctx)

	user, payload, err := fn(ctx, tx)
	if err != nil {
		if err == ErrNotFound {
			httpx.Error(w, r, errcodes.Missing("user not found"))
			return
		}
		httpx.Error(w, r, errcodes.Unexpected("could not update user"))
		return
	}

	env, _ := envelope.New(eventType, 1, service, logging.FromContext(ctx).CorrelationID, "", payload)
	if err := outbox.Enqueue(ctx, tx, kafkax.TopicUserEvents, user.ID, env); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not record user event"))
		return
	}
	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not update user"))
		return
	}
	httpx.JSON(w, http.StatusOK, toPublic(user))
}

// mintForLogin issues an access token and a fresh refresh-token family.
func (a *App) mintForLogin(ctx context.Context, user User, r *http.Request) (IssuedTokens, error) {
	access, exp, err := a.Tokens.signAccess(user.ID, user.Roles)
	if err != nil {
		return IssuedTokens{}, errcodes.Unexpected("could not sign access token")
	}
	rawRefresh, hash := newOpaqueRefresh()
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		return IssuedTokens{}, errcodes.Unexpected("database unavailable")
	}
	defer tx.Rollback(ctx)
	familyID := newFamilyID()
	if _, err := a.Store.InsertRefresh(ctx, tx, user.ID, familyID, hash,
		time.Now().Add(a.Tokens.RefreshTTL), r.UserAgent(), clientIP(r)); err != nil {
		return IssuedTokens{}, errcodes.Unexpected("could not store refresh token")
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedTokens{}, errcodes.Unexpected("could not complete login")
	}
	return IssuedTokens{
		AccessToken:  access,
		RefreshToken: rawRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(a.Tokens.AccessTTL.Seconds()),
		ExpiresAt:    exp,
	}, nil
}

// --- helpers ---

func normalizeEmail(email string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(e); err != nil {
		return ""
	}
	return e
}

func checkPassword(pw string) error {
	if len(pw) < 10 {
		return errcodes.BadRequest("password must be at least 10 characters")
	}
	if len(pw) > 200 {
		return errcodes.BadRequest("password is too long")
	}
	var hasLetter, hasOther bool
	for _, c := range pw {
		if unicode.IsLetter(c) {
			hasLetter = true
		} else {
			hasOther = true
		}
	}
	if !hasLetter || !hasOther {
		return errcodes.BadRequest("password must contain letters and at least one number or symbol")
	}
	return nil
}

func validRoles(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, errcodes.BadRequest("at least one role is required")
	}
	allowed := map[string]bool{authn.RoleUser: true, authn.RoleCreator: true, authn.RoleAdmin: true}
	seen := map[string]bool{}
	var out []string
	for _, r := range in {
		r = strings.ToUpper(strings.TrimSpace(r))
		if !allowed[r] {
			return nil, errcodes.BadRequest("unknown role: " + r)
		}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out, nil
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	h, _, _ := strings.Cut(r.RemoteAddr, ":")
	return h
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
