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

// BuildTopAlarms ranks events + correlation groups into the Top N Alarmas
// table: a correlated root-cause incident (e.g. two EventTypes co-occurring
// across components) becomes one row, keyed by RootCause; any event not
// swept into a correlation group becomes its own row, keyed by
// (Component, EventType). Both kinds of row rank together by
// (Severity desc, Count desc), matching how a reviewer would triage them
// side by side.
func BuildTopAlarms(events []domain.Event, groups []domain.CorrelationGroup, topN int, prevCounts map[string]int) []domain.TopAlarmRow {
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

	if topN > 0 && len(all) > topN {
		all = all[:topN]
	}
	for i := range all {
		all[i].Rank = i + 1
		if count, ok := prevCounts[all[i].Key]; ok {
			all[i].PrevCount = count
			all[i].FoundInPrevious = true
		}
	}
	return all
}
