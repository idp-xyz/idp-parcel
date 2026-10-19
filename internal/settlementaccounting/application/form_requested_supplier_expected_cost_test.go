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

// 钉 BUY 形成编排外层（票 demo-chain-gaps/08）：按评价回指读回 BUY 评价请求，以请求上的三件来源引用与由请求派生的
// 版本标识调形成编排本体；重放不多形成；没有回指零写入；回指不在册是矛盾；评价停下的各格是定局、零写入；只有依赖
// 答不出才是未决，并带根因。评价读口与预期成本库用 form_supplier_expected_cost_test.go 的替身。夹具全是合成串。

// buyRequestViewDouble 是 BUY 请求册的只读半边。
type buyRequestViewDouble struct {
	records map[string]ports.EvaluationRequestRecord
	err     error
	calls   int
}

func (double *buyRequestViewDouble) FindByID(_ context.Context, key ports.EvaluationRequestKey) (ports.EvaluationRequestRecord, bool, error) {
	double.calls++
	if double.err != nil {
		return ports.EvaluationRequestRecord{}, false, double.err
	}
	record, found := double.records[key.TenantID.String()+"/"+key.Request.String()]
	return record, found, nil
}

// register 在请求册上登一份请求：发生项 occurrence-7 的 version 版、费用项目 fee-linehaul、协议 agreement-9。
func (double *buyRequestViewDouble) register(t *testing.T, id, version string) {
	t.Helper()
	charge, err := domain.NewTransportChargeOccurrence(
		billValue(t, domain.NewChargeOccurrenceID, "occurrence-7"),
		billValue(t, domain.NewOccurrenceReasonReference, "BOOKING"),
		billValue(t, domain.NewOccurrenceVersion, version),
		costOccurredAt,
	)
	if err != nil {
		t.Fatalf("发生项：%v", err)
	}
	request, err := domain.SubmitEvaluationRequest(domain.EvaluationRequestSpec{
		ID:      billValue(t, domain.NewEvaluationRequestID, id),
		Scope:   billValue(t, domain.NewPrimaryScopeReference, "syn-scope-1"),
		Purpose: domain.BuySupplierCost,
		Sources: domain.EligibleSourceReferences{
			Occurrence: charge,
			FeeItem:    billValue(t, domain.NewFeeItemReference, "fee-linehaul"),
			Agreement:  billValue(t, domain.NewSupplierAgreementReference, "agreement-9"),
		},
		RequestedAt: costOccurredAt.Add(time.Minute),
		RequestedBy: billValue(t, domain.NewRequesterReference, "syn-settlement-job"),
	})
	if err != nil {
		t.Fatalf("请求：%v", err)
	}
	tenant := billValue(t, domain.NewTenantID, "tenant-1")
	double.records[tenant.String()+"/"+id] = ports.EvaluationRequestRecord{
		Key:     ports.EvaluationRequestKey{TenantID: tenant, Request: request.ID()},
		Request: request, RecordedAt: costRecordedAt,
	}
}

type requestedCostFixture struct {
	*expectedCostFixture
	requests *buyRequestViewDouble
	outer    *application.FormRequestedSupplierExpectedCostHandler
}

func newRequestedCostFixture(t *testing.T) *requestedCostFixture {
	t.Helper()
	fixture := &requestedCostFixture{
		expectedCostFixture: newExpectedCostFixture(t),
		requests:            &buyRequestViewDouble{records: map[string]ports.EvaluationRequestRecord{}},
	}
	fixture.requests.register(t, "EVREQ-SYN-1", "occurrence-7/v1")
	outer, err := application.NewFormRequestedSupplierExpectedCostHandler(application.FormRequestedSupplierExpectedCostDeps{
		Requests: fixture.requests, Costs: fixture.handler,
	})
	if err != nil {
		t.Fatalf("外层：%v", err)
	}
	fixture.outer = outer
	return fixture
}

// handle 以回指 request 的采用快照（取自评价读口替身）调外层；request 为空即没有回指。
func (fixture *requestedCostFixture) handle(t *testing.T, evaluation, request string) (application.RequestedSupplierExpectedCostResult, error) {
	t.Helper()
	adoption := fixture.evaluations.adoptions[evaluation]
	if adoption.Evaluation.String() == "" {
		adoption.Evaluation = billValue(t, domain.NewBuyEvaluationReference, evaluation)
	}
	if request != "" {
		adoption.Request = billValue(t, domain.NewEvaluationRequestID, request)
	}
	return fixture.outer.Handle(context.Background(), application.FormRequestedSupplierExpectedCostCommand{
		TenantID:   billValue(t, domain.NewTenantID, "tenant-1"),
		Evaluation: adoption,
	})
}

