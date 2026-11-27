package postgres

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 库区取库位的当前修订。认不出库区时两处都是空：拣货时这条任务就照整个波次的指派走。
const pickTaskZoneCode = `(SELECT place.zone_code
	    FROM warehouse_fulfillment.location_revision place
	   WHERE place.tenant_id = task.tenant_id AND place.warehouse_code = request.warehouse_code
	     AND place.location_code = task.location_code
	   ORDER BY place.revision DESC LIMIT 1)`

const pickTaskListColumns = `task_id, wave_id, request_id, owner_account_id, location_code, zone_code,
	goods_code, unit_name, stock_state, quantity, status, warehouse_code, single_unit, batch_pick,
	assignee, zone_assignee, released_at`

// pickTaskListed 一行一条拣货任务。库区指派只在这个库区单独派了人时有值，与 application.WaveHandler 的 pickAssignee 同一口径。
const pickTaskListed = `WITH listed AS (
	SELECT task.task_id, task.wave_id, task.request_id, request.owner_account_id, task.location_code,
	       coalesce(` + pickTaskZoneCode + `, '') AS zone_code,
	       task.goods_code, task.unit_name, task.stock_state, task.quantity, task.status,
	       wave.warehouse_code, wave.single_unit, wave.batch_pick, coalesce(wave.assignee, '') AS assignee,
	       coalesce((SELECT zoned.assignee
	                   FROM warehouse_fulfillment.wave_zone_assignee zoned
	                  WHERE zoned.tenant_id = task.tenant_id AND zoned.wave_id = task.wave_id
	                    AND zoned.zone_code = ` + pickTaskZoneCode + `
	                  LIMIT 1), '') AS zone_assignee,
	       wave.released_at
	  FROM warehouse_fulfillment.pick_task task
	  JOIN warehouse_fulfillment.wave wave
	    ON wave.tenant_id = task.tenant_id AND wave.wave_id = task.wave_id
	  JOIN warehouse_fulfillment.outbound_request request
	    ON request.tenant_id = task.tenant_id AND request.request_id = task.request_id
	 WHERE task.tenant_id = $1
)`

var pickTaskListBinding = ports.PickTaskPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"releasedAt": "released_at",
		"taskId":     "task_id",
	},
	Identity: []string{"task_id"},
	Filters: map[string]string{
		"warehouseCode": "warehouse_code",
		"waveId":        "wave_id",
		"status":        "status",
	},
	Keyword: []string{"task_id", "wave_id", "location_code", "goods_code"},
})

// ListPickTasks 按分页条件列出本租户的拣货任务。拣货工位用它按仓与待执行要队列，不把整个波次的任务都取来再挑。
func (store *Waves) ListPickTasks(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PickTaskListRow], error) {
	const operation = "list pick tasks"
	fail := func(err error) (cataloguepage.Result[ports.PickTaskListRow], error) {
		return cataloguepage.Result[ports.PickTaskListRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := pickTaskListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, pickTaskListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, pickTaskListed+" "+plan.PageSQL(pickTaskListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.PickTaskListRow
	for rows.Next() {
		var item ports.PickTaskListRow
		if err := rows.Scan(
			&item.TaskID, &item.WaveID, &item.RequestID, &item.OwnerAccount, &item.LocationCode, &item.ZoneCode,
			&item.GoodsCode, &item.Unit, &item.StockState, &item.Quantity, &item.Status,
			&item.WarehouseCode, &item.SingleUnit, &item.BatchPick, &item.Assignee, &item.ZoneAssignee, &item.ReleasedAt,
		); err != nil {
			return fail(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.PickTaskListRow, sortField string) cataloguepage.Position {
		if sortField == "taskId" {
			return cataloguepage.Position{Value: item.TaskID, Identity: []string{item.TaskID}}
		}
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.ReleasedAt), Identity: []string{item.TaskID}}
	})
}
