package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var _ ports.TransportCommissionStartAdvancement = (*TransportCommissions)(nil)

// AdvanceStart 把已开始委托的开始前移（ADR-0204 决定二）。WHERE 里的 `started_at > $3` 是并发闸：两段并发前移同一份委托时，
// READ COMMITTED 下后到的那一笔等先到者提交后按新值重判，只有更早的那一次落得上，开始只往前走；不在册、未开始、已取消与
// 「已经不晚于这一次」都答 false。与 Replace 一样只写开始两列与落册时刻，快照与成员只在 INSERT 出现。
func (repository *TransportCommissions) AdvanceStart(
	ctx context.Context,
	record ports.TransportCommissionRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("advance transport commission start: %w", err)
	}
	startedBasis, startedAt, started := record.Commission.TransportStarted()
	if !started {
		return false, fmt.Errorf("advance transport commission start: record carries no start")
	}
	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.transport_commission
		    SET started_at = $3, started_basis = $4, recorded_at = $5
		  WHERE tenant_id = $1
		    AND commission_id = $2
		    AND cancelled_at IS NULL
		    AND started_at IS NOT NULL
		    AND started_at > $3`,
		record.Key.TenantID.String(),
		record.Key.Commission.String(),
		startedAt.UTC(),
		startedBasis.String(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("advance transport commission start: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
