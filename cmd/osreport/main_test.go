package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"osreport/internal/domain"
)

// TestLogRotation_IntegrationTriggersRealRotation proves the rotation
// mechanics actually work when wired up the same way setupLogging does it
// (io.Writer -> lumberjack.Logger), without waiting for real production
// thresholds (10MB / hours of runtime) — this uses a 1MB MaxSize and
// writes past it directly. It's a stand-in for "let it run until it
// actually rotates in production," which isn't practical to wait for
// here.
func TestLogRotation_IntegrationTriggersRealRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	rotator := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    1, // MB — lumberjack's smallest unit
		MaxBackups: 2,
		Compress:   true, // same as production setupLogging
	}
	defer rotator.Close()

	line := []byte(strings.Repeat("x", 1024) + "\n") // 1KB/line
	for i := 0; i < 1200; i++ {                       // ~1.2MB, past the 1MB threshold
		if _, err := rotator.Write(line); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := rotator.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	rotatedFound := false
	for _, e := range entries {
		if e.Name() != "test.log" {
			rotatedFound = true
			t.Logf("rotated backup found: %s", e.Name())
		}
	}
	if !rotatedFound {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("no rotated backup file found after writing past MaxSize; dir contains: %v", names)
	}
}

func TestParseFlexibleTime_RFC3339(t *testing.T) {
	got, err := parseFlexibleTime("2026-07-14T18:04:17Z", false)
	if err != nil {
		t.Fatalf("parseFlexibleTime() error = %v", err)
	}
	want := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseFlexibleTime_DateOnly_StartOfDay(t *testing.T) {
	got, err := parseFlexibleTime("2026-07-14", false)
	if err != nil {
		t.Fatalf("parseFlexibleTime() error = %v", err)
	}
	want := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseFlexibleTime_DateOnly_EndOfDay(t *testing.T) {
	got, err := parseFlexibleTime("2026-07-14", true)
	if err != nil {
		t.Fatalf("parseFlexibleTime() error = %v", err)
	}
	want := time.Date(2026, 7, 14, 23, 59, 59, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseFlexibleTime_Invalid(t *testing.T) {
	if _, err := parseFlexibleTime("not-a-date", false); err == nil {
		t.Error("expected error for invalid input, got nil")
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"basic", "ERR,SYS,WRN", []string{"ERR", "SYS", "WRN"}},
		{"spaces", " ERR , SYS ,WRN ", []string{"ERR", "SYS", "WRN"}},
		{"empty entries dropped", "ERR,,SYS,", []string{"ERR", "SYS"}},
		{"empty string", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAndTrim(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitAndTrim(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitAndTrim(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// withCapturedLog redirects slog's default logger to a buffer for the
// duration of fn, restoring the previous default afterward.
func withCapturedLog(fn func()) string {
	prev := slog.Default()
	defer slog.SetDefault(prev)

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	fn()
	return buf.String()
}

func TestLogReportAnomalies_ZeroEvents(t *testing.T) {
	out := withCapturedLog(func() {
		logReportAnomalies(domain.ReportData{TotalEvents: 0})
	})
	if !strings.Contains(out, "zero events matched") {
		t.Errorf("log output = %q, want a zero-events warning", out)
	}
}

func TestLogReportAnomalies_HighSkipRate(t *testing.T) {
	out := withCapturedLog(func() {
		logReportAnomalies(domain.ReportData{TotalEvents: 90, SkippedDocs: 10}) // 10% > 5% threshold
	})
	if !strings.Contains(out, "high skipped-document rate") {
		t.Errorf("log output = %q, want a high-skip-rate warning", out)
	}
}

func TestLogReportAnomalies_NormalRun(t *testing.T) {
	out := withCapturedLog(func() {
		logReportAnomalies(domain.ReportData{TotalEvents: 1000, SkippedDocs: 1}) // 0.1%, well under threshold
	})
	if out != "" {
		t.Errorf("log output = %q, want no warnings for a normal run", out)
	}
}

func TestSetupLogging_AppendsAcrossCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "osreport.log")
	prev := slog.Default()
	defer slog.SetDefault(prev)

	_, closeLog := setupLogging(path)
	slog.Info("first run marker")
	closeLog()

	_, closeLog2 := setupLogging(path)
	slog.Info("second run marker")
	closeLog2()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "first run marker") || !strings.Contains(string(content), "second run marker") {
		t.Errorf("log file missing entries from one of the two runs:\n%s", content)
	}
}
