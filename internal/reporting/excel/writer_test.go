package excel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	xlsx "github.com/xuri/excelize/v2"

	"osreport/internal/domain"
)

func TestWriter_Write_ProducesReadableWorkbookWithExpectedSheets(t *testing.T) {
	ts := time.Date(2026, 7, 14, 18, 4, 17, 0, time.UTC)
	data := domain.ReportData{
		GeneratedAt: ts,
		From:        ts.Add(-24 * time.Hour),
		To:          ts,
		TotalEvents: 2,
		SkippedDocs: 1,
		TopAlarms: []domain.TopAlarmRow{
			{
				Rank: 1, Severity: domain.SeverityCritical, Component: "M3UA",
				Alarma:      "Enlace SS7 (M3UA/SIGTRAN) inestable",
				Descripcion: "Par recurrente de caida de ASP y timeout de conexion.",
				Ejemplo:     "aspsm_down_ind:1658 DOWN indication for peer GSMSC2-0_ASP",
				Count:       1,
			},
		},
	}

	outputPath := filepath.Join(t.TempDir(), "informe.xlsx")
	w := NewWriter()
	if err := w.Write(context.Background(), data, outputPath); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("output file not created: %v", err)
	}

	f, err := xlsx.OpenFile(outputPath)
	if err != nil {
		t.Fatalf("generated file is not a readable xlsx: %v", err)
	}
	defer f.Close()

	wantSheets := []string{"Resumen", "Top 10 Errores"}
	gotSheets := f.GetSheetList()
	for _, want := range wantSheets {
		found := false
		for _, got := range gotSheets {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("sheet %q missing, got sheets: %v", want, gotSheets)
		}
	}

	alarma, err := f.GetCellValue("Top 10 Errores", "D2")
	if err != nil {
		t.Fatalf("read Top 10 Errores!D2: %v", err)
	}
	if alarma != "Enlace SS7 (M3UA/SIGTRAN) inestable" {
		t.Errorf("Top 10 Errores!D2 (Alarma) = %q, want the M3UA root cause", alarma)
	}

	severidad, err := f.GetCellValue("Top 10 Errores", "B2")
	if err != nil {
		t.Fatalf("read Top 10 Errores!B2: %v", err)
	}
	if severidad != "Critico" {
		t.Errorf("Top 10 Errores!B2 (Severidad) = %q, want Critico", severidad)
	}
}
