package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// GovernanceRegisters 实现治理登记册四册的列表读端口（票 admin-skeleton-closure-batch/02
// 的三册，接管册依 ADR-0155；键形依 ADR-0083）。与写侧仓储分立成型：写侧按标识取单行、
// 供冲突预检与恢复门，这一口只做目录上列——两种消费不共口，理由与
// ports.GovernanceRegistryRead 口面注同句。接管的盘点 jsonb 不透出，装载归
// Takeovers.FindByInterval。
//
// 只读：上列不重建领域对象、不做冲突判断。语句**没有租户条件**——不是漏了 ADR-0003
// 那一句，是这些表没有租户列（产品级机制，ADR-0083 Decision 一），往语句里写租户
// 条件才是错的。恢复决定的盘点 jsonb 不透出，装载归 Resumptions.FindBySuspension。
//
// 取页按 ports 上各册的声明（ADR-0144）：总数一句、本页一句，同一组条件，不保证同一快照。
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

// 各册的落列。q 覆盖的列与各册声明头注同句；表没有租户列，取页的固定条件为空。
var (
	authorityIntervalPages = ports.AuthorityIntervalRegistryPages.MustBind(cataloguepage.Columns{
		Sorts:    map[string]string{"insertedAt": "inserted_at"},
		Identity: []string{"interval_id"},
		Keyword:  []string{"object_scope", "capability", "fact_kind", "authority"},
	})
	suspensionPages = ports.SuspensionRegistryPages.MustBind(cataloguepage.Columns{
		Sorts:    map[string]string{"occurredAt": "occurred_at"},
		Identity: []string{"suspension_id"},
		Keyword:  []string{"suspension_id", "trigger_source", "scope", "executed_by"},
	})
	resumptionPages = ports.ResumptionRegistryPages.MustBind(cataloguepage.Columns{
		Sorts:    map[string]string{"decidedAt": "decided_at"},
		Identity: []string{"suspension_id"},
		Keyword:  []string{"suspension_id", "decided_by"},
	})
	// 终点那一段必须与索引 takeover_record_by_effective 里的表达式逐字相同，规划器才认得那条索引。
	takeoverPages = ports.TakeoverRegistryPages.MustBind(cataloguepage.Columns{
		Sorts: map[string]string{"effectiveAt": "effective_at"},
		Identity: []string{
			"object_scope", "capability", "fact_kind", "authority", "from_at", "COALESCE(to_at, from_at)",
		},
		Keyword: []string{"object_scope", "capability", "fact_kind", "authority"},
	})
)

