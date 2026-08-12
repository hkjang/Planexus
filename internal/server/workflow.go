package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

type workflowStep struct {
	Type  string `json:"type"`
	Order int    `json:"order"`
	Role  string `json:"role,omitempty"`
}
type workflowTask struct {
	ID             uuid.UUID    `json:"id"`
	ResourceType   string       `json:"resourceType"`
	ResourceID     uuid.UUID    `json:"resourceId"`
	ResourceTitle  string       `json:"resourceTitle"`
	CurrentStep    int          `json:"currentStep"`
	Step           workflowStep `json:"step"`
	Status         string       `json:"status"`
	OrganizationID *uuid.UUID   `json:"organizationId,omitempty"`
	CreatedAt      time.Time    `json:"createdAt"`
	UpdatedAt      time.Time    `json:"updatedAt"`
}

func (s *Server) workflowTasks(r *http.Request, p auth.Principal) ([]workflowTask, error) {
	rows, err := s.pool.Query(r.Context(), `SELECT wi.id,wi.resource_type,wi.resource_id,p.title,wi.current_step,wd.steps,wi.status,p.organization_id,wi.created_at,wi.updated_at FROM workflow_instances wi JOIN workflow_definitions wd ON wd.id=wi.definition_id JOIN plans p ON p.id=wi.resource_id WHERE wi.resource_type='plan' AND wi.status='pending' ORDER BY wi.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []workflowTask{}
	for rows.Next() {
		var task workflowTask
		var stepsJSON json.RawMessage
		var steps []workflowStep
		if err = rows.Scan(&task.ID, &task.ResourceType, &task.ResourceID, &task.ResourceTitle, &task.CurrentStep, &stepsJSON, &task.Status, &task.OrganizationID, &task.CreatedAt, &task.UpdatedAt); err != nil {
			return nil, err
		}
		if json.Unmarshal(stepsJSON, &steps) != nil || task.CurrentStep < 0 || task.CurrentStep >= len(steps) {
			continue
		}
		task.Step = steps[task.CurrentStep]
		if canActWorkflow(p, task.Step, task.OrganizationID) {
			items = append(items, task)
		}
	}
	return items, rows.Err()
}
func canActWorkflow(p auth.Principal, step workflowStep, organizationID *uuid.UUID) bool {
	if p.Has("*") {
		return true
	}
	if step.Role != "" && roleIn(p.Roles, step.Role) {
		return true
	}
	switch step.Type {
	case "department_review":
		return roleIn(p.Roles, "department_manager") && p.OrganizationID != nil && organizationID != nil && *p.OrganizationID == *organizationID
	case "planning_review":
		return roleIn(p.Roles, "planning_manager") || roleIn(p.Roles, "planning_admin")
	case "approval":
		return roleIn(p.Roles, "planning_admin") || roleIn(p.Roles, "executive")
	}
	return false
}
func roleIn(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Server) listWorkflowTasks(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	items, err := s.workflowTasks(r, p)
	if err != nil {
		writeError(w, 500, "workflow_tasks_failed", err)
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) actWorkflowTask(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_workflow_task_id", err)
		return
	}
	var body struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err = decodeJSON(r, &body); err != nil {
		writeError(w, 400, "invalid_workflow_action", err)
		return
	}
	if !oneOf(body.Action, "approve", "reject") {
		writeError(w, 400, "invalid_workflow_action", errors.New("action must be approve or reject"))
		return
	}
	if body.Action == "reject" && len([]rune(body.Comment)) < 2 {
		writeError(w, 400, "rejection_comment_required", nil)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	defer tx.Rollback(r.Context())
	var resourceID uuid.UUID
	var current int
	var status string
	var stepsJSON, historyJSON json.RawMessage
	var organizationID *uuid.UUID
	err = tx.QueryRow(r.Context(), `SELECT wi.resource_id,wi.current_step,wi.status,wd.steps,wi.history,p.organization_id FROM workflow_instances wi JOIN workflow_definitions wd ON wd.id=wi.definition_id JOIN plans p ON p.id=wi.resource_id WHERE wi.id=$1 FOR UPDATE`, id).Scan(&resourceID, &current, &status, &stepsJSON, &historyJSON, &organizationID)
	if err != nil {
		writeError(w, 404, "workflow_task_not_found", nil)
		return
	}
	if status != "pending" {
		writeError(w, 409, "workflow_task_closed", nil)
		return
	}
	var steps []workflowStep
	if json.Unmarshal(stepsJSON, &steps) != nil || current < 0 || current >= len(steps) {
		writeError(w, 409, "workflow_definition_invalid", nil)
		return
	}
	if !canActWorkflow(p, steps[current], organizationID) {
		writeError(w, 403, "forbidden", nil)
		return
	}
	var history []map[string]any
	_ = json.Unmarshal(historyJSON, &history)
	history = append(history, map[string]any{"step": current, "stepType": steps[current].Type, "action": body.Action, "comment": body.Comment, "actorId": p.ID, "actorName": p.Username, "at": time.Now()})
	historyData, _ := json.Marshal(history)
	nextStatus := "pending"
	planStatus := "in_review"
	nextStep := current
	if body.Action == "reject" {
		nextStatus = "rejected"
		planStatus = "changes_requested"
	} else if current+1 >= len(steps) {
		nextStatus = "approved"
		planStatus = "confirmed"
	} else {
		nextStep = current + 1
	}
	_, err = tx.Exec(r.Context(), `UPDATE workflow_instances SET current_step=$1,status=$2,history=$3,updated_at=now() WHERE id=$4`, nextStep, nextStatus, historyData, id)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE plans SET status=$1,updated_at=now() WHERE id=$2`, planStatus, resourceID)
	}
	if err != nil {
		writeError(w, 500, "workflow_action_failed", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	event := "Approve"
	if body.Action == "reject" {
		event = "Reject"
	}
	s.audit(r, &p.ID, p.Username, event, "plan", resourceID.String(), body.Action, "success", map[string]any{"workflowInstanceId": id, "step": current, "stepType": steps[current].Type, "comment": body.Comment})
	writeJSON(w, 200, map[string]any{"id": id, "status": nextStatus, "planStatus": planStatus, "nextStep": nextStep})
}
