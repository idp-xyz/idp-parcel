package domain

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidRehydratedPickup 是揽收重建入口因快照数据本身而拒绝时给出的理由。与
// ErrInvalidOffsitePickup 分格的道理同交付与交接两侧：后者说「此刻要形成的这份不合规则」，
// 前者说「这份已经登记过的东西不可能是本上下文形成的」——处置是去查库里那一行或写它的
// 适配器，不是改调用方的入参。
var ErrInvalidRehydratedPickup = errors.New("transport fulfillment: invalid rehydrated offsite pickup")

// RehydrateOffsitePickupSpec 是一份对象级揽收在库里的样子。版本链两字段独立收下，因为
// Correct 把它们写在未导出字段上——没有这个入口，一份更正版本读回来会退化成首登，「新版
// 回指前身」的链在重启后就断了。
type RehydrateOffsitePickupSpec struct {
	TenantID    TenantID
	Object      CarriedObjectReference
	Task        PickupTaskReference
	Attempt     AttemptReference
	Place       PickupPlaceReference
	Control     TransportControlReference
	ExecutedBy  ExecutingPartyReference
	Version     PickupResultVersion
	OccurredAt  time.Time
	Corrects    PickupResultVersion
	CorrectedAt time.Time
}

// RehydrateOffsitePickup 从库里读到的产物重建一份对象级揽收。
//
// 完备性与 FormOffsitePickup 同一套：控制依据等七件加时间缺一不可——控制依据缺席的一行在这里
// 暴露，而不是变成一份看起来合法的有效收寄。版本链要么整体缺席（首登），要么回指前版且不自指
// ——与 Correct 立的门一一对应。更正时刻与被更正版本登记时刻的先后不在这里复验：那是跨行的
// 事实，重建门只看得见自己这一行。
func RehydrateOffsitePickup(spec RehydrateOffsitePickupSpec) (OffsitePickup, error) {
	if !spec.TenantID.valid() || !spec.Object.valid() || !spec.Task.valid() || !spec.Attempt.valid() ||
		!spec.Place.valid() || !spec.ExecutedBy.valid() || !spec.Version.valid() || spec.OccurredAt.IsZero() {
		return OffsitePickup{}, rehydratedPickupRefusal("揽收身份、任务、尝试、地点、执行方、版本或业务时间缺失")
	}
	if !spec.Control.valid() {
		return OffsitePickup{}, rehydratedPickupRefusal("控制依据缺失——没有控制依据的到场不是揽收")
	}
	if spec.Corrects.valid() != !spec.CorrectedAt.IsZero() {
		return OffsitePickup{}, rehydratedPickupRefusal("版本链半截——前版引用与更正时间必须同缺席或同在场")
	}
	if spec.Corrects.valid() && spec.Corrects == spec.Version {
		return OffsitePickup{}, rehydratedPickupRefusal("前版引用指向版本自己")
	}
	return OffsitePickup{
		tenantID:    spec.TenantID,
		object:      spec.Object,
		task:        spec.Task,
		attempt:     spec.Attempt,
		place:       spec.Place,
		control:     spec.Control,
		executedBy:  spec.ExecutedBy,
		version:     spec.Version,
		occurredAt:  spec.OccurredAt.UTC(),
		corrects:    spec.Corrects,
		correctedAt: spec.CorrectedAt.UTC(),
	}, nil
}

func rehydratedPickupRefusal(reason string) error {
	return fmt.Errorf("%w：%s", ErrInvalidRehydratedPickup, reason)
}
