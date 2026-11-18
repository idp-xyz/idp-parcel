package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var adjustmentRecordedAt = time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)

type chargeAdjustmentStoreDouble struct {
	records  map[string]ports.ChargeAdjustmentRecord
	saveErr  error
	findErr  error
	lostRead bool
	saves    int
}

func newChargeAdjustmentStore() *chargeAdjustmentStoreDouble {
	return &chargeAdjustmentStoreDouble{records: map[string]ports.ChargeAdjustmentRecord{}}
}

func (double *chargeAdjustmentStoreDouble) FindByKey(
	_ context.Context,
	key ports.ChargeAdjustmentKey,
) (ports.ChargeAdjustmentRecord, bool, error) {
	if double.findErr != nil {
		return ports.ChargeAdjustmentRecord{}, false, double.findErr
	}
	if double.lostRead {
		return ports.ChargeAdjustmentRecord{}, false, nil
	}
	record, found := double.records[key.Adjustment.String()]
	return record, found, nil
}

func (double *chargeAdjustmentStoreDouble) ListByCharge(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerChargeID,
) ([]ports.ChargeAdjustmentRecord, error) {
	listed := []ports.ChargeAdjustmentRecord{}
	for _, record := range double.records {
		if record.Adjustment.Charge() == charge {
			listed = append(listed, record)
		}
	}
	return listed, nil
}

func (double *chargeAdjustmentStoreDouble) Save(
	_ context.Context,
	record ports.ChargeAdjustmentRecord,
) (ports.ChargeAdjustmentSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.ChargeAdjustmentSaveOutcomeInvalid, double.saveErr
	}
	if _, found := double.records[record.Key.Adjustment.String()]; found {
		return ports.ChargeAdjustmentAlreadyRecorded, nil
	}
	double.records[record.Key.Adjustment.String()] = record
	return ports.ChargeAdjustmentSaved, nil
}

type adjustmentFixture struct {
	charges     *chargeStoreDouble
	adjustments *chargeAdjustmentStoreDouble
	handler     *application.RecordChargeAdjustmentHandler
}

func newAdjustmentFixture(t *testing.T) *adjustmentFixture {
	t.Helper()
	fixture := &adjustmentFixture{
		charges:     newChargeStore(),
		adjustments: newChargeAdjustmentStore(),
	}
	fixture.handler = application.NewRecordChargeAdjustmentHandler(application.RecordChargeAdjustmentDeps{
		Charges:     fixture.charges,
		Adjustments: fixture.adjustments,
		Clock:       confirmClock{at: adjustmentRecordedAt},
	})
	confirmed, err := provisionalCharge(t).Confirm(
		registeredFacts(t),
		billValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-1"),
		chargeConfirmedAt)
	if err != nil {
		t.Fatalf("confirm charge: %v", err)
	}
	fixture.charges.charges["charge-1"] = confirmed
	return fixture
}

func correctionCommand(t *testing.T, id string) application.RecordChargeAdjustmentCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.RecordChargeAdjustmentCommand{
		TenantID:           tenant,
		AdjustmentID:       id,
		ChargeID:           "charge-1",
		Kind:               domain.PricingCorrection,
		Direction:          domain.AdjustmentDebit,
		Evaluation:         "sell-eval/re-1",
		OriginalCurrency:   "USD",
		OriginalMinor:      900,
		SettlementCurrency: "USD",
		SettlementMinor:    900,
	}
}

