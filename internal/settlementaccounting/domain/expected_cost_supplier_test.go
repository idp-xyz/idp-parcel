package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件证 ADR-0358 的领域半边：预期成本每一版记下所依协议的供应商（形成与纠错必带、失效纠错照搬、旧行读回可缺），匹配门对点名了
// 成本的四种分类先核供应商。

func expectedCostSpecOf(t *testing.T, version, supplier, currency string) domain.SupplierExpectedCostSpec {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		settlementValue(t, domain.NewChargeOccurrenceID, "occurrence-1"),
		settlementValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		settlementValue(t, domain.NewOccurrenceVersion, "occurrence/v1"),
		billReceivedAt.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf("new occurrence: %v", err)
	}
	spec := domain.SupplierExpectedCostSpec{
		Version:            settlementValue(t, domain.NewSupplierCostVersionID, version),
		Occurrence:         occurrence,
		FeeItem:            settlementValue(t, domain.NewFeeItemReference, "fee-linehaul"),
		RuleVersion:        settlementValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rule/v1"),
		Agreement:          settlementValue(t, domain.NewSupplierAgreementReference, "agreement-1/v1"),
		Evaluation:         settlementValue(t, domain.NewBuyEvaluationReference, "buy-evaluation-1"),
		OriginalCurrency:   settlementValue(t, domain.NewCurrencyCode, currency),
		OriginalMinor:      12000,
		SettlementCurrency: settlementValue(t, domain.NewCurrencyCode, currency),
		SettlementMinor:    12000,
	}
	if supplier != "" {
		spec.Supplier = settlementValue(t, domain.NewSupplierPartyReference, supplier)
	}
	return spec
}

// Covers: `AT-SA-236`（领域半边：记下供应商）——首版与纠错版本都必带供应商；纠错版本记的是它自己那一版协议的供应商（后继协议
// 换了一家，纠错版本就记新的那一家），失效纠错版本照搬被纠正那一版的；原版本一字不动。
func TestAnExpectedCostRecordsTheSupplierOfItsAgreement(t *testing.T) {
	if _, err := domain.FormSupplierExpectedCost(expectedCostSpecOf(t, "cost/v1", "", "USD")); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("没有供应商的首版 err = %v, want ErrInvalidSupplierCost", err)
	}
	first, err := domain.FormSupplierExpectedCost(expectedCostSpecOf(t, "cost/v1", "partner-1", "USD"))
	if err != nil {
		t.Fatal(err)
	}
	if supplier, recorded := first.Supplier(); !recorded || supplier.String() != "partner-1" {
		t.Fatalf("首版供应商 = %q recorded=%v", supplier, recorded)
	}

	correction := domain.CostCorrectionSpec{
		Version:          settlementValue(t, domain.NewSupplierCostVersionID, "cost/v2"),
		Occurrence:       first.Occurrence(),
		RuleVersion:      first.RuleVersion(),
		Agreement:        settlementValue(t, domain.NewSupplierAgreementReference, "agreement-1/v2"),
		Evaluation:       settlementValue(t, domain.NewBuyEvaluationReference, "buy-evaluation-2"),
		OriginalCurrency: settlementValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:    12500,
		SettlementMinor:  12500,
		Reason:           domain.OccurrenceSupersededCorrection(),
	}
	if _, err := first.AppendCorrection(correction); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("没有供应商的纠错版本 err = %v, want ErrInvalidSupplierCost", err)
	}
	correction.Supplier = settlementValue(t, domain.NewSupplierPartyReference, "partner-2")
	corrected, err := first.AppendCorrection(correction)
	if err != nil {
		t.Fatal(err)
	}
	if supplier, _ := corrected.Supplier(); supplier.String() != "partner-2" {
		t.Fatalf("纠错版本供应商 = %q，want 后继协议的 partner-2", supplier)
	}
	if supplier, _ := first.Supplier(); supplier.String() != "partner-1" {
		t.Fatalf("原版本供应商被改成 %q", supplier)
	}

	invalidated, err := domain.NewTransportChargeOccurrence(first.Occurrence().ID(), first.Occurrence().Reason(),
		settlementValue(t, domain.NewOccurrenceVersion, "occurrence/v1-invalidated"), first.Occurrence().OccurredAt())
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := first.Withdraw(invalidated)
	if err != nil {
		t.Fatal(err)
	}
	if supplier, recorded := withdrawn.Supplier(); !recorded || supplier.String() != "partner-1" {
		t.Fatalf("失效纠错版本供应商 = %q recorded=%v，want 照搬被纠正那一版的", supplier, recorded)
	}
}

