package parcelpricing_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 BUY 消费门（票 demo-chain-gaps/08；票 sa-cc/01 的消费门接上形成路）：真算的 PP 评价经真 BUY 读口、真形成编排
// 与真外层走一遍，BUY 请求册与预期成本库是内存替身。门只管译引用、分方向与折结果：回指在册的已完成评价形成一份
// 供应商预期成本，版本标识由请求派生、三件来源引用取自请求、金额出自评价；SELL 评价与没有回指的 BUY 评价入账、零
// 写入；评价采用不了的各格入账、零写入；回指不在册是两侧矛盾、原样上抛；评价读不回、依赖答不出是未决并带根因。
// 夹具全为 SYN 卡（S 级）。

var buyGateRecordedAt = time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC)

type buyGateClock struct{}

func (buyGateClock) Now() time.Time { return buyGateRecordedAt }

// buyGateRequests 是 BUY 请求册的只读半边，按（租户/请求）存。
type buyGateRequests struct {
	records map[string]saports.EvaluationRequestRecord
	err     error
}

func (requests *buyGateRequests) FindByID(_ context.Context, key saports.EvaluationRequestKey) (saports.EvaluationRequestRecord, bool, error) {
	if requests.err != nil {
		return saports.EvaluationRequestRecord{}, false, requests.err
	}
	record, found := requests.records[key.TenantID.String()+"/"+key.Request.String()]
	return record, found, nil
}

// buyGateCosts 同时演预期成本读口与登记面：主键按版本，首版唯一按（发生项、费用项目、规则版本）。
type buyGateCosts struct {
	byVersion map[string]sadomain.SupplierExpectedCost
	saveErr   error
}

func (costs *buyGateCosts) LoadExpectedCost(_ context.Context, _ sadomain.TenantID, version sadomain.SupplierCostVersionID) (sadomain.SupplierExpectedCost, bool, error) {
	cost, found := costs.byVersion[version.String()]
	return cost, found, nil
}

func (costs *buyGateCosts) LoadFirstVersion(
	_ context.Context, _ sadomain.TenantID, occurrence sadomain.ChargeOccurrenceID, feeItem sadomain.FeeItemReference,
	ruleVersion sadomain.PurchaseRuleVersionReference,
) (sadomain.SupplierExpectedCost, bool, error) {
	for _, cost := range costs.byVersion {
		if cost.Occurrence().ID() == occurrence && cost.FeeItem() == feeItem && cost.RuleVersion() == ruleVersion {
			return cost, true, nil
		}
	}
	return sadomain.SupplierExpectedCost{}, false, nil
}

func (costs *buyGateCosts) Save(_ context.Context, tenant sadomain.TenantID, cost sadomain.SupplierExpectedCost, _ time.Time) (saports.ExpectedCostSaveOutcome, error) {
	if costs.saveErr != nil {
		return saports.ExpectedCostSaveOutcomeInvalid, costs.saveErr
	}
	if _, exists := costs.byVersion[cost.Version().String()]; exists {
		return saports.ExpectedCostAlreadyRecorded, nil
	}
	if _, exists, _ := costs.LoadFirstVersion(context.Background(), tenant, cost.Occurrence().ID(), cost.FeeItem(), cost.RuleVersion()); exists {
		return saports.ExpectedCostAlreadyRecorded, nil
	}
	costs.byVersion[cost.Version().String()] = cost
	return saports.ExpectedCostSaved, nil
}

type failingBuyView struct {
	err error
}

func (view *failingBuyView) LoadBuyEvaluation(context.Context, sadomain.TenantID, sadomain.BuyEvaluationReference) (saports.BuyEvaluationAdoption, bool, error) {
	return saports.BuyEvaluationAdoption{}, false, view.err
}

type buyGateFixture struct {
	gate     *adapter.FormOnBuyEvaluationRecordedAdapter
	requests *buyGateRequests
	costs    *buyGateCosts
}

