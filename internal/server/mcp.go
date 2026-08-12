package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpPolicy struct {
	Enabled      bool     `json:"enabled"`
	AllowedTools []string `json:"allowedTools"`
}

func (s *Server) loadMCPPolicy(r *http.Request) mcpPolicy {
	tools := mcpTools()
	policy := mcpPolicy{Enabled: true, AllowedTools: make([]string, 0, len(tools))}
	for _, tool := range tools {
		policy.AllowedTools = append(policy.AllowedTools, tool.Name)
	}
	_, _ = s.getSetting(r.Context(), "integration", "mcp", &policy)
	return policy
}

func (s *Server) mcpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(405)
			return
		}
		var req rpcRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, 400, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "Parse error"}})
			return
		}
		policy := s.loadMCPPolicy(r)
		if !policy.Enabled {
			writeJSON(w, http.StatusServiceUnavailable, rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32004, Message: "MCP is disabled by administrator policy"}})
			return
		}
		response := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			response.Result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]string{"name": "Planexus", "version": s.version}}
		case "notifications/initialized":
			w.WriteHeader(202)
			return
		case "tools/list":
			tools := []mcpTool{}
			for _, tool := range mcpTools() {
				if stringIn(policy.AllowedTools, tool.Name) {
					tools = append(tools, tool)
				}
			}
			response.Result = map[string]any{"tools": tools}
		case "tools/call":
			result, e := s.callMCPTool(r, req.Params, policy)
			if e != nil {
				response.Error = e
			} else {
				response.Result = map[string]any{"content": []map[string]any{{"type": "text", "text": string(result)}}, "isError": false}
			}
		default:
			response.Error = &rpcError{Code: -32601, Message: "Method not found"}
		}
		writeJSON(w, 200, response)
	})
}

