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
	handlerClockAt   = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
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
		})
	}
}

// Covers: UC-PS-001 一致性 — 依赖调用失败不是业务结果，必须显式浮出而不是压成未决。
func TestDependencyFailureSurfacesRatherThanBecomingAJudgement(t *testing.T) {
	failure := errors.New("reachability authority unavailable")
	fixture := newJudgmentFixture(t)
	fixture.reachability.err = failure

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the dependency failure", err)
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

	value.commercial = &commercialBasisDouble{t: t, outcome: application.CommercialBasisUnique, declaresReachabilityAsOf: true, record: record}
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
	t                        *testing.T
	outcome                  application.CommercialBasisOutcome
	declaresReachabilityAsOf bool
	record                   func(string)
	calls                    int
}

func (double *commercialBasisDouble) ResolveCommercialBasis(
	_ context.Context,
	_ ports.CommercialBasisQuery,
) (domain.CommercialBasisSnapshot, error) {
	double.t.Helper()
	double.record("resolve-commercial-basis")
	double.calls++

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

	snapshot, err := domain.NewCommercialBasisSnapshot(
		mustValue(double.t, domain.NewCommercialResolutionID, "RES-1"),
		mustValue(double.t, domain.NewRulePackageReference, "rules-1/v1"),
		mustValue(double.t, domain.NewCommercialViewRevision, "VIEW-1"),
		policies,
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
	rejected bool
}

func (store *judgmentRequestStore) RecordReachabilityJudgment(
	_ context.Context,
	_ domain.ShipmentRequestID,
	_ domain.ReachabilityJudgment,
) error {
	return nil
}

var (
	_ ports.CommercialBasisResolver    = (*commercialBasisDouble)(nil)
	_ ports.ReachabilityAssessor       = (*reachabilityDouble)(nil)
	_ ports.AcceptanceJudgmentRecorder = (*judgmentRequestStore)(nil)
)
