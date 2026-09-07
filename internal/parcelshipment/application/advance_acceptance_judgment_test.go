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
	// 三步都要出现且按序：第二阶段在类型和测试中可见是 UC-PC-002「给开发的交接」的要求，缺了它，
	// 「值由谁形成」就退回到编排自己拿时钟顶。
	if got, want := fixture.calls, []string{
		"resolve-commercial-basis",
		"form-judgment-as-of",
		"assess-reachability",
	}; !slices.Equal(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
	if kind := fixture.commercial.lastAsOfQuery.Declared.Kind(); kind != domain.ReachabilityJudgmentKind {
		t.Fatalf("second stage asked for %q, want REACHABILITY——按类取时点被退化成取第一个", kind)
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
	// 判断记在命令指名的提交版本上（ADR-0045 的版本维）：记错版本在替身上不会报错，只会在真库里
	// 让新版本的重判被旧版那份吞成重放。
	if got := fixture.requests.recordedVersions; len(got) != 1 || got[0] != fixture.command(t).SubmissionVersion {
		t.Fatalf("judgement recorded under versions %v, want exactly the command's %s", got, fixture.command(t).SubmissionVersion)
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
	// 两支各有各的未决原因：`确定不适用`是权威说了没有适用依据，`解析未决`是权威没得出答案。
	// 压成一格会让一次读取失败看起来像这个客户没有合同。
	nonApplicable := map[string]struct {
		applicability domain.CommercialApplicability
		reason        application.JudgmentPendingReason
	}{
		"no applicable basis": {domain.CommerciallyNotApplicable, application.CommercialBasisNotApplicable},
		"resolution pending":  {domain.CommercialApplicabilityUndetermined, application.CommercialBasisUndetermined},
	}

	for name, want := range nonApplicable {
		t.Run(name, func(t *testing.T) {
			fixture := newJudgmentFixture(t)
			fixture.commercial.applicability = want.applicability

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
			if result.PendingReason() != want.reason {
				t.Fatalf("pending reason = %q, want %q", result.PendingReason(), want.reason)
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

// Covers: UC-PS-001「建立或续办独立接受判断任务并追加判断与处理尝试」— 没能推进的一轮也
// 要在任务上留下记录，且记录带的原因与续办引用要与交回调用方的那一份一致。两处不一致，
// 调用方按引用查回来的就是另一轮。
func TestAnUndecidedRoundLeavesAResumableAttemptOnTheTask(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.commercial.err = errors.New("commercial resolver unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	attempts := fixture.requests.recordedAttempts
	if len(attempts) != 1 {
		t.Fatalf("recorded attempts = %d, want 1", len(attempts))
	}
	if attempts[0].Reason().String() != result.PendingReason().String() {
		t.Fatalf(
			"attempt reason = %q, result reason = %q; the task records a different round than the caller was told",
			attempts[0].Reason(), result.PendingReason(),
		)
	}
	if attempts[0].ContinuationReference() != result.ContinuationReference() {
		t.Fatal("the attempt carries a different continuation than the one handed back")
	}
	// 依赖抖动不是客户的资料缺口。记成客户补充会让系统去催客户补一份它并不缺的资料。
	if attempts[0].ResumePath() != domain.ResumeByInternalRetry {
		t.Fatalf("resume path = %q, want INTERNAL_RETRY", attempts[0].ResumePath())
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

// Covers: ADR-0025「翻译必须是全函数：每个取值都要有明确落点」与 ADR-0027 — 第二阶段四种
// 未成形要采取的动作互不相同：`未配置`等租户把 `PAR-COM-14` 的时点策略登记上，`未决`等依赖
// 恢复，`值被拒`要本方改这次请求，`依据未解析`要回第一阶段重解。压成一格，一个没配置的租户
// 参数就会被无休止内部重试，而重试永远等不到一次登记。
func TestEachUnformedAsOfOutcomeStallsUnderItsOwnReason(t *testing.T) {
	cases := map[ports.JudgmentAsOfOutcome]application.JudgmentPendingReason{
		ports.JudgmentAsOfBasisNotResolved: application.ReachabilityAsOfBasisNotResolved,
		ports.JudgmentAsOfNotConfigured:    application.ReachabilityAsOfNotConfigured,
		ports.JudgmentAsOfPending:          application.ReachabilityAsOfUnavailable,
		ports.JudgmentAsOfValueRejected:    application.ReachabilityAsOfValueRejected,
		ports.JudgmentAsOfInputNotAccepted: application.ReachabilityAsOfInputNotAccepted,
	}

	seen := make(map[string]ports.JudgmentAsOfOutcome, len(cases))
	for outcome, want := range cases {
		t.Run(outcome.String(), func(t *testing.T) {
			fixture := newJudgmentFixture(t)
			fixture.commercial.asOfOutcome = outcome

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.AcceptanceJudgmentUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
			}
			if result.PendingReason() != want {
				t.Fatalf("pending reason = %q, want %q", result.PendingReason(), want)
			}
			if fixture.reachability.calls != 0 {
				t.Fatal("时点没能形成，可达性权威却已经被问过了")
			}
		})

		// 续办引用由原因派生，四种未成形因此必须落在四个引用上——否则调用方按引用查回来的
		// 是另一种缺口，催的也是另一个人。
		fixture := newJudgmentFixture(t)
		fixture.commercial.asOfOutcome = outcome
		result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		reference := result.ContinuationReference().String()
		if reference == "" {
			t.Fatalf("%q 未决却没有续办引用", outcome)
		}
		if clash, exists := seen[reference]; exists {
			t.Fatalf("%q 与 %q 共用续办引用 %q", outcome, clash, reference)
		}
		seen[reference] = outcome
	}
}

// Covers: ADR-0025「翻译必须是全函数」与 UC-NR-002 —「请求冲突」和「未受理」是业务答案而非
// 技术故障，调用方必须能据以纠正这次请求。它们与「未形成判断」压成一格时，前两者会被当成
// 故障走内部重试，而重试改不了一个拼错或冲突的请求。
func TestARejectedRequestIsAnAnswerNotAFailure(t *testing.T) {
	cases := map[ports.ReachabilityOutcome]application.JudgmentPendingReason{
		ports.ReachabilityNotFormed:          application.ReachabilityJudgmentNotFormed,
		ports.ReachabilityRequestConflict:    application.ReachabilityRequestConflict,
		ports.ReachabilityRequestNotAccepted: application.ReachabilityRequestNotAccepted,
	}

	seen := make(map[string]ports.ReachabilityOutcome, len(cases))
	for outcome, want := range cases {
		fixture := newJudgmentFixture(t)
		fixture.reachability.outcome = outcome

		result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		if err != nil {
			t.Fatalf("%q: handle 上抛了错误，而它是一个业务答案: %v", outcome, err)
		}
		if result.Outcome() != application.AcceptanceJudgmentUndecided {
			t.Fatalf("%q: outcome = %q, want UNDECIDED", outcome, result.Outcome())
		}
		if result.PendingReason() != want {
			t.Fatalf("%q: pending reason = %q, want %q", outcome, result.PendingReason(), want)
		}
		if _, present := result.ReachabilityJudgment(); present {
			t.Fatalf("%q 却交回了一份判断", outcome)
		}

		reference := result.ContinuationReference().String()
		if clash, exists := seen[reference]; exists {
			t.Fatalf("%q 与 %q 共用续办引用 %q——纠正请求与重试依赖被并成了一条路", outcome, clash, reference)
		}
		seen[reference] = outcome
	}
}

// Covers: judgment_continuation.go 的分界「依赖答不出是业务结果，取回的东西根本不属于这个
// 请求则是编程错误」— 第二阶段交回封闭集合以外的答复属后者，上抛而不是编出一个未决原因。
func TestAnAsOfOutcomeOutsideTheClosedSetIsRaisedNotTranslated(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.commercial.asOfOutcome = ports.JudgmentAsOfOutcome(200)

	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, application.ErrUnexpectedAsOfOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedAsOfOutcome——集合外的答复被静默译成了某个未决原因", err)
	}
}

type judgmentFixture struct {
	handler      *application.AdvanceAcceptanceJudgmentHandler
	commercial   *commercialBasisDouble
	reachability *reachabilityDouble
	requests     *judgmentRequestStore
	repository   *awaitingRequestStore
	calls        []string
}

func newJudgmentFixture(t *testing.T) *judgmentFixture {
	t.Helper()
	value := &judgmentFixture{}
	record := func(name string) { value.calls = append(value.calls, name) }

	value.commercial = &commercialBasisDouble{
		t:                            t,
		applicability:                domain.CommerciallyApplicable,
		declaresReachabilityAsOf:     true,
		declaresFinancialControlAsOf: true,
		record:                       record,
	}
	value.reachability = &reachabilityDouble{t: t, value: domain.ReachabilityReachable, record: record}
	value.requests = &judgmentRequestStore{}
	value.repository = &awaitingRequestStore{t: t}
	value.handler = application.NewAdvanceAcceptanceJudgmentHandler(
		value.commercial,
		value.reachability,
		value.requests,
		value.repository,
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
	applicability                domain.CommercialApplicability
	declaresReachabilityAsOf     bool
	declaresFinancialControlAsOf bool
	manualReview                 domain.ManualReviewPolicy
	applicable                   []domain.AcceptanceCheckGroup
	pendingRouting               domain.PendingRoutingAllowance
	err                          error
	record                       func(string)
	calls                        int

	// 第二阶段：默认形成，测试按需改成四种未成形之一或让端口自己答不出。
	asOfOutcome         ports.JudgmentAsOfOutcome
	asOfErr             error
	asOfCalls           int
	lastAsOfQuery       ports.JudgmentAsOfQuery
	revalidateCalls     int
	lastRevalidation    ports.CommercialRevalidationQuery
	revalidationOutcome ports.CommercialRevalidationOutcome
}

func (double *commercialBasisDouble) ResolveCommercialBasis(
	_ context.Context,
	_ ports.CommercialBasisQuery,
) (ports.CommercialBasisResolution, error) {
	double.t.Helper()
	double.record("resolve-commercial-basis")
	double.calls++

	if double.err != nil {
		return ports.CommercialBasisResolution{}, double.err
	}
	if double.applicability != domain.CommerciallyApplicable {
		// 非适用的解析没有快照，只有适用性与原因引用——这正是端口不能「只返回对象」的原因。
		return ports.CommercialBasisResolution{
			Applicability: double.applicability,
			Reason:        mustValue(double.t, domain.NewCheckReason, "PC-"+double.applicability.String()),
		}, nil
	}
	policies := []domain.DeclaredAsOf{}
	if double.declaresReachabilityAsOf {
		policies = append(policies, declaredAsOfFor(double.t, domain.ReachabilityJudgmentKind))
	}
	if double.declaresFinancialControlAsOf {
		policies = append(policies, declaredAsOfFor(double.t, domain.FinancialControlJudgmentKind))
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
	snapshot, err := domain.NewCommercialBasisSnapshot(domain.CommercialBasisSnapshotSpec{
		ResolutionID:   mustValue(double.t, domain.NewCommercialResolutionID, "RES-1"),
		RulePackage:    mustValue(double.t, domain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision:   mustValue(double.t, domain.NewCommercialViewRevision, "VIEW-1"),
		DeclaredAsOf:   policies,
		Applicable:     applicable,
		ManualReview:   double.manualReview,
		PendingRouting: double.pendingRouting,
	})
	if err != nil {
		double.t.Fatalf("new commercial basis snapshot: %v", err)
	}
	return ports.CommercialBasisResolution{
		Snapshot:      snapshot,
		Applicability: domain.CommerciallyApplicable,
	}, nil
}

// declaredAsOfFor 造一份规则包声明。它不带时刻——值由适配器在第二阶段形成，声明这一头只有
// 语义与政策版本。
func declaredAsOfFor(t *testing.T, kind domain.JudgmentKind) domain.DeclaredAsOf {
	t.Helper()
	declared, err := domain.NewDeclaredAsOf(
		kind,
		mustValue(t, domain.NewAsOfSemanticsReference, "ASOF-SEMANTICS-"+kind.String()),
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new declared asOf: %v", err)
	}
	return declared
}

// formedAsOfFor 造一份已由提供方校验回显的时点，供夹具冒充第二阶段的成功答复。回显政策
// 必须另行构造——把第一阶段的声明直接塞进 NewJudgmentAsOf 已经编译不过。
func formedAsOfFor(t *testing.T, kind domain.JudgmentKind, at time.Time) domain.JudgmentAsOf {
	t.Helper()
	echoed, err := domain.NewEchoedAsOfPolicy(
		kind,
		mustValue(t, domain.NewAsOfSemanticsReference, "ASOF-SEMANTICS-"+kind.String()),
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new echoed asOf policy: %v", err)
	}
	asOf, err := domain.NewJudgmentAsOf(at, echoed)
	if err != nil {
		t.Fatalf("new judgment asOf: %v", err)
	}
	return asOf
}

// FormJudgmentAsOf 冒充 UC-PC-002 第二阶段。默认交回`已形成`，因为多数用例关心的是它之后
// 那一步；四种未成形与端口答不出各由测试显式设定。
func (double *commercialBasisDouble) FormJudgmentAsOf(
	_ context.Context,
	query ports.JudgmentAsOfQuery,
) (ports.JudgmentAsOfFormation, error) {
	double.t.Helper()
	double.record("form-judgment-as-of")
	double.asOfCalls++
	double.lastAsOfQuery = query

	if double.asOfErr != nil {
		return ports.JudgmentAsOfFormation{}, double.asOfErr
	}
	outcome := double.asOfOutcome
	if outcome == ports.JudgmentAsOfOutcomeInvalid {
		outcome = ports.JudgmentAsOfFormed
	}
	if outcome != ports.JudgmentAsOfFormed {
		// 未成形一律不带时点：交回一个零值，编排会拿一个没人授权过的时刻去推进权威判断。
		return ports.JudgmentAsOfFormation{Outcome: outcome}, nil
	}

	at := policyFormedAsOf
	if query.Declared.Kind() == domain.FinancialControlJudgmentKind {
		at = controlPolicyFormedAsOf
	}
	return ports.JudgmentAsOfFormation{
		Outcome: ports.JudgmentAsOfFormed,
		AsOf:    formedAsOfFor(double.t, query.Declared.Kind(), at),
	}, nil
}

// RevalidateCommercialBasis 冒充 UC-PC-002 步骤 8。默认交回`仍然成立`；`已失效`与权威读不到
// 由测试显式设定。
func (double *commercialBasisDouble) RevalidateCommercialBasis(
	ctx context.Context,
	query ports.CommercialRevalidationQuery,
) (ports.CommercialRevalidation, error) {
	double.t.Helper()
	double.record("revalidate-commercial-basis")
	double.revalidateCalls++
	double.lastRevalidation = query

	outcome := double.revalidationOutcome
	if outcome == ports.CommercialRevalidationOutcomeInvalid {
		outcome = ports.CommercialBasisStillValid
	}
	if outcome != ports.CommercialBasisStillValid {
		// 非`仍然成立`不带解析：带上一份，调用方会以为可以继续用它。
		return ports.CommercialRevalidation{
			Outcome: outcome,
			Reason:  mustValue(double.t, domain.NewCheckReason, "PC-"+outcome.String()),
		}, nil
	}

	resolution, err := double.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          query.Identity,
		ShipmentRequestID: query.ShipmentRequestID,
		SubmissionVersion: query.SubmissionVersion,
	})
	if err != nil {
		return ports.CommercialRevalidation{}, err
	}
	return ports.CommercialRevalidation{Outcome: ports.CommercialBasisStillValid, Resolution: resolution}, nil
}

type reachabilityDouble struct {
	t        *testing.T
	value    domain.ReachabilityValue
	outcome  ports.ReachabilityOutcome
	err      error
	record   func(string)
	calls    int
	lastAsOf domain.JudgmentAsOf
	// parcels 按序留下每一轮问的是哪个成员。计数分不出「逐成员各问一次」和「问了同一个
	// 成员两次」，而接受决定要的是每个成员各有一份判断。
	parcels []domain.DeclaredParcelID
}

func (double *reachabilityDouble) AssessParcelReachability(
	_ context.Context,
	request ports.ReachabilityRequest,
) (ports.ReachabilityAssessment, error) {
	double.t.Helper()
	double.record("assess-reachability")
	double.calls++
	double.lastAsOf = request.AsOf
	double.parcels = append(double.parcels, request.DeclaredParcelID)
	if double.err != nil {
		return ports.ReachabilityAssessment{}, double.err
	}
	outcome := double.outcome
	if outcome == ports.ReachabilityOutcomeInvalid {
		outcome = ports.ReachabilityAssessed
	}
	if outcome != ports.ReachabilityAssessed {
		// 非`已判断`一律不带判断：带上一份，编排会把一次没作出的判断记到任务上。
		return ports.ReachabilityAssessment{
			Outcome: outcome,
			Reason:  mustValue(double.t, domain.NewCheckReason, "NR-"+outcome.String()),
		}, nil
	}

	spec := domain.ReachabilityJudgmentSpec{
		JudgmentID: mustValue(double.t, domain.NewReachabilityJudgmentID, "NRJ-1"),
		ParcelID:   request.DeclaredParcelID,
		Value:      double.value,
		AsOf:       request.AsOf,
	}
	if double.value == domain.ReachabilityNotApplicable {
		spec.JudgmentID = domain.ReachabilityJudgmentID{}
		spec.Basis = mustValue(double.t, domain.NewReachabilityBasisReference, "LABEL_ONLY_CHANNEL_SERVICE")
	}
	judgement, err := domain.NewReachabilityJudgment(spec)
	if err != nil {
		double.t.Fatalf("new reachability judgement: %v", err)
	}
	return ports.ReachabilityAssessment{Outcome: ports.ReachabilityAssessed, Judgment: judgement}, nil
}

type judgmentRequestStore struct {
	rejected          bool
	err               error
	basisErr          error
	recordedControl   []domain.FinancialControlResult
	recordedAttempts  []domain.ProcessingAttempt
	adoptedResolution []domain.CommercialResolutionID
	// recordedTenants 收下每次记录时编排给出的租户。它存在是为了让「编排确实把租户传下去了」
	// 可被断言——租户漏传在替身上不会报错，只会在真库里变成一次跨租户读写。
	recordedTenants []domain.TenantID
	// recordedVersions 收下两类判断记录时编排给出的提交版本，理由同上：版本传错在替身上不会
	// 报错，只会在真库里让一份判断记到它从未判过的那一版（ADR-0045 的版本维）。
	recordedVersions []domain.SubmissionVersionID
}

// RecordAdoptedCommercialResolution 用自己的错误开关，不共用 err：记不下所采用的解析与
// 记不下判断停在不同步骤，共用一个开关就分不出编排到底卡在哪一处。
func (store *judgmentRequestStore) RecordAdoptedCommercialResolution(
	_ context.Context,
	tenant domain.TenantID,
	_ domain.ShipmentRequestID,
	resolution domain.CommercialResolutionID,
) error {
	store.recordedTenants = append(store.recordedTenants, tenant)
	if store.basisErr != nil {
		return store.basisErr
	}
	store.adoptedResolution = append(store.adoptedResolution, resolution)
	return nil
}

func (store *judgmentRequestStore) RecordReachabilityJudgment(
	_ context.Context,
	tenant domain.TenantID,
	_ domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
	_ domain.ReachabilityJudgment,
) error {
	store.recordedTenants = append(store.recordedTenants, tenant)
	store.recordedVersions = append(store.recordedVersions, version)
	return store.err
}

func (store *judgmentRequestStore) RecordFinancialControlResult(
	_ context.Context,
	tenant domain.TenantID,
	_ domain.ShipmentRequestID,
	version domain.SubmissionVersionID,
	result domain.FinancialControlResult,
) error {
	store.recordedTenants = append(store.recordedTenants, tenant)
	store.recordedVersions = append(store.recordedVersions, version)
	if store.err != nil {
		return store.err
	}
	store.recordedControl = append(store.recordedControl, result)
	return nil
}

// RecordProcessingAttempt 即便在 err 已设时也留下记录：编排把「记录尝试」与「记录判断」
// 分开处理，前者失败不改写本轮的未决原因，测试要能看到这一点。
func (store *judgmentRequestStore) RecordProcessingAttempt(
	_ context.Context,
	tenant domain.TenantID,
	_ domain.ShipmentRequestID,
	attempt domain.ProcessingAttempt,
) error {
	store.recordedTenants = append(store.recordedTenants, tenant)
	store.recordedAttempts = append(store.recordedAttempts, attempt)
	return nil
}

var (
	_ ports.CommercialBasisResolver    = (*commercialBasisDouble)(nil)
	_ ports.ReachabilityAssessor       = (*reachabilityDouble)(nil)
	_ ports.AcceptanceJudgmentRecorder = (*judgmentRequestStore)(nil)
)
