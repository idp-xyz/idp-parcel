package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestBuyEvaluationTriggerOnlyAcceptsOccurrenceFormed(t *testing.T) {
	reason, err := domain.NewOccurrenceReasonReference("BOOKING")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewBuyEvaluationTriggerRegistration(reason, domain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Moment().String() != "OCCURRENCE_FORMED" || registration.Reason().String() != "BOOKING" {
		t.Fatalf("登记走样：%s %s", registration.Moment(), registration.Reason())
	}
	if _, err := domain.BuyEvaluationTriggerMomentFromName("SETTLEMENT_PERIOD"); !errors.Is(err, domain.ErrInvalidBuyEvaluationTrigger) {
		t.Fatalf("结算周期被收成触发时点：%v", err)
	}
	if _, err := domain.NewBuyEvaluationTriggerRegistration(reason, domain.BuyEvaluationTriggerMomentInvalid); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空时点 err = %v", err)
	}
}
