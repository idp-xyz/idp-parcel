package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// SegmentCloseOutcome 是关段的结果代数。
//
// `仍有在场参与`单独成格而不并进`输入不受理`：两者的续办动作不同——前者去结束剩下的那几条参与
// （或者什么都不做，段本来就该继续存在），后者改输入。`早已关闭`是业务答案不是失败：声明重放、
// 并发下另一方先关，都答这一格。
type SegmentCloseOutcome uint8

const (
	SegmentCloseOutcomeInvalid SegmentCloseOutcome = iota
	SegmentClosedNow
	SegmentAlreadyClosed
	SegmentStillHasActiveParticipations
	SegmentToCloseNotFound
	SegmentCloseNotAccepted
	SegmentCloseUndecided
)

func (outcome SegmentCloseOutcome) String() string {
	switch outcome {
	case SegmentClosedNow:
		return "SEGMENT_CLOSED"
	case SegmentAlreadyClosed:
		return "SEGMENT_ALREADY_CLOSED"
	case SegmentStillHasActiveParticipations:
		return "SEGMENT_STILL_ACTIVE"
	case SegmentToCloseNotFound:
		return "SEGMENT_NOT_FOUND"
	case SegmentCloseNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case SegmentCloseUndecided:
		return "SEGMENT_CLOSE_UNDECIDED"
	default:
		return ""
	}
}

// CloseFulfillmentSegmentCommand 是「不再接受新对象」那个声明本身。
//
// CONTEXT 生命周期④要求「全部有效参与关系已经结束**且不再接受新对象**」才结束段。前半是参与关系
// 的状态，读回来就知道；后半是一个决定，任何状态都推导不出来——所以它只能作为一条命令由人给，
// 结束最后一条参与时不顺手做（票 tf-unwired-seven/07 的裁定）。
//
// ClosedAt 由调用方给：决定是何时做的不由写库那一刻定，与终止那一路的 EndedAt 同理。**不带依据
// 字段**——领域与表上都没有它，加它是改模型不是加编排。
type CloseFulfillmentSegmentCommand struct {
	TenantID domain.TenantID
	Segment  string
	ClosedAt time.Time
}

type CloseFulfillmentSegmentResult struct {
	outcome      SegmentCloseOutcome
	continuation string
}

func (result CloseFulfillmentSegmentResult) Outcome() SegmentCloseOutcome {
	return result.outcome
}

// ContinuationReference 只在`未决`时非空。
func (result CloseFulfillmentSegmentResult) ContinuationReference() string {
	return result.continuation
}

type CloseFulfillmentSegmentDeps struct {
	Segments ports.ActualFulfillmentSegmentRegistry
}

type CloseFulfillmentSegmentHandler struct {
	deps CloseFulfillmentSegmentDeps
}

func NewCloseFulfillmentSegmentHandler(deps CloseFulfillmentSegmentDeps) *CloseFulfillmentSegmentHandler {
	return &CloseFulfillmentSegmentHandler{deps: deps}
}

// Close 按声明关闭一个实际履约段。
//
// **整段读回、领域判定、再走窄口。** 窄口 `CloseSegment` 的前置条件只表达得出「尚未关闭」，
// 表达不了「全部参与已结束」——那是跨行条件（`ErrSegmentStillActive`），ADR-0097 里唯一一条靠
// 纪律而非结构守的约束，端口注释要求编排先读回整段走领域再落库。本编排就是那条纪律的唯一生产
// 调用方，不得绕过领域直接调窄口。
func (handler *CloseFulfillmentSegmentHandler) Close(
	ctx context.Context,
	command CloseFulfillmentSegmentCommand,
) (CloseFulfillmentSegmentResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || command.ClosedAt.IsZero() {
		return CloseFulfillmentSegmentResult{outcome: SegmentCloseNotAccepted}, nil
	}
	segment, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil {
		return CloseFulfillmentSegmentResult{outcome: SegmentCloseNotAccepted}, nil
	}

	key := ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segment}
	record, found, err := handler.deps.Segments.FindByKey(ctx, key)
	if err != nil {
		return segmentCloseUndecided(command), nil
	}
	if !found {
		return CloseFulfillmentSegmentResult{outcome: SegmentToCloseNotFound}, nil
	}

	closed, err := record.Segment.CloseSegment(command.ClosedAt)
	switch {
	case errors.Is(err, domain.ErrSegmentClosed):
		return CloseFulfillmentSegmentResult{outcome: SegmentAlreadyClosed}, nil
	case errors.Is(err, domain.ErrSegmentStillActive):
		// 一个仍在控制中的对象足以让段继续存在；这是正当结果，登记册一动不动。
		return CloseFulfillmentSegmentResult{outcome: SegmentStillHasActiveParticipations}, nil
	case err != nil:
		return CloseFulfillmentSegmentResult{outcome: SegmentCloseNotAccepted}, nil
	}

	closedAt, _ := closed.ClosedAt()
	saved, err := handler.deps.Segments.CloseSegment(ctx, key, closedAt)
	if err != nil {
		return segmentCloseUndecided(command), nil
	}
	switch saved {
	case ports.SegmentClosed:
		return CloseFulfillmentSegmentResult{outcome: SegmentClosedNow}, nil
	case ports.SegmentAlreadyClosed:
		// 并发下另一方先关：业务答案，不是本次失败。
		return CloseFulfillmentSegmentResult{outcome: SegmentAlreadyClosed}, nil
	default:
		return segmentCloseUndecided(command), nil
	}
}

func segmentCloseUndecided(command CloseFulfillmentSegmentCommand) CloseFulfillmentSegmentResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"SEGMENT_CLOSE_UNAVAILABLE",
		command.TenantID.String(),
		command.Segment,
	}, "\x00")))
	return CloseFulfillmentSegmentResult{
		outcome:      SegmentCloseUndecided,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
