package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

const reweighDay = 24 * time.Hour

// reweighCharge 形成一笔已确认费用：原币与结算币相同时不带换算依据，不同时带。
func reweighCharge(t *testing.T, originalCurrency string, originalMinor int64, settlementCurrency string, settlementMinor int64) domain.CustomerCharge {
	t.Helper()
	spec := domain.CustomerChargeSpec{
		ID:                 mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		FeeItem:            mustValue(t, domain.NewFeeItemReference, "BASE_FREIGHT"),
		Evaluation:         mustValue(t, domain.NewSellEvaluationReference, "evaluation-sell-0"),
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, originalCurrency),
		OriginalMinor:      originalMinor,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, settlementCurrency),
		SettlementMinor:    settlementMinor,
		Stage:              domain.ChargeEstimated,
		FormedAt:           chargeFormedAt,
	}
	if originalCurrency != settlementCurrency {
		spec.Conversion = mustValue(t, domain.NewConversionStepReference, "fx-0")
	}
	charge, err := domain.FormCustomerCharge(spec)
	if err != nil {
		t.Fatalf("form customer charge: %v", err)
	}
	confirmed, err := charge.Confirm(
		confirmationFacts(t),
		mustValue(t, domain.NewConfirmationBasisReference, "DELIVERY_FINALIZED/final-1"),
		chargeFormedAt.Add(reweighDay),
	)
	if err != nil {
		t.Fatalf("confirm customer charge: %v", err)
	}
	return confirmed
}

// reweighAdjustment 形成这笔费用上的一笔调整，结算币恒为 CNY。
func reweighAdjustment(
	t *testing.T,
	id string,
	kind domain.AdjustmentKind,
	direction domain.AdjustmentDirection,
	originalCurrency string,
	originalMinor, settlementMinor int64,
) domain.ChargeAdjustment {
	t.Helper()
	spec := domain.ChargeAdjustmentSpec{
		ID:                 mustValue(t, domain.NewChargeAdjustmentID, id),
		Charge:             mustValue(t, domain.NewCustomerChargeID, "charge-1"),
		Kind:               kind,
		Direction:          direction,
		OriginalCurrency:   mustValue(t, domain.NewCurrencyCode, originalCurrency),
		OriginalMinor:      originalMinor,
		SettlementCurrency: mustValue(t, domain.NewCurrencyCode, "CNY"),
		SettlementMinor:    settlementMinor,
		FormedAt:           chargeFormedAt.Add(2 * reweighDay),
	}
	switch kind {
	case domain.PricingCorrection:
		spec.Evaluation = mustValue(t, domain.NewSellEvaluationReference, "evaluation-"+id)
	case domain.CommercialConcession:
		spec.Authorization = mustValue(t, domain.NewCommercialAuthorizationReference, "authorization-"+id)
	}
	if originalCurrency != "CNY" {
		spec.Conversion = mustValue(t, domain.NewConversionStepReference, "fx-"+id)
	}
	adjustment, err := domain.FormChargeAdjustment(spec)
	if err != nil {
		t.Fatalf("form adjustment %s: %v", id, err)
	}
	return adjustment
}

