package domain

// SeverityRule maps an event to the report's severity scale. Rules are
// evaluated in order; the first match wins. Keep specific event-type rules
// before the raw-severity fallback, since ALERT_SEVERITY alone is
// misleading here: the actual outage indication (ASP_DOWN) is tagged "SYS"
// in the source data, not "ERR"/"CRIT" as its real impact would suggest.
type SeverityRule struct {
	Name   string
	Match  func(Event) bool
	Result Severity
}

// DefaultSeverityRules encodes the classification learned from real
// index-athonet data (ALERT_COMPONENT=M3UA, 2026-07-14): a same-second pair
// of "aspsm_down_ind ... DOWN indication" (raw severity SYS) and
// "handle_io_conn ... Connection timed out" (raw severity ERR) on the same
// SS7/SIGTRAN link. Passing ALERT_SEVERITY straight through would rank the
// actual outage (SYS) below its own symptom (ERR) — wrong for a report meant
// to rank findings by real impact.
var DefaultSeverityRules = []SeverityRule{
	{
		Name:   "asp-down-is-critical",
		Match:  func(e Event) bool { return e.EventType == "ASP_DOWN" },
		Result: SeverityCritical,
	},
	{
		// Without this, ASP_UP falls through to raw-sys-is-major (its raw
		// ALERT_SEVERITY, like ASP_DOWN's, is SYS) and a recovery event
		// ranks the same as an active failure.
		Name:   "asp-up-is-info",
		Match:  func(e Event) bool { return e.EventType == "ASP_UP" },
		Result: SeverityInfo,
	},
	{
		Name:   "conn-timeout-is-major",
		Match:  func(e Event) bool { return e.EventType == "CONN_TIMEOUT" || e.EventType == "CONN_RESET" },
		Result: SeverityMajor,
	},
	{
		Name:   "conn-refused-is-major",
		Match:  func(e Event) bool { return e.EventType == "CONN_REFUSED" },
		Result: SeverityMajor,
	},
	{
		Name:   "peer-io-error-is-major",
		Match:  func(e Event) bool { return e.EventType == "PEER_IO_ERROR" },
		Result: SeverityMajor,
	},
	{
		// A whole AS route unreachable — real counts show this flapping
		// with AS_REACHABLE at near-equal volume, so it reads as recurring
		// route instability rather than one sustained outage.
		Name:   "as-unreachable-is-major",
		Match:  func(e Event) bool { return e.EventType == "AS_UNREACHABLE" },
		Result: SeverityMajor,
	},
	{
		Name:   "as-reachable-is-info",
		Match:  func(e Event) bool { return e.EventType == "AS_REACHABLE" },
		Result: SeverityInfo,
	},
	{
		// ACTIVE -> DOWN is a real outage of the whole Application Server,
		// same tier as ASP_DOWN.
		Name:   "as-state-down-is-critical",
		Match:  func(e Event) bool { return e.EventType == "AS_STATE_DOWN" },
		Result: SeverityCritical,
	},
	{
		Name:   "as-state-recovering-is-minor",
		Match:  func(e Event) bool { return e.EventType == "AS_STATE_RECOVERING" },
		Result: SeverityMinor,
	},
	{
		Name:   "as-state-active-is-info",
		Match:  func(e Event) bool { return e.EventType == "AS_STATE_ACTIVE" },
		Result: SeverityInfo,
	},
	// TCAP/MAP/HSS_IMS/S6a/DIAM business-impact overrides, from the same
	// 2026-07-08..07-15 analysis as knownFunctionEventTypes in classify.go.
	{
		Name:   "tcap-cco-exhausted-is-critical",
		Match:  func(e Event) bool { return e.EventType == "TCAP_CCO_EXHAUSTED" },
		Result: SeverityCritical,
	},
	{
		Name:   "map-dsm-16015-stuck-is-critical",
		Match:  func(e Event) bool { return e.EventType == "MAP_DSM_16015_STUCK" },
		Result: SeverityCritical,
	},
	{
		Name:   "map-idh-link-fail-is-major",
		Match:  func(e Event) bool { return e.EventType == "MAP_IDH_LINK_FAIL" },
		Result: SeverityMajor,
	},
	{
		Name:   "map-dsm0-not-init-is-major",
		Match:  func(e Event) bool { return e.EventType == "MAP_DSM0_NOT_INIT" },
		Result: SeverityMajor,
	},
	{
		Name:   "map-acn-unsupported-is-minor",
		Match:  func(e Event) bool { return e.EventType == "MAP_ACN_UNSUPPORTED" },
		Result: SeverityMinor,
	},
	{
		Name:   "map-error-ind-unhandled-is-major",
		Match:  func(e Event) bool { return e.EventType == "MAP_ERROR_IND_UNHANDLED" },
		Result: SeverityMajor,
	},
	{
		Name:   "map-dsm-dummy-link-is-major",
		Match:  func(e Event) bool { return e.EventType == "MAP_DSM_DUMMY_LINK" },
		Result: SeverityMajor,
	},
	{
		Name:   "hss-unknown-imsi-is-major",
		Match:  func(e Event) bool { return e.EventType == "HSS_UNKNOWN_IMSI" },
		Result: SeverityMajor,
	},
	{
		Name:   "hss-unknown-msisdn-is-minor",
		Match:  func(e Event) bool { return e.EventType == "HSS_UNKNOWN_MSISDN" },
		Result: SeverityMinor,
	},
	{
		Name:   "s6a-unknown-imsi-is-major",
		Match:  func(e Event) bool { return e.EventType == "S6A_UNKNOWN_IMSI" },
		Result: SeverityMajor,
	},
	{
		Name:   "s6a-unknown-cp-is-minor",
		Match:  func(e Event) bool { return e.EventType == "S6A_UNKNOWN_CP" },
		Result: SeverityMinor,
	},
	{
		Name:   "diam-peer-down-is-major",
		Match:  func(e Event) bool { return e.EventType == "DIAM_PEER_DOWN" },
		Result: SeverityMajor,
	},
	{
		Name:   "diam-dynamic-peer-is-minor",
		Match:  func(e Event) bool { return e.EventType == "DIAM_DYNAMIC_PEER" },
		Result: SeverityMinor,
	},
	{
		Name:   "diam-orphan-answer-is-info",
		Match:  func(e Event) bool { return e.EventType == "DIAM_ORPHAN_ANSWER" },
		Result: SeverityInfo,
	},
	{
		// Raw severity is SYS (would default to Major via raw-sys-is-major
		// below), but the message itself ("N subs inside profile") reads as
		// routine provisioning telemetry (a subscription insert/delete
		// triggering a profile reload), not a fault — unlike ASP_DOWN's SYS
		// tag, which does mean a real outage. This is a first-pass read
		// from the message text alone (2026-08-03), not a multi-day
		// business-impact study like the M3UA/TCAP rules above — revisit if
		// this volume ever turns out to matter operationally.
		Name:   "hss-profile-reload-is-info",
		Match:  func(e Event) bool { return e.EventType == "HSS_PROFILE_RELOAD" },
		Result: SeverityInfo,
	},
	{
		Name:   "raw-err-is-major",
		Match:  func(e Event) bool { return e.RawSeverity == "ERR" },
		Result: SeverityMajor,
	},
	{
		Name:   "raw-sys-is-major",
		Match:  func(e Event) bool { return e.RawSeverity == "SYS" },
		Result: SeverityMajor,
	},
	{
		Name:   "raw-wrn-is-minor",
		Match:  func(e Event) bool { return e.RawSeverity == "WRN" },
		Result: SeverityMinor,
	},
	{
		Name:   "raw-inf-dbg-is-info",
		Match:  func(e Event) bool { return e.RawSeverity == "INF" || e.RawSeverity == "DBG" },
		Result: SeverityInfo,
	},
}

// ClassifySeverity assigns a Severity to e using rules, in order. Events
// matching no rule get SeverityUnknown — that's a signal to add a rule, not
// a value to silently default away.
func ClassifySeverity(e Event, rules []SeverityRule) Severity {
	for _, r := range rules {
		if r.Match(e) {
			return r.Result
		}
	}
	return SeverityUnknown
}

// ClassifyAll classifies every event in place and returns the slice for
// chaining.
func ClassifyAll(events []Event, rules []SeverityRule) []Event {
	for i := range events {
		events[i].Severity = ClassifySeverity(events[i], rules)
	}
	return events
}
