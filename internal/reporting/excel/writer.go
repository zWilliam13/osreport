package excel

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/xuri/excelize/v2"

	"osreport/internal/domain"
)

const timeLayout = "2006-01-02 15:04:05"

// Writer implements domain.ReportWriter using excelize. There is no
// reference template embedded yet — none was provided. Sheets are built
// from scratch below; swap in a real corporate template later via
// go:embed + f.OpenFile / cloning sheets, without changing the
// domain.ReportWriter contract.
type Writer struct{}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) Write(_ context.Context, data domain.ReportData, outputPath string) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	headerStyle, err := newHeaderStyle(f)
	if err != nil {
		return fmt.Errorf("build header style: %w", err)
	}

	resumenIdx, err := writeResumenSheet(f, data, headerStyle)
	if err != nil {
		return fmt.Errorf("write Resumen sheet: %w", err)
	}
	if err := writeTopAlarmsSheet(f, data.TopAlarms, data.HasPreviousReport, headerStyle); err != nil {
		return fmt.Errorf("write Top 10 Errores sheet: %w", err)
	}

	if err := f.DeleteSheet("Sheet1"); err != nil {
		return fmt.Errorf("delete default sheet: %w", err)
	}
	f.SetActiveSheet(resumenIdx)

	if _, err := os.Stat(outputPath); err == nil {
		slog.Warn("overwriting existing output file", "path", outputPath)
	}

	if err := f.SaveAs(outputPath); err != nil {
		return fmt.Errorf("save %s: %w", outputPath, err)
	}
	return nil
}

func writeResumenSheet(f *excelize.File, data domain.ReportData, headerStyle int) (int, error) {
	sheet := "Resumen"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return 0, err
	}

	rows := [][2]string{
		{"Generado", data.GeneratedAt.Format(timeLayout)},
		{"Desde", data.From.Format(timeLayout)},
		{"Hasta", data.To.Format(timeLayout)},
		{"Total de eventos", fmt.Sprintf("%d", data.TotalEvents)},
		{"Documentos omitidos", fmt.Sprintf("%d", data.SkippedDocs)},
		{"Alarmas en el Top", fmt.Sprintf("%d", len(data.TopAlarms))},
	}
	for i, row := range rows {
		r := i + 1
		if err := f.SetCellValue(sheet, fmt.Sprintf("A%d", r), row[0]); err != nil {
			return 0, err
		}
		if err := f.SetCellValue(sheet, fmt.Sprintf("B%d", r), row[1]); err != nil {
			return 0, err
		}
	}
	if err := f.SetCellStyle(sheet, "A1", fmt.Sprintf("A%d", len(rows)), headerStyle); err != nil {
		return 0, err
	}
	if err := f.SetColWidth(sheet, "A", "A", 28); err != nil {
		return 0, err
	}
	if err := f.SetColWidth(sheet, "B", "B", 22); err != nil {
		return 0, err
	}
	return idx, nil
}

// "Key" is a stable internal identifier used to match rows across runs
// (see domain.TopAlarmRow.Key) — it's what ReadPreviousCounts keys off of,
// not the human-facing Alarma text, so editing alarm_catalog.go's wording
// later doesn't silently reset every alarm's trend to "Nuevo".
var topAlarmHeaders = []string{"Rank", "Severidad", "Componente", "Alarma", "Descripcion", "Ejemplo", "Count", "Anterior", "Tendencia", "Key"}

var topAlarmColWidths = map[string]float64{
	"A": 6, "B": 10, "C": 12, "D": 28, "E": 55, "F": 45, "G": 9, "H": 10, "I": 12, "J": 24,
}

func writeTopAlarmsSheet(f *excelize.File, rows []domain.TopAlarmRow, hasPrevReport bool, headerStyle int) error {
	sheet := "Top 10 Errores"
	if _, err := f.NewSheet(sheet); err != nil {
		return err
	}
	if err := writeHeaderRow(f, sheet, topAlarmHeaders, headerStyle); err != nil {
		return err
	}

	wrapStyle, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
	})
	if err != nil {
		return err
	}
	severityStyles, err := buildSeverityStyles(f)
	if err != nil {
		return err
	}

	for i, row := range rows {
		r := i + 2
		prevCount := interface{}(nil)
		if row.FoundInPrevious {
			prevCount = row.PrevCount
		}
		values := []interface{}{
			row.Rank, SeverityLabel(row.Severity), row.Component,
			row.Alarma, row.Descripcion, row.Ejemplo, row.Count,
			prevCount, TrendLabel(row, hasPrevReport), row.Key,
		}
		if err := setRow(f, sheet, r, values); err != nil {
			return err
		}
		firstCell, _ := cellRef(1, r)
		lastCell, _ := cellRef(len(topAlarmHeaders), r)
		if err := f.SetCellStyle(sheet, firstCell, lastCell, wrapStyle); err != nil {
			return err
		}
		if err := applySeverityCellColor(f, sheet, r, row.Severity, severityStyles); err != nil {
			return err
		}
	}

	for col, width := range topAlarmColWidths {
		if err := f.SetColWidth(sheet, col, col, width); err != nil {
			return err
		}
	}

	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, Split: false, XSplit: 4, YSplit: 1,
		TopLeftCell: "E2", ActivePane: "bottomRight",
	}); err != nil {
		return err
	}

	lastRow := len(rows) + 1
	return f.AutoFilter(sheet, fmt.Sprintf("A1:J%d", lastRow), nil)
}

// TrendLabel renders the week-over-week change for row as a Spanish label:
// "N/A (primer reporte)" when there's nothing to compare against yet,
// "Nuevo" when this alarm wasn't in the previous report at all (including
// when its previous count was 0 — a percentage off zero isn't meaningful),
// otherwise a signed percentage.
func TrendLabel(row domain.TopAlarmRow, hasPrevReport bool) string {
	if !hasPrevReport {
		return "N/A (primer reporte)"
	}
	if !row.FoundInPrevious || row.PrevCount == 0 {
		return "Nuevo"
	}
	pct := float64(row.Count-row.PrevCount) / float64(row.PrevCount) * 100
	sign := ""
	if pct > 0 {
		sign = "+"
	}
	return fmt.Sprintf("%s%.0f%%", sign, pct)
}
