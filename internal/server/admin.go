package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT id,occurred_at,actor_id,actor_name,event_type,resource_type,resource_id,action,outcome,COALESCE(host(ip_address),''),user_agent,details FROM audit_logs ORDER BY occurred_at DESC LIMIT 500`)
	if err != nil {
		writeError(w, 500, "audit_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var occurred time.Time
		var actor *uuid.UUID
		var actorName, eventType, resourceType, resourceID, action, outcome, ip, agent string
		var details json.RawMessage
		if err := rows.Scan(&id, &occurred, &actor, &actorName, &eventType, &resourceType, &resourceID, &action, &outcome, &ip, &agent, &details); err != nil {
			writeError(w, 500, "audit_scan_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "occurredAt": occurred, "actorId": actor, "actorName": actorName, "eventType": eventType, "resourceType": resourceType, "resourceId": resourceID, "action": action, "outcome": outcome, "ipAddress": ip, "userAgent": agent, "details": details})
	}
	writeJSON(w, 200, items)
}

func (s *Server) listOrganizations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT id,parent_id,code,name,attributes,created_at,updated_at FROM organizations ORDER BY code`)
	if err != nil {
		writeError(w, 500, "organization_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var parent *uuid.UUID
		var code, name string
		var attributes json.RawMessage
		var created, updated time.Time
		if err = rows.Scan(&id, &parent, &code, &name, &attributes, &created, &updated); err != nil {
			writeError(w, 500, "organization_scan_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "parentId": parent, "code": code, "name": name, "attributes": attributes, "createdAt": created, "updatedAt": updated})
	}
	writeJSON(w, 200, items)
}
func (s *Server) createOrganization(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		ParentID   *uuid.UUID      `json:"parentId"`
		Code       string          `json:"code"`
		Name       string          `json:"name"`
		Attributes json.RawMessage `json:"attributes"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Code, body.Name) != nil {
		writeError(w, 400, "invalid_organization", err)
		return
	}
	if len(body.Attributes) == 0 {
		body.Attributes = []byte(`{}`)
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO organizations(id,parent_id,code,name,attributes) VALUES($1,$2,$3,$4,$5)`, id, body.ParentID, strings.TrimSpace(body.Code), strings.TrimSpace(body.Name), body.Attributes)
	if err != nil {
		writeError(w, 409, "organization_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "organization", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}
func (s *Server) updateOrganization(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_organization_id", err)
		return
	}
	var body struct {
		ParentID   *uuid.UUID      `json:"parentId"`
		Code       string          `json:"code"`
		Name       string          `json:"name"`
		Attributes json.RawMessage `json:"attributes"`
	}
	if err = decodeJSON(r, &body); err != nil || required(body.Code, body.Name) != nil {
		writeError(w, 400, "invalid_organization", err)
		return
	}
	if body.ParentID != nil && *body.ParentID == id {
		writeError(w, 400, "organization_cycle", nil)
		return
	}
	if len(body.Attributes) == 0 {
		body.Attributes = []byte(`{}`)
	}
	var cycle bool
	if body.ParentID != nil {
		err = s.pool.QueryRow(r.Context(), `WITH RECURSIVE descendants AS (SELECT id FROM organizations WHERE parent_id=$1 UNION ALL SELECT o.id FROM organizations o JOIN descendants d ON o.parent_id=d.id) SELECT EXISTS(SELECT 1 FROM descendants WHERE id=$2)`, id, *body.ParentID).Scan(&cycle)
		if err != nil {
			writeError(w, 500, "organization_cycle_check_failed", err)
			return
		}
		if cycle {
			writeError(w, 400, "organization_cycle", nil)
			return
		}
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE organizations SET parent_id=$1,code=$2,name=$3,attributes=$4,updated_at=now() WHERE id=$5`, body.ParentID, strings.TrimSpace(body.Code), strings.TrimSpace(body.Name), body.Attributes, id)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, 404, "organization_update_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Update", "organization", id.String(), "update", "success", nil)
	w.WriteHeader(204)
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT id,name,description,permissions,system,updated_at FROM roles ORDER BY system DESC,id`)
	if err != nil {
		writeError(w, 500, "role_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, description string
		var permissions []string
		var system bool
		var updated time.Time
		if err = rows.Scan(&id, &name, &description, &permissions, &system, &updated); err != nil {
			writeError(w, 500, "role_scan_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "description": description, "permissions": permissions, "system": system, "updatedAt": updated})
	}
	writeJSON(w, 200, items)
}

var permissionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(?::(?:[a-z][a-z0-9_]*|\*))?$`)

func (s *Server) updateRole(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id := chi.URLParam(r, "id")
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Name) != nil {
		writeError(w, 400, "invalid_role", err)
		return
	}
	permissions := sortedUnique(body.Permissions)
	for _, permission := range permissions {
		if permission != "*" && !permissionPattern.MatchString(permission) {
			writeError(w, 400, "invalid_permission", errors.New("invalid permission: "+permission))
			return
		}
	}
	if id == "system_admin" && !stringIn(permissions, "*") {
		writeError(w, 409, "system_admin_wildcard_required", nil)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE roles SET name=$1,description=$2,permissions=$3,updated_at=now() WHERE id=$4`, body.Name, body.Description, permissions, id)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, 404, "role_update_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Permission Change", "role", id, "update_permissions", "success", map[string]any{"permissions": permissions})
	w.WriteHeader(204)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT u.id,u.username,u.display_name,COALESCE(u.email,''),u.organization_id,COALESCE(u.title,''),u.active,u.must_change_password,COALESCE(array_agg(ur.role_id) FILTER(WHERE ur.role_id IS NOT NULL),'{}') FROM users u LEFT JOIN user_roles ur ON ur.user_id=u.id GROUP BY u.id ORDER BY u.username`)
	if err != nil {
		writeError(w, 500, "user_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var username, name, email, title string
		var org *uuid.UUID
		var active, mustChange bool
		var roles []string
		if err := rows.Scan(&id, &username, &name, &email, &org, &title, &active, &mustChange, &roles); err != nil {
			writeError(w, 500, "user_scan_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "username": username, "displayName": name, "email": email, "organizationId": org, "title": title, "active": active, "mustChangePassword": mustChange, "roles": roles})
	}
	writeJSON(w, 200, items)
}

func (s *Server) setUserRoles(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_user_id", err)
		return
	}
	var body struct {
		Roles []string `json:"roles"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, 400, "invalid_roles", err)
		return
	}
	roles := sortedUnique(body.Roles)
	if id == p.ID && !stringIn(roles, "system_admin") {
		writeError(w, 409, "cannot_remove_own_admin_role", nil)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `DELETE FROM user_roles WHERE user_id=$1`, id); err != nil {
		writeError(w, 500, "roles_update_failed", err)
		return
	}
	for _, role := range roles {
		tag, e := tx.Exec(r.Context(), `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE id=$2`, id, role)
		if e != nil || tag.RowsAffected() == 0 {
			writeError(w, 400, "unknown_role", e)
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Permission Change", "user", id.String(), "set_roles", "success", map[string]any{"roles": roles})
	w.WriteHeader(204)
}

func (s *Server) updateUserAdmin(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_user_id", err)
		return
	}
	var body struct {
		DisplayName    string     `json:"displayName"`
		Email          string     `json:"email"`
		Title          string     `json:"title"`
		OrganizationID *uuid.UUID `json:"organizationId"`
		Active         bool       `json:"active"`
	}
	if err = decodeJSON(r, &body); err != nil || required(body.DisplayName) != nil {
		writeError(w, 400, "invalid_user", err)
		return
	}
	if id == p.ID && !body.Active {
		writeError(w, 409, "cannot_disable_self", nil)
		return
	}
	tag, err := s.pool.Exec(r.Context(), `UPDATE users SET display_name=$1,email=NULLIF($2,''),title=NULLIF($3,''),organization_id=$4,active=$5,updated_at=now() WHERE id=$6`, body.DisplayName, body.Email, body.Title, body.OrganizationID, body.Active, id)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, 404, "user_update_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Update", "user", id.String(), "admin_update", "success", map[string]any{"organizationId": body.OrganizationID, "active": body.Active})
	w.WriteHeader(204)
}
