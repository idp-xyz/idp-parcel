package domain

import (
	"errors"
	"time"
)

// ErrInvalidRehydratedExecutionFact 是执行事实从持久化行重建失败的信号。
// RecordExecutionFact 不能兼职重建：它要一份承接决定才能守界，而行里只有事实本
// 身——覆盖范围是写入时已经判过的，重建门以结果字段自证完备，不逆推承接。
var ErrInvalidRehydratedExecutionFact = errors.New("node operations: rehydrated execution fact violates its invariants")

// RehydrateNodeExecutionFactSpec 是从行数据重建一份执行事实所需的全部字段。
type RehydrateNodeExecutionFactSpec struct {
	TenantID    TenantID
	Node        NodeReference
	Item        CollaborationItemReference
	Unit        HandlingUnitID
	Action      CollaborationActionKind
	Evidence    ExecutionEvidenceReference
	PerformedAt time.Time
}

// RehydrateNodeExecutionFact 验字段完备与动作封闭五值。不重走承接守界——那是
// 登记当时的判断，行里没有决定可依。
func RehydrateNodeExecutionFact(spec RehydrateNodeExecutionFactSpec) (NodeExecutionFact, error) {
	if !spec.TenantID.valid() ||
		!spec.Node.valid() ||
		!spec.Item.valid() ||
		!spec.Unit.valid() ||
		!spec.Action.valid() ||
		!spec.Evidence.valid() ||
		spec.PerformedAt.IsZero() {
		return NodeExecutionFact{}, ErrInvalidRehydratedExecutionFact
	}
	return NodeExecutionFact{
		tenantID:    spec.TenantID,
		node:        spec.Node,
		item:        spec.Item,
		unit:        spec.Unit,
		action:      spec.Action,
		evidence:    spec.Evidence,
		performedAt: spec.PerformedAt.UTC(),
	}, nil
}