func TestSettleReweighRecordsTheDeltaAndRefusesACreditPastZero(t *testing.T) {
	fixture := newAdjustmentFixture(t)
	tenant := billValue(t, domain.NewTenantID, "tenant-1")
	none, err := fixture.handler.SettleReweigh(context.Background(), application.ReweighDifferenceCommand{
		TenantID: tenant, ChargeID: "charge-1", Evaluation: "sell-eval-same",
		OriginalCurrency: "USD", OriginalMinor: 12000, SettlementCurrency: "USD", SettlementMinor: 12000,
	})
	if err != nil || none != application.ReweighDifferenceNone || fixture.adjustments.saves != 0 {
		t.Fatalf("差额为零 %s saves=%d err=%v", none, fixture.adjustments.saves, err)
	}
	recorded, err := fixture.handler.SettleReweigh(context.Background(), application.ReweighDifferenceCommand{
		TenantID: tenant, ChargeID: "charge-1", Evaluation: "sell-eval-up",
		OriginalCurrency: "USD", OriginalMinor: 15000, SettlementCurrency: "USD", SettlementMinor: 15000,
	})
	if err != nil || recorded != application.ReweighDifferenceRecorded || fixture.adjustments.saves != 1 {
		t.Fatalf("借项 %s saves=%d err=%v", recorded, fixture.adjustments.saves, err)
	}
	exceeds, err := fixture.handler.SettleReweigh(context.Background(), application.ReweighDifferenceCommand{
		TenantID: tenant, ChargeID: "charge-1", Evaluation: "sell-eval-down",
		OriginalCurrency: "USD", OriginalMinor: -100, SettlementCurrency: "USD", SettlementMinor: -100,
	})
	if err != nil || exceeds != application.ReweighDifferenceExceedsCharge {
		t.Fatalf("越过零 %s err=%v", exceeds, err)
	}
}

// newCrossCurrencyAdjustmentFixture 把 charge-1 换成一笔跨币种已确认费用：原币 EUR 10000、结算币 USD 12000，带换算依据。
func newCrossCurrencyAdjustmentFixture(t *testing.T) *adjustmentFixture {
	t.Helper()
	fixture := newAdjustmentFixture(t)
	provisional, err := domain.FormCustomerCharge(domain.CustomerChargeSpec{
		ID:                 billValue(t, domain.NewCustomerChargeID, "charge-1"),
		FeeItem:            billValue(t, domain.NewFeeItemReference, "fee-freight"),
		Evaluation:         billValue(t, domain.NewSellEvaluationReference, "sell-evaluation-1"),
		OriginalCurrency:   billValue(t, domain.NewCurrencyCode, "EUR"),
		OriginalMinor:      10000,
		SettlementCurrency: billValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    12000,
		Conversion:         billValue(t, domain.NewConversionStepReference, "fx/syn/step-0"),
		Stage:              domain.ChargeProvisional,
		FormedAt:           chargeFormedAt,
	})
	if err != nil {
		t.Fatalf("form cross currency charge: %v", err)
	}
	confirmed, err := provisional.Confirm(
		registeredFacts(t),
		billValue(t, domain.NewConfirmationBasisReference, "delivery-confirmed-1"),
		chargeConfirmedAt)
	if err != nil {
		t.Fatalf("confirm cross currency charge: %v", err)
	}
	fixture.charges.charges["charge-1"] = confirmed
	return fixture
}

// settleReweighAs 用一份结算币种为 USD 的新评价对 charge-1 结算差额。
func settleReweighAs(
	t *testing.T,
	fixture *adjustmentFixture,
	evaluation, originalCurrency string,
	originalMinor, settlementMinor int64,
	conversion string,
) application.ReweighDifferenceOutcome {
	t.Helper()
	outcome, err := fixture.handler.SettleReweigh(context.Background(), application.ReweighDifferenceCommand{
		TenantID: billValue(t, domain.NewTenantID, "tenant-1"), ChargeID: "charge-1", Evaluation: evaluation,
		OriginalCurrency: originalCurrency, OriginalMinor: originalMinor,
		SettlementCurrency: "USD", SettlementMinor: settlementMinor, Conversion: conversion,
	})
	if err != nil {
		t.Fatalf("重估 %s：%v", evaluation, err)
	}
	return outcome
}

