package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
	"github.com/jackc/pgx/v5"
)

type notificationRule struct {
	ID              uuid.UUID `json:"id"`
	OwnerID         uuid.UUID `json:"ownerId"`
	Name            string    `json:"name"`
	EntityType      string    `json:"entityType"`
	ConditionField  string    `json:"conditionField"`
	Operator        string    `json:"operator"`
	Threshold       string    `json:"threshold"`
	Severity        string    `json:"severity"`
	Channels        []string  `json:"channels"`
	Global          bool      `json:"global"`
	Enabled         bool      `json:"enabled"`
	CooldownMinutes int       `json:"cooldownMinutes"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
type notificationCandidate struct {
	ID        uuid.UUID
	Name      string
	Value     float64
	TextValue string
}

func (s *Server) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.evaluateNotifications(ctx)
			}
		}
	}()
}

func (s *Server) listNotificationRules(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,owner_id,name,entity_type,condition_field,operator,threshold,severity,channels,global,enabled,cooldown_minutes,created_at,updated_at FROM notification_rules WHERE owner_id=$1 OR $2 ORDER BY updated_at DESC`, p.ID, p.Has("*"))
	if err != nil {
		writeError(w, 500, "notification_rule_query_failed", err)
		return
	}
	defer rows.Close()
	items := []notificationRule{}
	for rows.Next() {
		var x notificationRule
		if err = rows.Scan(&x.ID, &x.OwnerID, &x.Name, &x.EntityType, &x.ConditionField, &x.Operator, &x.Threshold, &x.Severity, &x.Channels, &x.Global, &x.Enabled, &x.CooldownMinutes, &x.CreatedAt, &x.UpdatedAt); err != nil {
			writeError(w, 500, "notification_rule_scan_failed", err)
			return
		}
		items = append(items, x)
	}
	writeJSON(w, 200, items)
}

