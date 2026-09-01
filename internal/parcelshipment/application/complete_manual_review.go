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
type ManualReviewCompletionOutcome uint8

const (
	ManualReviewCompletionOutcomeInvalid ManualReviewCompletionOutcome = iota
	ManualReviewCompletionRecorded
	ManualReviewCompletionAlreadyDone
	ManualReviewTaskAlreadyClosed
	ManualReviewVersionSuperseded
	ManualReviewCompletionConflict
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
	default:
		return ""
	}
}

// CompleteManualReviewCommand 说明谁、凭什么授权与证据，为哪一份提交版本完成了复核。
// 三个引用必填由领域构造函数把守；版本必填是本层的门——复核完成挂在版本的判断任务上，
// 不指名版本的完成无从判断它签给了谁。
type CompleteManualReviewCommand struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Authority         domain.ReviewAuthorityReference
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
	Requests ports.ShipmentRequestRepository
	Clock    ports.Clock
}

type CompleteManualReviewHandler struct {
	deps CompleteManualReviewDeps
}

func NewCompleteManualReviewHandler(deps CompleteManualReviewDeps) *CompleteManualReviewHandler {
	return &CompleteManualReviewHandler{deps: deps}
}

// Handle 把一次已完成的人工复核记到目标提交版本的接受判断任务上。
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
		Authority:   command.Authority,
		Reviewer:    command.Reviewer,
		Evidence:    command.Evidence,
		CompletedAt: handler.deps.Clock.Now(),
	})
	if err != nil {
		// 引用的必填由 Intake 在构造领域值时把守，走到这里还缺就是装配或编程错误。
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
