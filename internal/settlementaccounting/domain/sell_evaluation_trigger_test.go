package domain

import "testing"

func TestSellEvaluationTriggerOnlyAcceptsOccurrenceFormed(t *testing.T) {
	reason, err := NewOccurrenceReasonReference("booking")
	if err != nil {
		t.Fatalf("reason: %v", err)
	}
	registration, err := NewSellEvaluationTriggerRegistration(reason, SellEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	if registration.Moment().String() != "OCCURRENCE_FORMED" {
		t.Fatalf("moment = %s", registration.Moment())
	}
	if _, err := SellEvaluationTriggerMomentFromName("CYCLE_BATCH"); err == nil {
		t.Fatal("词表外的时点被收下")
	}
	if _, err := NewSellEvaluationTriggerRegistration(reason, SellEvaluationTriggerMomentInvalid); err == nil {
		t.Fatal("无效时点被登记")
	}
}
