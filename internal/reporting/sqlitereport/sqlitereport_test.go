package sqlitereport

import (
	"path/filepath"
	"testing"
	"time"

	"osreport/internal/domain"
)

func TestRecordRefresh_AlarmHistoryAccumulatesAcrossRefreshes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard-informe.sqlite")
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)

	refreshes := []domain.ReportData{
		{GeneratedAt: ts, TotalEvents: 100, TopAlarms: []domain.TopAlarmRow{
			{Key: "m3ua-asp-down", Alarma: "Enlace inestable", Severity: domain.SeverityCritical, Component: "M3UA", Count: 5},
		}},
		{GeneratedAt: ts.Add(24 * time.Hour), TotalEvents: 150, TopAlarms: []domain.TopAlarmRow{
			{Key: "m3ua-asp-down", Alarma: "Enlace inestable", Severity: domain.SeverityCritical, Component: "M3UA", Count: 8},
		}},
		{GeneratedAt: ts.Add(48 * time.Hour), TotalEvents: 120, TopAlarms: []domain.TopAlarmRow{
			{Key: "m3ua-asp-down", Alarma: "Enlace inestable", Severity: domain.SeverityCritical, Component: "M3UA", Count: 3},
		}},
	}
	for i, data := range refreshes {
		if err := RecordRefresh(path, data); err != nil {
			t.Fatalf("RecordRefresh() #%d error = %v", i, err)
		}
	}

	alarmHistory, err := AlarmHistory(path)
	if err != nil {
		t.Fatalf("AlarmHistory() error = %v", err)
	}
	got := alarmHistory["m3ua-asp-down"]
	want := []int{5, 8, 3}
	if len(got) != len(want) {
		t.Fatalf("AlarmHistory()[m3ua-asp-down] = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AlarmHistory()[m3ua-asp-down][%d] = %d, want %d", i, got[i], want[i])
		}
	}

	totalEvents, err := TotalEventsHistory(path)
	if err != nil {
		t.Fatalf("TotalEventsHistory() error = %v", err)
	}
	wantTotals := []int{100, 150, 120}
	if len(totalEvents) != len(wantTotals) {
		t.Fatalf("TotalEventsHistory() = %v, want %v", totalEvents, wantTotals)
	}
	for i := range wantTotals {
		if totalEvents[i] != wantTotals[i] {
			t.Errorf("TotalEventsHistory()[%d] = %d, want %d", i, totalEvents[i], wantTotals[i])
		}
	}
}

func TestAlarmHistory_CapsAtSparklinePoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard-informe.sqlite")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < SparklinePoints+5; i++ {
		data := domain.ReportData{
			GeneratedAt: base.Add(time.Duration(i) * 24 * time.Hour),
			TopAlarms:   []domain.TopAlarmRow{{Key: "k", Count: i}},
		}
		if err := RecordRefresh(path, data); err != nil {
			t.Fatalf("RecordRefresh() #%d error = %v", i, err)
		}
	}

	history, err := AlarmHistory(path)
	if err != nil {
		t.Fatalf("AlarmHistory() error = %v", err)
	}
	got := history["k"]
	if len(got) != SparklinePoints {
		t.Fatalf("len(history[k]) = %d, want %d (should cap, not grow unbounded)", len(got), SparklinePoints)
	}
	// oldest-first ordering: the last SparklinePoints inserts are counts
	// [5..SparklinePoints+4], so the first surviving element is 5.
	if got[0] != 5 {
		t.Errorf("history[k][0] = %d, want 5 (oldest points should be trimmed, not newest)", got[0])
	}
	if got[len(got)-1] != SparklinePoints+4 {
		t.Errorf("history[k][last] = %d, want %d", got[len(got)-1], SparklinePoints+4)
	}
}

func TestRecordRefresh_PrunesRowsOlderThanRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard-informe.sqlite")
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)

	if err := RecordRefresh(path, domain.ReportData{
		GeneratedAt: old, TotalEvents: 1,
		TopAlarms: []domain.TopAlarmRow{{Key: "k", Count: 1}},
	}); err != nil {
		t.Fatalf("RecordRefresh() old error = %v", err)
	}
	if err := RecordRefresh(path, domain.ReportData{
		GeneratedAt: recent, TotalEvents: 2,
		TopAlarms: []domain.TopAlarmRow{{Key: "k", Count: 2}},
	}); err != nil {
		t.Fatalf("RecordRefresh() recent error = %v", err)
	}

	history, err := AlarmHistory(path)
	if err != nil {
		t.Fatalf("AlarmHistory() error = %v", err)
	}
	got := history["k"]
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("history[k] = %v, want [2] (the 2020 row should have been pruned)", got)
	}
}
