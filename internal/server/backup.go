package server

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/Planexus/internal/auth"
	"github.com/jackc/pgx/v5"
)

type backupTable struct {
	Name  string
	Query string
}

var backupTables = []backupTable{
	{"organizations", `WITH RECURSIVE tree AS (SELECT id,0 depth,ARRAY[id] path FROM organizations WHERE parent_id IS NULL UNION ALL SELECT o.id,t.depth+1,t.path||o.id FROM organizations o JOIN tree t ON o.parent_id=t.id WHERE NOT o.id=ANY(t.path)) SELECT row_to_json(o)::text FROM tree t JOIN organizations o ON o.id=t.id ORDER BY t.depth,o.code`},
	{"users", `SELECT row_to_json(t)::text FROM users t ORDER BY created_at,id`}, {"roles", `SELECT row_to_json(t)::text FROM roles t ORDER BY id`}, {"user_roles", `SELECT row_to_json(t)::text FROM user_roles t ORDER BY user_id,role_id`}, {"system_settings", `SELECT row_to_json(t)::text FROM system_settings t ORDER BY category,key`},
	{"strategies", `WITH RECURSIVE tree AS (SELECT id,0 depth,ARRAY[id] path FROM strategies WHERE parent_id IS NULL UNION ALL SELECT s.id,t.depth+1,t.path||s.id FROM strategies s JOIN tree t ON s.parent_id=t.id WHERE NOT s.id=ANY(t.path)) SELECT row_to_json(s)::text FROM tree t JOIN strategies s ON s.id=t.id ORDER BY t.depth,s.created_at`},
	{"kpis", `WITH RECURSIVE tree AS (SELECT id,0 depth,ARRAY[id] path FROM kpis WHERE parent_id IS NULL UNION ALL SELECT k.id,t.depth+1,t.path||k.id FROM kpis k JOIN tree t ON k.parent_id=t.id WHERE NOT k.id=ANY(t.path)) SELECT row_to_json(k)::text FROM tree t JOIN kpis k ON k.id=t.id ORDER BY t.depth,k.created_at`},
	{"projects", `SELECT row_to_json(t)::text FROM projects t ORDER BY created_at,id`}, {"plans", `SELECT row_to_json(t)::text FROM plans t ORDER BY created_at,id`}, {"workflow_definitions", `SELECT row_to_json(t)::text FROM workflow_definitions t ORDER BY updated_at,id`}, {"workflow_instances", `SELECT row_to_json(t)::text FROM workflow_instances t ORDER BY created_at,id`}, {"decisions", `SELECT row_to_json(t)::text FROM decisions t ORDER BY created_at,id`}, {"intelligence_items", `SELECT row_to_json(t)::text FROM intelligence_items t ORDER BY created_at,id`}, {"scenarios", `SELECT row_to_json(t)::text FROM scenarios t ORDER BY created_at,id`},
	{"personal_access_keys", `WITH RECURSIVE chain AS (SELECT id,0 depth FROM personal_access_keys WHERE replaced_by IS NULL UNION ALL SELECT k.id,c.depth+1 FROM personal_access_keys k JOIN chain c ON k.replaced_by=c.id) SELECT row_to_json(k)::text FROM chain c JOIN personal_access_keys k ON k.id=c.id ORDER BY c.depth,k.created_at`},
	{"audit_logs", `SELECT row_to_json(t)::text FROM audit_logs t ORDER BY occurred_at,id`}, {"ai_interactions", `SELECT row_to_json(t)::text FROM ai_interactions t ORDER BY created_at,id`}, {"import_jobs", `SELECT row_to_json(t)::text FROM import_jobs t ORDER BY created_at,id`}, {"import_job_records", `SELECT row_to_json(t)::text FROM import_job_records t ORDER BY import_job_id,resource_type,resource_id`}, {"notification_rules", `SELECT row_to_json(t)::text FROM notification_rules t ORDER BY created_at,id`}, {"notifications", `SELECT row_to_json(t)::text FROM notifications t ORDER BY created_at,id`},
}

