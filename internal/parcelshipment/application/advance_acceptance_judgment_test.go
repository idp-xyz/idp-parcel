package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	policyFormedAsOf = time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	// 财务控制的时点与可达性的刻意取不同值：两类判断各按规则包为自己声明的策略形成时点，
	// 取值相同的夹具分辨不出「按类取」和「取第一个」。
	controlPolicyFormedAsOf = time.Date(2026, 8, 6, 17, 30, 0, 0, time.UTC)
	handlerClockAt          = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
)

// Covers: UC-PS-001 步骤 4A/4B/6 与 UC-NR-002 — 商业解析先于逐项 asOf，逐项 asOf 先于
// 可达性判断；判断采用的时点来自规则包声明的策略，而不是编排自己的时钟。
func TestAcceptanceJudgmentResolvesBasisThenFormsAsOfThenAssesses(t *testing.T) {
	fixture := newJudgmentFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceJudgmentAdvanced {
		t.Fatalf("outcome = %q, want ADVANCED", result.Outcome())
	}
	if got, want := fixture.calls, []string{"resolve-commercial-basis", "assess-reachability"}; !slices.Equal(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}

	requested := fixture.reachability.lastAsOf
	if !requested.At().Equal(policyFormedAsOf) {
		t.Fatalf("reachability asOf = %v, want the policy-formed %v", requested.At(), policyFormedAsOf)
	}
	if requested.At().Equal(handlerClockAt) {
		t.Fatal("the orchestration substituted its own clock for the declared asOf semantics")
	}
	if requested.PolicyVersion().String() == "" {
		t.Fatal("the formed asOf did not carry the policy version that authorised it")
	}

	judgement, present := result.ReachabilityJudgment()
	if !present || judgement.Value() != domain.ReachabilityReachable {
		t.Fatalf("judgement = %#v present = %v", judgement, present)
	}
	if !judgement.AsOf().At().Equal(policyFormedAsOf) {
		t.Fatal("the provider did not echo the asOf it judged under")
	}
}

// Covers: UC-NR-002「任何实现不得把不可达直接写成委托已拒绝」— 三值判断被记录，委托
// 仍是已提交，接受决定不在本步形成。
func TestUnreachableIsRecordedWithoutRejectingTheRequest(t *testing.T) {
	for _, value := range []domain.ReachabilityValue{domain.ReachabilityUnreachable, domain.ReachabilityInsufficientEvidence} {
		t.Run(value.String(), func(t *testing.T) {
			fixture := newJudgmentFixture(t)
			fixture.reachability.value = value

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.AcceptanceJudgmentAdvanced {
				t.Fatalf("outcome = %q; a three-valued judgement is progress, not failure", result.Outcome())
			}
			judgement, present := result.ReachabilityJudgment()
			if !present || judgement.Value() != value {
				t.Fatalf("judgement value = %q, want %q", judgement.Value(), value)
			}
			if result.State() != domain.ShipmentRequestSubmitted {
				t.Fatalf("state = %q; the request left SUBMITTED without an acceptance decision", result.State())
			}
			if fixture.requests.rejected {
				t.Fatal("a reachability result was written straight into a rejection")
			}
		})
	}
}

// Covers: UC-NR-002 启动条件「商业解析暂时不可用时不能伪造商业资格」— 没有唯一商业依据
// 就不发起可达性判断，委托保持未决。
func TestNoReachabilityRequestWithoutAUniqueCommercialBasis(t *testing.T) {
	nonUnique := map[string]application.CommercialBasisOutcome{
		"no applicable basis":    application.CommercialBasisNotApplicable,
		"applicability conflict": application.CommercialBasisConflict,
		"resolution pending":     application.CommercialBasisPending,
	}

	for name, outcome := range nonUnique {
		t.Run(name, func(t *testing.T) {
			fixture := newJudgmentFixture(t)
			fixture.commercial.outcome = outcome

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.AcceptanceJudgmentUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
			}
			if fixture.reachability.calls != 0 {
				t.Fatal("reachability was assessed without a unique commercial basis")
			}
			if _, present := result.ReachabilityJudgment(); present {
				t.Fatal("an undecided result carried a reachability judgement")
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("an undecided result offers no continuation")
			}
			// 缺商业依据与缺时点声明停在不同阶段，未决原因与续办路径都必须不同：用例
			// 要求未决按原因维度分别统计，两条路径并成一条就把两种缺口混作一种。
			if result.PendingReason() != application.CommercialBasisNotUnique {
				t.Fatalf("pending reason = %q, want COMMERCIAL_BASIS_NOT_UNIQUE", result.PendingReason())
			}
			if result.ContinuationReference().String() == undeclaredReachabilityAsOfContinuation(t) {
				t.Fatal("a missing commercial basis continues under the same reference as an undeclared asOf")
			}
		})
	}
}

