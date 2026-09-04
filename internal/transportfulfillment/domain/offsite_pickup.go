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
	tenantID    TenantID
	object      CarriedObjectReference
	task        PickupTaskReference
	attempt     AttemptReference
	place       PickupPlaceReference
	control     TransportControlReference
	executedBy  ExecutingPartyReference
	version     PickupResultVersion
	occurredAt  time.Time
	corrects    PickupResultVersion
	correctedAt time.Time
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

// Corrects 交回本版本更正的前一版本（若本版本由更正产生）。被回指的那一版留在册上，但不再是
// 这个对象在这次尝试上的结果——「失效」在版本链里就由被回指表达，不需要一个可改写的失效位。
func (pickup OffsitePickup) Corrects() (PickupResultVersion, bool) {
	if !pickup.corrects.valid() {
		return PickupResultVersion{}, false
	}
	return pickup.corrects, true
}

func (pickup OffsitePickup) CorrectedAt() (time.Time, bool) {
	if pickup.correctedAt.IsZero() {
		return time.Time{}, false
	}
	return pickup.correctedAt, true
}

// PickupCorrection 携带一次更正给出的「证据说了什么」：地点、控制依据、执行方、发生时刻四格，
// 加新版本号与更正时刻。对象、任务、尝试不在其中——它们说的是「这是哪一次揽收」，改了它们就是
// 另一次揽收而不是更正（票 tf-segment-lifecycle-closure/08 裁决，取法同 HandoverCorrection）。
type PickupCorrection struct {
	Place       PickupPlaceReference
	Control     TransportControlReference
	ExecutedBy  ExecutingPartyReference
	OccurredAt  time.Time
	Version     PickupResultVersion
	CorrectedAt time.Time
}

// Correct 依据更正证据形成新版本：保留原事实与原结果（值语义，接收者不动），新版本回指被更正
// 版本（CONTEXT「来源证据被更正时，保留原事实和原判断，形成失效、替代及重新派生结果」）。
// 每格完备性同首登——控制依据仍必备：更正不能把一次揽收更正成一次失败到访，那是另一种事实，
// 走别的口。沿用原版本号即覆盖，构造期拒绝。
//
// 更正时刻不得早于被更正版本的**登记时刻**，而登记时刻是登记册的事实（ports.OffsitePickupRecord
// 的 RecordedAt），不在本类型上，那道先后由编排在读回前版时守；这里只守「更正时刻在场」。不拿
// OccurredAt 顶替下界：发生时刻本身是可更正的四格之一，被更正的那一版可能恰恰把它记晚了，以它
// 为下界会把一次正当的更正拒掉——交接与交付两侧能拿业务时间当下界，是因为它们的业务时间不在
// 更正范围内。
func (pickup OffsitePickup) Correct(correction PickupCorrection) (OffsitePickup, error) {
	if !pickup.version.valid() {
		return OffsitePickup{}, ErrInvalidOffsitePickup
	}
	if !correction.Version.valid() || correction.CorrectedAt.IsZero() {
		return OffsitePickup{}, ErrInvalidOffsitePickup
	}
	if correction.Version == pickup.version {
		return OffsitePickup{}, ErrInvalidOffsitePickup
	}
	corrected, err := FormOffsitePickup(OffsitePickupSpec{
		TenantID:   pickup.tenantID,
		Object:     pickup.object,
		Task:       pickup.task,
		Attempt:    pickup.attempt,
		Place:      correction.Place,
		Control:    correction.Control,
		ExecutedBy: correction.ExecutedBy,
		Version:    correction.Version,
		OccurredAt: correction.OccurredAt,
	})
	if err != nil {
		return OffsitePickup{}, err
	}
	corrected.corrects = pickup.version
	corrected.correctedAt = correction.CorrectedAt.UTC()
	return corrected, nil
}
