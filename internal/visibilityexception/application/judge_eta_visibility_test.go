package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type ruleViewDouble struct {
	basis domain.ETAVisibilityRuleBasis
	err   error
	asked []time.Time
}

func (double *ruleViewDouble) AssessETAVisibilityRule(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CustomerAccountReference,
	at time.Time,
) (domain.ETAVisibilityRuleBasis, error) {
	double.asked = append(double.asked, at)
	if double.err != nil {
		return domain.ETAVisibilityRuleBasis{}, double.err
	}
	return double.basis, nil
}

type judgementKey struct {
	tenant   domain.TenantID
	customer domain.CustomerAccountReference
	parcel   domain.TrackedParcelReference
}

// judgementStoreDouble 只替编排把判断版本记下来，不是库的行为凭据：Save 不做首发与替代的乐观锁，
// 乐观锁、版本行只增不改写与账户隔离以 adapters/postgres 的真库用例为准。
type judgementStoreDouble struct {
	current  map[judgementKey]domain.ETAVisibilityJudgement
	versions map[domain.ETAVisibilityJudgementVersionID]domain.ETAVisibilityJudgement
	findErr  error
	saveErr  error
	saves    int
}

func newJudgementStore() *judgementStoreDouble {
	return &judgementStoreDouble{
		current:  map[judgementKey]domain.ETAVisibilityJudgement{},
		versions: map[domain.ETAVisibilityJudgementVersionID]domain.ETAVisibilityJudgement{},
	}
}

func (double *judgementStoreDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
) (domain.ETAVisibilityJudgement, bool, error) {
	if double.findErr != nil {
		return domain.ETAVisibilityJudgement{}, false, double.findErr
	}
	judgement, found := double.current[judgementKey{tenant: tenant, customer: customer, parcel: parcel}]
	return judgement, found, nil
}

func (double *judgementStoreDouble) FindVersion(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CustomerAccountReference,
	_ domain.TrackedParcelReference,
	version domain.ETAVisibilityJudgementVersionID,
) (domain.ETAVisibilityJudgement, bool, error) {
	judgement, found := double.versions[version]
	return judgement, found, nil
}

func (double *judgementStoreDouble) Save(
	_ context.Context,
	tenant domain.TenantID,
	judgement domain.ETAVisibilityJudgement,
) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.current[judgementKey{tenant: tenant, customer: judgement.Customer(), parcel: judgement.Parcel()}] = judgement
	double.versions[judgement.Version()] = judgement
	double.saves++
	return nil
}

type judgementIdentityDouble struct {
	next int
	err  error
}

func (double *judgementIdentityDouble) NextETAVisibilityJudgementVersionID(
	_ context.Context,
) (domain.ETAVisibilityJudgementVersionID, error) {
	if double.err != nil {
		return domain.ETAVisibilityJudgementVersionID{}, double.err
	}
	double.next++
	return domain.NewETAVisibilityJudgementVersionID("evj-" + string(rune('0'+double.next)))
}

type judgeFixture struct {
	handler    *application.JudgeETAVisibilityHandler
	etas       *etaStoreDouble
	rules      *ruleViewDouble
	judgements *judgementStoreDouble
	identities *judgementIdentityDouble
}

var judgeClockAt = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)

func newJudgeFixture(t *testing.T) *judgeFixture {
	t.Helper()
	fixture := &judgeFixture{
		etas:       newETAStore(),
		rules:      &ruleViewDouble{basis: domain.UnconfiguredETARule()},
		judgements: newJudgementStore(),
		identities: &judgementIdentityDouble{},
	}
	fixture.handler = application.NewJudgeETAVisibilityHandler(application.JudgeETAVisibilityDeps{
		Predictions: fixture.etas,
		Rules:       fixture.rules,
		Judgements:  fixture.judgements,
		Identities:  fixture.identities,
		Clock:       fixedClock{at: judgeClockAt},
	})
	return fixture
}

func (fixture *judgeFixture) hasETA(t *testing.T, version, confidence string) domain.ETAPrediction {
	t.Helper()
	return fixture.hasETAFrom(t, domain.OperatorDerivedETA, version, confidence)
}