// Covers: UC-SA-002 步 5 BUY 方向经回指接上——请求上的发生项、费用项目、供应商协议与评价一起形成首版，版本标识由
// 请求派生，金额、换算与规则版本出自评价（AT-SA-176 形状）。同一份再来一次是重放，不多形成。
func TestARequestedBuyEvaluationFormsTheFirstVersionUnderTheDerivedIdentity(t *testing.T) {
	fixture := newRequestedCostFixture(t)

	result, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.RequestedSupplierExpectedCostAdopted {
		t.Fatalf("outcome = %s, want 已采用", result.Outcome())
	}
	cost, present := result.Formation().Cost()
	if !present || cost.Version().String() != "BUYSC-EVREQ-SYN-1" {
		t.Fatalf("成本 = %v present=%v, want 版本 BUYSC-EVREQ-SYN-1", cost.Version(), present)
	}
	if cost.Occurrence().ID().String() != "occurrence-7" || cost.Occurrence().Version().String() != "occurrence-7/v1" ||
		cost.FeeItem().String() != "fee-linehaul" || cost.Agreement().String() != "agreement-9" {
		t.Fatalf("来源引用没取自请求：%+v", cost)
	}
	if currency, minor := cost.SettlementAmount(); currency.String() != "CNY" || minor != 30000 ||
		cost.Evaluation().String() != "buy-eval-1" || cost.RuleVersion().String() != "buy-card/v3" {
		t.Fatalf("金额与评价 = %s %d / %s / %s", currency, minor, cost.Evaluation(), cost.RuleVersion())
	}

	replay, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil || replay.Outcome() != application.RequestedSupplierExpectedCostAdopted ||
		replay.Formation().Outcome() != application.ExpectedCostExistingResult {
		t.Fatalf("重放 = %s / %s err=%v", replay.Outcome(), replay.Formation().Outcome(), err)
	}
	if fixture.registry.saves != 1 || len(fixture.registry.costs) != 1 {
		t.Fatalf("重放多形成了：saves=%d costs=%d", fixture.registry.saves, len(fixture.registry.costs))
	}
}

