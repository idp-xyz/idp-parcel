package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	_ ports.TransportCommissionStartHold       = (*TransportCommissions)(nil)
	_ ports.TransportCommissionStartCorrection = (*TransportCommissions)(nil)
)

// HoldCommissionStart 取（租户、委托）的事务级咨询锁，事务结束时自动放。双 int 形式同本仓其余事务级锁；第二键撞上只让两份不相干的
// 委托多等一下。不锁委托行：取消与提交的写路照旧只认自己的 WHERE，这把锁只给采用与解除排先后。
func (repository *TransportCommissions) HoldCommissionStart(ctx context.Context, key ports.TransportCommissionKey) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("hold transport commission start: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('transport_fulfillment.transport_commission.start'), hashtext($1 || chr(31) || $2))`,
		key.TenantID.String(), key.Commission.String(),
	); err != nil {
		return fmt.Errorf("hold transport commission start: %w", err)
	}
	return nil
}

// CorrectStart 把开始写成 record 上的样子（撤回则两列清空），连同落册时刻（与 Replace 同一做法，ADR-0204 越权风险点 6）。WHERE 认
// 在册开始恰是 expected 的那一行、且未取消：解除握着开始锁，这一格对不上只可能是没走这把锁的写入者，答 false 由编排报出来。
func (repository *TransportCommissions) CorrectStart(
	ctx context.Context,
	record ports.TransportCommissionRecord,
	expectedBasis domain.ParticipationBasisReference,
	expectedAt time.Time,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("correct transport commission start: %w", err)
	}
	var startedAt *time.Time
	var startedBasis *string
	if basis, at, started := record.Commission.TransportStarted(); started {
		value, moment := basis.String(), at.UTC()
		startedBasis, startedAt = &value, &moment
	}
	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.transport_commission
		    SET started_at = $3, started_basis = $4, recorded_at = $5
		  WHERE tenant_id = $1
		    AND commission_id = $2
		    AND cancelled_at IS NULL
		    AND started_at = $6
		    AND started_basis = $7`,
		record.Key.TenantID.String(),
		record.Key.Commission.String(),
		startedAt,
		startedBasis,
		record.RecordedAt.UTC(),
		expectedAt.UTC(),
		expectedBasis.String(),
	)
	if err != nil {
		return false, fmt.Errorf("correct transport commission start: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
