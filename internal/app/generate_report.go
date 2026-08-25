package app

import (
	"context"
	"fmt"
	"time"

	"osreport/internal/domain"
)

// defaultTopN is used when Params.TopN is left at its zero value.
const defaultTopN = 10

// Params is what a caller (CLI flags today) provides to trigger a report
// run.
type Params struct {
	From       time.Time
	To         time.Time
	Index      string
	Components []string
	Severities []string
	OutputPath string
	TopN       int // rows in the Top N Alarmas table; <= 0 means defaultTopN

	// PrevCounts/PrevReportExists drive the week-over-week trend column —
	// the caller reads them from wherever the previous report lives
	// (typically the same OutputPath, before it gets overwritten).
	PrevCounts       map[string]int
	PrevReportExists bool
}

// GenerateReport orchestrates fetch -> classify -> correlate -> aggregate ->
// write. It depends only on domain interfaces, so it's testable with fakes
// for both EventRepository and ReportWriter — no real OpenSearch or Excel
// needed to test the orchestration itself. It returns the ReportData even
// on a write error (zero value on a search error) so the caller can log
// diagnostics like TotalEvents/SkippedDocs regardless of how it failed.
func GenerateReport(ctx context.Context, repo domain.EventRepository, writer domain.ReportWriter, p Params) (domain.ReportData, error) {
	criteria := domain.Criteria{
		From:       p.From,
		To:         p.To,
		Index:      p.Index,
		Components: p.Components,
		Severities: p.Severities,
	}

	events, skipped, err := repo.Search(ctx, criteria)
	if err != nil {
		return domain.ReportData{}, fmt.Errorf("search: %w", err)
	}

	domain.ClassifyAll(events, domain.DefaultSeverityRules)
	groups := domain.Correlate(events, domain.DefaultCorrelationRules)

	topN := p.TopN
	if topN <= 0 {
		topN = defaultTopN
	}
	data := domain.ReportData{
		GeneratedAt:       time.Now(),
		From:              p.From,
		To:                p.To,
		TotalEvents:       len(events),
		SkippedDocs:       skipped,
		TopAlarms:         BuildTopAlarms(events, groups, topN, p.PrevCounts, p.PrevReportExists),
		AllAlarms:         BuildAllAlarms(events, groups, p.PrevCounts, p.PrevReportExists),
		HasPreviousReport: p.PrevReportExists,
	}

	if err := writer.Write(ctx, data, p.OutputPath); err != nil {
		return data, fmt.Errorf("write report: %w", err)
	}
	return data, nil
}