// hasETAFrom 把一个来源的当前版直接放进预测库：两路来源并存时，判断读到的就是这样的库。
func (fixture *judgeFixture) hasETAFrom(t *testing.T, source domain.ETASourceKind, version, confidence string) domain.ETAPrediction {
	t.Helper()
	eta, err := domain.FormETAPrediction(domain.ETAPredictionSpec{
		Version:     mustValue(t, domain.NewETAVersionID, version),
		Parcel:      mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:   mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:      source,
		Inputs:      mustValue(t, domain.NewPredictionInputsReference, "inputs/"+version),
		Model:       mustValue(t, domain.NewPredictionModelReference, "eta-model/v4"),
		RangeFrom:   etaClockAt.Add(30 * time.Hour),
		RangeTo:     etaClockAt.Add(42 * time.Hour),
		Confidence:  mustValue(t, domain.NewConfidenceReference, confidence),
		PredictedAt: etaClockAt,
	})
	if err != nil {
		t.Fatalf("form eta: %v", err)
	}
	if err := fixture.etas.Save(context.Background(), mustValue(t, domain.NewTenantID, "tenant-1"), eta); err != nil {
		t.Fatalf("save eta: %v", err)
	}
	fixture.etas.saves = 0
	return eta
}

func judgeCommand(t *testing.T) application.JudgeETAVisibilityCommand {
	t.Helper()
	return application.JudgeETAVisibilityCommand{
		TenantID:  mustValue(t, domain.NewTenantID, "tenant-1"),
		Customer:  mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Parcel:    mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone: mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
	}
}

// Covers: AT-VE-187——内部 ETA 已形成，适用质量、合同和客户服务规则条件明确满足：形成引用候选 ETA 的
// 客户可见 ETA 判断，保存依据版本、获准对客表达与停止展示边界；不形成客户视图或通知（编排的依赖里
// 没有视图库、通知库，这里的结果也只有判断）。
func TestAMetGateJudgesTheCandidateETACustomerVisible(t *testing.T) {
	fixture := newJudgeFixture(t)
	eta := fixture.hasETA(t, "eta-1", "MEDIUM/history-based")
	fixture.rules.basis = viewJudgementRule(t, "MEDIUM/history-based")

	result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ETAVisibilityJudged {
		t.Fatalf("outcome = %q, want JUDGED", result.Outcome())
	}
	judgement, present := result.Judgement()
	if !present {
		t.Fatal("a judged result carries no judgement")
	}
	if judgement.Conclusion() != domain.CustomerVisibleETA || judgement.CandidateVersion() != eta.Version() {
		t.Fatalf("conclusion = %q candidate = %q", judgement.Conclusion(), judgement.CandidateVersion())
	}
	if _, boundary, permitted := judgement.CustomerExpression(); !permitted || boundary.String() == "" {
		t.Fatal("客户可见的判断没有留下获准对客表达与停止展示边界")
	}
	if judgement.JudgedAt() != judgeClockAt {
		t.Fatalf("judged at %v, want the clock's judgement moment", judgement.JudgedAt())
	}
	if len(fixture.rules.asked) != 1 || !fixture.rules.asked[0].Equal(judgeClockAt) {
		t.Fatalf("规则按 %v 取，要与判断时点同一刻 %v", fixture.rules.asked, judgeClockAt)
	}
	if fixture.judgements.saves != 1 || fixture.etas.saves != 0 {
		t.Fatalf("judgement saves = %d, eta saves = %d; 判断只写判断，不改内部预测", fixture.judgements.saves, fixture.etas.saves)
	}
}

// Covers: AT-VE-052——质量明确未达适用客户门槛：保存 ETA 不可见判断、候选 ETA、门槛版本与理由，内部 ETA
// 原样保留。
func TestAGateMissJudgesTheCandidateETANotVisibleAndKeepsTheInternalETA(t *testing.T) {
	fixture := newJudgeFixture(t)
	eta := fixture.hasETA(t, "eta-1", "MEDIUM/history-based")
	fixture.rules.basis = viewJudgementRule(t, "HIGH/carrier-confirmed")

	result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	judgement, _ := result.Judgement()
	if judgement.Conclusion() != domain.ETANotVisible || judgement.Reason() != domain.QualityBelowTheGate {
		t.Fatalf("conclusion = %q reason = %q", judgement.Conclusion(), judgement.Reason())
	}
	kept, found, _ := fixture.etas.FindCurrent(context.Background(), judgeCommand(t).TenantID,
		judgeCommand(t).Parcel, judgeCommand(t).Milestone, eta.Source())
	if !found || kept.Version() != eta.Version() {
		t.Fatal("内部 ETA 没有原样保留")
	}
}

