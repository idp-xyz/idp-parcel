package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidRehydratedCarrierPickup 是收寄重建入口因库面数据本身而拒绝时给出的理由。与
// ErrInvalidCarrierFirstEffectivePickup 分开：后者说「此刻要形成的这份不合规则」，前者说「这份已经登记过的
// 东西不可能是本上下文形成的」——处置是去查库里那一行或写它的适配器（同 ErrInvalidRehydratedDelivery）。
var ErrInvalidRehydratedCarrierPickup = errors.New("transport fulfillment: invalid rehydrated carrier first effective pickup")

// RehydrateCarrierFirstEffectivePickupSpec 是链上一版在库面的样子。字段一律当数据收下、不重算（ADR-0028）：
// 转换门的依据是调用期的事实，重放它等于拿今天的输入追认昨天的判断；这里只挡一行坏数据变成一版看起来合法的收寄。
type RehydrateCarrierFirstEffectivePickupSpec struct {
	TenantID   TenantID
	Object     CarriedObjectReference
	Fact       CarrierFirstEffectivePickupReference
	Version    CarrierFirstEffectivePickupVersion
	Result     CarrierPickupResult
	Carrier    CarrierSubject
	OccurredAt time.Time
	JudgedAt   time.Time
	Reason     PendingPickupReason
	Material   string
	Bases      []CarrierPickupBasis
	Supersedes CarrierFirstEffectivePickupVersion
}

// RehydrateCarrierFirstEffectivePickup 从库面重建一版。校验对齐三道构造门的形状判据（valid），不重走转换门。
func RehydrateCarrierFirstEffectivePickup(spec RehydrateCarrierFirstEffectivePickupSpec) (CarrierFirstEffectivePickup, error) {
	pickup := CarrierFirstEffectivePickup{
		tenantID:   spec.TenantID,
		object:     spec.Object,
		fact:       spec.Fact,
		version:    spec.Version,
		result:     spec.Result,
		carrier:    spec.Carrier,
		occurredAt: spec.OccurredAt.UTC(),
		judgedAt:   spec.JudgedAt.UTC(),
		reason:     spec.Reason,
		material:   strings.TrimSpace(spec.Material),
		bases:      append([]CarrierPickupBasis(nil), spec.Bases...),
		supersedes: spec.Supersedes,
	}
	if spec.OccurredAt.IsZero() {
		pickup.occurredAt = time.Time{}
	}
	if !pickup.valid() {
		return CarrierFirstEffectivePickup{}, fmt.Errorf("%w：结果 %q 与承运主体 / 业务时间 / 原因 / 依据 / 回指的搭配不是任何构造门产得出的",
			ErrInvalidRehydratedCarrierPickup, spec.Result.String())
	}
	return pickup, nil
}