func mcpTools() []mcpTool {
	obj := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	str := map[string]any{"type": "string"}
	return []mcpTool{
		{Name: "get_strategy", Description: "Get one authorized strategy by UUID", InputSchema: obj(map[string]any{"id": str}, "id")},
		{Name: "search_strategy", Description: "Search authorized strategies by name and description", InputSchema: obj(map[string]any{"query": str}, "query")},
		{Name: "get_kpi", Description: "Get one authorized KPI by UUID", InputSchema: obj(map[string]any{"id": str}, "id")},
		{Name: "get_kpi_performance", Description: "Get target, actual and achievement for one KPI", InputSchema: obj(map[string]any{"id": str}, "id")},
		{Name: "get_projects", Description: "List authorized strategic projects", InputSchema: obj(map[string]any{})},
		{Name: "get_project_risk", Description: "List high and critical project risks", InputSchema: obj(map[string]any{})},
		{Name: "get_budget_status", Description: "Summarize project budget and actual costs", InputSchema: obj(map[string]any{})},
		{Name: "get_decision_history", Description: "Get recent authorized decision history", InputSchema: obj(map[string]any{"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}})},
		{Name: "search_intelligence", Description: "Search authorized market, competitor, regulation and technology intelligence", InputSchema: obj(map[string]any{"query": str}, "query")},
		{Name: "run_scenario", Description: "Run one authorized scenario simulation by UUID", InputSchema: obj(map[string]any{"id": str}, "id")},
		{Name: "generate_executive_brief", Description: "Generate an evidence-based executive brief from current Planexus data", InputSchema: obj(map[string]any{})},
	}
}

func (s *Server) callMCPTool(r *http.Request, raw json.RawMessage, policy mcpPolicy) (json.RawMessage, *rpcError) {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if json.Unmarshal(raw, &call) != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}
	if !stringIn(policy.AllowedTools, call.Name) {
		return nil, &rpcError{Code: -32003, Message: "Tool disabled by administrator policy"}
	}
	p, _ := auth.PrincipalFrom(r.Context())
	permission := map[string]string{"get_strategy": "strategy:read", "search_strategy": "strategy:read", "get_kpi": "kpi:read", "get_kpi_performance": "kpi:read", "get_projects": "project:read", "get_project_risk": "project:read", "get_budget_status": "project:read", "get_decision_history": "decision:read", "search_intelligence": "intelligence:read", "run_scenario": "scenario:*", "generate_executive_brief": "dashboard:read"}[call.Name]
	if permission == "" {
		return nil, &rpcError{Code: -32602, Message: "Unknown tool"}
	}
	if !p.Has(permission) {
		return nil, &rpcError{Code: -32003, Message: "Permission denied"}
	}
	var result any
	var err error
	switch call.Name {
	case "get_strategy":
		result, err = s.mcpStrategy(r, p, stringArg(call.Arguments, "id"))
	case "search_strategy":
		result, err = s.mcpSearchStrategy(r, p, stringArg(call.Arguments, "query"))
	case "get_kpi", "get_kpi_performance":
		result, err = s.mcpKPI(r, p, stringArg(call.Arguments, "id"))
	case "get_projects":
		result, err = s.mcpProjects(r, p, false)
	case "get_project_risk":
		result, err = s.mcpProjects(r, p, true)
	case "get_budget_status":
		result, err = s.mcpBudget(r, p)
	case "get_decision_history":
		limit := intArg(call.Arguments, "limit", 20)
		if limit > 100 {
			limit = 100
		}
		rows, e := s.pool.Query(r.Context(), `SELECT id,title,decision_date,decision,reason,classification,organization_id FROM decisions ORDER BY decision_date DESC NULLS LAST LIMIT $1`, limit)
		if e != nil {
			err = e
			break
		}
		defer rows.Close()
		items := []map[string]any{}
		for rows.Next() {
			var id uuid.UUID
			var title, decision, reason, class string
			var date any
			var org *uuid.UUID
			if e = rows.Scan(&id, &title, &date, &decision, &reason, &class, &org); e != nil {
				err = e
				break
			}
			if p.Can("decision:read", auth.Resource{OrganizationID: org, Classification: class}) {
				items = append(items, map[string]any{"id": id, "title": title, "date": date, "decision": decision, "reason": reason, "classification": class})
			}
		}
		result = items
	case "search_intelligence":
		result, err = s.mcpSearchIntelligence(r, p, stringArg(call.Arguments, "query"))
	case "run_scenario":
		var id uuid.UUID
		id, err = uuid.Parse(stringArg(call.Arguments, "id"))
		if err == nil {
			result, _, _, err = s.simulateScenario(r, p, id)
		}
	case "generate_executive_brief":
		result, err = s.mcpBrief(r, p)
	}
	if err != nil {
		return nil, &rpcError{Code: -32000, Message: "Tool execution failed"}
	}
	data, _ := json.Marshal(result)
	s.audit(r, &p.ID, p.Username, "View", "mcp_tool", "", call.Name, "success", map[string]any{"arguments": call.Arguments})
	return data, nil
}

func (s *Server) mcpStrategy(r *http.Request, p auth.Principal, idText string) (any, error) {
	id, err := uuid.Parse(idText)
	if err != nil {
		return nil, err
	}
	var x strategyDTO
	err = s.pool.QueryRow(r.Context(), `SELECT id,parent_id,name,kind,description,period_start,period_end,version,status,classification,organization_id,owner_id,created_at FROM strategies WHERE id=$1`, id).Scan(&x.ID, &x.ParentID, &x.Name, &x.Kind, &x.Description, &x.PeriodStart, &x.PeriodEnd, &x.Version, &x.Status, &x.Classification, &x.OrganizationID, &x.OwnerID, &x.CreatedAt)
	if err != nil {
		return nil, err
	}
	if !p.Can("strategy:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
		return nil, authError{}
	}
	return x, nil
}
func (s *Server) mcpSearchStrategy(r *http.Request, p auth.Principal, query string) (any, error) {
	rows, err := s.pool.Query(r.Context(), `SELECT id,parent_id,name,kind,description,period_start,period_end,version,status,classification,organization_id,owner_id,created_at FROM strategies WHERE name ILIKE $1 OR description ILIKE $1 ORDER BY name LIMIT 50`, "%"+strings.ReplaceAll(query, "%", "\\%")+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []strategyDTO{}
	for rows.Next() {
		var x strategyDTO
		if err = rows.Scan(&x.ID, &x.ParentID, &x.Name, &x.Kind, &x.Description, &x.PeriodStart, &x.PeriodEnd, &x.Version, &x.Status, &x.Classification, &x.OrganizationID, &x.OwnerID, &x.CreatedAt); err != nil {
			return nil, err
		}
		if p.Can("strategy:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	return items, rows.Err()
}
func (s *Server) mcpKPI(r *http.Request, p auth.Principal, idText string) (any, error) {
	id, err := uuid.Parse(idText)
	if err != nil {
		return nil, err
	}
	var x kpiDTO
	err = s.pool.QueryRow(r.Context(), `SELECT id,strategy_id,parent_id,code,name,description,formula,unit,frequency,target,actual,CASE WHEN target=0 THEN 100 ELSE actual/target*100 END,weight,source,classification,organization_id,owner_id FROM kpis WHERE id=$1`, id).Scan(&x.ID, &x.StrategyID, &x.ParentID, &x.Code, &x.Name, &x.Description, &x.Formula, &x.Unit, &x.Frequency, &x.Target, &x.Actual, &x.Achievement, &x.Weight, &x.Source, &x.Classification, &x.OrganizationID, &x.OwnerID)
	if err != nil {
		return nil, err
	}
	if !p.Can("kpi:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
		return nil, authError{}
	}
	return x, nil
}
func (s *Server) mcpProjects(r *http.Request, p auth.Principal, riskOnly bool) (any, error) {
	query := `SELECT id,strategy_id,name,description,status,progress,risk,budget,actual_cost,start_date,end_date,score,classification,organization_id,owner_id FROM projects`
	if riskOnly {
		query += ` WHERE risk IN ('high','critical')`
	}
	query += ` ORDER BY updated_at DESC LIMIT 100`
	rows, err := s.pool.Query(r.Context(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []projectDTO{}
	for rows.Next() {
		var x projectDTO
		if err = rows.Scan(&x.ID, &x.StrategyID, &x.Name, &x.Description, &x.Status, &x.Progress, &x.Risk, &x.Budget, &x.ActualCost, &x.StartDate, &x.EndDate, &x.Score, &x.Classification, &x.OrganizationID, &x.OwnerID); err != nil {
			return nil, err
		}
		if p.Can("project:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	return items, rows.Err()
}

func (s *Server) mcpSearchIntelligence(r *http.Request, p auth.Principal, query string) (any, error) {
	rows, err := s.pool.Query(r.Context(), `SELECT id,category,title,source_name,source_url,published_at,summary,importance,company_relevance,potential_impact,risk,opportunity,recommended_action,classification,organization_id,owner_id,created_at FROM intelligence_items WHERE title ILIKE '%'||$1||'%' OR summary ILIKE '%'||$1||'%' ORDER BY importance DESC LIMIT 100`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []intelligenceDTO{}
	for rows.Next() {
		var x intelligenceDTO
		if err = rows.Scan(&x.ID, &x.Category, &x.Title, &x.SourceName, &x.SourceURL, &x.PublishedAt, &x.Summary, &x.Importance, &x.CompanyRelevance, &x.PotentialImpact, &x.Risk, &x.Opportunity, &x.RecommendedAction, &x.Classification, &x.OrganizationID, &x.OwnerID, &x.CreatedAt); err != nil {
			return nil, err
		}
		if p.Can("intelligence:read", auth.Resource{OwnerID: x.OwnerID, OrganizationID: x.OrganizationID, Classification: x.Classification}) {
			items = append(items, x)
		}
	}
	return items, rows.Err()
}
func (s *Server) mcpBudget(r *http.Request, p auth.Principal) (any, error) {
	rows, err := s.pool.Query(r.Context(), `SELECT budget,actual_cost,classification,organization_id,owner_id FROM projects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var budget, actual float64
	for rows.Next() {
		var b, a float64
		var class string
		var org, owner *uuid.UUID
		if err = rows.Scan(&b, &a, &class, &org, &owner); err != nil {
			return nil, err
		}
		if p.Can("project:read", auth.Resource{OwnerID: owner, OrganizationID: org, Classification: class}) {
			budget += b
			actual += a
		}
	}
	return map[string]any{"budget": budget, "actualCost": actual, "variance": budget - actual, "generatedAt": time.Now()}, rows.Err()
}

func (s *Server) mcpBrief(r *http.Request, p auth.Principal) (any, error) {
	rows, err := s.pool.Query(r.Context(), `SELECT target,actual,classification,organization_id,owner_id FROM kpis`)
	if err != nil {
		return nil, err
	}
	var sum float64
	var count int
	for rows.Next() {
		var target, actual float64
		var class string
		var org, owner *uuid.UUID
		if err = rows.Scan(&target, &actual, &class, &org, &owner); err != nil {
			rows.Close()
			return nil, err
		}
		if p.Can("kpi:read", auth.Resource{OwnerID: owner, OrganizationID: org, Classification: class}) {
			sum += min(100, safePercent(actual, target))
			if target == 0 {
				sum += 100
			}
			count++
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	health := float64(0)
	if count > 0 {
		health = sum / float64(count)
	}
	projects, err := s.mcpProjects(r, p, true)
	if err != nil {
		return nil, err
	}
	highRisk := len(projects.([]projectDTO))
	budgetResult, err := s.mcpBudget(r, p)
	if err != nil {
		return nil, err
	}
	budgetMap := budgetResult.(map[string]any)
	budget := budgetMap["budget"].(float64)
	actual := budgetMap["actualCost"].(float64)
	var pending int
	planRows, err := s.pool.Query(r.Context(), `SELECT classification,organization_id,owner_id FROM plans WHERE status IN ('draft','in_review','changes_requested')`)
	if err != nil {
		return nil, err
	}
	for planRows.Next() {
		var classification string
		var organizationID, ownerID *uuid.UUID
		if err = planRows.Scan(&classification, &organizationID, &ownerID); err != nil {
			planRows.Close()
			return nil, err
		}
		allowed := p.Has("plan:*") || p.Has("dashboard:executive") || (p.Has("plan:own") && ownerID != nil && *ownerID == p.ID) || (p.Has("plan:organization") && p.OrganizationID != nil && organizationID != nil && *p.OrganizationID == *organizationID)
		if allowed && p.Can("dashboard:read", auth.Resource{OwnerID: ownerID, OrganizationID: organizationID, Classification: classification}) {
			pending++
		}
	}
	planRows.Close()
	if err = planRows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"answer": []string{"Company KPI achievement: " + formatPercent(health), "High-risk projects: " + formatInt(highRisk), "Plans requiring action: " + formatInt(pending), "Budget utilization: " + formatPercent(safePercent(actual, budget))}, "confidence": 1, "evidence": map[string]any{"kpiAchievement": health, "highRiskProjects": highRisk, "pendingPlans": pending, "budget": budget, "actualCost": actual}, "source": "Planexus authorized transactional data", "generatedAt": time.Now(), "model": "deterministic"}, nil
}

type authError struct{}

func (authError) Error() string { return "permission denied" }
func stringArg(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}
func intArg(values map[string]any, key string, fallback int) int {
	if value, ok := values[key].(float64); ok {
		return int(value)
	}
	return fallback
}
func safePercent(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b * 100
}
func formatPercent(value float64) string { return formatFloat(value) + "%" }
func formatFloat(value float64) string   { return fmtFloat(value) }
func formatInt(value int) string         { return fmtInt(value) }
