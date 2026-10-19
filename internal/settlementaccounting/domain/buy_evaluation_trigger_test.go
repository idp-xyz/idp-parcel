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

// Covers: ADR-0208——费用项目是这一行的第三格，可缺；登了就随行比对，换一个或补登都不是同一份登记。
func TestBuyEvaluationTriggerCarriesAnOptionalFeeItem(t *testing.T) {
	reason, err := domain.NewOccurrenceReasonReference("ACTUAL_FULFILLMENT")
	if err != nil {
		t.Fatal(err)
	}
	bare, err := domain.NewBuyEvaluationTriggerRegistration(reason, domain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatal(err)
	}
	if _, registered := bare.FeeItem(); registered {
		t.Fatal("没登的费用项目被答成登了")
	}
	trunk, err := domain.NewFeeItemReference("SYN-FEE-ITEM-TRUNK-COST-01")
	if err != nil {
		t.Fatal(err)
	}
	withTrunk, err := bare.WithFeeItem(trunk)
	if err != nil {
		t.Fatal(err)
	}
	if feeItem, registered := withTrunk.FeeItem(); !registered || feeItem != trunk {
		t.Fatalf("费用项目 = %q registered=%v", feeItem, registered)
	}
	other, err := domain.NewFeeItemReference("SYN-FEE-ITEM-OTHER")
	if err != nil {
		t.Fatal(err)
	}
	withOther, err := bare.WithFeeItem(other)
	if err != nil {
		t.Fatal(err)
	}
	if !withTrunk.SameRegistration(withTrunk) || withTrunk.SameRegistration(withOther) || withTrunk.SameRegistration(bare) {
		t.Fatal("费用项目不同的两行被当成同一份登记")
	}
	if _, err := bare.WithFeeItem(domain.FeeItemReference{}); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空费用项目 err = %v", err)
	}
}
