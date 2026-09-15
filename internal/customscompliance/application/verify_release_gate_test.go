package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var gateAt = time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)

type gateConditionViewDouble struct {
	findings   []domain.PreconditionFinding
	configured bool
	err        error
}

func (double *gateConditionViewDouble) LoadPreconditionFindings(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DecisionScopeReference,
	_ domain.GuardedAction,
	_ domain.CustomsProcedureReference,
) ([]domain.PreconditionFinding, bool, error) {
	if double.err != nil {
		return nil, false, double.err
	}
	return double.findings, double.configured, nil
}

// dutyRuleViewDouble 是「税费付款」那一道规则行的读替身：found=false 即规则未配置。
type dutyRuleViewDouble struct {
	rule  domain.DutyPaymentGateRule
	found bool
	err   error
}

func (double *dutyRuleViewDouble) LoadDutyPaymentGateRule(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DecisionScopeReference,
	_ domain.GuardedAction,
	_ domain.CustomsProcedureReference,
) (domain.DutyPaymentGateRule, bool, error) {
	if double.err != nil {
		return domain.DutyPaymentGateRule{}, false, double.err
	}
	return double.rule, double.found, nil
}

// currentDutyVerificationDouble 是付款核对册「当前版」读替身：found=false 即该范围没有任何一版核对。
type currentDutyVerificationDouble struct {
	record ports.DutyVerificationRecord
	found  bool
	err    error
}

