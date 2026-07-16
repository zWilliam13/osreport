package opensearch

import (
	"encoding/json"
	"fmt"
	"time"

	"osreport/internal/domain"
)

// rawDoc mirrors only the fields the report actually needs from the real
// index-athonet documents. The full mapping has ~979 fields (a Fluent Bit
// re-wrapping artifact nests "record" inside itself several levels deep for
// some documents) — decoding the whole thing would be both wasteful and
// fragile. json.Unmarshal ignores fields not listed here, which is exactly
// what we want.
type rawDoc struct {
	ALERTComponent string `json:"ALERT_COMPONENT"`
	ALERTSeverity  string `json:"ALERT_SEVERITY"`
	Record         struct {
		TimeEvent string `json:"time_event"`
		HOSTNAME  string `json:"HOSTNAME"`
		MESSAGE   struct {
			Raw string `json:"raw"`
			Msg string `json:"msg"`
		} `json:"MESSAGE"`
	} `json:"record"`
}

// MapHit converts one OpenSearch hit's _source into a domain.Event. It
// returns an error for a hit that can't be interpreted at all (bad JSON,
// unparseable timestamp) — the caller decides whether to skip and continue
// or abort, this function never partially fabricates a timestamp.
func MapHit(id string, source json.RawMessage) (domain.Event, error) {
	var raw rawDoc
	if err := json.Unmarshal(source, &raw); err != nil {
		return domain.Event{}, fmt.Errorf("unmarshal hit %s: %w", id, err)
	}

	ts, err := time.Parse(time.RFC3339, raw.Record.TimeEvent)
	if err != nil {
		return domain.Event{}, fmt.Errorf("parse record.time_event on hit %s: %w", id, err)
	}

	message := raw.Record.MESSAGE.Raw
	if message == "" {
		message = raw.Record.MESSAGE.Msg
	}

	e := domain.Event{
		ID:          id,
		Timestamp:   ts,
		Component:   raw.ALERTComponent,
		RawSeverity: raw.ALERTSeverity,
		Host:        raw.Record.HOSTNAME,
		Message:     message,
	}

	if eventType, peer, remoteAddr, ok := domain.ExtractEventType(raw.ALERTComponent, message); ok {
		e.EventType = eventType
		e.Peer = peer
		e.RemoteAddr = remoteAddr
	}

	return e, nil
}
