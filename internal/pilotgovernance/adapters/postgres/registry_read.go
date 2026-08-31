package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// GovernanceRegisters 实现治理登记册三册的列表读端口（票 admin-skeleton-closure-batch/02，
// 键形依 ADR-0083）。与写侧仓储（AuthorityIntervals / Suspensions / Resumptions）分立
// 成型：那三口按标识取单行、供冲突预检与恢复门，这一口只做目录上列——两种消费不共口，
// 理由与 ports.GovernanceRegistryRead 口面注同句。
//
// 只读：上列不重建领域对象、不做冲突判断。语句**没有租户条件**——不是漏了 ADR-0003
// 那一句，是这些表没有租户列（产品级机制，ADR-0083 Decision 一），往语句里写租户
// 条件才是错的。恢复决定的盘点 jsonb 不透出，装载归 Resumptions.FindBySuspension。
type GovernanceRegisters struct {
	db *bentopg.DB
}

func NewGovernanceRegisters(db *bentopg.DB) (*GovernanceRegisters, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &GovernanceRegisters{db: db}, nil
}

var _ ports.GovernanceRegistryRead = (*GovernanceRegisters)(nil)

// registryListLimit 判据与各上下文目录读口同款：limit 非正是调用方编程错误，静默
// 答一页会把「忘了传」变成一个没人决定过的页大小。
func registryListLimit(operation string, limit int) error {
	if limit < 1 {
		return fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	return nil
}

// ListAuthorityIntervals 上列生产权威区间册。排序以登记时间倒序、同刻按追加序倒序
// 收尾（interval_id 只参与排序不透出），保证分页可重复。
func (registers *GovernanceRegisters) ListAuthorityIntervals(
	ctx context.Context,
	limit int,
) ([]ports.AuthorityIntervalRegistryRow, error) {
	if err := registryListLimit("list authority intervals", limit); err != nil {
		return nil, err
	}
	querier, err := registers.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list authority intervals: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_scope, capability, fact_kind, authority, from_at, to_at, inserted_at
		   FROM pilot_governance.authority_interval
		  ORDER BY inserted_at DESC, interval_id DESC
		  LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list authority intervals: %w", err)
	}
	defer rows.Close()

	intervals := make([]ports.AuthorityIntervalRegistryRow, 0, limit)
	for rows.Next() {
		var row ports.AuthorityIntervalRegistryRow
		var toAt *time.Time
		if err := rows.Scan(
			&row.ObjectScope, &row.Capability, &row.FactKind, &row.Authority,
			&row.FromAt, &toAt, &row.InsertedAt,
		); err != nil {
			return nil, fmt.Errorf("list authority intervals: %w", err)
		}
		if toAt != nil {
			row.ToAt, row.HasToAt = *toAt, true
		}
		intervals = append(intervals, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list authority intervals: %w", err)
	}
	return intervals, nil
}

// ListSuspensions 上列暂停决定册。排序以发生时刻倒序、同刻按暂停标识正序收尾。
func (registers *GovernanceRegisters) ListSuspensions(
	ctx context.Context,
	limit int,
) ([]ports.SuspensionRegistryRow, error) {
	if err := registryListLimit("list suspensions", limit); err != nil {
		return nil, err
	}
	querier, err := registers.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list suspensions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT suspension_id, trigger_source, basis, evidence, scope, executed_by,
		        occurred_at, effective_at, in_transit_note
		   FROM pilot_governance.suspension_decision
		  ORDER BY occurred_at DESC, suspension_id
		  LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list suspensions: %w", err)
	}
	defer rows.Close()

	suspensions := make([]ports.SuspensionRegistryRow, 0, limit)
	for rows.Next() {
		var row ports.SuspensionRegistryRow
		if err := rows.Scan(
			&row.SuspensionID, &row.TriggerSource, &row.Basis, &row.Evidence,
			&row.Scope, &row.ExecutedBy, &row.OccurredAt, &row.EffectiveAt,
			&row.InTransitNote,
		); err != nil {
			return nil, fmt.Errorf("list suspensions: %w", err)
		}
		suspensions = append(suspensions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list suspensions: %w", err)
	}
	return suspensions, nil
}

// ListResumptions 上列恢复决定册。排序以决定时刻倒序、同刻按被恢复的暂停标识正序
// 收尾；盘点 jsonb 不选列。
func (registers *GovernanceRegisters) ListResumptions(
	ctx context.Context,
	limit int,
) ([]ports.ResumptionRegistryRow, error) {
	if err := registryListLimit("list resumptions", limit); err != nil {
		return nil, err
	}
	querier, err := registers.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list resumptions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT suspension_id, release_evidence, consistency_check, inventory_taken_at,
		        decided_by, decided_at, effective_at
		   FROM pilot_governance.resumption_decision
		  ORDER BY decided_at DESC, suspension_id
		  LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list resumptions: %w", err)
	}
	defer rows.Close()

	resumptions := make([]ports.ResumptionRegistryRow, 0, limit)
	for rows.Next() {
		var row ports.ResumptionRegistryRow
		if err := rows.Scan(
			&row.SuspensionID, &row.ReleaseEvidence, &row.ConsistencyCheck,
			&row.InventoryTakenAt, &row.DecidedBy, &row.DecidedAt, &row.EffectiveAt,
		); err != nil {
			return nil, fmt.Errorf("list resumptions: %w", err)
		}
		resumptions = append(resumptions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list resumptions: %w", err)
	}
	return resumptions, nil
}