// registryListLimit 判据与各上下文目录读口同款：limit 非正是调用方编程错误，静默
// 答一页会把「忘了传」变成一个没人决定过的页大小。
func registryListLimit(operation string, limit int) error {
	if limit < 1 {
		return fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	return nil
}

// registryPage 是各册共用的取页：按 binding 拼出总数与本页两句，逐行交 scan 扫描，再切页编游标。position 只能取
// scan 扫回来的列值（cataloguepage.Finish 头注）。
func registryPage[Scanned any](
	ctx context.Context,
	db *bentopg.DB,
	binding *cataloguepage.Binding,
	query cataloguepage.Query,
	limit int,
	selection, source string,
	scan func(scan func(dest ...any) error) (Scanned, error),
	position func(row Scanned, sortField string) cataloguepage.Position,
) (cataloguepage.Result[Scanned], error) {
	plan, err := binding.Plan(query, "", nil, limit)
	if err != nil {
		return cataloguepage.Result[Scanned]{}, err
	}
	querier, err := db.ReadExecutor(ctx)
	if err != nil {
		return cataloguepage.Result[Scanned]{}, err
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(source), plan.Args...).Scan(&total); err != nil {
		return cataloguepage.Result[Scanned]{}, err
	}
	rows, err := querier.Query(ctx, plan.PageSQL(selection, source), plan.PageArgs...)
	if err != nil {
		return cataloguepage.Result[Scanned]{}, err
	}
	defer rows.Close()

	var fetched []Scanned
	for rows.Next() {
		row, err := scan(rows.Scan)
		if err != nil {
			return cataloguepage.Result[Scanned]{}, err
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return cataloguepage.Result[Scanned]{}, err
	}
	return cataloguepage.Finish(query, fetched, limit, total, position)
}

// scannedAuthorityInterval 带着代理键：它是行标识、要进游标，却不是区间的身份，不进上列行。
type scannedAuthorityInterval struct {
	intervalID int64
	row        ports.AuthorityIntervalRegistryRow
}

// ListAuthorityIntervals 上列生产权威区间册。缺省按登记时间倒序、同刻按追加序 interval_id 同向收尾。
func (registers *GovernanceRegisters) ListAuthorityIntervals(
	ctx context.Context,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.AuthorityIntervalRegistryRow], error) {
	if err := registryListLimit("list authority intervals", limit); err != nil {
		return cataloguepage.Result[ports.AuthorityIntervalRegistryRow]{}, err
	}
	page, err := registryPage(ctx, registers.db, authorityIntervalPages, query, limit,
		`interval_id, object_scope, capability, fact_kind, authority, from_at, to_at, inserted_at`,
		"pilot_governance.authority_interval",
		func(scan func(dest ...any) error) (scannedAuthorityInterval, error) {
			var scanned scannedAuthorityInterval
			var toAt *time.Time
			row := &scanned.row
			if err := scan(
				&scanned.intervalID, &row.ObjectScope, &row.Capability, &row.FactKind, &row.Authority,
				&row.FromAt, &toAt, &row.InsertedAt,
			); err != nil {
				return scannedAuthorityInterval{}, err
			}
			if toAt != nil {
				row.ToAt, row.HasToAt = *toAt, true
			}
			return scanned, nil
		},
		func(scanned scannedAuthorityInterval, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(scanned.row.InsertedAt),
				Identity: []string{cataloguepage.FormatInteger(scanned.intervalID)},
			}
		},
	)
	if err != nil {
		return cataloguepage.Result[ports.AuthorityIntervalRegistryRow]{}, fmt.Errorf("list authority intervals: %w", err)
	}
	intervals := make([]ports.AuthorityIntervalRegistryRow, 0, len(page.Rows))
	for _, scanned := range page.Rows {
		intervals = append(intervals, scanned.row)
	}
	return cataloguepage.Result[ports.AuthorityIntervalRegistryRow]{Rows: intervals, Next: page.Next, Total: page.Total}, nil
}

// ListSuspensions 上列暂停决定册。缺省按发生时刻倒序、同刻按暂停标识同向（倒序）收尾。
func (registers *GovernanceRegisters) ListSuspensions(
	ctx context.Context,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.SuspensionRegistryRow], error) {
	if err := registryListLimit("list suspensions", limit); err != nil {
		return cataloguepage.Result[ports.SuspensionRegistryRow]{}, err
	}
	page, err := registryPage(ctx, registers.db, suspensionPages, query, limit,
		`suspension_id, trigger_source, basis, evidence, scope, executed_by,
		 occurred_at, effective_at, in_transit_note`,
		"pilot_governance.suspension_decision",
		func(scan func(dest ...any) error) (ports.SuspensionRegistryRow, error) {
			var row ports.SuspensionRegistryRow
			err := scan(
				&row.SuspensionID, &row.TriggerSource, &row.Basis, &row.Evidence,
				&row.Scope, &row.ExecutedBy, &row.OccurredAt, &row.EffectiveAt,
				&row.InTransitNote,
			)
			return row, err
		},
		func(row ports.SuspensionRegistryRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(row.OccurredAt),
				Identity: []string{row.SuspensionID},
			}
		},
	)
	if err != nil {
		return cataloguepage.Result[ports.SuspensionRegistryRow]{}, fmt.Errorf("list suspensions: %w", err)
	}
	return page, nil
}

