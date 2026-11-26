package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (store *Waves) LoadTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode) (ports.Tote, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.Tote{}, false, err
	}
	var retiredAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT retired_at FROM warehouse_fulfillment.tote
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND tote_code = $3`,
		tenant.String(), warehouse.String(), code.String()).Scan(&retiredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.Tote{}, false, nil
	}
	if err != nil {
		return ports.Tote{}, false, fmt.Errorf("load tote: %w", err)
	}
	return ports.Tote{Warehouse: warehouse, Code: code, Retired: retiredAt != nil}, true, nil
}

// RegisterTote 一句里分三种：没有的新登，停用的恢复在用，已经在用的不动。恢复时登记人与时刻换成这一次的——
// 「这个箱现在是谁让它在用的」看的是最近一次。
func (store *Waves) RegisterTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode, operator domain.Registrant, at time.Time) (bool, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("register tote: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.tote (tenant_id, warehouse_code, tote_code, registered_by, registered_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (tenant_id, warehouse_code, tote_code) DO UPDATE
		    SET registered_by = EXCLUDED.registered_by, registered_at = EXCLUDED.registered_at,
		        retired_by = NULL, retired_at = NULL
		  WHERE tote.retired_at IS NOT NULL`,
		tenant.String(), warehouse.String(), code.String(), operator.String(), at.UTC())
	if err != nil {
		return false, fmt.Errorf("register tote: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// RetireTote 先锁住箱那一行再看它绑没绑着：绑定在拣货时也先锁这一行，两边排在同一行上，
// 不会一边停用、一边刚把货放进去。
func (store *Waves) RetireTote(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode, operator domain.Registrant, at time.Time) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("retire tote: %w", err)
	}
	var retiredAt *time.Time
	if err := executor.QueryRow(ctx,
		`SELECT retired_at FROM warehouse_fulfillment.tote
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND tote_code = $3
		    FOR UPDATE`,
		tenant.String(), warehouse.String(), code.String()).Scan(&retiredAt); err != nil {
		return fmt.Errorf("retire tote: %w", err)
	}
	if retiredAt != nil {
		return fmt.Errorf("retire tote: %s is already retired", code.String())
	}
	var bound bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM warehouse_fulfillment.tote_binding
		     WHERE tenant_id = $1 AND warehouse_code = $2 AND tote_code = $3 AND released_at IS NULL)`,
		tenant.String(), warehouse.String(), code.String()).Scan(&bound); err != nil {
		return fmt.Errorf("retire tote: %w", err)
	}
	if bound {
		return fmt.Errorf("retire tote: %s still holds an outbound request", code.String())
	}
	if _, err := executor.Exec(ctx,
		`UPDATE warehouse_fulfillment.tote SET retired_by = $4, retired_at = $5
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND tote_code = $3`,
		tenant.String(), warehouse.String(), code.String(), operator.String(), at.UTC()); err != nil {
		return fmt.Errorf("retire tote: %w", err)
	}
	return nil
}

func (store *Waves) ToteHolder(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.ToteCode) (domain.OutboundRequestID, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return domain.OutboundRequestID{}, false, err
	}
	var raw string
	err = querier.QueryRow(ctx,
		`SELECT request_id FROM warehouse_fulfillment.tote_binding
		  WHERE tenant_id = $1 AND warehouse_code = $2 AND tote_code = $3 AND released_at IS NULL`,
		tenant.String(), warehouse.String(), code.String()).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OutboundRequestID{}, false, nil
	}
	if err != nil {
		return domain.OutboundRequestID{}, false, fmt.Errorf("tote holder: %w", err)
	}
	request, err := domain.NewOutboundRequestID(raw)
	if err != nil {
		return domain.OutboundRequestID{}, false, fmt.Errorf("tote holder: %w", err)
	}
	return request, true, nil
}

const toteListColumns = `warehouse_code, tote_code, state, request_id, bound_at, registered_by, registered_at, retired_by, retired_at`

// 状态在这一句里按一条规则算：停用的是已停用，没停用又有一条还没放开的绑定是在用，其余空着。筛选与读出来的是同一列。
const toteListed = `WITH listed AS (
	SELECT tote.warehouse_code, tote.tote_code,
	       CASE WHEN tote.retired_at IS NOT NULL THEN 'RETIRED'
	            WHEN binding.request_id IS NOT NULL THEN 'IN_USE'
	            ELSE 'FREE' END AS state,
	       binding.request_id, binding.bound_at, tote.registered_by, tote.registered_at, tote.retired_by, tote.retired_at
	  FROM warehouse_fulfillment.tote tote
	  LEFT JOIN warehouse_fulfillment.tote_binding binding
	    ON binding.tenant_id = tote.tenant_id AND binding.warehouse_code = tote.warehouse_code
	   AND binding.tote_code = tote.tote_code AND binding.released_at IS NULL
	 WHERE tote.tenant_id = $1
)`

var toteListBinding = ports.TotePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"toteCode": "tote_code"},
	Identity: []string{"warehouse_code", "tote_code"},
	Filters: map[string]string{
		"warehouseCode": "warehouse_code",
		"state":         "state",
	},
	Keyword: []string{"tote_code", "request_id"},
})

// ListTotes 按分页条件列出本租户的周转箱，连同每个箱此刻绑着的出库请求。
func (store *Waves) ListTotes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ToteRow], error) {
	const operation = "list totes"
	fail := func(err error) (cataloguepage.Result[ports.ToteRow], error) {
		return cataloguepage.Result[ports.ToteRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := toteListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, toteListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, toteListed+" "+plan.PageSQL(toteListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.ToteRow
	for rows.Next() {
		var item ports.ToteRow
		var request, retiredBy *string
		if err := rows.Scan(
			&item.WarehouseCode, &item.ToteCode, &item.State, &request, &item.BoundAt,
			&item.RegisteredBy, &item.RegisteredAt, &retiredBy, &item.RetiredAt,
		); err != nil {
			return fail(err)
		}
		item.RequestID = valueOrEmpty(request)
		item.RetiredBy = valueOrEmpty(retiredBy)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.ToteRow, _ string) cataloguepage.Position {
		return cataloguepage.Position{Value: item.ToteCode, Identity: []string{item.WarehouseCode, item.ToteCode}}
	})
}
