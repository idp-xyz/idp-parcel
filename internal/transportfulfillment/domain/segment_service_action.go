package domain

import (
	"errors"
	"fmt"
)

// ErrServiceActionAlreadyDeclared：段服务动作在段成立时固定，之后不改（CONTEXT 生命周期①末句）。再声明
// 一次不是更正——声明错了是登记方另立新段的事，段登记册没有通用 Update（ADR-0097）。
var ErrServiceActionAlreadyDeclared = errors.New("transport fulfillment: the segment's service action is already declared")

// SegmentServiceAction 是控制事实登记方在实际履约段成立时对该段显式声明的运输动作（CONTEXT「段服务动作」），
// 封闭三格取 CONTEXT 首段列的运输动作：场外揽收、节点间运输、末端派送。由登记方声明，本上下文不从计划履约段
// 在路由里的位置、实际承运商或交接范围推导（ADR-0096 同判据）；声明为末端派送的段即派送段，末端派送任务的
// 内部触发只认它（ADR-0114）。
type SegmentServiceAction uint8

const (
	// SegmentServiceActionUndeclared 是未声明：一种答案不是缺陷——这样的段照常成立与结束，只是不触发依赖
	// 服务动作的派生。
	SegmentServiceActionUndeclared SegmentServiceAction = iota
	SegmentServesOffsitePickup
	SegmentServesLinehaul
	SegmentServesFinalDelivery
)

func (action SegmentServiceAction) String() string {
	switch action {
	case SegmentServesOffsitePickup:
		return "OFFSITE_PICKUP"
	case SegmentServesLinehaul:
		return "LINEHAUL"
	case SegmentServesFinalDelivery:
		return "FINAL_DELIVERY"
	default:
		return ""
	}
}

// Declared 报告这是不是三格之一（零值是未声明）。
func (action SegmentServiceAction) Declared() bool {
	return action >= SegmentServesOffsitePickup && action <= SegmentServesFinalDelivery
}

// ParseSegmentServiceAction 把库面或登记输入里的动作词认回封闭集合；词不在集合内即拒，不猜。空串不是
// 一个动作词——未声明由调用方按缺席处理，不经这里。
func ParseSegmentServiceAction(raw string) (SegmentServiceAction, error) {
	for action := SegmentServesOffsitePickup; action <= SegmentServesFinalDelivery; action++ {
		if action.String() == raw {
			return action, nil
		}
	}
	return SegmentServiceActionUndeclared, fmt.Errorf("%w: unknown segment service action %q", ErrInvalidFulfillmentSegment, raw)
}

// ServiceAction 交回段成立时声明的服务动作；未声明第二个返回值为 false。
func (segment ActualFulfillmentSegment) ServiceAction() (SegmentServiceAction, bool) {
	return segment.serviceAction, segment.serviceAction.Declared()
}

// IsDeliverySegment 答这是不是派送段：声明为末端派送。未声明与其他两格都答否。
func (segment ActualFulfillmentSegment) IsDeliverySegment() bool {
	return segment.serviceAction == SegmentServesFinalDelivery
}

// DeclareServiceAction 是声明的唯一一扇门，只在段成立那一刻开：段已声明过即拒；未成立、已关闭的段没有
// 「成立那一刻」可言，也拒。动作不在封闭集合内拒。值语义：交回带声明的新值，原值不动。
func (segment ActualFulfillmentSegment) DeclareServiceAction(action SegmentServiceAction) (ActualFulfillmentSegment, error) {
	if !action.Declared() {
		return ActualFulfillmentSegment{}, fmt.Errorf("%w: segment service action", ErrInvalidFulfillmentSegment)
	}
	if !segment.Established() || segment.closed {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	if segment.serviceAction.Declared() {
		return ActualFulfillmentSegment{}, ErrServiceActionAlreadyDeclared
	}
	declared := segment
	declared.serviceAction = action
	return declared, nil
}
