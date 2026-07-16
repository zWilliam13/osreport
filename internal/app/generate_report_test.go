package app

import (
	"context"
	"testing"
	"time"

	"osreport/internal/domain"
)

type fakeRepo struct {
	events  []domain.Event
	skipped int
}

func (f fakeRepo) Search(ctx context.Context, c domain.Criteria) ([]domain.Event, int, error) {
	return f.events, f.skipped, nil
}

type fakeWriter struct{}

func (f *fakeWriter) Write(ctx context.Context, data domain.ReportData, outputPath string) error {
	return nil
}

func TestGenerateReport_ClassifiesCorrelatesAndAggregates(t *testing.T) {
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	repo := fakeRepo{
		events: []domain.Event{
			{ID: "1", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "CONN_TIMEOUT", RawSeverity: "ERR", Message: "timeout"},
			{ID: "2", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "ASP_DOWN", RawSeverity: "SYS", Message: "down"},
		},
		skipped: 3,
	}
	writer := &fakeWriter{}

	data, err := GenerateReport(context.Background(), repo, writer, Params{
		From: ts.Add(-time.Hour), To: ts.Add(time.Hour), Index: "index-athonet", OutputPath: "unused.xlsx",
	})
	if err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}

	if data.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2", data.TotalEvents)
	}
	if data.SkippedDocs != 3 {
		t.Errorf("SkippedDocs = %d, want 3", data.SkippedDocs)
	}
	if len(data.TopAlarms) != 1 {
		t.Fatalf("TopAlarms = %d, want 1", len(data.TopAlarms))
	}
	row := data.TopAlarms[0]
	if row.Severity != domain.SeverityCritical {
		t.Errorf("TopAlarms[0].Severity = %v, want Critical", row.Severity)
	}
	if row.Component != "M3UA" {
		t.Errorf("TopAlarms[0].Component = %q, want M3UA", row.Component)
	}
	if row.Alarma != "Enlace SS7 (M3UA/SIGTRAN) inestable" {
		t.Errorf("TopAlarms[0].Alarma = %q, want the M3UA root cause", row.Alarma)
	}
	if row.Key != "m3ua-asp-down-with-conn-timeout" {
		t.Errorf("TopAlarms[0].Key = %q, want the stable CorrelationRule.Name", row.Key)
	}
	if row.Count != 1 {
		t.Errorf("TopAlarms[0].Count = %d, want 1", row.Count)
	}
	if row.Rank != 1 {
		t.Errorf("TopAlarms[0].Rank = %d, want 1", row.Rank)
	}
	if data.HasPreviousReport {
		t.Error("HasPreviousReport = true, want false when Params.PrevReportExists is unset")
	}
	if row.FoundInPrevious {
		t.Error("FoundInPrevious = true, want false with no PrevCounts given")
	}
}

func TestGenerateReport_TrendAgainstPreviousReport(t *testing.T) {
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	repo := fakeRepo{events: []domain.Event{
		{ID: "1", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "CONN_TIMEOUT", RawSeverity: "ERR", Message: "timeout"},
		{ID: "2", Host: "h1", Component: "M3UA", Timestamp: ts, EventType: "ASP_DOWN", RawSeverity: "SYS", Message: "down"},
	}}
	writer := &fakeWriter{}

	data, err := GenerateReport(context.Background(), repo, writer, Params{
		From: ts.Add(-time.Hour), To: ts.Add(time.Hour), Index: "index-athonet", OutputPath: "unused.xlsx",
		PrevCounts:       map[string]int{"m3ua-asp-down-with-conn-timeout": 4},
		PrevReportExists: true,
	})
	if err != nil {
		t.Fatalf("GenerateReport() error = %v", err)
	}

	if !data.HasPreviousReport {
		t.Error("HasPreviousReport = false, want true")
	}
	row := data.TopAlarms[0]
	if !row.FoundInPrevious {
		t.Fatal("FoundInPrevious = false, want true")
	}
	if row.PrevCount != 4 {
		t.Errorf("PrevCount = %d, want 4", row.PrevCount)
	}
}