// undeclaredReachabilityAsOfContinuation 取「规则包未声明可达性时点」那条路径的续办引用，
// 供别的未决路径与之比对。
func undeclaredReachabilityAsOfContinuation(t *testing.T) string {
	t.Helper()
	fixture := newJudgmentFixture(t)
	fixture.commercial.declaresReachabilityAsOf = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	return result.ContinuationReference().String()
}

// Covers: UC-PS-001 结果语义契约`尚未决定`「依赖不可用……返回未决原因和安全续办引用」与
// AT-PS-008 — 依赖调不通形成本上下文自己的未决，且必须指名是哪个依赖停了。
//
// 它绝不能变成一次判断：可达性权威答不出与它答了`不可达`是两回事，混起来会让一次故障读成
// 这个包裹的网络结论。所以下面既断言未决，也断言没有任何判断被形成或记录。
func TestAnUnavailableAuthorityBecomesPendingAndNeverAJudgement(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.reachability.err = errors.New("reachability authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.ReachabilityAuthorityUnavailable {
		t.Fatalf("pending reason = %q, want REACHABILITY_AUTHORITY_UNAVAILABLE", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("a stalled dependency left no continuation to resume from")
	}
	if _, present := result.ReachabilityJudgment(); present {
		t.Fatal("an unavailable authority still produced a reachability judgement")
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; the request left SUBMITTED without an acceptance decision", result.State())
	}
}

// Covers: UC-PS-001 步骤 9C「未决时保存当前判断、失败位置和安全续办依据」— 判断没能记到
// 任务上就不算推进。交回一个没记下的判断，接受那一步会引用一条查不回来的依据。
func TestAJudgementThatCannotBeRecordedDoesNotAdvanceTheTask(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.requests.err = errors.New("judgment task store unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.JudgmentNotRecorded {
		t.Fatalf("pending reason = %q, want JUDGMENT_NOT_RECORDED", result.PendingReason())
	}
	if _, present := result.ReachabilityJudgment(); present {
		t.Fatal("an unrecorded judgement was handed back as adopted")
	}
}

// Covers: UC-PS-001 结果语义契约`尚未决定`与 AT-PS-008 — 商业解析调不通同样形成未决，
// 且与「解析成功但依据不唯一」用不同原因：前者要重试依赖，后者要补商业缺口。
func TestAnUnavailableCommercialResolverIsPendingUnderItsOwnReason(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.commercial.err = errors.New("commercial resolver unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.PendingReason() != application.CommercialBasisUnavailable {
		t.Fatalf("pending reason = %q, want COMMERCIAL_BASIS_UNAVAILABLE", result.PendingReason())
	}
	if fixture.reachability.calls != 0 {
		t.Fatal("reachability was assessed although the commercial basis never resolved")
	}
}

// Covers: UC-PS-001 步骤 4B「不使用一个全局时间代替」— 规则包未为可达性声明 asOf 策略
// 时不得自行取一个时点，判断不发起。
func TestUndeclaredAsOfPolicyStopsBeforeAssessing(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.commercial.declaresReachabilityAsOf = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if fixture.reachability.calls != 0 {
		t.Fatal("reachability was assessed under an asOf nobody declared")
	}
	if result.PendingReason() != application.ReachabilityAsOfNotDeclared {
		t.Fatalf("pending reason = %q, want REACHABILITY_AS_OF_NOT_DECLARED", result.PendingReason())
	}
}

type judgmentFixture struct {
	handler      *application.AdvanceAcceptanceJudgmentHandler
	commercial   *commercialBasisDouble
	reachability *reachabilityDouble
	requests     *judgmentRequestStore
	calls        []string
}

func newJudgmentFixture(t *testing.T) *judgmentFixture {
	t.Helper()
	value := &judgmentFixture{}
	record := func(name string) { value.calls = append(value.calls, name) }

	value.commercial = &commercialBasisDouble{
		t:                            t,
		outcome:                      application.CommercialBasisUnique,
		declaresReachabilityAsOf:     true,
		declaresFinancialControlAsOf: true,
		record:                       record,
	}
	value.reachability = &reachabilityDouble{t: t, value: domain.ReachabilityReachable, record: record}
	value.requests = &judgmentRequestStore{}
	value.handler = application.NewAdvanceAcceptanceJudgmentHandler(
		value.commercial,
		value.reachability,
		value.requests,
		fixedClock{at: handlerClockAt},
	)
	return value
}

func (value *judgmentFixture) command(t *testing.T) application.AdvanceAcceptanceJudgmentCommand {
	t.Helper()
	return application.AdvanceAcceptanceJudgmentCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		DeclaredParcelID:  mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
	}
}

