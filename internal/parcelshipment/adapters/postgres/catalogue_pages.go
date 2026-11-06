package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// queueBacklog 是带计数的待办队列那一格积压数（票 catalogue-read-pagination-rollout spec「已带计数与 truncated 的待办队列保留这两格」）：整个队列的行数，不随筛选与
// q 变。本次没有任何条件时它就是 total，不多数一次；有条件时另数一次只带固定条件的。countSQL 只收租户一个参数。
func queueBacklog(
	ctx context.Context,
	querier bentopg.Querier,
	query cataloguepage.Query,
	total int64,
	countSQL string,
	tenant domain.TenantID,
) (int, error) {
	if len(query.Filters) == 0 && query.Keyword == "" {
		return int(total), nil
	}
	var backlog int
	if err := querier.QueryRow(ctx, countSQL, tenant.String()).Scan(&backlog); err != nil {
		return 0, fmt.Errorf("count backlog: %w", err)
	}
	return backlog, nil
}