func (double *currentDutyVerificationDouble) LoadCurrentDutyVerification(
	_ context.Context,
	_ domain.TenantID,
	_ domain.DecisionScopeReference,
) (ports.DutyVerificationRecord, bool, error) {
	if double.err != nil {
		return ports.DutyVerificationRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type gateStoreDouble struct {
	byKey map[ports.GateVerificationKey]domain.ReleaseGateVerification
	saved int
}

func (double *gateStoreDouble) FindByKey(
	_ context.Context,
	key ports.GateVerificationKey,
) (domain.ReleaseGateVerification, bool, error) {
	gate, found := double.byKey[key]
	return gate, found, nil
}

func (double *gateStoreDouble) Save(
	_ context.Context,
	key ports.GateVerificationKey,
	gate domain.ReleaseGateVerification,
) (ports.GateVerificationSaveOutcome, error) {
	if _, exists := double.byKey[key]; exists {
		return ports.GateVerificationAlreadyRecorded, nil
	}
	double.byKey[key] = gate
	double.saved++
	return ports.GateVerificationSaved, nil
}

type gateDownstreamDouble struct {
	intents []ports.GateVerificationHandoffIntent
	err     error
}

func (double *gateDownstreamDouble) HandOffGate(
	_ context.Context,
	intent ports.GateVerificationHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type gateFixture struct {
	handler       *application.VerifyReleaseGateHandler
	conditions    *gateConditionViewDouble
	dutyRules     *dutyRuleViewDouble
	verifications *currentDutyVerificationDouble
	store         *gateStoreDouble
	downstream    *gateDownstreamDouble
}

func fullGateDeps(fixture *gateFixture) application.VerifyReleaseGateDeps {
	return application.VerifyReleaseGateDeps{
		Conditions:        fixture.conditions,
		DutyRules:         fixture.dutyRules,
		DutyVerifications: fixture.verifications,
		Store:             fixture.store,
		Downstream:        fixture.downstream,
		Clock:             fixedClock{at: gateAt},
	}
}

// newGateFixture 的默认规则是「税费付款不构成本动作在本边界的前置条件」——让只钉逐项认定折叠的用例
// 不被这一道的规则拦住；要钉规则本身的用例自己换规则。
func newGateFixture(t *testing.T) *gateFixture {
	t.Helper()
	fixture := &gateFixture{
		conditions:    &gateConditionViewDouble{configured: true},
		dutyRules:     &dutyRuleViewDouble{rule: domain.DutyPaymentNotAPrecondition(), found: true},
		verifications: &currentDutyVerificationDouble{},
		store:         &gateStoreDouble{byKey: map[ports.GateVerificationKey]domain.ReleaseGateVerification{}},
		downstream:    &gateDownstreamDouble{},
	}
	handler, err := application.NewVerifyReleaseGateHandler(fullGateDeps(fixture))
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	fixture.handler = handler
	return fixture
}

func gateCommand(t *testing.T) application.VerifyReleaseGateCommand {
	t.Helper()
	return application.VerifyReleaseGateCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Scope:    mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		Action:   domain.OutboundRelease,
		Boundary: mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
	}
}

func finding(t *testing.T, reference string, state domain.PreconditionState) domain.PreconditionFinding {
	t.Helper()
	return domain.PreconditionFinding{
		Precondition: mustValue(t, domain.NewPreconditionReference, reference),
		State:        state,
	}
}

// synVerificationRecord 合成一版付款核对连同它的幂等键（版本指纹由用例给，因为换版要单独钉）。
func synVerificationRecord(
	t *testing.T,
	coverage domain.DutyCoverage,
	delta domain.DutyDelta,
	validity domain.DutyFactValidity,
	digest string,
) ports.DutyVerificationRecord {
	t.Helper()
	duty := mustValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1")
	funds := mustValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01")
	scope := mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1")
	fundsVersion := mustValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1")
	procedure := mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT")
	verification, err := domain.VerifyDutyPayment(duty, funds, fundsVersion, scope, procedure, coverage, delta, validity, gateAt.Add(-time.Hour))
	if err != nil {
		t.Fatalf("构造合成核对：%v", err)
	}
	return ports.DutyVerificationRecord{
		Key: ports.DutyVerificationKey{
			TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
			Duty:     duty, Funds: funds, Scope: scope, Digest: digest,
		},
		Verification: verification,
		Basis:        "SYN-ASSOCIATION-BASIS",
	}
}

func acceptFullNoDeltaValid(t *testing.T) domain.DutyPaymentGateRule {
	t.Helper()
	rule, err := domain.AcceptDutyPaymentWhen(
		[]domain.DutyCoverage{domain.CoverageFull},
		[]domain.DutyDelta{domain.DeltaNone},
		[]domain.DutyFactValidity{domain.FundsFactValid})
	if err != nil {
		t.Fatalf("构造接受集合规则：%v", err)
	}
	return rule
}

// Covers: CC CONTEXT「放行门禁核对」五值经折叠入册——冲突压过满足与未满足（事实打架
// 不是进度）；混合为部分满足；条件状态变化换指纹换版且两版并存；同状态重复核对返原
// 不出第二版。
func TestGateConclusionsFoldAndVersionByConditionState(t *testing.T) {
	fixture := newGateFixture(t)
	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DISPOSITION/verified", domain.PreconditionMet),
		finding(t, "RESTRICTION/released", domain.PreconditionUnmet),
	}

	partial, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("partial handle: %v", err)
	}
	gate, _ := partial.Gate()
	if partial.Outcome() != application.GateVerificationRecorded ||
		gate.Conclusion() != domain.GatePartiallyMet {
		t.Fatalf("outcome = %q conclusion = %q", partial.Outcome(), gate.Conclusion())
	}

	replay, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.GateVerificationExisting || fixture.store.saved != 1 {
		t.Fatalf("replay = %q saved = %d", replay.Outcome(), fixture.store.saved)
	}

	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DISPOSITION/verified", domain.PreconditionMet),
		finding(t, "RESTRICTION/released", domain.PreconditionMet),
	}
	met, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("met handle: %v", err)
	}
	metGate, _ := met.Gate()
	if met.Outcome() != application.GateVerificationRecorded ||
		metGate.Conclusion() != domain.GateMet || fixture.store.saved != 2 {
		t.Fatalf("outcome = %q conclusion = %q saved = %d", met.Outcome(), metGate.Conclusion(), fixture.store.saved)
	}

	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DISPOSITION/verified", domain.PreconditionConflicting),
		finding(t, "RESTRICTION/released", domain.PreconditionMet),
	}
	conflicting, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("conflicting handle: %v", err)
	}
	conflictGate, _ := conflicting.Gate()
	if conflictGate.Conclusion() != domain.GateConflicting {
		t.Fatalf("conclusion = %q; 冲突必须压过其余状态", conflictGate.Conclusion())
	}
}

