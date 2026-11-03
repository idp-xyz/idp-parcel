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

// 本文件证 ADR-0358 的编排半边：形成时问协议的供应商并记下、接收时匹配门核供应商、补记旧行。替身同形成与接收编排那一套。

// Covers: `AT-SA-236`（编排半边：形成）——首版记下所依协议的供应商，问的是命令上的那一版协议；重放不改；后继协议换了一家，
// 纠错版本记新的那一家、被纠正那一版原样；失效纠错版本照搬，不再问。
func TestTheFormedCostRecordsTheSupplierOfItsAgreement(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	fixture.suppliers.suppliers["agreement-9"] = "SYN-PARTY-TRUNK"
	fixture.suppliers.suppliers["agreement-10"] = "SYN-PARTY-LM"
	if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatalf("首版：%v", err)
	}
	first := fixture.registry.costs["BUYSC-EVREQ-SYN-1"]
	if supplier, recorded := first.Supplier(); !recorded || supplier.String() != "SYN-PARTY-TRUNK" {
		t.Fatalf("首版供应商 = %q recorded=%v", supplier, recorded)
	}
	if len(fixture.suppliers.asked) != 1 || fixture.suppliers.asked[0] != "agreement-9" {
		t.Fatalf("问了 %v，want 只问首版所依的 agreement-9", fixture.suppliers.asked)
	}
	if replay, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil || replay.Outcome() != application.RequestedSupplierExpectedCostAdopted {
		t.Fatalf("重放 = %s err=%v", replay.Outcome(), err)
	}

	asked := len(fixture.suppliers.asked)
	withdraw(t, withdrawalHandler(t, fixture), "occurrence-7/v2", "occurrence-7/v1")
	if len(fixture.suppliers.asked) != asked {
		t.Fatal("撤下那一路去问了协议的供应商——失效纠错版本照搬，不该问")
	}
	for _, cost := range fixture.registry.costs {
		if cost.Withdrawn() {
			if supplier, _ := cost.Supplier(); supplier.String() != "SYN-PARTY-TRUNK" {
				t.Fatalf("失效纠错版本供应商 = %q，want 照搬的 SYN-PARTY-TRUNK", supplier)
			}
		}
	}

	fixture.requests.registerUnder(t, "EVREQ-SYN-3", "occurrence-7/v3", "agreement-10")
	fixture.evaluations.adoptions["buy-eval-3"] = completedAdoption(t, "buy-eval-3")
	corrected, err := fixture.handle(t, "buy-eval-3", "EVREQ-SYN-3")
	if err != nil || corrected.Formation().Outcome() != application.ExpectedCostFormed {
		t.Fatalf("纠错版本 = %s err=%v", corrected.Formation().Outcome(), err)
	}
	cost, _ := corrected.Formation().Cost()
	if supplier, _ := cost.Supplier(); supplier.String() != "SYN-PARTY-LM" || cost.Agreement().String() != "agreement-10" {
		t.Fatalf("纠错版本 协议 %s 供应商 %q，want 后继协议 agreement-10 的 SYN-PARTY-LM", cost.Agreement(), supplier)
	}
	if supplier, _ := fixture.registry.costs["BUYSC-EVREQ-SYN-1"].Supplier(); supplier.String() != "SYN-PARTY-TRUNK" {
		t.Fatalf("被纠正那一版供应商被改成 %q", supplier)
	}
}

