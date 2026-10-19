package application

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// DepartureGatePreviewOutcome 是出发前门禁问询的结果代数（ADR-0219）。
//
// 没有「未决」一格：读数自己就装得下读不动与未决（ADR-0205 决定二），问询不落任何东西，也就没有登记册读不动、交出失败
// 那类要续办的停点。
type DepartureGatePreviewOutcome uint8

const (
	DepartureGatePreviewOutcomeInvalid DepartureGatePreviewOutcome = iota
	DepartureGatePreviewed
	DepartureGatePreviewNotAccepted
)

func (outcome DepartureGatePreviewOutcome) String() string {
	switch outcome {
	case DepartureGatePreviewed:
		return "DEPARTURE_GATE_PREVIEWED"
	case DepartureGatePreviewNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// PreviewDepartureGateCommand 携带一次出发前问询的输入：班次、声明的段与拟出发时点（ADR-0219 决定一）。
//
// 三格的读法逐字同 RecordMovementFactCommand 里同名的那三格（DepartsAt 对 OccurredAt）：段可缺、缺了读数停在「段未声明」，
// 与那条出发不报段时一样。事实身份、地点、来源、版本、资源与放行凭据不在这里——读数不取决于它们（ADR-0205 决定二、六），
// 收进来只会让人以为它们算数。
type PreviewDepartureGateCommand struct {
	TenantID  domain.TenantID
	Schedule  string
	Segment   string
	DepartsAt time.Time
}

type PreviewDepartureGateResult struct {
	outcome DepartureGatePreviewOutcome
	reading domain.DepartureGateReading
}

func (result PreviewDepartureGateResult) Outcome() DepartureGatePreviewOutcome {
	return result.outcome
}

// Reading 只在问到了时交回读数；未受理那一格没有读数，不是一格空读数。
func (result PreviewDepartureGateResult) Reading() (domain.DepartureGateReading, bool) {
	return result.reading, result.outcome == DepartureGatePreviewed
}

// PreviewDepartureGateHandler 答设备在动作之前的那一问：「此刻按这班、这段出发，关务怎么答」（ADR-0219，回答 ADR-0205
// 越权风险点 1）。
//
// 它只依赖出发登记用的那个 DepartureGateReader：依赖结构上没有任何写口，本上下文零写入不靠纪律。关务那一侧照 ADR-0206
// 决定六在自己的事务里记判断——问询也是对明确对象与拟执行动作的请求，事后同样要答得出「当时凭什么这么答」。
type PreviewDepartureGateHandler struct {
	reader *DepartureGateReader
}

// NewPreviewDepartureGateHandler 构造期拒缺件，理由同 NewDepartureGateReader。
func NewPreviewDepartureGateHandler(reader *DepartureGateReader) (*PreviewDepartureGateHandler, error) {
	if reader == nil {
		return nil, errors.New("transport fulfillment: the departure gate preview needs the departure gate reader")
	}
	return &PreviewDepartureGateHandler{reader: reader}, nil
}

// Preview 受理三格后走出发登记那一条读数路径，交回的读数与随后同参登记的出发同一套：同样的停点、同样逐对象问、同样的
// 压制次序。受理不了答未受理、不问任何一口；error 只留给读数自己拼不出来（实现坏了），与 DepartureGateReader.Read 同。
//
// 读数不存、不交给随后的出发：出发登记照 ADR-0205 在它自己那一刻重问。读到放行不等于出发那一刻仍放行。
func (handler *PreviewDepartureGateHandler) Preview(
	ctx context.Context,
	command PreviewDepartureGateCommand,
) (PreviewDepartureGateResult, error) {
	intent, accepted := departureGateIntentOf(command.TenantID, command.Schedule, command.Segment, command.DepartsAt)
	if !accepted {
		return PreviewDepartureGateResult{outcome: DepartureGatePreviewNotAccepted}, nil
	}
	reading, err := handler.reader.readIntent(ctx, intent)
	if err != nil {
		return PreviewDepartureGateResult{}, err
	}
	return PreviewDepartureGateResult{outcome: DepartureGatePreviewed, reading: reading}, nil
}
