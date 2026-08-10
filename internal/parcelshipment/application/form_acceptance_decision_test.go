package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-001 步骤 8「适用硬规则和所需判断全部通过时自动接受」与 AT-PS-033 — 规则
// 没有要求人工复核时系统自行形成接受，不等一个无依据的人工审批。
func TestAllAdoptedJudgmentsPassingFormsAnAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceDecided {
		t.Fatalf("outcome = %q, want DECIDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", result.State())
	}
	decision, present := result.AcceptanceDecision()
	if !present || !decision.Accepted() {
		t.Fatalf("decision = %#v present = %v", decision, present)
	}
	if fixture.requests.saved == nil {
		t.Fatal("an acceptance was formed but never saved")
	}
	if fixture.requests.saved.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("saved state = %q, want ACCEPTED", fixture.requests.saved.State())
	}
}

// Covers: UC-PS-001 接受条件`接受前财务控制`「不得默认放行」— 控制从未形成时接受不成立。
// 这是本编排最容易出错的一步：漏掉那一项校验，Decide 会看到「没有失败也没有待判断」而径直
// 接受，而那是一次以遗漏方式实现的默认放行。
func TestAMissingFinancialControlBlocksAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.controlOutcome = domain.FinancialControlOutcomeInvalid

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; a never-formed control let the request leave SUBMITTED", result.State())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("an undecided round recorded an acceptance decision")
	}
}

// Covers: UC-PS-001 接受条件「每个适用校验组都必须通过」的反面 — 规则包没把接受前财务
// 控制列为适用时，不得凭空塞一项永远满足不了的`无法判定`。一份合同本就不要求财务控制的
// 委托会因此永远接受不了，那是把「不适用」读成了「缺一项」。
func TestAnInapplicableFinancialControlDoesNotDeadlockAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicable = []domain.AcceptanceCheckGroup{domain.NetworkReachabilityCheck}
	fixture.judgments.controlOutcome = domain.FinancialControlOutcomeInvalid

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q; a control the rules never required blocked acceptance", result.State())
	}
}

// Covers: UC-PS-001「任一必需控制不通过时按策略拒绝」— 声明只能增加要求，减不掉失败。
// 规则包没把财务控制列为适用，但控制确实跑出了`业务限制`时，那次失败照样拒掉整份版本。
func TestARestrictedControlStillRejectsEvenWhenTheGroupIsNotDeclaredApplicable(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.applicable = []domain.AcceptanceCheckGroup{domain.NetworkReachabilityCheck}
	fixture.judgments.controlOutcome = domain.FinancialControlRestricted

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; a business restriction was discarded because its group was not declared applicable", result.State())
	}
}

// Covers: UC-PS-001 步骤 8「规则显式要求时进入人工复核」与 CONTEXT 所有权 — 规则包没有
// 声明复核策略时不接受，也不替它在「要求」与「不要求」之间挑一个。
func TestAnUndeclaredManualReviewPolicyBlocksAcceptance(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.commercial.manualReview = domain.ManualReviewNotDeclaredByRules

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.ManualReviewPolicyNotDeclared {
		t.Fatalf("pending reason = %q, want MANUAL_REVIEW_POLICY_NOT_DECLARED", result.PendingReason())
	}
	if fixture.requests.saved != nil {
		t.Fatal("an undecided round saved the request")
	}
}

// Covers: UC-PS-001 接受条件矩阵`标准网络可达性`「资料不足不得映射为不可达」与 AT-PS-006
// —— 资料不足保持未决，绝不写成拒绝。
func TestInsufficientEvidenceLeavesTheRequestSubmittedRatherThanRejected(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() == domain.ShipmentRequestRejected {
		t.Fatal("insufficient evidence was written straight into a rejection")
	}
	if result.Outcome() != application.AcceptanceUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
}

// Covers: UC-PS-001 接受条件矩阵`标准网络可达性`「明确不可达时整份当前提交版本不能直接
// 接受」与 AT-PS-005 — 一个成员不可达拒掉整份版本，不做成员级部分接受。
func TestOneUnreachableMemberRejectsTheWholeSubmissionVersion(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	decision, present := result.AcceptanceDecision()
	if !present || len(decision.FailedChecks()) != 1 {
		t.Fatalf("rejection did not record exactly the failing check: %#v", decision)
	}
}