// Covers: 编排纪律——目录未登记未决并指名（没有清单的门禁判断无从复核）；空清单折成不适用
// （此动作在此边界不受门禁——如实答案，与未登记分开）；意图失败核对不翻留续办。
func TestGateInventoryGapsStallAndEmptyListsMeanNotApplicable(t *testing.T) {
	fixture := newGateFixture(t)
	fixture.conditions.configured = false

	unconfigured, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("unconfigured handle: %v", err)
	}
	if unconfigured.Outcome() != application.GateVerificationUndecided ||
		unconfigured.UndecidedReason() != application.GateCatalogNotConfigured {
		t.Fatalf("outcome = %q reason = %q", unconfigured.Outcome(), unconfigured.UndecidedReason())
	}

	fixture.conditions.configured = true
	fixture.conditions.findings = nil
	notApplicable, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("not applicable handle: %v", err)
	}
	gate, _ := notApplicable.Gate()
	if gate.Conclusion() != domain.GateNotApplicable {
		t.Fatalf("conclusion = %q", gate.Conclusion())
	}

	fixture.downstream.err = errors.New("downstream unreachable")
	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "DISPOSITION/verified", domain.PreconditionMet),
	}
	held, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("held handle: %v", err)
	}
	if held.Outcome() != application.GateVerificationRecorded || held.HandoffReference() == "" {
		t.Fatalf("outcome = %q ref = %q", held.Outcome(), held.HandoffReference())
	}
}

// Covers: 票 sa-cc/06 判据 1「有核对 → 门禁记录带核对版本引用」——规则登了接受集合、当前核对三态都在
// 集内：「税费付款」那一道判满足、进清单、门禁记录挂读数（三态原值 + 核对版本引用，无合成布尔）；
// 核对换版（指纹变）而折出的判断不变 → 门禁另成一版指向新引用，不是重放。
func TestTheDutyPaymentGateIsJudgedByTheRegisteredRuleAndRecordedByReference(t *testing.T) {
	fixture := newGateFixture(t)
	fixture.dutyRules.rule = acceptFullNoDeltaValid(t)
	fixture.verifications.record = synVerificationRecord(t,
		domain.CoverageFull, domain.DeltaNone, domain.FundsFactValid, "SYN-DIGEST-V1")
	fixture.verifications.found = true
	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "RESTRICTION/released", domain.PreconditionMet),
	}

	first, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	gate, has := first.Gate()
	if first.Outcome() != application.GateVerificationRecorded || !has || gate.Conclusion() != domain.GateMet {
		t.Fatalf("outcome = %q conclusion = %q", first.Outcome(), gate.Conclusion())
	}
	preconditions := gate.Preconditions()
	if len(preconditions) != 2 || preconditions[1] != domain.DutyPaymentPrecondition {
		t.Fatalf("税费付款那一道没进前置条件清单：%v", preconditions)
	}
	reading, applied := gate.DutyPayment()
	if !applied || reading.State != domain.PreconditionMet ||
		reading.Coverage != domain.CoverageFull || reading.Delta != domain.DeltaNone || reading.Validity != domain.FundsFactValid ||
		reading.Verification.Version != "SYN-DIGEST-V1" || reading.Verification.Duty.String() != "SYN-DUTY-01/v1" {
		t.Fatalf("读数走样：%+v applied=%v", reading, applied)
	}

	fixture.verifications.record = synVerificationRecord(t,
		domain.CoverageFull, domain.DeltaNone, domain.FundsFactValid, "SYN-DIGEST-V2")
	second, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("handle after new verification version: %v", err)
	}
	secondGate, _ := second.Gate()
	secondReading, _ := secondGate.DutyPayment()
	if second.Outcome() != application.GateVerificationRecorded || fixture.store.saved != 2 ||
		secondReading.Verification.Version != "SYN-DIGEST-V2" {
		t.Fatalf("核对换版该另成一版门禁：outcome=%q saved=%d version=%q",
			second.Outcome(), fixture.store.saved, secondReading.Verification.Version)
	}
}