// requireReweighAdjustment 断言这份评价记下的那一笔调整的方向、两格金额与换算依据；conversion 为空即不带换算依据。
func requireReweighAdjustment(
	t *testing.T,
	fixture *adjustmentFixture,
	evaluation string,
	direction domain.AdjustmentDirection,
	originalCurrency string,
	originalMinor, settlementMinor int64,
	conversion string,
) {
	t.Helper()
	record, found := fixture.adjustments.records["REWEIGH-"+evaluation]
	if !found {
		t.Fatalf("册上没有 REWEIGH-%s", evaluation)
	}
	adjustment := record.Adjustment
	gotOriginalCurrency, gotOriginalMinor := adjustment.OriginalAmount()
	gotSettlementCurrency, gotSettlementMinor := adjustment.SettlementAmount()
	gotConversion, hasConversion := adjustment.Conversion()
	if adjustment.Direction() != direction ||
		gotOriginalCurrency.String() != originalCurrency || gotOriginalMinor != originalMinor ||
		gotSettlementCurrency.String() != "USD" || gotSettlementMinor != settlementMinor ||
		gotConversion.String() != conversion || hasConversion != (conversion != "") {
		t.Fatalf("调整 = %s 原币 %s %d 结算币 %s %d 换算依据 %q；want %s 原币 %s %d 结算币 USD %d 换算依据 %q",
			adjustment.Direction(), gotOriginalCurrency, gotOriginalMinor, gotSettlementCurrency, gotSettlementMinor, gotConversion,
			direction, originalCurrency, originalMinor, settlementMinor, conversion)
	}
}

// Covers: 差额按结算币判。新评价的原币种与费用的原币种不同，两边原币金额不能相减：这一笔按结算币记——原币种取结算币种、
// 两额相等、不带换算依据，结算币金额是结算币差额。
func TestSettleReweighRecordsBySettlementCurrencyWhenTheEvaluationOriginalCurrencyDiffers(t *testing.T) {
	t.Run("跨币种费用", func(t *testing.T) {
		fixture := newCrossCurrencyAdjustmentFixture(t)
		if outcome := settleReweighAs(t, fixture, "sell-eval-gbp", "GBP", 9000, 15000, "fx/syn/gbp"); outcome != application.ReweighDifferenceRecorded {
			t.Fatalf("答 %s，want ADJUSTMENT_RECORDED", outcome)
		}
		requireReweighAdjustment(t, fixture, "sell-eval-gbp", domain.AdjustmentDebit, "USD", 3000, 3000, "")
	})
	t.Run("同币种费用", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		if outcome := settleReweighAs(t, fixture, "sell-eval-eur", "EUR", 13000, 15000, "fx/syn/eur"); outcome != application.ReweighDifferenceRecorded {
			t.Fatalf("答 %s，want ADJUSTMENT_RECORDED", outcome)
		}
		requireReweighAdjustment(t, fixture, "sell-eval-eur", domain.AdjustmentDebit, "USD", 3000, 3000, "")
	})
}

// Covers: 这笔费用上已有一笔按结算币记的计价纠错（原币种是结算币种，不是费用的原币种）。下一代的新评价回到费用的原币种，
// 原币金额仍不可比，照样按结算币记，结算币差额照净额（费用加既有纠错）算。
func TestSettleReweighStaysInSettlementCurrencyAfterAnEarlierSettlementCurrencyCorrection(t *testing.T) {
	fixture := newCrossCurrencyAdjustmentFixture(t)
	seeded, err := fixture.handler.Handle(context.Background(), application.RecordChargeAdjustmentCommand{
		TenantID: billValue(t, domain.NewTenantID, "tenant-1"), AdjustmentID: "REWEIGH-sell-eval-gbp", ChargeID: "charge-1",
		Kind: domain.PricingCorrection, Direction: domain.AdjustmentDebit, Evaluation: "sell-eval-gbp",
		OriginalCurrency: "USD", OriginalMinor: 3000, SettlementCurrency: "USD", SettlementMinor: 3000,
	})
	if err != nil || seeded.Outcome() != application.ChargeAdjustmentRecordedOutcome {
		t.Fatalf("预置按结算币记的纠错 %s err=%v", seeded.Outcome(), err)
	}
	// 净额 12000 + 3000 = 15000；新评价结算币 14000，差额贷 1000。原币 EUR 11000 与费用的 EUR 10000 同币种，但既有纠错的原币种
	// 是 USD，原币净额算不出来，不能拿 11000 去减 13000 记一笔原币贷 2000。
	if outcome := settleReweighAs(t, fixture, "sell-eval-eur", "EUR", 11000, 14000, "fx/syn/eur"); outcome != application.ReweighDifferenceRecorded {
		t.Fatalf("答 %s，want ADJUSTMENT_RECORDED", outcome)
	}
	requireReweighAdjustment(t, fixture, "sell-eval-eur", domain.AdjustmentCredit, "USD", 1000, 1000, "")
}

