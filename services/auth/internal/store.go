package internal

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// User is the auth service's view of an account.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Roles        []string
	Status       string
	DisplayName  string
	CreatedAt    time.Time
}

// HasAdmin reports whether the user holds the ADMIN role.
func (u User) HasAdmin() bool { return containsRole(u.Roles, "ADMIN") }

// RefreshRow is a stored refresh token record.
type RefreshRow struct {
	ID        string
	UserID    string
	FamilyID  string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

// Store is the auth database.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the connection pool for transactional work in the app layer.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func containsRole(roles []string, r string) bool {
	for _, x := range roles {
		if x == r {
			return true
		}
	}
	return false
}

func appendRole(roles []string, r string) []string {
	if containsRole(roles, r) {
		return roles
	}
	return append(append([]string{}, roles...), r)
}

func (s *Store) CreateUser(ctx context.Context, tx pgx.Tx, email, emailNorm, hash, displayName string, roles []string) (User, error) {
	var u User
	err := tx.QueryRow(ctx,
		`INSERT INTO users (email, email_norm, password_hash, display_name, roles)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, email, password_hash, roles, status, display_name, created_at`,
		email, emailNorm, hash, displayName, roles,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Roles, &u.Status, &u.DisplayName, &u.CreatedAt)
	return u, err
}

func (s *Store) UserByEmailNorm(ctx context.Context, emailNorm string) (User, error) {
	return s.scanUser(ctx, `SELECT id, email, password_hash, roles, status, display_name, created_at
		FROM users WHERE email_norm = $1`, emailNorm)
}

func (s *Store) UserByID(ctx context.Context, id string) (User, error) {
	return s.scanUser(ctx, `SELECT id, email, password_hash, roles, status, display_name, created_at
		FROM users WHERE id = $1`, id)
}

func (s *Store) scanUser(ctx context.Context, q string, arg any) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, q, arg).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Roles, &u.Status, &u.DisplayName, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// UpdatePassword sets a new password hash and, in the same transaction, revokes
// every outstanding refresh token for the user so a password change ends all
// other sessions.
func (s *Store) UpdatePassword(ctx context.Context, userID, newHash string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ct, err := tx.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, newHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetRoles and SetStatus run inside the caller's transaction so the mutation
// and the outbox event it produces commit together.
func (s *Store) SetRoles(ctx context.Context, tx pgx.Tx, userID string, roles []string) (User, error) {
	return scanUserRow(ctx, tx,
		`UPDATE users SET roles = $2, updated_at = now() WHERE id = $1
		 RETURNING id, email, password_hash, roles, status, display_name, created_at`, userID, roles)
}

func (s *Store) SetStatus(ctx context.Context, tx pgx.Tx, userID, status string) (User, error) {
	return scanUserRow(ctx, tx,
		`UPDATE users SET status = $2, updated_at = now() WHERE id = $1
		 RETURNING id, email, password_hash, roles, status, display_name, created_at`, userID, status)
}

func scanUserRow(ctx context.Context, tx pgx.Tx, q string, args ...any) (User, error) {
	var u User
	err := tx.QueryRow(ctx, q, args...).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Roles, &u.Status, &u.DisplayName, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context, limit, offset int) ([]User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, email, password_hash, roles, status, display_name, created_at
		 FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Roles, &u.Status, &u.DisplayName, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// --- refresh tokens ---

func (s *Store) InsertRefresh(ctx context.Context, tx pgx.Tx, userID, familyID, hash string, expires time.Time, ua, ip string) (string, error) {
	var id string
	err := tx.QueryRow(ctx,
		`INSERT INTO refresh_tokens (user_id, family_id, token_hash, expires_at, user_agent, client_ip)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, familyID, hash, expires, ua, ip).Scan(&id)
	return id, err
}

func (s *Store) RefreshByHash(ctx context.Context, hash string) (RefreshRow, error) {
	var r RefreshRow
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, family_id, expires_at, used_at, revoked_at
		 FROM refresh_tokens WHERE token_hash = $1`, hash).
		Scan(&r.ID, &r.UserID, &r.FamilyID, &r.ExpiresAt, &r.UsedAt, &r.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshRow{}, ErrNotFound
	}
	return r, err
}

// RotateRefresh marks old as used, links it to the replacement, and inserts the
// new token, all in one transaction.
func (s *Store) RotateRefresh(ctx context.Context, oldID, userID, familyID, newHash string, expires time.Time, ua, ip string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var newID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO refresh_tokens (user_id, family_id, token_hash, expires_at, user_agent, client_ip)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		userID, familyID, newHash, expires, ua, ip).Scan(&newID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET used_at = now(), replaced_by = $2 WHERE id = $1`,
		oldID, newID); err != nil {
		return "", err
	}
	return newID, tx.Commit(ctx)
}

// RevokeFamily revokes every unrevoked token in a family (token reuse response).
func (s *Store) RevokeFamily(ctx context.Context, familyID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`,
		familyID)
	return err
}

// RevokeToken revokes a single token by id (logout).
func (s *Store) RevokeToken(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	return err
}

// --- login throttling ---

func (s *Store) RecordLoginAttempt(ctx context.Context, emailNorm, ip string, ok bool) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO login_attempts (email_norm, client_ip, successful) VALUES ($1, $2, $3)`,
		emailNorm, ip, ok)
	return err
}

// RecentFailedLogins counts failed attempts for an email in the given window.
func (s *Store) RecentFailedLogins(ctx context.Context, emailNorm string, since time.Duration) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM login_attempts
		 WHERE email_norm = $1 AND successful = false AND at > now() - $2::interval`,
		emailNorm, since.String()).Scan(&n)
	return n, err
}
