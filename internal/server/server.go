package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hkjang/Planexus/internal/auth"
	"github.com/hkjang/Planexus/internal/secure"
	"github.com/hkjang/Planexus/internal/webui"
)

type Server struct {
	pool          *pgxpool.Pool
	auth          *auth.Service
	vault         *secure.Vault
	version       string
	loginMu       sync.Mutex
	loginAttempts map[string]loginAttempt
	apiRateMu     sync.Mutex
	apiRates      map[string]apiRateBucket
	apiRateLimit  int
	apiRatePolicy time.Time
	metricsMu     sync.Mutex
	metrics       map[metricKey]*metricValue
}

type loginAttempt struct {
	Failures     int
	BlockedUntil time.Time
	LastAttempt  time.Time
}

type apiRateBucket struct {
	WindowStart time.Time
	Count       int
}

type metricKey struct{ Method, Path, Status string }
type metricValue struct {
	Count           uint64
	DurationSeconds float64
}

func New(pool *pgxpool.Pool, vault *secure.Vault, version string) *Server {
	return &Server{pool: pool, auth: auth.New(pool), vault: vault, version: version, loginAttempts: map[string]loginAttempt{}, apiRates: map[string]apiRateBucket{}, apiRateLimit: 600, metrics: map[metricKey]*metricValue{}}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(s.securityHeaders)
	r.Use(s.observeHTTP)
	r.Use(s.requestTimeout)
	r.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "live"}) })
	r.Get("/health/ready", s.readiness)
	r.Get("/api/v1/version", s.versionInfo)
	r.Get("/metrics", s.prometheusMetrics)
	r.Get("/openapi.yaml", s.openAPI)
	r.Get("/api/v1/auth/config", s.authConfig)
	r.Post("/api/v1/auth/login", s.localLogin)
	r.Get("/api/v1/auth/oidc/login", s.oidcLogin)
	r.Get("/api/v1/auth/oidc/callback", s.oidcCallback)

	r.Group(func(api chi.Router) {
		api.Use(s.auth.Middleware)
		api.Use(s.apiRateLimitMiddleware)
		api.Use(s.auditViews)
		api.Get("/api/v1/auth/me", s.me)
		api.Post("/api/v1/auth/logout", s.logout)
		api.Put("/api/v1/auth/password", auth.RequireInteractive(s.changePassword))
		api.Get("/api/v1/dashboard/executive", auth.Require("dashboard:read", s.executiveDashboard))
		api.Get("/api/v1/dashboard/personal", auth.RequireAPIKeyScope("dashboard:read", s.personalDashboard))
		api.Get("/api/v1/search", s.globalSearch)
		api.Get("/api/v1/strategies", auth.Require("strategy:read", s.listStrategies))
		api.Post("/api/v1/strategies", auth.Require("strategy:*", s.createStrategy))
		api.Get("/api/v1/kpis", auth.Require("kpi:read", s.listKPIs))
		api.Post("/api/v1/kpis", auth.Require("kpi:*", s.createKPI))
		api.Get("/api/v1/projects", auth.Require("project:read", s.listProjects))
		api.Post("/api/v1/projects", auth.Require("project:*", s.createProject))
		api.Get("/api/v1/plans", auth.Require("plan:own", s.listPlans))
		api.Post("/api/v1/plans", auth.Require("plan:own", s.createPlan))
		api.Post("/api/v1/plans/{id}/submit", auth.Require("plan:own", s.submitPlan))
		api.Get("/api/v1/workflow/tasks", auth.RequireAPIKeyScope("approval:*", s.listWorkflowTasks))
		api.Post("/api/v1/workflow/tasks/{id}/action", auth.RequireAPIKeyScope("approval:*", s.actWorkflowTask))
		api.Get("/api/v1/decisions", auth.Require("decision:read", s.listDecisions))
		api.Post("/api/v1/decisions", auth.Require("decision:*", s.createDecision))
		api.Get("/api/v1/intelligence", auth.Require("intelligence:read", s.listIntelligence))
		api.Post("/api/v1/intelligence", auth.Require("intelligence:*", s.createIntelligence))
		api.Get("/api/v1/scenarios", auth.Require("scenario:read", s.listScenarios))
		api.Post("/api/v1/scenarios", auth.Require("scenario:*", s.createScenario))
		api.Post("/api/v1/scenarios/{id}/run", auth.Require("scenario:*", s.runScenario))
		api.Post("/api/v1/ai/query", auth.Require("ai:query", s.aiQuery))
		api.Get("/api/v1/import/templates/{entityType}", auth.Require("import:*", s.downloadImportTemplate))
		api.Post("/api/v1/import/preview", auth.Require("import:*", s.previewImport))
		api.Post("/api/v1/import/{id}/commit", auth.Require("import:*", s.commitImport))
		api.Post("/api/v1/import/{id}/rollback", auth.Require("import:*", s.rollbackImport))
		api.Get("/api/v1/import/history", auth.Require("import:*", s.listImports))
		api.Get("/api/v1/notification-rules", auth.RequireAPIKeyScope("notification:manage", s.listNotificationRules))
		api.Post("/api/v1/notification-rules", auth.RequireAPIKeyScope("notification:manage", s.createNotificationRule))
		api.Delete("/api/v1/notification-rules/{id}", auth.RequireAPIKeyScope("notification:manage", s.deleteNotificationRule))
		api.Get("/api/v1/notifications", auth.RequireAPIKeyScope("notification:read", s.listNotifications))
		api.Post("/api/v1/notifications/{id}/read", auth.RequireAPIKeyScope("notification:read", s.readNotification))
		api.Post("/api/v1/admin/notifications/evaluate", auth.Require("*", s.evaluateNotificationsHandler))
		api.Get("/api/v1/keys", auth.RequireInteractive(s.listKeys))
		api.Get("/api/v1/keys/policy", auth.RequireInteractive(s.getKeyPolicy))
		api.Post("/api/v1/keys", auth.RequireInteractive(s.createKey))
		api.Post("/api/v1/keys/{id}/rotate", auth.RequireInteractive(s.rotateKey))
		api.Delete("/api/v1/keys/{id}", auth.RequireInteractive(s.revokeKey))
		api.Get("/api/v1/admin/settings", auth.Require("*", s.listSettings))
		api.Put("/api/v1/admin/settings/{category}/{key}", auth.Require("*", s.putSetting))
		api.Delete("/api/v1/admin/settings/{category}/{key}", auth.Require("*", s.deleteSetting))
		api.Get("/api/v1/admin/authentication/oidc", auth.Require("*", s.getOIDCAdmin))
		api.Put("/api/v1/admin/authentication/oidc", auth.Require("*", s.putOIDCAdmin))
		api.Get("/api/v1/admin/ai", auth.Require("*", s.getAIAdmin))
		api.Put("/api/v1/admin/ai", auth.Require("*", s.putAIAdmin))
		api.Get("/api/v1/admin/ai/usage", auth.Require("*", s.getAIUsage))
		api.Get("/api/v1/admin/workflows", auth.Require("*", s.getWorkflows))
		api.Put("/api/v1/admin/workflows/{resourceType}", auth.Require("*", s.putWorkflow))
		api.Get("/api/v1/admin/audit", auth.Require("audit:read", s.listAudit))
		api.Get("/api/v1/admin/users", auth.Require("*", s.listUsers))
		api.Put("/api/v1/admin/users/{id}", auth.Require("*", s.updateUserAdmin))
		api.Post("/api/v1/admin/users/{id}/roles", auth.Require("*", s.setUserRoles))
		api.Get("/api/v1/admin/organizations", auth.Require("*", s.listOrganizations))
		api.Post("/api/v1/admin/organizations", auth.Require("*", s.createOrganization))
		api.Put("/api/v1/admin/organizations/{id}", auth.Require("*", s.updateOrganization))
		api.Get("/api/v1/admin/roles", auth.Require("*", s.listRoles))
		api.Put("/api/v1/admin/roles/{id}", auth.Require("*", s.updateRole))
		api.Get("/api/v1/admin/health", auth.Require("*", s.systemHealth))
		api.Get("/api/v1/admin/backups", auth.Require("*", s.listBackups))
		api.Get("/api/v1/admin/backups/export", auth.Require("*", s.exportBackup))
		api.Post("/api/v1/admin/backups/validate", auth.Require("*", s.validateBackup))
		api.Post("/api/v1/admin/backups/restore", auth.Require("*", s.restoreBackup))
		api.Handle("/mcp", s.mcpHandler())
	})

	s.mountFrontend(r)
	return r
}

