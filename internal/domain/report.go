package domain

import "time"

// TopAlarmRow is one row of the Top N Alarmas table — either a correlated
// root-cause incident (spanning one or more EventTypes/components) or a
// standalone EventType not covered by any correlation rule.
type TopAlarmRow struct {
	Rank        int
	Severity    Severity
	Component   string // single component, or "+"-joined for a cross-component correlation
	Alarma      string // short human title
	Descripcion string // root-cause narrative
	Ejemplo     string // representative sample message, PII masked
	Count       int

	// Key is a stable identifier for week-over-week matching — a
	// CorrelationRule.Name for correlation-based rows, or
	// "Component|EventType" for standalone rows. Unlike Alarma (the
	// display title), Key doesn't change if alarm_catalog.go's wording is
	// edited later, so trend comparison survives catalog updates.
	Key string

	// Week-over-week trend, populated when a previous report exists at the
	// same output path (see ReportData.HasPreviousReport). FoundInPrevious
	// is false when this alarm didn't appear in the previous report at
	// all (new pattern, or just outside its Top N).
	PrevCount       int
	FoundInPrevious bool
}

// ReportData is everything the reporting layer needs to render the Excel
// output — already aggregated, no OpenSearch or Excel concerns leak in here.
type ReportData struct {
	GeneratedAt time.Time
	From, To    time.Time
	TotalEvents int
	SkippedDocs int // hits that matched the query but couldn't be mapped into an Event
	TopAlarms   []TopAlarmRow

	// HasPreviousReport is false on a first run (no prior report to
	// compare against) — TopAlarmRow.PrevCount/FoundInPrevious are
	// meaningless in that case, not just zero.
	HasPreviousReport bool
}
