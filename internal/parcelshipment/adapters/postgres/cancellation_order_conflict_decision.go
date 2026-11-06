package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// CancellationOrderConflictDecisions 实现 ports.CancellationOrderConflictDecisions：取消先后冲突的决定留痕（ADR-0295）。只追加，主键是冲突行的
// 主键，一份冲突至多被决定一次。
type CancellationOrderConflictDecisions struct {
	db *bentopg.DB
}

func NewCancellationOrderConflictDecisions(db *bentopg.DB) (*CancellationOrderConflictDecisions, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &CancellationOrderConflictDecisions{db: db}, nil
}

var _ ports.CancellationOrderConflictDecisions = (*CancellationOrderConflictDecisions)(nil)

// DecideCancellationOrderConflict 追加一次决定。要环境事务：决定留痕、作废与续办信封同笔落地。
//
// 一句 INSERT … SELECT 连冲突行一并核对：租户条件和冲突存在性在同一句里，册上没有这份冲突（或属于另一个租户）时一行都插不进去，与「已决定」在
// 0 行受影响上相同，再读一次留痕分开两者。两人同时决定同一份冲突时，后到的那句在主键上等先到的事务结束：先到的提交，后到的 DO NOTHING 并读回
// 赢家；先到的回滚，后到的接着插。
func (decisions *CancellationOrderConflictDecisions) DecideCancellationOrderConflict(
	ctx context.Context,
	decision ports.CancellationOrderConflictDecision,
) (ports.CancellationOrderConflictDecisionSaveOutcome, ports.CancellationOrderConflictDecision, error) {
	executor, err := decisions.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CancellationOrderConflictDecisionSaveOutcomeInvalid, ports.CancellationOrderConflictDecision{},
			fmt.Errorf("decide cancellation order conflict: %w", err)
	}
	if decision.Kind.String() == "" {
		return ports.CancellationOrderConflictDecisionSaveOutcomeInvalid, ports.CancellationOrderConflictDecision{},
			fmt.Errorf("decide cancellation order conflict: decision kind is not a kind")
	}
	key := decision.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.cancellation_order_conflict_decision
			(tenant_id, parcel_id, boundary_kind, boundary_ref, cancellation_id,
			 decision_kind, decided_by, decision_basis, decided_at)
		 SELECT c.tenant_id, c.parcel_id, c.boundary_kind, c.boundary_ref, c.cancellation_id,
		        $6, $7, $8, $9
		   FROM parcel_shipment.cancellation_order_conflict c
		  WHERE c.tenant_id = $1 AND c.parcel_id = $2 AND c.boundary_kind = $3
		    AND c.boundary_ref = $4 AND c.cancellation_id = $5
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(), key.Parcel.String(), key.BoundaryKind.String(), key.BoundaryRef.String(), key.Cancellation.String(),
		decision.Kind.String(), decision.Decider.String(), decision.Basis.String(), decision.DecidedAt.UTC(),
	)
	if err != nil {
		return ports.CancellationOrderConflictDecisionSaveOutcomeInvalid, ports.CancellationOrderConflictDecision{},
			fmt.Errorf("decide cancellation order conflict: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.CancellationOrderConflictDecisionRecorded, decision, nil
	}
	winner, found, err := decisions.FindConflictDecision(ctx, key)
	if err != nil {
		return ports.CancellationOrderConflictDecisionSaveOutcomeInvalid, ports.CancellationOrderConflictDecision{}, err
	}
	if !found {
		return ports.CancellationOrderConflictNotRegistered, ports.CancellationOrderConflictDecision{}, nil
	}
	return ports.CancellationOrderConflictAlreadyDecided, winner, nil
}

