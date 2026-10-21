package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ErrEvaluationUnformedRecordingUndecided 是「评价未形成」那一路唯一的未决哨兵：请求册答不出，停因记没记下未知，整笔回滚
// 等重投。生产装配把它登进这一路的哨兵名单。
var ErrEvaluationUnformedRecordingUndecided = errors.New(
	"settlement accounting parcelpricing adapter: recording the unformed evaluation request is undecided")

// EvaluationUnformedRecording 是本适配器转交的编排，生产装配接 saapplication.RecordEvaluationUnformedHandler。
type EvaluationUnformedRecording interface {
	Handle(
		ctx context.Context,
		command saapplication.RecordEvaluationUnformedCommand,
	) (saapplication.RecordEvaluationUnformedResult, error)
}

// unformedCauses 是计价那一侧「为什么答不出评价」的词到本上下文停因的对照。计价的三格各对一个停因；词表外的词是两侧
// 契约漂了，不猜它像哪一格。
var unformedCauses = map[string]sadomain.EvaluationStopCause{
	"PRICE_CARD_NOT_CONFIGURED":         sadomain.StopPriceCardNotConfigured,
	"PRICE_CARD_APPLICABILITY_CONFLICT": sadomain.StopPriceCardApplicabilityConflict,
	"PRICING_INPUT_UNAVAILABLE":         sadomain.StopPricingInputUnavailable,
}

// unformedDirections 是计价的价格方向到本上下文评价方向的对照：BUY 请求问的是供应商成本，SELL 请求问的是客户费用。
var unformedDirections = map[string]sadomain.EvaluationDirection{
	"BUY":  sadomain.BuyEvaluationDirection,
	"SELL": sadomain.SellEvaluationDirection,
}

// RecordOnEvaluationRequestUnformedAdapter 把计价的「评价未形成」译成本上下文的命令（ADR-0025：跨上下文翻译在消费侧）。
type RecordOnEvaluationRequestUnformedAdapter struct {
	recording EvaluationUnformedRecording
}

func NewRecordOnEvaluationRequestUnformedAdapter(recording EvaluationUnformedRecording) (*RecordOnEvaluationRequestUnformedAdapter, error) {
	if recording == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: evaluation unformed recording is nil")
	}
	return &RecordOnEvaluationRequestUnformedAdapter{recording: recording}, nil
}

var _ sainbox.UnformedEvaluationRequestHandler = (*RecordOnEvaluationRequestUnformedAdapter)(nil)

// HandleUnformedEvaluationRequest 译 → 记 → 折结果。译不过去与命令被拒都是确定性坏答案，落 publish_failed；编排上抛的
// 矛盾（请求不在册上、册上另有结论）原样交回，同样落 publish_failed；只有册答不出才是未决。
func (adapter *RecordOnEvaluationRequestUnformedAdapter) HandleUnformedEvaluationRequest(
	ctx context.Context,
	unformed sainbox.UnformedEvaluationRequest,
) error {
	tenant, err := sadomain.NewTenantID(unformed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	direction, known := unformedDirections[unformed.Direction]
	if !known {
		return fmt.Errorf("%w: direction %q", ErrUntranslatableAnswer, unformed.Direction)
	}
	cause, known := unformedCauses[unformed.Cause]
	if !known {
		return fmt.Errorf("%w: unformed cause %q", ErrUntranslatableAnswer, unformed.Cause)
	}
	command := saapplication.RecordEvaluationUnformedCommand{TenantID: tenant, Direction: direction, Cause: cause}
	switch direction {
	case sadomain.SellEvaluationDirection:
		if command.SellRequest, err = sadomain.NewSellEvaluationRequestID(unformed.Request); err != nil {
			return fmt.Errorf("%w: sell request: %v", ErrUntranslatableReference, err)
		}
	default:
		if command.BuyRequest, err = sadomain.NewEvaluationRequestID(unformed.Request); err != nil {
			return fmt.Errorf("%w: buy request: %v", ErrUntranslatableReference, err)
		}
	}
	result, err := adapter.recording.Handle(ctx, command)
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case saapplication.EvaluationUnformedRecorded:
		return nil
	case saapplication.EvaluationUnformedUndecided:
		return fmt.Errorf("%w: request %s: %v", ErrEvaluationUnformedRecordingUndecided, unformed.Request, result.UndecidedCause())
	case saapplication.EvaluationUnformedNotAccepted:
		return fmt.Errorf("%w: unformed %s request %s refused", ErrUntranslatableAnswer, unformed.Direction, unformed.Request)
	default:
		return fmt.Errorf("settlement accounting parcelpricing adapter: unexpected unformed recording outcome %d", result.Outcome())
	}
}
