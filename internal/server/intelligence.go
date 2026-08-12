package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

type intelligenceDTO struct {
	ID                uuid.UUID  `json:"id"`
	Category          string     `json:"category"`
	Title             string     `json:"title"`
	SourceName        string     `json:"sourceName"`
	SourceURL         string     `json:"sourceUrl"`
	PublishedAt       *time.Time `json:"publishedAt,omitempty"`
	Summary           string     `json:"summary"`
	Importance        int        `json:"importance"`
	CompanyRelevance  string     `json:"companyRelevance"`
	PotentialImpact   string     `json:"potentialImpact"`
	Risk              string     `json:"risk"`
	Opportunity       string     `json:"opportunity"`
	RecommendedAction string     `json:"recommendedAction"`
	Classification    string     `json:"classification"`
	OrganizationID    *uuid.UUID `json:"organizationId,omitempty"`
	OwnerID           *uuid.UUID `json:"ownerId,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

func (s *Server) listIntelligence(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := s.pool.Query(r.Context(), `SELECT id,category,title,source_name,source_url,published_at,summary,importance,company_relevance,potential_impact,risk,opportunity,recommended_action,classification,organization_id,owner_id,created_at FROM intelligence_items WHERE ($1='' OR category=$1) AND ($2='' OR title ILIKE '%'||$2||'%' OR summary ILIKE '%'||$2||'%') ORDER BY importance DESC,published_at DESC NULLS LAST LIMIT 500`, category, query)
	if err != nil {
		writeError(w, 500, "intelligence_query_failed", err)
		return
	}
	defer rows.Close()
	items := []intelligenceDTO{}
	for rows.Next() {
		var x intelligenceDTO
		if err := rows.Scan(&x.ID, &x.Category, &x.Title, &x.SourceName, &x.SourceURL, &x.PublishedAt, &x.Summary, &x.Importance, &x.CompanyRelevance, &x.PotentialImpact, &x.Risk, &x.Opportunity, &x.RecommendedAction, &x.Classification, &x.OrganizationID, &x.OwnerID, &x.CreatedAt); err != nil {
			writeError(w, 500, "intelligence_scan_failed", err)
			return
		}
		if p.Can("intelligence:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createIntelligence(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		Category          string     `json:"category"`
		Title             string     `json:"title"`
		SourceName        string     `json:"sourceName"`
		SourceURL         string     `json:"sourceUrl"`
		PublishedAt       *time.Time `json:"publishedAt"`
		RawContent        string     `json:"rawContent"`
		Summary           string     `json:"summary"`
		Importance        int        `json:"importance"`
		CompanyRelevance  string     `json:"companyRelevance"`
		PotentialImpact   string     `json:"potentialImpact"`
		Risk              string     `json:"risk"`
		Opportunity       string     `json:"opportunity"`
		RecommendedAction string     `json:"recommendedAction"`
		Classification    string     `json:"classification"`
		OrganizationID    *uuid.UUID `json:"organizationId"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Category, body.Title) != nil {
		writeError(w, 400, "invalid_intelligence", err)
		return
	}
	if !oneOf(body.Category, "competitor", "market", "customer", "regulation", "technology", "economic", "investment", "product") {
		writeError(w, 400, "invalid_intelligence_category", nil)
		return
	}
	if body.Importance < 0 || body.Importance > 100 {
		writeError(w, 400, "invalid_importance", nil)
		return
	}
	if body.Classification == "" {
		body.Classification = "internal"
	}
	if !validSecurityClassification(body.Classification) {
		writeError(w, http.StatusBadRequest, "invalid_classification", nil)
		return
	}
	if !p.Can("intelligence:*", auth.Resource{OwnerID: &p.ID, OrganizationID: body.OrganizationID, Classification: body.Classification}) {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO intelligence_items(id,category,title,source_name,source_url,published_at,raw_content,summary,importance,company_relevance,potential_impact,risk,opportunity,recommended_action,classification,organization_id,owner_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`, id, body.Category, body.Title, body.SourceName, body.SourceURL, body.PublishedAt, body.RawContent, body.Summary, body.Importance, body.CompanyRelevance, body.PotentialImpact, body.Risk, body.Opportunity, body.RecommendedAction, body.Classification, body.OrganizationID, p.ID)
	if err != nil {
		writeError(w, 400, "intelligence_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "intelligence", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}

type scenarioDTO struct {
	ID             uuid.UUID       `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Status         string          `json:"status"`
	Assumptions    json.RawMessage `json:"assumptions"`
	Results        json.RawMessage `json:"results,omitempty"`
	StrategyID     *uuid.UUID      `json:"strategyId,omitempty"`
	Classification string          `json:"classification"`
	OrganizationID *uuid.UUID      `json:"organizationId,omitempty"`
	OwnerID        *uuid.UUID      `json:"ownerId,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

func (s *Server) listScenarios(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	rows, err := s.pool.Query(r.Context(), `SELECT id,name,description,status,assumptions,results,strategy_id,classification,organization_id,owner_id,created_at,updated_at FROM scenarios ORDER BY updated_at DESC`)
	if err != nil {
		writeError(w, 500, "scenario_query_failed", err)
		return
	}
	defer rows.Close()
	items := []scenarioDTO{}
	for rows.Next() {
		var x scenarioDTO
		if err := rows.Scan(&x.ID, &x.Name, &x.Description, &x.Status, &x.Assumptions, &x.Results, &x.StrategyID, &x.Classification, &x.OrganizationID, &x.OwnerID, &x.CreatedAt, &x.UpdatedAt); err != nil {
			writeError(w, 500, "scenario_scan_failed", err)
			return
		}
		if p.Can("scenario:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	writeJSON(w, 200, items)
}

func (s *Server) createScenario(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	var body struct {
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		Assumptions    json.RawMessage `json:"assumptions"`
		StrategyID     *uuid.UUID      `json:"strategyId"`
		Classification string          `json:"classification"`
		OrganizationID *uuid.UUID      `json:"organizationId"`
	}
	if err := decodeJSON(r, &body); err != nil || required(body.Name) != nil {
		writeError(w, 400, "invalid_scenario", err)
		return
	}
	if len(body.Assumptions) == 0 {
		body.Assumptions = []byte(`{}`)
	}
	if body.Classification == "" {
		body.Classification = "confidential"
	}
	if !validSecurityClassification(body.Classification) {
		writeError(w, http.StatusBadRequest, "invalid_classification", nil)
		return
	}
	if !p.Can("scenario:*", auth.Resource{OwnerID: &p.ID, OrganizationID: body.OrganizationID, Classification: body.Classification}) {
		writeError(w, http.StatusForbidden, "forbidden", nil)
		return
	}
	id := uuid.New()
	_, err := s.pool.Exec(r.Context(), `INSERT INTO scenarios(id,name,description,assumptions,strategy_id,classification,organization_id,owner_id,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8)`, id, body.Name, body.Description, body.Assumptions, body.StrategyID, body.Classification, body.OrganizationID, p.ID)
	if err != nil {
		writeError(w, 400, "scenario_create_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "scenario", id.String(), "create", "success", nil)
	writeJSON(w, 201, map[string]any{"id": id})
}

func (s *Server) runScenario(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_scenario_id", err)
		return
	}
	result, status, code, err := s.simulateScenario(r, p, id)
	if err != nil {
		writeError(w, status, code, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) simulateScenario(r *http.Request, p auth.Principal, id uuid.UUID) (map[string]any, int, string, error) {
	var assumptions json.RawMessage
	var owner, org *uuid.UUID
	var class string
	err := s.pool.QueryRow(r.Context(), `SELECT assumptions,owner_id,organization_id,classification FROM scenarios WHERE id=$1`, id).Scan(&assumptions, &owner, &org, &class)
	if err != nil {
		return nil, 404, "scenario_not_found", errors.New("scenario not found")
	}
	if !p.Can("scenario:*", auth.Resource{OwnerID: owner, OrganizationID: org, Classification: class}) {
		return nil, 403, "forbidden", errors.New("permission denied")
	}
	var values struct {
		CostReductionPercent float64 `json:"costReductionPercent"`
		RevenueChangePercent float64 `json:"revenueChangePercent"`
		RiskTolerance        string  `json:"riskTolerance"`
	}
	if err = json.Unmarshal(assumptions, &values); err != nil {
		return nil, 400, "invalid_scenario_assumptions", err
	}
	if values.CostReductionPercent < 0 || values.CostReductionPercent > 100 || values.RevenueChangePercent < -100 {
		return nil, 400, "invalid_scenario_assumptions", errors.New("percent assumption is outside permitted range")
	}
	projects, err := s.mcpProjects(r, p, false)
	if err != nil {
		return nil, 500, "scenario_data_failed", err
	}
	items := projects.([]projectDTO)
	var budget, actual float64
	affected := []map[string]any{}
	for _, project := range items {
		budget += project.Budget
		actual += project.ActualCost
		reduction := project.Budget * values.CostReductionPercent / 100
		if reduction > 0 {
			affected = append(affected, map[string]any{"projectId": project.ID, "name": project.Name, "currentBudget": project.Budget, "recommendedReduction": reduction, "risk": project.Risk})
		}
	}
	result := map[string]any{"baselineBudget": budget, "baselineActualCost": actual, "costReduction": budget * values.CostReductionPercent / 100, "simulatedBudget": budget * (1 - values.CostReductionPercent/100), "revenueChangePercent": values.RevenueChangePercent, "affectedProjects": affected, "risk": "Simulation result requires manager review before application", "generatedAt": time.Now(), "method": "deterministic portfolio impact v1"}
	data, _ := json.Marshal(result)
	_, err = s.pool.Exec(r.Context(), `UPDATE scenarios SET status='simulation',results=$1,updated_at=now() WHERE id=$2`, data, id)
	if err != nil {
		return nil, 500, "scenario_update_failed", err
	}
	s.audit(r, &p.ID, p.Username, "Update", "scenario", id.String(), "run", "success", map[string]any{"projectCount": len(items)})
	return result, 200, "", nil
}