func (s *Server) apiRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		limit := s.loadAPIRateLimit(r.Context())
		allowed, retry := s.consumeAPIRate(p.ID.String(), limit, time.Now())
		if !allowed {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retry.Seconds()))
			writeError(w, http.StatusTooManyRequests, "api_rate_limited", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) loadAPIRateLimit(ctx context.Context) int {
	s.apiRateMu.Lock()
	defer s.apiRateMu.Unlock()
	if s.apiRateLimit <= 0 {
		s.apiRateLimit = 600
	}
	if time.Since(s.apiRatePolicy) < 30*time.Second {
		return s.apiRateLimit
	}
	var policy struct {
		RequestsPerMinute int `json:"requestsPerMinute"`
	}
	if ok, err := s.getSetting(ctx, "system", "api_rate_limit", &policy); err == nil && ok && policy.RequestsPerMinute >= 60 && policy.RequestsPerMinute <= 10000 {
		s.apiRateLimit = policy.RequestsPerMinute
	} else {
		s.apiRateLimit = 600
	}
	s.apiRatePolicy = time.Now()
	for key, bucket := range s.apiRates {
		if time.Since(bucket.WindowStart) > 2*time.Minute {
			delete(s.apiRates, key)
		}
	}
	return s.apiRateLimit
}

