package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: ADR-0232 决定七的账单匹配那一半（评审 4）——账单一行点名的预期成本版本已撤下（失效纠错版本）或已有后继（被撤下、被纠正过的
// 原版），都答 EXPECTED_COST_NOT_CURRENT、带出是哪一行哪一版与它的后继，不落账单、不交意图；点名当前那一版照常匹配。
func TestABillLineNamingANonCurrentExpectedCostIsNotMatched(t *testing.T) {
	fixture := newBillFixture(t)
	original := fixture.costs.costs["cost/v1"]
	invalidated, err := domain.NewTransportChargeOccurrence(original.Occurrence().ID(), original.Occurrence().Reason(),
		billValue(t, domain.NewOccurrenceVersion, "occurrence/v2"), original.Occurrence().OccurredAt())
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := original.Withdraw(invalidated)
	if err != nil {
		t.Fatal(err)
	}
	fixture.costs.costs[withdrawn.Version().String()] = withdrawn

	for name, version := range map[string]string{"原版": "cost/v1", "失效纠错版本": withdrawn.Version().String()} {
		command := billCommand(t, "bill-"+name)
		command.Directives[0].ExpectedVersion = billValue(t, domain.NewSupplierCostVersionID, version)
		result, err := fixture.handler.Handle(context.Background(), command)
		stale, present := result.NotCurrent()
		if err != nil || result.Outcome() != application.BillExpectedCostNotCurrent || !present ||
			stale.Version.String() != version || stale.Line.String() != "line-1" {
			t.Fatalf("%s：outcome = %q %+v err=%v", name, result.Outcome(), stale, err)
		}
		if name == "原版" && (stale.Withdrawn || stale.Successor != withdrawn.Version()) {
			t.Fatalf("原版应带出后继：%+v", stale)
		}
		if name == "失效纠错版本" && !stale.Withdrawn {
			t.Fatalf("失效纠错版本应标已撤下：%+v", stale)
		}
	}
	if len(fixture.store.records) != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("非当前那一版也落了账单 %d 份、意图 %d 份", len(fixture.store.records), len(fixture.handoff.intents))
	}

	current := newBillFixture(t)
	head := current.costs.costs["cost/v1"]
	corrected, err := head.AppendCorrection(domain.CostCorrectionSpec{
		Version: billValue(t, domain.NewSupplierCostVersionID, "cost/v1-corrected"), Occurrence: invalidated,
		RuleVersion: head.RuleVersion(), Agreement: head.Agreement(), Evaluation: head.Evaluation(),
		OriginalCurrency: billValue(t, domain.NewCurrencyCode, "USD"), OriginalMinor: 12000, SettlementMinor: 12000,
		Reason: domain.OccurrenceSupersededCorrection(),
	})
	if err != nil {
		t.Fatal(err)
	}
	current.costs.costs[corrected.Version().String()] = corrected
	command := billCommand(t, "bill-current")
	command.Directives[0].ExpectedVersion = corrected.Version()
	if result, err := current.handler.Handle(context.Background(), command); err != nil || result.Outcome() != application.BillReceived {
		t.Fatalf("点名链头那一版 = %q %v, want BILL_RECEIVED", result.Outcome(), err)
	}
}