// Covers: AT-VE-170——适用规则缺失或冲突：形成 ETA 可见性待判断，理由分得开缺失与冲突，不按默认值判为可见。
func TestAMissingOrConflictingRuleJudgesPendingWithItsOwnReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		basis  domain.ETAVisibilityRuleBasis
		reason domain.ETAVisibilityReason
	}{
		{"规则缺失", domain.UnconfiguredETARule(), domain.CustomerRuleNotConfigured},
		{"规则冲突", domain.ConflictingETARules(), domain.CustomerRuleInConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newJudgeFixture(t)
			fixture.hasETA(t, "eta-1", "HIGH/carrier-confirmed")
			fixture.rules.basis = tc.basis

			result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			judgement, _ := result.Judgement()
			if judgement.Conclusion() != domain.ETAVisibilityPendingJudgement || judgement.Reason() != tc.reason {
				t.Fatalf("conclusion = %q reason = %q", judgement.Conclusion(), judgement.Reason())
			}
		})
	}
}

// Covers: AT-VE-213——同一候选 ETA 版本、同一规则依据重放：不签新身份、不写库、不换代，答已有判断。
func TestTheSameCandidateAndBasisIsAReplay(t *testing.T) {
	fixture := newJudgeFixture(t)
	fixture.hasETA(t, "eta-1", "MEDIUM/history-based")
	fixture.rules.basis = viewJudgementRule(t, "MEDIUM/history-based")
	first, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	firstJudgement, _ := first.Judgement()
	issued, saved := fixture.identities.next, fixture.judgements.saves

	replay, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.ETAVisibilityExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	if again, _ := replay.Judgement(); again.Version() != firstJudgement.Version() {
		t.Fatal("重放换了判断版本")
	}
	if fixture.identities.next != issued || fixture.judgements.saves != saved {
		t.Fatal("重放签了新身份或写了库")
	}
}

// Covers: AT-VE-213 与 UC-VE-003 规则「新 ETA、质量门槛、合同或客户服务规则版本生效时必须形成新的 ETA 客户可见性判断
// 和替代关系，原判断不覆盖」——新 ETA 版本换版、规则版本换版各自形成替代判断并指回前版，前版按版本读得回。
func TestANewETAVersionOrANewRuleVersionSupersedesTheJudgementKeepingHistory(t *testing.T) {
	fixture := newJudgeFixture(t)
	ctx := context.Background()
	fixture.hasETA(t, "eta-1", "MEDIUM/history-based")
	fixture.rules.basis = viewJudgementRule(t, "MEDIUM/history-based")
	first, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	firstJudgement, _ := first.Judgement()

	// 新 ETA 版本：可信度依据变了，不再达标。
	fixture.hasETA(t, "eta-2", "LOW/sparse")
	second, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Outcome() != application.ETAVisibilityRejudged {
		t.Fatalf("outcome = %q, want REJUDGED", second.Outcome())
	}
	secondJudgement, _ := second.Judgement()
	if prior, superseding := secondJudgement.PriorVersion(); !superseding || prior != firstJudgement.Version() {
		t.Fatalf("prior = %q, want %q", prior, firstJudgement.Version())
	}
	if secondJudgement.Conclusion() != domain.ETANotVisible || secondJudgement.CandidateVersion().String() != "eta-2" {
		t.Fatalf("second = %#v", secondJudgement)
	}
	kept, found, _ := fixture.judgements.FindVersion(ctx, judgeCommand(t).TenantID, judgeCommand(t).Customer,
		judgeCommand(t).Parcel, firstJudgement.Version())
	if !found || kept.Conclusion() != domain.CustomerVisibleETA {
		t.Fatal("原判断没有按版本读回，或被改写")
	}

	// 同一 ETA 版本、规则换版：门槛版本不同即另一份发现，也换代。
	other, err := domain.NewETAVisibilityRule(domain.ETAVisibilityRuleSpec{
		GateVersion:     mustValue(t, domain.NewETAQualityGateVersion, "gate/v2"),
		ServiceRule:     mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v3"),
		TargetMilestone: mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Qualifying:      []domain.ConfidenceReference{mustValue(t, domain.NewConfidenceReference, "LOW/sparse")},
		Expression:      domain.ETAShownAsPointValue,
		StopBoundary:    mustValue(t, domain.NewETAStopShowingBoundary, "stop/x"),
	})
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}
	fixture.rules.basis = domain.ConfiguredETARule(other)
	third, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("third: %v", err)
	}
	thirdJudgement, _ := third.Judgement()
	if third.Outcome() != application.ETAVisibilityRejudged || thirdJudgement.Conclusion() != domain.CustomerVisibleETA {
		t.Fatalf("outcome = %q conclusion = %q", third.Outcome(), thirdJudgement.Conclusion())
	}
}

