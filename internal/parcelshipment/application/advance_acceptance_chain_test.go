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

// Covers: UC-PS-001 步骤 8「自动接受」与 AT-PS-033 — 提交之后接受判断三步按序自行推进，
// 可达性逐成员一轮，财务控制与形成决定各按整份委托一次。
//
// 顺序按调用先后断言而不是逐字比对调用串：替身在重校验里会顺带再解析一次商业依据，逐字
// 比对会把那处替身细节钉进用例，换一个替身实现就红，而三步的先后并没有变。
func TestTheChainAssessesEachMemberThenControlsThenDecides(t *testing.T) {
	fixture := newChainFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceChainDecided {
		t.Fatalf("outcome = %q reason = %q, want DECIDED", result.Outcome(), result.PendingReason())
	}
	if result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED", result.State())
	}
	if _, present := result.AcceptanceDecision(); !present {
		t.Fatal("链答已决定却没有交回那份决定")
	}

	// 逐成员各一轮：一份委托的成员各有各的可达性结论，问一次拿去顶两个成员，就是把
	// 另一个成员的判断编出来。
	declared := []domain.DeclaredParcelID{
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
	}
	if !slices.Equal(fixture.reachability.parcels, declared) {
		t.Fatalf("assessed %v, want one round per declared member %v", fixture.reachability.parcels, declared)
	}
	// 财务控制按整份委托一次。跟着成员循环发一次，货主的钱会被按成员数重复占用。
	if fixture.controller.calls != 1 {
		t.Fatalf("financial control calls = %d, want exactly 1 for the whole request", fixture.controller.calls)
	}

	lastAssess := lastCallIndex(fixture.calls, "assess-reachability")
	firstControl := slices.Index(fixture.calls, "apply-financial-control")
	revalidate := slices.Index(fixture.calls, "revalidate-commercial-basis")
	if firstControl < lastAssess {
		t.Fatalf("calls = %v；财务控制早于最后一个成员的可达性", fixture.calls)
	}
	if revalidate < firstControl {
		t.Fatalf("calls = %v；形成决定早于财务控制", fixture.calls)
	}
}

// Covers: UC-PS-001 结果语义契约`尚未决定` — 一条腿停下即整条停下，后面两步不再走。
//
// 往下走有害而不只是多余：形成决定读的是**已记录**的判断，缺一条它只会再答一次`尚未
// 决定`，代价是多记一次处理尝试，还把停顿原因换成了下游那一步的措辞——运维据此会去查
// 决定，而实际停的是可达性这条腿。
func TestAStalledMemberStopsTheChainBeforeControlAndDecision(t *testing.T) {
	fixture := newChainFixture(t)
	fixture.reachability.err = errors.New("reachability authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceChainUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.Stage() != application.AcceptanceChainReachabilityStage {
		t.Fatalf("stage = %q, want REACHABILITY_JUDGMENT", result.Stage())
	}
	if result.PendingReason() != application.ReachabilityAuthorityUnavailable {
		t.Fatalf("reason = %q, want REACHABILITY_AUTHORITY_UNAVAILABLE", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("停下的一轮没留续办引用")
	}
	// 停在第一个成员就不问第二个：一份判断都记不下时，把剩下的成员挨个问一遍只会多记
	// 几次处理尝试，而它们随本轮回滚一并消失。
	if fixture.reachability.calls != 1 {
		t.Fatalf("assessed %d members after the first one stalled, want 1", fixture.reachability.calls)
	}
	if fixture.controller.calls != 0 {
		t.Fatal("可达性还没判完就发起了资金占用")
	}
	if len(fixture.downstream.intents) != 0 {
		t.Fatal("链停在可达性，却交出了一份接受决定意图")
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q；推进一条判断腿不是接受也不是拒绝", result.State())
	}
}

// Covers: UC-PS-001 接受条件`接受前财务控制`「不得默认放行」— 控制这条腿停下时不往形成
// 决定走。走过去会让「控制从未形成」被读成决定那一步的缺口。
func TestAStalledFinancialControlStopsTheChainBeforeTheDecision(t *testing.T) {
	fixture := newChainFixture(t)
	fixture.controller.err = errors.New("settlement authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Stage() != application.AcceptanceChainFinancialControlStage {
		t.Fatalf("stage = %q, want FINANCIAL_CONTROL_JUDGMENT", result.Stage())
	}
	if result.Outcome() != application.AcceptanceChainUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	// 两个成员都判过了才轮到控制：控制腿停下不该把已经走完的可达性也回卷成没走。
	if fixture.reachability.calls != 2 {
		t.Fatalf("assessed %d members, want both before control ran", fixture.reachability.calls)
	}
	if len(fixture.downstream.intents) != 0 {
		t.Fatal("控制没形成，却交出了一份接受决定意图")
	}
}

// Covers: CONTEXT 接受判断任务「等待人工复核」— 停在形成决定那一步时 Stage 报的是决定，
// 与前两条各占一格。三条共用一格，运维就分不出该去看哪条腿。
func TestAnUndecidedDecisionReportsTheDecisionStage(t *testing.T) {
	fixture := newChainFixture(t)
	fixture.commercial.manualReview = domain.ManualReviewRequiredByRules

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.AcceptanceChainUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.Stage() != application.AcceptanceChainDecisionStage {
		t.Fatalf("stage = %q, want ACCEPTANCE_DECISION", result.Stage())
	}
	if result.PendingReason() != application.ManualReviewPending {
		t.Fatalf("reason = %q, want MANUAL_REVIEW_PENDING", result.PendingReason())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("未决的一轮交回了一份决定")
	}
}

// Covers: ADR-0025「翻译必须是全函数」的反面 — 编排上抛的错误原样上抛，不折成未决。
//
// 折成未决的代价是无休止重投：集合外的答复是编程错误，重投同一份信封改不了它，而消费门
// 对未决的处置正是回滚重投。
func TestAnOrchestrationErrorIsRaisedRatherThanFoldedIntoUndecided(t *testing.T) {
	fixture := newChainFixture(t)
	fixture.commercial.asOfOutcome = ports.JudgmentAsOfOutcome(200)

	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, application.ErrUnexpectedAsOfOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedAsOfOutcome——集合外的答复被折成了一次未决", err)
	}
}

