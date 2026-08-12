package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

type settingRecord struct {
	Category  string          `json:"category"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value,omitempty"`
	Sensitive bool            `json:"sensitive"`
	Version   int64           `json:"version"`
	UpdatedAt string          `json:"updatedAt"`
}

func (s *Server) getSetting(ctx context.Context, category, key string, target any) (bool, error) {
	var plain []byte
	var encrypted *string
	var sensitive bool
	err := s.pool.QueryRow(ctx, `SELECT value::text,encrypted_value,sensitive FROM system_settings WHERE category=$1 AND key=$2`, category, key).Scan(&plain, &encrypted, &sensitive)
	if err != nil {
		if auth.ScanNoRows(err) {
			return false, nil
		}
		return false, err
	}
	if sensitive {
		if encrypted == nil {
			return false, errors.New("sensitive setting is corrupt")
		}
		decrypted, err := s.vault.Decrypt(*encrypted, category+"/"+key)
		if err != nil {
			return false, err
		}
		plain = decrypted
	}
	return true, json.Unmarshal(plain, target)
}

func (s *Server) listSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT category,key,CASE WHEN sensitive THEN NULL ELSE value END,sensitive,version,updated_at::text FROM system_settings ORDER BY category,key`)
	if err != nil {
		writeError(w, 500, "settings_query_failed", err)
		return
	}
	defer rows.Close()
	items := []settingRecord{}
	for rows.Next() {
		var item settingRecord
		var value []byte
		if err := rows.Scan(&item.Category, &item.Key, &value, &item.Sensitive, &item.Version, &item.UpdatedAt); err != nil {
			writeError(w, 500, "settings_scan_failed", err)
			return
		}
		if !item.Sensitive {
			item.Value = value
		}
		items = append(items, item)
	}
	writeJSON(w, 200, items)
}

func (s *Server) putSetting(w http.ResponseWriter, r *http.Request) {
	category, key := chi.URLParam(r, "category"), chi.URLParam(r, "key")
	var body struct {
		Value     json.RawMessage `json:"value"`
		Sensitive bool            `json:"sensitive"`
	}
	if err := decodeJSON(r, &body); err != nil || len(body.Value) == 0 {
		writeError(w, 400, "invalid_setting", err)
		return
	}
	if !json.Valid(body.Value) {
		writeError(w, 400, "invalid_setting_json", nil)
		return
	}
	if !settingNamePattern.MatchString(category) || !settingNamePattern.MatchString(key) {
		writeError(w, 400, "invalid_setting_name", nil)
		return
	}
	if err := validateKnownSetting(category, key, body.Value); err != nil {
		writeError(w, 400, "invalid_setting_value", err)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	var plain any
	var encrypted any
	if body.Sensitive {
		value, err := s.vault.Encrypt(body.Value, category+"/"+key)
		if err != nil {
			writeError(w, 500, "setting_encrypt_failed", err)
			return
		}
		encrypted = value
	} else {
		plain = body.Value
	}
	_, err := s.pool.Exec(r.Context(), `INSERT INTO system_settings(category,key,value,encrypted_value,sensitive,updated_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(category,key) DO UPDATE SET value=EXCLUDED.value,encrypted_value=EXCLUDED.encrypted_value,sensitive=EXCLUDED.sensitive,version=system_settings.version+1,updated_by=EXCLUDED.updated_by,updated_at=now()`, category, key, plain, encrypted, body.Sensitive, p.ID)
	if err != nil {
		writeError(w, 500, "setting_write_failed", err)
		return
	}
	if category == "system" && key == "api_rate_limit" {
		s.invalidateAPIRatePolicy()
	}
	s.audit(r, &p.ID, p.Username, "Configuration Change", "setting", category+"/"+key, "update", "success", map[string]any{"sensitive": body.Sensitive})
	w.WriteHeader(http.StatusNoContent)
}

var settingNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func validateKnownSetting(category, key string, value json.RawMessage) error {
	switch category + "/" + key {
	case "system/session":
		var policy struct {
			TimeoutMinutes int `json:"timeoutMinutes"`
		}
		if err := json.Unmarshal(value, &policy); err != nil || policy.TimeoutMinutes < 15 || policy.TimeoutMinutes > 720 {
			return errors.New("timeoutMinutes must be between 15 and 720")
		}
	case "system/api_rate_limit":
		var policy struct {
			RequestsPerMinute int `json:"requestsPerMinute"`
		}
		if err := json.Unmarshal(value, &policy); err != nil || policy.RequestsPerMinute < 60 || policy.RequestsPerMinute > 10000 {
			return errors.New("requestsPerMinute must be between 60 and 10000")
		}
	case "security/personal_keys":
		var policy keyPolicy
		if err := json.Unmarshal(value, &policy); err != nil {
			return errors.New("invalid personal key policy")
		}
		if policy.MaxLifetimeDays < 1 || policy.MaxLifetimeDays > 3650 || policy.RotationOverlapHours < 0 || policy.RotationOverlapHours > 720 || len(policy.AllowedScopes) == 0 {
			return errors.New("personal key policy is outside allowed bounds")
		}
		for _, scope := range policy.AllowedScopes {
			if scope != "*" && !permissionPattern.MatchString(scope) {
				return errors.New("invalid allowed scope: " + scope)
			}
		}
	case "integration/mcp":
		var policy mcpPolicy
		if err := json.Unmarshal(value, &policy); err != nil {
			return errors.New("invalid MCP policy")
		}
		known := map[string]bool{}
		for _, tool := range mcpTools() {
			known[tool.Name] = true
		}
		for _, tool := range policy.AllowedTools {
			if !known[tool] {
				return errors.New("unknown MCP tool: " + tool)
			}
		}
	}
	return nil
}

func (s *Server) deleteSetting(w http.ResponseWriter, r *http.Request) {
	category, key := chi.URLParam(r, "category"), chi.URLParam(r, "key")
	p, _ := auth.PrincipalFrom(r.Context())
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM system_settings WHERE category=$1 AND key=$2`, category, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "setting_delete_failed", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "setting_not_found", nil)
		return
	}
	if category == "system" && key == "api_rate_limit" {
		s.invalidateAPIRatePolicy()
	}
	s.audit(r, &p.ID, p.Username, "Configuration Change", "setting", category+"/"+key, "delete", "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	var cfg OIDCSettings
	ok, err := s.getSetting(r.Context(), "authentication", "oidc", &cfg)
	if err != nil {
		writeError(w, 500, "auth_config_failed", err)
		return
	}
	writeJSON(w, 200, map[string]any{"localEnabled": true, "oidcEnabled": ok && cfg.Enabled, "oidcLabel": cfg.DisplayName})
}

