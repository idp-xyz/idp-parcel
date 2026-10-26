package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CancellationBoundaryLocks 实现 ports.CancellationBoundaryLock。
type CancellationBoundaryLocks struct {
	db *bentopg.DB
}

func NewCancellationBoundaryLocks(db *bentopg.DB) (*CancellationBoundaryLocks, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &CancellationBoundaryLocks{db: db}, nil
}

var _ ports.CancellationBoundaryLock = (*CancellationBoundaryLocks)(nil)

// LockCancellationBoundary 取（租户，包裹）的事务级咨询锁，事务结束即放。走 RequireExecutor：在连接池上取，锁随那条语句的
// 自动提交当场放掉，挡不住任何一路。不对哪一行 FOR UPDATE：三处争的都是先手提交之前还不在册的那一行（取消、责任起点、
// 终局），没有行可锁。
//
// 键照本仓其余事务级锁的双 int 形式：第一键是锁空间名的散列，第二键是租户与包裹以 chr(31) 拼接后的散列；撞上只让两件
// 不相干的包裹多等一会儿。同一事务里再取一次是重入。
func (locks *CancellationBoundaryLocks) LockCancellationBoundary(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) error {
	executor, err := locks.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock cancellation boundary: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('parcel_shipment.parcel.cancellation_boundary'),
		        hashtext($1 || chr(31) || $2))`,
		tenant.String(),
		parcel.String(),
	); err != nil {
		return fmt.Errorf("lock cancellation boundary: %w", err)
	}
	return nil
}
