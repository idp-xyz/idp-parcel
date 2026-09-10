package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-001 步骤 7 与 4B — 商业解析先于逐项 asOf，逐项 asOf 先于财务控制；本类
// 控制采用的时点来自规则包为`接受前财务控制`声明的策略，既不是可达性的那一个，也不是
// 编排自己的时钟。
func TestFinancialControlRunsUnderTheAsOfDeclaredForItsOwnKind(t *testing.T) {
	fixture := newFinancialControlFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceJudgmentAdvanced {
		t.Fatalf("outcome = %q, want ADVANCED", result.Outcome())
	}
	if got, want := fixture.calls, []string{
		"resolve-commercial-basis",
		"form-judgment-as-of",
		"apply-financial-control",
	}; !slices.Equal(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
	if kind := fixture.commercial.lastAsOfQuery.Declared.Kind(); kind != domain.FinancialControlJudgmentKind {
		t.Fatalf("second stage asked for %q, want FINANCIAL_CONTROL——用了可达性那一项的声明", kind)
	}

	requested := fixture.controller.lastAsOf
	if !requested.At().Equal(controlPolicyFormedAsOf) {
		t.Fatalf("control asOf = %v, want the policy-formed %v", requested.At(), controlPolicyFormedAsOf)
	}
	if requested.At().Equal(policyFormedAsOf) {
		t.Fatal("the orchestration reused the reachability asOf for the financial control")
	}
	if requested.At().Equal(handlerClockAt) {
		t.Fatal("the orchestration substituted its own clock for the declared asOf semantics")
	}

	control, present := result.FinancialControlResult()
	if !present || control.Outcome() != domain.FinancialControlHeld {
		t.Fatalf("control = %#v present = %v", control, present)
	}
	if !control.AsOf().At().Equal(controlPolicyFormedAsOf) {
		t.Fatal("the provider did not echo the asOf it controlled under")
	}
	if len(fixture.requests.recordedControl) != 1 {
		t.Fatalf("recorded %d control results, want the adopted one kept on the judgment task", len(fixture.requests.recordedControl))
	}
}

// Covers: UC-PS-001 步骤 4B「不得为全部判断套用一个全局时间」与接受条件「不得默认放行」
// —— 规则包只为可达性声明了策略时，财务控制不得借用它发起。一次已经发出的资金占用收不
// 回来，而未决可以续办。
func TestNoControlIsIssuedUnderAnAsOfNobodyDeclared(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.commercial.declaresFinancialControlAsOf = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if fixture.controller.calls != 0 {
		t.Fatal("a pre-acceptance control was issued under an asOf nobody declared")
	}
	if _, present := result.FinancialControlResult(); present {
		t.Fatal("an undecided round carried a financial control result")
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("an undecided result offers no continuation")
	}
	if result.PendingReason() != application.FinancialControlAsOfNotDeclared {
		t.Fatalf("pending reason = %q, want FINANCIAL_CONTROL_AS_OF_NOT_DECLARED", result.PendingReason())
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; the request left SUBMITTED without an acceptance decision", result.State())
	}
}

// Covers: UC-PS-001 步骤 7「读取合同的版本化财务控制策略」— 策略活在客户合同里，没有
// 唯一商业依据就没有合同可读，因而不发起控制，也不代它答`明确无控制`。
func TestNoControlIsIssuedWithoutAUniqueCommercialBasis(t *testing.T) {
	nonApplicable := map[string]struct {
		applicability domain.CommercialApplicability
		reason        application.JudgmentPendingReason
	}{
		"no applicable basis": {domain.CommerciallyNotApplicable, application.CommercialBasisNotApplicable},
		"resolution pending":  {domain.CommercialApplicabilityUndetermined, application.CommercialBasisUndetermined},
	}

	for name, want := range nonApplicable {
		t.Run(name, func(t *testing.T) {
			fixture := newFinancialControlFixture(t)
			fixture.commercial.applicability = want.applicability

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.AcceptanceJudgmentUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
			}
			if fixture.controller.calls != 0 {
				t.Fatal("a pre-acceptance control was issued without a unique commercial basis")
			}
			if _, present := result.FinancialControlResult(); present {
				t.Fatal("an undecided round carried a financial control result")
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("an undecided result offers no continuation")
			}
			// 缺商业依据与缺时点声明停在不同阶段，未决原因与续办路径都必须不同：用例
			// 要求未决按原因维度分别统计，两条路径并成一条就把两种缺口混作一种。
			if result.PendingReason() != want.reason {
				t.Fatalf("pending reason = %q, want %q", result.PendingReason(), want.reason)
			}
			if result.ContinuationReference().String() == undeclaredAsOfContinuation(t) {
				t.Fatal("a missing commercial basis continues under the same reference as an undeclared asOf")
			}
		})
	}
}

