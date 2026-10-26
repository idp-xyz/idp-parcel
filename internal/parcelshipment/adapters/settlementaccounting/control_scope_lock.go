package settlementaccounting

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ControlScopeLocks 把 parcel-shipment 在保存委托之前取控制作用域锁的端口接到 settlement-accounting 的同一把锁上。
// 提供方施加与释放控制取的就是那一把，所以这里不另建锁、只译坐标：占用作用域是施加时随控制结果存下的三段引用，译成
// 提供方的结算作用域。
type ControlScopeLocks struct {
	inner saports.PreAcceptanceControlScopeLock
}

// NewControlScopeLocks 不在构造时报错，好让装配点直接包在提供方的锁外面；少接一个件在第一次取锁时答装配缺口（错误），
// 取不到锁的各路按保存委托失败的同一格处置。
func NewControlScopeLocks(inner saports.PreAcceptanceControlScopeLock) *ControlScopeLocks {
	return &ControlScopeLocks{inner: inner}
}

var _ psports.PreAcceptanceControlScopeLock = (*ControlScopeLocks)(nil)

// LockControlScope 取（租户，占用作用域）的锁，事务结束即放。身份或作用域译不成提供方的值时不取：提供方的释放在读任何账本
// 之前就以`未受理`短路（releaseAnswerFor），那一笔不会碰两本账，也就没有要排队的东西；在这里报错会把一笔本该记进释放未受理册
// 的占用变成一次撤回类命令的失败。
func (locks *ControlScopeLocks) LockControlScope(
	ctx context.Context,
	tenant psdomain.TenantID,
	scope psdomain.OccupationScope,
) error {
	if locks == nil || locks.inner == nil {
		return errors.New("parcel shipment settlementaccounting adapter: control scope lock is not assembled")
	}
	settlementTenant, err := sadomain.NewTenantID(tenant.String())
	if err != nil {
		return nil
	}
	settlement := settlementScopeFor(scope)
	if settlement == (sadomain.SettlementScope{}) {
		return nil
	}
	if err := locks.inner.LockControlScope(ctx, settlementTenant, settlement); err != nil {
		return fmt.Errorf("lock control scope: %w", err)
	}
	return nil
}
