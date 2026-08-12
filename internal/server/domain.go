package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
	"github.com/jackc/pgx/v5"
)

func (s *Server) executiveDashboard(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var result struct {
		StrategyCount    int     `json:"strategyCount"`
		KPIHealth        float64 `json:"kpiHealth"`
		ProjectCount     int     `json:"projectCount"`
		HighRiskProjects int     `json:"highRiskProjects"`
		BudgetTotal      float64 `json:"budgetTotal"`
		ActualCost       float64 `json:"actualCost"`
		PendingPlans     int     `json:"pendingPlans"`
		DecisionCount    int     `json:"decisionCount"`
	}
	strategyRows, err := s.pool.Query(r.Context(), `SELECT classification,organization_id,owner_id FROM strategies`)
	if err != nil {
		writeError(w, 500, "dashboard_query_failed", err)
		return
	}
	for strategyRows.Next() {
		var classification string
		var organizationID, ownerID *uuid.UUID
		if err = strategyRows.Scan(&classification, &organizationID, &ownerID); err != nil {
			strategyRows.Close()
			writeError(w, 500, "dashboard_query_failed", err)
			return
		}
		if p.Can("strategy:read", auth.Resource{OwnerID: ownerID, OrganizationID: organizationID, Classification: classification}) {
			result.StrategyCount++
		}
	}
	strategyRows.Close()

	kpiRows, err := s.pool.Query(r.Context(), `SELECT target,actual,classification,organization_id,owner_id FROM kpis`)
	if err != nil {
		writeError(w, 500, "dashboard_query_failed", err)
		return
	}
	var kpiTotal float64
	var kpiCount int
	for kpiRows.Next() {
		var target, actual float64
		var classification string
		var organizationID, ownerID *uuid.UUID
		if err = kpiRows.Scan(&target, &actual, &classification, &organizationID, &ownerID); err != nil {
			kpiRows.Close()
			writeError(w, 500, "dashboard_query_failed", err)
			return
		}
		if p.Can("kpi:read", auth.Resource{OwnerID: ownerID, OrganizationID: organizationID, Classification: classification}) {
			achievement := 100.0
			if target != 0 {
				achievement = min(100, actual/target*100)
			}
			kpiTotal += achievement
			kpiCount++
		}
	}
	kpiRows.Close()
	if kpiCount > 0 {
		result.KPIHealth = kpiTotal / float64(kpiCount)
	}

	projectRows, err := s.pool.Query(r.Context(), `SELECT risk,budget,actual_cost,classification,organization_id,owner_id FROM projects`)
	if err != nil {
		writeError(w, 500, "dashboard_query_failed", err)
		return
	}
	for projectRows.Next() {
		var risk, classification string
		var budget, actual float64
		var organizationID, ownerID *uuid.UUID
		if err = projectRows.Scan(&risk, &budget, &actual, &classification, &organizationID, &ownerID); err != nil {
			projectRows.Close()
			writeError(w, 500, "dashboard_query_failed", err)
			return
		}
		if p.Can("project:read", auth.Resource{OwnerID: ownerID, OrganizationID: organizationID, Classification: classification}) {
			result.ProjectCount++
			result.BudgetTotal += budget
			result.ActualCost += actual
			if risk == "high" || risk == "critical" {
				result.HighRiskProjects++
			}
		}
	}
	projectRows.Close()

	planRows, err := s.pool.Query(r.Context(), `SELECT status,classification,organization_id,owner_id FROM plans WHERE status IN ('draft','in_review','changes_requested')`)
	if err != nil {
		writeError(w, 500, "dashboard_query_failed", err)
		return
	}
	for planRows.Next() {
		var status, classification string
		var organizationID, ownerID *uuid.UUID
		if err = planRows.Scan(&status, &classification, &organizationID, &ownerID); err != nil {
			planRows.Close()
			writeError(w, 500, "dashboard_query_failed", err)
			return
		}
		allowed := p.Has("plan:*") || p.Has("dashboard:executive") || (p.Has("plan:own") && ownerID != nil && *ownerID == p.ID) || (p.OrganizationID != nil && organizationID != nil && *p.OrganizationID == *organizationID && p.Has("plan:organization"))
		if allowed && p.Can("dashboard:read", auth.Resource{OwnerID: ownerID, OrganizationID: organizationID, Classification: classification}) {
			result.PendingPlans++
		}
	}
	planRows.Close()

	decisionRows, err := s.pool.Query(r.Context(), `SELECT classification,organization_id FROM decisions`)
	if err != nil {
		writeError(w, 500, "dashboard_query_failed", err)
		return
	}
	for decisionRows.Next() {
		var classification string
		var organizationID *uuid.UUID
		if err = decisionRows.Scan(&classification, &organizationID); err != nil {
			decisionRows.Close()
			writeError(w, 500, "dashboard_query_failed", err)
			return
		}
		if p.Can("decision:read", auth.Resource{OrganizationID: organizationID, Classification: classification}) {
			result.DecisionCount++
		}
	}
	decisionRows.Close()
	writeJSON(w, 200, result)
}

