package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	stdlog "log"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"osreport/internal/app"
	"osreport/internal/domain"
	"osreport/internal/infra/config"
	osinfra "osreport/internal/infra/opensearch"
	"osreport/internal/reporting/excel"
)

//go:embed static/dashboard.html.tmpl
var dashboardTemplateFS embed.FS

var dashboardTemplate = template.Must(template.New("dashboard.html.tmpl").Funcs(template.FuncMap{
	"severityLabel": excel.SeverityLabel,
	"severityColor": func(s domain.Severity) string {
		color, ok := excel.SeverityColor(s)
		if !ok {
			return "eeeeee"
		}
		return color
	},
	"trendLabel": excel.TrendLabel,
}).ParseFS(dashboardTemplateFS, "static/dashboard.html.tmpl"))

// HTTP server timeouts. WriteTimeout is sized for the slowest route
// (/api/refresh, which runs the full pipeline synchronously) rather than
// the instant ones (/, /health, /api/top-alarms) — the stdlib server
// applies one WriteTimeout per connection, not per route.
const (
	httpReadHeaderTimeout = 5 * time.Second
	httpReadTimeout       = 15 * time.Second
	httpWriteTimeout      = 5 * time.Minute
	httpIdleTimeout       = 120 * time.Second
)

// dashboardServer holds everything a background refresh needs to re-run
// the same fetch -> classify -> correlate -> aggregate -> write pipeline
// the batch CLI uses, plus the cached result the HTTP handlers serve.
type dashboardServer struct {
	repo       domain.EventRepository
	writer     domain.ReportWriter
	index      string
	components []string
	severities []string
	windowDays int
	topN       int
	timeout    time.Duration
	outputPath string

	// fileMu guards outputPath itself, not just the in-memory cache: refresh
	// takes it exclusively (TryLock, so an overlapping tick/manual trigger
	// skips instead of racing another refresh's excelize SaveAs), and
	// handleExport takes it as a reader (RLock, so a concurrent refresh
	// can't rewrite the file out from under an in-flight download — a
	// plain per-refresh mutex caught refresh-vs-refresh but not
	// refresh-vs-export).
	fileMu sync.RWMutex

	mu          sync.RWMutex // guards the three fields below only
	cache       domain.ReportData
	lastUpdated time.Time
	lastError   string
}

