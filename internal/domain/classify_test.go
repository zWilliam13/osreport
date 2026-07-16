package domain

import "testing"

func TestExtractEventType(t *testing.T) {
	cases := []struct {
		name           string
		component      string
		message        string
		wantType       string
		wantPeer       string
		wantRemoteAddr string
		wantOK         bool
	}{
		{
			name:           "real ASP down indication",
			component:      "M3UA",
			message:        "SYS M3UA    aspsm_down_ind:1658 DOWN indication for peer GSMSC2-0_ASP",
			wantType:       "ASP_DOWN",
			wantPeer:       "GSMSC2-0_ASP",
			wantRemoteAddr: "",
			wantOK:         true,
		},
		{
			name:           "real connection timeout",
			component:      "M3UA",
			message:        "ERR M3UA    handle_io_conn:363 Connection to 181.176.252.249:3036 failed: Connection timed out",
			wantType:       "CONN_TIMEOUT",
			wantPeer:       "",
			wantRemoteAddr: "181.176.252.249:3036",
			wantOK:         true,
		},
		{
			name:           "connection refused variant",
			component:      "M3UA",
			message:        "ERR M3UA    handle_io_conn:363 Connection to 10.0.0.1:3036 failed: Connection refused",
			wantType:       "CONN_REFUSED",
			wantRemoteAddr: "10.0.0.1:3036",
			wantOK:         true,
		},
		{
			name:      "unrecognized function falls back to function name",
			component: "M3UA",
			message:   "INF M3UA    some_new_func:42 something happened",
			wantType:  "some_new_func",
			wantOK:    true,
		},
		{
			name:      "malformed message does not match shape",
			component: "M3UA",
			message:   "totally unstructured text with no function marker",
			wantOK:    false,
		},
		{
			name:      "real TCAP CCO exhaustion",
			component: "TCAP",
			message:   "ERR TCAP    tcap_cco_invoke_ind:837 Unable to reserve CCO state machine",
			wantType:  "TCAP_CCO_EXHAUSTED",
			wantOK:    true,
		},
		{
			name:      "real MAP DSM 16015 stuck slot",
			component: "MAP",
			message:   "ERR MAP     map_invoke_ind:525 DSM 16015 not initilialized",
			wantType:  "MAP_DSM_16015_STUCK",
			wantOK:    true,
		},
		{
			name:      "real MAP IDH linking failure",
			component: "MAP",
			message:   "ERR MAP     map_req_remap_ids:294 IDH linking failure",
			wantType:  "MAP_IDH_LINK_FAIL",
			wantOK:    true,
		},
		{
			name:      "real HSS unknown IMSI",
			component: "HSS_IMS",
			message:   "ERR HSS_IMS hss_map_AuthInfo_ind:325 Unknown IMSI 716191000011483f",
			wantType:  "HSS_UNKNOWN_IMSI",
			wantOK:    true,
		},
		{
			name:      "real S6a unknown IMSI in AIR",
			component: "S6a",
			message:   "WRN S6a     S6_manage_AIR:2742 AIR for Unknown IMSI [716191000019397]",
			wantType:  "S6A_UNKNOWN_IMSI",
			wantOK:    true,
		},
		{
			name:      "real DIAM dynamic peer notice (no func:line shape)",
			component: "DIAM",
			message:   "WRN DIAM    Dynamic peer with local IP 172.26.3.34 and remote IP 172.26.3.40:3868",
			wantType:  "DIAM_DYNAMIC_PEER",
			wantOK:    true,
		},
		{
			name:      "real DIAM peer down notice (no func:line shape)",
			component: "DIAM",
			message:   "WRN DIAM    Connection to [mcptt-server.mcx.pe] is down; reconnection in progress.",
			wantType:  "DIAM_PEER_DOWN",
			wantOK:    true,
		},
		{
			name:      "DIAM-shaped text from a different component does not misfire",
			component: "PGW",
			message:   "Dynamic peer with local IP 172.26.3.34 and remote IP 172.26.3.40:3868",
			wantOK:    false,
		},
		{
			name:      "real MAP error indication unhandled",
			component: "MAP",
			message:   "ERR MAP     map_error_ind:752 DSM 39719 has no ERROR INDICATION handling",
			wantType:  "MAP_ERROR_IND_UNHANDLED",
			wantOK:    true,
		},
		{
			name:      "real MAP DSM dummy link",
			component: "MAP",
			message:   "ERR MAP     map_dsm_reserve_dummy:377 Linking dummy DSM 39650 with lower DHA 39843",
			wantType:  "MAP_DSM_DUMMY_LINK",
			wantOK:    true,
		},
		{
			name:      "message with an embedded newline still matches to the real end of string",
			component: "MAP",
			message:   "ERR MAP     map_invoke_ind:525 DSM 16015 not initilialized\nextra trailing detail on a second line",
			wantType:  "MAP_DSM_16015_STUCK",
			wantOK:    true,
		},
		{
			name:      "real M3UA AS unreachable (DUNA)",
			component: "M3UA",
			message:   "WRN M3UA    as_set_duna:600 AS [MOVISTAR_LV] DUNA for [14663/0], set to unreachable",
			wantType:  "AS_UNREACHABLE",
			wantOK:    true,
		},
		{
			name:      "real M3UA AS reachable again (DAVA)",
			component: "M3UA",
			message:   "WRN M3UA    as_set_dava:559 AS [MOVISTAR_LV] DAVA for [14663/0] again available",
			wantType:  "AS_REACHABLE",
			wantOK:    true,
		},
		{
			name:      "real M3UA AS active to down",
			component: "M3UA",
			message:   "WRN M3UA    as_active_to_down:771 AS ENTEL-AS02 transition from ACTIVE to DOWN",
			wantType:  "AS_STATE_DOWN",
			wantOK:    true,
		},
		{
			name:      "real M3UA AS down to inactive",
			component: "M3UA",
			message:   "WRN M3UA    as_down_to_inactive:618 AS [ENTEL-AS02] transition from DOWN to INACTIVE",
			wantType:  "AS_STATE_RECOVERING",
			wantOK:    true,
		},
		{
			name:      "real M3UA AS inactive to active",
			component: "M3UA",
			message:   "WRN M3UA    as_inactive_to_active:631 AS ENTEL-AS02 transition from INACTIVE to ACTIVE",
			wantType:  "AS_STATE_ACTIVE",
			wantOK:    true,
		},
		{
			name:      "real M3UA peer IO error",
			component: "M3UA",
			message:   "ERR M3UA    handle_io_peer_err:255 socket 86 io-id 132924 error Connection timed out (110)",
			wantType:  "PEER_IO_ERROR",
			wantOK:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotPeer, gotAddr, ok := ExtractEventType(tc.component, tc.message)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if gotType != tc.wantType {
				t.Errorf("eventType = %q, want %q", gotType, tc.wantType)
			}
			if gotPeer != tc.wantPeer {
				t.Errorf("peer = %q, want %q", gotPeer, tc.wantPeer)
			}
			if gotAddr != tc.wantRemoteAddr {
				t.Errorf("remoteAddr = %q, want %q", gotAddr, tc.wantRemoteAddr)
			}
		})
	}
}
