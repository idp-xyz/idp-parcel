package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ReviewCatalogue 实现 ports.ReviewCatalogueRead：运输履约查阅页的列表读面
// （ADR-0077，票 admin-skeleton-closure-batch/05）。
//
// 目录上列是照实转写，不经领域重建门：重建是写路与按键读回的纪律（坏行要在那里
// 响亮），检索列面把登记的字段原样透出。容量四量在 SQL 里按预占子表求和——求和的
// 三个口径（预占量、已释放、实际使用）逐维独立，读口不互相抵扣，也不代算可用量。
//
// 所有语句显式携带租户条件（作用域不是过滤器而是身份的一部分）；读走 ReadExecutor，
// 事务外用显式注入的连接池。
type ReviewCatalogue struct {
	db *bentopg.DB
}

var _ ports.ReviewCatalogueRead = (*ReviewCatalogue)(nil)

func NewReviewCatalogue(db *bentopg.DB) (*ReviewCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ReviewCatalogue{db: db}, nil
}

// ListTransportSchedules 上列班次登记册：晚出发的在前，同刻按班次标识稳定排序。
func (catalogue *ReviewCatalogue) ListTransportSchedules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.TransportScheduleCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list transport schedules: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transport schedules: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT schedule_id, direction, departs_at, recorded_at
		   FROM transport_fulfillment.transport_schedule
		  WHERE tenant_id = $1
		  ORDER BY departs_at DESC, schedule_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list transport schedules: %w", err)
	}
	defer rows.Close()

	list := make([]ports.TransportScheduleCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.TransportScheduleCatalogueRow
		if err := rows.Scan(&row.ScheduleID, &row.Direction, &row.DepartsAt, &row.RecordedAt); err != nil {
			return nil, fmt.Errorf("list transport schedules: %w", err)
		}
		row.DepartsAt = row.DepartsAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list transport schedules: %w", err)
	}
	return list, nil
}

// ListCapacityPools 上列容量池登记册：三个和按预占子表求和（bigint 求和在 PG 里是
// numeric，落回 bigint 由 SQL 显式转换），无预占的池三量为零。行上没有业务时刻可
// 排序（登记时刻是提交簿记），按池标识稳定排序。
func (catalogue *ReviewCatalogue) ListCapacityPools(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CapacityPoolCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list capacity pools: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list capacity pools: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT p.pool_id, p.schedule_id, p.unit_ref, p.capacity,
		        COALESCE(SUM(r.quantity), 0)::bigint,
		        COALESCE(SUM(r.released), 0)::bigint,
		        COALESCE(SUM(r.consumed), 0)::bigint,
		        p.recorded_at
		   FROM transport_fulfillment.capacity_pool p
		   LEFT JOIN transport_fulfillment.capacity_reservation r
		     ON r.tenant_id = p.tenant_id
		    AND r.pool_id = p.pool_id
		  WHERE p.tenant_id = $1
		  GROUP BY p.pool_id, p.schedule_id, p.unit_ref, p.capacity, p.recorded_at
		  ORDER BY p.pool_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list capacity pools: %w", err)
	}
	defer rows.Close()

	list := make([]ports.CapacityPoolCatalogueRow, 0, limit)
	for rows.Next() {
		var row ports.CapacityPoolCatalogueRow
		if err := rows.Scan(
			&row.PoolID, &row.Schedule, &row.Unit, &row.Capacity,
			&row.Reserved, &row.Released, &row.Consumed, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list capacity pools: %w", err)
		}
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list capacity pools: %w", err)
	}
	return list, nil
}

// ListTransportHandovers 上列权威交接判断登记册：一行一版本、版本链完整可见，新近
// 裁决在前，同刻按（对象，范围，版本）稳定排序。
func (catalogue *ReviewCatalogue) ListTransportHandovers(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.TransportHandoverCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list transport handovers: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list transport handovers: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_ref, scope_ref, handover_version,
		        released_by, received_by, verdict,
		        COALESCE(basis_ref, ''),
		        COALESCE(corrects_version, ''),
		        corrected_at, judged_at, recorded_at
		   FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = $1
		  ORDER BY judged_at DESC, object_ref, scope_ref, handover_version
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list transport handovers: %w", err)
	}
	defer rows.Close()

	list := make([]ports.TransportHandoverCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row         ports.TransportHandoverCatalogueRow
			correctedAt *time.Time
		)
		if err := rows.Scan(
			&row.Object, &row.Scope, &row.Version,
			&row.ReleasedBy, &row.ReceivedBy, &row.Verdict,
			&row.Basis, &row.CorrectsVersion,
			&correctedAt, &row.JudgedAt, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list transport handovers: %w", err)
		}
		if correctedAt != nil {
			utc := correctedAt.UTC()
			row.CorrectedAt = &utc
		}
		row.JudgedAt = row.JudgedAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list transport handovers: %w", err)
	}
	return list, nil
}

// ListEffectiveDeliveries 上列有效交付结果册：只列当前版（is_current），新近交付在
// 前，同刻按（对象，尝试）稳定排序。历史版本按键与版本走详情读口，不在列面展开。
func (catalogue *ReviewCatalogue) ListEffectiveDeliveries(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.EffectiveDeliveryCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list effective deliveries: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list effective deliveries: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_ref, attempt_ref, delivery_version,
		        place_ref, method_ref, recipient_ref, proof_ref,
		        COALESCE(corrects_version, ''),
		        corrected_at, occurred_at, recorded_at
		   FROM transport_fulfillment.effective_delivery
		  WHERE tenant_id = $1
		    AND is_current
		  ORDER BY occurred_at DESC, object_ref, attempt_ref
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list effective deliveries: %w", err)
	}
	defer rows.Close()

	list := make([]ports.EffectiveDeliveryCatalogueRow, 0, limit)
	for rows.Next() {
		var (
			row         ports.EffectiveDeliveryCatalogueRow
			correctedAt *time.Time
		)
		if err := rows.Scan(
			&row.Object, &row.Attempt, &row.Version,
			&row.Place, &row.Method, &row.Recipient, &row.Proof,
			&row.CorrectsVersion,
			&correctedAt, &row.OccurredAt, &row.RecordedAt,
		); err != nil {
			return nil, fmt.Errorf("list effective deliveries: %w", err)
		}
		if correctedAt != nil {
			utc := correctedAt.UTC()
			row.CorrectedAt = &utc
		}
		row.OccurredAt = row.OccurredAt.UTC()
		row.RecordedAt = row.RecordedAt.UTC()
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list effective deliveries: %w", err)
	}
	return list, nil
}