// runServe implements `osreport serve`: a long-running process that keeps
// a Top N Alarmas dashboard current via a background refresh instead of
// requiring someone to run the batch CLI by hand. The top-level recover
// mirrors run()'s in main.go — this is meant to run unattended for a long
// time, so a panic during setup must still be logged to osreport.log
// rather than only flashing on a console nobody's watching.
func runServe(args []string) (err error) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var (
		port       = fs.String("port", "8080", "HTTP port to listen on")
		bindAll    = fs.Bool("bind-all", false, "listen on all network interfaces instead of only localhost — required before exposing this on a LAN (Fase 2); has no authentication of its own")
		index      = fs.String("index", "index-athonet", "OpenSearch index to query")
		component  = fs.String("component", "M3UA,TCAP,MAP,HSS_IMS,S6a,DIAM", "comma-separated ALERT_COMPONENT values to include (empty = any)")
		severities = fs.String("severity", "ERR,SYS,WRN", "comma-separated ALERT_SEVERITY values to include (empty = any)")
		windowDays = fs.Int("window-days", 7, "rolling window size in days: from = today - window-days, to = today")
		refresh    = fs.Duration("refresh-interval", 24*time.Hour, "how often to re-run the pipeline in the background")
		topN       = fs.Int("top", 10, "number of rows in the Top N Alarmas table")
		pageSize   = fs.Int("page-size", 0, "OpenSearch search_after page size (0 = default 1000, max 10000)")
		timeout    = fs.Duration("timeout", 0, "deadline for each background refresh (0 = use OS_TIMEOUT_SECONDS from env)")
		logFile    = fs.String("log-file", "osreport.log", "path to append run logs to, alongside stderr")
		output     = fs.String("output", "dashboard-informe.xlsx", "xlsx path the dashboard keeps refreshed for export (separate from the batch CLI's default)")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	// time.NewTicker panics on a non-positive duration — validate before
	// setupLogging even runs, so a bad flag fails with a clear message
	// instead of an unrecovered panic once runScheduler starts.
	if *refresh <= 0 {
		return fmt.Errorf("--refresh-interval must be positive, got %s", refresh)
	}
	if *windowDays <= 0 {
		return fmt.Errorf("--window-days must be positive, got %d", *windowDays)
	}
	if strings.TrimSpace(*output) == "" {
		return fmt.Errorf("--output must not be empty")
	}

	logWriter, closeLog := setupLogging(*logFile)
	defer closeLog()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered during serve startup", "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("internal error: %v", r)
		}
	}()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if *timeout > 0 {
		cfg.Timeout = *timeout
	}

	client, err := osinfra.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("opensearch client: %w", err)
	}

	srv := &dashboardServer{
		repo:       osinfra.NewRepository(client, *pageSize),
		writer:     excel.NewWriter(),
		index:      *index,
		components: splitAndTrim(*component),
		severities: splitAndTrim(*severities),
		windowDays: *windowDays,
		topN:       *topN,
		timeout:    cfg.Timeout,
		outputPath: *output,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.runScheduler(ctx, *refresh)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/", srv.handleDashboard)
	mux.HandleFunc("/api/top-alarms", srv.handleTopAlarmsJSON)
	mux.HandleFunc("/api/refresh", srv.handleManualRefresh)
	mux.HandleFunc("/export.xlsx", srv.handleExport)

	host := "127.0.0.1"
	if *bindAll {
		host = ""
	}
	addr := host + ":" + *port
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
		// net/http already recovers a panicking handler on its own (one
		// bad request doesn't take down the server), but by default it
		// reports that via the stdlib "log" package's default logger,
		// which writes straight to stderr — bypassing slog and never
		// reaching osreport.log. Point it at the same writer instead.
		ErrorLog: stdlog.New(logWriter, "", stdlog.LstdFlags),
	}

	slog.Info("dashboard listening", "addr", addr, "bind_all", *bindAll, "refresh_interval", refresh.String(),
		"index", srv.index, "components", srv.components, "severities", srv.severities,
		"window_days", srv.windowDays, "top", srv.topN, "output", srv.outputPath, "refresh_timeout", srv.timeout.String())
	if *bindAll {
		slog.Warn("listening on all interfaces with no authentication — anyone on this network can read cluster data and trigger refreshes")
	}
	return httpSrv.ListenAndServe()
}

// runScheduler refreshes immediately on startup (so the dashboard isn't
// empty while waiting for the first tick), then on every interval tick
// until ctx is canceled.
func (s *dashboardServer) runScheduler(ctx context.Context, interval time.Duration) {
	s.refreshRecovered(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshRecovered(ctx)
		}
	}
}

// refreshRecovered wraps refresh with a panic recovery. This runs in a
// long-lived background goroutine, separate from any single HTTP
// request — net/http's built-in per-request recovery doesn't apply here.
// An unrecovered panic in ANY goroutine (not just main) takes down the
// entire Go process, so without this, one bad refresh cycle would kill
// the whole dashboard rather than just fail that one cycle.
func (s *dashboardServer) refreshRecovered(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered during scheduled refresh", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	s.refresh(ctx)
}