// undeclaredAsOfContinuation 取「规则包未声明本类时点」那条路径的续办引用，供别的未决
// 路径与之比对。
func undeclaredAsOfContinuation(t *testing.T) string {
	t.Helper()
	fixture := newFinancialControlFixture(t)
	fixture.commercial.declaresFinancialControlAsOf = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	return result.ContinuationReference().String()
}

// Covers: CONTEXT「不拥有价格、余额、冻结或信用暴露」与 UC-PS-001 结果语义 — `业务限制`
// 和`明确无控制`都是取得的判断，本步照原样记下，不在这里升格为拒绝。受限那一支自 ADR-0132 起
// 在记录之前要读到受限项的失败处置（正文登 REJECT 也是一种处置），夹具据此摆一行；处置本身
// 不改这里的结论——去不去拒绝仍由接受决定那一步回答。
func TestARestrictiveControlIsRecordedWithoutRejectingTheRequest(t *testing.T) {
	for _, outcome := range []domain.FinancialControlOutcome{
		domain.FinancialControlRestricted,
		domain.FinancialControlNotApplicable,
	} {
		t.Run(outcome.String(), func(t *testing.T) {
			fixture := newFinancialControlFixture(t)
			fixture.controller.outcome = outcome
			fixture.dispositions.found = true
			fixture.dispositions.dispositions = map[domain.ControlItemKind]domain.AdoptedControlDisposition{
				domain.PrepaidFreezeControlItem: adoptedDispositionFor(t, domain.RejectOnControlFailure),
			}

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.AcceptanceJudgmentAdvanced {
				t.Fatalf("outcome = %q; an authoritative control result is progress, not failure", result.Outcome())
			}
			control, present := result.FinancialControlResult()
			if !present || control.Outcome() != outcome {
				t.Fatalf("control outcome = %q, want %q", control.Outcome(), outcome)
			}
			if control.Basis().String() == "" {
				t.Fatal("a non-held control result was adopted without the basis that explains it")
			}
			if result.State() != domain.ShipmentRequestSubmitted {
				t.Fatalf("state = %q; the request left SUBMITTED without an acceptance decision", result.State())
			}
			if fixture.requests.rejected {
				t.Fatal("a financial control result was written straight into a rejection")
			}
		})
	}
}

