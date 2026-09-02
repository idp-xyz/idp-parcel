package application

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrAcceptanceChainNotAssembled 说明三步里有一步没接上。它与未决分开：未决等的是一个
// 会回来的依赖，而少接一个编排等多久都不会长出来——压成未决会让消费门无休止重投。
var ErrAcceptanceChainNotAssembled = errors.New("parcel shipment: acceptance chain is not assembled")

// ErrAcceptanceChainHasNoMembers 说明这一轮拿到的成员清单是空的。一份`已提交`委托必有
// 声明成员（提交门禁要求非空），因此空清单是上游给错了而不是本轮该等的东西。
var ErrAcceptanceChainHasNoMembers = errors.New("parcel shipment: acceptance chain has no declared members")

// AcceptanceChainStage 说明接受链停在哪一步。它不是第四种处理结果，只是未决时的定位：
// 三步各自的未决原因集合有重叠（`商业依据不适用`三步都答得出），只报原因会让运维分不出
// 该去看可达性那条腿还是财务控制那条腿。
type AcceptanceChainStage uint8

const (
	AcceptanceChainStageInvalid AcceptanceChainStage = iota
	AcceptanceChainReachabilityStage
	AcceptanceChainFinancialControlStage
	AcceptanceChainDecisionStage
)

func (stage AcceptanceChainStage) String() string {
	switch stage {
	case AcceptanceChainReachabilityStage:
		return "REACHABILITY_JUDGMENT"
	case AcceptanceChainFinancialControlStage:
		return "FINANCIAL_CONTROL_JUDGMENT"
	case AcceptanceChainDecisionStage:
		return "ACCEPTANCE_DECISION"
	default:
		return ""
	}
}

// AcceptanceChainOutcome 是推进一整条接受链的处理结果。取值只有两个，与形成决定那一步
// 同义：本轮形成了决定，或者没形成。链停在哪一步由 Stage 单独交回，不折进结果取值——
// 折进去就得为每一步各造一个「未决」，而调用方对三者的处置完全相同（回滚重投）。
type AcceptanceChainOutcome uint8

const (
	AcceptanceChainOutcomeInvalid AcceptanceChainOutcome = iota
	AcceptanceChainDecided
	AcceptanceChainUndecided
)

