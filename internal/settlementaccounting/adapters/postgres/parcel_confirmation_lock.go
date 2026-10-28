package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ParcelConfirmationLocks 实现 ports.ParcelConfirmationSerializer：按（租户、包裹）取事务级咨询锁。终局那一路读「这件包裹
// 形成了哪笔费用」、形成那一路读「这件包裹终局了没有」，两路各在自己的事务里；不排队的话，两边恰好都在对方提交之前读，
// 这件费用就两头都记不上依据（ADR-0336 越权风险点 3）。锁空间名与其余事务级锁同一种双 int 形式。
type ParcelConfirmationLocks struct {
	db *bentopg.DB
}

var _ ports.ParcelConfirmationSerializer = (*ParcelConfirmationLocks)(nil)

func NewParcelConfirmationLocks(db *bentopg.DB) (*ParcelConfirmationLocks, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &ParcelConfirmationLocks{db: db}, nil
}

func (locks *ParcelConfirmationLocks) SerializeParcelConfirmation(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.ParcelReference,
) error {
	executor, err := locks.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("serialize parcel confirmation: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.charge_confirmation.parcel'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), parcel.String(),
	); err != nil {
		return fmt.Errorf("serialize parcel confirmation: 取包裹确认锁：%w", err)
	}
	return nil
}
