package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ManualReviewCompletionOutcome 是记录一次人工复核完成的应用处理结果。
//
// `已有完成`与`任务已完结`分开：前者是同一提交版本上的重复提交或换人补签，读回既有完成即可；
// 后者是决定已越过提交边界（或任务已随撤回停止），复核没有可补录的对象——CONTEXT 明写补录
// 等于让复核去追认一个已经形成的结果。`版本已换代`单列：操作员复核的是某一份提交版本，新版本
// 到达后那份复核对象已不存在，把完成记到新任务上就是把 A 版的判断签到 B 版头上。
//
// `未获授权`与`授权规则未配置`是 party-commercial 答复的两格落点（UC-PC-003 结果表），照主动拒绝
// 那一步的分法：前者是确定的业务答案，续办也补不出授权来；后者是租户还没把 `PAR-COM-14` 的授权
// 规则登记上——首发期没有租户，每一次询问都落在这一格，压成前者等于对一个尚未配置的产品说
// 「你无权复核」。两格都什么也不落库。
type ManualReviewCompletionOutcome uint8

const (
	ManualReviewCompletionOutcomeInvalid ManualReviewCompletionOutcome = iota
	ManualReviewCompletionRecorded
	ManualReviewCompletionAlreadyDone
	ManualReviewTaskAlreadyClosed
	ManualReviewVersionSuperseded
	ManualReviewCompletionConflict
	ManualReviewNotAuthorized
	ManualReviewAuthorityRulesNotConfigured
)

func (outcome ManualReviewCompletionOutcome) String() string {
	switch outcome {
	case ManualReviewCompletionRecorded:
		return "RECORDED"
	case ManualReviewCompletionAlreadyDone:
		return "ALREADY_COMPLETED"
	case ManualReviewTaskAlreadyClosed:
		return "TASK_ALREADY_CLOSED"
	case ManualReviewVersionSuperseded:
		return "VERSION_SUPERSEDED"
	case ManualReviewCompletionConflict:
		return "REVISION_CONFLICT"
	case ManualReviewNotAuthorized:
		return "NOT_AUTHORIZED"
	case ManualReviewAuthorityRulesNotConfigured:
		return "AUTHORITY_RULES_NOT_CONFIGURED"
	default:
		return ""
	}
}

// CompleteManualReviewCommand 说明谁、凭什么证据，为哪一份提交版本完成了复核。
//
// 它不带授权引用：那由 party-commercial 签发（所采用的授权规则版本），本编排去问，不由调用方
// 声明——与 RejectShipmentRequestCommand 同一条理由，自带一个就等于自己给自己签字。此前这一格由
// Intake 整组注入、没人向 PC 问过，票 wiring-baseline-remainder/04 接的就是这条。
// 复核人与证据必填由领域构造函数把守；版本必填是本层的门——复核完成挂在版本的判断任务上，
// 不指名版本的完成无从判断它签给了谁。
type CompleteManualReviewCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Reviewer          domain.ReviewerReference
	Evidence          domain.ReviewEvidenceReference
}

type CompleteManualReviewResult struct {
	outcome        ManualReviewCompletionOutcome
	state          domain.ShipmentRequestState
	completion     domain.ManualReviewCompletion
	hasCompletion  bool
	decision       domain.AcceptanceDecision
	hasDecision    bool
	currentVersion domain.SubmissionVersionID
}

func (result CompleteManualReviewResult) Outcome() ManualReviewCompletionOutcome {
	return result.outcome
}

func (result CompleteManualReviewResult) State() domain.ShipmentRequestState {
	return result.state
}

// Completion 交回任务上的完成留痕：本次记下的，或`已有完成`时先到的那一份。缺席即本轮
// 没有完成可读（任务已完结或版本已换代）。
func (result CompleteManualReviewResult) Completion() (domain.ManualReviewCompletion, bool) {
	return result.completion, result.hasCompletion
}

// AcceptanceDecision 只在`任务已完结`且决定确实形成时给出，调用方据以告知操作员结果已定，
// 复核无处可签。撤回导致的停止没有决定可交。
func (result CompleteManualReviewResult) AcceptanceDecision() (domain.AcceptanceDecision, bool) {
	return result.decision, result.hasDecision
}

// CurrentVersion 是读取时刻的当前提交版本，`版本已换代`时调用方据以重读队列。
func (result CompleteManualReviewResult) CurrentVersion() domain.SubmissionVersionID {
	return result.currentVersion
}

type CompleteManualReviewDeps struct {
	Requests   ports.ShipmentRequestRepository
	Authorizer ports.ManualReviewAuthorizer
	Clock      ports.Clock
}

type CompleteManualReviewHandler struct {
	deps CompleteManualReviewDeps
}

func NewCompleteManualReviewHandler(deps CompleteManualReviewDeps) *CompleteManualReviewHandler {
	return &CompleteManualReviewHandler{deps: deps}
}