// Covers: 另一份请求点名同一发生项的新版本、同一费用项目，评价命中同一规则版本——首版已在别的版本标识上，不重复
// 形成；答`已有首版`并交回先到者，零新增（重述走计价纠错，不在本编排）。
func TestASecondRequestOnTheSameOccurrenceFeeItemAndRuleFindsTheFirstVersion(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	fixture.requests.register(t, "EVREQ-SYN-2", "occurrence-7/v2")
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

// Covers: 没有回指的评价不归本编排：不读请求册、不形成。回指了却在册上查不到是两侧矛盾，作错误上抛、零写入。
func TestTheRequestBackReferenceDecidesWhetherThisOrchestrationOwnsTheEvaluation(t *testing.T) {
	fixture := newRequestedCostFixture(t)

	result, err := fixture.handle(t, "buy-eval-1", "")
	if err != nil || result.Outcome() != application.RequestedSupplierExpectedCostNoBuyRequest {
		t.Fatalf("没有回指 = %s err=%v", result.Outcome(), err)
	}
	if fixture.requests.calls != 0 || fixture.registry.saves != 0 {
		t.Fatalf("没有回指却读了请求册 %d 次、写了 %d 次", fixture.requests.calls, fixture.registry.saves)
	}

	_, err = fixture.handle(t, "buy-eval-1", "EVREQ-SYN-404")
	if !errors.Is(err, application.ErrBuyEvaluationRequestNotRecorded) {
		t.Fatalf("回指不在册 err = %v", err)
	}
	if fixture.registry.saves != 0 {
		t.Fatalf("矛盾时写了 %d 次", fixture.registry.saves)
	}
}

// Covers: AT-SA-175、AT-SA-177 与评价停下的其余各格——不可计价、评价冲突、待判断、未形成、跨币种无换算步骤、金额
// 未声明都是这份评价的定局：零写入，形成编排那一层的格照原样交回；不是未决（评价不可变，计价对同一份请求也不再
// 形成第二份评价，重投无益）。
func TestEvaluationsThatCannotBeAdoptedAreSettledWithoutWrites(t *testing.T) {
	for _, grade := range []struct {
		evaluation string
		formation  application.ExpectedCostOutcome
		reason     application.ExpectedCostUndecidedReason
	}{
		{"buy-eval-unratable", application.ExpectedCostUnratable, application.ExpectedCostUndecidedReasonNone},
		{"buy-eval-conflict", application.EvaluationConflict, application.ExpectedCostUndecidedReasonNone},
		{"buy-eval-pending", application.ExpectedCostUndecided, application.EvaluationPending},
		{"buy-eval-not-formed", application.ExpectedCostUndecided, application.EvaluationNotFormed},
		{"buy-eval-unconverted", application.ExpectedCostUndecided, application.ConversionStepMissing},
		{"buy-eval-undeclared", application.ExpectedCostUndecided, application.EvaluationAmountPrecisionUndeclared},
	} {
		t.Run(grade.evaluation, func(t *testing.T) {
			fixture := newRequestedCostFixture(t)
			result, err := fixture.handle(t, grade.evaluation, "EVREQ-SYN-1")
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.RequestedSupplierExpectedCostEvaluationNotAdopted ||
				result.Formation().Outcome() != grade.formation || result.Formation().UndecidedReason() != grade.reason {
				t.Fatalf("outcome = %s / %s / %s, want 采用不了 / %s / %s", result.Outcome(),
					result.Formation().Outcome(), result.Formation().UndecidedReason(), grade.formation, grade.reason)
			}
			if result.UndecidedReason() != application.ExpectedCostUndecidedReasonNone || fixture.registry.saves != 0 {
				t.Fatalf("定局却带了未决原因 %s，或写了 %d 次", result.UndecidedReason(), fixture.registry.saves)
			}
		})
	}
}

// Covers: 依赖答不出才是未决——请求册、评价读口、预期成本库各指名自己的原因，并带依赖原样答出的错误。
func TestDependencyFailuresAreUndecidedWithTheirReasonsAndCauses(t *testing.T) {
	requestsDown := errors.New("request register: connection refused")
	fixture := newRequestedCostFixture(t)
	fixture.requests.err = requestsDown
	result, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil || result.Outcome() != application.RequestedSupplierExpectedCostUndecided ||
		result.UndecidedReason() != application.BuyEvaluationRequestViewUnavailable || !errors.Is(result.UndecidedCause(), requestsDown) {
		t.Fatalf("请求册答不出 = %s / %s / %v err=%v", result.Outcome(), result.UndecidedReason(), result.UndecidedCause(), err)
	}

	viewDown := errors.New("evaluation view: timeout")
	fixture = newRequestedCostFixture(t)
	fixture.evaluations.err = viewDown
	result, err = fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil || result.Outcome() != application.RequestedSupplierExpectedCostUndecided ||
		result.UndecidedReason() != application.EvaluationViewUnavailable || !errors.Is(result.UndecidedCause(), viewDown) {
		t.Fatalf("评价读口答不出 = %s / %s / %v err=%v", result.Outcome(), result.UndecidedReason(), result.UndecidedCause(), err)
	}

	registryDown := errors.New("expected cost registry: deadlock")
	fixture = newRequestedCostFixture(t)
	fixture.registry.saveErr = registryDown
	result, err = fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1")
	if err != nil || result.Outcome() != application.RequestedSupplierExpectedCostUndecided ||
		result.UndecidedReason() != application.ExpectedCostRegistryUnavailable || !errors.Is(result.UndecidedCause(), registryDown) {
		t.Fatalf("预期成本库答不出 = %s / %s / %v err=%v", result.Outcome(), result.UndecidedReason(), result.UndecidedCause(), err)
	}
}

// Covers: 两侧说法矛盾作错误上抛、不作未决——派生版本上已有另一份内容；刚读到`已完成`的评价形成时却未受理（合计为零）。
func TestContradictionsAreErrorsNotUndecided(t *testing.T) {
	fixture := newRequestedCostFixture(t)
	if _, err := fixture.handle(t, "buy-eval-1", "EVREQ-SYN-1"); err != nil {
		t.Fatalf("首版：%v", err)
	}
	fixture.evaluations.adoptions["buy-eval-other"] = completedAdoption(t, "buy-eval-other")
	if _, err := fixture.handle(t, "buy-eval-other", "EVREQ-SYN-1"); !errors.Is(err, application.ErrSupplierCostVersionContradiction) {
		t.Fatalf("同一派生版本换了评价 err = %v", err)
	}

	zero := completedAdoption(t, "buy-eval-zero")
	zero.OriginalMinor, zero.SettlementMinor = 0, 0
	fixture = newRequestedCostFixture(t)
	fixture.evaluations.adoptions["buy-eval-zero"] = zero
	if _, err := fixture.handle(t, "buy-eval-zero", "EVREQ-SYN-1"); !errors.Is(err, application.ErrUnexpectedRequestedExpectedCostFormation) {
		t.Fatalf("合计为零 err = %v", err)
	}
	if fixture.registry.saves != 0 {
		t.Fatalf("矛盾时写了 %d 次", fixture.registry.saves)
	}
}

func TestTheRequestedExpectedCostHandlerRefusesNilDependencies(t *testing.T) {
	fixture := newExpectedCostFixture(t)
	for name, deps := range map[string]application.FormRequestedSupplierExpectedCostDeps{
		"缺请求册":  {Costs: fixture.handler},
		"缺形成编排": {Requests: &buyRequestViewDouble{}},
	} {
		if _, err := application.NewFormRequestedSupplierExpectedCostHandler(deps); !errors.Is(err, application.ErrNilDependency) {
			t.Fatalf("%s：err = %v", name, err)
		}
	}
}