// refresh re-runs the exact pipeline the batch CLI uses (including writing
// outputPath, so /export.xlsx and week-over-week trend both come for free)
// and swaps it into the cache. A failed refresh leaves the previous cache
// in place — a dashboard showing slightly stale data beats one that goes
// blank because OpenSearch hiccuped for a minute.
//
// fileMu.TryLock (exclusive) ensures only one refresh ever runs at a time
// and blocks out any in-flight /export.xlsx read too: without it, the
// scheduler tick and a manual /api/refresh could both call excelize
// SaveAs on outputPath concurrently (corrupting the file), or a refresh
// could rewrite outputPath while handleExport is mid-read (truncated
// download), and ReadPreviousCounts could read a half-written file and
// silently break the trend column.
func (s *dashboardServer) refresh(ctx context.Context) {
	if !s.fileMu.TryLock() {
		slog.Warn("output file busy (refresh or export in progress), skipping this trigger")
		return
	}
	defer s.fileMu.Unlock()

	// In-process fileMu only protects against this same serve instance
	// racing itself; the file lock additionally guards against an
	// external `osreport` batch run (a scheduled task, a manual
	// run-report.bat) pointed at the same --output path at the same time.
	releaseLock, err := excel.AcquireFileLock(s.outputPath)
	if err != nil {
		slog.Warn("skipping refresh, output file locked by another process", "error", err)
		return
	}
	defer releaseLock()

	start := time.Now()
	from := start.AddDate(0, 0, -s.windowDays)

	prevCounts, prevExists, err := excel.ReadPreviousCounts(s.outputPath)
	if err != nil {
		slog.Error("dashboard refresh: read previous report", "error", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	params := app.Params{
		From:             from,
		To:               start,
		Index:            s.index,
		Components:       s.components,
		Severities:       s.severities,
		OutputPath:       s.outputPath,
		TopN:             s.topN,
		PrevCounts:       prevCounts,
		PrevReportExists: prevExists,
	}

	data, err := app.GenerateReport(runCtx, s.repo, s.writer, params)

	s.mu.Lock()
	if err != nil {
		s.lastError = err.Error()
		s.mu.Unlock()
		slog.Error("dashboard refresh failed", "error", err,
			"index", s.index, "components", s.components, "from", from, "to", start,
			"duration_ms", time.Since(start).Milliseconds())
		return
	}
	s.cache = data
	s.lastUpdated = time.Now()
	s.lastError = ""
	s.mu.Unlock()

	// Anomaly logging does its own (fast) I/O and doesn't touch the cache —
	// no reason to hold s.mu while it runs and block every HTTP handler.
	slog.Info("refresh completed", "total_events", data.TotalEvents, "skipped_docs", data.SkippedDocs,
		"top_alarms", len(data.TopAlarms), "duration_ms", time.Since(start).Milliseconds())
	logReportAnomalies(data)
}

func (s *dashboardServer) snapshot() (data domain.ReportData, lastUpdated time.Time, lastError string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cache, s.lastUpdated, s.lastError
}

func (s *dashboardServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type dashboardViewModel struct {
	Data        domain.ReportData
	LastUpdated string
	LastError   string
	HasData     bool
	ExportURL   string // absolute URL of /export.xlsx, for pointing PowerBI's Web.Contents directly at it
}

func (s *dashboardServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data, lastUpdated, lastError := s.snapshot()
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	view := dashboardViewModel{
		Data:      data,
		LastError: lastError,
		HasData:   !lastUpdated.IsZero(),
		ExportURL: scheme + "://" + r.Host + "/export.xlsx",
	}
	if view.HasData {
		view.LastUpdated = lastUpdated.Format("2006-01-02 15:04:05")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := dashboardTemplate.Execute(w, view); err != nil {
		slog.Error("render dashboard template", "error", err)
		http.Error(w, "error rendering dashboard", http.StatusInternalServerError)
	}
}

func (s *dashboardServer) handleTopAlarmsJSON(w http.ResponseWriter, r *http.Request) {
	data, lastUpdated, lastError := s.snapshot()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"data":         data,
		"last_updated": lastUpdated,
		"last_error":   lastError,
	}); err != nil {
		slog.Error("encode /api/top-alarms response", "error", err)
	}
}

// handleManualRefresh triggers an immediate refresh synchronously (so the
// redirect back to "/" already reflects it) — the dashboard's "Actualizar
// ahora" button, for whenever someone doesn't want to wait for the next
// scheduled tick. refresh() itself is the concurrency guard: if a
// scheduled tick is already running, this just skips instead of racing it.
func (s *dashboardServer) handleManualRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.refreshRecovered(r.Context())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleExport serves the same .xlsx file the last refresh wrote via
// excel.Writer — no re-generation here, just the file already on disk.
// fileMu.RLock blocks only against an in-flight refresh's write (see the
// comment on fileMu); concurrent exports don't block each other.
func (s *dashboardServer) handleExport(w http.ResponseWriter, r *http.Request) {
	data, lastUpdated, _ := s.snapshot()
	if lastUpdated.IsZero() {
		http.Error(w, "no hay reporte generado todavia", http.StatusServiceUnavailable)
		return
	}

	s.fileMu.RLock()
	defer s.fileMu.RUnlock()

	filename := fmt.Sprintf("Top10-Errores-%s_%s.xlsx", data.From.Format("20060102"), data.To.Format("20060102"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, s.outputPath)
}

// statusRecorder captures the status code a handler writes — plain
// http.ResponseWriter doesn't expose it, and loggingMiddleware needs it
// after the handler has already run.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// loggingMiddleware logs one line per request (method, path, status,
// duration, remote address) — on an unattended process nobody is
// tailing live, this is what turns "the dashboard felt slow yesterday"
// into something actually answerable from osreport.log.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration_ms", time.Since(start).Milliseconds(), "remote", r.RemoteAddr)
	})
}
