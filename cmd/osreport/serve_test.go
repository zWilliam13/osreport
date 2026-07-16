package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"osreport/internal/domain"
	"osreport/internal/reporting/excel"
)

// TestRunServe_RejectsNonPositiveRefreshInterval guards against a real
// crash: time.NewTicker panics on a non-positive duration, so
// --refresh-interval 0 (or negative) must fail fast here, before
// runScheduler ever reaches NewTicker. This must return before touching
// config.Load()/real credentials, so it's safe to run without a live
// OpenSearch cluster.
func TestRunServe_RejectsNonPositiveRefreshInterval(t *testing.T) {
	for _, v := range []string{"0s", "-5s"} {
		t.Run(v, func(t *testing.T) {
			err := runServe([]string{"--refresh-interval", v})
			if err == nil {
				t.Fatalf("runServe with --refresh-interval %s: expected an error, got nil", v)
			}
			if !strings.Contains(err.Error(), "refresh-interval") {
				t.Errorf("error = %q, want it to mention refresh-interval", err.Error())
			}
		})
	}
}

func TestRunServe_RejectsNonPositiveWindowDays(t *testing.T) {
	for _, v := range []string{"0", "-3"} {
		t.Run(v, func(t *testing.T) {
			err := runServe([]string{"--window-days", v})
			if err == nil {
				t.Fatalf("runServe with --window-days %s: expected an error, got nil", v)
			}
			if !strings.Contains(err.Error(), "window-days") {
				t.Errorf("error = %q, want it to mention window-days", err.Error())
			}
		})
	}
}

// TestDashboardServer_ManyRefreshCyclesNoGoroutineLeak is a compressed
// stand-in for "let it run for 24h+": we can't wait real hours here, so
// instead this runs many refresh cycles back-to-back (fakeServeRepo, no
// real cluster calls, so it's fast) and checks the goroutine count
// doesn't creep up — the signal a real long-run leak (a context never
// canceled, a ticker never stopped, a lock never released) would leave.
// It does NOT replace watching a real deployment over days, but it does
// catch the class of bug that class of test would eventually surface.
func TestDashboardServer_ManyRefreshCyclesNoGoroutineLeak(t *testing.T) {
	ts := time.Now()
	events := []domain.Event{
		{ID: "1", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "ASP_DOWN", RawSeverity: "SYS", Message: "down"},
		{ID: "2", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "CONN_TIMEOUT", RawSeverity: "ERR", Message: "timeout"},
	}
	srv := newTestServer(t, events)

	srv.refresh(context.Background()) // warm up
	runtime.GC()
	before := runtime.NumGoroutine()

	const cycles = 200
	for i := 0; i < cycles; i++ {
		srv.refresh(context.Background())

		req := httptest.NewRequest(http.MethodGet, "/api/top-alarms", nil)
		rec := httptest.NewRecorder()
		srv.handleTopAlarmsJSON(rec, req)

		req2 := httptest.NewRequest(http.MethodGet, "/", nil)
		rec2 := httptest.NewRecorder()
		srv.handleDashboard(rec2, req2)
	}

	runtime.GC()
	after := runtime.NumGoroutine()

	// small slack for GC/runtime housekeeping goroutines that come and go
	// on their own — this isn't about an exact count, it's about ruling
	// out unbounded growth proportional to the number of cycles.
	if after > before+5 {
		t.Errorf("possible goroutine leak after %d refresh cycles: before=%d after=%d", cycles, before, after)
	}
}

type fakeServeRepo struct {
	events []domain.Event
}

func (f fakeServeRepo) Search(ctx context.Context, c domain.Criteria) ([]domain.Event, int, error) {
	return f.events, 0, nil
}

// panickyRepo simulates an unexpected bug deep in the pipeline (a nil
// dereference, an out-of-bounds index — anything) to prove
// refreshRecovered actually survives it.
type panickyRepo struct{}

func (panickyRepo) Search(ctx context.Context, c domain.Criteria) ([]domain.Event, int, error) {
	panic("simulated unexpected failure")
}

