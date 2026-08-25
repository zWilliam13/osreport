package domain

import (
	"regexp"
	"strings"
)

// Real message shapes observed in index-athonet, ALERT_COMPONENT=M3UA:
//
//	ERR M3UA    handle_io_conn:363 Connection to 181.176.252.249:3036 failed: Connection timed out
//	SYS M3UA    aspsm_down_ind:1658 DOWN indication for peer GSMSC2-0_ASP
//
// messageLineRe splits off the "<func>:<line> <rest>" shape common to this
// log format. It intentionally does not try to parse the leading
// "<SEVERITY> <COMPONENT>" prefix — those are already available as
// ALERT_SEVERITY/ALERT_COMPONENT fields. The (?s) flag makes "." match
// newlines too — without it, a message with an embedded newline (a
// multi-line body, seen on some components) would fail to match at all,
// since "(.*)$" couldn't reach the real end of the string.
var messageLineRe = regexp.MustCompile(`(?s)(?:^|\s)(\w+):(\d+)\s+(.*)$`)

var peerRe = regexp.MustCompile(`for peer (\S+)`)
var remoteAddrRe = regexp.MustCompile(`Connection to (\S+) failed`)

// knownFunctionEventTypes maps a source function name to a stable event
// type code. Extend this table as new function names are observed in
// production logs — do not guess codes for functions never seen in data.
//
// TCAP/MAP/HSS_IMS/S6a entries below confirmed against real index-athonet
// data for ALERT_COMPONENT in {TCAP,MAP,HSS_IMS,S6a}, 2026-07-08..07-15.
var knownFunctionEventTypes = map[string]string{
	"aspsm_down_ind": "ASP_DOWN",
	"aspsm_up_ind":   "ASP_UP",

	// M3UA Application Server (not ASP) state signaling — confirmed against
	// real data, 2026-07-08..07-16. DUNA/DAVA are the standard M3UA
	// Destination-Unavailable/Available notifications for a whole AS route
	// (e.g. "AS [MOVISTAR_LV] DUNA for [14663/0]"), distinct from a single
	// ASP going down. Real counts show DUNA and DAVA firing in near-equal
	// volume — a route flapping and recovering repeatedly, not a sustained
	// outage, hence Major rather than Critical (see severity.go).
	"as_set_duna": "AS_UNREACHABLE",
	"as_set_dava": "AS_REACHABLE",
	// Full AS state-machine transitions — ACTIVE->DOWN is a real outage of
	// that Application Server (not just one ASP); DOWN->INACTIVE and
	// INACTIVE->ACTIVE are recovery-path steps.
	"as_active_to_down":     "AS_STATE_DOWN",
	"as_down_to_inactive":   "AS_STATE_RECOVERING",
	"as_inactive_to_active": "AS_STATE_ACTIVE",
	// Distinct from handle_io_conn (which covers the M3UA-level
	// Connection-to-X-failed shape) — this is a lower-level socket I/O
	// error ("socket N io-id N error ..."), confirmed as its own message
	// shape in real data.
	"handle_io_peer_err": "PEER_IO_ERROR",

	// TCAP CCO pool exhaustion — tcap_cco_invoke_ind and tcap_cco_reserve
	// fire together (same count observed in real data) for the same
	// underlying resource-pool exhaustion.
	"tcap_cco_invoke_ind": "TCAP_CCO_EXHAUSTED",
	"tcap_cco_reserve":    "TCAP_CCO_EXHAUSTED",

	// MAP — each maps to the one message shape confirmed in real data for
	// that function; an unobserved message under the same function name
	// would need its own disambiguation before reusing these codes.
	"map_invoke_ind":        "MAP_DSM_16015_STUCK",
	"map_req_remap_ids":     "MAP_IDH_LINK_FAIL",
	"map_start_dsm_timer":   "MAP_DSM0_NOT_INIT",
	"map_begin_ind":         "MAP_ACN_UNSUPPORTED",
	"map_error_ind":         "MAP_ERROR_IND_UNHANDLED",
	"map_dsm_reserve_dummy": "MAP_DSM_DUMMY_LINK",

	// HSS_IMS
	"hss_map_AuthInfo_ind": "HSS_UNKNOWN_IMSI",
	"hss_map_sriSM_ind":    "HSS_UNKNOWN_MSISDN",

	// S6a
	"S6_manage_AIR": "S6A_UNKNOWN_IMSI",
	"s6_manage_NOR": "S6A_UNKNOWN_CP",

	// DIAM — act_Process_Recv_Ans_Message follows the func:line shape;
	// the other two DIAM patterns (dynamic peer, mcptt down) don't and are
	// special-cased before messageLineRe below.
	"act_Process_Recv_Ans_Message": "DIAM_ORPHAN_ANSWER",

	// Confirmed in real index-athonet data, 2026-08-03 — surfaced only after
	// aggregate.go started guaranteeing visibility for patterns below the
	// Top N cut (each was running 1,000-2,000 hits/week, always crowded out
	// by the bigger recurring alarms above).
	"hss_reload_subscriptions_with_insdel_subs": "HSS_PROFILE_RELOAD",
	"tcap_dha_end_ind":                          "TCAP_END_UNALLOCATED",
	"_dia_notify_route_failure":                 "DIAM_ROUTE_FAILURE",
	"map_hlr_Auth_close_ind":                    "MAP_AUTH_CLOSE_NOT_IMPLEMENTED",

	// Identical counts in real data (848 == 848, 2026-08-03) — set_state and
	// release fire 1:1 for the same DHA, so treated as one signal rather
	// than two. tcap_start_dha_timer (an older, already-tracked pattern with
	// its own history) is a *different* count in the same data and is left
	// uncataloged-by-code on purpose — see alarm_catalog.go, which gives it
	// a title without touching its Key.
	"tcap_dha_set_state": "TCAP_DHA_NOT_INIT",
	"tcap_dha_release":   "TCAP_DHA_NOT_INIT",
}

