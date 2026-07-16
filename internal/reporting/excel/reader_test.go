package excel

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"osreport/internal/domain"
)

func TestReadPreviousCounts_NoFileYet(t *testing.T) {
	counts, existed, err := ReadPreviousCounts(filepath.Join(t.TempDir(), "does-not-exist.xlsx"))
	if err != nil {
		t.Fatalf("ReadPreviousCounts() error = %v", err)
	}
	if existed {
		t.Error("existed = true, want false for a path that doesn't exist")
	}
	if counts != nil {
		t.Errorf("counts = %v, want nil", counts)
	}
}

func TestReadPreviousCounts_RoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	data := domain.ReportData{
		GeneratedAt: ts,
		From:        ts.Add(-24 * time.Hour),
		To:          ts,
		TopAlarms: []domain.TopAlarmRow{
			{Rank: 1, Severity: domain.SeverityCritical, Component: "M3UA", Alarma: "Enlace SS7 (M3UA/SIGTRAN) inestable", Key: "m3ua-asp-down-with-conn-timeout", Count: 52425},
			{Rank: 2, Severity: domain.SeverityMajor, Component: "HSS_IMS", Alarma: "IMSI desconocido (AuthInfo)", Key: "HSS_IMS|HSS_UNKNOWN_IMSI", Count: 1954},
		},
	}

	path := filepath.Join(t.TempDir(), "informe.xlsx")
	if err := NewWriter().Write(context.Background(), data, path); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	counts, existed, err := ReadPreviousCounts(path)
	if err != nil {
		t.Fatalf("ReadPreviousCounts() error = %v", err)
	}
	if !existed {
		t.Fatal("existed = false, want true")
	}
	if counts["m3ua-asp-down-with-conn-timeout"] != 52425 {
		t.Errorf("M3UA count = %d, want 52425", counts["m3ua-asp-down-with-conn-timeout"])
	}
	if counts["HSS_IMS|HSS_UNKNOWN_IMSI"] != 1954 {
		t.Errorf("HSS_IMS count = %d, want 1954", counts["HSS_IMS|HSS_UNKNOWN_IMSI"])
	}
}

// TestReadPreviousCounts_PreKeyColumnReportIsSkippedNotErrored verifies
// that a report written before the Key column existed (9 columns instead
// of 10) degrades gracefully: no trend data recovered from it, but no
// error either — the next run just starts a fresh baseline.
func TestReadPreviousCounts_PreKeyColumnReportIsSkippedNotErrored(t *testing.T) {
	f := excelize.NewFile()
	const sheet = "Top 10 Errores"
	f.NewSheet(sheet)
	f.SetSheetRow(sheet, "A1", &[]interface{}{"Rank", "Severidad", "Componente", "Alarma", "Descripcion", "Ejemplo", "Count", "Anterior", "Tendencia"})
	f.SetSheetRow(sheet, "A2", &[]interface{}{1, "Critico", "M3UA", "Enlace SS7 (M3UA/SIGTRAN) inestable", "desc", "ejemplo", 52425, nil, "N/A (primer reporte)"})

	path := filepath.Join(t.TempDir(), "old-informe.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs() error = %v", err)
	}

	counts, existed, err := ReadPreviousCounts(path)
	if err != nil {
		t.Fatalf("ReadPreviousCounts() error = %v", err)
	}
	if !existed {
		t.Fatal("existed = false, want true")
	}
	if len(counts) != 0 {
		t.Errorf("counts = %v, want empty (no Key column to read)", counts)
	}
}

func TestTrendLabel(t *testing.T) {
	cases := []struct {
		name          string
		row           domain.TopAlarmRow
		hasPrevReport bool
		want          string
	}{
		{
			name:          "first run ever",
			row:           domain.TopAlarmRow{Count: 100},
			hasPrevReport: false,
			want:          "N/A (primer reporte)",
		},
		{
			name:          "new alarm not in previous report",
			row:           domain.TopAlarmRow{Count: 100, FoundInPrevious: false},
			hasPrevReport: true,
			want:          "Nuevo",
		},
		{
			name:          "increase",
			row:           domain.TopAlarmRow{Count: 120, PrevCount: 100, FoundInPrevious: true},
			hasPrevReport: true,
			want:          "+20%",
		},
		{
			name:          "decrease",
			row:           domain.TopAlarmRow{Count: 80, PrevCount: 100, FoundInPrevious: true},
			hasPrevReport: true,
			want:          "-20%",
		},
		{
			name:          "no change",
			row:           domain.TopAlarmRow{Count: 100, PrevCount: 100, FoundInPrevious: true},
			hasPrevReport: true,
			want:          "0%",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TrendLabel(tc.row, tc.hasPrevReport)
			if got != tc.want {
				t.Errorf("TrendLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}