// qualifyingRule 造一份把若干可信度依据都登为达标的规则，别的项与 viewJudgementRule 一致。
func qualifyingRule(t *testing.T, refs ...string) domain.ETAVisibilityRuleBasis {
	t.Helper()
	return ruleVariant(t, refs[0], func(spec *domain.ETAVisibilityRuleSpec) {
		spec.Qualifying = nil
		for _, ref := range refs {
			spec.Qualifying = append(spec.Qualifying, mustValue(t, domain.NewConfidenceReference, ref))
		}
	})
}

// Covers: AT-VE-051 的选取半边与 UC-VE-003 规则「候选取运营企业派生的当前版，没有才取承运商提供的当前版」——两路来源各有当前版时，
// 判断引用运营企业那一版，来源进候选引用；承运商那一版更新、可信度更高、本来能过门槛，也不顶替。
func TestWhenBothSourcesHaveAnETATheOperatorDerivedOneIsTheJudgementCandidate(t *testing.T) {
	fixture := newJudgeFixture(t)
	fixture.hasETAFrom(t, domain.CarrierProvidedETA, "eta-carrier-1", "HIGH/carrier-confirmed")
	operator := fixture.hasETAFrom(t, domain.OperatorDerivedETA, "eta-op-1", "MEDIUM/history-based")
	fixture.rules.basis = viewJudgementRule(t, "MEDIUM/history-based")

	result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	judgement, present := result.Judgement()
	if !present || result.Outcome() != application.ETAVisibilityJudged {
		t.Fatalf("outcome = %q present = %v", result.Outcome(), present)
	}
	if judgement.CandidateVersion() != operator.Version() || judgement.CandidateSource() != domain.OperatorDerivedETA {
		t.Fatalf("候选 = %q（%v），want 运营企业派生的 %q", judgement.CandidateVersion(), judgement.CandidateSource(), operator.Version())
	}
}

// Covers: AT-VE-051 的选取半边与 AT-VE-213——只有承运商一路时它就是候选；运营企业派生的预测到了，候选换手：形成替代判断指回前版，
// 前一版判断仍读得回、仍引用承运商那一版（来源随候选引用留存）。
func TestACarrierETAIsTheCandidateOnlyUntilAnOperatorDerivedOneArrives(t *testing.T) {
	fixture := newJudgeFixture(t)
	ctx := context.Background()
	fixture.rules.basis = qualifyingRule(t, "MEDIUM/history-based", "HIGH/carrier-confirmed")
	carrier := fixture.hasETAFrom(t, domain.CarrierProvidedETA, "eta-carrier-1", "HIGH/carrier-confirmed")

	first, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	firstJudgement, _ := first.Judgement()
	if first.Outcome() != application.ETAVisibilityJudged ||
		firstJudgement.CandidateVersion() != carrier.Version() || firstJudgement.CandidateSource() != domain.CarrierProvidedETA {
		t.Fatalf("outcome = %q 候选 = %q（%v），want 承运商 %q 的首个判断", first.Outcome(),
			firstJudgement.CandidateVersion(), firstJudgement.CandidateSource(), carrier.Version())
	}

	operator := fixture.hasETAFrom(t, domain.OperatorDerivedETA, "eta-op-1", "MEDIUM/history-based")
	second, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	secondJudgement, _ := second.Judgement()
	if second.Outcome() != application.ETAVisibilityRejudged ||
		secondJudgement.CandidateVersion() != operator.Version() || secondJudgement.CandidateSource() != domain.OperatorDerivedETA {
		t.Fatalf("outcome = %q 候选 = %q（%v），want 换手到运营企业 %q", second.Outcome(),
			secondJudgement.CandidateVersion(), secondJudgement.CandidateSource(), operator.Version())
	}
	if prior, superseding := secondJudgement.PriorVersion(); !superseding || prior != firstJudgement.Version() {
		t.Fatalf("prior = %q, want %q", prior, firstJudgement.Version())
	}
	kept, found, _ := fixture.judgements.FindVersion(ctx, judgeCommand(t).TenantID, judgeCommand(t).Customer,
		judgeCommand(t).Parcel, firstJudgement.Version())
	if !found || kept.CandidateVersion() != carrier.Version() || kept.CandidateSource() != domain.CarrierProvidedETA {
		t.Fatal("换手之后前一版判断读不回，或它的候选引用被改写")
	}
}