type commercialBasisDouble struct {
	t                            *testing.T
	outcome                      application.CommercialBasisOutcome
	declaresReachabilityAsOf     bool
	declaresFinancialControlAsOf bool
	manualReview                 domain.ManualReviewPolicy
	applicable                   []domain.AcceptanceCheckGroup
	err                          error
	record                       func(string)
	calls                        int
}

func (double *commercialBasisDouble) ResolveCommercialBasis(
	_ context.Context,
	_ ports.CommercialBasisQuery,
) (domain.CommercialBasisSnapshot, error) {
	double.t.Helper()
	double.record("resolve-commercial-basis")
	double.calls++

	if double.err != nil {
		return domain.CommercialBasisSnapshot{}, double.err
	}
	if double.outcome != application.CommercialBasisUnique {
		return domain.CommercialBasisSnapshot{}, nil
	}
	policies := []domain.DeclaredAsOf{}
	if double.declaresReachabilityAsOf {
		policy, err := domain.NewDeclaredAsOf(
			domain.ReachabilityJudgmentKind,
			policyFormedAsOf,
			mustValue(double.t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
		)
		if err != nil {
			double.t.Fatalf("new declared asOf: %v", err)
		}
		policies = append(policies, policy)
	}
	if double.declaresFinancialControlAsOf {
		policy, err := domain.NewDeclaredAsOf(
			domain.FinancialControlJudgmentKind,
			controlPolicyFormedAsOf,
			mustValue(double.t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
		)
		if err != nil {
			double.t.Fatalf("new declared asOf: %v", err)
		}
		policies = append(policies, policy)
	}

	// 适用集合由规则包声明，因此夹具给出而不是被测代码兜底。默认声明的两组正是今天有
	// 生产者的两组；换一个规则包就换一个集合，生产侧没有默认值。
	groups := double.applicable
	if len(groups) == 0 {
		groups = []domain.AcceptanceCheckGroup{
			domain.PreAcceptanceFinancialControlCheck,
			domain.NetworkReachabilityCheck,
		}
	}
	applicable, err := domain.NewApplicableCheckGroups(groups...)
	if err != nil {
		double.t.Fatalf("new applicable check groups: %v", err)
	}
	snapshot, err := domain.NewCommercialBasisSnapshot(
		mustValue(double.t, domain.NewCommercialResolutionID, "RES-1"),
		mustValue(double.t, domain.NewRulePackageReference, "rules-1/v1"),
		mustValue(double.t, domain.NewCommercialViewRevision, "VIEW-1"),
		policies,
		applicable,
		double.manualReview,
	)
	if err != nil {
		double.t.Fatalf("new commercial basis snapshot: %v", err)
	}
	return snapshot, nil
}

type reachabilityDouble struct {
	t        *testing.T
	value    domain.ReachabilityValue
	err      error
	record   func(string)
	calls    int
	lastAsOf domain.JudgmentAsOf
}

func (double *reachabilityDouble) AssessParcelReachability(
	_ context.Context,
	request ports.ReachabilityRequest,
) (domain.ReachabilityJudgment, error) {
	double.t.Helper()
	double.record("assess-reachability")
	double.calls++
	double.lastAsOf = request.AsOf
	if double.err != nil {
		return domain.ReachabilityJudgment{}, double.err
	}

	judgement, err := domain.NewReachabilityJudgment(
		mustValue(double.t, domain.NewReachabilityJudgmentID, "NRJ-1"),
		request.DeclaredParcelID,
		double.value,
		request.AsOf,
	)
	if err != nil {
		double.t.Fatalf("new reachability judgement: %v", err)
	}
	return judgement, nil
}

type judgmentRequestStore struct {
	rejected        bool
	err             error
	recordedControl []domain.FinancialControlResult
}

func (store *judgmentRequestStore) RecordReachabilityJudgment(
	_ context.Context,
	_ domain.ShipmentRequestID,
	_ domain.ReachabilityJudgment,
) error {
	return store.err
}

func (store *judgmentRequestStore) RecordFinancialControlResult(
	_ context.Context,
	_ domain.ShipmentRequestID,
	result domain.FinancialControlResult,
) error {
	if store.err != nil {
		return store.err
	}
	store.recordedControl = append(store.recordedControl, result)
	return nil
}

var (
	_ ports.CommercialBasisResolver    = (*commercialBasisDouble)(nil)
	_ ports.ReachabilityAssessor       = (*reachabilityDouble)(nil)
	_ ports.AcceptanceJudgmentRecorder = (*judgmentRequestStore)(nil)
)
