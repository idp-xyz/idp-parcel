// record_evaluation_unformed.go 接 ADR-0229 决定三的计价一段：计价对一份评价请求答不出评价时，经信封告诉结算，结算在
// 这份请求上记停因。这几格重投不会变好，按未决重投只会耗尽失败预算、信封即弃，请求在结算这一侧就永远停在`已提交`、
// 谁也看不出它卡在哪；记下停因，补登之后才有东西按它续办。
package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrUnformedEvaluationRequestMissing 说明计价说它对一份请求答不出评价，结算的请求册上却没有这份请求。请求先于它的
	// 信封在同一库提交，计价按标识读回请求才作答，所以这不是可见性滞后，是两侧说法矛盾。
	ErrUnformedEvaluationRequestMissing = errors.New("settlement accounting: the request pricing could not answer is absent from the register")
	// ErrUnformedEvaluationRequestConcluded 说明那份请求在册上已有另一份结论或另一个停因。计价对一份请求只作答一次，
	// 撞上别的结论只可能是两侧说法矛盾；不覆盖，作错误交人看。
	ErrUnformedEvaluationRequestConcluded = errors.New("settlement accounting: the request pricing could not answer already carries another conclusion")
)

// RecordEvaluationUnformedCommand 是计价答不出评价的那一拍：哪个方向的哪份请求、停在哪一格。
type RecordEvaluationUnformedCommand struct {
	TenantID    domain.TenantID
	Direction   domain.EvaluationDirection
	SellRequest domain.SellEvaluationRequestID
	BuyRequest  domain.EvaluationRequestID
	Cause       domain.EvaluationStopCause
}

// RecordEvaluationUnformedOutcome 是本编排的答案。
type RecordEvaluationUnformedOutcome uint8

const (
	RecordEvaluationUnformedOutcomeInvalid RecordEvaluationUnformedOutcome = iota
	// EvaluationUnformedRecorded：停因记下了，或册上已是同一个停因（重放）。
	EvaluationUnformedRecorded
	// EvaluationUnformedNotAccepted：命令立不住（方向与请求标识对不上、停因不是计价那一段的）。
	EvaluationUnformedNotAccepted
	// EvaluationUnformedUndecided：请求册答不出，整笔回滚等重投。
	EvaluationUnformedUndecided
)

func (outcome RecordEvaluationUnformedOutcome) String() string {
	switch outcome {
	case EvaluationUnformedRecorded:
		return "EVALUATION_UNFORMED_RECORDED"
	case EvaluationUnformedNotAccepted:
		return "EVALUATION_UNFORMED_NOT_ACCEPTED"
	case EvaluationUnformedUndecided:
		return "EVALUATION_UNFORMED_UNDECIDED"
	default:
		return ""
	}
}

// RecordEvaluationUnformedResult 带答案与未决时依赖原样答出的错误。
type RecordEvaluationUnformedResult struct {
	outcome RecordEvaluationUnformedOutcome
	cause   error
}

func (result RecordEvaluationUnformedResult) Outcome() RecordEvaluationUnformedOutcome {
	return result.outcome
}

func (result RecordEvaluationUnformedResult) UndecidedCause() error { return result.cause }

// RecordEvaluationUnformedDeps 两本请求册各一读一写：SELL 那本记结论（停因推出`未形成`），BUY 那本记停因。
type RecordEvaluationUnformedDeps struct {
	SellRequests    ports.SellEvaluationRequestView
	SellConclusions ports.SellEvaluationRequestConclusionRecorder
	BuyRequests     ports.EvaluationRequestView
	BuyStops        ports.EvaluationRequestStopRecorder
	Clock           ports.Clock
}

type RecordEvaluationUnformedHandler struct {
	deps RecordEvaluationUnformedDeps
}

func NewRecordEvaluationUnformedHandler(deps RecordEvaluationUnformedDeps) (*RecordEvaluationUnformedHandler, error) {
	if deps.SellRequests == nil || deps.SellConclusions == nil || deps.BuyRequests == nil || deps.BuyStops == nil ||
		deps.Clock == nil {
		return nil, fmt.Errorf("%w: record evaluation unformed", ErrNilDependency)
	}
	return &RecordEvaluationUnformedHandler{deps: deps}, nil
}

