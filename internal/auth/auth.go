package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const CookieName = "planexus_session"

type Principal struct {
	ID                 uuid.UUID  `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"displayName"`
	Email              string     `json:"email,omitempty"`
	OrganizationID     *uuid.UUID `json:"organizationId,omitempty"`
	Title              string     `json:"title,omitempty"`
	Roles              []string   `json:"roles"`
	Permissions        []string   `json:"permissions"`
	MustChangePassword bool       `json:"mustChangePassword"`
	CSRFToken          string     `json:"csrfToken,omitempty"`
	AuthMethod         string     `json:"authMethod"`
}

type Resource struct {
	OwnerID        *uuid.UUID
	OrganizationID *uuid.UUID
	Classification string
}

type contextKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func randomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func tokenHash(token string) []byte { h := sha256.Sum256([]byte(token)); return h[:] }

func (s *Service) LocalLogin(ctx context.Context, username, password string) (Principal, string, error) {
	var p Principal
	var hash *string
	var active bool
	err := s.pool.QueryRow(ctx, `SELECT id,username,display_name,COALESCE(email,''),organization_id,COALESCE(title,''),password_hash,active,must_change_password FROM users WHERE lower(username)=lower($1)`, strings.TrimSpace(username)).Scan(
		&p.ID, &p.Username, &p.DisplayName, &p.Email, &p.OrganizationID, &p.Title, &hash, &active, &p.MustChangePassword)
	if err != nil || hash == nil || !active || bcrypt.CompareHashAndPassword([]byte(*hash), []byte(password)) != nil {
		// Preserve roughly similar work for an unknown username.
		if hash == nil {
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoO5lSjD.6MEmQm9JtTOQvixXq2iA7Rzuy"), []byte(password))
		}
		return Principal{}, "", errors.New("invalid credentials")
	}
	p.AuthMethod = "local"
	if err := s.loadGrants(ctx, &p); err != nil {
		return Principal{}, "", err
	}
	token, csrf, err := s.createSession(ctx, p.ID, "local")
	if err != nil {
		return Principal{}, "", err
	}
	p.CSRFToken = csrf
	return p, token, nil
}

func (s *Service) CreateOIDCSession(ctx context.Context, userID uuid.UUID) (Principal, string, error) {
	p, err := s.loadUser(ctx, userID)
	if err != nil {
		return Principal{}, "", err
	}
	token, csrf, err := s.createSession(ctx, p.ID, "oidc")
	if err != nil {
		return Principal{}, "", err
	}
	p.AuthMethod, p.CSRFToken = "oidc", csrf
	return p, token, nil
}

func (s *Service) createSession(ctx context.Context, userID uuid.UUID, method string) (string, string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken(24)
	if err != nil {
		return "", "", err
	}
	timeout := 12 * time.Hour
	var value []byte
	if settingErr := s.pool.QueryRow(ctx, `SELECT value FROM system_settings WHERE category='system' AND key='session' AND sensitive=false`).Scan(&value); settingErr == nil {
		var policy struct {
			TimeoutMinutes int `json:"timeoutMinutes"`
		}
		if json.Unmarshal(value, &policy) == nil && policy.TimeoutMinutes >= 15 && policy.TimeoutMinutes <= 720 {
			timeout = time.Duration(policy.TimeoutMinutes) * time.Minute
		}
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_token,auth_method,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), userID, tokenHash(token), csrf, method, time.Now().Add(timeout))
	return token, csrf, err
}

func (s *Service) Authenticate(r *http.Request) (Principal, error) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return s.authenticateAPIKey(r.Context(), strings.TrimSpace(h[7:]))
	}
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return Principal{}, errors.New("authentication required")
	}
	var p Principal
	var sessionID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `
		SELECT u.id,u.username,u.display_name,COALESCE(u.email,''),u.organization_id,COALESCE(u.title,''),u.must_change_password,s.id,s.csrf_token,s.auth_method
		FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active=true`, tokenHash(cookie.Value)).Scan(
		&p.ID, &p.Username, &p.DisplayName, &p.Email, &p.OrganizationID, &p.Title, &p.MustChangePassword, &sessionID, &p.CSRFToken, &p.AuthMethod)
	if err != nil {
		return Principal{}, errors.New("invalid or expired session")
	}
	if err = s.loadGrants(r.Context(), &p); err != nil {
		return Principal{}, err
	}
	_, _ = s.pool.Exec(r.Context(), `UPDATE sessions SET last_seen_at=now() WHERE id=$1 AND last_seen_at<now()-interval '5 minutes'`, sessionID)
	return p, nil
}

