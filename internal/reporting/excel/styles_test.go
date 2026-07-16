package excel

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"osreport/internal/domain"
)

// TestWriter_Write_ReusesSeverityStyleAcrossRows guards against
// regenerating a new style per row (which would bloat the .xlsx as --top
// grows): two rows with the same Severity must end up with the same
// style ID on their Severidad cell, not two separate-but-identical ones.
func TestWriter_Write_ReusesSeverityStyleAcrossRows(t *testing.T) {
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	data := domain.ReportData{
		GeneratedAt: ts,
		From:        ts.Add(-24 * time.Hour),
		To:          ts,
		TopAlarms: []domain.TopAlarmRow{
			{Rank: 1, Severity: domain.SeverityCritical, Component: "M3UA", Alarma: "A", Key: "a", Count: 10},
			{Rank: 2, Severity: domain.SeverityCritical, Component: "TCAP", Alarma: "B", Key: "b", Count: 5},
		},
	}

	path := filepath.Join(t.TempDir(), "informe.xlsx")
	if err := NewWriter().Write(context.Background(), data, path); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open written file: %v", err)
	}
	defer f.Close()

	style1, err := f.GetCellStyle("Top 10 Errores", "B2")
	if err != nil {
		t.Fatalf("GetCellStyle(B2): %v", err)
	}
	style2, err := f.GetCellStyle("Top 10 Errores", "B3")
	if err != nil {
		t.Fatalf("GetCellStyle(B3): %v", err)
	}
	if style1 != style2 {
		t.Errorf("two Critico rows got different style IDs (%d vs %d) — style is being rebuilt per row instead of reused", style1, style2)
	}
}
