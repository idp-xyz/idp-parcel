package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ControlReleaseRefusalCatalogue 实现 ports.ControlReleaseRefusalCatalogue：运营查阅按租户列释放未受理册上还没被处理的行（ADR-0293
// 决定一）。「未处理」是册行没有对应的处理留痕，不是原行上的状态列；租户条件进每条语句（ADR-0003）。
type ControlReleaseRefusalCatalogue struct {
	db *bentopg.DB
}

func NewControlReleaseRefusalCatalogue(db *bentopg.DB) (*ControlReleaseRefusalCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ControlReleaseRefusalCatalogue{db: db}, nil
}

var _ ports.ControlReleaseRefusalCatalogue = (*ControlReleaseRefusalCatalogue)(nil)

const unhandledControlReleaseRefusalFilter = `
		  FROM parcel_shipment.control_release_refusal r
		 WHERE r.tenant_id = $1
		   AND NOT EXISTS (
		       SELECT 1
		         FROM parcel_shipment.control_release_refusal_handling h
		        WHERE h.tenant_id = r.tenant_id
		          AND h.shipment_request_id = r.shipment_request_id
		          AND h.submission_version = r.submission_version
		          AND h.control_result_id = r.control_result_id)`

// ListUnhandledControlReleaseRefusals 上列本租户册上未处理的行，最早登记的在前、同刻按委托与版本收尾，至多 limit 行；Count 是未处理的
// 全部行数，不受 limit 截断。两句不在同一快照里：计数与清单是此刻的大概，处理动作的成败由处理口自己的主键判。
func (catalogue *ControlReleaseRefusalCatalogue) ListUnhandledControlReleaseRefusals(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) (ports.UnhandledControlReleaseRefusals, error) {
	if limit <= 0 {
		return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: limit %d is not a positive page size", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: %w", err)
	}
	var count int
	if err := querier.QueryRow(ctx, `SELECT count(*)`+unhandledControlReleaseRefusalFilter, tenant.String()).Scan(&count); err != nil {
		return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: count: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT r.shipment_request_id, r.submission_version, r.control_result_id, r.customer_account_id,
		        r.occupation_legal_entity, r.occupation_settlement_account, r.occupation_currency,
		        r.reason_ref, r.recorded_at`+unhandledControlReleaseRefusalFilter+`
		 ORDER BY r.recorded_at, r.shipment_request_id, r.submission_version, r.control_result_id
		 LIMIT $2`,
		tenant.String(), limit)
	if err != nil {
		return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: %w", err)
	}
	defer rows.Close()

	listed := ports.UnhandledControlReleaseRefusals{Refusals: []ports.ControlReleaseRefusal{}, Count: count}
	for rows.Next() {
		var (
			requestRaw, version, resultID, account, reason string
			scope                                          occupationScopeRow
			recordedAt                                     time.Time
		)
		if err := rows.Scan(&requestRaw, &version, &resultID, &account,
			&scope.legalEntity, &scope.account, &scope.currency, &reason, &recordedAt); err != nil {
			return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: %w", err)
		}
		requestID, err := domain.NewShipmentRequestID(requestRaw)
		if err != nil {
			return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: %w", err)
		}
		refusal, err := controlReleaseRefusal(tenant, requestID, version, resultID, account, scope, reason, recordedAt)
		if err != nil {
			return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: %w", err)
		}
		listed.Refusals = append(listed.Refusals, refusal)
	}
	if err := rows.Err(); err != nil {
		return ports.UnhandledControlReleaseRefusals{}, fmt.Errorf("list unhandled control release refusals: %w", err)
	}
	return listed, nil
}