func (s *Server) consumeAPIRate(key string, limit int, now time.Time) (bool, time.Duration) {
	s.apiRateMu.Lock()
	defer s.apiRateMu.Unlock()
	bucket := s.apiRates[key]
	if bucket.WindowStart.IsZero() || now.Sub(bucket.WindowStart) >= time.Minute {
		bucket = apiRateBucket{WindowStart: now}
	}
	if bucket.Count >= limit {
		retry := time.Minute - now.Sub(bucket.WindowStart)
		if retry < time.Second {
			retry = time.Second
		}
		return false, retry
	}
	bucket.Count++
	s.apiRates[key] = bucket
	return true, 0
}

func (s *Server) invalidateAPIRatePolicy() {
	s.apiRateMu.Lock()
	s.apiRatePolicy = time.Time{}
	s.apiRateMu.Unlock()
}

func (s *Server) requestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout := 30 * time.Second
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/backups/") {
			timeout = 10 * time.Minute
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mountFrontend(r chi.Router) {
	dist, err := fs.Sub(webui.Dist, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(dist))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path != "" {
			if f, err := dist.Open(path); err == nil {
				_ = f.Close()
				files.ServeHTTP(w, req)
				return
			}
		}
		content, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.Error(w, "UI unavailable", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(content)
	})
}

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.pool.Ping(ctx); err != nil {
		writeError(w, 503, "database_not_ready", err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ready"})
}

func (s *Server) versionInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]string{"service": "Planexus", "version": s.version})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	writeJSON(w, 200, p)
}

