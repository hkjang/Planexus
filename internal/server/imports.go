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
	"github.com/xuri/excelize/v2"
)

const maxImportFileSize = 20 << 20
const maxImportRows = 10000

type importField struct {
	Key      string
	Header   string
	Required bool
	Aliases  []string
}

var importFields = map[string][]importField{
	"strategy": {{"name", "Name", true, []string{"전략명", "strategy name"}}, {"kind", "Kind", true, []string{"유형", "type"}}, {"description", "Description", false, []string{"설명"}}, {"classification", "Classification", false, []string{"보안등급"}}, {"status", "Status", false, []string{"상태"}}},
	"kpi":      {{"code", "Code", true, []string{"kpi code", "kpi id"}}, {"name", "Name", true, []string{"kpi명", "kpi name"}}, {"description", "Description", false, []string{"설명"}}, {"target", "Target", false, []string{"목표"}}, {"actual", "Actual", false, []string{"실적"}}, {"unit", "Unit", false, []string{"단위"}}, {"frequency", "Frequency", false, []string{"주기"}}, {"weight", "Weight", false, []string{"가중치"}}, {"source", "Source", false, []string{"출처"}}, {"classification", "Classification", false, []string{"보안등급"}}},
	"project":  {{"name", "Name", true, []string{"프로젝트명", "project name"}}, {"description", "Description", false, []string{"설명"}}, {"status", "Status", false, []string{"상태"}}, {"progress", "Progress", false, []string{"진척률"}}, {"risk", "Risk", false, []string{"위험"}}, {"budget", "Budget", false, []string{"예산"}}, {"actual_cost", "Actual Cost", false, []string{"실제비용", "actualcost"}}, {"classification", "Classification", false, []string{"보안등급"}}},
}