// Covers: ADR-0049 第三条的同一条理由在装配面上的落法 — 少接一个编排响亮报错，不当作
// 未决。未决等的是一个会回来的依赖，而少接一步等多久都不会长出来；压成未决，消费门会
// 一路重投到失败预算耗尽，现场看到的是「一直在等」而不是「接线漏了」。
func TestAnUnassembledChainFailsLoudlyInsteadOfWaiting(t *testing.T) {
	for name, drop := range map[string]func(*application.AdvanceAcceptanceChainDeps){
		"reachability":      func(deps *application.AdvanceAcceptanceChainDeps) { deps.Reachability = nil },
		"financial control": func(deps *application.AdvanceAcceptanceChainDeps) { deps.FinancialControl = nil },
		"decision":          func(deps *application.AdvanceAcceptanceChainDeps) { deps.Decision = nil },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newChainFixture(t)
			deps := fixture.deps
			drop(&deps)

			_, err := application.NewAdvanceAcceptanceChainHandler(deps).
				Handle(context.Background(), fixture.command(t))
			if !errors.Is(err, application.ErrAcceptanceChainNotAssembled) {
				t.Fatalf("err = %v, want ErrAcceptanceChainNotAssembled", err)
			}
		})
	}
}

// Covers: UC-PS-001 提交门禁「声明成员非空」在下游的镜像 — 空清单是上游给错了，不是本轮
// 该等的东西。当成「没有成员要判，直接形成决定」会让一份根本不成立的委托走完接受。
func TestAnEmptyMemberListIsRejectedRatherThanReadAsNothingToJudge(t *testing.T) {
	fixture := newChainFixture(t)
	command := fixture.command(t)
	command.DeclaredParcelIDs = nil

	_, err := fixture.handler.Handle(context.Background(), command)
	if !errors.Is(err, application.ErrAcceptanceChainHasNoMembers) {
		t.Fatalf("err = %v, want ErrAcceptanceChainHasNoMembers", err)
	}
	if fixture.reachability.calls != 0 || fixture.controller.calls != 0 {
		t.Fatal("空清单仍然推进了判断")
	}
}