func (s *Server) getOIDCAdmin(w http.ResponseWriter, r *http.Request) {
	var cfg OIDCSettings
	ok, err := s.getSetting(r.Context(), "authentication", "oidc", &cfg)
	if err != nil {
		writeError(w, 500, "oidc_setting_failed", err)
		return
	}
	if !ok {
		cfg = OIDCSettings{DisplayName: "Company SSO", Scopes: []string{"openid", "profile", "email", "groups"}, GroupMapping: map[string]string{}, RoleMapping: map[string]string{}}
	}
	if cfg.ClientSecret != "" {
		cfg.ClientSecret = "********"
	}
	writeJSON(w, 200, cfg)
}

func (s *Server) putOIDCAdmin(w http.ResponseWriter, r *http.Request) {
	var cfg OIDCSettings
	if err := decodeJSON(r, &cfg); err != nil {
		writeError(w, 400, "invalid_oidc_setting", err)
		return
	}
	var current OIDCSettings
	_, _ = s.getSetting(r.Context(), "authentication", "oidc", &current)
	if cfg.ClientSecret == "" || cfg.ClientSecret == "********" {
		cfg.ClientSecret = current.ClientSecret
	}
	if cfg.Enabled {
		if err := required(cfg.IssuerURL, cfg.ClientID, cfg.ClientSecret); err != nil {
			writeError(w, 400, "incomplete_oidc_setting", err)
			return
		}
	}
	if cfg.DisplayName == "" {
		cfg.DisplayName = "Company SSO"
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{"openid", "profile", "email", "groups"}
	}
	plain, _ := json.Marshal(cfg)
	encrypted, err := s.vault.Encrypt(plain, "authentication/oidc")
	if err != nil {
		writeError(w, 500, "setting_encrypt_failed", err)
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	_, err = s.pool.Exec(r.Context(), `INSERT INTO system_settings(category,key,value,encrypted_value,sensitive,updated_by) VALUES('authentication','oidc',NULL,$1,true,$2) ON CONFLICT(category,key) DO UPDATE SET value=NULL,encrypted_value=EXCLUDED.encrypted_value,sensitive=true,version=system_settings.version+1,updated_by=EXCLUDED.updated_by,updated_at=now()`, encrypted, p.ID)
	if err != nil {
		writeError(w, 500, "setting_write_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Configuration Change", "setting", "authentication/oidc", "update", "success", map[string]any{"enabled": cfg.Enabled, "issuer": cfg.IssuerURL})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getWorkflows(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT resource_type,enabled,steps,rejection_policy,updated_at FROM workflow_definitions ORDER BY resource_type`)
	if err != nil {
		writeError(w, 500, "workflow_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var resource, policy string
		var enabled bool
		var steps json.RawMessage
		var updated any
		if err := rows.Scan(&resource, &enabled, &steps, &policy, &updated); err != nil {
			writeError(w, 500, "workflow_scan_failed", err)
			return
		}
		items = append(items, map[string]any{"resourceType": resource, "enabled": enabled, "steps": steps, "rejectionPolicy": policy, "updatedAt": updated})
	}
	writeJSON(w, 200, items)
}

func (s *Server) putWorkflow(w http.ResponseWriter, r *http.Request) {
	resource := chi.URLParam(r, "resourceType")
	var body struct {
		Enabled         bool            `json:"enabled"`
		Steps           json.RawMessage `json:"steps"`
		RejectionPolicy string          `json:"rejectionPolicy"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, 400, "invalid_workflow", err)
		return
	}
	if len(body.Steps) == 0 {
		body.Steps = []byte(`[]`)
	}
	var steps []workflowStep
	if err := json.Unmarshal(body.Steps, &steps); err != nil {
		writeError(w, 400, "invalid_workflow_steps", err)
		return
	}
	if body.Enabled && len(steps) == 0 {
		writeError(w, 400, "workflow_steps_required", errors.New("enabled workflow requires at least one step"))
		return
	}
	for _, step := range steps {
		if !oneOf(step.Type, "department_review", "planning_review", "approval") {
			writeError(w, 400, "invalid_workflow_step", errors.New("unsupported workflow step: "+step.Type))
			return
		}
	}
	if body.RejectionPolicy == "" {
		body.RejectionPolicy = "return_to_author"
	}
	p, _ := auth.PrincipalFrom(r.Context())
	_, err := s.pool.Exec(r.Context(), `INSERT INTO workflow_definitions(id,resource_type,enabled,steps,rejection_policy,updated_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(resource_type) DO UPDATE SET enabled=EXCLUDED.enabled,steps=EXCLUDED.steps,rejection_policy=EXCLUDED.rejection_policy,updated_by=EXCLUDED.updated_by,updated_at=now()`, uuid.New(), resource, body.Enabled, body.Steps, body.RejectionPolicy, p.ID)
	if err != nil {
		writeError(w, 500, "workflow_write_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Configuration Change", "workflow", resource, "update", "success", map[string]any{"enabled": body.Enabled})
	w.WriteHeader(204)
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