// Covers: 三态任一不在接受集合内 → 这一道未满足（是规则判出的结论，不是停点），随其余项折叠；
// 「不构成前置条件」的规则不读核对、不进清单、不挂读数——核对册空着也照常形成。
func TestTheDutyPaymentGateAnswersUnmetByRuleAndIsSkippedWhenNotAPrecondition(t *testing.T) {
	fixture := newGateFixture(t)
	fixture.dutyRules.rule = acceptFullNoDeltaValid(t)
	fixture.verifications.record = synVerificationRecord(t,
		domain.CoveragePartial, domain.DeltaShort, domain.FundsFactValid, "SYN-DIGEST-V1")
	fixture.verifications.found = true
	fixture.conditions.findings = []domain.PreconditionFinding{
		finding(t, "RESTRICTION/released", domain.PreconditionMet),
	}

	unmet, err := fixture.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	gate, _ := unmet.Gate()
	reading, applied := gate.DutyPayment()
	if unmet.Outcome() != application.GateVerificationRecorded || gate.Conclusion() != domain.GatePartiallyMet ||
		!applied || reading.State != domain.PreconditionUnmet || reading.Coverage != domain.CoveragePartial {
		t.Fatalf("部分覆盖 + 不足该判这一道未满足、整体部分满足：outcome=%q conclusion=%q reading=%+v",
			unmet.Outcome(), gate.Conclusion(), reading)
	}

	waived := newGateFixture(t)
	waived.verifications.err = errors.New("must not be read")
	waived.conditions.findings = []domain.PreconditionFinding{
		finding(t, "RESTRICTION/released", domain.PreconditionMet),
	}
	result, err := waived.handler.Handle(context.Background(), gateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	waivedGate, _ := result.Gate()
	if result.Outcome() != application.GateVerificationRecorded || waivedGate.Conclusion() != domain.GateMet ||
		len(waivedGate.Preconditions()) != 1 {
		t.Fatalf("不构成前置条件该跳过这一道：outcome=%q conclusion=%q preconditions=%v",
			result.Outcome(), waivedGate.Conclusion(), waivedGate.Preconditions())
	}
	if _, has := waivedGate.DutyPayment(); has {
		t.Fatal("不构成前置条件却挂了读数")
	}
}

// Covers: 「税费付款」那一道的三个诚实停点各有名、都不是「未满足」（ADR-0137 决定三）：目录里没有
// 这一道的规则行 → 规则未配置；规则要读核对而该范围没有任何一版 → 无核对未决；三态任一为`待确认` /
// `冲突` → 未决指名。另：这一道同时登了认定与规则，说的是两件事，也停下等登记方收掉一样。停点不入册。
func TestTheDutyPaymentGateStopsHonestlyInsteadOfAnsweringUnmet(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*gateFixture)
		want   application.VerifyGateUndecidedReason
	}{
		{"规则未配置", func(f *gateFixture) { f.dutyRules.found = false }, application.DutyPaymentGateRuleNotConfigured},
		{"规则读口故障", func(f *gateFixture) { f.dutyRules.err = errors.New("rule view down") }, application.DutyPaymentGateRuleViewUnavailable},
		{"无核对", func(f *gateFixture) {
			f.dutyRules.rule = acceptFullNoDeltaValid(t)
			f.verifications.found = false
		}, application.DutyVerificationAbsent},
		{"核对读口故障", func(f *gateFixture) {
			f.dutyRules.rule = acceptFullNoDeltaValid(t)
			f.verifications.err = errors.New("verification view down")
		}, application.DutyVerificationViewUnavailable},
		{"差额待确认", func(f *gateFixture) {
			f.dutyRules.rule = acceptFullNoDeltaValid(t)
			f.verifications.record = synVerificationRecord(t, domain.CoverageFull, domain.DeltaPending, domain.FundsFactValid, "SYN-DIGEST-V1")
			f.verifications.found = true
		}, application.DutyVerificationPending},
		{"有效性冲突", func(f *gateFixture) {
			f.dutyRules.rule = acceptFullNoDeltaValid(t)
			f.verifications.record = synVerificationRecord(t, domain.CoverageFull, domain.DeltaNone, domain.FundsFactConflicting, "SYN-DIGEST-V1")
			f.verifications.found = true
		}, application.DutyVerificationPending},
		// 资金事实新版本到达后编排形成的 (a′) 版（覆盖承前、差额 / 有效性都待确认，票 sa-cc/19 裁决 1）成为当前一版：
		// 事实变了、人没重核之前门禁答未决，不放也不判失败（完成判据 (4)）。
		{"新版本到达后待重核的那一版", func(f *gateFixture) {
			f.dutyRules.rule = acceptFullNoDeltaValid(t)
			f.verifications.record = synVerificationRecord(t, domain.CoverageFull, domain.DeltaPending, domain.FundsFactPending, "SYN-DIGEST-V2")
			f.verifications.found = true
		}, application.DutyVerificationPending},
		{"认定与规则并存", func(f *gateFixture) {
			f.conditions.findings = []domain.PreconditionFinding{{Precondition: domain.DutyPaymentPrecondition, State: domain.PreconditionMet}}
		}, application.DutyPaymentFindingRegisteredBesideRule},
	}
	for _, testCase := range cases {
		fixture := newGateFixture(t)
		fixture.conditions.findings = []domain.PreconditionFinding{finding(t, "RESTRICTION/released", domain.PreconditionMet)}
		testCase.mutate(fixture)
		result, err := fixture.handler.Handle(context.Background(), gateCommand(t))
		if err != nil || result.Outcome() != application.GateVerificationUndecided || result.UndecidedReason() != testCase.want {
			t.Fatalf("%s：err=%v outcome=%q reason=%q, want 未决 %q", testCase.name, err, result.Outcome(), result.UndecidedReason(), testCase.want)
		}
		if fixture.store.saved != 0 {
			t.Fatalf("%s：停点落了册", testCase.name)
		}
	}
}