func (outcome AcceptanceChainOutcome) String() string {
	switch outcome {
	case AcceptanceChainDecided:
		return "DECIDED"
	case AcceptanceChainUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// AdvanceAcceptanceChainCommand 带整份委托的成员清单：可达性逐成员判断，财务控制与形成
// 决定按整份委托一次。三者的作用面不同，因此清单只在本层展开，不下推给后两步。
type AdvanceAcceptanceChainCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelIDs []domain.DeclaredParcelID
}

type AdvanceAcceptanceChainResult struct {
	outcome      AcceptanceChainOutcome
	stage        AcceptanceChainStage
	state        domain.ShipmentRequestState
	decision     domain.AcceptanceDecision
	hasDecision  bool
	reason       JudgmentPendingReason
	continuation domain.OwnershipContinuationReference
}

func (result AdvanceAcceptanceChainResult) Outcome() AcceptanceChainOutcome {
	return result.outcome
}

// Stage 在未决时说明停在哪一步；形成决定那一轮它是 `ACCEPTANCE_DECISION`。
func (result AdvanceAcceptanceChainResult) Stage() AcceptanceChainStage {
	return result.stage
}

// State 是本轮之后委托的生命周期状态。前两步停下时恒为`已提交`——推进一条判断腿不是
// 接受也不是拒绝。
func (result AdvanceAcceptanceChainResult) State() domain.ShipmentRequestState {
	return result.state
}

// AcceptanceDecision 只在本轮真形成了决定时给出，理由与 FormAcceptanceDecisionResult
// 那一条相同：交回一份零值决定，下游最容易把它读成没有障碍。
func (result AdvanceAcceptanceChainResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

func (result AdvanceAcceptanceChainResult) PendingReason() JudgmentPendingReason {
	return result.reason
}

// ResumePath 交回本轮未决该由谁来续。消费门按它决定回滚重投还是入账（ADR-0094 Decision 一），
// 因此这里只是把 `JudgmentPendingReason.resumePath()` 那份唯一映射转交出去，不在此另立一套。
//
// 交恢复动作而不是交原因，是为了让消费门够不到「原因」这一层：它一旦按原因名字判，同一份
// 「谁能推动这件事」的知识就有了第二处定义，而两处漂开时的症状是同一个原因在两个门下一个
// 重投一个入账。形成决定那一轮它是零值——那一轮没有人在等。
func (result AdvanceAcceptanceChainResult) ResumePath() domain.ResumePath {
	return result.reason.resumePath()
}

func (result AdvanceAcceptanceChainResult) ContinuationReference() domain.OwnershipContinuationReference {
	return result.continuation
}

// AdvanceAcceptanceChainDeps 收拢三步编排。用结构体而不是位置参数，理由与
// FormAcceptanceDecisionDeps 同：一排同族的处理器在调用点认不出谁是谁。
type AdvanceAcceptanceChainDeps struct {
	Reachability     *AdvanceAcceptanceJudgmentHandler
	FinancialControl *AdvanceFinancialControlJudgmentHandler
	Decision         *FormAcceptanceDecisionHandler
}

// AdvanceAcceptanceChainHandler 按序推进接受判断的三步。
//
// 它自己不判任何一格，也不碰委托聚合：三步各自的不变量留在各自的编排里，本层只决定
// 顺序与在哪一步停。之所以要有这一层，是因为「提交之后自动推进接受判断」这件事在用例里
// 是一件事（UC-PS-001 步骤 8「自动接受」、`AT-PS-033`「不等待无依据的人工审批」），
// 而实现上是三个编排——没有这一层，顺序就得由每个装配点各写一遍。
type AdvanceAcceptanceChainHandler struct {
	deps AdvanceAcceptanceChainDeps
}

func NewAdvanceAcceptanceChainHandler(deps AdvanceAcceptanceChainDeps) *AdvanceAcceptanceChainHandler {
	return &AdvanceAcceptanceChainHandler{deps: deps}
}

// Handle 推进一轮：逐成员可达性 → 整份委托财务控制 → 形成决定。
//
// 任一步未决即整条停下，不往下走。往下走没有意义而且有害：形成决定读的是**已记录**的
// 判断，缺一条它只会再答一次`尚未决定`，代价是多记一次处理尝试，还把停顿原因换成了
// 下游那一步的措辞——运维据此会去查决定，而实际停的是上游那条腿。
//
// 装配缺件（三步任缺一个）在此响亮报错而不是当作未决：未决是「等一个依赖」，而少接一个
// 编排等着也不会长出来。
func (handler *AdvanceAcceptanceChainHandler) Handle(
	ctx context.Context,
	command AdvanceAcceptanceChainCommand,
) (AdvanceAcceptanceChainResult, error) {
	if handler.deps.Reachability == nil ||
		handler.deps.FinancialControl == nil ||
		handler.deps.Decision == nil {
		return AdvanceAcceptanceChainResult{}, ErrAcceptanceChainNotAssembled
	}
	if len(command.DeclaredParcelIDs) == 0 {
		return AdvanceAcceptanceChainResult{}, ErrAcceptanceChainHasNoMembers
	}

	// 逐成员推进可达性：命令按成员发起（AdvanceAcceptanceJudgmentCommand 带
	// DeclaredParcelID），而一份委托的成员各有各的可达性结论。任一成员停下即整条停——
	// 接受决定是整份委托的，缺一个成员的判断就形不成。
	for _, parcel := range command.DeclaredParcelIDs {
		result, err := handler.deps.Reachability.Handle(ctx, AdvanceAcceptanceJudgmentCommand{
			Identity:          command.Identity,
			ShipmentRequestID: command.ShipmentRequestID,
			SubmissionVersion: command.SubmissionVersion,
			DeclaredParcelID:  parcel,
		})
		if err != nil {
			return AdvanceAcceptanceChainResult{}, err
		}
		if result.Outcome() != AcceptanceJudgmentAdvanced {
			return AdvanceAcceptanceChainResult{
				outcome:      AcceptanceChainUndecided,
				stage:        AcceptanceChainReachabilityStage,
				state:        result.State(),
				reason:       result.PendingReason(),
				continuation: result.ContinuationReference(),
			}, nil
		}
	}

	control, err := handler.deps.FinancialControl.Handle(ctx, AdvanceFinancialControlJudgmentCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
	if err != nil {
		return AdvanceAcceptanceChainResult{}, err
	}
	if control.Outcome() != AcceptanceJudgmentAdvanced {
		return AdvanceAcceptanceChainResult{
			outcome:      AcceptanceChainUndecided,
			stage:        AcceptanceChainFinancialControlStage,
			state:        control.State(),
			reason:       control.PendingReason(),
			continuation: control.ContinuationReference(),
		}, nil
	}

	decision, err := handler.deps.Decision.Handle(ctx, FormAcceptanceDecisionCommand{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
	})
	if err != nil {
		return AdvanceAcceptanceChainResult{}, err
	}
	formed, hasDecision := decision.AcceptanceDecision()
	if decision.Outcome() != AcceptanceDecided {
		return AdvanceAcceptanceChainResult{
			outcome:      AcceptanceChainUndecided,
			stage:        AcceptanceChainDecisionStage,
			state:        decision.State(),
			reason:       decision.PendingReason(),
			continuation: decision.ContinuationReference(),
		}, nil
	}
	return AdvanceAcceptanceChainResult{
		outcome:     AcceptanceChainDecided,
		stage:       AcceptanceChainDecisionStage,
		state:       decision.State(),
		decision:    formed,
		hasDecision: hasDecision,
	}, nil
}