// Covers: AT-VE-213 的重放半边与 AT-VE-051 的选取半边——候选取决于来源，不取决于谁后到：运营企业一路是候选时，
// 承运商那一路出新版本，候选没变，判断答已有结果、不换代、不签身份；运营企业一路出新版本，候选版本变了，换代。
func TestANewVersionOfTheNonCandidateSourceLeavesTheJudgementAlone(t *testing.T) {
	fixture := newJudgeFixture(t)
	ctx := context.Background()
	fixture.rules.basis = qualifyingRule(t, "MEDIUM/history-based", "HIGH/carrier-confirmed")
	fixture.hasETAFrom(t, domain.CarrierProvidedETA, "eta-carrier-1", "HIGH/carrier-confirmed")
	fixture.hasETAFrom(t, domain.OperatorDerivedETA, "eta-op-1", "MEDIUM/history-based")
	first, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	firstJudgement, _ := first.Judgement()
	issued, saved := fixture.identities.next, fixture.judgements.saves

	fixture.hasETAFrom(t, domain.CarrierProvidedETA, "eta-carrier-2", "HIGH/carrier-confirmed")
	replay, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("after carrier refresh: %v", err)
	}
	if again, _ := replay.Judgement(); replay.Outcome() != application.ETAVisibilityExistingResult || again.Version() != firstJudgement.Version() {
		t.Fatalf("承运商出新版本后 outcome = %q 判断 %q, want EXISTING_RESULT 且仍是 %q",
			replay.Outcome(), again.Version(), firstJudgement.Version())
	}
	if fixture.identities.next != issued || fixture.judgements.saves != saved {
		t.Fatal("非候选来源的新版本换了判断版本或写了库")
	}

	fixture.hasETAFrom(t, domain.OperatorDerivedETA, "eta-op-2", "MEDIUM/history-based")
	rejudged, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("after operator refresh: %v", err)
	}
	if next, _ := rejudged.Judgement(); rejudged.Outcome() != application.ETAVisibilityRejudged || next.CandidateVersion().String() != "eta-op-2" {
		t.Fatalf("运营企业出新版本后 outcome = %q 候选 = %q, want REJUDGED 且候选 eta-op-2", rejudged.Outcome(), next.CandidateVersion())
	}
}

// Covers: AT-VE-052 与 UC-VE-003 规则「候选取运营企业派生的当前版」——运营企业那一版质量未达门槛、承运商那一版本来能过：
// 判断仍落在运营企业那一版上，答 ETA 不可见，不换一个能过门槛的候选。门槛挑的是结论，不是候选。
func TestTheOperatorDerivedCandidateIsNotSwappedForACarrierETAThatWouldPassTheGate(t *testing.T) {
	fixture := newJudgeFixture(t)
	fixture.rules.basis = viewJudgementRule(t, "HIGH/carrier-confirmed")
	fixture.hasETAFrom(t, domain.CarrierProvidedETA, "eta-carrier-1", "HIGH/carrier-confirmed")
	operator := fixture.hasETAFrom(t, domain.OperatorDerivedETA, "eta-op-1", "LOW/sparse")

	result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	judgement, _ := result.Judgement()
	if judgement.Conclusion() != domain.ETANotVisible || judgement.CandidateVersion() != operator.Version() {
		t.Fatalf("结论 = %q 候选 = %q，want ETA 不可见且引用运营企业 %q", judgement.Conclusion(), judgement.CandidateVersion(), operator.Version())
	}
}