type importValidationError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (s *Server) downloadImportTemplate(w http.ResponseWriter, r *http.Request) {
	entity := chi.URLParam(r, "entityType")
	fields, ok := importFields[entity]
	if !ok {
		writeError(w, 400, "unsupported_import_entity", nil)
		return
	}
	file := excelize.NewFile()
	defer file.Close()
	sheet := "Import"
	_ = file.SetSheetName("Sheet1", sheet)
	for i, field := range fields {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = file.SetCellValue(sheet, cell, field.Header)
	}
	if r.URL.Query().Get("sample") == "true" {
		samples := map[string]map[string]any{
			"strategy": {"name": "Imported Growth Objective", "kind": "objective", "description": "Sample import row", "classification": "internal", "status": "draft"},
			"kpi":      {"code": "SAMPLE-001", "name": "Sample KPI", "target": 100, "actual": 80, "unit": "%", "frequency": "monthly", "classification": "internal"},
			"project":  {"name": "Sample Project", "status": "planned", "progress": 0, "risk": "low", "budget": 1000000, "actual_cost": 0, "classification": "internal"},
		}
		for i, field := range fields {
			if value, exists := samples[entity][field.Key]; exists {
				cell, _ := excelize.CoordinatesToCellName(i+1, 2)
				_ = file.SetCellValue(sheet, cell, value)
			}
		}
	}
	headerStyle, _ := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"175CD3"}, Pattern: 1}, Alignment: &excelize.Alignment{Vertical: "center"}})
	end, _ := excelize.CoordinatesToCellName(len(fields), 1)
	_ = file.SetCellStyle(sheet, "A1", end, headerStyle)
	_ = file.SetRowHeight(sheet, 1, 26)
	for i := range fields {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = file.SetColWidth(sheet, col, col, 20)
	}
	_ = file.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="planexus-%s-import-template.xlsx"`, entity))
	if err := file.Write(w); err != nil {
		return
	}
	p, _ := auth.PrincipalFrom(r.Context())
	s.audit(r, &p.ID, p.Username, "Export", "import_template", entity, "download", "success", nil)
}

func (s *Server) previewImport(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxImportFileSize+(1<<20))
	if err := r.ParseMultipartForm(maxImportFileSize); err != nil {
		writeError(w, 400, "invalid_import_form", err)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	entity := strings.TrimSpace(r.FormValue("entityType"))
	fields, ok := importFields[entity]
	if !ok {
		writeError(w, 400, "unsupported_import_entity", nil)
		return
	}
	upload, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "import_file_required", err)
		return
	}
	defer upload.Close()
	if header.Size <= 0 || header.Size > maxImportFileSize {
		writeError(w, 400, "import_file_size", errors.New("XLSX must be between 1 byte and 20MB"))
		return
	}
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
		writeError(w, 400, "import_file_type", errors.New("only .xlsx files are accepted"))
		return
	}
	book, err := excelize.OpenReader(upload, excelize.Options{RawCellValue: true, UnzipSizeLimit: 100 << 20, UnzipXMLSizeLimit: 100 << 20})
	if err != nil {
		writeError(w, 400, "invalid_xlsx", err)
		return
	}
	defer book.Close()
	sheets := book.GetSheetList()
	if len(sheets) == 0 {
		writeError(w, 400, "empty_workbook", nil)
		return
	}
	rows, err := book.Rows(sheets[0])
	if err != nil {
		writeError(w, 400, "worksheet_read_failed", err)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		writeError(w, 400, "empty_workbook", nil)
		return
	}
	headers, err := rows.Columns()
	if err != nil {
		writeError(w, 400, "worksheet_read_failed", err)
		return
	}
	if len(headers) > 100 {
		writeError(w, 400, "too_many_columns", nil)
		return
	}
	var requested map[string]string
	if raw := strings.TrimSpace(r.FormValue("mapping")); raw != "" {
		if err = json.Unmarshal([]byte(raw), &requested); err != nil {
			writeError(w, 400, "invalid_column_mapping", err)
			return
		}
	}
	mapping, validation := resolveImportMapping(fields, headers, requested)
	valid := []map[string]string{}
	total := 0
	seenCodes := map[string]bool{}
	for rows.Next() {
		if total >= maxImportRows {
			validation = append(validation, importValidationError{Row: total + 2, Message: "maximum 10000 data rows exceeded"})
			break
		}
		columns, readErr := rows.Columns()
		if readErr != nil {
			writeError(w, 400, "worksheet_read_failed", readErr)
			return
		}
		if emptyExcelRow(columns) {
			continue
		}
		total++
		rowNumber := total + 1
		item := map[string]string{}
		for _, field := range fields {
			if index, exists := mapping[field.Key]; exists && index < len(columns) {
				value := strings.TrimSpace(columns[index])
				if len(value) > 100000 {
					validation = append(validation, importValidationError{Row: rowNumber, Field: field.Key, Message: "cell exceeds 100000 characters"})
					continue
				}
				item[field.Key] = value
			}
		}
		rowErrors := validateImportRow(entity, rowNumber, item)
		classification := defaultString(strings.ToLower(item["classification"]), "internal")
		if !p.Can(entity+":*", auth.Resource{Classification: classification}) {
			rowErrors = append(rowErrors, importValidationError{Row: rowNumber, Field: "classification", Message: "current user cannot import this security classification or entity"})
		}
		if entity == "kpi" && item["code"] != "" {
			normalized := strings.ToLower(item["code"])
			if seenCodes[normalized] {
				rowErrors = append(rowErrors, importValidationError{Row: rowNumber, Field: "code", Message: "duplicate code in workbook"})
			}
			seenCodes[normalized] = true
		}
		validation = append(validation, rowErrors...)
		if len(rowErrors) == 0 {
			valid = append(valid, item)
		}
	}
	if err = rows.Error(); err != nil {
		writeError(w, 400, "worksheet_read_failed", err)
		return
	}
	dataJSON, _ := json.Marshal(valid)
	errorsJSON, _ := json.Marshal(validation)
	mappingJSON, _ := json.Marshal(mappingNames(mapping, headers))
	invalidCount := total - len(valid)
	if len(validation) > 0 && invalidCount == 0 {
		invalidCount = 1
	}
	jobID := uuid.New()
	_, err = s.pool.Exec(r.Context(), `INSERT INTO import_jobs(id,entity_type,file_name,mapping,preview_data,validation_errors,total_rows,valid_rows,invalid_rows,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, jobID, entity, safeFileName(header.Filename), mappingJSON, dataJSON, errorsJSON, total, len(valid), invalidCount, p.ID)
	if err != nil {
		writeError(w, 500, "import_preview_store_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "import_job", jobID.String(), "preview", "success", map[string]any{"entityType": entity, "totalRows": total, "validRows": len(valid)})
	preview := valid
	if len(preview) > 50 {
		preview = preview[:50]
	}
	writeJSON(w, 201, map[string]any{"id": jobID, "entityType": entity, "fileName": safeFileName(header.Filename), "mapping": mappingNames(mapping, headers), "totalRows": total, "validRows": len(valid), "invalidRows": invalidCount, "errors": validation, "preview": preview, "previewTruncated": len(valid) > 50})
}

func resolveImportMapping(fields []importField, headers []string, requested map[string]string) (map[string]int, []importValidationError) {
	headerIndex := map[string]int{}
	for i, h := range headers {
		headerIndex[normalizeHeader(h)] = i
	}
	result := map[string]int{}
	validation := []importValidationError{}
	for _, field := range fields {
		candidates := []string{field.Header, field.Key}
		candidates = append(candidates, field.Aliases...)
		if requested[field.Key] != "" {
			candidates = []string{requested[field.Key]}
		}
		for _, candidate := range candidates {
			if index, ok := headerIndex[normalizeHeader(candidate)]; ok {
				result[field.Key] = index
				break
			}
		}
		if _, ok := result[field.Key]; !ok && field.Required {
			validation = append(validation, importValidationError{Row: 1, Field: field.Key, Message: "required column is not mapped"})
		}
	}
	return result, validation
}
func validateImportRow(entity string, row int, item map[string]string) []importValidationError {
	errorsList := []importValidationError{}
	requiredFields := map[string][]string{"strategy": {"name", "kind"}, "kpi": {"code", "name"}, "project": {"name"}}[entity]
	for _, field := range requiredFields {
		if item[field] == "" {
			errorsList = append(errorsList, importValidationError{Row: row, Field: field, Message: "required value is empty"})
		}
	}
	if entity == "strategy" && item["kind"] != "" && !oneOf(strings.ToLower(item["kind"]), "vision", "mission", "theme", "objective") {
		errorsList = append(errorsList, importValidationError{Row: row, Field: "kind", Message: "must be vision, mission, theme or objective"})
	}
	numeric := []string{}
	if entity == "kpi" {
		numeric = []string{"target", "actual", "weight"}
	}
	if entity == "project" {
		numeric = []string{"progress", "budget", "actual_cost"}
	}
	for _, field := range numeric {
		if value := item[field]; value != "" {
			number, err := parseImportNumber(value)
			if err != nil {
				errorsList = append(errorsList, importValidationError{Row: row, Field: field, Message: "must be numeric"})
			} else if field == "progress" && (number < 0 || number > 100) {
				errorsList = append(errorsList, importValidationError{Row: row, Field: field, Message: "must be between 0 and 100"})
			}
		}
	}
	if value := strings.ToLower(item["classification"]); value != "" && !oneOf(value, "public", "internal", "confidential", "executive", "restricted") {
		errorsList = append(errorsList, importValidationError{Row: row, Field: "classification", Message: "unknown security classification"})
	}
	return errorsList
}

func (s *Server) commitImport(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_import_id", err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	defer tx.Rollback(r.Context())
	var entity, status string
	var data json.RawMessage
	var invalid int
	err = tx.QueryRow(r.Context(), `SELECT entity_type,status,preview_data,invalid_rows FROM import_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&entity, &status, &data, &invalid)
	if err != nil {
		writeError(w, 404, "import_not_found", nil)
		return
	}
	if status != "preview" {
		writeError(w, 409, "import_not_committable", nil)
		return
	}
	if invalid > 0 {
		writeError(w, 409, "import_has_validation_errors", nil)
		return
	}
	var records []map[string]string
	if err = json.Unmarshal(data, &records); err != nil {
		writeError(w, 500, "import_data_corrupt", err)
		return
	}
	createdAt := time.Now()
	for _, record := range records {
		classification := defaultString(strings.ToLower(record["classification"]), "internal")
		if !p.Can(entity+":*", auth.Resource{Classification: classification}) {
			writeError(w, http.StatusForbidden, "forbidden", nil)
			return
		}
		resourceID := uuid.New()
		if err = insertImportRecord(r.Context(), tx, entity, resourceID, p.ID, createdAt, record); err != nil {
			writeError(w, 409, "import_commit_failed", err)
			return
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO import_job_records(import_job_id,resource_type,resource_id,created_at) VALUES($1,$2,$3,$4)`, id, entity, resourceID, createdAt); err != nil {
			writeError(w, 500, "import_commit_failed", err)
			return
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE import_jobs SET status='imported',imported_at=now(),preview_data='[]'::jsonb WHERE id=$1`, id)
	if err != nil {
		writeError(w, 500, "import_commit_failed", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Create", "import_job", id.String(), "commit", "success", map[string]any{"entityType": entity, "records": len(records)})
	writeJSON(w, 200, map[string]any{"id": id, "status": "imported", "importedRows": len(records)})
}

func insertImportRecord(ctx context.Context, tx pgx.Tx, entity string, id, userID uuid.UUID, createdAt time.Time, item map[string]string) error {
	classification := defaultString(strings.ToLower(item["classification"]), "internal")
	switch entity {
	case "strategy":
		_, err := tx.Exec(ctx, `INSERT INTO strategies(id,name,kind,description,status,classification,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8)`, id, item["name"], strings.ToLower(item["kind"]), item["description"], defaultString(item["status"], "draft"), classification, userID, createdAt)
		return err
	case "kpi":
		target, _ := parseImportNumber(item["target"])
		actual, _ := parseImportNumber(item["actual"])
		weight, _ := parseImportNumber(item["weight"])
		_, err := tx.Exec(ctx, `INSERT INTO kpis(id,code,name,description,unit,frequency,target,actual,weight,source,classification,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)`, id, item["code"], item["name"], item["description"], item["unit"], defaultString(item["frequency"], "monthly"), target, actual, weight, item["source"], classification, userID, createdAt)
		return err
	case "project":
		progress, _ := parseImportNumber(item["progress"])
		budget, _ := parseImportNumber(item["budget"])
		actual, _ := parseImportNumber(item["actual_cost"])
		_, err := tx.Exec(ctx, `INSERT INTO projects(id,name,description,status,progress,risk,budget,actual_cost,classification,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)`, id, item["name"], item["description"], defaultString(item["status"], "planned"), progress, defaultString(strings.ToLower(item["risk"]), "low"), budget, actual, classification, userID, createdAt)
		return err
	default:
		return errors.New("unsupported import entity")
	}
}

func (s *Server) rollbackImport(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 400, "invalid_import_id", err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	defer tx.Rollback(r.Context())
	var status string
	err = tx.QueryRow(r.Context(), `SELECT status FROM import_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&status)
	if err != nil {
		writeError(w, 404, "import_not_found", nil)
		return
	}
	if status != "imported" {
		writeError(w, 409, "import_not_rollbackable", nil)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT resource_type,resource_id,created_at FROM import_job_records WHERE import_job_id=$1`, id)
	if err != nil {
		writeError(w, 500, "rollback_query_failed", err)
		return
	}
	type record struct {
		kind    string
		id      uuid.UUID
		created time.Time
	}
	records := []record{}
	for rows.Next() {
		var x record
		if err = rows.Scan(&x.kind, &x.id, &x.created); err != nil {
			rows.Close()
			writeError(w, 500, "rollback_query_failed", err)
			return
		}
		records = append(records, x)
	}
	rows.Close()
	for _, record := range records {
		table := map[string]string{"strategy": "strategies", "kpi": "kpis", "project": "projects"}[record.kind]
		if table == "" {
			writeError(w, 500, "rollback_record_invalid", nil)
			return
		}
		tag, deleteErr := tx.Exec(r.Context(), `DELETE FROM `+table+` WHERE id=$1 AND updated_at=$2`, record.id, record.created)
		if deleteErr != nil || tag.RowsAffected() != 1 {
			writeError(w, 409, "rollback_conflict", errors.New("an imported record was changed or referenced after import"))
			return
		}
	}
	_, err = tx.Exec(r.Context(), `DELETE FROM import_job_records WHERE import_job_id=$1`, id)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE import_jobs SET status='rolled_back',rolled_back_at=now() WHERE id=$1`, id)
	}
	if err != nil {
		writeError(w, 500, "rollback_failed", err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "transaction_failed", err)
		return
	}
	s.audit(r, &p.ID, p.Username, "Delete", "import_job", id.String(), "rollback", "success", map[string]any{"records": len(records)})
	writeJSON(w, 200, map[string]any{"id": id, "status": "rolled_back", "removedRows": len(records)})
}

