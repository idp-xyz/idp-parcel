package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var chargeFormedAt = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

func estimatedCharge(t *testing.T) domain.CustomerCharge {
	t.Helper()
	charge, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:          mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		FeeItem:     mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:  mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-1"),
		Currency:    mustValue(t, domain.NewCurrencyCode, "CNY"),
		AmountMinor: 45600,
		Stage:       domain.ChargeEstimated,
		FormedAt:    chargeFormedAt,
	})
	if err != nil {
		t.Fatalf("form customer charge: %v", err)
	}
	return charge
}

// Covers: UC-SA-002「费用可以经历预估、暂估和确认」与结果契约「费用已确认：确认条件
// 已满足」——预估起步、确认带依据定格；直接以已确认起步不允许（确认是显式判断不是
// 初值）；已确认不再确认第二次；确认不改金额。
func TestAChargeWalksFromEstimateToConfirmation(t *testing.T) {
	charge := estimatedCharge(t)
	if charge.Stage() != domain.ChargeEstimated {
		t.Fatalf("stage = %q", charge.Stage())
	}
	if _, confirmed := charge.Confirmation(); confirmed {
		t.Fatal("预估费用凭空带了确认依据")
	}

	confirmed, err := charge.Confirm(
		mustValue(t, domain.NewConfirmationBasisReference, "DELIVERY_FINALIZED/final-1"),
		chargeFormedAt.Add(48*time.Hour),
	)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if confirmed.Stage() != domain.ChargeConfirmed {
		t.Fatalf("stage = %q", confirmed.Stage())
	}
	basis, present := confirmed.Confirmation()
	if !present || basis.String() != "DELIVERY_FINALIZED/final-1" {
		t.Fatalf("confirmation = %v present = %v", basis, present)
	}
	_, amount := confirmed.Amount()
	if amount != 45600 {
		t.Fatal("确认改了金额——金额变化只能走调整")
	}
	if charge.Stage() != domain.ChargeEstimated {
		t.Fatal("原费用被改写了")
	}

	if _, err := confirmed.Confirm(
		mustValue(t, domain.NewConfirmationBasisReference, "AGAIN"),
		chargeFormedAt.Add(72*time.Hour),
	); !errors.Is(err, domain.ErrChargeAlreadyFinal) {
		t.Fatalf("err = %v; 确认确了两次", err)
	}

	direct := domain.CustomerChargeSpec{
		ID:          mustValue(t, domain.NewCustomerChargeID, "charge-2"),
		FeeItem:     mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:  mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-2"),
		Currency:    mustValue(t, domain.NewCurrencyCode, "CNY"),
		AmountMinor: 100,
		Stage:       domain.ChargeConfirmed,
		FormedAt:    chargeFormedAt,
	}
	if _, err := domain.FormCustomerCharge(direct); !errors.Is(err, domain.ErrInvalidCustomerCharge) {
		t.Fatalf("err = %v; 直接以已确认起步被收下了", err)
	}
}

// Covers: `AT-SA-056`「人工提交『冲销』但未说明语义——拒绝无语义调整；本用例只接受
// 计价纠错或商业让利」——种类封闭二值（赔付/索赔退款/追偿/供应商贷项没有格），证据或
// 授权必备，方向必备；调整是新对象不改写原费用。
func TestAdjustmentsDemandTheirSemanticKind(t *testing.T) {
	adjustment, err := domain.FormChargeAdjustment(domain.ChargeAdjustmentSpec{
		ID:          mustValue(t, domain.NewChargeAdjustmentID, "adjustment-1"),
		Charge:      mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		Kind:        domain.CommercialConcession,
		Direction:   domain.AdjustmentCredit,
		Authority:   mustValue(t, domain.NewAdjustmentAuthorityReference, "CONCESSION-APPROVAL/9"),
		Currency:    mustValue(t, domain.NewCurrencyCode, "CNY"),
		AmountMinor: 5600,
		FormedAt:    chargeFormedAt.Add(72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form charge adjustment: %v", err)
	}
	if adjustment.Kind() != domain.CommercialConcession || adjustment.Direction() != domain.AdjustmentCredit {
		t.Fatalf("adjustment = %#v", adjustment)
	}

	semanticless := domain.ChargeAdjustmentSpec{
		ID:          mustValue(t, domain.NewChargeAdjustmentID, "adjustment-2"),
		Charge:      mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		Kind:        domain.AdjustmentKindInvalid,
		Direction:   domain.AdjustmentCredit,
		Authority:   mustValue(t, domain.NewAdjustmentAuthorityReference, "MANUAL/void"),
		Currency:    mustValue(t, domain.NewCurrencyCode, "CNY"),
		AmountMinor: 100,
		FormedAt:    chargeFormedAt,
	}
	if _, err := domain.FormChargeAdjustment(semanticless); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 无语义的冲销被收下了", err)
	}

	unauthorized := semanticless
	unauthorized.Kind = domain.PricingCorrection
	unauthorized.Authority = domain.AdjustmentAuthorityReference{}
	if _, err := domain.FormChargeAdjustment(unauthorized); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 没有证据的纠错被收下了", err)
	}
}