// Handle 把一次已完成的人工复核记到目标提交版本的接受判断任务上。
//
// 授权先于一切写动作，也先于版本核对与领域判断（顺序同 RejectShipmentRequestHandler）：
// 「只由规则授权的角色按证据完成复核」（UC-PS-001 `AT-PS-034`）里的「规则」属 party-commercial，
// 编排拿到委托就去问它，未获授权时不碰聚合；`版本已换代`那类续办提示是给有权复核的人重读队列
// 用的，不该先于授权答给任何人。
//
// 它只记录完成，不驱动判断：CONTEXT 明写「复核完成本身不形成决定，决定仍由判断任务按适用
// 规则形成」，而判断的推进属派发一拍（ADR-0081）——完成落库的同一事务交出「复核已完成」
// 信封（装配点的事务壳承担，见 cmd/parcel-api），接受链由消费门按信封再驱一拍。在这里顺手
// 推链，判断就有了第二个驱动点，两个驱动点的未决语义会各自漂移。
func (handler *CompleteManualReviewHandler) Handle(
	ctx context.Context,
	command CompleteManualReviewCommand,
) (CompleteManualReviewResult, error) {
	request, found, err := handler.deps.Requests.FindBySourceIdentity(ctx, command.Identity)
	if err != nil {
		return CompleteManualReviewResult{}, fmt.Errorf("complete manual review: %w", err)
	}
	if !found {
		// 指名一份查不到的委托是调用方的错。接 HTTP 时与其余未形成答案一并 5xx，
		// 不细分出「未找到」——理由同撤回端点：拆开就是把统一不可见结果拆开。
		return CompleteManualReviewResult{}, fmt.Errorf("complete manual review: %w", domain.ErrInvalidShipmentRequest)
	}

	authority, err := handler.deps.Authorizer.AuthorizeManualReview(ctx, ports.ManualReviewAuthorizationQuery{
		Identity:          command.Identity,
		ShipmentRequestID: command.ShipmentRequestID,
		SubmissionVersion: command.SubmissionVersion,
		Reviewer:          command.Reviewer,
		Evidence:          command.Evidence,
	})
	if err != nil {
		// 权威答不出是未形成，不冒充不允许或未配置（UC-PC-003 结果表）；与本编排其余
		// 未形成答案一并上抛，HTTP 侧 5xx。
		return CompleteManualReviewResult{}, fmt.Errorf("complete manual review: review authority: %w", err)
	}
	switch authority.Outcome {
	case ports.AuthorizationGranted:
	case ports.AuthorizationRefused:
		return CompleteManualReviewResult{
			outcome: ManualReviewNotAuthorized,
			state:   request.State(),
		}, nil
	case ports.AuthorizationRulesNotConfigured:
		return CompleteManualReviewResult{
			outcome: ManualReviewAuthorityRulesNotConfigured,
			state:   request.State(),
		}, nil
	default:
		return CompleteManualReviewResult{}, ErrUnexpectedAuthorizationOutcome
	}

	task := request.AcceptanceDecisionTask()
	current := task.SubmissionVersionID()
	if current != command.SubmissionVersion {
		// 复核对象已被新提交版本换代。不记到新任务上，也不报错重试——重试一万次版本
		// 也不会换回来；操作员要做的是按当前版本重新复核。
		return CompleteManualReviewResult{
			outcome:        ManualReviewVersionSuperseded,
			state:          request.State(),
			currentVersion: current,
		}, nil
	}

	completion, err := domain.NewManualReviewCompletion(domain.ManualReviewCompletionSpec{
		Authority:   authority.Authority,
		Reviewer:    command.Reviewer,
		Evidence:    command.Evidence,
		CompletedAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		// 复核人与证据的必填由 Intake 在构造领域值时把守，授权引用由端口契约保证`已授权`时必带；
		// 走到这里还缺就是装配或编程错误。
		return CompleteManualReviewResult{}, fmt.Errorf("complete manual review: %w", err)
	}

	reviewed, err := request.CompleteManualReview(completion)
	if err != nil {
		if errors.Is(err, domain.ErrManualReviewAlreadyCompleted) {
			existing, _ := task.ManualReviewCompletion()
			return CompleteManualReviewResult{
				outcome:        ManualReviewCompletionAlreadyDone,
				state:          request.State(),
				completion:     existing,
				hasCompletion:  true,
				currentVersion: current,
			}, nil
		}
		if errors.Is(err, domain.ErrAcceptanceTaskComplete) {
			decision, formed := request.AcceptanceDecision()
			return CompleteManualReviewResult{
				outcome:        ManualReviewTaskAlreadyClosed,
				state:          request.State(),
				decision:       decision,
				hasDecision:    formed,
				currentVersion: current,
			}, nil
		}
		return CompleteManualReviewResult{}, fmt.Errorf("complete manual review: %w", err)
	}

	saved, err := handler.deps.Requests.Save(ctx, command.Identity, reviewed)
	if err != nil {
		return CompleteManualReviewResult{}, fmt.Errorf("complete manual review: %w", err)
	}
	if saved != ports.ShipmentRequestSaved {
		// 版本冲突是业务答案（ADR-0031）：抢先那一方可能是另一次完成、一次换代或一个
		// 决定。调用方重读再重放，本层不代猜库里此刻是什么。
		return CompleteManualReviewResult{
			outcome:        ManualReviewCompletionConflict,
			state:          request.State(),
			currentVersion: current,
		}, nil
	}

	return CompleteManualReviewResult{
		outcome:        ManualReviewCompletionRecorded,
		state:          reviewed.State(),
		completion:     completion,
		hasCompletion:  true,
		currentVersion: current,
	}, nil
}
