package domain

import "time"

// Severity is the report's own severity scale, distinct from the raw
// ALERT_SEVERITY value coming from OpenSearch (which is a syslog priority
// name like DBG/INF/WRN/ERR/SYS, not a business severity).
type Severity int

const (
	SeverityUnknown Severity = iota
	SeverityInfo
	SeverityMinor
	SeverityMajor
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "Info"
	case SeverityMinor:
		return "Minor"
	case SeverityMajor:
		return "Major"
	case SeverityCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// Event is the domain representation of one OpenSearch hit from
// index-athonet, after mapping out of the raw document shape.
type Event struct {
	ID          string
	Timestamp   time.Time
	Component   string // ALERT_COMPONENT: M3UA, DIAM, PCRF, GTP, ...
	RawSeverity string // ALERT_SEVERITY: DBG, INF, WRN, ERR, SYS
	Severity    Severity
	Host        string // record.HOSTNAME
	Message     string // record.MESSAGE.raw
	EventType   string // derived from Message via pattern extraction (e.g. ASP_DOWN, CONN_TIMEOUT)
	Peer        string // extracted peer name, when present ("for peer X")
	RemoteAddr  string // extracted ip:port, when present ("Connection to X failed")
}