// Covers: ADR-0358 决定一读口三格——问不动答未决（SUPPLIER_AGREEMENT_VIEW_UNAVAILABLE，零写入，外层也落未决而不记停因）；PC 不认
// 那一版协议、答出译不过来的值都是两侧矛盾，上抛、零写入。
func TestTheAgreementSupplierCellsOfFormation(t *testing.T) {
	t.Run("unreadable is undecided", func(t *testing.T) {
		fixture := newRequestedCostFixture(t)
		down := errors.New("party commercial down")
		fixture.suppliers.err = down
		result, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
		if err != nil || result.Outcome() != application.RequestedSupplierExpectedCostUndecided ||
			result.Formation().UndecidedReason() != application.SupplierAgreementViewUnavailable || !errors.Is(result.Formation().UndecidedCause(), down) {
			t.Fatalf("outcome = %s / %s err=%v", result.Outcome(), result.Formation().UndecidedReason(), err)
		}
		if len(fixture.registry.costs) != 0 {
			t.Fatal("问不动协议的供应商却落了成本")
		}
	})
	t.Run("an agreement party-commercial does not know is a contradiction", func(t *testing.T) {
		fixture := newRequestedCostFixture(t)
		fixture.suppliers.unknown["agreement-9"] = true
		if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); !errors.Is(err, application.ErrOccurrenceAgreementNotRegistered) {
			t.Fatalf("err = %v, want ErrOccurrenceAgreementNotRegistered", err)
		}
		if len(fixture.registry.costs) != 0 {
			t.Fatal("PC 不认的协议落了成本")
		}
	})
	t.Run("an untranslatable answer is a contradiction", func(t *testing.T) {
		fixture := newRequestedCostFixture(t)
		fixture.suppliers.err = ports.ErrSupplierAgreementSupplierUntranslatable
		if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); !errors.Is(err, ports.ErrSupplierAgreementSupplierUntranslatable) {
			t.Fatalf("err = %v, want ErrSupplierAgreementSupplierUntranslatable", err)
		}
		if len(fixture.registry.costs) != 0 {
			t.Fatal("译不过来的答复落了成本")
		}
	})
	t.Run("a formation wired without the view is an error", func(t *testing.T) {
		evaluations := newBuyEvaluationView()
		evaluations.adoptions["buy-eval-1"] = completedAdoption(t, "buy-eval-1")
		handler := application.NewFormSupplierExpectedCostHandler(application.FormSupplierExpectedCostDeps{
			Evaluations: evaluations, Costs: newExpectedCostRegistry(), Registry: newExpectedCostRegistry(), Clock: costClock{at: costRecordedAt},
		})
		if _, err := handler.Handle(context.Background(), expectedCostCommand(t, "cost-7/v1", "buy-eval-1")); err == nil {
			t.Fatal("缺协议供应商读口却形成了成本")
		}
	})
}

// Covers: `AT-SA-236`、`AT-SA-237`（编排半边：接收）——一行点名别家的预期成本答供应商不符、带出行、版本与那一版的供应商；点名没记
// 供应商的旧行答供应商未记；两格都不落账单、不交意图；当前版判据排在供应商之前；同一家的照常接收。
func TestABillLineNamingAnotherSuppliersCostIsRefused(t *testing.T) {
	fixture := newBillFixture(t)
	fixture.costs.costs["cost/v1"] = fixtureExpectedCostOf(t, "cost/v1", 12000, "partner-2")

	refused, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-other"))
	if err != nil {
		t.Fatal(err)
	}
	refusal, has := refused.SupplierRefusal()
	if refused.Outcome() != application.BillExpectedCostSupplierMismatch || !has ||
		refusal.Line.String() != "line-1" || refusal.Version.String() != "cost/v1" || refusal.Supplier.String() != "partner-2" {
		t.Fatalf("outcome = %q refusal = %+v", refused.Outcome(), refusal)
	}
	if refused.Outcome().String() != "EXPECTED_COST_SUPPLIER_MISMATCH" || refused.ContinuationReference() == "" {
		t.Fatalf("答复名 %q 续办引用 %q", refused.Outcome(), refused.ContinuationReference())
	}
	if len(fixture.store.records) != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatal("点名别家成本的账单落了库或交了意图")
	}

	spec := domain.RehydrateSupplierExpectedCostSpec{}
	legacyFrom := fixtureExpectedCost(t, "cost/v1", 12000)
	spec.Version, spec.Occurrence, spec.FeeItem = legacyFrom.Version(), legacyFrom.Occurrence(), legacyFrom.FeeItem()
	spec.RuleVersion, spec.Agreement, spec.Evaluation = legacyFrom.RuleVersion(), legacyFrom.Agreement(), legacyFrom.Evaluation()
	spec.OriginalCurrency, spec.OriginalMinor = legacyFrom.OriginalAmount()
	spec.SettlementCurrency, spec.SettlementMinor = legacyFrom.SettlementAmount()
	legacy, err := domain.RehydrateSupplierExpectedCost(spec)
	if err != nil {
		t.Fatal(err)
	}
	fixture.costs.costs["cost/v1"] = legacy
	unrecorded, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-legacy"))
	if err != nil {
		t.Fatal(err)
	}
	refusal, has = unrecorded.SupplierRefusal()
	if unrecorded.Outcome() != application.BillExpectedCostSupplierUnrecorded || !has || refusal.Version.String() != "cost/v1" ||
		refusal.Supplier.String() != "" || unrecorded.Outcome().String() != "EXPECTED_COST_SUPPLIER_UNRECORDED" {
		t.Fatalf("outcome = %q refusal = %+v", unrecorded.Outcome(), refusal)
	}
	if len(fixture.store.records) != 0 {
		t.Fatal("点名没记供应商的旧行的账单落了库")
	}

	// 点名的那一版已撤下又是别家的：先答不是当前版——那一版本就不该再拿来对账，供应商对不对是后话。
	otherWithdrawn, err := fixtureExpectedCostOf(t, "cost/v1", 12000, "partner-2").Withdraw(billOccurrenceInvalidated(t))
	if err != nil {
		t.Fatal(err)
	}
	fixture.costs.costs["cost/v1"] = otherWithdrawn
	if stale, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-stale")); err != nil ||
		stale.Outcome() != application.BillExpectedCostNotCurrent {
		t.Fatalf("撤下的别家成本 = %q err=%v, want EXPECTED_COST_NOT_CURRENT", stale.Outcome(), err)
	}

	fixture.costs.costs["cost/v1"] = fixtureExpectedCost(t, "cost/v1", 12000)
	if received, err := fixture.handler.Handle(context.Background(), billCommand(t, "bill-own")); err != nil || received.Outcome() != application.BillReceived {
		t.Fatalf("同一家的账单 = %q err=%v", received.Outcome(), err)
	}
}