// Covers: UC-PS-001 AT-PS-035「接受提交未成立时按原关联释放冻结」— 拒绝就是接受确定未成立，
// 冻结不能留在原处占着货主的钱。释放按原控制结果关联发起，本上下文不拥有金额或账户。
func TestARejectionReleasesTheFreezeByItsOriginalAssociation(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d, want exactly 1", fixture.release.calls)
	}
	if fixture.release.controlResultID != "SAC-1" {
		t.Fatalf("released %q, want the original control association SAC-1", fixture.release.controlResultID)
	}
}

// Covers: UC-PS-001 AT-PS-035「接受已经成立…不重复控制或释放合法冻结」— 接受成立时那笔冻结
// 是合法的，转接受后流程，不能在这里放掉。
func TestAnAcceptanceDoesNotReleaseTheFreeze(t *testing.T) {
	fixture := newDecisionFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.release.calls != 0 {
		t.Fatalf("release calls = %d; an accepted request had its lawful freeze released", fixture.release.calls)
	}
}

// Covers: UC-PS-001 AT-PS-035「释放失败…保持补偿未决」与 CONTEXT「撤回提交和释放属于可补偿
// 编排」— 释放失败不回滚已经越过提交边界的拒绝，只把补偿留成可续办。
func TestAFailedReleaseKeepsTheRejectionAndLeavesCompensationPending(t *testing.T) {
	fixture := newDecisionFixture(t)
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
	fixture.release.err = errors.New("settlement authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q; a failed release rolled back a rejection that already crossed the boundary", result.State())
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("a failed release left no continuation to resume the compensation")
	}
}

type decisionFixture struct {
	handler    *application.FormAcceptanceDecisionHandler
	commercial *commercialBasisDouble
	judgments  *recordedJudgmentsDouble
	requests   *decidableRequestStore
	release    *controlReleaseDouble
	recorder   *judgmentRequestStore
}

func newDecisionFixture(t *testing.T) *decisionFixture {
	t.Helper()
	value := &decisionFixture{}
	value.commercial = &commercialBasisDouble{
		t:                            t,
		outcome:                      application.CommercialBasisUnique,
		declaresReachabilityAsOf:     true,
		declaresFinancialControlAsOf: true,
		manualReview:                 domain.ManualReviewNotRequiredByRules,
		record:                       func(string) {},
	}
	value.judgments = &recordedJudgmentsDouble{
		t: t,
		reachability: map[string]domain.ReachabilityValue{
			"parcel-1": domain.ReachabilityReachable,
			"parcel-2": domain.ReachabilityReachable,
		},
		controlOutcome: domain.FinancialControlHeld,
	}
	value.requests = &decidableRequestStore{t: t}
	value.release = &controlReleaseDouble{}
	value.recorder = &judgmentRequestStore{}
	value.handler = application.NewFormAcceptanceDecisionHandler(application.FormAcceptanceDecisionDeps{
		Requests:   value.requests,
		Commercial: value.commercial,
		Judgments:  value.judgments,
		Recorder:   value.recorder,
		Release:    value.release,
		Identities: &decisionIdentityFactory{t: t},
		Clock:      fixedClock{at: handlerClockAt},
	})
	return value
}

// controlReleaseDouble 留住释放请求所携带的原控制关联。断言这个而不是"调用过就行"，是因为
// 释放错一笔冻结与不释放同样糟——货主的另一份委托会被无故解冻。
type controlReleaseDouble struct {
	calls           int
	controlResultID string
	err             error
}

func (double *controlReleaseDouble) ReleasePreAcceptanceControl(
	_ context.Context,
	request ports.ControlReleaseRequest,
) error {
	double.calls++
	double.controlResultID = request.ControlResultID.String()
	return double.err
}

func (value *decisionFixture) command(t *testing.T) application.FormAcceptanceDecisionCommand {
	t.Helper()
	return application.FormAcceptanceDecisionCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
	}
}

// recordedJudgmentsDouble 用 controlOutcome 的零值表示「控制从未形成」，与领域侧同一约定：
// 另设一个布尔会让两处可以互相矛盾。
type recordedJudgmentsDouble struct {
	t              *testing.T
	reachability   map[string]domain.ReachabilityValue
	controlOutcome domain.FinancialControlOutcome
	err            error
}