type backupManifest struct {
	Format         string    `json:"format"`
	FormatVersion  int       `json:"formatVersion"`
	Service        string    `json:"service"`
	ServiceVersion string    `json:"serviceVersion"`
	SchemaVersion  int       `json:"schemaVersion"`
	KeyFingerprint string    `json:"keyFingerprint"`
	CreatedAt      time.Time `json:"createdAt"`
	Tables         []string  `json:"tables"`
}

type backupIntegrity struct {
	Algorithm string            `json:"algorithm"`
	Digests   map[string]string `json:"digests"`
	Signature string            `json:"signature"`
}
type countedHashWriter struct {
	writer io.Writer
	hash   hash.Hash
	count  int64
}

func (w *countedHashWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		_, _ = w.hash.Write(p[:n])
		w.count += int64(n)
	}
	return n, err
}

func (s *Server) exportBackup(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	jobID := uuid.New()
	fileName := "planexus-backup-" + time.Now().UTC().Format("20060102T150405Z") + ".plxbackup"
	_, err := s.pool.Exec(r.Context(), `INSERT INTO backup_jobs(id,operation,file_name,status,created_by) VALUES($1,'export',$2,'running',$3)`, jobID, fileName, p.ID)
	if err != nil {
		writeError(w, 500, "backup_start_failed", err)
		return
	}
	conn, err := s.pool.Acquire(r.Context())
	if err != nil {
		s.finishBackupJob(r.Context(), jobID, "failed", 0, "", map[string]any{"error": err.Error()})
		writeError(w, 500, "backup_connection_failed", err)
		return
	}
	defer conn.Release()
	tx, err := conn.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		writeError(w, 500, "backup_transaction_failed", err)
		return
	}
	defer tx.Rollback(r.Context())
	var schemaVersion int
	if err = tx.QueryRow(r.Context(), `SELECT COALESCE(max(version),0) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
		writeError(w, 500, "backup_manifest_failed", err)
		return
	}
	tableNames := make([]string, len(backupTables))
	for i, table := range backupTables {
		tableNames[i] = table.Name
	}
	manifest := backupManifest{Format: "planexus-logical-backup", FormatVersion: 2, Service: "Planexus", ServiceVersion: s.version, SchemaVersion: schemaVersion, KeyFingerprint: s.vault.Fingerprint(), CreatedAt: time.Now().UTC(), Tables: tableNames}
	w.Header().Set("Content-Type", "application/vnd.planexus.backup+zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	counter := &countedHashWriter{writer: w, hash: sha256.New()}
	archive := zip.NewWriter(counter)
	digests := map[string]string{}
	manifestEntry, err := archive.Create("manifest.json")
	if err == nil {
		manifestData, marshalErr := json.Marshal(manifest)
		if marshalErr != nil {
			err = marshalErr
		} else {
			manifestData = append(manifestData, '\n')
			_, err = manifestEntry.Write(manifestData)
			digest := sha256.Sum256(manifestData)
			digests["manifest.json"] = hex.EncodeToString(digest[:])
		}
	}
	for _, table := range backupTables {
		if err != nil {
			break
		}
		entryName := table.Name + ".ndjson"
		entry, createErr := archive.Create(entryName)
		if createErr != nil {
			err = createErr
			break
		}
		digest := sha256.New()
		entryWriter := io.MultiWriter(entry, digest)
		rows, queryErr := tx.Query(r.Context(), table.Query)
		if queryErr != nil {
			err = queryErr
			break
		}
		for rows.Next() {
			var line string
			if scanErr := rows.Scan(&line); scanErr != nil {
				err = scanErr
				break
			}
			if _, writeErr := io.WriteString(entryWriter, line+"\n"); writeErr != nil {
				err = writeErr
				break
			}
		}
		if rowErr := rows.Err(); err == nil && rowErr != nil {
			err = rowErr
		}
		rows.Close()
		if err != nil {
			break
		}
		digests[entryName] = hex.EncodeToString(digest.Sum(nil))
	}
	if err == nil {
		digestPayload, _ := json.Marshal(digests)
		integrity := backupIntegrity{Algorithm: "SHA-256+HMAC-SHA-256", Digests: digests, Signature: s.vault.MAC(digestPayload, "backup-integrity-v2")}
		integrityEntry, createErr := archive.Create("integrity.json")
		if createErr != nil {
			err = createErr
		} else {
			err = json.NewEncoder(integrityEntry).Encode(integrity)
		}
	}
	if closeErr := archive.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	checksum := hex.EncodeToString(counter.hash.Sum(nil))
	if err != nil {
		s.finishBackupJob(contextWithoutCancel(r.Context()), jobID, "failed", counter.count, checksum, map[string]any{"error": err.Error()})
		return
	}
	s.finishBackupJob(contextWithoutCancel(r.Context()), jobID, "completed", counter.count, checksum, map[string]any{"schemaVersion": schemaVersion, "tables": len(backupTables)})
	s.audit(r, &p.ID, p.Username, "Export", "backup", jobID.String(), "export", "success", map[string]any{"sizeBytes": counter.count, "checksum": checksum})
}

func (s *Server) validateBackup(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	archive, manifest, fileName, closeFile, err := s.openBackupUpload(w, r)
	if closeFile != nil {
		defer closeFile()
	}
	if err != nil {
		writeError(w, 400, "backup_invalid", err)
		return
	}
	jobID := uuid.New()
	details := map[string]any{"formatVersion": manifest.FormatVersion, "schemaVersion": manifest.SchemaVersion, "serviceVersion": manifest.ServiceVersion, "tables": len(archive.File)}
	_, _ = s.pool.Exec(r.Context(), `INSERT INTO backup_jobs(id,operation,file_name,status,details,created_by,completed_at) VALUES($1,'validate',$2,'completed',$3,$4,now())`, jobID, fileName, mustJSON(details), p.ID)
	writeJSON(w, 200, map[string]any{"valid": true, "manifest": manifest, "entries": len(archive.File), "jobId": jobID})
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.PrincipalFrom(r.Context())
	archive, manifest, fileName, closeFile, err := s.openBackupUpload(w, r)
	if closeFile != nil {
		defer closeFile()
	}
	if err != nil {
		writeError(w, 400, "backup_invalid", err)
		return
	}
	if r.FormValue("confirmation") != "RESTORE PLANEXUS" {
		writeError(w, 400, "restore_confirmation_required", errors.New("type RESTORE PLANEXUS to confirm destructive restore"))
		return
	}
	entries := map[string]*zip.File{}
	for _, entry := range archive.File {
		entries[entry.Name] = entry
	}
	tx, err := s.pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		writeError(w, 500, "restore_transaction_failed", err)
		return
	}
	defer tx.Rollback(r.Context())
	tableNames := make([]string, len(backupTables))
	for i, table := range backupTables {
		tableNames[i] = table.Name
	}
	_, err = tx.Exec(r.Context(), `TRUNCATE `+strings.Join(tableNames, ",")+` CASCADE`)
	if err != nil {
		writeError(w, 500, "restore_clear_failed", err)
		return
	}
	restored := 0
	for _, table := range backupTables {
		entry := entries[table.Name+".ndjson"]
		reader, openErr := entry.Open()
		if openErr != nil {
			writeError(w, 400, "backup_entry_failed", openErr)
			return
		}
		decoder := json.NewDecoder(io.LimitReader(reader, 5<<30))
		for {
			var record json.RawMessage
			decodeErr := decoder.Decode(&record)
			if errors.Is(decodeErr, io.EOF) {
				break
			}
			if decodeErr != nil {
				reader.Close()
				writeError(w, 400, "backup_record_invalid", decodeErr)
				return
			}
			if len(record) > 100<<20 {
				reader.Close()
				writeError(w, 400, "backup_record_too_large", nil)
				return
			}
			_, insertErr := tx.Exec(r.Context(), `INSERT INTO `+table.Name+` SELECT * FROM json_populate_record(NULL::`+table.Name+`,$1::json)`, record)
			if insertErr != nil {
				reader.Close()
				writeError(w, 409, "restore_record_failed", insertErr)
				return
			}
			restored++
		}
		reader.Close()
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "restore_commit_failed", err)
		return
	}
	jobID := uuid.New()
	_, _ = s.pool.Exec(contextWithoutCancel(r.Context()), `INSERT INTO backup_jobs(id,operation,file_name,status,details,created_by,completed_at) SELECT $1,'restore',$2,'completed',$3,CASE WHEN EXISTS(SELECT 1 FROM users WHERE id=$4) THEN $4 ELSE NULL END,now()`, jobID, fileName, mustJSON(map[string]any{"records": restored, "sourceVersion": manifest.ServiceVersion}), p.ID)
	_, _ = s.pool.Exec(contextWithoutCancel(r.Context()), `INSERT INTO audit_logs(id,actor_name,event_type,resource_type,resource_id,action,outcome,details) VALUES($1,'system','Configuration Change','backup',$2,'restore','success',$3)`, uuid.New(), jobID.String(), mustJSON(map[string]any{"records": restored, "sourceVersion": manifest.ServiceVersion}))
	writeJSON(w, 200, map[string]any{"status": "restored", "records": restored, "sourceVersion": manifest.ServiceVersion, "jobId": jobID, "loginRequired": true})
}

func (s *Server) openBackupUpload(w http.ResponseWriter, r *http.Request) (*zip.Reader, backupManifest, string, func(), error) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, backupManifest{}, "", nil, err
	}
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		cleanup()
		return nil, backupManifest{}, "", nil, err
	}
	closeFile := func() { _ = file.Close(); cleanup() }
	if header.Size <= 0 || header.Size > 1<<30 {
		return nil, backupManifest{}, safeFileName(header.Filename), closeFile, errors.New("backup must be between 1 byte and 1GB")
	}
	archive, err := zip.NewReader(file, header.Size)
	if err != nil {
		return nil, backupManifest{}, safeFileName(header.Filename), closeFile, err
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, entry := range archive.File {
		if entry.UncompressedSize64 > 5<<30 {
			return nil, backupManifest{}, safeFileName(header.Filename), closeFile, errors.New("backup entry is too large")
		}
		total += entry.UncompressedSize64
		if total > 10<<30 {
			return nil, backupManifest{}, safeFileName(header.Filename), closeFile, errors.New("backup expands beyond 10GB")
		}
		if entries[entry.Name] != nil {
			return nil, backupManifest{}, safeFileName(header.Filename), closeFile, errors.New("backup contains duplicate entries")
		}
		entries[entry.Name] = entry
	}
	manifestEntry := entries["manifest.json"]
	if manifestEntry == nil {
		return nil, backupManifest{}, safeFileName(header.Filename), closeFile, errors.New("manifest is missing")
	}
	reader, err := manifestEntry.Open()
	if err != nil {
		return nil, backupManifest{}, safeFileName(header.Filename), closeFile, err
	}
	var manifest backupManifest
	err = json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(&manifest)
	reader.Close()
	if err != nil {
		return nil, manifest, safeFileName(header.Filename), closeFile, err
	}
	if manifest.Format != "planexus-logical-backup" || manifest.FormatVersion != 2 {
		return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("unsupported Planexus backup format")
	}
	if manifest.KeyFingerprint != s.vault.Fingerprint() {
		return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("backup ENCRYPTION_KEY fingerprint does not match")
	}
	var currentSchema int
	if err = s.pool.QueryRow(r.Context(), `SELECT COALESCE(max(version),0) FROM schema_migrations`).Scan(&currentSchema); err != nil {
		return nil, manifest, safeFileName(header.Filename), closeFile, err
	}
	if manifest.SchemaVersion != currentSchema {
		return nil, manifest, safeFileName(header.Filename), closeFile, fmt.Errorf("backup schema version %d does not match current version %d", manifest.SchemaVersion, currentSchema)
	}
	integrityEntry := entries["integrity.json"]
	if integrityEntry == nil {
		return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("backup integrity record is missing")
	}
	integrityReader, integrityErr := integrityEntry.Open()
	if integrityErr != nil {
		return nil, manifest, safeFileName(header.Filename), closeFile, integrityErr
	}
	var integrity backupIntegrity
	integrityErr = json.NewDecoder(io.LimitReader(integrityReader, 1<<20)).Decode(&integrity)
	integrityReader.Close()
	if integrityErr != nil || integrity.Algorithm != "SHA-256+HMAC-SHA-256" {
		return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("backup integrity record is invalid")
	}
	digestPayload, _ := json.Marshal(integrity.Digests)
	if !s.vault.VerifyMAC(digestPayload, "backup-integrity-v2", integrity.Signature) {
		return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("backup integrity signature does not match")
	}
	manifestDigest, digestErr := checksumZipEntry(manifestEntry)
	if digestErr != nil || !strings.EqualFold(integrity.Digests["manifest.json"], manifestDigest) {
		return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("backup manifest checksum does not match")
	}
	if len(manifest.Tables) != len(backupTables) {
		return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("backup table manifest does not match current schema")
	}
	for tableIndex, table := range backupTables {
		if manifest.Tables[tableIndex] != table.Name {
			return nil, manifest, safeFileName(header.Filename), closeFile, errors.New("backup table manifest does not match current schema")
		}
		entryName := table.Name + ".ndjson"
		entry := entries[entryName]
		if entry == nil {
			return nil, manifest, safeFileName(header.Filename), closeFile, fmt.Errorf("backup table %s is missing", table.Name)
		}
		digest, digestErr := checksumZipEntry(entry)
		if digestErr != nil || !strings.EqualFold(integrity.Digests[entryName], digest) {
			return nil, manifest, safeFileName(header.Filename), closeFile, fmt.Errorf("backup table %s checksum does not match", table.Name)
		}
	}
	return archive, manifest, safeFileName(header.Filename), closeFile, nil
}

func checksumZipEntry(entry *zip.File) (string, error) {
	reader, err := entry.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, io.LimitReader(reader, int64(entry.UncompressedSize64)+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	rows, err := s.pool.Query(r.Context(), `SELECT id,operation,file_name,status,size_bytes,checksum,details,created_by,created_at,completed_at FROM backup_jobs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		writeError(w, 500, "backup_history_failed", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var operation, file, status, checksum string
		var size int64
		var details json.RawMessage
		var user *uuid.UUID
		var created time.Time
		var completed *time.Time
		if err = rows.Scan(&id, &operation, &file, &status, &size, &checksum, &details, &user, &created, &completed); err != nil {
			writeError(w, 500, "backup_history_failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "operation": operation, "fileName": file, "status": status, "sizeBytes": size, "checksum": checksum, "details": details, "createdBy": user, "createdAt": created, "completedAt": completed})
	}
	writeJSON(w, 200, items)
}
func (s *Server) finishBackupJob(ctx context.Context, id uuid.UUID, status string, size int64, checksum string, details any) {
	_, _ = s.pool.Exec(ctx, `UPDATE backup_jobs SET status=$1,size_bytes=$2,checksum=$3,details=$4,completed_at=now() WHERE id=$5`, status, size, checksum, mustJSON(details), id)
}
func mustJSON(value any) []byte                                { data, _ := json.Marshal(value); return data }
func contextWithoutCancel(ctx context.Context) context.Context { return context.WithoutCancel(ctx) }
