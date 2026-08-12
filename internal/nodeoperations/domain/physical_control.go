package domain

import (
	"errors"
	"time"
)

var ErrInvalidPhysicalControl = errors.New("node operations: invalid physical control")

// ControlEstablishmentKind 是节点控制成立的封闭二来源（CONTEXT「节点控制」节）：客户
// 直接送站形成有效节点收寄，或 transport-fulfillment 的权威交接结果确认控制已经转入。
// 刻意没有第三格——扫描、卸载、发现实物都立不起控制。
type ControlEstablishmentKind uint8

const (
	ControlEstablishmentKindInvalid ControlEstablishmentKind = iota
	EstablishedByNodeIntake
	EstablishedByHandoverIn
)

func (kind ControlEstablishmentKind) valid() bool {
	return kind == EstablishedByNodeIntake || kind == EstablishedByHandoverIn
}

func (kind ControlEstablishmentKind) String() string {
	switch kind {
	case EstablishedByNodeIntake:
		return "NODE_INTAKE"
	case EstablishedByHandoverIn:
		return "HANDOVER_IN"
	default:
		return ""
	}
}

// ControlBasisReference 指名控制成立的依据：有效节点收寄结果或权威交接结果。
type ControlBasisReference struct{ requiredValue }

func NewControlBasisReference(value string) (ControlBasisReference, error) {
	required, err := newRequiredValue("control basis reference", value)
	return ControlBasisReference{required}, err
}

// TransferOutReference 指名 transport-fulfillment 针对明确对象形成的权威运输交接
// `已交接`结果。它是控制转出的唯一依据类型——已拒收或待确认不转出控制，备货、装载
// 或节点侧交出扫描本身也不结束控制：那些结果在 TF 的权威交接对象上就不是`已交接`，
// 从源头就构造不出这个引用。
type TransferOutReference struct{ requiredValue }

func NewTransferOutReference(value string) (TransferOutReference, error) {
	required, err := newRequiredValue("transfer out reference", value)
	return TransferOutReference{required}, err
}

// PhysicalControlSpec 是控制成立所需的全部输入。
type PhysicalControlSpec struct {
	TenantID      TenantID
	Unit          HandlingUnitID
	Node          NodeReference
	Kind          ControlEstablishmentKind
	Basis         ControlBasisReference
	EstablishedAt time.Time
}

// PhysicalControl 是明确节点对单件作业实物的保管与后续作业责任关系（CONTEXT：实物
// 控制不是所有权、库存所有权、客户责任或仅由位置推断的状态）。逐对象成立与结束——
// 部分交接时各对象分别转出或保留，批次、集运单元或车辆范围不形成一刀切的控制结果，
// 所以这个类型上根本没有批次维度。
type PhysicalControl struct {
	tenantID      TenantID
	unit          HandlingUnitID
	node          NodeReference
	kind          ControlEstablishmentKind
	basis         ControlBasisReference
	establishedAt time.Time
	releasedBy    TransferOutReference
	releasedAt    time.Time
}

// EstablishPhysicalControl 按两来源之一成立控制。
func EstablishPhysicalControl(spec PhysicalControlSpec) (PhysicalControl, error) {
	if !spec.TenantID.valid() ||
		!spec.Unit.valid() ||
		!spec.Node.valid() ||
		!spec.Kind.valid() ||
		!spec.Basis.valid() ||
		spec.EstablishedAt.IsZero() {
		return PhysicalControl{}, ErrInvalidPhysicalControl
	}
	return PhysicalControl{
		tenantID:      spec.TenantID,
		unit:          spec.Unit,
		node:          spec.Node,
		kind:          spec.Kind,
		basis:         spec.Basis,
		establishedAt: spec.EstablishedAt.UTC(),
	}, nil
}

func (control PhysicalControl) TenantID() TenantID {
	return control.tenantID
}

func (control PhysicalControl) Unit() HandlingUnitID {
	return control.unit
}

func (control PhysicalControl) Node() NodeReference {
	return control.node
}

func (control PhysicalControl) EstablishmentKind() ControlEstablishmentKind {
	return control.kind
}

func (control PhysicalControl) Basis() ControlBasisReference {
	return control.basis
}

func (control PhysicalControl) EstablishedAt() time.Time {
	return control.establishedAt
}

// Active 报告控制是否仍然在身。控制期间位置、测量、分拣、集拆、封装各自成事实——
// 它们都不出现在这个类型上，任何单项作业事实都不重新定义控制起点，也不结束控制。
func (control PhysicalControl) Active() bool {
	return control.releasedAt.IsZero()
}

// Release 报告转出依据与时刻，只在已转出的控制上给出。
func (control PhysicalControl) Release() (TransferOutReference, time.Time, bool) {
	return control.releasedBy, control.releasedAt, !control.releasedAt.IsZero()
}

// TransferOut 依据权威运输交接的`已交接`结果转出控制。转出不可逆：没有有效交付、
// 权威交接或明确控制终止依据时，控制责任不得因任何单项作业事实而消失——反过来，
// 已转出的控制也不会因为后续扫描回来。转出时刻不得早于成立时刻。
func (control PhysicalControl) TransferOut(
	handover TransferOutReference,
	at time.Time,
) (PhysicalControl, error) {
	if _, _, released := control.Release(); released {
		return PhysicalControl{}, ErrInvalidPhysicalControl
	}
	if !handover.valid() || at.IsZero() || at.Before(control.establishedAt) {
		return PhysicalControl{}, ErrInvalidPhysicalControl
	}
	transferred := control
	transferred.releasedBy = handover
	transferred.releasedAt = at.UTC()
	return transferred, nil
}