// Covers: AT-VE-214 在两路来源并存时仍成立——规则指名的目标里程碑不是这个里程碑：两路来源的 ETA 都不判，
// 也不借用目标里程碑上别的 ETA；该账户在该包裹上已有的判断不变。
func TestBothSourcesAtAnotherMilestoneThanTheRuleTargetAreNotJudgedAndTheExistingJudgementStays(t *testing.T) {
	fixture := newJudgeFixture(t)
	ctx := context.Background()
	fixture.hasETAFrom(t, domain.CarrierProvidedETA, "eta-carrier-1", "HIGH/carrier-confirmed")
	fixture.hasETAFrom(t, domain.OperatorDerivedETA, "eta-op-1", "MEDIUM/history-based")
	fixture.rules.basis = viewJudgementRule(t, "MEDIUM/history-based")
	first, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	firstJudgement, _ := first.Judgement()
	issued, saved := fixture.identities.next, fixture.judgements.saves

	fixture.rules.basis = ruleVariant(t, "MEDIUM/history-based", func(spec *domain.ETAVisibilityRuleSpec) {
		spec.TargetMilestone = mustValue(t, domain.NewMilestoneReference, "OUT_FOR_DELIVERY")
	})
	result, err := fixture.handler.Handle(ctx, judgeCommand(t))
	if err != nil {
		t.Fatalf("outside: %v", err)
	}
	if result.Outcome() != application.ETAVisibilityOutsideRuleTarget {
		t.Fatalf("outcome = %q, want OUTSIDE_RULE_TARGET", result.Outcome())
	}
	if fixture.identities.next != issued || fixture.judgements.saves != saved {
		t.Fatal("不判的里程碑签了身份或写了库")
	}
	current, found, _ := fixture.judgements.FindCurrent(ctx, judgeCommand(t).TenantID, judgeCommand(t).Customer, judgeCommand(t).Parcel)
	if !found || current.Version() != firstJudgement.Version() {
		t.Fatal("该账户在该包裹上已有的判断被动了")
	}
}

// Covers: AT-VE-214——规则指名的客户可见 ETA 目标里程碑不是这份 ETA 的里程碑：不判、不签身份、不写库。
func TestAnETAForAnotherMilestoneThanTheRuleTargetIsNotJudged(t *testing.T) {
	fixture := newJudgeFixture(t)
	fixture.hasETA(t, "eta-1", "MEDIUM/history-based")
	rule, err := domain.NewETAVisibilityRule(domain.ETAVisibilityRuleSpec{
		GateVersion:     mustValue(t, domain.NewETAQualityGateVersion, "gate/v1"),
		ServiceRule:     mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v3"),
		TargetMilestone: mustValue(t, domain.NewMilestoneReference, "OUT_FOR_DELIVERY"),
		Qualifying:      []domain.ConfidenceReference{mustValue(t, domain.NewConfidenceReference, "MEDIUM/history-based")},
		Expression:      domain.ETAShownAsPointValue,
		StopBoundary:    mustValue(t, domain.NewETAStopShowingBoundary, "stop/x"),
	})
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}
	fixture.rules.basis = domain.ConfiguredETARule(rule)

	result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ETAVisibilityOutsideRuleTarget {
		t.Fatalf("outcome = %q, want OUTSIDE_RULE_TARGET", result.Outcome())
	}
	if fixture.identities.next != 0 || fixture.judgements.saves != 0 {
		t.Fatal("不判的 ETA 仍然签了身份或写了库")
	}
}

// Covers: 没有内部 ETA 就没有可判断的候选（信息不足允许不形成 ETA，UC-VE-003 主流程第 6 步只对「已形成的
// 内部 ETA」作判断）：不判，不问规则，不用计划时间凑一个候选。
func TestWithoutAnInternalETAThereIsNothingToJudge(t *testing.T) {
	fixture := newJudgeFixture(t)

	result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ETAVisibilityNoCandidate {
		t.Fatalf("outcome = %q, want NO_CANDIDATE_ETA", result.Outcome())
	}
	if len(fixture.rules.asked) != 0 || fixture.judgements.saves != 0 {
		t.Fatal("没有候选 ETA 却问了规则或写了判断")
	}
}

