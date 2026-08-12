package server

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
)

type searchResult struct {
	ID             uuid.UUID  `json:"id"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	Subtitle       string     `json:"subtitle"`
	Classification string     `json:"classification"`
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	OwnerID        *uuid.UUID `json:"ownerId,omitempty"`
}

func (s *Server) globalSearch(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 {
		writeJSON(w, http.StatusOK, []searchResult{})
		return
	}
	like := "%" + escapeLike(query) + "%"
	rows, err := s.pool.Query(r.Context(), `
		SELECT entity_type,id,title,subtitle,classification,organization_id,owner_id FROM (
			SELECT 'strategy'::text entity_type,id,name title,description subtitle,classification,organization_id,owner_id,updated_at FROM strategies WHERE name ILIKE $1 ESCAPE '\' OR description ILIKE $1 ESCAPE '\'
			UNION ALL SELECT 'kpi',id,name,code || CASE WHEN description='' THEN '' ELSE ' · ' || description END,classification,organization_id,owner_id,updated_at FROM kpis WHERE name ILIKE $1 ESCAPE '\' OR code ILIKE $1 ESCAPE '\' OR description ILIKE $1 ESCAPE '\'
			UNION ALL SELECT 'project',id,name,description,classification,organization_id,owner_id,updated_at FROM projects WHERE name ILIKE $1 ESCAPE '\' OR description ILIKE $1 ESCAPE '\'
			UNION ALL SELECT 'plan',id,title,period,classification,organization_id,owner_id,updated_at FROM plans WHERE title ILIKE $1 ESCAPE '\' OR period ILIKE $1 ESCAPE '\'
			UNION ALL SELECT 'decision',id,title,decision || CASE WHEN reason='' THEN '' ELSE ' · ' || reason END,classification,organization_id,created_by,updated_at FROM decisions WHERE title ILIKE $1 ESCAPE '\' OR decision ILIKE $1 ESCAPE '\' OR reason ILIKE $1 ESCAPE '\'
			UNION ALL SELECT 'intelligence',id,title,summary,classification,organization_id,owner_id,updated_at FROM intelligence_items WHERE title ILIKE $1 ESCAPE '\' OR summary ILIKE $1 ESCAPE '\' OR raw_content ILIKE $1 ESCAPE '\'
		) search_index ORDER BY updated_at DESC LIMIT 100`, like)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_failed", err)
		return
	}
	defer rows.Close()
	items := []searchResult{}
	permissions := map[string]string{
		"strategy": "strategy:read", "kpi": "kpi:read", "project": "project:read",
		"decision": "decision:read", "intelligence": "intelligence:read",
	}
	for rows.Next() {
		var item searchResult
		if err := rows.Scan(&item.Type, &item.ID, &item.Title, &item.Subtitle, &item.Classification, &item.OrganizationID, &item.OwnerID); err != nil {
			writeError(w, http.StatusInternalServerError, "search_scan_failed", err)
			return
		}
		allowed := false
		if item.Type == "plan" {
			allowed = canReadPlan(p, item.OwnerID, item.OrganizationID, item.Classification)
		} else if permission := permissions[item.Type]; permission != "" {
			allowed = p.Can(permission, auth.Resource{OwnerID: item.OwnerID, OrganizationID: item.OrganizationID, Classification: item.Classification})
		}
		if allowed {
			items = append(items, item)
			if len(items) == 30 {
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "search_scan_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "View", "global_search", "", "search", "success", map[string]any{"queryLength": len([]rune(query)), "resultCount": len(items)})
	writeJSON(w, http.StatusOK, items)
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
