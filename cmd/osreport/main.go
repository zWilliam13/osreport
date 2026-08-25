package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"osreport/internal/app"
	"osreport/internal/domain"
	"osreport/internal/infra/config"
	osinfra "osreport/internal/infra/opensearch"
	"osreport/internal/reporting/excel"
)

// Log rotation limits: a weekly cron writes a handful of KB per run, but
// nothing should be allowed to grow forever on an unattended schedule.
const (
	logMaxSizeMB  = 10 // rotate once the active file hits this size
	logMaxBackups = 5  // keep this many rotated files before deleting the oldest
	logMaxAgeDays = 90 // also delete rotated files older than this, regardless of count
)

// skippedDocsWarnThreshold flags a run where an unusually large share of
// matched hits couldn't be mapped into an Event — a signal the mapper needs
// attention (schema drift, new document shape), not just normal noise.
const skippedDocsWarnThreshold = 0.05

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		err = runServe(os.Args[2:])
	} else {
		err = run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "osreport:", err)
		os.Exit(1)
	}
}

// run executes the batch CLI. The top-level recover is a safety net for a
// long-lived, unattended scheduled task: without it, a bug that panics
// anywhere in the call chain prints only to raw stderr (bypassing slog
// entirely — see setupLogging) and vanishes without a trace once that
// console window is gone. With it, the panic and its stack trace land in
// osreport.log like any other failure.
func run() (err error) {
	var (
		from       = flag.String("from", "", "start of range, RFC3339 or YYYY-MM-DD (required)")
		to         = flag.String("to", "", "end of range, RFC3339 or YYYY-MM-DD (required)")
		index      = flag.String("index", "index-athonet", "OpenSearch index to query")
		component  = flag.String("component", "TCAP,MAP,HSS_IMS,S6a,DIAM", "comma-separated ALERT_COMPONENT values to include (empty = any)")
		severities = flag.String("severity", "ERR,SYS,WRN", "comma-separated ALERT_SEVERITY values to include (empty = any)")
		output     = flag.String("output", "informe.xlsx", "output .xlsx path")
		timeout    = flag.Duration("timeout", 0, "overall deadline for the run, e.g. 5m (0 = use OS_TIMEOUT_SECONDS from env)")
		pageSize   = flag.Int("page-size", 0, "OpenSearch search_after page size (0 = default 1000)")
		topN       = flag.Int("top", 10, "number of rows in the Top N Alarmas table")
		logFile    = flag.String("log-file", "osreport.log", "path to append run logs to, alongside stderr")
	)
	flag.Parse()

	_, closeLog := setupLogging(*logFile)
	defer closeLog()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered in batch run", "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("internal error: %v", r)
		}
	}()

	if *from == "" || *to == "" {
		flag.Usage()
		return fmt.Errorf("--from and --to are required")
	}
	if strings.TrimSpace(*output) == "" {
		return fmt.Errorf("--output must not be empty")
	}

	fromTime, err := parseFlexibleTime(*from, false)
	if err != nil {
		return fmt.Errorf("--from: %w", err)
	}
	toTime, err := parseFlexibleTime(*to, true)
	if err != nil {
		return fmt.Errorf("--to: %w", err)
	}
	if !toTime.After(fromTime) {
		return fmt.Errorf("--to (%s) must be after --from (%s)", toTime, fromTime)
	}

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
	repo := osinfra.NewRepository(client, *pageSize)
	writer := excel.NewWriter()

	// Cross-process guard: a scheduled run, a manual double-click of
	// run-report.bat, and `osreport serve` can all target the same
	// --output path. Without this, two processes could both call
	// excelize SaveAs on it at once and corrupt the file.
	releaseLock, err := excel.AcquireFileLock(*output)
	if err != nil {
		return err
	}
	defer releaseLock()

	// Read the previous run's report (if any) before it gets overwritten,
	// so this run can show a week-over-week trend per alarm.
	prevCounts, prevExists, err := excel.ReadPreviousCounts(*output)
	if err != nil {
		return fmt.Errorf("read previous report: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	components := splitAndTrim(*component)
	params := app.Params{
		From:             fromTime,
		To:               toTime,
		Index:            *index,
		Components:       components,
		Severities:       splitAndTrim(*severities),
		OutputPath:       *output,
		TopN:             *topN,
		PrevCounts:       prevCounts,
		PrevReportExists: prevExists,
	}

	start := time.Now()
	data, err := app.GenerateReport(ctx, repo, writer, params)
	if err != nil {
		slog.Error("report generation failed", "error", err,
			"index", *index, "components", components, "from", fromTime, "to", toTime,
			"duration_ms", time.Since(start).Milliseconds())
		return err
	}

	slog.Info("run completed", "total_events", data.TotalEvents, "skipped_docs", data.SkippedDocs,
		"top_alarms", len(data.TopAlarms), "duration_ms", time.Since(start).Milliseconds())
	logReportAnomalies(data)

	fmt.Printf("informe generado: %s\n", *output)
	return nil
}

// setupLogging routes slog output to both stderr and a self-rotating log
// file at path, so a scheduled/double-clicked run that nobody is watching
// still leaves a durable trail without growing forever — lumberjack
// rotates path once it passes logMaxSizeMB, gzips the rotated copy, and
// prunes old ones past logMaxBackups/logMaxAgeDays. It also returns the
// underlying writer so callers can point other loggers (e.g.
// http.Server.ErrorLog, which net/http uses to report recovered handler
// panics) at the same destination instead of the stdlib "log" package's
// default logger, which would otherwise write straight to stderr and
// never reach the file.
func setupLogging(path string) (w io.Writer, close func()) {
	rotator := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    logMaxSizeMB,
		MaxBackups: logMaxBackups,
		MaxAge:     logMaxAgeDays,
		Compress:   true,
	}
	w = io.MultiWriter(os.Stderr, rotator)
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))
	return w, func() { _ = rotator.Close() }
}

// logReportAnomalies flags conditions that indicate the run itself is
// broken rather than just reporting a quiet week — these would otherwise
// go unnoticed until someone happens to open the .xlsx and wonder why it
// looks empty or thin.
func logReportAnomalies(data domain.ReportData) {
	if data.TotalEvents == 0 {
		slog.Warn("zero events matched — check OpenSearch connectivity, filters, or date range",
			"from", data.From, "to", data.To)
		return
	}

	total := data.TotalEvents + data.SkippedDocs
	if rate := float64(data.SkippedDocs) / float64(total); rate > skippedDocsWarnThreshold {
		slog.Warn("high skipped-document rate — mapper may need attention (schema drift, new document shape)",
			"skipped", data.SkippedDocs, "total_matched", total, "rate", fmt.Sprintf("%.1f%%", rate*100))
	}
}

func parseFlexibleTime(s string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		if endOfDay {
			return t.Add(24*time.Hour - time.Second), nil
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q, expected RFC3339 or YYYY-MM-DD", s)
}

func splitAndTrim(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