func (s *Server) localLogin(w http.ResponseWriter, r *http.Request) {
	var body struct{ Username, Password string }
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, 400, "invalid_request", err)
		return
	}
	loginKey := remoteIP(r) + "/" + strings.ToLower(strings.TrimSpace(body.Username))
	if wait := s.loginBlocked(loginKey); wait > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", wait.Seconds()))
		writeError(w, 429, "login_rate_limited", nil)
		return
	}
	p, token, err := s.auth.LocalLogin(r.Context(), body.Username, body.Password)
	if err != nil {
		s.loginFailed(loginKey)
		s.audit(r, nil, body.Username, "Login", "session", "", "login", "failure", nil)
		writeError(w, 401, "invalid_credentials", nil)
		return
	}
	s.loginSucceeded(loginKey)
	setSessionCookie(w, r, token, 12*time.Hour)
	s.audit(r, &p.ID, p.Username, "Login", "session", "", "login", "success", map[string]any{"method": "local"})
	writeJSON(w, 200, p)
}

func (s *Server) loginBlocked(key string) time.Duration {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	attempt := s.loginAttempts[key]
	if time.Now().Before(attempt.BlockedUntil) {
		return time.Until(attempt.BlockedUntil)
	}
	return 0
}
func (s *Server) loginFailed(key string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	attempt := s.loginAttempts[key]
	if now.Sub(attempt.LastAttempt) > 15*time.Minute {
		attempt.Failures = 0
	}
	attempt.Failures++
	attempt.LastAttempt = now
	if attempt.Failures >= 5 {
		attempt.BlockedUntil = now.Add(15 * time.Minute)
	}
	s.loginAttempts[key] = attempt
	for k, v := range s.loginAttempts {
		if now.Sub(v.LastAttempt) > time.Hour {
			delete(s.loginAttempts, k)
		}
	}
}
func (s *Server) loginSucceeded(key string) {
	s.loginMu.Lock()
	delete(s.loginAttempts, key)
	s.loginMu.Unlock()
}

func (s *Server) auditViews(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		if wrapped.Status() >= 400 {
			return
		}
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			return
		}
		resource := strings.TrimPrefix(r.URL.Path, "/api/v1/")
		if resource == "" {
			resource = "api"
		}
		s.audit(r, &p.ID, p.Username, "View", strings.Split(resource, "/")[0], resource, "read", "success", nil)
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	if c, err := r.Cookie(auth.CookieName); err == nil {
		_ = s.auth.Logout(r.Context(), c.Value)
	}
	setSessionCookie(w, r, "", -time.Hour)
	s.audit(r, &p.ID, p.Username, "Logout", "session", "", "logout", "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		Current string `json:"currentPassword"`
		Next    string `json:"newPassword"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, 400, "invalid_request", err)
		return
	}
	if err := s.auth.ChangePassword(r.Context(), p.ID, body.Current, body.Next); err != nil {
		writeError(w, 400, "password_change_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Update", "user", p.ID.String(), "change_password", "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string, maxAge time.Duration) {
	secureCookie := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: value, Path: "/", HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: int(maxAge.Seconds())})
}

func (s *Server) audit(r *http.Request, actorID *uuid.UUID, actorName, eventType, resourceType, resourceID, action, outcome string, details any) {
	var data []byte
	if details == nil {
		data = []byte(`{}`)
	} else {
		data, _ = json.Marshal(details)
	}
	_, err := s.pool.Exec(r.Context(), `INSERT INTO audit_logs(id,actor_id,actor_name,event_type,resource_type,resource_id,action,outcome,ip_address,user_agent,details) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,NULLIF($9,'')::inet,$10,$11)`, uuid.New(), actorID, actorName, eventType, resourceType, resourceID, action, outcome, remoteIP(r), r.UserAgent(), data)
	if err != nil {
		slog.Error("write audit log", "error", err, "action", action)
	}
}

func remoteIP(r *http.Request) string {
	value := r.RemoteAddr
	if i := strings.LastIndex(value, ":"); i > 0 {
		value = value[:i]
	}
	return strings.Trim(value, "[]")
}

func decodeJSON(r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code string, err error) {
	body := map[string]string{"error": code}
	if err != nil && status < 500 {
		body["message"] = err.Error()
	}
	writeJSON(w, status, body)
}
func parseUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid id")
	}
	return id, nil
}
func required(values ...string) error {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return errors.New("required field is empty")
		}
	}
	return nil
}