// FindConflictDecision 读一份冲突的决定留痕。租户条件进语句，否定结果不区分「没决定」与「属于另一个租户」。
func (decisions *CancellationOrderConflictDecisions) FindConflictDecision(
	ctx context.Context,
	key ports.CancellationOrderConflictKey,
) (ports.CancellationOrderConflictDecision, bool, error) {
	querier, err := decisions.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CancellationOrderConflictDecision{}, false, fmt.Errorf("find cancellation order conflict decision: %w", err)
	}
	var (
		kind, decider, basis string
		decidedAt            time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT decision_kind, decided_by, decision_basis, decided_at
		   FROM parcel_shipment.cancellation_order_conflict_decision
		  WHERE tenant_id = $1 AND parcel_id = $2 AND boundary_kind = $3
		    AND boundary_ref = $4 AND cancellation_id = $5`,
		key.TenantID.String(), key.Parcel.String(), key.BoundaryKind.String(), key.BoundaryRef.String(), key.Cancellation.String(),
	).Scan(&kind, &decider, &basis, &decidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CancellationOrderConflictDecision{}, false, nil
	}
	if err != nil {
		return ports.CancellationOrderConflictDecision{}, false, fmt.Errorf("find cancellation order conflict decision: %w", err)
	}
	decision, err := conflictDecisionOf(key, kind, decider, basis, decidedAt)
	if err != nil {
		return ports.CancellationOrderConflictDecision{}, false, fmt.Errorf("find cancellation order conflict decision: %w", err)
	}
	return decision, true, nil
}

func conflictDecisionOf(
	key ports.CancellationOrderConflictKey,
	kindRaw, deciderRaw, basisRaw string,
	decidedAt time.Time,
) (ports.CancellationOrderConflictDecision, error) {
	kind, err := domain.ParseCancellationOrderConflictDecisionKind(kindRaw)
	if err != nil {
		return ports.CancellationOrderConflictDecision{}, err
	}
	decider, err := domain.NewCancellationOrderConflictDeciderReference(deciderRaw)
	if err != nil {
		return ports.CancellationOrderConflictDecision{}, err
	}
	basis, err := domain.NewCancellationOrderConflictDecisionBasis(basisRaw)
	if err != nil {
		return ports.CancellationOrderConflictDecision{}, err
	}
	return ports.CancellationOrderConflictDecision{
		Key:       key,
		Kind:      kind,
		Decider:   decider,
		Basis:     basis,
		DecidedAt: decidedAt.UTC(),
	}, nil
}

// CancellationOrderConflictCatalogue 实现 ports.CancellationOrderConflictCatalogue：运营查阅按租户列冲突册上还没被决定的行（ADR-0295 决定一）。
// 「未解开」是冲突行没有对应的决定留痕，不是原行上的状态列；租户条件进每条语句（ADR-0003）。
type CancellationOrderConflictCatalogue struct {
	db *bentopg.DB
}

func NewCancellationOrderConflictCatalogue(db *bentopg.DB) (*CancellationOrderConflictCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &CancellationOrderConflictCatalogue{db: db}, nil
}

var _ ports.CancellationOrderConflictCatalogue = (*CancellationOrderConflictCatalogue)(nil)

const (
	cancellationOrderConflictSource = "parcel_shipment.cancellation_order_conflict c"
	// unresolvedCancellationOrderConflict 是「未解开」：冲突行没有对应的决定留痕。
	unresolvedCancellationOrderConflict = `c.tenant_id = $1
		   AND NOT EXISTS (
		       SELECT 1
		         FROM parcel_shipment.cancellation_order_conflict_decision d
		        WHERE d.tenant_id = c.tenant_id
		          AND d.parcel_id = c.parcel_id
		          AND d.boundary_kind = c.boundary_kind
		          AND d.boundary_ref = c.boundary_ref
		          AND d.cancellation_id = c.cancellation_id)`
)

// cancellationOrderConflictPages 把 ports.CancellationOrderConflictPages 落到列上；q 在包裹、边界那一代与取消上做字面包含。
var cancellationOrderConflictPages = ports.CancellationOrderConflictPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"detectedAt": "c.detected_at"},
	Identity: []string{"c.parcel_id", "c.boundary_kind", "c.boundary_ref", "c.cancellation_id"},
	Filters:  map[string]string{"boundaryKind": "c.boundary_kind"},
	Keyword:  []string{"c.parcel_id", "c.boundary_ref", "c.cancellation_id"},
})

// scannedCancellationOrderConflict 带着取页读回的键列原值：游标位置从它取，与 ORDER BY 用的是同一份。
type scannedCancellationOrderConflict struct {
	entry                                 ports.CancellationOrderConflictEntry
	parcel, kind, reference, cancellation string
	detectedAt                            time.Time
}

// ListUnresolvedCancellationOrderConflicts 上列本租户册上未解开的行的一页：缺省最早发现的在前、同刻按（包裹、边界种类、边界那一代、
// 取消）收尾。Count 是未解开的全部行数，不受页与条件截断。几句不在同一快照里：计数与清单是此刻的大概，决定的成败由决定口自己
// 的主键判。
func (catalogue *CancellationOrderConflictCatalogue) ListUnresolvedCancellationOrderConflicts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.UnresolvedCancellationOrderConflicts, error) {
	fail := func(err error) (ports.UnresolvedCancellationOrderConflicts, error) {
		return ports.UnresolvedCancellationOrderConflicts{}, fmt.Errorf("list unresolved cancellation order conflicts: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit %d is not a positive page size", limit))
	}
	plan, err := cancellationOrderConflictPages.Plan(query, unresolvedCancellationOrderConflict, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(cancellationOrderConflictSource), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	backlog, err := queueBacklog(ctx, querier, query, total,
		"SELECT count(*) FROM "+cancellationOrderConflictSource+" WHERE "+unresolvedCancellationOrderConflict, tenant)
	if err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		"c.parcel_id, c.boundary_kind, c.boundary_ref, c.cancellation_id, c.cancelled_at, c.boundary_at, c.detected_at",
		cancellationOrderConflictSource), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	var fetched []scannedCancellationOrderConflict
	for rows.Next() {
		var row scannedCancellationOrderConflict
		var cancelledAt, boundaryAt time.Time
		if err := rows.Scan(&row.parcel, &row.kind, &row.reference, &row.cancellation, &cancelledAt, &boundaryAt, &row.detectedAt); err != nil {
			return fail(err)
		}
		parcel, err := domain.NewDeclaredParcelID(row.parcel)
		if err != nil {
			return fail(err)
		}
		if row.entry, err = conflictEntry(tenant, parcel, row.kind, row.reference, row.cancellation, cancelledAt, boundaryAt, row.detectedAt); err != nil {
			return fail(err)
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row scannedCancellationOrderConflict, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    cataloguepage.FormatInstant(row.detectedAt),
				Identity: []string{row.parcel, row.kind, row.reference, row.cancellation},
			}
		})
	if err != nil {
		return fail(err)
	}
	listed := ports.UnresolvedCancellationOrderConflicts{
		Conflicts: make([]ports.CancellationOrderConflictEntry, 0, len(page.Rows)),
		Next:      page.Next,
		Total:     page.Total,
		Count:     backlog,
	}
	for _, row := range page.Rows {
		listed.Conflicts = append(listed.Conflicts, row.entry)
	}
	return listed, nil
}