// Covers: 结算币差额为零就不记，不管原币那一格差多少：答 NO_DIFFERENCE，不是 SOURCE_NOT_ACCEPTED，调用方因此有结论、
// 不会一直重投。
func TestSettleReweighRecordsNothingWhenTheSettlementDifferenceIsZero(t *testing.T) {
	fixture := newCrossCurrencyAdjustmentFixture(t)
	outcome := settleReweighAs(t, fixture, "sell-eval-fx-drift", "EUR", 11000, 12000, "fx/syn/drift")
	if outcome != application.ReweighDifferenceNone || outcome.String() != "NO_DIFFERENCE" || fixture.adjustments.saves != 0 {
		t.Fatalf("答 %s saves=%d，want NO_DIFFERENCE、不记", outcome, fixture.adjustments.saves)
	}
}

// Covers: 结算币差额不为零，但原币差额记不成一笔原币差额——原币差额为零，或与结算币差额异号——这一笔按结算币记。
// 同币种费用上两额必须相等：原币差额与结算币差额对不上时同样按结算币记，不拿一对注定被形成门拒绝的金额去试。
func TestSettleReweighRecordsBySettlementCurrencyWhenTheOriginalDifferenceCannotBeRecorded(t *testing.T) {
	t.Run("原币差额为零", func(t *testing.T) {
		fixture := newCrossCurrencyAdjustmentFixture(t)
		if outcome := settleReweighAs(t, fixture, "sell-eval-flat", "EUR", 10000, 13000, "fx/syn/flat"); outcome != application.ReweighDifferenceRecorded {
			t.Fatalf("答 %s，want ADJUSTMENT_RECORDED", outcome)
		}
		requireReweighAdjustment(t, fixture, "sell-eval-flat", domain.AdjustmentDebit, "USD", 1000, 1000, "")
	})
	t.Run("两个差额异号", func(t *testing.T) {
		fixture := newCrossCurrencyAdjustmentFixture(t)
		if outcome := settleReweighAs(t, fixture, "sell-eval-opposite", "EUR", 11000, 11000, "fx/syn/opposite"); outcome != application.ReweighDifferenceRecorded {
			t.Fatalf("答 %s，want ADJUSTMENT_RECORDED", outcome)
		}
		requireReweighAdjustment(t, fixture, "sell-eval-opposite", domain.AdjustmentCredit, "USD", 1000, 1000, "")
	})
	t.Run("同币种费用上两个差额不等", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		if outcome := settleReweighAs(t, fixture, "sell-eval-unequal", "USD", 12500, 13000, ""); outcome != application.ReweighDifferenceRecorded {
			t.Fatalf("答 %s，want ADJUSTMENT_RECORDED", outcome)
		}
		requireReweighAdjustment(t, fixture, "sell-eval-unequal", domain.AdjustmentDebit, "USD", 1000, 1000, "")
	})
}

// Covers: 防回归——跨币种费用上原币可比（新评价与既有计价纠错的原币种都与费用相同）、原币差额与结算币差额同号时，照旧连原币
// 差额一起记，原币种取费用的，换算依据取新评价的；下一代接着在原币可比的链上记。
func TestSettleReweighKeepsTheOriginalDifferenceWhenItIsComparableAndSameSigned(t *testing.T) {
	fixture := newCrossCurrencyAdjustmentFixture(t)
	if outcome := settleReweighAs(t, fixture, "sell-eval-up", "EUR", 11000, 13500, "fx/syn/up"); outcome != application.ReweighDifferenceRecorded {
		t.Fatalf("涨价答 %s，want ADJUSTMENT_RECORDED", outcome)
	}
	requireReweighAdjustment(t, fixture, "sell-eval-up", domain.AdjustmentDebit, "EUR", 1000, 1500, "fx/syn/up")
	if outcome := settleReweighAs(t, fixture, "sell-eval-down", "EUR", 10500, 12600, "fx/syn/down"); outcome != application.ReweighDifferenceRecorded {
		t.Fatalf("回落答 %s，want ADJUSTMENT_RECORDED", outcome)
	}
	requireReweighAdjustment(t, fixture, "sell-eval-down", domain.AdjustmentCredit, "EUR", 500, 900, "fx/syn/down")
}