func (double *recordedJudgmentsDouble) LoadRecordedJudgments(
	_ context.Context,
	_ domain.ShipmentRequestID,
) (ports.RecordedJudgments, error) {
	double.t.Helper()
	if double.err != nil {
		return ports.RecordedJudgments{}, double.err
	}

	recorded := ports.RecordedJudgments{}
	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		value, present := double.reachability[parcel]
		if !present {
			continue
		}
		judgment, err := domain.NewReachabilityJudgment(
			mustValue(double.t, domain.NewReachabilityJudgmentID, "NRJ-"+parcel),
			mustValue(double.t, domain.NewDeclaredParcelID, parcel),
			value,
			declaredAsOfFor(double.t, domain.ReachabilityJudgmentKind, policyFormedAsOf),
		)
		if err != nil {
			double.t.Fatalf("new reachability judgement: %v", err)
		}
		recorded.Reachability = append(recorded.Reachability, judgment)
	}

	if double.controlOutcome != domain.FinancialControlOutcomeInvalid {
		basis := domain.ControlBasisReference{}
		if double.controlOutcome != domain.FinancialControlHeld {
			basis = mustValue(double.t, domain.NewControlBasisReference, "PC-CONTROL-BASIS-1")
		}
		control, err := domain.NewFinancialControlResult(
			mustValue(double.t, domain.NewFinancialControlResultID, "SAC-1"),
			double.controlOutcome,
			basis,
			declaredAsOfFor(double.t, domain.FinancialControlJudgmentKind, controlPolicyFormedAsOf),
		)
		if err != nil {
			double.t.Fatalf("new financial control result: %v", err)
		}
		recorded.FinancialControl = control
	}
	return recorded, nil
}

// decidableRequestStore 交回一份已提交、含两个声明成员的委托，并留住被决定后保存的那一份。
type decidableRequestStore struct {
	t     *testing.T
	saved *domain.ShipmentRequest
	err   error
}

func (store *decidableRequestStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
	return submittedRequest(store.t), true, nil
}

func (store *decidableRequestStore) Insert(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ShipmentRequest,
) error {
	return nil
}

func (store *decidableRequestStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) error {
	if store.err != nil {
		return store.err
	}
	store.saved = &request
	return nil
}

// submittedRequest 直接经领域构造一份含两个声明成员的`已提交`委托。不走提交编排，是因为
// 本编排的被测行为从委托已经存在开始，把建单那一段拉进来只会让失败原因难定位。
func submittedRequest(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	decidedAt := time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	decision, err := domain.NewProductionOwnershipDecision(domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        domain.ProductionAuthorityIDPParcel,
		AdmissionControl: domain.AdmissionControlOpen,
		RuleVersion:      mustValue(t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             decidedAt,
		Validity:         validity(t),
		Revision:         mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       decidedAt,
	})
	if err != nil {
		t.Fatalf("new ownership decision: %v", err)
	}
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		scope.Digest(),
		mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
		decidedAt,
	)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	candidate, err := domain.NewSubmissionCandidate(
		submittedFingerprint(t),
		mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		[]domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
	)
	if err != nil {
		t.Fatalf("new submission candidate: %v", err)
	}
	request, err := domain.SubmitShipmentRequest(domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		TaskID:      mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: decidedAt,
	})
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}
	return request
}

func submittedFingerprint(t *testing.T) domain.SourceSubmissionFingerprint {
	t.Helper()
	value, err := domain.NewSourceSubmissionFingerprint(
		sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		mustValue(t, domain.NewPayloadDigest, "digest-1"),
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 7, 10, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new source fingerprint: %v", err)
	}
	return value
}

func declaredAsOfFor(t *testing.T, kind domain.JudgmentKind, at time.Time) domain.JudgmentAsOf {
	t.Helper()
	asOf, err := domain.NewDeclaredAsOf(kind, at, mustValue(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"))
	if err != nil {
		t.Fatalf("new declared asOf: %v", err)
	}
	return asOf
}

type decisionIdentityFactory struct{ t *testing.T }

func (factory *decisionIdentityFactory) NextAcceptanceDecisionID(
	_ context.Context,
) (domain.AcceptanceDecisionID, error) {
	factory.t.Helper()
	return mustValue(factory.t, domain.NewAcceptanceDecisionID, "decision-1"), nil
}

var (
	_ ports.ShipmentRequestRepository   = (*decidableRequestStore)(nil)
	_ ports.RecordedJudgmentReader      = (*recordedJudgmentsDouble)(nil)
	_ ports.AcceptanceDecisionIdentity  = (*decisionIdentityFactory)(nil)
	_ ports.PreAcceptanceControlRelease = (*controlReleaseDouble)(nil)
)
