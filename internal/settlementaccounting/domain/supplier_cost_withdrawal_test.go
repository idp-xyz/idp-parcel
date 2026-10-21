package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: AT-SA-206 的领域半边（ADR-0232 决定七）——失效纠错版本由被纠正那一版派生标识、回指它、原因是发生项失效，发生项引用换成失效的
// 那一版，计价结果原样带过去；原版不动；已失效的再失效答已失效；换发生项 ID 或沿用同一版发生项都拒。之后按新评价的纠错把它接回有效。
func TestAWithdrawalVersionCarriesTheCostAndStatesItNoLongerStands(t *testing.T) {
	original, err := domain.FormSupplierExpectedCost(crossCurrencySpec(t))
	if err != nil {
		t.Fatal(err)
	}
	invalidated, err := domain.NewTransportChargeOccurrence(original.Occurrence().ID(), original.Occurrence().Reason(),
		mustValue(t, domain.NewOccurrenceVersion, "V2"), original.Occurrence().OccurredAt())
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := original.Withdraw(invalidated)
	if err != nil {
		t.Fatal(err)
	}
	prior, _ := withdrawn.PriorVersion()
	reason, _ := withdrawn.CorrectionReason()
	_, before := original.SettlementAmount()
	_, after := withdrawn.SettlementAmount()
	if !withdrawn.Withdrawn() || prior != original.Version() || withdrawn.Version().String() != "BUYSCW-"+original.Version().String() ||
		reason.String() != "OCCURRENCE_INVALIDATED" || withdrawn.Occurrence().Version().String() != "V2" || after != before || original.Withdrawn() {
		t.Fatalf("失效纠错版本 = %+v", withdrawn)
	}
	if _, err := withdrawn.Withdraw(invalidated); !errors.Is(err, domain.ErrSupplierCostAlreadyWithdrawn) {
		t.Fatalf("已失效再失效 err = %v", err)
	}
	if _, err := original.Withdraw(original.Occurrence()); err == nil {
		t.Fatal("沿用同一版发生项也失效了")
	}
	stranger, err := domain.NewTransportChargeOccurrence(mustValue(t, domain.NewChargeOccurrenceID, "occurrence-other"),
		original.Occurrence().Reason(), mustValue(t, domain.NewOccurrenceVersion, "V2"), original.Occurrence().OccurredAt())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := original.Withdraw(stranger); err == nil {
		t.Fatal("换了发生项 ID 也失效了")
	}
	if _, err := domain.SupplierCostWithdrawalVersionID(domain.SupplierCostVersionID{}); err == nil {
		t.Fatal("空版本派生出了失效纠错标识")
	}
}

// 重建门：失效纠错版本必回指它纠正的那一版。
func TestARehydratedWithdrawalNeedsItsPriorVersion(t *testing.T) {
	original, err := domain.FormSupplierExpectedCost(crossCurrencySpec(t))
	if err != nil {
		t.Fatal(err)
	}
	spec := domain.RehydrateSupplierExpectedCostSpec{
		Version: original.Version(), Occurrence: original.Occurrence(), FeeItem: original.FeeItem(), RuleVersion: original.RuleVersion(),
		Agreement: original.Agreement(), Evaluation: original.Evaluation(), Withdrawn: true,
	}
	spec.OriginalCurrency, spec.OriginalMinor = original.OriginalAmount()
	spec.SettlementCurrency, spec.SettlementMinor = original.SettlementAmount()
	spec.Conversion, _ = original.Conversion()
	if _, err := domain.RehydrateSupplierExpectedCost(spec); !errors.Is(err, domain.ErrInvalidSupplierCost) {
		t.Fatalf("没有回指的失效纠错版本重建 err = %v", err)
	}
}
