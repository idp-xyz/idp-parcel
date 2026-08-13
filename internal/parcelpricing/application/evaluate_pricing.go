// Package application 编排 parcelpricing 的评价用例。计算全部在领域（EvaluatePricing
// 是纯函数），这里只做受理、幂等、存续与交付的协调。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrUnexpectedEvaluationSave 说明评价库交回了封闭集合以外的写入结果。
var ErrUnexpectedEvaluationSave = errors.New("parcel pricing: unexpected evaluation save outcome")

// EvaluatePricingOutcome 是评价请求的应用处理结果。
type EvaluatePricingOutcome uint8

const (
	EvaluatePricingOutcomeInvalid EvaluatePricingOutcome = iota
	EvaluationRecorded
	EvaluationExistingResult
	EvaluationConflict
	EvaluationRequestNotAccepted
	EvaluationUndecided
)

func (outcome EvaluatePricingOutcome) String() string {
	switch outcome {
	case EvaluationRecorded:
		return "RECORDED"
	case EvaluationExistingResult:
		return "EXISTING_RESULT"
	case EvaluationConflict:
		return "CONFLICT"
	case EvaluationRequestNotAccepted:
		return "NOT_ACCEPTED"
	case EvaluationUndecided:
		return "UNDECIDED"
	default:
		return ""
	}
}

// EvaluatePricingCommand 携带一次评价请求。请求本体（方案版本、输入快照、证据层级、
// 重放引用）全部在领域对象内，编排不拆解它。
type EvaluatePricingCommand struct {
	Request domain.EvaluationRequest
}

type EvaluatePricingResult struct {
	outcome    EvaluatePricingOutcome
	evaluation domain.PricingEvaluation
	hasRecord  bool
	handoffRef string
}

func (result EvaluatePricingResult) Outcome() EvaluatePricingOutcome {
	return result.outcome
}

func (result EvaluatePricingResult) Evaluation() (domain.PricingEvaluation, bool) {
	return result.evaluation, result.hasRecord
}

// HandoffReference 非空说明评价已入册但意图还没交出去，重放会重发同一份。
func (result EvaluatePricingResult) HandoffReference() string {
	return result.handoffRef
}

type EvaluatePricingDeps struct {
	Store      ports.EvaluationStore
	Downstream ports.EvaluationHandoff
	Clock      ports.Clock
}

type EvaluatePricingHandler struct {
	deps EvaluatePricingDeps
}

func NewEvaluatePricingHandler(deps EvaluatePricingDeps) *EvaluatePricingHandler {
	return &EvaluatePricingHandler{deps: deps}
}

// Handle 把一次评价请求推进到入册结果：幂等按评价标识（同标识同语义即重放返原——
// 评价是纯计算，重复请求不重算不换结果；同标识异语义是冲突不顶替）→ EvaluatePricing
// 纯函数（失败评价同样是版本化结果，照样入册与交付——解释里带着失败原因，不是丢弃品）
// → 保存与意图。金额、精度、取整全在领域结果内，编排零算术。
func (handler *EvaluatePricingHandler) Handle(
	ctx context.Context,
	command EvaluatePricingCommand,
) (EvaluatePricingResult, error) {
	if command.Request.ID().String() == "" {
		return EvaluatePricingResult{outcome: EvaluationRequestNotAccepted}, nil
	}

	existing, found, err := handler.deps.Store.FindByID(ctx, command.Request.ID())
	if err != nil {
		return EvaluatePricingResult{outcome: EvaluationUndecided}, nil
	}
	if found {
		return handler.settleAgainstExisting(ctx, command, existing), nil
	}

	evaluation := domain.EvaluatePricing(command.Request)

	saved, err := handler.deps.Store.Save(ctx, evaluation)
	if err != nil {
		return EvaluatePricingResult{outcome: EvaluationUndecided}, nil
	}
	switch saved {
	case ports.EvaluationSaved:
		result := EvaluatePricingResult{
			outcome:    EvaluationRecorded,
			evaluation: evaluation,
			hasRecord:  true,
		}
		result.handoffRef = handler.handOff(ctx, evaluation)
		return result, nil
	case ports.EvaluationAlreadyRecorded:
		winner, found, err := handler.deps.Store.FindByID(ctx, command.Request.ID())
		if err != nil || !found {
			return EvaluatePricingResult{outcome: EvaluationUndecided}, nil
		}
		return handler.settleAgainstExisting(ctx, command, winner), nil
	default:
		return EvaluatePricingResult{}, fmt.Errorf("%w: %d", ErrUnexpectedEvaluationSave, saved)
	}
}

// settleAgainstExisting 分辨重放与冒名：把本次请求重算一遍（纯函数，无副作用）后比
// 语义摘要——摘要含选中事实、金额与解释的全部语义，同摘要即同一次计算的重复请求，
// 异摘要即同标识装了不同内容，原评价不顶替。
func (handler *EvaluatePricingHandler) settleAgainstExisting(
	ctx context.Context,
	command EvaluatePricingCommand,
	existing domain.PricingEvaluation,
) EvaluatePricingResult {
	replayed := domain.EvaluatePricing(command.Request)
	if replayed.SemanticDigest() != existing.SemanticDigest() {
		return EvaluatePricingResult{outcome: EvaluationConflict}
	}
	result := EvaluatePricingResult{
		outcome:    EvaluationExistingResult,
		evaluation: existing,
		hasRecord:  true,
	}
	result.handoffRef = handler.handOff(ctx, existing)
	return result
}

// handOff 交发布意图。失败不翻评价，留续办引用重发同一份。
func (handler *EvaluatePricingHandler) handOff(
	ctx context.Context,
	evaluation domain.PricingEvaluation,
) string {
	if err := handler.deps.Downstream.HandOffEvaluation(ctx, ports.EvaluationHandoffIntent{
		Evaluation: evaluation,
	}); err == nil {
		return ""
	}
	return "CONT-EVALUATION/" + evaluation.ID().String()
}
