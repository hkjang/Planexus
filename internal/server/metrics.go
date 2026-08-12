package server

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) observeHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		pattern := chi.RouteContext(r.Context()).RoutePattern()
		if pattern == "" {
			pattern = "unmatched"
		}
		status := wrapped.Status()
		if status == 0 {
			status = 200
		}
		key := metricKey{Method: r.Method, Path: pattern, Status: strconv.Itoa(status)}
		s.metricsMu.Lock()
		metric := s.metrics[key]
		if metric == nil {
			metric = &metricValue{}
			s.metrics[key] = metric
		}
		metric.Count++
		metric.DurationSeconds += time.Since(started).Seconds()
		s.metricsMu.Unlock()
	})
}

func (s *Server) prometheusMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.metricsMu.Lock()
	keys := make([]metricKey, 0, len(s.metrics))
	snapshot := map[metricKey]metricValue{}
	for key, value := range s.metrics {
		keys = append(keys, key)
		snapshot[key] = *value
	}
	s.metricsMu.Unlock()
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Method+keys[i].Path+keys[i].Status < keys[j].Method+keys[j].Path+keys[j].Status
	})
	fmt.Fprintln(w, "# HELP planexus_http_requests_total HTTP requests handled by Planexus.")
	fmt.Fprintln(w, "# TYPE planexus_http_requests_total counter")
	for _, key := range keys {
		value := snapshot[key]
		fmt.Fprintf(w, "planexus_http_requests_total{method=%q,path=%q,status=%q} %d\n", key.Method, key.Path, key.Status, value.Count)
	}
	fmt.Fprintln(w, "# HELP planexus_http_request_duration_seconds_sum Cumulative HTTP request duration.")
	fmt.Fprintln(w, "# TYPE planexus_http_request_duration_seconds_sum counter")
	for _, key := range keys {
		value := snapshot[key]
		fmt.Fprintf(w, "planexus_http_request_duration_seconds_sum{method=%q,path=%q,status=%q} %.6f\n", key.Method, key.Path, key.Status, value.DurationSeconds)
	}
	stats := s.pool.Stat()
	fmt.Fprintf(w, "planexus_db_connections{state=\"acquired\"} %d\n", stats.AcquiredConns())
	fmt.Fprintf(w, "planexus_db_connections{state=\"idle\"} %d\n", stats.IdleConns())
	fmt.Fprintf(w, "planexus_db_connections{state=\"total\"} %d\n", stats.TotalConns())
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	fmt.Fprintf(w, "planexus_go_heap_bytes %d\n", memory.HeapAlloc)
	fmt.Fprintf(w, "planexus_build_info{version=%q} 1\n", s.version)
}

func (s *Server) systemHealth(w http.ResponseWriter, r *http.Request) {
	status := "healthy"
	dbStatus := "up"
	if err := s.pool.Ping(r.Context()); err != nil {
		status = "degraded"
		dbStatus = "down"
	}
	stats := s.pool.Stat()
	var migration int
	_ = s.pool.QueryRow(r.Context(), `SELECT COALESCE(max(version),0) FROM schema_migrations`).Scan(&migration)
	ai, _ := s.loadAISettings(r.Context())
	writeJSON(w, 200, map[string]any{"status": status, "version": s.version, "database": map[string]any{"status": dbStatus, "totalConnections": stats.TotalConns(), "acquiredConnections": stats.AcquiredConns(), "idleConnections": stats.IdleConns(), "migrationVersion": migration}, "aiGateway": map[string]any{"enabled": ai.Enabled, "models": len(ai.Models)}, "runtime": map[string]any{"goVersion": runtime.Version(), "goroutines": runtime.NumGoroutine()}, "checkedAt": time.Now()})
}