// Covers: ADR-0029——读不动才是未决，且各依赖各有自己的恢复动作，不压成一格；任何一格未决都不签身份、不写判断。
func TestUnavailableJudgementDependenciesAreUndecidedUnderTheirOwnReasons(t *testing.T) {
	boom := errors.New("down")
	for _, tc := range []struct {
		name   string
		break_ func(*judgeFixture)
		want   application.JudgeETAVisibilityUndecidedReason
	}{
		{"候选 ETA 库", func(f *judgeFixture) { f.etas.listErr = boom }, application.CandidateETAUnavailable},
		{"规则视图", func(f *judgeFixture) { f.rules.err = boom }, application.ETAVisibilityRuleUnavailable},
		{"判断库读", func(f *judgeFixture) { f.judgements.findErr = boom }, application.ETAVisibilityJudgementStoreUnavailable},
		{"判断库写", func(f *judgeFixture) { f.judgements.saveErr = boom }, application.ETAVisibilityJudgementStoreUnavailable},
		{"身份签发", func(f *judgeFixture) { f.identities.err = boom }, application.ETAVisibilityIdentityUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newJudgeFixture(t)
			fixture.hasETA(t, "eta-1", "MEDIUM/history-based")
			fixture.rules.basis = viewJudgementRule(t, "MEDIUM/history-based")
			tc.break_(fixture)

			result, err := fixture.handler.Handle(context.Background(), judgeCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.ETAVisibilityUndecided || result.UndecidedReason() != tc.want {
				t.Fatalf("outcome = %q reason = %q, want UNDECIDED %q", result.Outcome(), result.UndecidedReason(), tc.want)
			}
			if _, present := result.Judgement(); present || fixture.judgements.saves != 0 {
				t.Fatal("未决却交出了判断或写了库")
			}
		})
	}
}

// Covers: 受理门——租户、账户、包裹、里程碑缺一即未受理，不读任何依赖（一次已发出的查询收不回来）。
func TestACommandMissingItsIdentityIsNotAccepted(t *testing.T) {
	for name, blank := range map[string]func(*application.JudgeETAVisibilityCommand){
		"租户":  func(c *application.JudgeETAVisibilityCommand) { c.TenantID = domain.TenantID{} },
		"账户":  func(c *application.JudgeETAVisibilityCommand) { c.Customer = domain.CustomerAccountReference{} },
		"包裹":  func(c *application.JudgeETAVisibilityCommand) { c.Parcel = domain.TrackedParcelReference{} },
		"里程碑": func(c *application.JudgeETAVisibilityCommand) { c.Milestone = domain.MilestoneReference{} },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newJudgeFixture(t)
			fixture.etas.listErr = errors.New("must not be read")
			command := judgeCommand(t)
			blank(&command)

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil || result.Outcome() != application.ETAVisibilityNotAccepted {
				t.Fatalf("outcome = %q err = %v, want NOT_ACCEPTED", result.Outcome(), err)
			}
		})
	}
}

var _ ports.ETAVisibilityJudgementStore = (*judgementStoreDouble)(nil)

// ruleVariant 造一份规则，baseline 与 viewJudgementRule 一致；change 只改一项，别的项原样。
func ruleVariant(t *testing.T, qualifying string, change func(*domain.ETAVisibilityRuleSpec)) domain.ETAVisibilityRuleBasis {
	t.Helper()
	spec := domain.ETAVisibilityRuleSpec{
		GateVersion:     mustValue(t, domain.NewETAQualityGateVersion, "gate/v1"),
		ServiceRule:     mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v3"),
		TargetMilestone: mustValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Qualifying:      []domain.ConfidenceReference{mustValue(t, domain.NewConfidenceReference, qualifying)},
		Expression:      domain.ETAShownAsTimeRange,
		StopBoundary:    mustValue(t, domain.NewETAStopShowingBoundary, "stop/after-delivered"),
	}
	if change != nil {
		change(&spec)
	}
	rule, err := domain.NewETAVisibilityRule(spec)
	if err != nil {
		t.Fatalf("new rule: %v", err)
	}
	return domain.ConfiguredETARule(rule)
}

