package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

type keyPolicy struct {
	MaxLifetimeDays      int      `json:"maxLifetimeDays"`
	RotationOverlapHours int      `json:"rotationOverlapHours"`
	AllowedScopes        []string `json:"allowedScopes"`
}
type keyRequest struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	ExpiresInDays int      `json:"expiresInDays"`
}

func (s *Server) loadKeyPolicy(r *http.Request) keyPolicy {
	cfg := keyPolicy{MaxLifetimeDays: 90, RotationOverlapHours: 24, AllowedScopes: []string{"strategy:read", "kpi:read", "project:read", "decision:read", "dashboard:read"}}
	_, _ = s.getSetting(r.Context(), "security", "personal_keys", &cfg)
	if cfg.MaxLifetimeDays <= 0 {
		cfg.MaxLifetimeDays = 90
	}
	if cfg.RotationOverlapHours < 0 {
		cfg.RotationOverlapHours = 0
	}
	return cfg
}

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,name,prefix,scopes,expires_at,last_used_at,revoked_at,created_at,replaced_by FROM personal_access_keys WHERE user_id=$1 ORDER BY created_at DESC`, p.ID)
	if err != nil {
		writeError(w, 500, "key_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, prefix string
		var scopes []string
		var expires, last, revoked, created any
		var replaced *uuid.UUID
		if err := rows.Scan(&id, &name, &prefix, &scopes, &expires, &last, &revoked, &created, &replaced); err != nil {
			writeError(w, 500, "key_scan_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "prefix": prefix, "scopes": scopes, "expiresAt": expires, "lastUsedAt": last, "revokedAt": revoked, "createdAt": created, "replacedBy": replaced})
	}
	writeJSON(w, 200, items)
}

func (s *Server) getKeyPolicy(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	policy := s.loadKeyPolicy(r)
	allowed := make([]string, 0, len(policy.AllowedScopes))
	for _, scope := range policy.AllowedScopes {
		if p.Has(scope) {
			allowed = append(allowed, scope)
		}
	}
	policy.AllowedScopes = allowed
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body keyRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, 400, "invalid_key", err)
		return
	}
	token, prefix, scopes, expires, err := s.issueKey(r, p, body)
	if err != nil {
		writeError(w, 400, "key_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "personal_key", prefix, "create", "success", map[string]any{"scopes": scopes})
	writeJSON(w, 201, map[string]any{"token": token, "prefix": prefix, "scopes": scopes, "expiresAt": expires, "notice": "This key is displayed once. Store it securely."})
}

func (s *Server) issueKey(r *http.Request, p auth.Principal, body keyRequest) (string, string, []string, *time.Time, error) {
	if strings.TrimSpace(body.Name) == "" {
		return "", "", nil, nil, errors.New("name is required")
	}
	policy := s.loadKeyPolicy(r)
	if body.ExpiresInDays <= 0 {
		body.ExpiresInDays = policy.MaxLifetimeDays
	}
	if body.ExpiresInDays > policy.MaxLifetimeDays {
		return "", "", nil, nil, errors.New("requested lifetime exceeds policy")
	}
	scopes := sortedUnique(body.Scopes)
	if len(scopes) == 0 {
		scopes = []string{"strategy:read", "kpi:read"}
	}
	for _, scope := range scopes {
		if !p.Has(scope) {
			return "", "", nil, nil, errors.New("scope exceeds current permissions: " + scope)
		}
		if len(policy.AllowedScopes) > 0 && !stringIn(policy.AllowedScopes, scope) && !stringIn(policy.AllowedScopes, "*") {
			return "", "", nil, nil, errors.New("scope is not allowed by key policy: " + scope)
		}
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", nil, nil, err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	prefixBytes := make([]byte, 6)
	if _, err := rand.Read(prefixBytes); err != nil {
		return "", "", nil, nil, err
	}
	prefix := strings.ToLower(base64.RawURLEncoding.EncodeToString(prefixBytes))
	token := "plx_" + prefix + "." + secret
	hash := sha256.Sum256([]byte(token))
	expires := time.Now().Add(time.Duration(body.ExpiresInDays) * 24 * time.Hour)
	_, err := s.pool.Exec(r.Context(), `INSERT INTO personal_access_keys(id,user_id,name,prefix,secret_hash,scopes,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), p.ID, body.Name, prefix, hash[:], scopes, expires)
	return token, prefix, scopes, &expires, err
}

func (s *Server) rotateKey(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	oldID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_key_id", err)
		return
	}
	var body keyRequest
	_ = decodeJSON(r, &body)
	var name string
	var scopes []string
	var oldExpires *time.Time
	err = s.pool.QueryRow(r.Context(), `SELECT name,scopes,expires_at FROM personal_access_keys WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, oldID, p.ID).Scan(&name, &scopes, &oldExpires)
	if err != nil {
		writeError(w, 404, "key_not_found", nil)
		return
	}
	if body.Name == "" {
		body.Name = name + " (rotated)"
	}
	if len(body.Scopes) == 0 {
		body.Scopes = scopes
	}
	if body.ExpiresInDays == 0 && oldExpires != nil {
		days := int(time.Until(*oldExpires).Hours() / 24)
		if days > 0 {
			body.ExpiresInDays = days
		}
	}
	token, prefix, newScopes, expires, err := s.issueKey(r, p, body)
	if err != nil {
		writeError(w, 400, "key_rotate_failed", err)
		return
	}
	var newID uuid.UUID
	err = s.pool.QueryRow(r.Context(), `SELECT id FROM personal_access_keys WHERE prefix=$1 AND user_id=$2 ORDER BY created_at DESC LIMIT 1`, prefix, p.ID).Scan(&newID)
	if err != nil {
		writeError(w, 500, "key_rotate_failed", err)
		return
	}
	overlap := s.loadKeyPolicy(r).RotationOverlapHours
	_, err = s.pool.Exec(r.Context(), `UPDATE personal_access_keys SET replaced_by=$1,expires_at=LEAST(COALESCE(expires_at,$2),$2) WHERE id=$3 AND user_id=$4`, newID, time.Now().Add(time.Duration(overlap)*time.Hour), oldID, p.ID)
	if err != nil {
		writeError(w, 500, "key_rotate_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Update", "personal_key", oldID.String(), "rotate", "success", map[string]any{"newKeyId": newID, "overlapHours": overlap})
	writeJSON(w, 201, map[string]any{"token": token, "prefix": prefix, "scopes": newScopes, "expiresAt": expires, "previousKeyExpiresAt": time.Now().Add(time.Duration(overlap) * time.Hour), "notice": "This replacement key is displayed once."})
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_key_id", err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE personal_access_keys SET revoked_at=now() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, id, p.ID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, 404, "key_not_found", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Delete", "personal_key", id.String(), "revoke", "success", nil)
	w.WriteHeader(204)
}

func stringIn(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}
