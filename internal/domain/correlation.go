package domain

import (
	"sort"
	"strings"
	"time"
)

// CorrelationRule groups events that, taken together, point at one
// underlying root cause rather than N independent problems.
//
// The join key used here is (Host, Component, time window) — NOT a shared
// peer/IP identifier. Real data shows why: the ASP_DOWN message carries a
// peer name ("for peer GSMSC2-0_ASP") while the CONN_TIMEOUT message on the
// same link carries an IP:port instead ("Connection to 181.176.252.249:3036
// failed") — the two messages don't share a directly comparable field. Both
// extracted identifiers are kept on the individual Events for a human
// reviewing the report to cross-check, but the code does not assert they
// match.
type CorrelationRule struct {
	Name        string
	Components  []string // ALERT_COMPONENT values this rule applies to; nil/empty = any component
	EventTypes  []string // EventTypes[0] is the "cause" indicator used as the row's representative count
	Window      time.Duration
	RootCause   string
	Description string // root-cause narrative for the Top N Alarmas report
}

// DefaultCorrelationRules encodes live cases confirmed against index-athonet:
//   - ASP_DOWN and CONN_TIMEOUT on the same M3UA link, seconds apart,
//     repeating (2026-07-14) — one unstable SS7/SIGTRAN link surfacing as
//     two distinct log lines.
//   - TCAP_CCO_EXHAUSTED and MAP_IDH_LINK_FAIL on the same host, both
//     symptoms of the same TCAP/MAP transaction-pool exhaustion
//     (2026-07-08..07-15) — this one spans two different
//     ALERT_COMPONENT values (TCAP and MAP), unlike the M3UA case.
var DefaultCorrelationRules = []CorrelationRule{
	{
		Name:        "m3ua-asp-down-with-conn-timeout",
		Components:  []string{"M3UA"},
		EventTypes:  []string{"ASP_DOWN", "CONN_TIMEOUT"},
		Window:      30 * time.Second,
		RootCause:   "Enlace SS7 (M3UA/SIGTRAN) inestable",
		Description: "Par recurrente de caida de ASP y timeout de conexion sobre el mismo enlace - el enlace se cae y reconecta repetidamente, afectando senalizacion hacia ese peer.",
	},
	{
		Name:        "tcap-map-pool-exhausted",
		Components:  []string{"TCAP", "MAP"},
		EventTypes:  []string{"TCAP_CCO_EXHAUSTED", "MAP_IDH_LINK_FAIL"},
		Window:      60 * time.Second,
		RootCause:   "Agotamiento de pool de transacciones TCAP/MAP (SMS, autenticacion, ubicacion)",
		Description: "TCAP no logra reservar slots de control (CCO) y, segundos despues en el mismo host, MAP no puede reasociar el dialogo de respuesta - mismo agotamiento de pool visto desde dos componentes distintos.",
	},
}

type CorrelationGroup struct {
	RootCause string
	Component string // distinct Component values among Events, joined with "+" (e.g. "TCAP+MAP")
	Host      string
	Events    []Event
	Severity  Severity // worst severity among Events
}

// Correlate detects co-occurrence of a rule's EventTypes within its Window,
// per (Host, rule.Components) bucket. It does not mutate events.
func Correlate(events []Event, rules []CorrelationRule) []CorrelationGroup {
	var groups []CorrelationGroup

	for _, rule := range rules {
		buckets := bucketByHost(events, rule.Components)
		for host, bucket := range buckets {
			groups = append(groups, correlateBucket(bucket, rule, host)...)
		}
	}
	return groups
}

// bucketByHost groups events by Host, restricted to components (nil/empty
// matches any component).
func bucketByHost(events []Event, components []string) map[string][]Event {
	allowed := make(map[string]bool, len(components))
	for _, c := range components {
		allowed[c] = true
	}

	buckets := make(map[string][]Event)
	for _, e := range events {
		if len(allowed) > 0 && !allowed[e.Component] {
			continue
		}
		buckets[e.Host] = append(buckets[e.Host], e)
	}
	for host := range buckets {
		b := buckets[host]
		sort.Slice(b, func(i, j int) bool { return b[i].Timestamp.Before(b[j].Timestamp) })
		buckets[host] = b
	}
	return buckets
}

func correlateBucket(bucket []Event, rule CorrelationRule, host string) []CorrelationGroup {
	var groups []CorrelationGroup
	n := len(bucket)

	for i := 0; i < n; {
		windowEnd := bucket[i].Timestamp.Add(rule.Window)
		seen := make(map[string][]Event, len(rule.EventTypes))

		j := i
		for j < n && !bucket[j].Timestamp.After(windowEnd) {
			for _, want := range rule.EventTypes {
				if bucket[j].EventType == want {
					seen[want] = append(seen[want], bucket[j])
				}
			}
			j++
		}

		if allTypesPresent(seen, rule.EventTypes) {
			groups = append(groups, buildGroup(rule, host, seen))
			i = j // skip past this window instead of re-scanning overlapping events
			continue
		}
		i++
	}
	return groups
}

func allTypesPresent(seen map[string][]Event, want []string) bool {
	for _, t := range want {
		if len(seen[t]) == 0 {
			return false
		}
	}
	return true
}

func buildGroup(rule CorrelationRule, host string, seen map[string][]Event) CorrelationGroup {
	var events []Event
	worst := SeverityUnknown
	seenComponent := map[string]bool{}
	var components []string
	for _, t := range rule.EventTypes {
		for _, e := range seen[t] {
			events = append(events, e)
			if e.Severity > worst {
				worst = e.Severity
			}
			if !seenComponent[e.Component] {
				seenComponent[e.Component] = true
				components = append(components, e.Component)
			}
		}
	}
	return CorrelationGroup{
		RootCause: rule.RootCause,
		Component: strings.Join(components, "+"),
		Host:      host,
		Events:    events,
		Severity:  worst,
	}
}
