package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// RouteHandoffLogs 实现 ports.RouteHandoffLog。它是 UC-NR-001 步骤 2 分界重放与冲突
// 的那本册子：登记按（租户+交接关联）恰一份指纹，比对交给编排。
type RouteHandoffLogs struct {
	db *bentopg.DB
}

func NewRouteHandoffLogs(db *bentopg.DB) (*RouteHandoffLogs, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &RouteHandoffLogs{db: db}, nil
}

var _ ports.RouteHandoffLog = (*RouteHandoffLogs)(nil)

// FindDigest 取回该关联已登的指纹。没有登记只回 false——那是「这份交接头一次来」，
// 与「登记册读不到」是两回事，后者走 error 让编排停在未决而不是当成首次受理。
func (repository *RouteHandoffLogs) FindDigest(
	ctx context.Context,
	tenant domain.TenantID,
	correlation domain.RequestCorrelationID,
) (string, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return "", false, fmt.Errorf("find route handoff digest: %w", err)
	}

	var digest string
	err = querier.QueryRow(ctx,
		`SELECT digest
		   FROM network_routing.route_handoff_log
		  WHERE tenant_id = $1
		    AND correlation_id = $2`,
		tenant.String(),
		correlation.String(),
	).Scan(&digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find route handoff digest: %w", err)
	}
	return digest, true, nil
}

// Append 登记首份指纹。撞键静默不写：并发下另一方先登记时本方的指纹要么与它相同
// （同一份交接，重放），要么不同（冲突）——两种都由下一次 FindDigest 读回赢家判定，
// 而覆盖会把冲突抹成重放。因此这里没有 DO UPDATE，也不把撞键当错误上报。
func (repository *RouteHandoffLogs) Append(
	ctx context.Context,
	tenant domain.TenantID,
	correlation domain.RequestCorrelationID,
	digest string,
) error {
	if digest == "" {
		return errors.New("append route handoff digest: digest is empty")
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("append route handoff digest: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO network_routing.route_handoff_log
			(tenant_id, correlation_id, digest)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		correlation.String(),
		digest,
	)
	if err != nil {
		return fmt.Errorf("append route handoff digest: %w", err)
	}
	return nil
}