// Covers: 三个 Stage 各有自己的字面 — Stage 会随未决原因一起写进消费门的错误正文，两格
// 同字面时运维在日志里认不出停的是哪条腿。
func TestEachStageNamesItself(t *testing.T) {
	seen := make(map[string]application.AcceptanceChainStage, 3)
	for _, stage := range []application.AcceptanceChainStage{
		application.AcceptanceChainReachabilityStage,
		application.AcceptanceChainFinancialControlStage,
		application.AcceptanceChainDecisionStage,
	} {
		name := stage.String()
		if name == "" {
			t.Fatalf("stage %d has no name", stage)
		}
		if clash, exists := seen[name]; exists {
			t.Fatalf("stage %d 与 %d 共用字面 %q", stage, clash, name)
		}
		seen[name] = stage
	}
	if application.AcceptanceChainStageInvalid.String() != "" {
		t.Fatal("零值 Stage 有名字；它会作为一个正常阶段被打进日志")
	}
}

// chainFixture 用三步各自的真编排接起来，只在端口一层放替身。
//
// 不给三步各造一个可替换的 handler 替身：本层要验的正是「顺序」与「在哪一步停」，而停
// 在哪一步由各步自己的未决判断决定；把三步换成能任意作答的替身，验的就只剩本层自己那
// 三个 if。
type chainFixture struct {
	handler      *application.AdvanceAcceptanceChainHandler
	deps         application.AdvanceAcceptanceChainDeps
	commercial   *commercialBasisDouble
	reachability *reachabilityDouble
	controller   *financialControlDouble
	requests     *decidableRequestStore
	downstream   *decisionHandoffDouble
	calls        []string
}

func newChainFixture(t *testing.T) *chainFixture {
	t.Helper()
	value := &chainFixture{}
	record := func(name string) { value.calls = append(value.calls, name) }

	// 三步共用一份商业依据替身，与生产装配同形（同一个适配器实例服务三条腿）。各给一份
	// 会让「三步问的是同一个权威」验不出来——那正是重校验能抓到提交前失效的前提。
	value.commercial = &commercialBasisDouble{
		t:                            t,
		applicability:                domain.CommerciallyApplicable,
		declaresReachabilityAsOf:     true,
		declaresFinancialControlAsOf: true,
		manualReview:                 domain.ManualReviewNotRequiredByRules,
		record:                       record,
	}
	value.reachability = &reachabilityDouble{t: t, value: domain.ReachabilityReachable, record: record}
	value.controller = &financialControlDouble{t: t, outcome: domain.FinancialControlHeld, record: record}
	value.requests = &decidableRequestStore{t: t}
	value.downstream = &decisionHandoffDouble{}

	recorder := &judgmentRequestStore{}
	clock := fixedClock{at: handlerClockAt}
	value.deps = application.AdvanceAcceptanceChainDeps{
		Reachability: application.NewAdvanceAcceptanceJudgmentHandler(
			value.commercial, value.reachability, recorder, clock),
		FinancialControl: application.NewAdvanceFinancialControlJudgmentHandler(
			value.commercial, value.controller, recorder, clock),
		Decision: application.NewFormAcceptanceDecisionHandler(application.FormAcceptanceDecisionDeps{
			Requests:   value.requests,
			Commercial: value.commercial,
			// 决定那一步读的是**已记录**的判断，而记录端口在本夹具里是个不回读的替身。
			// 因此这里给一份与命令成员一致的已记录判断，冒充前两步刚记下的那些。
			Judgments: &recordedJudgmentsDouble{
				t: t,
				reachability: map[string]domain.ReachabilityValue{
					"parcel-1": domain.ReachabilityReachable,
					"parcel-2": domain.ReachabilityReachable,
				},
				controlOutcome: domain.FinancialControlHeld,
			},
			Reachability: &reachabilityRevalidatorDouble{outcome: ports.ReachabilityJudgmentStillCurrent},
			Recorder:     recorder,
			Release:      &controlReleaseDouble{},
			Downstream:   value.downstream,
			Identities:   &decisionIdentityFactory{t: t},
			Clock:        clock,
		}),
	}
	value.handler = application.NewAdvanceAcceptanceChainHandler(value.deps)
	return value
}

func (value *chainFixture) command(t *testing.T) application.AdvanceAcceptanceChainCommand {
	t.Helper()
	return application.AdvanceAcceptanceChainCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
			mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
		},
	}
}

// lastCallIndex 取某个调用名最后一次出现的位置。逐成员那一步会出现多次，比先后要拿最后
// 一次——拿第一次只能说明控制晚于**某一个**成员。
func lastCallIndex(calls []string, name string) int {
	last := -1
	for index, call := range calls {
		if call == name {
			last = index
		}
	}
	return last
}
