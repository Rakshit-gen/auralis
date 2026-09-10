package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type tokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type authResp struct {
	User   publicUser `json:"user"`
	Tokens tokenPair  `json:"tokens"`
}

func post(t *testing.T, srv string, path string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(http.MethodPost, srv+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

func registerUser(t *testing.T, srv, email string) authResp {
	t.Helper()
	resp, data := post(t, srv, "/auth/register", map[string]string{
		"email": email, "password": "correct-horse-9", "display_name": "Test Listener",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register %s: status %d body %s", email, resp.StatusCode, data)
	}
	var out authResp
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRegisterLoginFlow(t *testing.T) {
	app, _ := newTestApp(t)
	srv := newServer(app)
	defer srv.Close()

	reg := registerUser(t, srv.URL, "listener@example.com")
	if reg.User.Roles[0] != "USER" || reg.Tokens.AccessToken == "" || reg.Tokens.RefreshToken == "" {
		t.Fatalf("unexpected register response: %+v", reg)
	}

	// Duplicate email is a conflict.
	resp, _ := post(t, srv.URL, "/auth/register", map[string]string{
		"email": "listener@example.com", "password": "correct-horse-9",
	}, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate register: expected 409, got %d", resp.StatusCode)
	}

	// Wrong password fails.
	resp, _ = post(t, srv.URL, "/auth/login", map[string]string{
		"email": "listener@example.com", "password": "wrong-password-1",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login: expected 401, got %d", resp.StatusCode)
	}

	// Correct password succeeds.
	resp, data := post(t, srv.URL, "/auth/login", map[string]string{
		"email": "LISTENER@example.com", "password": "correct-horse-9",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d body %s", resp.StatusCode, data)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	app, _ := newTestApp(t)
	srv := newServer(app)
	defer srv.Close()

	reg := registerUser(t, srv.URL, "rotate@example.com")
	first := reg.Tokens.RefreshToken

	// Rotate once: the old token is consumed, a new one issued.
	resp, data := post(t, srv.URL, "/auth/refresh", map[string]string{"refresh_token": first}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: expected 200, got %d body %s", resp.StatusCode, data)
	}
	var rotated struct {
		Tokens tokenPair `json:"tokens"`
	}
	_ = json.Unmarshal(data, &rotated)
	if rotated.Tokens.RefreshToken == first {
		t.Fatal("refresh token was not rotated")
	}

	// Reusing the first (now consumed) token must fail and revoke the family.
	resp, _ = post(t, srv.URL, "/auth/refresh", map[string]string{"refresh_token": first}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused token: expected 401, got %d", resp.StatusCode)
	}
	// The rotated token is now also dead because the family was revoked.
	resp, _ = post(t, srv.URL, "/auth/refresh", map[string]string{"refresh_token": rotated.Tokens.RefreshToken}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("family revocation: expected rotated token to be rejected, got %d", resp.StatusCode)
	}
}

func TestChangePasswordRequiresIdentityAndCurrentPassword(t *testing.T) {
	app, _ := newTestApp(t)
	srv := newServer(app)
	defer srv.Close()

	reg := registerUser(t, srv.URL, "pw@example.com")

	// No identity headers: unauthorized.
	resp, _ := post(t, srv.URL, "/auth/password", map[string]string{
		"current_password": "correct-horse-9", "new_password": "another-good-1",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no identity: expected 401, got %d", resp.StatusCode)
	}

	h := identityHeaders(reg.User.ID, "USER")

	// Wrong current password: unauthorized.
	resp, _ = post(t, srv.URL, "/auth/password", map[string]string{
		"current_password": "not-it-99", "new_password": "another-good-1",
	}, h)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current pw: expected 401, got %d", resp.StatusCode)
	}

	// Correct: succeeds and the new password logs in.
	resp, _ = post(t, srv.URL, "/auth/password", map[string]string{
		"current_password": "correct-horse-9", "new_password": "another-good-1",
	}, h)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change pw: expected 204, got %d", resp.StatusCode)
	}
	resp, _ = post(t, srv.URL, "/auth/login", map[string]string{
		"email": "pw@example.com", "password": "another-good-1",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login with new pw: expected 200, got %d", resp.StatusCode)
	}

	// The refresh token issued before the password change is now revoked.
	resp, _ = post(t, srv.URL, "/auth/refresh", map[string]string{
		"refresh_token": reg.Tokens.RefreshToken,
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("pre-change refresh token: expected 401 after password change, got %d", resp.StatusCode)
	}
}

func TestAdminRoleManagement(t *testing.T) {
	app, _ := newTestApp(t)
	srv := newServer(app)
	defer srv.Close()

	admin := registerUser(t, srv.URL, "admin@example.com")
	target := registerUser(t, srv.URL, "target@example.com")

	// A plain USER cannot manage roles.
	resp, _ := post(t, srv.URL, "/auth/admin/users/"+target.User.ID+"/roles",
		map[string]any{"roles": []string{"USER", "CREATOR"}}, identityHeaders(admin.User.ID, "USER"))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("USER managing roles: expected 403, got %d", resp.StatusCode)
	}

	// An ADMIN can.
	resp, data := post(t, srv.URL, "/auth/admin/users/"+target.User.ID+"/roles",
		map[string]any{"roles": []string{"USER", "CREATOR"}}, identityHeaders(admin.User.ID, "ADMIN"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ADMIN managing roles: expected 200, got %d body %s", resp.StatusCode, data)
	}
	var updated publicUser
	_ = json.Unmarshal(data, &updated)
	if len(updated.Roles) != 2 || updated.Roles[1] != "CREATOR" {
		t.Fatalf("roles not applied: %+v", updated.Roles)
	}

	// The role change is recorded in the outbox for the user service to consume.
	var count int
	if err := app.Store.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM outbox_events WHERE envelope->>'event_type' = 'user.roles_changed'`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 roles_changed outbox event, got %d", count)
	}
}
