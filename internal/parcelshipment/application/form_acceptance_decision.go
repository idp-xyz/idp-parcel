package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AcceptanceDecisionOutcome 是形成接受决定这一步的应用处理结果。它只有两个取值：形成了
// 决定，或者本轮没形成。接受与拒绝的区别在委托的生命周期状态里，不在这里再复制一份——
// 复制过来就得靠两处判断回答「到底决定了没有」。
type AcceptanceDecisionOutcome uint8

const (
	AcceptanceDecisionOutcomeInvalid AcceptanceDecisionOutcome = iota
	AcceptanceDecided
	AcceptanceUndecided
)

func (outcome AcceptanceDecisionOutcome) String() string {
	switch outcome {
	case AcceptanceDecided:
		return "DECIDED"
	case AcceptanceUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type FormAcceptanceDecisionCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
}

type FormAcceptanceDecisionResult struct {
	outcome      AcceptanceDecisionOutcome
	state        domain.ShipmentRequestState
	decision     domain.AcceptanceDecision
	hasDecision  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
	compensation domain.OwnershipContinuationReference
	handoff      domain.OwnershipContinuationReference
}

func (result FormAcceptanceDecisionResult) Outcome() AcceptanceDecisionOutcome {
	return result.outcome
}

// State 是本轮之后委托的生命周期状态。未形成决定时它是`已提交`——用例把`尚未决定`定为应用
// 处理结果而不是委托的新终局状态。
func (result FormAcceptanceDecisionResult) State() domain.ShipmentRequestState {
	return result.state
}

// AcceptanceDecision 只在本轮形成了决定时给出。未决时若照样交回一个零值决定，下游会读到一份
// 既非接受也非拒绝的空决定，而空决定最容易被当成没有障碍。
func (result FormAcceptanceDecisionResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

func (result FormAcceptanceDecisionResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result FormAcceptanceDecisionResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// CompensationReference 只在决定已经成立、而随附的资金释放没能确定完成时给出。它与
// ContinuationReference 分开：后者续办的是尚未形成的决定，前者续办的是已成立决定留下的
// 补偿。合成一个会让调用方分不清该重判还是该重放。
func (result FormAcceptanceDecisionResult) CompensationReference() domain.OwnershipContinuationReference {
	return result.compensation
}

// DecisionHandoffReference 只在决定已经成立、而它的发布意图没能确定交出时给出。与
// CompensationReference 平行，也同样与 ContinuationReference 分开：三者续办的分别是发布、
// 补偿与尚未形成的决定，合成一个会让调用方分不清该重放哪一件（`AT-PS-013`）。
func (result FormAcceptanceDecisionResult) DecisionHandoffReference() domain.OwnershipContinuationReference {
	return result.handoff
}

// FormAcceptanceDecisionDeps 收拢本编排的协作方。用结构体而不是位置参数：一排同为接口的
// 参数在调用点认不出谁是谁，理由与 CommercialBasisSnapshotSpec 相同（不写个数——那种计数
// 过期时没有任何东西会变红）。
type FormAcceptanceDecisionDeps struct {
	Requests   ports.ShipmentRequestRepository
	Commercial ports.CommercialBasisResolver
	Judgments  ports.RecordedJudgmentReader
	Recorder   ports.AcceptanceJudgmentRecorder
	Release    ports.PreAcceptanceControlRelease
	Downstream ports.AcceptanceDecisionHandoff
	Identities ports.AcceptanceDecisionIdentity
	Clock      ports.Clock
}

type FormAcceptanceDecisionHandler struct {
	deps FormAcceptanceDecisionDeps
}

func NewFormAcceptanceDecisionHandler(deps FormAcceptanceDecisionDeps) *FormAcceptanceDecisionHandler {
	return &FormAcceptanceDecisionHandler{deps: deps}
}

// Handle 把接受判断任务上已采用的权威判断装配成校验结果，交给委托聚合形成一次接受、拒绝
// 或`尚未决定`。
//
// 它自己不判任何一组：三值可达性与财务控制结果由各自的权威上下文形成，本编排只做翻译与
// 装配。哪个取值算失败写在领域的翻译函数里，覆盖够不够写在聚合的 Decide 里——两者都不在
// 这里，因为它们是接受语言的一部分而不是编排顺序的一部分。
func (handler *FormAcceptanceDecisionHandler) Handle(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
) (FormAcceptanceDecisionResult, error) {
	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return handler.undecided(ctx, command, ShipmentRequestUnavailable, domain.ShipmentRequestStateInvalid), nil
	}
	if !found {
		// 命令指名了一份不存在的委托。这不是依赖答不出，而是调用方对世界的判断就是错的，
		// 因此上抛而不是形成未决——给它一个业务取值，编程错误就会安静地混进未决统计。
		//
		// 接 HTTP 时这条错误必须映射为 CONTEXT 的`统一不可见结果`：FindBySourceIdentity 的
		// 否定结果不区分「不存在」与「属于另一个租户或客户账户」，照字面映射成 404 会把
		// 对象存在与否透露给越权的调用方。
		return FormAcceptanceDecisionResult{}, fmt.Errorf("form acceptance decision: %w", domain.ErrInvalidShipmentRequest)
	}

	// 接受（或拒绝）已经越过提交边界时，本编排只交回那一份历史决定。`UC-PC-002` 步骤 8
	// 的重校验是提交前窗口（`AT-PC-026`）；提交后再拿退役/替代后的视图去审，会把已冻结的
	// 快照改写成未决——那正是 `AT-PC-025` 禁止的追溯改写。并发下 Find 仍可能读到尚未
	// 落库的旧像，那时仍靠下面 Decide 的 `ErrDecisionAlreadyFormed` 交回原决定。
	if _, formed := request.AcceptanceDecision(); formed {
		return handler.existing(ctx, command, request), nil
	}

	// 判断先读回来，因为它带着这些判断所采用的那次解析——提交决定前该走重解还是首次解析，
	// 由它决定。依据不再适用时也要读：先前可能已经形成过冻结（`AT-PC-026` 的提交前失效），
	// 而拒绝要按原关联把它解除。资金不会因为解析结论变了就自己回来。
	recorded, err := handler.deps.Judgments.LoadRecordedJudgments(ctx, command.ShipmentRequestID)
	if err != nil {
		return handler.undecided(ctx, command, RecordedJudgmentsUnavailable, request.State()), nil
	}

	resolution, stalled, err := handler.revalidateOrResolve(ctx, command, recorded.AdoptedCommercialResolution)
	if err != nil {
		return handler.undecided(ctx, command, CommercialBasisUnavailable, request.State()), nil
	}
	if stalled != PendingReasonNone {
		// 重校验那一步自己就停住了本轮，且它比这里更清楚停在哪一格。原因由它交回而不是在这里
		// 按一个布尔重新推断：第三阶段有三种停法（依据被推翻、标识不被承认、入参立不起来），
		// 恢复动作虽然都由本方发起，未决统计与续办引用却必须分得开。压成一个布尔，三者会共用
		// 同一条续办引用，调用方按引用查回来的是另一种缺口。
		return handler.undecided(ctx, command, stalled, request.State()), nil
	}
	if resolution.Applicability == domain.CommercialApplicabilityUndetermined {
		return handler.undecided(ctx, command, CommercialBasisUndetermined, request.State()), nil
	}
	commercialChecks, err := domain.CommercialBasisChecksFor(resolution.Applicability, resolution.Reason)
	if err != nil {
		return FormAcceptanceDecisionResult{}, fmt.Errorf("translate commercial basis: %w", err)
	}
	basis := resolution.Snapshot

	// 复核策略只在依据适用时才成为门：确定性不通过要形成拒绝，而拒绝不依赖规则包的其余
	// 声明——没有适用依据的解析本来也带不出复核策略，卡在这里等于让那个结论永远决定不了。
	if resolution.Applicability == domain.CommerciallyApplicable && !basis.ManualReviewPolicy().Declared() {
		return handler.undecided(ctx, command, ManualReviewPolicyNotDeclared, request.State()), nil
	}

	checks, err := assembleChecks(recorded, basis.PendingRoutingAllowance())
	if err != nil {
		return FormAcceptanceDecisionResult{}, fmt.Errorf("assemble acceptance checks: %w", err)
	}
	checks = append(commercialChecks, checks...)

	decisionID, err := handler.deps.Identities.NextAcceptanceDecisionID(ctx)
	if err != nil {
		return handler.undecided(ctx, command, DecisionIdentityUnavailable, request.State()), nil
	}

	decided, err := request.Decide(domain.AcceptanceDecisionSpec{
		DecisionID: decisionID,
		Checks:     checks,
		Basis:      basis,
		DecidedAt:  handler.deps.Clock.Now(),
	})
	if err != nil {
		// 同一提交版本已经有决定是业务答案而非故障：用例要求并发处理返回同一结果，调用方
		// 据以读取原决定，而不是当作故障重试。
		if errors.Is(err, domain.ErrDecisionAlreadyFormed) {
			return handler.existing(ctx, command, request), nil
		}
		return FormAcceptanceDecisionResult{}, fmt.Errorf("decide: %w", err)
	}

	decision, formed := decided.AcceptanceDecision()
	if !formed {
		// 聚合看过全部校验后仍未形成决定：有待判断的组、有未被判断的成员或适用组，或者
		// 规则要求的人工复核尚未完成。委托保持`已提交`，任务继续可续办。
		return handler.undecided(ctx, command, pendingReasonFor(decided), request.State()), nil
	}
	saved, err := handler.deps.Requests.Save(ctx, command.Identity, decided)
	if err != nil {
		// 决定没能越过提交边界就不算形成。交回一个没落库的接受，下游会按一份查不回来的
		// 接受基线继续办。
		//
		// 这里不释放冻结：保存失败时接受成没成立无从确定，而释放要求「接受确定未成立」。
		// 不确定就释放，会把一次其实已经落库的接受连同它合法占用的资金一起放掉。
		return handler.undecided(ctx, command, DecisionNotRecorded, request.State()), nil
	}
	if saved != ports.ShipmentRequestSaved {
		// 本方这次决定确定没落库，但抢先那一方写下了什么本方并不知道——可能正是一次接受。
		// 所以这一支同样不释放冻结，而且理由比上一支更硬：那里是「成没成立无从确定」，这里
		// 确知有人写了东西。
		//
		// 交回的 state 是**读取时**那一份，不是库里此刻那一份——冲突恰恰意味着后者已经变了。
		// 这里不为它多读一次：恢复动作本就是重读再重放，而本编排以 FindBySourceIdentity
		// 开头，续办重入时自然会读到新的那一份。
		reason, err := saveStallReason(saved)
		if err != nil {
			return FormAcceptanceDecisionResult{}, err
		}
		return handler.undecided(ctx, command, reason, request.State()), nil
	}

	return FormAcceptanceDecisionResult{
		outcome:      AcceptanceDecided,
		state:        decided.State(),
		decision:     decision,
		hasDecision:  true,
		compensation: handler.releaseIfRejected(ctx, command, decided, recorded.FinancialControl),
		handoff:      handler.handOffDecision(ctx, command, decided),
	}, nil
}

// revalidateOrResolve 执行 `UC-PC-002` 步骤 8：已经有采用过的解析时按它重解，看这份依据在
// 提交决定前是否仍然成立。第二个返回值是本轮的未决原因，`PendingReasonNone` 表示没停。
//
// 它交回原因而不是一个「是否已被推翻」的布尔：第三阶段有三种停法，而布尔只表达得了一种，
// 其余两种要么被迫冒充`已失效`，要么落进 default 变成端口坏了。前者把本方记坏标识混进
// 失效统计，后者把一个合法答复报成故障。
//
// 没有采用过的解析时走首次解析，而不是停下。一份还没推进过任何判断的委托同样可能要在这里
// 定下来——权威说这个范围确定没有适用依据时，那是一次拒绝，停下会让它永远挂着。
//
// 反过来，有采用过的解析时绝不能改走首次解析：那样拿回来的是决定时刻的新解析，与它自己比
// 永远相容，而判断是在旧依据的时点策略下形成的。`AT-PC-026` 要抓的正是这个窗口。
func (handler *FormAcceptanceDecisionHandler) revalidateOrResolve(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
	adopted domain.CommercialResolutionID,
) (ports.CommercialBasisResolution, JudgmentPendingReason, error) {
	if adopted.String() == "" {
		resolution, err := handler.resolve(ctx, command)
		return resolution, PendingReasonNone, err
	}

	revalidation, err := handler.deps.Commercial.RevalidateCommercialBasis(ctx, ports.CommercialRevalidationQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		Resolution:        adopted,
	})
	if err != nil {
		return ports.CommercialBasisResolution{}, PendingReasonNone, err
	}

	switch revalidation.Outcome {
	case ports.CommercialBasisStillValid:
		return revalidation.Resolution, PendingReasonNone, nil
	case ports.CommercialRevalidationUndetermined:
		// 权威读不到不是失效。交回`无法判定`让编排保持未决，本方重试同一次重校验即可——
		// 把它当成失效会去重解，而重解可能选中另一份依据，等于用一次读取失败换掉了原依据。
		return ports.CommercialBasisResolution{
			Applicability: domain.CommercialApplicabilityUndetermined,
			Reason:        revalidation.Reason,
		}, PendingReasonNone, nil
	case ports.CommercialBasisSuperseded:
		// 原解析已被新修订推翻。本轮不形成决定：判断是在旧依据的时点策略下形成的，拿它们去
		// 配一份新依据就是在两套依据上作一次决定。
		return handler.resolveAdoptedAgain(ctx, command, CommercialBasisSuperseded)
	case ports.CommercialRevalidationBasisNotResolved:
		// 权威不承认本方记下的那个标识——它从未签发，或者不属于这个客户账户（两者由提供方
		// 合并，本方不得拆开，见 ports 上的说明）。恢复动作与`已失效`同为回第一阶段重解，
		// 因此走同一条路；原因分开，因为这一格意味着本方的采用记录本身可疑，而`已失效`是一
		// 桩能拿去跟客户解释的商业事实。混进同一格，本方写坏标识就会计进失效统计。
		return handler.resolveAdoptedAgain(ctx, command, CommercialRevalidationBasisNotResolved)
	case ports.CommercialRevalidationInputNotAccepted:
		// 提供方在任何查询发生之前就短路拒绝了：身份或标识立不起来。这一格**不**重解——
		// 重解要拿同一个立不起来的身份去问第一阶段，只会再停一轮；缺口在本方的入参上。
		return ports.CommercialBasisResolution{}, CommercialRevalidationInputNotAccepted, nil
	default:
		return ports.CommercialBasisResolution{}, PendingReasonNone, fmt.Errorf(
			"revalidate commercial basis: %w", ErrUnexpectedRevalidationOutcome,
		)
	}
}

// resolveAdoptedAgain 在所记标识不再可用后重解一次，并把新解析记为所采用的那一份。
//
// 记下这一步是循环终止的地方：不换掉所记标识，下一轮又会拿同一个标识去重校验，永远得到同一
// 个答复。换掉之后判断在新依据下重做，重校验也比对新的那一份。
//
// 两种停法共用它，因为「回第一阶段重解并换掉所记标识」这个动作对两者字面相同；停下来时报的
// 原因由调用处给定，不在这里推断。动作相同不等于原因相同——原因参与续办派生与未决统计，而
// 那两样恰恰要求分得开。
//
// 重解本身失败或仍不唯一时不改写所记标识：那种情况下没有「新的那一份」可采用，用例要的是
// 「无法重解保持未决」，而不是把一个不成立的解析记成已采用。
func (handler *FormAcceptanceDecisionHandler) resolveAdoptedAgain(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
	stalled JudgmentPendingReason,
) (ports.CommercialBasisResolution, JudgmentPendingReason, error) {
	resolution, err := handler.resolve(ctx, command)
	if err != nil {
		return ports.CommercialBasisResolution{}, PendingReasonNone, err
	}
	if resolution.Applicability != domain.CommerciallyApplicable {
		return resolution, PendingReasonNone, nil
	}
	if err := handler.deps.Recorder.RecordAdoptedCommercialResolution(
		ctx,
		command.ShipmentRequestID,
		resolution.Snapshot.ResolutionID(),
	); err != nil {
		return ports.CommercialBasisResolution{}, PendingReasonNone, err
	}
	return resolution, stalled, nil
}

func (handler *FormAcceptanceDecisionHandler) resolve(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
) (ports.CommercialBasisResolution, error) {
	return handler.deps.Commercial.ResolveCommercialBasis(ctx, ports.CommercialBasisQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
}

// pendingReasonFor 把聚合写下的等待态译成本层的未决原因。哪一类缺口该由谁来续已经由 Decide
// 判定，这里只做一一对应；在这里另立一套判断，就是把接受语言复制到应用层，两处早晚会分叉。
//
// 等待态缺席时落回`判断未完成`：那说明聚合没形成决定也没说停在哪，是本层读不懂的状态，按最
// 保守的一支处理——内部续办不会去惊动客户，也不会派出一次没人要求的复核。
func pendingReasonFor(decided domain.ShipmentRequest) JudgmentPendingReason {
	waiting, present := decided.AcceptanceDecisionTask().WaitingOn()
	if !present {
		return AcceptanceJudgmentIncomplete
	}
	switch waiting {
	case domain.ResumeByCustomerSupplement:
		return CustomerSupplementPending
	case domain.ResumeByManualReview:
		return ManualReviewPending
	default:
		return AcceptanceJudgmentIncomplete
	}
}

// releaseIfRejected 在拒绝越过提交边界后按原关联解除资金控制，并在解除没能确定完成时交回
// 续办引用。
//
// 只有拒绝触发释放：拒绝是`接受确定未成立`，而接受成立时那笔冻结是合法的，转接受后流程。
// 释放失败不回滚拒绝——决定已经越过提交边界，回滚它等于让一次补偿失败改写业务结果；补偿
// 按原关联另行续办，这正是 CONTEXT 说的「撤回提交和释放属于可补偿编排，不要求跨上下文
// 分布式事务」。
func (handler *FormAcceptanceDecisionHandler) releaseIfRejected(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
	decided domain.ShipmentRequest,
	control domain.FinancialControlResult,
) domain.OwnershipContinuationReference {
	if decided.State() != domain.ShipmentRequestRejected {
		return domain.OwnershipContinuationReference{}
	}
	// 没有形成过控制就没有可释放的关联。凭空发一次释放会让 settlement-accounting 去认领
	// 一笔不存在的冻结。
	if control.Outcome() != domain.FinancialControlHeld {
		return domain.OwnershipContinuationReference{}
	}

	if err := handler.deps.Release.ReleasePreAcceptanceControl(ctx, ports.ControlReleaseRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		ControlResultID:   control.ResultID(),
	}); err != nil {
		return judgmentContinuation(
			ControlReleasePending,
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
			control.ResultID().String(),
		)
	}
	return domain.OwnershipContinuationReference{}
}

// assembleChecks 把已记录的判断逐条译成校验结果。有结果就记，没有就不记。
//
// 「控制从未形成因而不该接受」不在这里挡——聚合的适用组覆盖检查已经挡住了：本组被声明适用
// 却一项校验都没到场，它就不接受。在这里再塞一个`无法判定`是同一条规则的第二处实现，而且
// 塞错了更糟：本组没被声明适用时凭空塞一项永远满足不了的`无法判定`，会让一份合同本就不
// 要求财务控制的委托永远接受不了。
//
// 结果存在就一律记下，哪怕规则包没把本组列为适用：一次`业务限制`不该因为不在适用集合里就
// 被丢掉——声明只能增加要求，减不掉失败。
func assembleChecks(
	recorded ports.RecordedJudgments,
	pendingRouting domain.PendingRoutingAllowance,
) ([]domain.AcceptanceCheck, error) {
	checks := make([]domain.AcceptanceCheck, 0, len(recorded.Reachability)+1)
	for _, judgment := range recorded.Reachability {
		check, err := domain.ReachabilityCheckFor(judgment, pendingRouting)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}

	if recorded.FinancialControl.Outcome() == domain.FinancialControlOutcomeInvalid {
		return checks, nil
	}
	control, err := domain.FinancialControlCheckFor(recorded.FinancialControl)
	if err != nil {
		return nil, err
	}
	return append(checks, control), nil
}

// existing 交回该提交版本已经形成的那一个决定，并把同一份发布意图再交一次。用例要求并发
// 处理返回同一结果或明确冲突；重发是因为本上下文不记意图完没完成——只答`已有结果`就收工，
// 一份首次投递失败的决定会永远停在「本上下文已提交、下游从不知道」的状态，而这条路上没有
// 别的东西会去补发（`AT-PS-013`「重试同一发布意图」）。
func (handler *FormAcceptanceDecisionHandler) existing(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
	request domain.ShipmentRequest,
) FormAcceptanceDecisionResult {
	decision, formed := request.AcceptanceDecision()
	return FormAcceptanceDecisionResult{
		outcome:     AcceptanceDecided,
		state:       request.State(),
		decision:    decision,
		hasDecision: formed,
		handoff:     handler.handOffDecision(ctx, command, request),
	}
}

// handOffDecision 把已成立的决定交给适用下游，交不出去时交回发布续办引用。
//
// 决定与状态取自读回的聚合而不取本次命令，理由与资料修订的 handOff 相同：重放路径上交的
// 该是读回来的那一份。没有决定可交时不发意图（例如已撤回态走到 ErrDecisionAlreadyFormed）
// ——没有决定就没有可认领的标识。失败不改写业务结果，也不往任务上记处理尝试：决定任务已经
// 完成，记一条「未推进」会让一份已决的委托看起来还卡着。
func (handler *FormAcceptanceDecisionHandler) handOffDecision(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
	request domain.ShipmentRequest,
) domain.OwnershipContinuationReference {
	decision, formed := request.AcceptanceDecision()
	if !formed {
		return domain.OwnershipContinuationReference{}
	}
	if err := handler.deps.Downstream.HandOffAcceptanceDecision(ctx, ports.AcceptanceDecisionHandoffIntent{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		DecisionID:        decision.DecisionID(),
		State:             request.State(),
	}); err != nil {
		return judgmentContinuation(
			AcceptanceDecisionNotHandedOff,
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
			decision.DecisionID().String(),
		)
	}
	return domain.OwnershipContinuationReference{}
}

// undecided 交回本轮的未决结果。state 由调用点给出而不是在这里假定`已提交`：未决要交回的是
// 已知事实，而委托当前是什么状态正是本轮可能已经查到的事实之一。取不到委托的那一轮传零值。
func (handler *FormAcceptanceDecisionHandler) undecided(
	ctx context.Context,
	command FormAcceptanceDecisionCommand,
	reason JudgmentPendingReason,
	state domain.ShipmentRequestState,
) FormAcceptanceDecisionResult {
	continuation := judgmentContinuation(
		reason,
		command.Identity.TenantID().String(),
		command.Identity.CustomerAccountID().String(),
		command.ShipmentRequestID.String(),
		command.SubmissionVersion.String(),
	)
	recordAttempt(ctx, handler.deps.Recorder, handler.deps.Clock, command.ShipmentRequestID, reason, continuation)

	return FormAcceptanceDecisionResult{
		outcome:      AcceptanceUndecided,
		state:        state,
		reason:       reason,
		continuation: continuation,
	}
}
