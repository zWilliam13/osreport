package app

import (
	"regexp"
	"sort"
	"time"

	"osreport/internal/domain"
)

var (
	imsiRe   = regexp.MustCompile(`\b\d{14,16}f?\b`)
	msisdnRe = regexp.MustCompile(`\+\d{9,15}\b`)
)

// maskPII replaces subscriber identifiers (IMSI, MSISDN) in a sample
// message with placeholders before it goes into a report — the report is
// meant to show the alarm pattern, not repeat real subscriber numbers.
func maskPII(message string) string {
	message = imsiRe.ReplaceAllString(message, "<imsi>")
	message = msisdnRe.ReplaceAllString(message, "<numero>")
	return message
}

type alarmAcc struct {
	row      domain.TopAlarmRow
	lastSeen time.Time
}

// rankAlarms ranks events + correlation groups into one list: a correlated
// root-cause incident (e.g. two EventTypes co-occurring across components)
// becomes one row, keyed by RootCause; any event not swept into a
// correlation group becomes its own row, keyed by (Component, EventType).
// Both kinds of row rank together by (Severity desc, Count desc), matching
// how a reviewer would triage them side by side — no cut applied here, every
// distinct alarm in this run's data comes back. PrevCount/FoundInPrevious
// are populated (when hasPrevReport) before the caller does any truncation,
// so a row's "new" status never depends on whether some later cut kept it.
func rankAlarms(events []domain.Event, groups []domain.CorrelationGroup, prevCounts map[string]int, hasPrevReport bool) []domain.TopAlarmRow {
	ruleByRootCause := make(map[string]domain.CorrelationRule, len(domain.DefaultCorrelationRules))
	for _, r := range domain.DefaultCorrelationRules {
		ruleByRootCause[r.RootCause] = r
	}

	consumed := make(map[string]bool)
	byRootCause := map[string]*alarmAcc{}
	var rootCauseOrder []string

	for _, g := range groups {
		rule := ruleByRootCause[g.RootCause]
		primaryType := ""
		if len(rule.EventTypes) > 0 {
			primaryType = rule.EventTypes[0]
		}

		a, ok := byRootCause[g.RootCause]
		if !ok {
			a = &alarmAcc{row: domain.TopAlarmRow{
				Key:         rule.Name,
				Component:   g.Component,
				Alarma:      g.RootCause,
				Descripcion: rule.Description,
			}}
			byRootCause[g.RootCause] = a
			rootCauseOrder = append(rootCauseOrder, g.RootCause)
		}

		for _, e := range g.Events {
			consumed[e.ID] = true
			if e.Severity > a.row.Severity {
				a.row.Severity = e.Severity
			}
			if e.EventType != primaryType {
				continue
			}
			a.row.Count++
			if e.Timestamp.After(a.lastSeen) {
				a.lastSeen = e.Timestamp
				a.row.Ejemplo = maskPII(e.Message)
			}
		}
	}

	byEventType := map[string]*alarmAcc{}
	var eventTypeOrder []string
	for _, e := range events {
		if consumed[e.ID] {
			continue
		}
		key := e.Component + "|" + e.EventType
		a, ok := byEventType[key]
		if !ok {
			info := domain.DescribeAlarm(e.EventType)
			a = &alarmAcc{row: domain.TopAlarmRow{
				Key:         key,
				Component:   e.Component,
				Alarma:      info.Alarma,
				Descripcion: info.Descripcion,
			}}
			byEventType[key] = a
			eventTypeOrder = append(eventTypeOrder, key)
		}
		a.row.Count++
		if e.Severity > a.row.Severity {
			a.row.Severity = e.Severity
		}
		if e.Timestamp.After(a.lastSeen) {
			a.lastSeen = e.Timestamp
			a.row.Ejemplo = maskPII(e.Message)
		}
	}

	all := make([]domain.TopAlarmRow, 0, len(rootCauseOrder)+len(eventTypeOrder))
	for _, k := range rootCauseOrder {
		all = append(all, byRootCause[k].row)
	}
	for _, k := range eventTypeOrder {
		all = append(all, byEventType[k].row)
	}

	// Stable so two rows tied on both Severity and Count keep their
	// pre-sort (processing) order across runs of the same underlying
	// data, instead of an arbitrary order sort.Slice doesn't guarantee.
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Severity != all[j].Severity {
			return all[i].Severity > all[j].Severity
		}
		return all[i].Count > all[j].Count
	})

	if hasPrevReport {
		for i := range all {
			if count, ok := prevCounts[all[i].Key]; ok {
				all[i].PrevCount = count
				all[i].FoundInPrevious = true
			}
		}
	}

	for i := range all {
		all[i].Rank = i + 1
	}
	return all
}

// BuildAllAlarms returns every distinct alarm this run classified, ranked
// (Severity desc, Count desc) but not cut to any Top N — the durable,
// store-everything counterpart to BuildTopAlarms. A pattern too small or too
// low-severity to make the Top N report is still a real thing that happened;
// this is what a caller persists (see sqlitereport) so nothing is silently
// discarded just because the dashboard/xlsx only has room for the most
// dangerous few.
func BuildAllAlarms(events []domain.Event, groups []domain.CorrelationGroup, prevCounts map[string]int, hasPrevReport bool) []domain.TopAlarmRow {
	return rankAlarms(events, groups, prevCounts, hasPrevReport)
}

// BuildTopAlarms returns the topN most dangerous alarms (Severity desc,
// Count desc) — what the dashboard/xlsx actually display. See
// BuildAllAlarms for the uncut version used for storage.
func BuildTopAlarms(events []domain.Event, groups []domain.CorrelationGroup, topN int, prevCounts map[string]int, hasPrevReport bool) []domain.TopAlarmRow {
	all := rankAlarms(events, groups, prevCounts, hasPrevReport)
	if topN > 0 && len(all) > topN {
		all = all[:topN]
	}
	return all
}
