package domain

import (
	"context"
	"time"
)

// Criteria is the query scope requested by whoever triggers a report run
// (CLI flags today, an HTTP handler if that's ever added later).
type Criteria struct {
	From       time.Time
	To         time.Time
	Index      string
	Components []string // ALERT_COMPONENT values to include, e.g. ["M3UA"]; empty = any
	Severities []string // ALERT_SEVERITY values to include, e.g. ["ERR","SYS","WRN"]
}

// EventRepository is implemented by infra/opensearch. The domain and app
// layers depend only on this interface, never on the OpenSearch client
// directly. skipped counts hits that matched the query but couldn't be
// mapped into an Event (malformed document, unparseable timestamp) — the
// report surfaces this so a reviewer knows the totals aren't 100% of what
// OpenSearch matched.
type EventRepository interface {
	Search(ctx context.Context, c Criteria) (events []Event, skipped int, err error)
}

// ReportWriter is implemented by reporting/excel.
type ReportWriter interface {
	Write(ctx context.Context, data ReportData, outputPath string) error
}