func (s *Service) authenticateAPIKey(ctx context.Context, token string) (Principal, error) {
	var p Principal
	var keyID uuid.UUID
	var scopes []string
	err := s.pool.QueryRow(ctx, `
		SELECT u.id,u.username,u.display_name,COALESCE(u.email,''),u.organization_id,COALESCE(u.title,''),u.must_change_password,k.id,k.scopes
		FROM personal_access_keys k JOIN users u ON u.id=k.user_id
		WHERE k.secret_hash=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now()) AND u.active=true`, tokenHash(token)).Scan(
		&p.ID, &p.Username, &p.DisplayName, &p.Email, &p.OrganizationID, &p.Title, &p.MustChangePassword, &keyID, &scopes)
	if err != nil {
		return Principal{}, errors.New("invalid or expired API key")
	}
	p.AuthMethod, p.Permissions = "api_key", scopes
	current := Principal{ID: p.ID}
	if err := s.loadGrants(ctx, &current); err != nil {
		return Principal{}, err
	}
	p.Roles = current.Roles
	p.Permissions = effectiveKeyScopes(scopes, current.Permissions)
	_, _ = s.pool.Exec(ctx, `UPDATE personal_access_keys SET last_used_at=now() WHERE id=$1 AND (last_used_at IS NULL OR last_used_at<now()-interval '5 minutes')`, keyID)
	return p, nil
}

func (s *Service) loadUser(ctx context.Context, id uuid.UUID) (Principal, error) {
	var p Principal
	err := s.pool.QueryRow(ctx, `SELECT id,username,display_name,COALESCE(email,''),organization_id,COALESCE(title,''),must_change_password FROM users WHERE id=$1 AND active=true`, id).Scan(&p.ID, &p.Username, &p.DisplayName, &p.Email, &p.OrganizationID, &p.Title, &p.MustChangePassword)
	if err != nil {
		return Principal{}, err
	}
	err = s.loadGrants(ctx, &p)
	return p, err
}

func (s *Service) loadGrants(ctx context.Context, p *Principal) error {
	rows, err := s.pool.Query(ctx, `SELECT r.id,unnest(r.permissions) FROM roles r JOIN user_roles ur ON ur.role_id=r.id WHERE ur.user_id=$1`, p.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	roleSeen := map[string]bool{}
	permSeen := map[string]bool{}
	for rows.Next() {
		var role, permission string
		if err := rows.Scan(&role, &permission); err != nil {
			return err
		}
		if !roleSeen[role] {
			p.Roles = append(p.Roles, role)
			roleSeen[role] = true
		}
		if !permSeen[permission] {
			p.Permissions = append(p.Permissions, permission)
			permSeen[permission] = true
		}
	}
	return rows.Err()
}

func effectiveKeyScopes(scopes, currentPermissions []string) []string {
	current := Principal{Permissions: currentPermissions}
	effective := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if current.Has(scope) {
			effective = append(effective, scope)
		}
	}
	return effective
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(token))
	return err
}

func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, current, next string) error {
	if len(next) < 12 {
		return errors.New("new password must contain at least 12 characters")
	}
	var hash *string
	if err := s.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, userID).Scan(&hash); err != nil {
		return err
	}
	if hash == nil || bcrypt.CompareHashAndPassword([]byte(*hash), []byte(current)) != nil {
		return errors.New("current password is incorrect")
	}
	nextHash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE users SET password_hash=$1,must_change_password=false,updated_at=now() WHERE id=$2`, string(nextHash), userID)
	return err
}

func (p Principal) Has(permission string) bool {
	for _, granted := range p.Permissions {
		if granted == "*" || granted == permission {
			return true
		}
		if strings.HasSuffix(granted, ":*") && strings.HasPrefix(permission, strings.TrimSuffix(granted, "*")) {
			return true
		}
	}
	return false
}

func (p Principal) Can(permission string, resource Resource) bool {
	if !p.Has(permission) {
		return false
	}
	if p.Has("*") {
		return true
	}
	if level(resource.Classification) >= level("executive") && !contains(p.Roles, "executive") {
		return false
	}
	if level(resource.Classification) >= level("restricted") {
		return resource.OrganizationID != nil && p.OrganizationID != nil && *resource.OrganizationID == *p.OrganizationID
	}
	return true
}

func level(classification string) int {
	switch strings.ToLower(classification) {
	case "public":
		return 0
	case "internal":
		return 1
	case "confidential":
		return 2
	case "executive":
		return 3
	case "restricted":
		return 4
	default:
		return 4
	}
}
func contains(values []string, wanted string) bool {
	for _, v := range values {
		if subtle.ConstantTimeCompare([]byte(v), []byte(wanted)) == 1 {
			return true
		}
	}
	return false
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Authenticate(r)
		if err != nil {
			http.Error(w, `{"error":"authentication_required"}`, http.StatusUnauthorized)
			return
		}
		if p.MustChangePassword && !passwordChangePath(r.URL.Path) {
			http.Error(w, `{"error":"password_change_required"}`, http.StatusPreconditionRequired)
			return
		}
		if p.AuthMethod != "api_key" && unsafeMethod(r.Method) && r.Header.Get("X-CSRF-Token") != p.CSRFToken {
			http.Error(w, `{"error":"invalid_csrf_token"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

func passwordChangePath(path string) bool {
	return path == "/api/v1/auth/me" || path == "/api/v1/auth/password" || path == "/api/v1/auth/logout"
}

func unsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func Require(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok || !p.Has(permission) {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// RequireAPIKeyScope preserves normal browser-session behavior while requiring
// an explicit scope when the caller authenticates with a personal key.
func RequireAPIKeyScope(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok || (p.AuthMethod == "api_key" && !p.Has(permission)) {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func RequireInteractive(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFrom(r.Context())
		if !ok || p.AuthMethod == "api_key" {
			http.Error(w, `{"error":"interactive_session_required"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func ScanNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
