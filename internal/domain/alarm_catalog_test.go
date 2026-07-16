package domain

import "testing"

func TestDescribeAlarm_KnownEventType(t *testing.T) {
	info := DescribeAlarm("ASP_DOWN")
	if info.Alarma == "" {
		t.Error("Alarma is empty for a cataloged EventType")
	}
	if info.Descripcion == "" {
		t.Error("Descripcion is empty for a cataloged EventType")
	}
}

func TestDescribeAlarm_UncatalogedEventTypeFallsBackToItsOwnName(t *testing.T) {
	info := DescribeAlarm("some_never_seen_function")
	if info.Alarma != "some_never_seen_function" {
		t.Errorf("Alarma = %q, want the raw EventType surfaced as-is", info.Alarma)
	}
	if info.Descripcion == "" {
		t.Error("Descripcion is empty for an uncataloged EventType")
	}
}

// TestDescribeAlarm_EmptyEventTypeDoesNotProduceBlankAlarma covers an
// event whose message matched no known shape at all (e.g. a document with
// no MESSAGE.raw/msg body) — EventType stays "" in that case. Falling
// through to the generic uncataloged case would render an empty Alarma
// cell in the report, which looks broken rather than informative.
func TestDescribeAlarm_EmptyEventTypeDoesNotProduceBlankAlarma(t *testing.T) {
	info := DescribeAlarm("")
	if info.Alarma == "" {
		t.Error("Alarma is blank for eventType \"\" — would render as an empty cell in the report")
	}
	if info.Descripcion == "" {
		t.Error("Descripcion is empty for eventType \"\"")
	}
}
