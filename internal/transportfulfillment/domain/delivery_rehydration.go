package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedDelivery 是交付重建入口因快照数据本身而拒绝时给出的理由。与
// ErrInvalidEffectiveDelivery 分开：后者说「此刻要形成的这份不合规则」，前者说「这份
// 已经登记过的东西不可能是本上下文形成的」——处置是去查库里那一行或写它的适配器
// （与 parcel-shipment / settlement-accounting 的重建哨兵同一条分格纪律）。
var ErrInvalidRehydratedDelivery = errors.New("transport fulfillment: invalid rehydrated effective delivery")

// RehydrateEffectiveDeliverySpec 是一份交付生效在库里的样子。字段一律当数据收下，
// 不重算（ADR-0028 同款）：妥投判断在 FormEffectiveDelivery 那道门，这里只挡一行
// 坏数据变成一份看起来合法的交付。
type RehydrateEffectiveDeliverySpec struct {
	TenantID    TenantID
	Object      CarriedObjectReference
	Attempt     AttemptReference
	Place       AttemptPlaceReference
	Method      DeliveryMethodReference
	Recipient   ReceivingPartyReference
	Proof       DeliveryProofReference
	Version     DeliveryResultVersion
	OccurredAt  time.Time
	Corrects    DeliveryResultVersion
	CorrectedAt time.Time
}

// RehydrateEffectiveDelivery 从库里读到的产物重建一份交付生效。
//
// 校验对齐构造与更正两道门的不变量：POD/方式/接收方缺一不可（没有符合当时规则的
// 交付证明就没有生效交付）；版本链要么整体缺席（首登），要么回指前版且不自指、更正
// 时间不早于交付。
func RehydrateEffectiveDelivery(spec RehydrateEffectiveDeliverySpec) (EffectiveDelivery, error) {
	if !spec.TenantID.valid() || !spec.Object.valid() || !spec.Attempt.valid() ||
		!spec.Place.valid() || !spec.Version.valid() || spec.OccurredAt.IsZero() {
		return EffectiveDelivery{}, rehydratedDeliveryRefusal("交付身份、地点、版本或业务时间缺失")
	}
	if !spec.Method.valid() || !spec.Recipient.valid() || !spec.Proof.valid() {
		return EffectiveDelivery{}, rehydratedDeliveryRefusal("POD、交付方式或接收方缺失——没有交付证明就没有生效交付")
	}
	if spec.Corrects.valid() != !spec.CorrectedAt.IsZero() {
		return EffectiveDelivery{}, rehydratedDeliveryRefusal("版本链半截——前版引用与更正时间必须同缺席或同在场")
	}
	if spec.Corrects.valid() {
		if spec.Corrects == spec.Version {
			return EffectiveDelivery{}, rehydratedDeliveryRefusal("前版引用指向版本自己")
		}
		if spec.CorrectedAt.Before(spec.OccurredAt) {
			return EffectiveDelivery{}, rehydratedDeliveryRefusal("更正时间早于交付")
		}
	}
	return EffectiveDelivery{
		tenantID:    spec.TenantID,
		object:      spec.Object,
		attempt:     spec.Attempt,
		place:       spec.Place,
		method:      spec.Method,
		recipient:   spec.Recipient,
		proof:       spec.Proof,
		version:     spec.Version,
		occurredAt:  spec.OccurredAt.UTC(),
		corrects:    spec.Corrects,
		correctedAt: spec.CorrectedAt.UTC(),
	}, nil
}

func rehydratedDeliveryRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedDelivery, reason)
}
