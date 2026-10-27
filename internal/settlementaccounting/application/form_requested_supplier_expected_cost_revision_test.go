package application_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件补发生项修订驱动纠错的几格边界（票 demo-chain-gaps/15，ADR-0232 决定八）：同一版发生项上的第二份请求、替代评价换结算币、
// 撤下那一路还没把失效记到结算时的两种乱序。替身同形成编排那一套，夹具全是合成串。

// Covers: ADR-0200 决定三仍成立的那一格——同一版发生项、另一份请求，评价命中同一价卡版本：答已有首版、交回先到者，零新增。
func TestAnotherRequestOnTheSameOccurrenceVersionFindsTheFirstVersion(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	fixture.requests.register(t, "EVREQ-SYN-2", "occurrence-7/v1")
	if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatalf("首版：%v", err)
	}
	fixture.evaluations.adoptions["buy-eval-2"] = completedAdoption(t, "buy-eval-2")

	result, err := fixture.handle(t, "buy-eval-2", "EVREQ-SYN-2")
	if err != nil || result.Outcome() != application.RequestedSupplierExpectedCostFirstVersionExists {
		t.Fatalf("outcome = %s err=%v, want 已有首版", result.Outcome(), err)
	}
	if first, _ := result.Formation().Cost(); first.Version().String() != "BUYSC-EVREQ-SYN-1" || len(fixture.registry.costs) != 1 {
		t.Fatalf("交回 %s，成本 %d 份", first.Version(), len(fixture.registry.costs))
	}
}

// Covers: ADR-0232 越权风险点 11——替代版本的新评价换了合同结算币：纠错版本表达不了，响亮失败，零写入。
func TestASupersedingEvaluationChangingTheSettlementCurrencyIsRaised(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	fixture.requests.register(t, "EVREQ-SYN-3", "occurrence-7/v3")
	if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatalf("首版：%v", err)
	}
	withdraw(t, withdrawalHandler(t, fixture), "occurrence-7/v2", "occurrence-7/v1")
	usd := completedAdoption(t, "buy-eval-3")
	usd.SettlementCurrency = billValue(t, domain.NewCurrencyCode, "USD")
	usd.SettlementMinor = usd.OriginalMinor
	usd.Conversion = domain.ConversionStepReference{}
	fixture.evaluations.adoptions["buy-eval-3"] = usd

	_, err := fixture.handle(t, "buy-eval-3", "EVREQ-SYN-3")
	if !errors.Is(err, application.ErrCorrectionChangesSettlementCurrency) {
		t.Fatalf("err = %v, want ErrCorrectionChangesSettlementCurrency", err)
	}
	if len(fixture.registry.costs) != 2 {
		t.Fatalf("换结算币的评价也落了成本：首版、失效纠错版本之外共 %d 份", len(fixture.registry.costs))
	}
}

// Covers: ADR-0232 决定八「只接在失效纠错版本后面」——撤下那一路还没把 v1 的失效记到结算：替代版本 v3 的评价先到，答未决、零写入；
// 失效记下之后原样重投，接在失效纠错版本后面。
func TestASupersedingEvaluationWaitsForTheInvalidationToReachSettlement(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	fixture.requests.register(t, "EVREQ-SYN-3", "occurrence-7/v3")
	if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatalf("首版：%v", err)
	}
	fixture.evaluations.adoptions["buy-eval-3"] = completedAdoption(t, "buy-eval-3")

	early, err := fixture.handle(t, "buy-eval-3", "EVREQ-SYN-3")
	if err != nil || early.Outcome() != application.RequestedSupplierExpectedCostUndecided ||
		early.Formation().UndecidedReason() != application.OccurrenceInvalidationNotRecorded || len(fixture.registry.costs) != 1 {
		t.Fatalf("先到的 v3 评价 = %s / %s err=%v，成本 %d 份", early.Outcome(), early.Formation().UndecidedReason(), err, len(fixture.registry.costs))
	}

	withdraw(t, withdrawalHandler(t, fixture), "occurrence-7/v2", "occurrence-7/v1")
	later, err := fixture.handle(t, "buy-eval-3", "EVREQ-SYN-3")
	if err != nil || later.Formation().Outcome() != application.ExpectedCostFormed {
		t.Fatalf("重投 = %s err=%v", later.Formation().Outcome(), err)
	}
	cost, _ := later.Formation().Cost()
	if prior, _ := cost.PriorVersion(); prior.String() != "BUYSCW-BUYSC-EVREQ-SYN-1" {
		t.Fatalf("纠错接在 %s 后面，want 失效纠错版本", prior)
	}
}

