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
		ID:                 mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-1"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      45600,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    45600,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
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
	_, amount := confirmed.SettlementAmount()
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
		ID:                 mustValue(t, domain.NewCustomerChargeID, "charge-2"),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-2"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      100,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    100,
		Stage:              domain.ChargeConfirmed,
		FormedAt:           chargeFormedAt,
	}
	if _, err := domain.FormCustomerCharge(direct); !errors.Is(err, domain.ErrInvalidCustomerCharge) {
		t.Fatalf("err = %v; 直接以已确认起步被收下了", err)
	}
}

// Covers: SA CONTEXT「赔付、追偿、税费、币种与法人」——「每条费用分别保存原币金额、
// 合同结算币金额及换算依据」「原币金额、合同结算币金额和换算依据是一条费用从同一个
// 评价采用来的一组」「原币与合同结算币相同时两个金额必须相等」。客户费用受三件组
// 约束由票 supplier-expected-cost-correction/04 裁定，此处是它的构造面。
func TestAChargeCarriesTheCurrencyTripleFromItsEvaluation(t *testing.T) {
	cross, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 mustValue(t, domain.NewCustomerChargeID, "charge-cross"),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-3"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      4200,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    30240,
		Conversion:         mustValue(t, domain.NewConversionStepReference, "conversion-step-1"),
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
	})
	if err != nil {
		t.Fatalf("form cross-currency charge: %v", err)
	}
	originalCurrency, originalMinor := cross.OriginalAmount()
	settlementCurrency, settlementMinor := cross.SettlementAmount()
	if originalCurrency.String() != "USD" || originalMinor != 4200 ||
		settlementCurrency.String() != "CNY" || settlementMinor != 30240 {
		t.Fatalf("三件组读回变形：%s %d / %s %d",
			originalCurrency, originalMinor, settlementCurrency, settlementMinor)
	}
	if conversion, present := cross.Conversion(); !present || conversion.String() != "conversion-step-1" {
		t.Fatal("跨币种费用丢了换算依据")
	}

	missingConversion := domain.CustomerChargeSpec{
		ID:                 mustValue(t, domain.NewCustomerChargeID, "charge-cross-2"),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-3"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      4200,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    30240,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
	}
	if _, err := domain.FormCustomerCharge(missingConversion); !errors.Is(err, domain.ErrConversionStepMissing) {
		t.Fatalf("err = %v; 没有换算步骤的跨币种费用被收下了", err)
	}

	unequalSameCurrency := domain.CustomerChargeSpec{
		ID:                 mustValue(t, domain.NewCustomerChargeID, "charge-same-2"),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-4"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      45600,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    45700,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
	}
	if _, err := domain.FormCustomerCharge(unequalSameCurrency); !errors.Is(err, domain.ErrInvalidCustomerCharge) {
		t.Fatalf("err = %v; 同币种两额不等被收下了", err)
	}

	if _, present := estimatedCharge(t).Conversion(); present {
		t.Fatal("同币种费用凭空带了换算依据")
	}
}