// ExtractEventType derives a stable EventType plus any embedded peer/remote
// address identifiers from a raw M3UA-style log line. It returns ok=false
// when the message doesn't match the expected "<func>:<line> <rest>" shape,
// so callers can fall back to treating the event as unclassified rather than
// silently mis-tagging it. component is the ALERT_COMPONENT field, used to
// scope component-specific text patterns (like the two DIAM cases below)
// so an unrelated component can't collide with them.
func ExtractEventType(component, message string) (eventType, peer, remoteAddr string, ok bool) {
	// These two DIAM patterns don't follow the "<func>:<line> <rest>" shape
	// (confirmed against real data) — no line number in the message at all,
	// so messageLineRe never matches them. Gated on component so an
	// unrelated component's log line can't collide with this substring
	// match by coincidence.
	if component == "DIAM" {
		switch {
		case strings.Contains(message, "Dynamic peer with local IP"):
			return "DIAM_DYNAMIC_PEER", "", "", true
		case strings.Contains(message, "is down; reconnection in progress"):
			return "DIAM_PEER_DOWN", "", "", true
		}
	}

	m := messageLineRe.FindStringSubmatch(message)
	if m == nil {
		return "", "", "", false
	}
	function, rest := m[1], m[3]

	if p := peerRe.FindStringSubmatch(rest); p != nil {
		peer = p[1]
	}
	if r := remoteAddrRe.FindStringSubmatch(rest); r != nil {
		remoteAddr = r[1]
	}

	if et, known := knownFunctionEventTypes[function]; known {
		return et, peer, remoteAddr, true
	}

	// handle_io_conn covers several distinct outcomes (timeout, refused,
	// reset) — disambiguate on the message body rather than lumping them
	// into one code, since only "Connection timed out" was confirmed
	// against real data.
	if function == "handle_io_conn" {
		switch {
		case strings.Contains(rest, "timed out"):
			return "CONN_TIMEOUT", peer, remoteAddr, true
		case strings.Contains(rest, "refused"):
			return "CONN_REFUSED", peer, remoteAddr, true
		case strings.Contains(rest, "reset"):
			return "CONN_RESET", peer, remoteAddr, true
		default:
			return "CONN_ERROR", peer, remoteAddr, true
		}
	}

	// Unrecognized function: surface the function name itself as the type
	// instead of a generic "UNKNOWN" bucket, so new patterns are visible
	// (and rankable) in the report instead of disappearing into one catch-all.
	return function, peer, remoteAddr, true
}