// newBuyGateFixture 在 BUY 请求册上登一份 EVREQ-SYN-1（发生项 SYN-OCC-1@v1、费用项目 SYN-FEE-LINEHAUL、协议
// SYN-AGREEMENT-1），评价库装 evaluations。
func newBuyGateFixture(t *testing.T, evaluations ...ppdomain.PricingEvaluation) *buyGateFixture {
	t.Helper()
	tenant := must[sadomain.TenantID](t)(sadomain.NewTenantID("tenant-1"))
	occurrence := must[sadomain.TransportChargeOccurrence](t)(sadomain.NewTransportChargeOccurrence(
		must[sadomain.ChargeOccurrenceID](t)(sadomain.NewChargeOccurrenceID("SYN-OCC-1")),
		must[sadomain.OccurrenceReasonReference](t)(sadomain.NewOccurrenceReasonReference("SYN-REASON-BOOKING")),
		must[sadomain.OccurrenceVersion](t)(sadomain.NewOccurrenceVersion("v1")),
		basisAt,
	))
	request := must[sadomain.EvaluationRequest](t)(sadomain.SubmitEvaluationRequest(sadomain.EvaluationRequestSpec{
		ID:      must[sadomain.EvaluationRequestID](t)(sadomain.NewEvaluationRequestID("EVREQ-SYN-1")),
		Scope:   must[sadomain.PrimaryScopeReference](t)(sadomain.NewPrimaryScopeReference("scope-1")),
		Purpose: sadomain.BuySupplierCost,
		Sources: sadomain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    must[sadomain.FeeItemReference](t)(sadomain.NewFeeItemReference("SYN-FEE-LINEHAUL")),
			Agreement:  must[sadomain.SupplierAgreementReference](t)(sadomain.NewSupplierAgreementReference("SYN-AGREEMENT-1")),
		},
		RequestedAt: basisAt,
		RequestedBy: must[sadomain.RequesterReference](t)(sadomain.NewRequesterReference("SYN-SETTLEMENT-JOB")),
	}))
	fixture := &buyGateFixture{
		requests: &buyGateRequests{records: map[string]saports.EvaluationRequestRecord{
			"tenant-1/EVREQ-SYN-1": {
				Key:     saports.EvaluationRequestKey{TenantID: tenant, Request: request.ID()},
				Request: request, RecordedAt: basisAt,
			},
		}},
		costs: &buyGateCosts{byVersion: map[string]sadomain.SupplierExpectedCost{}},
	}
	view := newAdapter(t, evaluations...)
	fixture.gate = must[*adapter.FormOnBuyEvaluationRecordedAdapter](t)(adapter.NewFormOnBuyEvaluationRecordedAdapter(
		view, fixture.formation(t, view)))
	return fixture
}

// formation 用本夹具的请求册与预期成本库装一只真外层，评价读口由调用方给。
func (fixture *buyGateFixture) formation(t *testing.T, view saports.BuyEvaluationView) *saapplication.FormRequestedSupplierExpectedCostHandler {
	t.Helper()
	inner := saapplication.NewFormSupplierExpectedCostHandler(saapplication.FormSupplierExpectedCostDeps{
		Evaluations: view, Costs: fixture.costs, Registry: fixture.costs, Clock: buyGateClock{},
	})
	return must[*saapplication.FormRequestedSupplierExpectedCostHandler](t)(saapplication.NewFormRequestedSupplierExpectedCostHandler(
		saapplication.FormRequestedSupplierExpectedCostDeps{Requests: fixture.requests, Costs: inner}))
}

func (fixture *buyGateFixture) handle(t *testing.T, evaluation string) error {
	t.Helper()
	return fixture.gate.HandleRecordedBuyEvaluation(t.Context(), sainbox.RecordedBuyEvaluation{
		TenantID: "tenant-1", EvaluationID: evaluation,
	})
}

func roundedBuyCard(t *testing.T) ppdomain.PricingPlanVersion {
	return buyPlan(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, true)
}

// Covers: 回指在册的已完成 BUY 评价经门形成一份供应商预期成本首版：版本标识由请求派生，三件来源引用取自请求，金额与
// 规则版本出自评价（12.5 USD → 1250 分）。同一封再投一次入账，仍只有一份。
func TestTheBuyGateFormsTheSupplierExpectedCostFromTheRequestedEvaluation(t *testing.T) {
	fixture := newBuyGateFixture(t, requestedEvaluation(t, "eval-buy-1", "EVREQ-SYN-1", roundedBuyCard(t), "Z1"))

	if err := fixture.handle(t, "eval-buy-1"); err != nil {
		t.Fatalf("门：%v", err)
	}
	cost, saved := fixture.costs.byVersion["BUYSC-EVREQ-SYN-1"]
	if !saved || len(fixture.costs.byVersion) != 1 {
		t.Fatalf("预期成本库 = %v，want 恰一份 BUYSC-EVREQ-SYN-1", fixture.costs.byVersion)
	}
	if cost.Occurrence().ID().String() != "SYN-OCC-1" || cost.Occurrence().Version().String() != "v1" ||
		cost.FeeItem().String() != "SYN-FEE-LINEHAUL" || cost.Agreement().String() != "SYN-AGREEMENT-1" {
		t.Fatalf("来源引用没取自请求：发生项 %s@%s 项目 %s 协议 %s",
			cost.Occurrence().ID(), cost.Occurrence().Version(), cost.FeeItem(), cost.Agreement())
	}
	if currency, minor := cost.SettlementAmount(); currency.String() != "USD" || minor != 1250 ||
		cost.Evaluation().String() != "eval-buy-1" || cost.RuleVersion().String() != "SYN-BUY-CARD@v2" {
		t.Fatalf("金额与评价 = %s %d / %s / %s", currency, minor, cost.Evaluation(), cost.RuleVersion())
	}
	if err := fixture.handle(t, "eval-buy-1"); err != nil || len(fixture.costs.byVersion) != 1 {
		t.Fatalf("重放：err = %v，成本 %d 份", err, len(fixture.costs.byVersion))
	}
}

