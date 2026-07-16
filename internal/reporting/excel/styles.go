package excel

import (
	"github.com/xuri/excelize/v2"

	"osreport/internal/domain"
)

// severityColors gives each report severity a distinct fill, matching the
// usual red/orange/yellow/gray convention so a reviewer can triage by
// scanning color before reading any text. Exported (as SeverityColor) so
// the web dashboard can render the same color language as the .xlsx.
var severityColors = map[domain.Severity]string{
	domain.SeverityCritical: "FFC7CE", // red
	domain.SeverityMajor:    "FFD9A0", // orange
	domain.SeverityMinor:    "FFEB9C", // yellow
	domain.SeverityInfo:     "D9D9D9", // gray
}

var severityLabels = map[domain.Severity]string{
	domain.SeverityCritical: "Critico",
	domain.SeverityMajor:    "Alto",
	domain.SeverityMinor:    "Medio",
	domain.SeverityInfo:     "Bajo",
}

// SeverityLabel is the Spanish label shown in the report; SeverityUnknown
// (and anything else uncataloged) shows as "Desconocido" rather than
// silently picking one of the known labels.
func SeverityLabel(s domain.Severity) string {
	if label, ok := severityLabels[s]; ok {
		return label
	}
	return "Desconocido"
}

// SeverityColor is the hex fill color (no leading "#") for s, and whether
// s has one at all — SeverityUnknown deliberately doesn't, so callers can
// leave it unstyled instead of guessing a color for it.
func SeverityColor(s domain.Severity) (color string, ok bool) {
	color, ok = severityColors[s]
	return color, ok
}

// buildSeverityStyles creates one style per known severity color, up
// front — writeTopAlarmsSheet calls this once per report, not once per
// row, so a report with a large --top doesn't accumulate a duplicate
// style definition (and bloat the .xlsx) for every single row.
func buildSeverityStyles(f *excelize.File) (map[domain.Severity]int, error) {
	styles := make(map[domain.Severity]int, len(severityColors))
	for sev, color := range severityColors {
		styleID, err := f.NewStyle(&excelize.Style{
			Fill:      excelize.Fill{Type: "pattern", Color: []string{color}, Pattern: 1},
			Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"},
		})
		if err != nil {
			return nil, err
		}
		styles[sev] = styleID
	}
	return styles, nil
}

// applySeverityCellColor fills the Severidad cell (column B) of row with
// the pre-built style for sev. Rows whose severity has no entry in
// styles (SeverityUnknown) are left unstyled on purpose — an unstyled
// row is a visible prompt to add a classification rule, not something to
// quietly force into a color.
func applySeverityCellColor(f *excelize.File, sheet string, row int, sev domain.Severity, styles map[domain.Severity]int) error {
	styleID, ok := styles[sev]
	if !ok {
		return nil
	}
	cell, err := cellRef(2, row)
	if err != nil {
		return err
	}
	return f.SetCellStyle(sheet, cell, cell, styleID)
}

func newHeaderStyle(f *excelize.File) (int, error) {
	return f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
	})
}