// Covers: UC-PS-001 接受条件「不得默认放行」与结果语义契约`尚未决定`「依赖不可用……返回
// 未决原因和安全续办引用」— 控制端口调不通时形成未决，并且不得留下任何看起来通过了的
// 痕迹：既不记`明确无控制`，也不记一个空结果。
func TestAnUnavailableControllerIsPendingAndNeverADefaultPass(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.controller.err = errors.New("settlement authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.FinancialControlUnavailable {
		t.Fatalf("pending reason = %q, want FINANCIAL_CONTROL_UNAVAILABLE", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("a stalled dependency left no continuation to resume from")
	}
	if _, present := result.FinancialControlResult(); present {
		t.Fatal("a failed control call still produced a control result")
	}
	if len(fixture.requests.recordedControl) != 0 {
		t.Fatalf("recorded %d control results after the call failed", len(fixture.requests.recordedControl))
	}
}

// Covers: UC-PS-001 步骤 9C「未决时保存当前判断、失败位置和安全续办依据」— 控制结果没能
// 记到任务上就不算推进，接受那一步不该引用一条查不回来的控制。
func TestAControlThatCannotBeRecordedDoesNotAdvanceTheTask(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.requests.err = errors.New("judgment task store unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.PendingReason() != application.JudgmentNotRecorded {
		t.Fatalf("pending reason = %q, want JUDGMENT_NOT_RECORDED", result.PendingReason())
	}
	if _, present := result.FinancialControlResult(); present {
		t.Fatal("an unrecorded control result was handed back as adopted")
	}
}

type financialControlFixture struct {
	handler      *application.AdvanceFinancialControlJudgmentHandler
	commercial   *commercialBasisDouble
	controller   *financialControlDouble
	dispositions *controlDispositionDouble
	requests     *judgmentRequestStore
	repository   *awaitingRequestStore
	calls        []string
}

func newFinancialControlFixture(t *testing.T) *financialControlFixture {
	t.Helper()
	value := &financialControlFixture{}
	record := func(name string) { value.calls = append(value.calls, name) }

	value.commercial = &commercialBasisDouble{
		t:                            t,
		applicability:                domain.CommerciallyApplicable,
		declaresReachabilityAsOf:     true,
		declaresFinancialControlAsOf: true,
		record:                       record,
	}
	value.controller = &financialControlDouble{t: t, outcome: domain.FinancialControlHeld, record: record}
	value.dispositions = &controlDispositionDouble{record: record}
	value.requests = &judgmentRequestStore{}
	value.repository = &awaitingRequestStore{t: t}
	value.handler = application.NewAdvanceFinancialControlJudgmentHandler(
		value.commercial,
		value.controller,
		value.dispositions,
		value.requests,
		value.repository,
		fixedClock{at: handlerClockAt},
	)
	return value
}

// controlDispositionDouble 替处置读口作答：按夹具摆好的行交回，或按开关答不出 / 未形成。
type controlDispositionDouble struct {
	dispositions map[domain.ControlItemKind]domain.AdoptedControlDisposition
	found        bool
	err          error
	record       func(string)
	calls        int
	lastQuery    ports.ControlDispositionQuery
}

func (double *controlDispositionDouble) LoadControlDispositions(
	_ context.Context,
	query ports.ControlDispositionQuery,
) (map[domain.ControlItemKind]domain.AdoptedControlDisposition, bool, error) {
	if double.record != nil {
		double.record("load-control-dispositions")
	}
	double.calls++
	double.lastQuery = query
	if double.err != nil {
		return nil, false, double.err
	}
	return double.dispositions, double.found, nil
}

var _ ports.ControlDispositionView = (*controlDispositionDouble)(nil)

// adoptedDispositionFor 造一份采用引用，供夹具按种类摆行。
func adoptedDispositionFor(
	t *testing.T,
	disposition domain.ControlFailureDisposition,
) domain.AdoptedControlDisposition {
	t.Helper()
	adopted, err := domain.NewAdoptedControlDisposition(
		disposition, mustValue(t, domain.NewControlResponsibilityReference, "CONTRACT-CLAUSE-7"))
	if err != nil {
		t.Fatalf("new adopted control disposition: %v", err)
	}
	return adopted
}

// Covers: ADR-0132 决定三——SA 交回含受限项的结果时，编排在记录之前经本上下文自己的商业缝读受限项的
// 失败处置与责任引用，凭本轮采用的商业解析回指，采用到受限项上随同一份结果落库；成立的结果不问。
func TestARestrictedControlAdoptsItsDispositionsBeforeItIsRecorded(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.controller.outcome = domain.FinancialControlRestricted
	fixture.dispositions.found = true
	fixture.dispositions.dispositions = map[domain.ControlItemKind]domain.AdoptedControlDisposition{
		domain.PrepaidFreezeControlItem: adoptedDispositionFor(t, domain.AuthorizedDispositionOnControlFailure),
	}

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceJudgmentAdvanced {
		t.Fatalf("outcome = %q, want ADVANCED", result.Outcome())
	}
	if got, want := fixture.calls, []string{
		"resolve-commercial-basis",
		"form-judgment-as-of",
		"apply-financial-control",
		"load-control-dispositions",
	}; !slices.Equal(got, want) {
		t.Fatalf("call order = %v, want %v——处置要在控制交回之后、记录之前读", got, want)
	}
	if fixture.dispositions.lastQuery.Resolution.String() != "RES-1" {
		t.Fatalf("disposition query resolution = %q, want the adopted RES-1——读口要凭本轮采用的那次解析回指",
			fixture.dispositions.lastQuery.Resolution)
	}
	if len(fixture.requests.recordedControl) != 1 {
		t.Fatalf("recorded %d control results, want 1", len(fixture.requests.recordedControl))
	}
	recorded := fixture.requests.recordedControl[0]
	if !recorded.AwaitsAuthorizedDisposition() {
		t.Fatal("落库的受限结果没带上采用的处置——Decide 会按过渡口径把它拒掉")
	}
	adopted, present := recorded.Items()[0].AdoptedDisposition()
	if !present || adopted.Responsibility().String() != "CONTRACT-CLAUSE-7" {
		t.Fatalf("recorded item disposition = %+v (present=%v)", adopted, present)
	}
	control, present := result.FinancialControlResult()
	if !present || !control.AwaitsAuthorizedDisposition() {
		t.Fatal("交回的控制结果与落库的那份不一致")
	}
}

// Covers: 同一决定的反面——成立与`明确无控制`没有去向可问，读口一次也不被叫到。
func TestASatisfiedControlDoesNotAskForDispositions(t *testing.T) {
	for _, outcome := range []domain.FinancialControlOutcome{
		domain.FinancialControlHeld, domain.FinancialControlCreditExposed, domain.FinancialControlNotApplicable,
	} {
		t.Run(outcome.String(), func(t *testing.T) {
			fixture := newFinancialControlFixture(t)
			fixture.controller.outcome = outcome

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AcceptanceJudgmentAdvanced {
				t.Fatalf("outcome = %q, want ADVANCED", result.Outcome())
			}
			if fixture.dispositions.calls != 0 {
				t.Fatalf("处置读口被问了 %d 次——%s 没有受限项，无去向可读", fixture.dispositions.calls, outcome)
			}
		})
	}
}

// Covers: ADR-0132 决定三「对不上……停在等待内部续办，不折成任一去向」——读口答不出、正文那一侧没有可读的行、
// 范围下缺受限项那一种类的行，三格各停各的，都不记录结果、都不折成 REJECT 或授权处置，续办路径都是内部重试。
func TestARestrictedControlWithoutAnAdoptableDispositionStopsWithoutRecording(t *testing.T) {
	cases := map[string]struct {
		arrange func(*controlDispositionDouble)
		reason  application.JudgmentPendingReason
	}{
		"view unavailable": {
			arrange: func(double *controlDispositionDouble) { double.err = errors.New("pc unreachable") },
			reason:  application.ControlDispositionUnavailable,
		},
		"nothing registered for the scope": {
			arrange: func(double *controlDispositionDouble) { double.found = false },
			reason:  application.ControlDispositionNotFormed,
		},
		"restricted kind missing on the scope": {
			arrange: func(double *controlDispositionDouble) {
				double.found = true
				double.dispositions = map[domain.ControlItemKind]domain.AdoptedControlDisposition{
					domain.CreditCheckControlItem: adoptedDispositionFor(t, domain.RejectOnControlFailure),
				}
			},
			reason: application.ControlDispositionNotFormed,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFinancialControlFixture(t)
			fixture.controller.outcome = domain.FinancialControlRestricted
			tc.arrange(fixture.dispositions)

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AcceptanceJudgmentUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
			}
			if result.PendingReason() != tc.reason {
				t.Fatalf("pending reason = %q, want %q", result.PendingReason(), tc.reason)
			}
			if len(fixture.requests.recordedControl) != 0 {
				t.Fatal("没读到处置的受限结果被记到了任务上——接受那一步会按过渡口径拒掉它")
			}
			if _, present := result.FinancialControlResult(); present {
				t.Fatal("未决那一轮交回了控制结果")
			}
			if len(fixture.requests.recordedAttempts) != 1 ||
				fixture.requests.recordedAttempts[0].ResumePath() != domain.ResumeByInternalRetry {
				t.Fatalf("attempts = %v, want one attempt on INTERNAL_RETRY——对不上处置只有本方重读推得动",
					fixture.requests.recordedAttempts)
			}
		})
	}
}