// Handle 读请求 → 记停因。请求不在册上、册上另有结论都是矛盾，上抛；只有册答不出才是未决。
func (handler *RecordEvaluationUnformedHandler) Handle(
	ctx context.Context,
	command RecordEvaluationUnformedCommand,
) (RecordEvaluationUnformedResult, error) {
	if command.TenantID.String() == "" || command.Cause.Stage() != domain.StopInPricing {
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedNotAccepted}, nil
	}
	switch command.Direction {
	case domain.SellEvaluationDirection:
		if command.SellRequest.String() == "" || command.BuyRequest.String() != "" {
			return RecordEvaluationUnformedResult{outcome: EvaluationUnformedNotAccepted}, nil
		}
		return handler.recordSell(ctx, command)
	case domain.BuyEvaluationDirection:
		if command.BuyRequest.String() == "" || command.SellRequest.String() != "" {
			return RecordEvaluationUnformedResult{outcome: EvaluationUnformedNotAccepted}, nil
		}
		return handler.recordBuy(ctx, command)
	default:
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedNotAccepted}, nil
	}
}

func (handler *RecordEvaluationUnformedHandler) recordSell(
	ctx context.Context,
	command RecordEvaluationUnformedCommand,
) (RecordEvaluationUnformedResult, error) {
	key := ports.SellEvaluationRequestKey{TenantID: command.TenantID, Request: command.SellRequest}
	if _, found, err := handler.deps.SellRequests.FindByID(ctx, key); err != nil {
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedUndecided, cause: err}, nil
	} else if !found {
		return RecordEvaluationUnformedResult{}, fmt.Errorf("%w: sell request %s", ErrUnformedEvaluationRequestMissing, command.SellRequest)
	}
	conclusion, err := domain.NewStoppedSellEvaluationRequestConclusion(command.Cause, handler.deps.Clock.Now())
	if err != nil {
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedNotAccepted}, nil
	}
	effect, err := handler.deps.SellConclusions.RecordConclusion(ctx, key, conclusion)
	if err != nil {
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedUndecided, cause: err}, nil
	}
	switch effect {
	case ports.SellEvaluationRequestConcluded, ports.SellEvaluationRequestConclusionReplayed:
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedRecorded}, nil
	case ports.SellEvaluationRequestConcludedOtherwise:
		return RecordEvaluationUnformedResult{}, fmt.Errorf("%w: sell request %s, wanted %s",
			ErrUnformedEvaluationRequestConcluded, command.SellRequest, command.Cause)
	default:
		return RecordEvaluationUnformedResult{}, fmt.Errorf("%w: %d", ErrUnexpectedSellRequestConclusionEffect, effect)
	}
}

func (handler *RecordEvaluationUnformedHandler) recordBuy(
	ctx context.Context,
	command RecordEvaluationUnformedCommand,
) (RecordEvaluationUnformedResult, error) {
	key := ports.EvaluationRequestKey{TenantID: command.TenantID, Request: command.BuyRequest}
	if _, found, err := handler.deps.BuyRequests.FindByID(ctx, key); err != nil {
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedUndecided, cause: err}, nil
	} else if !found {
		return RecordEvaluationUnformedResult{}, fmt.Errorf("%w: buy request %s", ErrUnformedEvaluationRequestMissing, command.BuyRequest)
	}
	effect, err := handler.deps.BuyStops.RecordStop(ctx, key, command.Cause, handler.deps.Clock.Now())
	if err != nil {
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedUndecided, cause: err}, nil
	}
	switch effect {
	case ports.EvaluationRequestStopped, ports.EvaluationRequestStopReplayed:
		return RecordEvaluationUnformedResult{outcome: EvaluationUnformedRecorded}, nil
	case ports.EvaluationRequestStoppedOtherwise:
		return RecordEvaluationUnformedResult{}, fmt.Errorf("%w: buy request %s, wanted %s",
			ErrUnformedEvaluationRequestConcluded, command.BuyRequest, command.Cause)
	default:
		return RecordEvaluationUnformedResult{}, fmt.Errorf("settlement accounting: unexpected buy evaluation request stop effect %d", effect)
	}
}