// Covers: 复磅差额按结算币判——新评价的结算币金额减去费用加既有计价纠错的净额，商业让利不算；结算币差额为零不记，
// 不管原币那一格差多少；原币种都相同、原币差额不为零且与结算币差额同号时连原币差额一并记，否则按结算币记，
// 原币即结算币、不带换算依据。
func TestSettleReweighDifferenceFollowsTheSettlementCurrencyRule(t *testing.T) {
	// 费用：原币 USD 1000，结算币 CNY 7000，带换算依据。
	charge := reweighCharge(t, "USD", 1000, "CNY", 7000)
	debitCorrection := reweighAdjustment(t, "adj-debit", domain.PricingCorrection, domain.AdjustmentDebit, "USD", 70, 500)
	creditCorrection := reweighAdjustment(t, "adj-credit", domain.PricingCorrection, domain.AdjustmentCredit, "USD", 100, 700)
	otherCurrencyCorrection := reweighAdjustment(t, "adj-eur", domain.PricingCorrection, domain.AdjustmentDebit, "EUR", 60, 500)
	concession := reweighAdjustment(t, "adj-concession", domain.CommercialConcession, domain.AdjustmentCredit, "USD", 140, 1000)

	// 同币种的费用：原币与结算币都是 CNY。
	sameCurrencyCharge := reweighCharge(t, "CNY", 7000, "CNY", 7000)

	settlementOnly := func(direction domain.AdjustmentDirection, settlementMinor int64) domain.ReweighDifference {
		return domain.ReweighDifference{
			Needed: true, Direction: direction, SettlementMinor: settlementMinor,
			OriginalCurrency: "CNY", OriginalMinor: settlementMinor,
		}
	}
	withOriginal := func(direction domain.AdjustmentDirection, settlementMinor, originalMinor int64, conversion string) domain.ReweighDifference {
		return domain.ReweighDifference{
			Needed: true, Direction: direction, SettlementMinor: settlementMinor,
			CarriesOriginalDifference: true, OriginalCurrency: "USD", OriginalMinor: originalMinor, Conversion: conversion,
		}
	}

	cases := []struct {
		name        string
		charge      domain.CustomerCharge
		adjustments []domain.ChargeAdjustment
		evaluation  domain.ReweighEvaluation
		want        domain.ReweighDifference
	}{
		{
			name:       "结算币差额为零不记",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1000, SettlementCurrency: "CNY", SettlementMinor: 7000, Conversion: "fx-1"},
			want:       domain.ReweighDifference{},
		},
		{
			name:       "结算币差额为零而原币差额不为零也不记",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1200, SettlementCurrency: "CNY", SettlementMinor: 7000, Conversion: "fx-1"},
			want:       domain.ReweighDifference{},
		},
		{
			name:       "新评价高于净额记借项并带原币差额",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1100, SettlementCurrency: "CNY", SettlementMinor: 8000, Conversion: "fx-1"},
			want:       withOriginal(domain.AdjustmentDebit, 1000, 100, "fx-1"),
		},
		{
			name:       "新评价低于净额记贷项并带原币差额",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 900, SettlementCurrency: "CNY", SettlementMinor: 6000, Conversion: "fx-1"},
			want:       withOriginal(domain.AdjustmentCredit, 1000, 100, "fx-1"),
		},
		{
			name:        "既有借项纠错计入净额，差额只算超出的部分",
			charge:      charge,
			adjustments: []domain.ChargeAdjustment{debitCorrection},
			evaluation:  domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1100, SettlementCurrency: "CNY", SettlementMinor: 8000, Conversion: "fx-1"},
			want:        withOriginal(domain.AdjustmentDebit, 500, 30, "fx-1"),
		},
		{
			name:        "既有借项纠错之后新评价回到净额，不记",
			charge:      charge,
			adjustments: []domain.ChargeAdjustment{debitCorrection},
			evaluation:  domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1070, SettlementCurrency: "CNY", SettlementMinor: 7500, Conversion: "fx-1"},
			want:        domain.ReweighDifference{},
		},
		{
			name:        "既有贷项纠错计入净额，新评价回到原价记借项",
			charge:      charge,
			adjustments: []domain.ChargeAdjustment{creditCorrection},
			evaluation:  domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1000, SettlementCurrency: "CNY", SettlementMinor: 7000, Conversion: "fx-1"},
			want:        withOriginal(domain.AdjustmentDebit, 700, 100, "fx-1"),
		},
		{
			name:        "商业让利不计入净额：新评价等于费用时不记",
			charge:      charge,
			adjustments: []domain.ChargeAdjustment{concession},
			evaluation:  domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1000, SettlementCurrency: "CNY", SettlementMinor: 7000, Conversion: "fx-1"},
			want:        domain.ReweighDifference{},
		},
		{
			name:        "商业让利不计入净额：差额不被让利放大",
			charge:      charge,
			adjustments: []domain.ChargeAdjustment{concession},
			evaluation:  domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1100, SettlementCurrency: "CNY", SettlementMinor: 8000, Conversion: "fx-1"},
			want:        withOriginal(domain.AdjustmentDebit, 1000, 100, "fx-1"),
		},
		{
			name:       "新评价的原币种与费用不同，按结算币记",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "EUR", OriginalMinor: 1300, SettlementCurrency: "CNY", SettlementMinor: 8000, Conversion: "fx-1"},
			want:       settlementOnly(domain.AdjustmentDebit, 1000),
		},
		{
			name:        "既有纠错的原币种与费用不同，按结算币记",
			charge:      charge,
			adjustments: []domain.ChargeAdjustment{otherCurrencyCorrection},
			evaluation:  domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1100, SettlementCurrency: "CNY", SettlementMinor: 8000, Conversion: "fx-1"},
			want:        settlementOnly(domain.AdjustmentDebit, 500),
		},
		{
			name:       "原币差额与结算币差额异号，按结算币记",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 900, SettlementCurrency: "CNY", SettlementMinor: 8000, Conversion: "fx-1"},
			want:       settlementOnly(domain.AdjustmentDebit, 1000),
		},
		{
			name:       "原币差额为零，按结算币记",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1000, SettlementCurrency: "CNY", SettlementMinor: 6000, Conversion: "fx-1"},
			want:       settlementOnly(domain.AdjustmentCredit, 1000),
		},
		{
			name:       "跨币种而新评价不带换算依据，按结算币记",
			charge:     charge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "USD", OriginalMinor: 1100, SettlementCurrency: "CNY", SettlementMinor: 8000},
			want:       settlementOnly(domain.AdjustmentDebit, 1000),
		},
		{
			name:       "同币种且两个差额相等，连原币差额一并记",
			charge:     sameCurrencyCharge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "CNY", OriginalMinor: 8000, SettlementCurrency: "CNY", SettlementMinor: 8000},
			want: domain.ReweighDifference{
				Needed: true, Direction: domain.AdjustmentDebit, SettlementMinor: 1000,
				CarriesOriginalDifference: true, OriginalCurrency: "CNY", OriginalMinor: 1000,
			},
		},
		{
			name:       "同币种而两个差额不等，按结算币记",
			charge:     sameCurrencyCharge,
			evaluation: domain.ReweighEvaluation{OriginalCurrency: "CNY", OriginalMinor: 7500, SettlementCurrency: "CNY", SettlementMinor: 8000},
			want:       settlementOnly(domain.AdjustmentDebit, 1000),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.SettleReweighDifference(tc.charge, tc.adjustments, tc.evaluation)
			if got != tc.want {
				t.Fatalf("差额 = %+v，want %+v", got, tc.want)
			}
		})
	}
}

// Covers: 一笔调整折成对费用金额的增减，借项加、贷项减。
func TestAdjustmentDirectionSignsTheAmountAgainstTheCharge(t *testing.T) {
	if got := domain.AdjustmentDebit.Signed(300); got != 300 {
		t.Fatalf("借项 = %d，want 300", got)
	}
	if got := domain.AdjustmentCredit.Signed(300); got != -300 {
		t.Fatalf("贷项 = %d，want -300", got)
	}
}