func (s *Server) listImports(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT id,entity_type,file_name,mapping,validation_errors,status,total_rows,valid_rows,invalid_rows,created_by,created_at,imported_at,rolled_back_at FROM import_jobs ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		writeError(w, 500, "import_history_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, user uuid.UUID
		var entity, file, status string
		var mapping, validation json.RawMessage
		var total, valid, invalid int
		var created time.Time
		var imported, rolled *time.Time
		if err = rows.Scan(&id, &entity, &file, &mapping, &validation, &status, &total, &valid, &invalid, &user, &created, &imported, &rolled); err != nil {
			writeError(w, 500, "import_history_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "entityType": entity, "fileName": file, "mapping": mapping, "errors": validation, "status": status, "totalRows": total, "validRows": valid, "invalidRows": invalid, "createdBy": user, "createdAt": created, "importedAt": imported, "rolledBackAt": rolled})
	}
	writeJSON(w, 200, items)
}

func normalizeHeader(value string) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "", `.`, "").Replace(strings.TrimSpace(value)))
}
func mappingNames(mapping map[string]int, headers []string) map[string]string {
	result := map[string]string{}
	for key, index := range mapping {
		if index >= 0 && index < len(headers) {
			result[key] = headers[index]
		}
	}
	return result
}
func emptyExcelRow(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
func parseImportNumber(value string) (float64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), 64)
}
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func safeFileName(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	parts := strings.Split(value, "/")
	value = parts[len(parts)-1]
	value = strings.Map(func(r rune) rune {
		if r < ' ' || r == '\x7f' {
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > 255 {
		value = string(runes[:255])
	}
	return value
}