// Covers: `AT-SA-237`（领域半边：旧行）——本记录之前形成的行读回时没有供应商，读作未记；有就照记。
func TestARehydratedCostWithoutASupplierReadsAsUnrecorded(t *testing.T) {
	spec := expectedCostSpecOf(t, "cost/legacy", "", "USD")
	legacy, err := domain.RehydrateSupplierExpectedCost(domain.RehydrateSupplierExpectedCostSpec{
		Version: spec.Version, Occurrence: spec.Occurrence, FeeItem: spec.FeeItem, RuleVersion: spec.RuleVersion,
		Agreement: spec.Agreement, Evaluation: spec.Evaluation, OriginalCurrency: spec.OriginalCurrency, OriginalMinor: spec.OriginalMinor,
		SettlementCurrency: spec.SettlementCurrency, SettlementMinor: spec.SettlementMinor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if supplier, recorded := legacy.Supplier(); recorded || supplier.String() != "" {
		t.Fatalf("旧行供应商 = %q recorded=%v，want 未记", supplier, recorded)
	}
}

// Covers: `AT-SA-236`、`AT-SA-237`（领域半边：匹配门）——点名了预期成本的四种分类先核供应商：别家的答 ErrBillSupplierMismatch，
// 没记的答 ErrExpectedCostSupplierUnrecorded，都排在币种之前；同一家的照常往下核；没有对应发生项点名了成本是形状错，不答成供应商
// 不符。
func TestMatchingChecksTheExpectedCostSupplier(t *testing.T) {
	claim := receivedClaim(t) // 供应商 partner-1，USD，line-1 主张 12000
	line := settlementValue(t, domain.NewBillLineReference, "line-1")
	basis := settlementValue(t, domain.NewMatchBasisReference, "basis-1")
	form := func(supplier, currency string) domain.SupplierExpectedCost {
		cost, err := domain.FormSupplierExpectedCost(expectedCostSpecOf(t, "cost/v1", supplier, currency))
		if err != nil {
			t.Fatal(err)
		}
		return cost
	}
	spec := expectedCostSpecOf(t, "cost/legacy", "", "USD")
	legacy, err := domain.RehydrateSupplierExpectedCost(domain.RehydrateSupplierExpectedCostSpec{
		Version: spec.Version, Occurrence: spec.Occurrence, FeeItem: spec.FeeItem, RuleVersion: spec.RuleVersion,
		Agreement: spec.Agreement, Evaluation: spec.Evaluation, OriginalCurrency: spec.OriginalCurrency, OriginalMinor: spec.OriginalMinor,
		SettlementCurrency: spec.SettlementCurrency, SettlementMinor: spec.SettlementMinor,
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, testCase := range map[string]struct {
		classification domain.MatchClassification
		expected       domain.SupplierExpectedCost
		basis          domain.MatchBasisReference
		want           error
	}{
		"同一家已对上":       {domain.LineMatched, form("partner-1", "USD"), domain.MatchBasisReference{}, nil},
		"别家已对上":        {domain.LineMatched, form("partner-2", "USD"), domain.MatchBasisReference{}, domain.ErrBillSupplierMismatch},
		"别家价差":         {domain.PriceVariance, form("partner-2", "USD"), basis, domain.ErrBillSupplierMismatch},
		"别家量差":         {domain.QuantityVariance, form("partner-2", "USD"), basis, domain.ErrBillSupplierMismatch},
		"别家重复计费":       {domain.DuplicateBilling, form("partner-2", "USD"), basis, domain.ErrBillSupplierMismatch},
		"别家又跨币种：先答不符":  {domain.LineMatched, form("partner-2", "EUR"), domain.MatchBasisReference{}, domain.ErrBillSupplierMismatch},
		"同一家跨币种：答换算":   {domain.LineMatched, form("partner-1", "EUR"), domain.MatchBasisReference{}, domain.ErrCrossCurrencyMatch},
		"没记供应商的旧行":     {domain.LineMatched, legacy, domain.MatchBasisReference{}, domain.ErrExpectedCostSupplierUnrecorded},
		"没有对应发生项却点了别家": {domain.NoMatchingOccurrence, form("partner-2", "USD"), basis, domain.ErrInvalidBillLineMatch},
	} {
		_, err := domain.MatchBillLine(claim, line, testCase.classification, testCase.expected, testCase.basis, billMatchedAt)
		if !errors.Is(err, testCase.want) || (testCase.want == nil && err != nil) {
			t.Fatalf("%s: err = %v, want %v", name, err, testCase.want)
		}
	}
	// 没有对应发生项、也不点名成本：没有可核的供应商，照旧成立。
	if _, err := domain.MatchBillLine(claim, line, domain.NoMatchingOccurrence, domain.SupplierExpectedCost{}, basis, billMatchedAt); err != nil {
		t.Fatalf("没有对应发生项：%v", err)
	}
}
