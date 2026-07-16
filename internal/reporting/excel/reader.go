package excel

import (
	"fmt"
	"os"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// ReadPreviousCounts reads the "Top 10 Errores" sheet of a previously
// generated report at path, keyed by the stable Key column (see
// domain.TopAlarmRow.Key — not the human-facing Alarma text, so editing
// alarm_catalog.go's wording doesn't silently reset every trend to
// "Nuevo"), for computing a week-over-week trend. It returns
// (nil, false, nil) when path doesn't exist yet — that's the normal
// first-run case, not an error.
//
// A report written before the Key column existed has only 9 columns; rows
// like that are skipped (no trend data recoverable from them), not
// treated as an error — the next run just starts a fresh trend baseline.
func ReadPreviousCounts(path string) (map[string]int, bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, false, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("open previous report %s: %w", path, err)
	}
	defer f.Close()

	rows, err := f.GetRows("Top 10 Errores")
	if err != nil {
		return nil, false, fmt.Errorf("read Top 10 Errores from previous report: %w", err)
	}

	const keyCol = 9 // column J: Rank,Severidad,Componente,Alarma,Descripcion,Ejemplo,Count,Anterior,Tendencia,Key
	counts := make(map[string]int)
	for i, row := range rows {
		if i == 0 || len(row) <= keyCol {
			continue // header row, or a pre-Key-column report
		}
		countStr, key := row[6], row[keyCol]
		count, err := strconv.Atoi(countStr)
		if err != nil || key == "" {
			continue
		}
		counts[key] = count
	}
	return counts, true, nil
}