// Covers: SELL 评价与没有回指的 BUY 评价都入账、零写入：前者归扇出的 SELL 那一路，后者不是为 BUY 请求形成的。
func TestTheBuyGateLetsSellAndUnreferencedEvaluationsPass(t *testing.T) {
	sell := requestedEvaluation(t, "eval-sell-1", "SELLREQ-SYN-1",
		buyPlan(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), "Z1")
	unreferenced := evaluate(t, "eval-buy-unreferenced", roundedBuyCard(t), packageInput(t, "tenant-1", "Z1"))
	fixture := newBuyGateFixture(t, sell, unreferenced)

	for _, id := range []string{"eval-sell-1", "eval-buy-unreferenced"} {
		if err := fixture.handle(t, id); err != nil {
			t.Fatalf("%s 被当成了错误：%v", id, err)
		}
	}
	if len(fixture.costs.byVersion) != 0 {
		t.Fatalf("零写入被破：成本 %d 份", len(fixture.costs.byVersion))
	}
}

// Covers: 评价采用不了的各格入账、零写入：待判断（分区落空）与金额未声明（卡没声明取整）都是评价自己的定局，评价
// 不可变，计价对同一份请求也不再形成第二份评价，重投这一封不会变（ADR-0094：失败预算只花在会自愈的依赖上）。
func TestTheBuyGatePassesEvaluationsThatCannotBeAdopted(t *testing.T) {
	pending := requestedEvaluation(t, "eval-buy-pending", "EVREQ-SYN-1", roundedBuyCard(t), "Z9")
	undeclared := requestedEvaluation(t, "eval-buy-undeclared", "EVREQ-SYN-1",
		buyPlan(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, false), "Z1")
	for name, evaluation := range map[string]ppdomain.PricingEvaluation{"待判断": pending, "金额未声明": undeclared} {
		t.Run(name, func(t *testing.T) {
			fixture := newBuyGateFixture(t, evaluation)
			if err := fixture.handle(t, evaluation.ID().String()); err != nil {
				t.Fatalf("应入账：%v", err)
			}
			if len(fixture.costs.byVersion) != 0 {
				t.Fatalf("采用不了的评价落了 %d 份成本", len(fixture.costs.byVersion))
			}
		})
	}
}

// Covers: 回指非空却不在 BUY 请求册上是两侧数据矛盾——请求与信封同库先于评价提交，不存在可见性滞后——原样上抛、
// 不是未决、零写入。
func TestABuyEvaluationReferringToAnUnrecordedRequestIsAContradiction(t *testing.T) {
	fixture := newBuyGateFixture(t, requestedEvaluation(t, "eval-buy-stray", "EVREQ-SYN-404", roundedBuyCard(t), "Z1"))

	err := fixture.handle(t, "eval-buy-stray")
	if !errors.Is(err, saapplication.ErrBuyEvaluationRequestNotRecorded) {
		t.Fatalf("err = %v, want ErrBuyEvaluationRequestNotRecorded", err)
	}
	if errors.Is(err, adapter.ErrSupplierCostFormationUndecided) || errors.Is(err, adapter.ErrEvaluationNotVisible) {
		t.Fatalf("err = %v, 两侧矛盾不得译成未决", err)
	}
	if len(fixture.costs.byVersion) != 0 {
		t.Fatalf("矛盾时落了 %d 份成本", len(fixture.costs.byVersion))
	}
}