// TestDashboardServer_RefreshRecoveredSurvivesPanic is the direct proof
// for the most serious bug found this round: an unrecovered panic in ANY
// goroutine kills the entire Go process, and refresh() runs in a
// long-lived background goroutine separate from main. Without
// refreshRecovered's defer/recover, this test's call would crash the
// whole test binary instead of returning.
func TestDashboardServer_RefreshRecoveredSurvivesPanic(t *testing.T) {
	srv := newTestServer(t, nil)
	srv.repo = panickyRepo{}

	done := make(chan struct{})
	go func() {
		srv.refreshRecovered(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// good: the panic was recovered, refreshRecovered returned normally
	case <-time.After(5 * time.Second):
		t.Fatal("refreshRecovered did not return — panic was not recovered")
	}

	// the server must still be fully usable afterward (this is the whole
	// point: one bad refresh doesn't take the dashboard down)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.handleHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("server unusable after recovering a panic: /health status = %d", rec.Code)
	}
}

func newTestServer(t *testing.T, events []domain.Event) *dashboardServer {
	t.Helper()
	return &dashboardServer{
		repo:       fakeServeRepo{events: events},
		writer:     excel.NewWriter(),
		index:      "index-athonet",
		components: []string{"M3UA"},
		severities: []string{"ERR", "SYS", "WRN"},
		windowDays: 7,
		topN:       10,
		timeout:    30 * time.Second,
		outputPath: filepath.Join(t.TempDir(), "dashboard-informe.xlsx"),
	}
}

// TestDashboardServer_RefreshSkipsWhenExternalLockHeld simulates a batch
// CLI run (or another process entirely) holding the cross-process file
// lock on the same outputPath — refresh() must skip gracefully (no
// panic, no corrupted state) rather than race that external writer.
func TestDashboardServer_RefreshSkipsWhenExternalLockHeld(t *testing.T) {
	srv := newTestServer(t, []domain.Event{
		{ID: "1", Host: "h1", Component: "M3UA", Timestamp: time.Now(), EventType: "ASP_DOWN", RawSeverity: "SYS", Message: "down"},
	})

	release, err := excel.AcquireFileLock(srv.outputPath)
	if err != nil {
		t.Fatalf("setup: AcquireFileLock() error = %v", err)
	}
	defer release()

	srv.refresh(context.Background())

	if _, lastUpdated, _ := srv.snapshot(); !lastUpdated.IsZero() {
		t.Error("cache was populated even though the output file was externally locked")
	}
}