type backfillCostsDouble struct {
	awaiting []ports.ExpectedCostAwaitingSupplier
	recorded map[string]string
	taken    map[string]bool
	listErr  error
	saveErr  error
}

func (double *backfillCostsDouble) ListExpectedCostsAwaitingSupplier(context.Context, domain.TenantID) ([]ports.ExpectedCostAwaitingSupplier, error) {
	return double.awaiting, double.listErr
}

func (double *backfillCostsDouble) RecordExpectedCostSupplier(
	_ context.Context, _ domain.TenantID, version domain.SupplierCostVersionID, supplier domain.SupplierPartyReference,
) (bool, error) {
	if double.saveErr != nil {
		return false, double.saveErr
	}
	if double.taken[version.String()] {
		return false, nil
	}
	double.recorded[version.String()] = supplier.String()
	return true, nil
}

func awaitingCost(t *testing.T, version, agreement string) ports.ExpectedCostAwaitingSupplier {
	t.Helper()
	return ports.ExpectedCostAwaitingSupplier{
		Version:   billValue(t, domain.NewSupplierCostVersionID, version),
		Agreement: billValue(t, domain.NewSupplierAgreementReference, agreement),
	}
}

// Covers: `AT-SA-237`（编排半边：补记）——按各版的协议问 PC、同一版协议只问一次，只写仍为空的；PC 不认、答出译不过来的留空列出，
// 不挡其余；别人先写了的不算这一次补的；读不动、写不进答未决；没给租户不受理。
func TestBackfillRecordsTheAgreementSupplierOfLegacyCosts(t *testing.T) {
	newHandler := func(costs *backfillCostsDouble, suppliers *agreementSupplierDouble) *application.BackfillExpectedCostSupplierHandler {
		handler, err := application.NewBackfillExpectedCostSupplierHandler(application.BackfillExpectedCostSupplierDeps{Costs: costs, Suppliers: suppliers})
		if err != nil {
			t.Fatal(err)
		}
		return handler
	}
	tenant := application.BackfillExpectedCostSupplierCommand{TenantID: billValue(t, domain.NewTenantID, "tenant-1")}

	costs := &backfillCostsDouble{
		awaiting: []ports.ExpectedCostAwaitingSupplier{
			awaitingCost(t, "cost-a", "agreement-trunk/v1"),
			awaitingCost(t, "cost-b", "agreement-trunk/v1"),
			awaitingCost(t, "cost-c", "agreement-gone/v1"),
			awaitingCost(t, "cost-d", "agreement-lm/v1"),
			awaitingCost(t, "cost-e", "agreement-trunk/v1"),
		},
		recorded: map[string]string{},
		taken:    map[string]bool{"cost-e": true},
	}
	suppliers := newAgreementSupplierDouble()
	suppliers.suppliers["agreement-trunk/v1"] = "SYN-PARTY-TRUNK"
	suppliers.suppliers["agreement-lm/v1"] = "SYN-PARTY-LM"
	suppliers.unknown["agreement-gone/v1"] = true

	result, err := newHandler(costs, suppliers).Handle(context.Background(), tenant)
	if err != nil || result.Outcome != application.ExpectedCostSupplierBackfilled {
		t.Fatalf("outcome = %s err=%v", result.Outcome, err)
	}
	if costs.recorded["cost-a"] != "SYN-PARTY-TRUNK" || costs.recorded["cost-b"] != "SYN-PARTY-TRUNK" || costs.recorded["cost-d"] != "SYN-PARTY-LM" ||
		len(costs.recorded) != 3 || len(result.Recorded) != 3 {
		t.Fatalf("写下 %v，交回 %+v", costs.recorded, result.Recorded)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0].Version.String() != "cost-c" ||
		result.Unresolved[0].Gap != application.ExpectedCostAgreementNotRegistered || result.Unresolved[0].Gap.String() != "AGREEMENT_NOT_REGISTERED" {
		t.Fatalf("补不上的 = %+v", result.Unresolved)
	}
	if len(suppliers.asked) != 3 {
		t.Fatalf("问了 %v，want 每版协议只问一次", suppliers.asked)
	}

	untranslatable := newAgreementSupplierDouble()
	untranslatable.err = ports.ErrSupplierAgreementSupplierUntranslatable
	result, _ = newHandler(&backfillCostsDouble{awaiting: costs.awaiting[:1], recorded: map[string]string{}}, untranslatable).Handle(context.Background(), tenant)
	if result.Outcome != application.ExpectedCostSupplierBackfilled || len(result.Unresolved) != 1 ||
		result.Unresolved[0].Gap != application.ExpectedCostAgreementSupplierUntranslatable {
		t.Fatalf("译不过来 = %s %+v", result.Outcome, result.Unresolved)
	}

	down := errors.New("down")
	unreadable := newAgreementSupplierDouble()
	unreadable.err = down
	for name, testCase := range map[string]struct {
		costs     *backfillCostsDouble
		suppliers *agreementSupplierDouble
	}{
		"PC 读不动": {&backfillCostsDouble{awaiting: costs.awaiting[:1], recorded: map[string]string{}}, unreadable},
		"列不出旧行":  {&backfillCostsDouble{listErr: down}, newAgreementSupplierDouble()},
		"写不进那一格": {&backfillCostsDouble{awaiting: costs.awaiting[:1], recorded: map[string]string{}, saveErr: down}, newAgreementSupplierDouble()},
	} {
		result, err := newHandler(testCase.costs, testCase.suppliers).Handle(context.Background(), tenant)
		if err != nil || result.Outcome != application.ExpectedCostSupplierBackfillUndecided || !errors.Is(result.Cause, down) {
			t.Fatalf("%s: outcome = %s cause=%v err=%v", name, result.Outcome, result.Cause, err)
		}
	}

	if result, _ := newHandler(&backfillCostsDouble{}, newAgreementSupplierDouble()).Handle(context.Background(),
		application.BackfillExpectedCostSupplierCommand{}); result.Outcome != application.ExpectedCostSupplierBackfillNotAccepted {
		t.Fatalf("没给租户 = %s", result.Outcome)
	}
	if _, err := application.NewBackfillExpectedCostSupplierHandler(application.BackfillExpectedCostSupplierDeps{}); err == nil {
		t.Fatal("缺依赖却构造成功")
	}
}

// billOccurrenceInvalidated 是 fixtureExpectedCostOf 那一份发生项的失效版本，给撤下用。
func billOccurrenceInvalidated(t *testing.T) domain.TransportChargeOccurrence {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		billValue(t, domain.NewChargeOccurrenceID, "occurrence-1"),
		billValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		billValue(t, domain.NewOccurrenceVersion, "occurrence/v1-invalidated"),
		billArrivedAt.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	return occurrence
}
