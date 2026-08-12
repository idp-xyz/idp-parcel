// Package domain 承载运输履约的领域模型：场外揽收、履约尝试、运输控制与交接事实。
// 它回答「由谁在什么实际范围内控制并运输哪些实物」，不拥有客户委托、路由计划、节点内
// 作业或费用结算；包裹身份属 parcel-shipment，这里只引用载运对象。
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue           = errors.New("transport fulfillment: blank value")
	ErrInvalidOffsitePickup = errors.New("transport fulfillment: invalid offsite pickup")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// CarriedObjectReference 指名一个载运对象。它是对正式包裹身份或集运单元的引用——
// 两者分别属 parcel-shipment 与 node-operations，本上下文不铸造它们。
type CarriedObjectReference struct{ requiredValue }

func NewCarriedObjectReference(value string) (CarriedObjectReference, error) {
	required, err := newRequiredValue("carried object reference", value)
	return CarriedObjectReference{required}, err
}

// PickupTaskReference 指名要求执行揽收的工作范围。任务表达需要完成什么，不等于已经
// 到场、取得控制或完成交付（CONTEXT 语言）——所以它只是揽收结果上的一个引用。
type PickupTaskReference struct{ requiredValue }

func NewPickupTaskReference(value string) (PickupTaskReference, error) {
	required, err := newRequiredValue("pickup task reference", value)
	return PickupTaskReference{required}, err
}

// AttemptReference 指名实际发生的那次到场与执行过程。改约或重派形成新尝试，不覆盖
// 旧尝试；揽收结果锚在具体一次尝试上，审计才答得出「哪次到场收的」。
type AttemptReference struct{ requiredValue }

func NewAttemptReference(value string) (AttemptReference, error) {
	required, err := newRequiredValue("attempt reference", value)
	return AttemptReference{required}, err
}

// PickupPlaceReference 指名实际接货位置。
type PickupPlaceReference struct{ requiredValue }

func NewPickupPlaceReference(value string) (PickupPlaceReference, error) {
	required, err := newRequiredValue("pickup place reference", value)
	return PickupPlaceReference{required}, err
}

// TransportControlReference 指名运输方取得控制的依据。它是场外揽收与一次失败到场的
// 分界——客户不在、货物未备好、包装不合格都没有控制依据，构造期就进不来。
type TransportControlReference struct{ requiredValue }

func NewTransportControlReference(value string) (TransportControlReference, error) {
	required, err := newRequiredValue("transport control reference", value)
	return TransportControlReference{required}, err
}

// ExecutingPartyReference 指名实际执行揽收的运输方。
type ExecutingPartyReference struct{ requiredValue }

func NewExecutingPartyReference(value string) (ExecutingPartyReference, error) {
	required, err := newRequiredValue("executing party reference", value)
	return ExecutingPartyReference{required}, err
}

// PickupResultVersion 是揽收结果的版本标识。parcel-shipment 的采用判断按它幂等；来源
// 更正形成新版本不覆盖本版。
type PickupResultVersion struct{ requiredValue }

func NewPickupResultVersion(value string) (PickupResultVersion, error) {
	required, err := newRequiredValue("pickup result version", value)
	return PickupResultVersion{required}, err
}

// OffsitePickupSpec 是形成一次场外揽收结果所需的全部输入。
type OffsitePickupSpec struct {
	TenantID   TenantID
	Object     CarriedObjectReference
	Task       PickupTaskReference
	Attempt    AttemptReference
	Place      PickupPlaceReference
	Control    TransportControlReference
	ExecutedBy ExecutingPartyReference
	Version    PickupResultVersion
	OccurredAt time.Time
}

// OffsitePickup 是对象级的场外揽收结果：明确载运对象在一次实际到场中被接收、运输方
// 取得控制（CONTEXT：「只有在明确载运对象形成有效收寄或权威交接并由运输方取得控制时，
// 才建立履约参与关系」）。控制依据必备——失败结果不制造实际履约段，分界就在这份依据上。
// 逐对象成立：整批揽收结论只能由对象级结果派生。
type OffsitePickup struct {
	tenantID   TenantID
	object     CarriedObjectReference
	task       PickupTaskReference
	attempt    AttemptReference
	place      PickupPlaceReference
	control    TransportControlReference
	executedBy ExecutingPartyReference
	version    PickupResultVersion
	occurredAt time.Time
}

func FormOffsitePickup(spec OffsitePickupSpec) (OffsitePickup, error) {
	if !spec.TenantID.valid() ||
		!spec.Object.valid() ||
		!spec.Task.valid() ||
		!spec.Attempt.valid() ||
		!spec.Place.valid() ||
		!spec.Control.valid() ||
		!spec.ExecutedBy.valid() ||
		!spec.Version.valid() ||
		spec.OccurredAt.IsZero() {
		return OffsitePickup{}, ErrInvalidOffsitePickup
	}
	return OffsitePickup{
		tenantID:   spec.TenantID,
		object:     spec.Object,
		task:       spec.Task,
		attempt:    spec.Attempt,
		place:      spec.Place,
		control:    spec.Control,
		executedBy: spec.ExecutedBy,
		version:    spec.Version,
		occurredAt: spec.OccurredAt.UTC(),
	}, nil
}

func (pickup OffsitePickup) TenantID() TenantID {
	return pickup.tenantID
}

func (pickup OffsitePickup) Object() CarriedObjectReference {
	return pickup.object
}

func (pickup OffsitePickup) Task() PickupTaskReference {
	return pickup.task
}

func (pickup OffsitePickup) Attempt() AttemptReference {
	return pickup.attempt
}

func (pickup OffsitePickup) Place() PickupPlaceReference {
	return pickup.place
}

func (pickup OffsitePickup) Control() TransportControlReference {
	return pickup.control
}

func (pickup OffsitePickup) ExecutedBy() ExecutingPartyReference {
	return pickup.executedBy
}

func (pickup OffsitePickup) Version() PickupResultVersion {
	return pickup.version
}

// OccurredAt 是实际接货发生时间——parcel-shipment 的责任起点与正式承诺生效时间最终
// 锚在它上，消息与处理时间不能替代。
func (pickup OffsitePickup) OccurredAt() time.Time {
	return pickup.occurredAt
}