// Covers: ADR-0232 决定八的另一种乱序——替代版本 v3 的评价已先形成首版，早一版 v1 的评价才到、结算也还没记 v1 失效：不接在链头后面
// （接上去链头就退回 v1），答未决、零写入；失效记下之后原样重投，答已失效、入账，链头仍是 v3 那一版。
func TestALateEvaluationOfAnEarlierVersionIsNotAppendedAfterTheSupersedingOne(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	fixture.requests.register(t, "EVREQ-SYN-3", "occurrence-7/v3")
	fixture.evaluations.adoptions["buy-eval-3"] = completedAdoption(t, "buy-eval-3")
	if result, err := fixture.handle(t, "buy-eval-3", "EVREQ-SYN-3"); err != nil || result.Formation().Outcome() != application.ExpectedCostFormed {
		t.Fatalf("v3 首版：%s %v", result.Formation().Outcome(), err)
	}

	late, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil || late.Outcome() != application.RequestedSupplierExpectedCostUndecided || len(fixture.registry.costs) != 1 {
		t.Fatalf("晚到的 v1 评价 = %s err=%v，成本 %d 份", late.Outcome(), err, len(fixture.registry.costs))
	}

	if result := withdraw(t, withdrawalHandler(t, fixture), "occurrence-7/v2", "occurrence-7/v1"); result.Outcome() != application.SupplierCostWithdrawalNothingFormed {
		t.Fatalf("v2 失效 = %s，依 v3 的链头不该被撤", result.Outcome())
	}
	settled, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil || settled.Outcome() != application.RequestedSupplierExpectedCostOccurrenceInvalidated || len(fixture.registry.costs) != 1 {
		t.Fatalf("重投 = %s err=%v，成本 %d 份", settled.Outcome(), err, len(fixture.registry.costs))
	}
}

// Covers: AT-SA-215 落在纠错那一路——失效之后长出的替代版本 v3，评价合计为零：不接纠错版本，链头仍是失效纠错版本（当前有效
// 预期成本照旧为空），v3 的请求记`负向结束（零额）`（ADR-0306 决定三）。
func TestASupersedingEvaluationTotallingZeroAppendsNoCorrection(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	fixture.requests.register(t, "EVREQ-SYN-3", "occurrence-7/v3")
	if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatalf("首版：%v", err)
	}
	withdraw(t, withdrawalHandler(t, fixture), "occurrence-7/v2", "occurrence-7/v1")
	zero := completedAdoption(t, "buy-eval-3")
	zero.OriginalMinor, zero.SettlementMinor = 0, 0
	fixture.evaluations.adoptions["buy-eval-3"] = zero

	result, err := fixture.handle(t, "buy-eval-3", "EVREQ-SYN-3")
	if err != nil || result.Outcome() != application.RequestedSupplierExpectedCostZeroTotal {
		t.Fatalf("零额替代评价 = %s err=%v", result.Outcome(), err)
	}
	if len(fixture.registry.costs) != 2 {
		t.Fatalf("零额也接了纠错：首版、失效纠错版本之外共 %d 份", len(fixture.registry.costs))
	}
	if record := fixture.requests.records["tenant-1/EVREQ-SYN-3"]; !record.ZeroTotal {
		t.Fatal("v3 的请求没记零额")
	}
}
