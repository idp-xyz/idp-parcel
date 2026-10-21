package application_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件证发生项失效驱动的预期成本纠错（票 demo-chain-gaps/15，ADR-0232 决定七、八）：失效撤下依那一版形成的链头、记下那一版已失效，
// 原版不动；重投不重复撤；替代版本的新评价接在失效纠错版本后面重述全额。替身同形成编排那一套，夹具全是合成串。

func withdrawalHandler(t *testing.T, fixture *requestedCostFixture) *application.WithdrawSupplierExpectedCostHandler {
	t.Helper()
	handler, err := application.NewWithdrawSupplierExpectedCostHandler(application.WithdrawSupplierExpectedCostDeps{
		Registry: fixture.registry, Costs: fixture.registry, Invalidations: fixture.registry, Clock: costClock{at: costRecordedAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func invalidatedOccurrence(t *testing.T, version string) domain.TransportChargeOccurrence {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		billValue(t, domain.NewChargeOccurrenceID, "occurrence-7"),
		billValue(t, domain.NewOccurrenceReasonReference, "BOOKING"),
		billValue(t, domain.NewOccurrenceVersion, version),
		costOccurredAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	return occurrence
}

func withdraw(t *testing.T, handler *application.WithdrawSupplierExpectedCostHandler, version, invalidates string) application.SupplierCostWithdrawalResult {
	t.Helper()
	result, err := handler.Withdraw(t.Context(), application.WithdrawSupplierExpectedCostCommand{
		TenantID:    billValue(t, domain.NewTenantID, "tenant-1"),
		Occurrence:  invalidatedOccurrence(t, version),
		Invalidates: billValue(t, domain.NewOccurrenceVersion, invalidates),
	})
	if err != nil {
		t.Fatalf("撤下不该上抛：%v", err)
	}
	return result
}

// Covers: AT-SA-164、AT-SA-206——依 v1 形成的首版在发生项 v2 失效之后撤下：长出一版失效纠错版本，回指首版、原因是发生项失效、发生项引用换成 v2，
// 金额原样；首版一字不动，不形成账单贷项。同一封失效信重投答重放、不多一版；一封依别的版本的旧失效信不动它。
func TestAnOccurrenceInvalidationWithdrawsTheCostFormedFromThatVersion(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatal(err)
	}
	handler := withdrawalHandler(t, fixture)

	result := withdraw(t, handler, "occurrence-7/v2", "occurrence-7/v1")
	if result.Outcome() != application.SupplierCostWithdrawn || len(result.Withdrawn()) != 1 {
		t.Fatalf("撤下 = %s，%d 版", result.Outcome(), len(result.Withdrawn()))
	}
	withdrawn := result.Withdrawn()[0]
	prior, _ := withdrawn.PriorVersion()
	reason, _ := withdrawn.CorrectionReason()
	_, minor := withdrawn.SettlementAmount()
	if withdrawn.Version().String() != "BUYSCW-BUYSC-EVREQ-SYN-1" || prior.String() != "BUYSC-EVREQ-SYN-1" || !withdrawn.Withdrawn() ||
		reason.String() != "OCCURRENCE_INVALIDATED" || withdrawn.Occurrence().Version().String() != "occurrence-7/v2" || minor != 30000 {
		t.Fatalf("失效纠错版本 = %+v", withdrawn)
	}
	if first := fixture.registry.costs["BUYSC-EVREQ-SYN-1"]; first.Withdrawn() || first.Occurrence().Version().String() != "occurrence-7/v1" {
		t.Fatal("首版被改了")
	}
	if replay := withdraw(t, handler, "occurrence-7/v2", "occurrence-7/v1"); replay.Outcome() != application.SupplierCostWithdrawalExisting ||
		len(fixture.registry.costs) != 2 {
		t.Fatalf("重投 = %s，成本 %d 版", replay.Outcome(), len(fixture.registry.costs))
	}
	if stale := withdraw(t, handler, "occurrence-7/v9", "occurrence-7/v8"); stale.Outcome() != application.SupplierCostWithdrawalNothingFormed ||
		len(fixture.registry.costs) != 2 {
		t.Fatalf("别的版本的失效信 = %s", stale.Outcome())
	}
}

// Covers: AT-SA-208、AT-SA-209——失效信先到、那一版的评价还在路上：没有可撤的成本，答 NO_COST_FORMED，但记下了那一版已失效，之后到的评价
// 不再形成成本。失效之后长出的替代版本（v3）的评价接在失效纠错版本后面重述全额：回指它、原因是发生项替代、不再是失效。
func TestALateEvaluationOfAnInvalidatedVersionFormsNothingAndTheSupersedingOneRestatesTheCost(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	handler := withdrawalHandler(t, fixture)
	if early := withdraw(t, handler, "occurrence-7/v2", "occurrence-7/v1"); early.Outcome() != application.SupplierCostWithdrawalNothingFormed {
		t.Fatalf("评价还没到 = %s", early.Outcome())
	}
	late, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil || late.Outcome() != application.RequestedSupplierExpectedCostOccurrenceInvalidated || len(fixture.registry.costs) != 0 {
		t.Fatalf("晚到的评价 = %s err=%v，成本 %d 版", late.Outcome(), err, len(fixture.registry.costs))
	}

	other := newRequestedCostFixture(t)
	if _, err := other.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatal(err)
	}
	withdraw(t, withdrawalHandler(t, other), "occurrence-7/v2", "occurrence-7/v1")
	other.requests.register(t, "EVREQ-SYN-3", "occurrence-7/v3")
	other.evaluations.adoptions["buy-eval-3"] = completedAdoption(t, "buy-eval-3")
	restated, err := other.handle(t, "buy-eval-3", "EVREQ-SYN-3")
	if err != nil || restated.Formation().Outcome() != application.ExpectedCostFormed {
		t.Fatalf("替代版本的评价 = %s err=%v", restated.Formation().Outcome(), err)
	}
	cost, _ := restated.Formation().Cost()
	prior, _ := cost.PriorVersion()
	reason, _ := cost.CorrectionReason()
	if prior.String() != "BUYSCW-BUYSC-EVREQ-SYN-1" || reason.String() != "OCCURRENCE_SUPERSEDED" || cost.Withdrawn() ||
		cost.Occurrence().Version().String() != "occurrence-7/v3" {
		t.Fatalf("重述 = %+v", cost)
	}
}