func (s *Server) createNotificationRule(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		Name            string   `json:"name"`
		EntityType      string   `json:"entityType"`
		ConditionField  string   `json:"conditionField"`
		Operator        string   `json:"operator"`
		Threshold       string   `json:"threshold"`
		Severity        string   `json:"severity"`
		Channels        []string `json:"channels"`
		Global          bool     `json:"global"`
		CooldownMinutes int      `json:"cooldownMinutes"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Name, body.EntityType, body.ConditionField, body.Operator, body.Threshold) != nil {
		writeError(w, 400, "invalid_notification_rule", err)
		return
	}
	if !oneOf(body.EntityType, "kpi", "project", "intelligence", "key") || !validNotificationField(body.EntityType, body.ConditionField) || !oneOf(body.Operator, "lt", "lte", "eq", "gte", "gt") {
		writeError(w, 400, "invalid_notification_condition", nil)
		return
	}
	if body.ConditionField == "risk" {
		if !oneOf(strings.ToLower(body.Threshold), "low", "medium", "high", "critical") {
			writeError(w, 400, "invalid_notification_threshold", nil)
			return
		}
	} else if _, parseErr := strconv.ParseFloat(strings.TrimSpace(body.Threshold), 64); parseErr != nil {
		writeError(w, 400, "invalid_notification_threshold", parseErr)
		return
	}
	if body.Severity == "" {
		body.Severity = "warning"
	}
	if !oneOf(body.Severity, "info", "warning", "critical") {
		writeError(w, 400, "invalid_notification_severity", nil)
		return
	}
	if len(body.Channels) == 0 {
		body.Channels = []string{"system"}
	}
	for _, channel := range body.Channels {
		if !oneOf(channel, "system", "email", "messenger", "webhook") {
			writeError(w, 400, "invalid_notification_channel", nil)
			return
		}
	}
	if body.Global && !p.Has("*") {
		writeError(w, 403, "forbidden", nil)
		return
	}
	if body.CooldownMinutes <= 0 {
		body.CooldownMinutes = 1440
	}
	if body.CooldownMinutes > 43200 {
		writeError(w, 400, "invalid_notification_cooldown", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO notification_rules(id,owner_id,name,entity_type,condition_field,operator,threshold,severity,channels,global,cooldown_minutes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$2)`, id, p.ID, body.Name, body.EntityType, body.ConditionField, body.Operator, body.Threshold, body.Severity, sortedUnique(body.Channels), body.Global, body.CooldownMinutes)
	if err != nil {
		writeError(w, 400, "notification_rule_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "notification_rule", id.String(), "create", "success", map[string]any{"entityType": body.EntityType, "global": body.Global})
	_, _ = s.evaluateNotifications(r.Context())
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) deleteNotificationRule(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_notification_rule_id", err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `DELETE FROM notification_rules WHERE id=$1 AND (owner_id=$2 OR $3)`, id, p.ID, p.Has("*"))
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, 404, "notification_rule_not_found", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Delete", "notification_rule", id.String(), "delete", "success", nil)
	w.WriteHeader(204)
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	unreadOnly := r.URL.Query().Get("unread") == "true"
	rows, err := s.pool.Query(r.Context(), `SELECT id,rule_id,severity,title,message,resource_type,resource_id,channels,delivery_status,read_at,created_at FROM notifications WHERE user_id=$1 AND (NOT $2 OR read_at IS NULL) ORDER BY created_at DESC LIMIT 200`, p.ID, unreadOnly)
	if err != nil {
		writeError(w, 500, "notification_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var ruleID, resourceID *uuid.UUID
		var severity, title, message, resource string
		var channels []string
		var delivery json.RawMessage
		var readAt *time.Time
		var created time.Time
		if err = rows.Scan(&id, &ruleID, &severity, &title, &message, &resource, &resourceID, &channels, &delivery, &readAt, &created); err != nil {
			writeError(w, 500, "notification_scan_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "ruleId": ruleID, "severity": severity, "title": title, "message": message, "resourceType": resource, "resourceId": resourceID, "channels": channels, "deliveryStatus": delivery, "readAt": readAt, "createdAt": created})
	}
	writeJSON(w, 200, items)
}
func (s *Server) readNotification(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_notification_id", err)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND user_id=$2`, id, p.ID)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, 404, "notification_not_found", err)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) evaluateNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	count, err := s.evaluateNotifications(r.Context())
	if err != nil {
		writeError(w, 500, "notification_evaluation_failed", err)
		return
	}
	writeJSON(w, 200, map[string]any{"generated": count, "evaluatedAt": time.Now()})
}

func (s *Server) evaluateNotifications(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,owner_id,name,entity_type,condition_field,operator,threshold,severity,channels,global,enabled,cooldown_minutes,created_at,updated_at FROM notification_rules WHERE enabled=true`)
	if err != nil {
		return 0, err
	}
	rules := []notificationRule{}
	for rows.Next() {
		var x notificationRule
		if err = rows.Scan(&x.ID, &x.OwnerID, &x.Name, &x.EntityType, &x.ConditionField, &x.Operator, &x.Threshold, &x.Severity, &x.Channels, &x.Global, &x.Enabled, &x.CooldownMinutes, &x.CreatedAt, &x.UpdatedAt); err != nil {
			rows.Close()
			return 0, err
		}
		rules = append(rules, x)
	}
	rows.Close()
	generated := 0
	for _, rule := range rules {
		candidates, e := s.notificationCandidates(ctx, rule)
		if e != nil {
			return generated, e
		}
		for _, candidate := range candidates {
			if !matchesNotification(rule, candidate) {
				continue
			}
			bucket := time.Now().Unix() / (int64(rule.CooldownMinutes) * 60)
			dedupe := fmt.Sprintf("%s/%s/%d", rule.ID, candidate.ID, bucket)
			delivery := map[string]string{}
			for _, channel := range rule.Channels {
				if channel == "system" {
					delivery[channel] = "delivered"
				} else {
					delivery[channel] = "not_configured"
				}
			}
			deliveryJSON, _ := json.Marshal(delivery)
			message := fmt.Sprintf("%s: %s 조건이 충족되었습니다 (현재 값 %s %s %s)", candidate.Name, rule.ConditionField, candidateDisplay(candidate), rule.Operator, rule.Threshold)
			tag, e := s.pool.Exec(ctx, `INSERT INTO notifications(id,user_id,rule_id,dedupe_key,severity,title,message,resource_type,resource_id,channels,delivery_status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(dedupe_key) DO NOTHING`, uuid.New(), rule.OwnerID, rule.ID, dedupe, rule.Severity, rule.Name, message, rule.EntityType, candidate.ID, rule.Channels, deliveryJSON)
			if e != nil {
				return generated, e
			}
			generated += int(tag.RowsAffected())
		}
	}
	return generated, nil
}

func (s *Server) notificationCandidates(ctx context.Context, rule notificationRule) ([]notificationCandidate, error) {
	ownerFilter := !rule.Global
	items := []notificationCandidate{}
	var rows pgx.Rows
	var err error
	switch rule.EntityType {
	case "kpi":
		rows, err = s.pool.Query(ctx, `SELECT id,name,CASE WHEN target=0 THEN 100 ELSE actual/target*100 END,'' FROM kpis WHERE NOT $1 OR owner_id=$2`, ownerFilter, rule.OwnerID)
	case "project":
		if rule.ConditionField == "risk" {
			rows, err = s.pool.Query(ctx, `SELECT id,name,CASE risk WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 ELSE 1 END,risk FROM projects WHERE NOT $1 OR owner_id=$2`, ownerFilter, rule.OwnerID)
		} else if rule.ConditionField == "deadline_days" {
			rows, err = s.pool.Query(ctx, `SELECT id,name,COALESCE(end_date-current_date,999999),'' FROM projects WHERE NOT $1 OR owner_id=$2`, ownerFilter, rule.OwnerID)
		} else {
			rows, err = s.pool.Query(ctx, `SELECT id,name,CASE WHEN budget=0 THEN 0 ELSE actual_cost/budget*100 END,'' FROM projects WHERE NOT $1 OR owner_id=$2`, ownerFilter, rule.OwnerID)
		}
	case "intelligence":
		rows, err = s.pool.Query(ctx, `SELECT id,title,importance,'' FROM intelligence_items WHERE NOT $1 OR owner_id=$2`, ownerFilter, rule.OwnerID)
	case "key":
		rows, err = s.pool.Query(ctx, `SELECT id,name,COALESCE(EXTRACT(epoch FROM (expires_at-now()))/86400,999999),' ' FROM personal_access_keys WHERE user_id=$1 AND revoked_at IS NULL`, rule.OwnerID)
	default:
		return items, errors.New("unsupported notification entity")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var x notificationCandidate
		if err = rows.Scan(&x.ID, &x.Name, &x.Value, &x.TextValue); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func validNotificationField(entity, field string) bool {
	return map[string]map[string]bool{"kpi": {"achievement": true}, "project": {"risk": true, "budget_percent": true, "deadline_days": true}, "intelligence": {"importance": true}, "key": {"expiry_days": true}}[entity][field]
}
func matchesNotification(rule notificationRule, c notificationCandidate) bool {
	threshold, err := strconv.ParseFloat(strings.TrimSpace(rule.Threshold), 64)
	if err != nil && rule.ConditionField == "risk" {
		threshold = map[string]float64{"low": 1, "medium": 2, "high": 3, "critical": 4}[strings.ToLower(rule.Threshold)]
	}
	if threshold == 0 && strings.TrimSpace(rule.Threshold) != "0" && rule.ConditionField != "risk" {
		return false
	}
	switch rule.Operator {
	case "lt":
		return c.Value < threshold
	case "lte":
		return c.Value <= threshold
	case "eq":
		return c.Value == threshold
	case "gte":
		return c.Value >= threshold
	case "gt":
		return c.Value > threshold
	}
	return false
}
func candidateDisplay(c notificationCandidate) string {
	if strings.TrimSpace(c.TextValue) != "" {
		return c.TextValue
	}
	return strconv.FormatFloat(c.Value, 'f', 1, 64)
}
