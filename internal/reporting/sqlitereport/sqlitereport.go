// Package sqlitereport keeps a small history of past refreshes in a SQLite
// file, so the dashboard can draw its own trend sparklines (per-alarm and
// total-events) without depending on an external BI tool.
package sqlitereport

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"osreport/internal/domain"
	"osreport/internal/reporting/excel"
)

const (
	// retentionDays bounds how far back history is kept — old rows are
	// pruned on every write so the file doesn't grow forever.
	retentionDays = 180

	// SparklinePoints is how many of the most recent refreshes a sparkline
	// covers. Exported so serve.go's read calls and the dashboard template
	// agree on the same window without repeating the number.
	SparklinePoints = 12
)

// RecordRefresh appends one row per TopAlarmRow plus one overall totals row
// for this refresh, then prunes anything older than retentionDays. Unlike a
// snapshot overwrite, this never deletes the current refresh's predecessors
// — that history is the whole point.
func RecordRefresh(path string, data domain.ReportData) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS top_alarms_history (
			generated_at TEXT,
			key          TEXT,
			alarma       TEXT,
			severity     TEXT,
			component    TEXT,
			count        INTEGER
		)
	`); err != nil {
		return fmt.Errorf("create top_alarms_history: %w", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS report_meta_history (
			generated_at TEXT,
			total_events INTEGER,
			skipped_docs INTEGER
		)
	`); err != nil {
		return fmt.Errorf("create report_meta_history: %w", err)
	}
	// Unlike top_alarms_history (the dashboard's own Top N, used for its
	// sparklines/badge), this table gets every alarm this run classified,
	// cut or not — the durable "nothing gets thrown away" record.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS all_alarms_history (
			generated_at TEXT,
			key          TEXT,
			alarma       TEXT,
			descripcion  TEXT,
			severity     TEXT,
			component    TEXT,
			count        INTEGER
		)
	`); err != nil {
		return fmt.Errorf("create all_alarms_history: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // no-op once Commit has succeeded

	generatedAt := data.GeneratedAt.Format(time.RFC3339)

	stmt, err := tx.Prepare(`
		INSERT INTO top_alarms_history (generated_at, key, alarma, severity, component, count)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, row := range data.TopAlarms {
		if _, err := stmt.Exec(generatedAt, row.Key, row.Alarma, excel.SeverityLabel(row.Severity),
			row.Component, row.Count); err != nil {
			return fmt.Errorf("insert top_alarms_history row: %w", err)
		}
	}

	allStmt, err := tx.Prepare(`
		INSERT INTO all_alarms_history (generated_at, key, alarma, descripcion, severity, component, count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare all_alarms insert: %w", err)
	}
	defer allStmt.Close()

	for _, row := range data.AllAlarms {
		if _, err := allStmt.Exec(generatedAt, row.Key, row.Alarma, row.Descripcion,
			excel.SeverityLabel(row.Severity), row.Component, row.Count); err != nil {
			return fmt.Errorf("insert all_alarms_history row: %w", err)
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO report_meta_history (generated_at, total_events, skipped_docs)
		VALUES (?, ?, ?)
	`, generatedAt, data.TotalEvents, data.SkippedDocs); err != nil {
		return fmt.Errorf("insert report_meta_history: %w", err)
	}

	cutoff := data.GeneratedAt.AddDate(0, 0, -retentionDays).Format(time.RFC3339)
	if _, err := tx.Exec(`DELETE FROM top_alarms_history WHERE generated_at < ?`, cutoff); err != nil {
		return fmt.Errorf("prune top_alarms_history: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM all_alarms_history WHERE generated_at < ?`, cutoff); err != nil {
		return fmt.Errorf("prune all_alarms_history: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM report_meta_history WHERE generated_at < ?`, cutoff); err != nil {
		return fmt.Errorf("prune report_meta_history: %w", err)
	}

	return tx.Commit()
}

// LatestAllAlarms returns every alarm recorded at the most recent
// generated_at timestamp in all_alarms_history — the full "everything that
// happened last refresh" list, ordered worst-first (severity, then count).
// Returns nil, nil if the table doesn't exist yet (no refresh has run with
// sqlite history enabled).
func LatestAllAlarms(path string) ([]AllAlarmRecord, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT key, alarma, descripcion, severity, component, count
		FROM all_alarms_history
		WHERE generated_at = (SELECT MAX(generated_at) FROM all_alarms_history)
		ORDER BY
			CASE severity
				WHEN 'Critico' THEN 4
				WHEN 'Alto' THEN 3
				WHEN 'Medio' THEN 2
				WHEN 'Bajo' THEN 1
				ELSE 0
			END DESC,
			count DESC
	`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var out []AllAlarmRecord
	for rows.Next() {
		var rec AllAlarmRecord
		if err := rows.Scan(&rec.Key, &rec.Alarma, &rec.Descripcion, &rec.Severity, &rec.Component, &rec.Count); err != nil {
			return nil, fmt.Errorf("scan all_alarms_history row: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all_alarms_history: %w", err)
	}
	return out, nil
}

// AllAlarmRecord is one stored row from all_alarms_history.
type AllAlarmRecord struct {
	Key         string
	Alarma      string
	Descripcion string
	Severity    string
	Component   string
	Count       int
}

// AlarmHistory returns, per alarm Key, the count from up to the last
// SparklinePoints refreshes — oldest first, ready to feed straight into a
// sparkline left-to-right.
func AlarmHistory(path string) (map[string][]int, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT key, count FROM top_alarms_history ORDER BY generated_at ASC`)
	if err != nil {
		// A brand new sqlite file (no history recorded yet) has no
		// top_alarms_history table at all — that's not an error, just an
		// empty result.
		return map[string][]int{}, nil
	}
	defer rows.Close()

	history := map[string][]int{}
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, fmt.Errorf("scan top_alarms_history row: %w", err)
		}
		history[key] = append(history[key], count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top_alarms_history: %w", err)
	}

	for key, counts := range history {
		history[key] = lastN(counts, SparklinePoints)
	}
	return history, nil
}

// TotalEventsHistory returns up to the last SparklinePoints total_events
// values, oldest first.
func TotalEventsHistory(path string) ([]int, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT total_events FROM report_meta_history ORDER BY generated_at ASC`)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var counts []int
	for rows.Next() {
		var total int
		if err := rows.Scan(&total); err != nil {
			return nil, fmt.Errorf("scan report_meta_history row: %w", err)
		}
		counts = append(counts, total)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report_meta_history: %w", err)
	}

	return lastN(counts, SparklinePoints), nil
}

func lastN(counts []int, n int) []int {
	if len(counts) <= n {
		return counts
	}
	return counts[len(counts)-n:]
}