func (s *Server) personalDashboard(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	type recommendation struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		Title    string `json:"title"`
		Reason   string `json:"reason"`
		Priority int    `json:"priority"`
		Severity string `json:"severity"`
		Path     string `json:"path"`
	}
	var result struct {
		OwnedKPIs        int              `json:"ownedKpis"`
		OwnedProjects    int              `json:"ownedProjects"`
		MyPlans          int              `json:"myPlans"`
		PendingApprovals int              `json:"pendingApprovals"`
		ActiveKeys       int              `json:"activeKeys"`
		Recommendations  []recommendation `json:"recommendations"`
	}
	err := s.pool.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM kpis WHERE owner_id=$1),(SELECT count(*) FROM projects WHERE owner_id=$1),(SELECT count(*) FROM plans WHERE owner_id=$1),(SELECT count(*) FROM personal_access_keys WHERE user_id=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()))`, p.ID).Scan(&result.OwnedKPIs, &result.OwnedProjects, &result.MyPlans, &result.ActiveKeys)
	if err != nil {
		writeError(w, 500, "personal_dashboard_failed", err)
		return
	}
	result.Recommendations = []recommendation{}
	rows, recommendationErr := s.pool.Query(r.Context(), `
		SELECT entity_type,id,title,reason,priority,severity,path FROM (
			SELECT 'kpi' entity_type,id::text,name title,format('목표 대비 달성률 %s%%',round(actual/target*100,1)) reason,CASE WHEN actual/target*100<80 THEN 90 ELSE 75 END priority,CASE WHEN actual/target*100<80 THEN 'critical' ELSE 'warning' END severity,'/kpi' path FROM kpis WHERE owner_id=$1 AND target>0 AND actual/target*100<90
			UNION ALL SELECT 'project',id::text,name,format('프로젝트 위험 등급 %s',risk),CASE WHEN risk='critical' THEN 100 ELSE 90 END,'critical','/projects' FROM projects WHERE owner_id=$1 AND risk IN ('high','critical')
			UNION ALL SELECT 'key',id::text,name,format('개인 키가 %s일 이내 만료',GREATEST(0,floor(EXTRACT(epoch FROM (expires_at-now()))/86400))),70,'warning','/profile/security' FROM personal_access_keys WHERE user_id=$1 AND revoked_at IS NULL AND expires_at BETWEEN now() AND now()+interval '7 days'
		) priorities ORDER BY priority DESC,title LIMIT 10`, p.ID)
	if recommendationErr == nil {
		defer rows.Close()
		for rows.Next() {
			var item recommendation
			if rows.Scan(&item.Type, &item.ID, &item.Title, &item.Reason, &item.Priority, &item.Severity, &item.Path) == nil {
				result.Recommendations = append(result.Recommendations, item)
			}
		}
	}
	if tasks, taskErr := s.workflowTasks(r, p); taskErr == nil {
		result.PendingApprovals = len(tasks)
		for _, task := range tasks {
			result.Recommendations = append(result.Recommendations, recommendation{Type: "approval", ID: task.ID.String(), Title: task.ResourceTitle, Reason: task.Step.Type + " 승인 요청", Priority: 95, Severity: "critical", Path: "/personal"})
		}
	}
	sort.SliceStable(result.Recommendations, func(i, j int) bool { return result.Recommendations[i].Priority > result.Recommendations[j].Priority })
	if len(result.Recommendations) > 10 {
		result.Recommendations = result.Recommendations[:10]
	}
	writeJSON(w, 200, result)
}

type strategyDTO struct {
	ID             uuid.UUID  `json:"id"`
	ParentID       *uuid.UUID `json:"parentId,omitempty"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Description    string     `json:"description"`
	PeriodStart    *time.Time `json:"periodStart,omitempty"`
	PeriodEnd      *time.Time `json:"periodEnd,omitempty"`
	Version        int        `json:"version"`
	Status         string     `json:"status"`
	Classification string     `json:"classification"`
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	OwnerID        *uuid.UUID `json:"ownerId,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

func (s *Server) listStrategies(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,parent_id,name,kind,description,period_start,period_end,version,status,classification,organization_id,owner_id,created_at FROM strategies ORDER BY kind,name`)
	if err != nil {
		writeError(w, 500, "strategy_query_failed", err)
		return
	}
	defer rows.Close()
	items := []strategyDTO{}
	for rows.Next() {
		var x strategyDTO
		if err := rows.Scan(&x.ID, &x.ParentID, &x.Name, &x.Kind, &x.Description, &x.PeriodStart, &x.PeriodEnd, &x.Version, &x.Status, &x.Classification, &x.OrganizationID, &x.OwnerID, &x.CreatedAt); err != nil {
			writeError(w, 500, "strategy_scan_failed", err)
			return
		}
		if p.Can("strategy:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createStrategy(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		ParentID       *uuid.UUID `json:"parentId"`
		Name           string     `json:"name"`
		Kind           string     `json:"kind"`
		Description    string     `json:"description"`
		PeriodStart    *time.Time `json:"periodStart"`
		PeriodEnd      *time.Time `json:"periodEnd"`
		Status         string     `json:"status"`
		Classification string     `json:"classification"`
		OrganizationID *uuid.UUID `json:"organizationId"`
		OwnerID        *uuid.UUID `json:"ownerId"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Name, body.Kind) != nil {
		writeError(w, 400, "invalid_strategy", err)
		return
	}
	if !oneOf(body.Kind, "vision", "mission", "theme", "objective") {
		writeError(w, 400, "invalid_strategy_kind", nil)
		return
	}
	if body.Status == "" {
		body.Status = "draft"
	}
	if body.Classification == "" {
		body.Classification = "internal"
	}
	if !validSecurityClassification(body.Classification) {
		writeError(w, http.StatusBadRequest, "invalid_classification", nil)
		return
	}
	if !p.Can("strategy:*", auth.Resource{OwnerID: body.OwnerID, OrganizationID: body.OrganizationID, Classification: body.Classification}) {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO strategies(id,parent_id,name,kind,description,period_start,period_end,status,classification,organization_id,owner_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, body.ParentID, strings.TrimSpace(body.Name), body.Kind, body.Description, body.PeriodStart, body.PeriodEnd, body.Status, body.Classification, body.OrganizationID, body.OwnerID, p.ID)
	if err != nil {
		writeError(w, 400, "strategy_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "strategy", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}

type kpiDTO struct {
	ID             uuid.UUID  `json:"id"`
	StrategyID     *uuid.UUID `json:"strategyId,omitempty"`
	ParentID       *uuid.UUID `json:"parentId,omitempty"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Formula        string     `json:"formula"`
	Unit           string     `json:"unit"`
	Frequency      string     `json:"frequency"`
	Target         float64    `json:"target"`
	Actual         float64    `json:"actual"`
	Achievement    float64    `json:"achievement"`
	Weight         float64    `json:"weight"`
	Source         string     `json:"source"`
	Classification string     `json:"classification"`
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	OwnerID        *uuid.UUID `json:"ownerId,omitempty"`
}

func (s *Server) listKPIs(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,strategy_id,parent_id,code,name,description,formula,unit,frequency,target,actual,CASE WHEN target=0 THEN 100 ELSE actual/target*100 END,weight,source,classification,organization_id,owner_id FROM kpis ORDER BY code`)
	if err != nil {
		writeError(w, 500, "kpi_query_failed", err)
		return
	}
	defer rows.Close()
	items := []kpiDTO{}
	for rows.Next() {
		var x kpiDTO
		if err := rows.Scan(&x.ID, &x.StrategyID, &x.ParentID, &x.Code, &x.Name, &x.Description, &x.Formula, &x.Unit, &x.Frequency, &x.Target, &x.Actual, &x.Achievement, &x.Weight, &x.Source, &x.Classification, &x.OrganizationID, &x.OwnerID); err != nil {
			writeError(w, 500, "kpi_scan_failed", err)
			return
		}
		if p.Can("kpi:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createKPI(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		StrategyID     *uuid.UUID `json:"strategyId"`
		ParentID       *uuid.UUID `json:"parentId"`
		Code           string     `json:"code"`
		Name           string     `json:"name"`
		Description    string     `json:"description"`
		Formula        string     `json:"formula"`
		Unit           string     `json:"unit"`
		Frequency      string     `json:"frequency"`
		Target         float64    `json:"target"`
		Actual         float64    `json:"actual"`
		Weight         float64    `json:"weight"`
		Source         string     `json:"source"`
		Classification string     `json:"classification"`
		OrganizationID *uuid.UUID `json:"organizationId"`
		OwnerID        *uuid.UUID `json:"ownerId"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Code, body.Name) != nil {
		writeError(w, 400, "invalid_kpi", err)
		return
	}
	if body.Frequency == "" {
		body.Frequency = "monthly"
	}
	if body.Classification == "" {
		body.Classification = "internal"
	}
	if !validSecurityClassification(body.Classification) {
		writeError(w, http.StatusBadRequest, "invalid_classification", nil)
		return
	}
	if !p.Can("kpi:*", auth.Resource{OwnerID: body.OwnerID, OrganizationID: body.OrganizationID, Classification: body.Classification}) {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO kpis(id,strategy_id,parent_id,code,name,description,formula,unit,frequency,target,actual,weight,source,classification,organization_id,owner_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, id, body.StrategyID, body.ParentID, body.Code, body.Name, body.Description, body.Formula, body.Unit, body.Frequency, body.Target, body.Actual, body.Weight, body.Source, body.Classification, body.OrganizationID, body.OwnerID, p.ID)
	if err != nil {
		writeError(w, 400, "kpi_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "kpi", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}

type projectDTO struct {
	ID             uuid.UUID       `json:"id"`
	StrategyID     *uuid.UUID      `json:"strategyId,omitempty"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Status         string          `json:"status"`
	Progress       float64         `json:"progress"`
	Risk           string          `json:"risk"`
	Budget         float64         `json:"budget"`
	ActualCost     float64         `json:"actualCost"`
	StartDate      *time.Time      `json:"startDate,omitempty"`
	EndDate        *time.Time      `json:"endDate,omitempty"`
	Score          json.RawMessage `json:"score"`
	Classification string          `json:"classification"`
	OrganizationID *uuid.UUID      `json:"organizationId,omitempty"`
	OwnerID        *uuid.UUID      `json:"ownerId,omitempty"`
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,strategy_id,name,description,status,progress,risk,budget,actual_cost,start_date,end_date,score,classification,organization_id,owner_id FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		writeError(w, 500, "project_query_failed", err)
		return
	}
	defer rows.Close()
	items := []projectDTO{}
	for rows.Next() {
		var x projectDTO
		if err := rows.Scan(&x.ID, &x.StrategyID, &x.Name, &x.Description, &x.Status, &x.Progress, &x.Risk, &x.Budget, &x.ActualCost, &x.StartDate, &x.EndDate, &x.Score, &x.Classification, &x.OrganizationID, &x.OwnerID); err != nil {
			writeError(w, 500, "project_scan_failed", err)
			return
		}
		if p.Can("project:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		StrategyID     *uuid.UUID      `json:"strategyId"`
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		Status         string          `json:"status"`
		Progress       float64         `json:"progress"`
		Risk           string          `json:"risk"`
		Budget         float64         `json:"budget"`
		ActualCost     float64         `json:"actualCost"`
		StartDate      *time.Time      `json:"startDate"`
		EndDate        *time.Time      `json:"endDate"`
		Score          json.RawMessage `json:"score"`
		Classification string          `json:"classification"`
		OrganizationID *uuid.UUID      `json:"organizationId"`
		OwnerID        *uuid.UUID      `json:"ownerId"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Name) != nil {
		writeError(w, 400, "invalid_project", err)
		return
	}
	if body.Status == "" {
		body.Status = "planned"
	}
	if body.Risk == "" {
		body.Risk = "low"
	}
	if body.Classification == "" {
		body.Classification = "internal"
	}
	if !validSecurityClassification(body.Classification) {
		writeError(w, http.StatusBadRequest, "invalid_classification", nil)
		return
	}
	if len(body.Score) == 0 {
		body.Score = []byte(`{}`)
	}
	if !p.Can("project:*", auth.Resource{OwnerID: body.OwnerID, OrganizationID: body.OrganizationID, Classification: body.Classification}) {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO projects(id,strategy_id,name,description,status,progress,risk,budget,actual_cost,start_date,end_date,score,classification,organization_id,owner_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, id, body.StrategyID, body.Name, body.Description, body.Status, body.Progress, body.Risk, body.Budget, body.ActualCost, body.StartDate, body.EndDate, body.Score, body.Classification, body.OrganizationID, body.OwnerID, p.ID)
	if err != nil {
		writeError(w, 400, "project_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "project", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) listPlans(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,title,period,organization_id,owner_id,status,version,content,classification,created_at FROM plans WHERE $2 OR owner_id=$1 OR ($4 AND organization_id=$3) ORDER BY updated_at DESC`, p.ID, p.Has("plan:*"), p.OrganizationID, p.Has("plan:organization"))
	if err != nil {
		writeError(w, 500, "plan_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var title, period, status, class string
		var org, owner *uuid.UUID
		var version int
		var content json.RawMessage
		var created time.Time
		if err := rows.Scan(&id, &title, &period, &org, &owner, &status, &version, &content, &class, &created); err != nil {
			writeError(w, 500, "plan_scan_failed", err)
			return
		}
		if canReadPlan(p, owner, org, class) {
			items = append(items, map[string]any{"id": id, "title": title, "period": period, "organizationId": org, "ownerId": owner, "status": status, "version": version, "content": content, "classification": class, "createdAt": created})
		}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		Title          string          `json:"title"`
		Period         string          `json:"period"`
		OrganizationID *uuid.UUID      `json:"organizationId"`
		Content        json.RawMessage `json:"content"`
		Classification string          `json:"classification"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Title, body.Period) != nil {
		writeError(w, 400, "invalid_plan", err)
		return
	}
	if len(body.Content) == 0 {
		body.Content = []byte(`{}`)
	}
	if body.Classification == "" {
		body.Classification = "internal"
	}
	if !validSecurityClassification(body.Classification) {
		writeError(w, http.StatusBadRequest, "invalid_classification", nil)
		return
	}
	if !p.Has("plan:*") {
		if body.OrganizationID != nil && (p.OrganizationID == nil || *body.OrganizationID != *p.OrganizationID) {
			writeError(w, http.StatusForbidden, "forbidden", nil)
			return
		}
		body.OrganizationID = p.OrganizationID
	}
	if !p.Can("plan:own", auth.Resource{OwnerID: &p.ID, OrganizationID: body.OrganizationID, Classification: body.Classification}) {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO plans(id,title,period,organization_id,owner_id,content,classification,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$5)`, id, body.Title, body.Period, body.OrganizationID, p.ID, body.Content, body.Classification)
	if err != nil {
		writeError(w, 400, "plan_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "plan", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) submitPlan(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_plan_id", err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	defer tx.Rollback(r.Context())
	var ownerID, organizationID *uuid.UUID
	var classification, currentStatus string
	err = tx.QueryRow(r.Context(), `SELECT owner_id,organization_id,classification,status FROM plans WHERE id=$1 FOR UPDATE`, id).Scan(&ownerID, &organizationID, &classification, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "plan_not_found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "plan_query_failed", err)
		return
	}
	resource := auth.Resource{OwnerID: ownerID, OrganizationID: organizationID, Classification: classification}
	allowed := (p.Has("plan:*") && p.Can("plan:*", resource)) || (ownerID != nil && *ownerID == p.ID && p.Can("plan:own", resource))
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	if !oneOf(currentStatus, "draft", "changes_requested") {
		writeError(w, http.StatusConflict, "plan_not_submittable", nil)
		return
	}
	var definitionID uuid.UUID
	var enabled bool
	err = tx.QueryRow(r.Context(), `SELECT id,enabled FROM workflow_definitions WHERE resource_type='plan'`).Scan(&definitionID, &enabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "workflow_query_failed", err)
		return
	}
	status := "confirmed"
	if enabled {
		status = "in_review"
		_, err = tx.Exec(r.Context(), `INSERT INTO workflow_instances(id,definition_id,resource_type,resource_id,status,created_by) VALUES($1,$2,'plan',$3,'pending',$4)`, uuid.New(), definitionID, id, p.ID)
		if err != nil {
			writeError(w, 500, "workflow_start_failed", err)
			return
		}
	}
	tag, err := tx.Exec(r.Context(), `UPDATE plans SET status=$1,updated_at=now() WHERE id=$2 AND status IN ('draft','changes_requested')`, status, id)
	if err != nil || tag.RowsAffected() == 0 {
		writeError(w, 409, "plan_not_submittable", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Update", "plan", id.String(), "submit", "success", map[string]any{"workflowEnabled": enabled, "status": status})
	writeJSON(w, 200, map[string]any{"status": status, "workflowEnabled": enabled})
}

func canReadPlan(p auth.Principal, ownerID, organizationID *uuid.UUID, classification string) bool {
	resource := auth.Resource{OwnerID: ownerID, OrganizationID: organizationID, Classification: classification}
	return (p.Has("plan:*") && p.Can("plan:*", resource)) ||
		(ownerID != nil && *ownerID == p.ID && p.Can("plan:own", resource)) ||
		(p.Has("plan:organization") && organizationID != nil && p.OrganizationID != nil && *organizationID == *p.OrganizationID && p.Can("plan:organization", resource))
}

func (s *Server) listDecisions(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,title,decision_date,decision_maker_id,background,options,decision,reason,evidence,related_kpi_id,related_project_id,classification,organization_id,created_at FROM decisions ORDER BY decision_date DESC NULLS LAST,created_at DESC`)
	if err != nil {
		writeError(w, 500, "decision_query_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var title, background, decision, reason, class string
		var date *time.Time
		var maker, kpi, project, org *uuid.UUID
		var options, evidence json.RawMessage
		var created time.Time
		if err := rows.Scan(&id, &title, &date, &maker, &background, &options, &decision, &reason, &evidence, &kpi, &project, &class, &org, &created); err != nil {
			writeError(w, 500, "decision_scan_failed", err)
			return
		}
		if p.Can("decision:read", auth.Resource{OrganizationID: org, Classification: class}) {
			items = append(items, map[string]any{"id": id, "title": title, "decisionDate": date, "decisionMakerId": maker, "background": background, "options": options, "decision": decision, "reason": reason, "evidence": evidence, "relatedKpiId": kpi, "relatedProjectId": project, "classification": class, "organizationId": org, "createdAt": created})
		}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createDecision(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		Title            string          `json:"title"`
		DecisionDate     *time.Time      `json:"decisionDate"`
		Background       string          `json:"background"`
		Options          json.RawMessage `json:"options"`
		Decision         string          `json:"decision"`
		Reason           string          `json:"reason"`
		Evidence         json.RawMessage `json:"evidence"`
		RelatedKPIID     *uuid.UUID      `json:"relatedKpiId"`
		RelatedProjectID *uuid.UUID      `json:"relatedProjectId"`
		Classification   string          `json:"classification"`
		OrganizationID   *uuid.UUID      `json:"organizationId"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Title) != nil {
		writeError(w, 400, "invalid_decision", err)
		return
	}
	if len(body.Options) == 0 {
		body.Options = []byte(`[]`)
	}
	if len(body.Evidence) == 0 {
		body.Evidence = []byte(`[]`)
	}
	if body.Classification == "" {
		body.Classification = "internal"
	}
	if !validSecurityClassification(body.Classification) {
		writeError(w, http.StatusBadRequest, "invalid_classification", nil)
		return
	}
	if !p.Can("decision:*", auth.Resource{OwnerID: &p.ID, OrganizationID: body.OrganizationID, Classification: body.Classification}) {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO decisions(id,title,decision_date,decision_maker_id,background,options,decision,reason,evidence,related_kpi_id,related_project_id,classification,organization_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$4)`, id, body.Title, body.DecisionDate, p.ID, body.Background, body.Options, body.Decision, body.Reason, body.Evidence, body.RelatedKPIID, body.RelatedProjectID, body.Classification, body.OrganizationID)
	if err != nil {
		writeError(w, 400, "decision_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "decision", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}

func oneOf(value string, allowed ...string) bool {
	for _, x := range allowed {
		if value == x {
			return true
		}
	}
	return false
}