// TestDashboardServer_DashboardWithZeroAlarmsShowsEmptyState covers a
// refresh that succeeds but matches zero events (HasData becomes true —
// a real report was generated — but TopAlarms is empty). Without an
// explicit empty-state branch, the template would render a header-only
// table that looks broken rather than clearly saying "no alarms."
func TestDashboardServer_DashboardWithZeroAlarmsShowsEmptyState(t *testing.T) {
	srv := newTestServer(t, nil) // no events at all
	srv.refresh(context.Background())

	if _, lastUpdated, _ := srv.snapshot(); lastUpdated.IsZero() {
		t.Fatal("setup: refresh did not populate the cache")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.handleDashboard(rec, req)

	if !strings.Contains(rec.Body.String(), "Sin alarmas en este periodo") {
		t.Errorf("expected the empty-state message, got: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "<table>") {
		t.Error("should not render a (header-only) table when there are zero alarms")
	}
}

func TestDashboardServer_Health(t *testing.T) {
	srv := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestDashboardServer_DashboardBeforeFirstRefresh(t *testing.T) {
	srv := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Todavia no hay") {
		t.Errorf("body missing the no-data message: %s", rec.Body.String())
	}
}

func TestDashboardServer_ExportBeforeFirstRefresh(t *testing.T) {
	srv := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/export.xlsx", nil)
	rec := httptest.NewRecorder()

	srv.handleExport(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestDashboardServer_RefreshThenServeData(t *testing.T) {
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	srv := newTestServer(t, []domain.Event{
		{ID: "1", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "CONN_TIMEOUT", RawSeverity: "ERR", Message: "timeout"},
		{ID: "2", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "ASP_DOWN", RawSeverity: "SYS", Message: "down"},
	})

	srv.refresh(context.Background())

	data, lastUpdated, lastError := srv.snapshot()
	if lastError != "" {
		t.Fatalf("lastError = %q, want empty", lastError)
	}
	if lastUpdated.IsZero() {
		t.Fatal("lastUpdated is zero after refresh")
	}
	if len(data.TopAlarms) != 1 {
		t.Fatalf("TopAlarms = %d, want 1", len(data.TopAlarms))
	}

	// dashboard HTML now shows the row
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.handleDashboard(rec, req)
	if !strings.Contains(rec.Body.String(), "Enlace SS7") {
		t.Errorf("dashboard HTML missing expected alarm: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Web.Contents(\""+"http://"+req.Host+"/export.xlsx"+"\")") {
		t.Errorf("dashboard HTML missing the PowerBI Web.Contents hint pointing at this host's /export.xlsx: %s", rec.Body.String())
	}

	// JSON API reflects the same data
	req2 := httptest.NewRequest(http.MethodGet, "/api/top-alarms", nil)
	rec2 := httptest.NewRecorder()
	srv.handleTopAlarmsJSON(rec2, req2)
	var body map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := body["data"]; !ok {
		t.Errorf("JSON missing \"data\" key: %s", rec2.Body.String())
	}

	// export now serves the real file the refresh wrote
	req3 := httptest.NewRequest(http.MethodGet, "/export.xlsx", nil)
	rec3 := httptest.NewRecorder()
	srv.handleExport(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("export status = %d, want 200", rec3.Code)
	}
	if rec3.Body.Len() == 0 {
		t.Error("export body is empty")
	}
}

// blockingWriter delegates to a real domain.ReportWriter but lets a test
// pause it mid-write, so a test can deterministically create the overlap
// window between an in-flight refresh and a concurrent export.
type blockingWriter struct {
	real    domain.ReportWriter
	started chan struct{}
	release chan struct{}
}

func (w *blockingWriter) Write(ctx context.Context, data domain.ReportData, outputPath string) error {
	close(w.started)
	<-w.release
	return w.real.Write(ctx, data, outputPath)
}

// TestDashboardServer_ExportBlocksWhileRefreshWriting proves fileMu
// actually coordinates refresh vs. export, not just refresh vs. refresh:
// without RLock in handleExport, this test would see the export return
// immediately (racing the write) instead of waiting for it to finish.
func TestDashboardServer_ExportBlocksWhileRefreshWriting(t *testing.T) {
	ts := time.Now()
	events := []domain.Event{
		{ID: "1", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "ASP_DOWN", RawSeverity: "SYS", Message: "down"},
	}
	srv := newTestServer(t, events)

	// Establish a real first report on disk so handleExport gets past its
	// "no data yet" check and actually reaches the fileMu.RLock line.
	srv.refresh(context.Background())
	if _, lastUpdated, _ := srv.snapshot(); lastUpdated.IsZero() {
		t.Fatal("setup: first refresh did not populate the cache")
	}

	bw := &blockingWriter{real: excel.NewWriter(), started: make(chan struct{}), release: make(chan struct{})}
	srv.writer = bw

	refreshDone := make(chan struct{})
	go func() {
		srv.refresh(context.Background())
		close(refreshDone)
	}()
	<-bw.started // refresh now holds fileMu exclusively, blocked inside Write

	exportDone := make(chan struct{})
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/export.xlsx", nil)
		rec := httptest.NewRecorder()
		srv.handleExport(rec, req)
		close(exportDone)
	}()

	select {
	case <-exportDone:
		t.Fatal("handleExport returned while a refresh was still writing outputPath — fileMu.RLock did not block")
	case <-time.After(150 * time.Millisecond):
		// expected: export is still waiting on fileMu.RLock
	}

	close(bw.release)
	<-refreshDone

	select {
	case <-exportDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handleExport did not complete after the refresh released fileMu")
	}
}

func TestDashboardServer_ManualRefreshRedirects(t *testing.T) {
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	srv := newTestServer(t, []domain.Event{
		{ID: "1", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "ASP_DOWN", RawSeverity: "SYS", Message: "down"},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
	rec := httptest.NewRecorder()
	srv.handleManualRefresh(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rec.Code)
	}
	if _, _, lastError := srv.snapshot(); lastError != "" {
		t.Errorf("lastError = %q, want empty after manual refresh", lastError)
	}
}
