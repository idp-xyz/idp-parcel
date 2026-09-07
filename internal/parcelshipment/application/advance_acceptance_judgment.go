package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AcceptanceJudgmentOutcome 是接受判断任务推进一轮的应用处理结果。两个取值都不是接受
// 或拒绝：任务保持可续办，委托保持`已提交`。
type AcceptanceJudgmentOutcome uint8

const (
	AcceptanceJudgmentOutcomeInvalid AcceptanceJudgmentOutcome = iota
	AcceptanceJudgmentAdvanced
	AcceptanceJudgmentUndecided
)

func (outcome AcceptanceJudgmentOutcome) String() string {
	switch outcome {
	case AcceptanceJudgmentAdvanced:
		return "ADVANCED"
	case AcceptanceJudgmentUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

type AdvanceAcceptanceJudgmentCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelID  domain.DeclaredParcelID
}

type AdvanceAcceptanceJudgmentResult struct {
	outcome      AcceptanceJudgmentOutcome
	judgment     domain.ReachabilityJudgment
	hasJudgment  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
}

func (result AdvanceAcceptanceJudgmentResult) Outcome() AcceptanceJudgmentOutcome {
	return result.outcome
}

// PendingReason 说明本轮停在哪一步。用例要求`尚未决定`返回未决原因，而不是只给一个续办
// 引用：调用方据原因决定该补缺口还是该重试依赖。
func (result AdvanceAcceptanceJudgmentResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

func (result AdvanceAcceptanceJudgmentResult) ReachabilityJudgment() (domain.ReachabilityJudgment, bool) {
	return result.judgment, result.hasJudgment
}

func (result AdvanceAcceptanceJudgmentResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// State 报告本轮之后委托的生命周期状态，恒为`已提交`：接受判断任务推进了一步——无论
// 判断结果如何——都不是接受也不是拒绝。
func (result AdvanceAcceptanceJudgmentResult) State() domain.ShipmentRequestState {
	return domain.ShipmentRequestSubmitted
}

// reachabilityAsOfReasons 是可达性判断在第二阶段四种未成形上的未决原因。`未配置`与`未决`
// 分开尤其要紧：前者等租户把 `PAR-COM-14` 的时点策略登记上，后者等依赖恢复，压成一格会对着
// 一个没配置的租户参数无休止内部重试。
var reachabilityAsOfReasons = asOfPendingReasons{
	basisNotResolved: ReachabilityAsOfBasisNotResolved,
	notConfigured:    ReachabilityAsOfNotConfigured,
	unavailable:      ReachabilityAsOfUnavailable,
	valueRejected:    ReachabilityAsOfValueRejected,
	inputNotAccepted: ReachabilityAsOfInputNotAccepted,
}

// reachabilityStallReasons 是 network-routing 三种非「已判断」答复各自的未决原因。
//
// 与第二阶段那一套同一机制：一张写明的表加一次查表，查不到就是端口交回了封闭集合以外的东西。
// `请求冲突`与`未受理`要本方纠正这次请求，`未形成判断`才是等依赖——三者共用一格，前两者会被
// 当成故障重试，而重试改不了一个拼错或冲突的请求。
var reachabilityStallReasons = map[ports.ReachabilityOutcome]JudgmentPendingReason{
	ports.ReachabilityNotFormed:          ReachabilityJudgmentNotFormed,
	ports.ReachabilityRequestConflict:    ReachabilityRequestConflict,
	ports.ReachabilityRequestNotAccepted: ReachabilityRequestNotAccepted,
}

type AdvanceAcceptanceJudgmentHandler struct {
	commercial   ports.CommercialBasisResolver
	reachability ports.ReachabilityAssessor
	recorder     ports.AcceptanceJudgmentRecorder
	requests     ports.ShipmentRequestRepository
	clock        ports.Clock
}

// requests 只在`判断时点未配置`那一停上用到：把`等待运营登记`的等待态在决定之前落库
// （ADR-0094 Decision 五）。判断本身仍经 recorder 记到任务上，不经聚合——本步不形成决定，
// 也不该为了记一条判断去重写整份委托。
func NewAdvanceAcceptanceJudgmentHandler(
	commercial ports.CommercialBasisResolver,
	reachability ports.ReachabilityAssessor,
	recorder ports.AcceptanceJudgmentRecorder,
	requests ports.ShipmentRequestRepository,
	clock ports.Clock,
) *AdvanceAcceptanceJudgmentHandler {
	return &AdvanceAcceptanceJudgmentHandler{
		commercial:   commercial,
		reachability: reachability,
		recorder:     recorder,
		requests:     requests,
		clock:        clock,
	}
}

// Handle 把一个接受判断任务推进一步：采用唯一商业依据，按该依据声明的策略形成可达性
// 判断时点，再记录路由权威返回的判断。
//
// 它不形成接受或拒绝，也绝不拿自己的时钟顶替声明的时点。没有唯一依据、本类判断没有被
// 声明时点、或者依赖答不出时，它停下并保持可续办，而不是在一个无人授权的时刻上继续。
//
// 依赖答不出形成未决而不上抛：用例把`依赖不可用`列为`尚未决定`的成因，并要求该结果带上
// 未决原因与安全续办引用。未决原因指名是哪个依赖停了，因此「权威答不出」与「权威答了
// 不可达」仍然分得开——这正是本步绝不形成判断的原因。
func (handler *AdvanceAcceptanceJudgmentHandler) Handle(
	ctx context.Context,
	command AdvanceAcceptanceJudgmentCommand,
) (AdvanceAcceptanceJudgmentResult, error) {
	// 本步只推进判断，不形成决定，因此前半段任何一处停下都保持可续办：据不据一次确定性
	// 商业失败拒绝，由形成决定那一步回答。
	adopted, stall, err := formAdoptedBasis(
		ctx,
		handler.commercial,
		handler.recorder,
		commercialBasisScope{
			Identity:          command.Identity,
			ShipmentRequestID: command.ShipmentRequestID,
			SubmissionVersion: command.SubmissionVersion,
		},
		domain.ReachabilityJudgmentKind,
		ReachabilityAsOfNotDeclared,
		reachabilityAsOfReasons,
	)
	if err != nil {
		return AdvanceAcceptanceJudgmentResult{}, err
	}
	if stall.stopped() {
		reason, err := awaitOperatorRegistration(ctx, handler.requests, command.Identity, stall.reason)
		if err != nil {
			return AdvanceAcceptanceJudgmentResult{}, err
		}
		scope := stall.scope
		if reason != stall.reason {
			// 原因换成了保存那一格自己的，范围随之放下：续办引用由原因与范围共同派生，拿旧范围
			// 拼新原因会造出一条谁也查不回来的引用。
			scope = nil
		}
		return handler.undecided(ctx, command, reason, scope...), nil
	}

	assessment, err := handler.reachability.AssessParcelReachability(ctx, ports.ReachabilityRequest{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		DeclaredParcelID:  command.DeclaredParcelID,
		AsOf:              adopted.asOf,
	})
	if err != nil {
		return handler.undecided(ctx, command, ReachabilityAuthorityUnavailable), nil
	}
	if assessment.Outcome != ports.ReachabilityAssessed {
		reason, ok := reachabilityStallReasons[assessment.Outcome]
		if !ok {
			return AdvanceAcceptanceJudgmentResult{}, ErrUnexpectedAssessmentOutcome
		}
		return handler.undecided(ctx, command, reason), nil
	}
	judgment := assessment.Judgment
	// 判断没能记到任务上就不算推进。交回一个没记下的判断，接受那一步会引用一条查不回来
	// 的依据。
	if err := handler.recorder.RecordReachabilityJudgment(
		ctx, command.Identity.TenantID(), command.ShipmentRequestID, command.SubmissionVersion, judgment); err != nil {
		return handler.undecided(ctx, command, JudgmentNotRecorded), nil
	}

	return AdvanceAcceptanceJudgmentResult{
		outcome:     AcceptanceJudgmentAdvanced,
		judgment:    judgment,
		hasJudgment: true,
	}, nil
}

// undecided 的 scope 是本轮范围之外还要参与续办派生的东西，例如提供方交回的原因引用：同一
// 未决原因下的两种缺口靠它分开。
func (handler *AdvanceAcceptanceJudgmentHandler) undecided(
	ctx context.Context,
	command AdvanceAcceptanceJudgmentCommand,
	reason JudgmentPendingReason,
	scope ...string,
) AdvanceAcceptanceJudgmentResult {
	continuation := judgmentContinuation(
		reason,
		append([]string{
			command.Identity.TenantID().String(),
			command.Identity.CustomerAccountID().String(),
			command.ShipmentRequestID.String(),
			command.SubmissionVersion.String(),
			command.DeclaredParcelID.String(),
		}, scope...)...,
	)
	recordAttempt(ctx, handler.recorder, handler.clock,
		command.Identity.TenantID(), command.ShipmentRequestID, reason, continuation)

	return AdvanceAcceptanceJudgmentResult{
		outcome:      AcceptanceJudgmentUndecided,
		reason:       reason,
		continuation: continuation,
	}
}
