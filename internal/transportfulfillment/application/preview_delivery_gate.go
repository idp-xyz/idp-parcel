package application

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// DeliveryGatePreviewOutcome 是派送出发前交付门禁问询的结果代数（ADR-0238）。与出发前问询同形：没有「未决」一格——读数自己
// 就装得下读不动与未决，问询不落任何东西，也就没有登记册读不动、交出失败那类要续办的停点。
type DeliveryGatePreviewOutcome uint8

const (
	DeliveryGatePreviewOutcomeInvalid DeliveryGatePreviewOutcome = iota
	DeliveryGatePreviewed
	DeliveryGatePreviewNotAccepted
)

func (outcome DeliveryGatePreviewOutcome) String() string {
	switch outcome {
	case DeliveryGatePreviewed:
		return "DELIVERY_GATE_PREVIEWED"
	case DeliveryGatePreviewNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// PreviewDeliveryGateCommand 携带一次交付前问询的输入：对象与拟交付时点（ADR-0238 决定一）。
//
// 对象的读法逐字同 RegisterEffectiveDeliveryCommand 的那一格；拟交付时点对的是随后那次派送尝试里这件对象结果的发生时刻——交付
// 登记按它问关务。尝试、方式、接收方与 POD 不在这里：读数不取决于它们，派送出发之前也还没有尝试。
type PreviewDeliveryGateCommand struct {
	TenantID   domain.TenantID
	Object     string
	DeliversAt time.Time
}

type PreviewDeliveryGateResult struct {
	outcome DeliveryGatePreviewOutcome
	reading domain.DeliveryGateReading
}

func (result PreviewDeliveryGateResult) Outcome() DeliveryGatePreviewOutcome {
	return result.outcome
}

// Reading 只在问到了时交回读数；未受理那一格没有读数，不是一格空读数。
func (result PreviewDeliveryGateResult) Reading() (domain.DeliveryGateReading, bool) {
	return result.reading, result.outcome == DeliveryGatePreviewed
}

// PreviewDeliveryGateHandler 答尾程设备在派送出发之前的那一问：「这件按这一刻交到收件方手里，关务怎么答」（ADR-0238，回答
// ADR-0213 越权风险点 20）。
//
// 它只依赖交付登记用的那个 DeliveryGateReader：依赖结构上没有任何写口，本上下文零写入不靠纪律。关务那一侧照 ADR-0206 决定六
// 在自己的事务里记判断，随后同参的交付登记在同样三源上命中同一判断。
type PreviewDeliveryGateHandler struct {
	reader *DeliveryGateReader
}

// NewPreviewDeliveryGateHandler 构造期拒缺件，理由同 NewDeliveryGateReader。
func NewPreviewDeliveryGateHandler(reader *DeliveryGateReader) (*PreviewDeliveryGateHandler, error) {
	if reader == nil {
		return nil, errors.New("transport fulfillment: the delivery gate preview needs the delivery gate reader")
	}
	return &PreviewDeliveryGateHandler{reader: reader}, nil
}

// Preview 受理两格后走交付登记那一条读数路径，交回的读数与随后同参登记的交付同一套：同样取 at、同样的停点、同样问关务。受理不了
// 答未受理、不问任何一口；error 只留给读数自己拼不出来（实现坏了），与 DeliveryGateReader.ReadRegistration 同。
//
// 读数不存、不交给随后的交付：交付登记照 ADR-0213 决定五在它自己那一刻重问。读到放行不等于交到收件方手里那一刻仍放行。
func (handler *PreviewDeliveryGateHandler) Preview(
	ctx context.Context,
	command PreviewDeliveryGateCommand,
) (PreviewDeliveryGateResult, error) {
	intent, accepted := deliveryGateIntentOf(command.TenantID, command.Object, command.DeliversAt)
	if !accepted {
		return PreviewDeliveryGateResult{outcome: DeliveryGatePreviewNotAccepted}, nil
	}
	reading, err := handler.reader.readIntent(ctx, intent)
	if err != nil {
		return PreviewDeliveryGateResult{}, err
	}
	return PreviewDeliveryGateResult{outcome: DeliveryGatePreviewed, reading: reading}, nil
}