func (value *financialControlFixture) command(t *testing.T) application.AdvanceFinancialControlJudgmentCommand {
	t.Helper()
	return application.AdvanceFinancialControlJudgmentCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
	}
}

type financialControlDouble struct {
	t           *testing.T
	outcome     domain.FinancialControlOutcome
	portOutcome ports.PreAcceptanceControlOutcome
	err         error
	record      func(string)
	calls       int
	lastAsOf    domain.JudgmentAsOf
}

func (double *financialControlDouble) ApplyPreAcceptanceFinancialControl(
	_ context.Context,
	request ports.FinancialControlRequest,
) (ports.PreAcceptanceControlAssessment, error) {
	double.t.Helper()
	double.record("apply-financial-control")
	double.calls++
	double.lastAsOf = request.AsOf
	if double.err != nil {
		return ports.PreAcceptanceControlAssessment{}, double.err
	}
	outcome := double.portOutcome
	if outcome == ports.PreAcceptanceControlOutcomeInvalid {
		outcome = ports.PreAcceptanceControlFormed
	}
	if outcome != ports.PreAcceptanceControlFormed {
		// 非`已形成`一律不带结果：带上一份，一次没能执行的控制会看起来像通过了。
		return ports.PreAcceptanceControlAssessment{
			Outcome: outcome,
			Reason:  mustValue(double.t, domain.NewCheckReason, "SAC-"+outcome.String()),
		}, nil
	}

	// `明确无控制`下提供方不形成冻结，也就没有结果标识可交回；夹具按结论挑逐项（financialControlOf）。
	return ports.PreAcceptanceControlAssessment{
		Outcome: ports.PreAcceptanceControlFormed,
		Result:  financialControlOf(double.t, double.outcome, "SAC-1", request.AsOf),
	}, nil
}

var _ ports.PreAcceptanceFinancialController = (*financialControlDouble)(nil)