// Covers: 评价读不回、读口答不出是可见性滞后的未决；请求册、预期成本库答不出是形成未决。未决都带根因，排查看得到
// 依赖答了什么，不只剩原因名。
func TestTheBuyGateUndecidedCarriesItsRootCause(t *testing.T) {
	evaluation := requestedEvaluation(t, "eval-buy-1", "EVREQ-SYN-1", roundedBuyCard(t), "Z1")

	if err := newBuyGateFixture(t).handle(t, "eval-buy-1"); !errors.Is(err, adapter.ErrEvaluationNotVisible) {
		t.Fatalf("评价还读不回 err = %v", err)
	}

	viewDown := errors.New("pricing evaluation store: connection refused")
	fixture := newBuyGateFixture(t)
	view := &failingBuyView{err: viewDown}
	gate := must[*adapter.FormOnBuyEvaluationRecordedAdapter](t)(adapter.NewFormOnBuyEvaluationRecordedAdapter(view, fixture.formation(t, view)))
	err := gate.HandleRecordedBuyEvaluation(t.Context(), sainbox.RecordedBuyEvaluation{TenantID: "tenant-1", EvaluationID: "eval-buy-1"})
	if !errors.Is(err, adapter.ErrEvaluationNotVisible) || !errors.Is(err, viewDown) {
		t.Fatalf("读口答不出 err = %v, want 可见性未决并带根因", err)
	}

	requestsDown := errors.New("buy evaluation request register: connection reset")
	fixture = newBuyGateFixture(t, evaluation)
	fixture.requests.err = requestsDown
	err = fixture.handle(t, "eval-buy-1")
	if !errors.Is(err, adapter.ErrSupplierCostFormationUndecided) || !errors.Is(err, requestsDown) {
		t.Fatalf("请求册答不出 err = %v, want 形成未决并带根因", err)
	}

	registryDown := errors.New("supplier expected cost store: deadlock detected")
	fixture = newBuyGateFixture(t, evaluation)
	fixture.costs.saveErr = registryDown
	err = fixture.handle(t, "eval-buy-1")
	if !errors.Is(err, adapter.ErrSupplierCostFormationUndecided) || !errors.Is(err, registryDown) {
		t.Fatalf("预期成本库答不出 err = %v, want 形成未决并带根因", err)
	}
	if len(fixture.costs.byVersion) != 0 {
		t.Fatalf("未决时落了 %d 份成本", len(fixture.costs.byVersion))
	}
}

// Covers: 提供方交出词汇表之外的内容重投不会变好，原样上抛、不译成未决；空白引用是引用坏了，响亮报错。
func TestTheBuyGateRefusesUntranslatableAnswersAndReferences(t *testing.T) {
	fixture := newBuyGateFixture(t)
	view := &failingBuyView{err: fmt.Errorf("%w: evaluation status %q", adapter.ErrUntranslatableAnswer, "WEIRD")}
	gate := must[*adapter.FormOnBuyEvaluationRecordedAdapter](t)(adapter.NewFormOnBuyEvaluationRecordedAdapter(view, fixture.formation(t, view)))
	err := gate.HandleRecordedBuyEvaluation(t.Context(), sainbox.RecordedBuyEvaluation{TenantID: "tenant-1", EvaluationID: "eval-buy-1"})
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) || errors.Is(err, adapter.ErrEvaluationNotVisible) {
		t.Fatalf("词汇表之外 err = %v, want 原样上抛、不是未决", err)
	}

	for name, recorded := range map[string]sainbox.RecordedBuyEvaluation{
		"空白租户": {TenantID: "   ", EvaluationID: "eval-buy-1"},
		"空白评价": {TenantID: "tenant-1", EvaluationID: " "},
	} {
		t.Run(name, func(t *testing.T) {
			if err := fixture.gate.HandleRecordedBuyEvaluation(t.Context(), recorded); !errors.Is(err, adapter.ErrUntranslatableReference) {
				t.Fatalf("err = %v, want ErrUntranslatableReference", err)
			}
		})
	}
}

func TestTheBuyGateRefusesNilDependencies(t *testing.T) {
	fixture := newBuyGateFixture(t)
	view := newAdapter(t)
	if _, err := adapter.NewFormOnBuyEvaluationRecordedAdapter(nil, fixture.formation(t, view)); err == nil {
		t.Fatal("nil 读口被收下了")
	}
	if _, err := adapter.NewFormOnBuyEvaluationRecordedAdapter(view, nil); err == nil {
		t.Fatal("nil 形成编排被收下了")
	}
}