// Covers: `AT-SA-056`「人工提交『冲销』但未说明语义——拒绝无语义调整；本用例只接受
// 计价纠错或商业让利」——种类封闭二值（赔付/索赔退款/追偿/供应商贷项没有格），依据
// 按种类各占一格，方向必备；调整是新对象不改写原费用。
func TestAdjustmentsDemandTheirSemanticKind(t *testing.T) {
	adjustment, err := domain.FormChargeAdjustment(domain.ChargeAdjustmentSpec{
		ID:                 mustValue(t, domain.NewChargeAdjustmentID, "adjustment-1"),
		Charge:             mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		Kind:               domain.CommercialConcession,
		Direction:          domain.AdjustmentCredit,
		Authorization:      mustValue(t, domain.NewCommercialAuthorizationReference, "CONCESSION-APPROVAL/9"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      5600,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    5600,
		FormedAt:           chargeFormedAt.Add(72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form charge adjustment: %v", err)
	}
	if adjustment.Kind() != domain.CommercialConcession || adjustment.Direction() != domain.AdjustmentCredit {
		t.Fatalf("adjustment = %#v", adjustment)
	}
	if _, ok := adjustment.Authorization(); !ok {
		t.Fatal("让利类交不回商业授权引用")
	}
	if _, ok := adjustment.Evaluation(); ok {
		t.Fatal("让利类不该交回评价引用")
	}

	semanticless := domain.ChargeAdjustmentSpec{
		ID:                 mustValue(t, domain.NewChargeAdjustmentID, "adjustment-2"),
		Charge:             mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		Kind:               domain.AdjustmentKindInvalid,
		Direction:          domain.AdjustmentCredit,
		Authorization:      mustValue(t, domain.NewCommercialAuthorizationReference, "MANUAL/void"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      100,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    100,
		FormedAt:           chargeFormedAt,
	}
	if _, err := domain.FormChargeAdjustment(semanticless); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 无语义的冲销被收下了", err)
	}

	unauthorized := semanticless
	unauthorized.Kind = domain.PricingCorrection
	unauthorized.Authorization = domain.CommercialAuthorizationReference{}
	if _, err := domain.FormChargeAdjustment(unauthorized); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 没有证据的纠错被收下了", err)
	}
}

// Covers: 票 supplier-expected-cost-correction/05 第二、三问裁定——依据按种类分格且
// 有此无彼：纠错挂评价、让利挂授权，填错格与两格齐填一样拒。同包 CustomerCharge 把
// evaluation 与 confirmation 分开建，相邻类型对「依据」的建模粒度从此一致。
func TestAdjustmentBasesAreSlottedByKind(t *testing.T) {
	correction := domain.ChargeAdjustmentSpec{
		ID:                 mustValue(t, domain.NewChargeAdjustmentID, "adjustment-slot-1"),
		Charge:             mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		Kind:               domain.PricingCorrection,
		Direction:          domain.AdjustmentDebit,
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "sell-eval/re-2"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "CNY"),
		OriginalMinor:      900,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    900,
		FormedAt:           chargeFormedAt.Add(time.Hour),
	}
	adjustment, err := domain.FormChargeAdjustment(correction)
	if err != nil {
		t.Fatalf("form correction: %v", err)
	}
	if _, ok := adjustment.Evaluation(); !ok {
		t.Fatal("纠错类交不回评价引用")
	}
	if _, ok := adjustment.Authorization(); ok {
		t.Fatal("纠错类不该交回商业授权引用")
	}

	wrongSlot := correction
	wrongSlot.Evaluation = domain.SellEvaluationReference{}
	wrongSlot.Authorization = mustValue(t, domain.NewCommercialAuthorizationReference, "CONCESSION-APPROVAL/9")
	if _, err := domain.FormChargeAdjustment(wrongSlot); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 纠错挂着授权被收下了——Kind 之外没有第二个维分辨依据", err)
	}

	bothSlots := correction
	bothSlots.Authorization = mustValue(t, domain.NewCommercialAuthorizationReference, "CONCESSION-APPROVAL/9")
	if _, err := domain.FormChargeAdjustment(bothSlots); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 两格齐填被收下了", err)
	}

	concessionWithEvaluation := correction
	concessionWithEvaluation.Kind = domain.CommercialConcession
	if _, err := domain.FormChargeAdjustment(concessionWithEvaluation); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 让利挂着评价被收下了——让利不是重评价", err)
	}
}

// Covers: SA CONTEXT「费用调整同受三件组约束」（票 05 第一、四问裁定）——跨币种缺
// 换算依据即拒（不自行取汇率补算），同币种两额不等即拒（没有换算却造出第二个数）；
// 跨币种成形后原币对、结算币对与换算依据都读得回，结算币身份由类型自己说出。
func TestAdjustmentCurrencyTripleMirrorsTheChargeGate(t *testing.T) {
	cross := domain.ChargeAdjustmentSpec{
		ID:                 mustValue(t, domain.NewChargeAdjustmentID, "adjustment-triple-1"),
		Charge:             mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		Kind:               domain.PricingCorrection,
		Direction:          domain.AdjustmentDebit,
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "sell-eval/re-3"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      1000,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    7200,
		FormedAt:           chargeFormedAt.Add(time.Hour),
	}
	if _, err := domain.FormChargeAdjustment(cross); !errors.Is(err, domain.ErrConversionStepMissing) {
		t.Fatalf("err = %v; 跨币种没有换算依据被收下了", err)
	}

	cross.Conversion = mustValue(t, domain.NewConversionStepReference, "conversion/usd-cny/2026-08-21")
	adjustment, err := domain.FormChargeAdjustment(cross)
	if err != nil {
		t.Fatalf("form cross-currency adjustment: %v", err)
	}
	if currency, amount := adjustment.OriginalAmount(); currency.String() != "USD" || amount != 1000 {
		t.Fatalf("original = %s/%d, want USD/1000", currency, amount)
	}
	if currency, amount := adjustment.SettlementAmount(); currency.String() != "CNY" || amount != 7200 {
		t.Fatalf("settlement = %s/%d, want CNY/7200", currency, amount)
	}
	if _, ok := adjustment.Conversion(); !ok {
		t.Fatal("跨币种调整交不回换算依据")
	}

	disagreeing := cross
	disagreeing.SettlementCurrency = disagreeing.OriginalCurrency
	disagreeing.Conversion = domain.ConversionStepReference{}
	if _, err := domain.FormChargeAdjustment(disagreeing); !errors.Is(err, domain.ErrInvalidAdjustment) {
		t.Fatalf("err = %v; 同币种两额不等被收下了", err)
	}
}