// Covers: 重估的差额只对着计价该收的净额算，商业让利不在内。价格没变的重估不记调整；价格变了，差额是新旧价格之差，
// 让掉的那一笔原样留着；贷项会不会把费用让成负数仍连让利一起看。
func TestSettleReweighLeavesACommercialConcessionAlone(t *testing.T) {
	fixture := newAdjustmentFixture(t)
	tenant := billValue(t, domain.NewTenantID, "tenant-1")
	concession, err := fixture.handler.Handle(context.Background(), application.RecordChargeAdjustmentCommand{
		TenantID: tenant, AdjustmentID: "concession-1", ChargeID: "charge-1",
		Kind: domain.CommercialConcession, Direction: domain.AdjustmentCredit, Authorization: "CONCESSION-APPROVAL/9",
		OriginalCurrency: "USD", OriginalMinor: 2000, SettlementCurrency: "USD", SettlementMinor: 2000,
	})
	if err != nil || concession.Outcome() != application.ChargeAdjustmentRecordedOutcome {
		t.Fatalf("让利 %s err=%v", concession.Outcome(), err)
	}
	settle := func(evaluation string, minor int64) application.ReweighDifferenceOutcome {
		t.Helper()
		outcome, err := fixture.handler.SettleReweigh(context.Background(), application.ReweighDifferenceCommand{
			TenantID: tenant, ChargeID: "charge-1", Evaluation: evaluation,
			OriginalCurrency: "USD", OriginalMinor: minor, SettlementCurrency: "USD", SettlementMinor: minor,
		})
		if err != nil {
			t.Fatalf("重估 %s：%v", evaluation, err)
		}
		return outcome
	}
	if outcome := settle("sell-eval-same", 12000); outcome != application.ReweighDifferenceNone || len(fixture.adjustments.records) != 1 {
		t.Fatalf("价格没变的重估答 %s，册上 %d 笔；want 无差额、只有那笔让利", outcome, len(fixture.adjustments.records))
	}
	if outcome := settle("sell-eval-up", 15000); outcome != application.ReweighDifferenceRecorded {
		t.Fatalf("涨价的重估答 %s", outcome)
	}
	debit, found := fixture.adjustments.records["REWEIGH-sell-eval-up"]
	if _, minor := debit.Adjustment.SettlementAmount(); !found || debit.Adjustment.Direction() != domain.AdjustmentDebit || minor != 3000 {
		t.Fatalf("借项 = %v %s %d，want 借 3000（15000 减 12000，不把让利补回来）", found, debit.Adjustment.Direction(), minor)
	}
	// 12000 + 3000 - 2000 = 13000。重估回 12000 贷 3000；再往下到 1500，按计价要贷 10500，连让利一起就成了负数。
	if outcome := settle("sell-eval-back", 12000); outcome != application.ReweighDifferenceRecorded {
		t.Fatalf("回落的重估答 %s", outcome)
	}
	if outcome := settle("sell-eval-low", 1500); outcome != application.ReweighDifferenceExceedsCharge {
		t.Fatalf("越过零的重估答 %s，want 贷项越过费用", outcome)
	}
}

