package domain

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return ts
}

func TestCorrelate_RealASPDownWithTimeoutCase(t *testing.T) {
	events := []Event{
		{
			ID: "1", Host: "048z000503y0", Component: "M3UA",
			Timestamp: mustParse(t, "2026-07-14T18:04:17Z"),
			EventType: "CONN_TIMEOUT", RawSeverity: "ERR", Severity: SeverityMajor,
			RemoteAddr: "181.176.252.249:3036",
		},
		{
			ID: "2", Host: "048z000503y0", Component: "M3UA",
			Timestamp: mustParse(t, "2026-07-14T18:04:17Z"),
			EventType: "ASP_DOWN", RawSeverity: "SYS", Severity: SeverityCritical,
			Peer: "GSMSC2-0_ASP",
		},
	}

	groups := Correlate(events, DefaultCorrelationRules)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	g := groups[0]
	if g.RootCause != "Enlace SS7 (M3UA/SIGTRAN) inestable" {
		t.Errorf("unexpected root cause: %q", g.RootCause)
	}
	if g.Severity != SeverityCritical {
		t.Errorf("group severity = %v, want Critical (worst of the pair)", g.Severity)
	}
	if len(g.Events) != 2 {
		t.Errorf("got %d events in group, want 2", len(g.Events))
	}
}

func TestCorrelate_DoesNotMergeAcrossHosts(t *testing.T) {
	events := []Event{
		{ID: "1", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:04:17Z"), EventType: "CONN_TIMEOUT"},
		{ID: "2", Host: "host-B", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:04:17Z"), EventType: "ASP_DOWN"},
	}

	groups := Correlate(events, DefaultCorrelationRules)

	if len(groups) != 0 {
		t.Fatalf("got %d groups, want 0 — events are on different hosts", len(groups))
	}
}

func TestCorrelate_CrossComponentTCAPWithMAP(t *testing.T) {
	events := []Event{
		{
			ID: "1", Host: "048z000503y0", Component: "TCAP",
			Timestamp: mustParse(t, "2026-07-10T10:00:00Z"),
			EventType: "TCAP_CCO_EXHAUSTED", RawSeverity: "ERR", Severity: SeverityCritical,
		},
		{
			ID: "2", Host: "048z000503y0", Component: "MAP",
			Timestamp: mustParse(t, "2026-07-10T10:00:30Z"),
			EventType: "MAP_IDH_LINK_FAIL", RawSeverity: "ERR", Severity: SeverityMajor,
		},
	}

	groups := Correlate(events, DefaultCorrelationRules)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	g := groups[0]
	if g.RootCause != "Agotamiento de pool de transacciones TCAP/MAP (SMS, autenticacion, ubicacion)" {
		t.Errorf("unexpected root cause: %q", g.RootCause)
	}
	if g.Component != "TCAP+MAP" {
		t.Errorf("Component = %q, want %q", g.Component, "TCAP+MAP")
	}
	if g.Severity != SeverityCritical {
		t.Errorf("group severity = %v, want Critical (worst of the pair)", g.Severity)
	}
}

// TestCorrelate_BurstWithinOneWindowMergesIntoASingleGroup documents a
// deliberate (if non-obvious) design choice: correlateBucket anchors one
// window at the first unconsumed event and sweeps in EVERY matching event
// through the end of that window — it does not pair events 1:1. A rapid
// flapping link (3 ASP_DOWN + 2 CONN_TIMEOUT, all within the 30s window)
// becomes ONE incident with all 5 events, not up to 3 separate pairs. This
// matches the real M3UA case this rule was built from ("the link flaps
// repeatedly") — a reviewer unfamiliar with this file mistook it for a bug
// during an earlier audit, so it's pinned down here explicitly.
func TestCorrelate_BurstWithinOneWindowMergesIntoASingleGroup(t *testing.T) {
	events := []Event{
		{ID: "1", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:00:00Z"), EventType: "ASP_DOWN", Severity: SeverityCritical},
		{ID: "2", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:00:02Z"), EventType: "CONN_TIMEOUT", Severity: SeverityMajor},
		{ID: "3", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:00:05Z"), EventType: "ASP_DOWN", Severity: SeverityCritical},
		{ID: "4", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:00:10Z"), EventType: "CONN_TIMEOUT", Severity: SeverityMajor},
		{ID: "5", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:00:15Z"), EventType: "ASP_DOWN", Severity: SeverityCritical},
	}

	groups := Correlate(events, DefaultCorrelationRules)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1 (the whole burst merges into a single incident)", len(groups))
	}
	if len(groups[0].Events) != 5 {
		t.Errorf("got %d events in the group, want all 5", len(groups[0].Events))
	}
}

func TestCorrelate_DoesNotMergeOutsideWindow(t *testing.T) {
	events := []Event{
		{ID: "1", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:00:00Z"), EventType: "CONN_TIMEOUT"},
		{ID: "2", Host: "host-A", Component: "M3UA", Timestamp: mustParse(t, "2026-07-14T18:10:00Z"), EventType: "ASP_DOWN"},
	}

	groups := Correlate(events, DefaultCorrelationRules)

	if len(groups) != 0 {
		t.Fatalf("got %d groups, want 0 — 10 minutes apart exceeds the 30s window", len(groups))
	}
}
