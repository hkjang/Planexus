package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

const oidcStateCookie = "planexus_oidc_state"

type OIDCSettings struct {
	Enabled      bool              `json:"enabled"`
	DisplayName  string            `json:"displayName"`
	IssuerURL    string            `json:"issuerUrl"`
	ClientID     string            `json:"clientId"`
	ClientSecret string            `json:"clientSecret"`
	Scopes       []string          `json:"scopes"`
	RedirectURL  string            `json:"redirectUrl,omitempty"`
	GroupMapping map[string]string `json:"groupMapping,omitempty"`
	RoleMapping  map[string]string `json:"roleMapping,omitempty"`
}

type oidcState struct {
	State     string    `json:"state"`
	Verifier  string    `json:"verifier"`
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (s *Server) loadOIDC(ctx context.Context) (OIDCSettings, error) {
	var cfg OIDCSettings
	ok, err := s.getSetting(ctx, "authentication", "oidc", &cfg)
	if err != nil {
		return cfg, err
	}
	if !ok || !cfg.Enabled {
		return cfg, errors.New("OIDC is not enabled")
	}
	if err := required(cfg.IssuerURL, cfg.ClientID, cfg.ClientSecret); err != nil {
		return cfg, fmt.Errorf("incomplete OIDC configuration: %w", err)
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = "Company SSO"
	}
	return cfg, nil
}

func callbackURL(r *http.Request, cfg OIDCSettings) string {
	if cfg.RedirectURL != "" {
		return cfg.RedirectURL
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/api/v1/auth/oidc/callback"
}

func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadOIDC(r.Context())
	if err != nil {
		writeError(w, 503, "oidc_unavailable", err)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), cfg.IssuerURL)
	if err != nil {
		writeError(w, 502, "oidc_discovery_failed", err)
		return
	}
	state, err := randomURLToken(24)
	if err != nil {
		writeError(w, 500, "oidc_state_failed", err)
		return
	}
	verifier := oauth2.GenerateVerifier()
	nonce, err := randomURLToken(32)
	if err != nil {
		writeError(w, 500, "oidc_state_failed", err)
		return
	}
	record, _ := json.Marshal(oidcState{State: state, Verifier: verifier, Nonce: nonce, ExpiresAt: time.Now().Add(10 * time.Minute)})
	envelope, err := s.vault.Encrypt(record, "oidc-state")
	if err != nil {
		writeError(w, 500, "oidc_state_failed", err)
		return
	}
	secureCookie := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookie, Value: url.QueryEscape(envelope), Path: "/api/v1/auth/oidc/callback", HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	oauthCfg := oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: callbackURL(r, cfg), Scopes: cfg.Scopes}
	http.Redirect(w, r, oauthCfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(oidcStateCookie)
	if err != nil {
		writeError(w, 400, "oidc_state_missing", nil)
		return
	}
	envelope, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		writeError(w, 400, "oidc_state_invalid", nil)
		return
	}
	plain, err := s.vault.Decrypt(envelope, "oidc-state")
	if err != nil {
		writeError(w, 400, "oidc_state_invalid", nil)
		return
	}
	var state oidcState
	if json.Unmarshal(plain, &state) != nil || time.Now().After(state.ExpiresAt) || state.State != r.URL.Query().Get("state") {
		writeError(w, 400, "oidc_state_invalid", nil)
		return
	}
	cfg, err := s.loadOIDC(r.Context())
	if err != nil {
		writeError(w, 503, "oidc_unavailable", err)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), cfg.IssuerURL)
	if err != nil {
		writeError(w, 502, "oidc_discovery_failed", err)
		return
	}
	oauthCfg := oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: callbackURL(r, cfg), Scopes: cfg.Scopes}
	token, err := oauthCfg.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(state.Verifier))
	if err != nil {
		writeError(w, 401, "oidc_exchange_failed", err)
		return
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok {
		writeError(w, 401, "oidc_token_missing", nil)
		return
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(r.Context(), rawID)
	if err != nil {
		writeError(w, 401, "oidc_token_invalid", err)
		return
	}
	if idToken.Nonce == "" || idToken.Nonce != state.Nonce {
		writeError(w, 401, "oidc_nonce_invalid", nil)
		return
	}
	var claims struct {
		Subject     string   `json:"sub"`
		Username    string   `json:"preferred_username"`
		Name        string   `json:"name"`
		Email       string   `json:"email"`
		Groups      []string `json:"groups"`
		RealmAccess struct {
			Roles []string `json:"roles"`
		} `json:"realm_access"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Subject == "" {
		writeError(w, 401, "oidc_claims_invalid", err)
		return
	}
	// OIDC subject identifiers are unique only within an issuer. Scope the stored
	// identity so changing Keycloak realms cannot bind an existing user by sub alone.
	userID, err := s.upsertOIDCUser(r.Context(), idToken.Issuer+"|"+claims.Subject, claims.Username, claims.Name, claims.Email)
	if err != nil {
		writeError(w, 500, "oidc_user_failed", err)
		return
	}
	roles := []string{}
	for _, g := range claims.Groups {
		roles = append(roles, cfg.GroupMapping[g])
	}
	for _, role := range claims.RealmAccess.Roles {
		roles = append(roles, cfg.RoleMapping[role])
	}
	if err = s.assignMappedRoles(r.Context(), userID, roles); err != nil {
		writeError(w, 500, "oidc_role_sync_failed", err)
		return
	}
	p, sessionToken, err := s.auth.CreateOIDCSession(r.Context(), userID)
	if err != nil {
		writeError(w, 500, "oidc_session_failed", err)
		return
	}
	setSessionCookie(w, r, sessionToken, 12*time.Hour)
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookie, Value: "", Path: "/api/v1/auth/oidc/callback", MaxAge: -1, HttpOnly: true})
	s.audit(r, &p.ID, p.Username, "Login", "session", "", "login", "success", map[string]any{"method": "oidc"})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) upsertOIDCUser(ctx context.Context, subject, username, name, email string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE oidc_subject=$1`, subject).Scan(&id)
	if err == nil {
		_, err = s.pool.Exec(ctx, `UPDATE users SET display_name=COALESCE(NULLIF($2,''),display_name),email=COALESCE(NULLIF($3,''),email),updated_at=now() WHERE id=$1`, id, name, email)
		return id, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	if username == "" {
		username = email
	}
	if username == "" {
		username = "oidc-" + subject
	}
	var exists bool
	_ = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE lower(username)=lower($1))`, username).Scan(&exists)
	if exists {
		identityHash := sha256.Sum256([]byte(subject))
		username = fmt.Sprintf("%s-%x", username, identityHash[:4])
	}
	if name == "" {
		name = username
	}
	id = uuid.New()
	_, err = s.pool.Exec(ctx, `INSERT INTO users(id,username,display_name,email,oidc_subject) VALUES($1,$2,$3,NULLIF($4,''),$5)`, id, username, name, email, subject)
	if err == nil {
		_, _ = s.pool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,'general_user') ON CONFLICT DO NOTHING`, id)
	}
	return id, err
}

func (s *Server) assignMappedRoles(ctx context.Context, userID uuid.UUID, roles []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1 AND scope->>'source'='oidc'`, userID); err != nil {
		return err
	}
	for _, role := range sortedUnique(roles) {
		if role == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_id,scope) SELECT $1,id,'{"source":"oidc"}'::jsonb FROM roles WHERE id=$2 ON CONFLICT(user_id,role_id) DO UPDATE SET scope=EXCLUDED.scope`, userID, role); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func randomURLToken(size int) (string, error) { return oauth2.GenerateVerifier()[:size], nil }
