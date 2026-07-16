package domain

import "testing"

func TestClassifySeverity(t *testing.T) {
	cases := []struct {
		name string
		e    Event
		want Severity
	}{
		{
			name: "ASP down is Critical even though raw severity is SYS",
			e:    Event{EventType: "ASP_DOWN", RawSeverity: "SYS"},
			want: SeverityCritical,
		},
		{
			name: "connection timeout is Major",
			e:    Event{EventType: "CONN_TIMEOUT", RawSeverity: "ERR"},
			want: SeverityMajor,
		},
		{
			name: "ASP up (recovery) is Info even though raw severity is SYS",
			e:    Event{EventType: "ASP_UP", RawSeverity: "SYS"},
			want: SeverityInfo,
		},
		{
			name: "unmatched event type falls back to raw ERR -> Major",
			e:    Event{EventType: "some_new_func", RawSeverity: "ERR"},
			want: SeverityMajor,
		},
		{
			name: "raw WRN with no specific rule -> Minor",
			e:    Event{EventType: "some_new_func", RawSeverity: "WRN"},
			want: SeverityMinor,
		},
		{
			name: "raw DBG -> Info",
			e:    Event{EventType: "some_new_func", RawSeverity: "DBG"},
			want: SeverityInfo,
		},
		{
			name: "nothing matches -> Unknown, not silently defaulted",
			e:    Event{EventType: "some_new_func", RawSeverity: "WEIRD"},
			want: SeverityUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifySeverity(tc.e, DefaultSeverityRules)
			if got != tc.want {
				t.Errorf("ClassifySeverity() = %v, want %v", got, tc.want)
			}
		})
	}
}