// Covers: ADR-0087 决定二与 SA CONTEXT「已确认费用……由对应唯一创建用例追加调整明细；
// 原确认费用仍保留，对账纳入和资金核销不能代替金额调整」——调整独立成册地形成，不碰
// 原费用、不需要任何对账单参与；重复触发返回原结果。
func TestAnAdjustmentIsAppendedWithoutTouchingTheChargeOrAStatement(t *testing.T) {
	fixture := newAdjustmentFixture(t)

	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ChargeAdjustmentRecordedOutcome {
		t.Fatalf("outcome = %q, want ADJUSTMENT_RECORDED", result.Outcome())
	}
	adjustment, present := result.Adjustment()
	if !present || adjustment.Kind() != domain.PricingCorrection {
		t.Fatalf("adjustment = %#v present = %v", adjustment, present)
	}
	if !adjustment.FormedAt().Equal(adjustmentRecordedAt) {
		t.Fatalf("形成时点 = %s；它该由时钟给出，不由调用方声称", adjustment.FormedAt())
	}

	// 原费用一字未动：确认阶段、金额、确认依据都还是原来那份。
	stored := fixture.charges.charges["charge-1"]
	if stored.Stage() != domain.ChargeConfirmed {
		t.Fatalf("原费用阶段变成了 %q", stored.Stage())
	}
	if _, amount := stored.SettlementAmount(); amount != 12000 {
		t.Fatalf("原费用金额变成了 %d——调整是新对象，不改写费用本体", amount)
	}

	// 重复触发返回原结果，不重登。
	replay, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.ChargeAdjustmentReplayedOutcome {
		t.Fatalf("outcome = %q, want ALREADY_RECORDED", replay.Outcome())
	}
	if len(fixture.adjustments.records) != 1 {
		t.Fatalf("册上有 %d 笔——重复触发造出了第二笔", len(fixture.adjustments.records))
	}
}

// Covers: 调整只挂在已确认费用上（CONTEXT「已确认费用发现迟到事实……追加调整明细」）；
// 无语义种类、填错依据格由领域形成门拒；依赖故障与提交矛盾各归其格（ADR-0029）。
func TestAdjustmentGridsSplitByRecoveryAction(t *testing.T) {
	t.Run("an unconfirmed charge is its own grid", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		fixture.charges.charges["charge-1"] = provisionalCharge(t)
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-2"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentChargeNotConfirmed {
			t.Fatalf("outcome = %q；调预估不是调整，是重新预估", result.Outcome())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("专格没有留下续办引用")
		}
		if fixture.adjustments.saves != 0 {
			t.Fatal("费用还没确认就把调整登进去了")
		}
	})

	t.Run("a missing charge is not accepted", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		command := correctionCommand(t, "adj-3")
		command.ChargeID = "charge-9"
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
	})

	t.Run("a semanticless kind never reaches the register", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		command := correctionCommand(t, "adj-4")
		command.Kind = domain.AdjustmentKindInvalid
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentNotAccepted {
			t.Fatalf("outcome = %q；无语义的「冲销」进了册", result.Outcome())
		}
		if fixture.adjustments.saves != 0 {
			t.Fatal("形成门没拦住就写册了")
		}
	})

	t.Run("a correction carrying an authorization is refused", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		command := correctionCommand(t, "adj-5")
		command.Authorization = "CONCESSION-APPROVAL/9"
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentNotAccepted {
			t.Fatalf("outcome = %q；纠错挂着授权被收下了", result.Outcome())
		}
	})

	t.Run("a charge store failure is undecided with its reason", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		fixture.charges.findErr = errors.New("charge store down")
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-6"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentUndecided ||
			result.UndecidedReason() != application.AdjustmentChargeStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a register failure is undecided with its own reason", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		fixture.adjustments.saveErr = errors.New("register down")
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-7"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AdjustmentUndecided ||
			result.UndecidedReason() != application.AdjustmentRegisterUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("依赖故障没有留下续办引用")
		}
	})

	t.Run("a replay whose original cannot be read back is undecided", func(t *testing.T) {
		fixture := newAdjustmentFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-8")); err != nil {
			t.Fatalf("first handle: %v", err)
		}
		// 册答`已登记`却读不回原件：这不是重放成功，是册此刻说不出话——答未决而不是
		// 拿刚形成的那一份冒充原件。
		fixture.adjustments.lostRead = true
		result, err := fixture.handler.Handle(context.Background(), correctionCommand(t, "adj-8"))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if result.Outcome() != application.AdjustmentUndecided ||
			result.UndecidedReason() != application.AdjustmentRegisterUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.AdjustmentUndecidedReason{
			application.AdjustmentChargeStoreUnavailable, application.AdjustmentRegisterUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 2 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.AdjustmentUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第三个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