// Covers: AT-VE-213 与 UC-VE-003 规则「新 ETA、质量门槛、合同或客户服务规则版本生效时必须形成新的 ETA 客户可见性判断
// 和替代关系，原判断不覆盖」——候选 ETA 版本不变、结论也不变，只要判断带出去的依据（门槛版本、客户服务规则版本、
// 获准对客表达、停止展示边界）变了就换代；依据没变的重放不换代。不可见的判断本来不带表达与停止边界，
// 这两项变化不构成另一份发现。
func TestAChangedRuleBasisSupersedesTheJudgementEvenWhenTheConclusionStaysTheSame(t *testing.T) {
	type change = func(*domain.ETAVisibilityRuleSpec)
	for _, tc := range []struct {
		name       string
		qualifying string
		change     change
		want       application.JudgeETAVisibilityOutcome
	}{
		{"可见：门槛版本变", "MEDIUM/history-based", func(s *domain.ETAVisibilityRuleSpec) {
			s.GateVersion = mustValue(t, domain.NewETAQualityGateVersion, "gate/v2")
		}, application.ETAVisibilityRejudged},
		{"可见：客户服务规则版本变", "MEDIUM/history-based", func(s *domain.ETAVisibilityRuleSpec) {
			s.ServiceRule = mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v4")
		}, application.ETAVisibilityRejudged},
		{"可见：对客表达变", "MEDIUM/history-based", func(s *domain.ETAVisibilityRuleSpec) {
			s.Expression = domain.ETAShownAsPointValue
		}, application.ETAVisibilityRejudged},
		{"可见：停止展示边界变", "MEDIUM/history-based", func(s *domain.ETAVisibilityRuleSpec) {
			s.StopBoundary = mustValue(t, domain.NewETAStopShowingBoundary, "stop/after-72h")
		}, application.ETAVisibilityRejudged},
		{"可见：依据没变", "MEDIUM/history-based", nil, application.ETAVisibilityExistingResult},
		{"不可见：门槛版本变", "HIGH/carrier-confirmed", func(s *domain.ETAVisibilityRuleSpec) {
			s.GateVersion = mustValue(t, domain.NewETAQualityGateVersion, "gate/v2")
		}, application.ETAVisibilityRejudged},
		{"不可见：客户服务规则版本变", "HIGH/carrier-confirmed", func(s *domain.ETAVisibilityRuleSpec) {
			s.ServiceRule = mustValue(t, domain.NewCustomerServiceRuleVersion, "contract-c1/v4")
		}, application.ETAVisibilityRejudged},
		{"不可见：表达与停止边界变（判断本不带）", "HIGH/carrier-confirmed", func(s *domain.ETAVisibilityRuleSpec) {
			s.Expression = domain.ETAShownAsPointValue
			s.StopBoundary = mustValue(t, domain.NewETAStopShowingBoundary, "stop/after-72h")
		}, application.ETAVisibilityExistingResult},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newJudgeFixture(t)
			ctx := context.Background()
			fixture.hasETA(t, "eta-1", "MEDIUM/history-based")
			fixture.rules.basis = ruleVariant(t, tc.qualifying, nil)
			first, err := fixture.handler.Handle(ctx, judgeCommand(t))
			if err != nil {
				t.Fatalf("first: %v", err)
			}
			firstJudgement, _ := first.Judgement()

			fixture.rules.basis = ruleVariant(t, tc.qualifying, tc.change)
			second, err := fixture.handler.Handle(ctx, judgeCommand(t))
			if err != nil {
				t.Fatalf("second: %v", err)
			}

			if second.Outcome() != tc.want {
				t.Fatalf("outcome = %q, want %q", second.Outcome(), tc.want)
			}
			secondJudgement, _ := second.Judgement()
			if secondJudgement.CandidateVersion() != firstJudgement.CandidateVersion() ||
				secondJudgement.Conclusion() != firstJudgement.Conclusion() {
				t.Fatalf("候选版本或结论变了，这一例要的是只变依据：%#v", secondJudgement)
			}
			if tc.want == application.ETAVisibilityExistingResult {
				if secondJudgement.Version() != firstJudgement.Version() || fixture.judgements.saves != 1 {
					t.Fatalf("重放换了代或写了库：version = %q saves = %d", secondJudgement.Version(), fixture.judgements.saves)
				}
				return
			}
			if prior, superseding := secondJudgement.PriorVersion(); !superseding || prior != firstJudgement.Version() {
				t.Fatalf("prior = %q, want %q", prior, firstJudgement.Version())
			}
			if secondJudgement.Version() == firstJudgement.Version() || fixture.judgements.saves != 2 {
				t.Fatalf("换代应签新版本并写库：version = %q saves = %d", secondJudgement.Version(), fixture.judgements.saves)
			}
			kept, found, _ := fixture.judgements.FindVersion(ctx, judgeCommand(t).TenantID, judgeCommand(t).Customer,
				judgeCommand(t).Parcel, firstJudgement.Version())
			if !found || kept.Finding() != firstJudgement.Finding() {
				t.Fatal("原判断没有按版本读回，或被改写")
			}
		})
	}
}