// ListResumptions 上列恢复决定册。缺省按决定时刻倒序、同刻按被恢复的暂停标识同向（倒序）收尾。
// 盘点 jsonb 不选列。
func (registers *GovernanceRegisters) ListResumptions(
	ctx context.Context,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ResumptionRegistryRow], error) {
	if err := registryListLimit("list resumptions", limit); err != nil {
		return cataloguepage.Result[ports.ResumptionRegistryRow]{}, err
	}
	page, err := registryPage(ctx, registers.db, resumptionPages, query, limit,
		`suspension_id, release_evidence, consistency_check, inventory_taken_at,
		 decided_by, decided_at, effective_at`,
		"pilot_governance.resumption_decision",
		func(scan func(dest ...any) error) (ports.ResumptionRegistryRow, error) {
			var row ports.ResumptionRegistryRow
			err := scan(
				&row.SuspensionID, &row.ReleaseEvidence, &row.ConsistencyCheck,
				&row.InventoryTakenAt, &row.DecidedBy, &row.DecidedAt, &row.EffectiveAt,
			)
			return row, err
		},
		func(row ports.ResumptionRegistryRow, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(row.DecidedAt),
				Identity: []string{row.SuspensionID},
			}
		},
	)
	if err != nil {
		return cataloguepage.Result[ports.ResumptionRegistryRow]{}, fmt.Errorf("list resumptions: %w", err)
	}
	return page, nil
}

// ListTakeovers 上列对象级接管册。缺省按生效时刻倒序，同刻按行标识同向（倒序）收尾（ports.TakeoverRegistryPages
// 头注）。盘点 jsonb 不选列。
func (registers *GovernanceRegisters) ListTakeovers(
	ctx context.Context,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.TakeoverRegistryRow], error) {
	if err := registryListLimit("list takeovers", limit); err != nil {
		return cataloguepage.Result[ports.TakeoverRegistryRow]{}, err
	}
	page, err := registryPage(ctx, registers.db, takeoverPages, query, limit,
		`object_scope, capability, fact_kind, authority, from_at, to_at,
		 stop_evidence, accepted_facts, pending_externals, actual_control,
		 responsibilities, next_action, inventory_taken_at, effective_at`,
		"pilot_governance.takeover_record",
		func(scan func(dest ...any) error) (ports.TakeoverRegistryRow, error) {
			var row ports.TakeoverRegistryRow
			var toAt *time.Time
			if err := scan(
				&row.ObjectScope, &row.Capability, &row.FactKind, &row.Authority,
				&row.FromAt, &toAt, &row.StopEvidence, &row.AcceptedFacts,
				&row.PendingExternals, &row.ActualControl, &row.Responsibilities,
				&row.NextAction, &row.InventoryTakenAt, &row.EffectiveAt,
			); err != nil {
				return ports.TakeoverRegistryRow{}, err
			}
			if toAt != nil {
				row.ToAt, row.HasToAt = *toAt, true
			}
			return row, nil
		},
		takeoverPosition,
	)
	if err != nil {
		return cataloguepage.Result[ports.TakeoverRegistryRow]{}, fmt.Errorf("list takeovers: %w", err)
	}
	return page, nil
}

// takeoverPosition 的末段是 COALESCE(to_at, from_at) 在 Go 侧的同一算法，两个值都取自扫回来的列。
func takeoverPosition(row ports.TakeoverRegistryRow, _ string) cataloguepage.Position {
	end := row.FromAt
	if row.HasToAt {
		end = row.ToAt
	}
	return cataloguepage.Position{
		Value: cataloguepage.FormatInstant(row.EffectiveAt),
		Identity: []string{
			row.ObjectScope, row.Capability, row.FactKind, row.Authority,
			cataloguepage.FormatInstant(row.FromAt), cataloguepage.FormatInstant(end),
		},
	}
}