// 构造门：VerifyReleaseGateDeps 每一口各缺一次，构造期就以具名错误停下（形照
// NewDutyPaymentReconciliationHandler）。表里的口要与 Deps 的字段一一对上——加口不加表，这里不会红。
func TestTheGateHandlerNamesWhichDependencyIsMissing(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*application.VerifyReleaseGateDeps)
	}{
		{"gate condition view", func(deps *application.VerifyReleaseGateDeps) { deps.Conditions = nil }},
		{"duty payment gate rule view", func(deps *application.VerifyReleaseGateDeps) { deps.DutyRules = nil }},
		{"current duty verification view", func(deps *application.VerifyReleaseGateDeps) { deps.DutyVerifications = nil }},
		{"gate verification store", func(deps *application.VerifyReleaseGateDeps) { deps.Store = nil }},
		{"gate verification handoff", func(deps *application.VerifyReleaseGateDeps) { deps.Downstream = nil }},
		{"clock", func(deps *application.VerifyReleaseGateDeps) { deps.Clock = nil }},
	}
	for _, testCase := range cases {
		deps := fullGateDeps(newGateFixture(t))
		testCase.mutate(&deps)
		handler, err := application.NewVerifyReleaseGateHandler(deps)
		if !errors.Is(err, application.ErrNilDependency) {
			t.Fatalf("缺 %s：err = %v, want ErrNilDependency", testCase.name, err)
		}
		if !strings.Contains(err.Error(), testCase.name) {
			t.Fatalf("缺 %s：错误没点名那一口：%v", testCase.name, err)
		}
		if handler != nil {
			t.Fatalf("缺 %s：拒了还交出编排", testCase.name)
		}
	}
}
