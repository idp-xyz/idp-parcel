package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const outboundRequestListColumns = `
	request_id, owner_account_id, warehouse_code, origin, request_key, dispatch,
	service_product, recipient, address, basis, operator_id, status, document_number,
	numbering_absence, received_at, parent_request_id, platform_stocking,
	prefetch_shipment_id, prefetch_declared_parcel_id, partial_shipment`

// single_unit_word 是「单品单件」筛选用的那一列：这张请求要拣的只有一件，即预占需求合计是 1 个基本单位。
// 口径与 waveIsSingleUnit 对每张请求问的是同一句；代理仓的请求不预占，合计是 0，不算。
const outboundRequestListed = `WITH listed AS (
	SELECT ` + outboundRequestListColumns + `,
	       ((SELECT coalesce(sum(reservation.wanted), 0)
	           FROM warehouse_fulfillment.outbound_reservation reservation
	          WHERE reservation.tenant_id = request.tenant_id
	            AND reservation.request_id = request.request_id) = 1)::text AS single_unit_word
	  FROM warehouse_fulfillment.outbound_request request
	 WHERE request.tenant_id = $1
)`

var outboundRequestListBinding = ports.OutboundRequestPages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"receivedAt": "received_at",
		"requestId":  "request_id",
	},
	Identity: []string{"request_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"dispatch":      "dispatch",
		"status":        "status",
		"singleUnit":    "single_unit_word",
	},
	Keyword: []string{"request_id", "document_number", "owner_account_id", "warehouse_code", "recipient", "request_key"},
})

// List 按分页条件列出本租户的出库请求及其明细和预占汇总。
func (store *OutboundRequests) List(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.OutboundRequestRow], error) {
	const operation = "list outbound requests"
	fail := func(err error) (cataloguepage.Result[ports.OutboundRequestRow], error) {
		return cataloguepage.Result[ports.OutboundRequestRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := outboundRequestListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, outboundRequestListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, outboundRequestListed+" "+plan.PageSQL(outboundRequestListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.OutboundRequestRow
	for rows.Next() {
		var (
			item                                              ports.OutboundRequestRow
			service, basis, operator, number, absence, parent *string
			prefetchShipment, prefetchDeclared, partial       *string
		)
		if err := rows.Scan(
			&item.RequestID, &item.OwnerAccount, &item.WarehouseCode, &item.Origin, &item.RequestKey, &item.Dispatch,
			&service, &item.Recipient, &item.Address, &basis, &operator, &item.Status, &number,
			&absence, &item.ReceivedAt, &parent, &item.PlatformStocking,
			&prefetchShipment, &prefetchDeclared, &partial,
		); err != nil {
			return fail(err)
		}
		item.PartialShipment = valueOrEmpty(partial)
		item.Service = valueOrEmpty(service)
		item.Basis = valueOrEmpty(basis)
		item.Operator = valueOrEmpty(operator)
		item.DocumentNumber = valueOrEmpty(number)
		item.NumberingAbsence = valueOrEmpty(absence)
		item.ParentRequestID = valueOrEmpty(parent)
		item.PrefetchShipmentID = valueOrEmpty(prefetchShipment)
		item.PrefetchDeclaredParcelID = valueOrEmpty(prefetchDeclared)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	for index := range items {
		lineRows, err := querier.Query(ctx,
			`SELECT goods_code, unit_name, stock_state, quantity
			   FROM warehouse_fulfillment.outbound_request_line
			  WHERE tenant_id = $1 AND request_id = $2
			  ORDER BY line_index`,
			tenant.String(), items[index].RequestID)
		if err != nil {
			return fail(err)
		}
		for lineRows.Next() {
			var line ports.OutboundRequestLineRow
			if err := lineRows.Scan(&line.GoodsCode, &line.Unit, &line.StockState, &line.Quantity); err != nil {
				lineRows.Close()
				return fail(err)
			}
			items[index].Lines = append(items[index].Lines, line)
		}
		if err := lineRows.Err(); err != nil {
			lineRows.Close()
			return fail(err)
		}
		lineRows.Close()
		if err := querier.QueryRow(ctx,
			`SELECT coalesce(sum(quantity), 0)::bigint, coalesce(sum(wanted), 0)::bigint
			   FROM warehouse_fulfillment.outbound_reservation
			  WHERE tenant_id = $1 AND request_id = $2`,
			tenant.String(), items[index].RequestID,
		).Scan(&items[index].ReservedQuantity, &items[index].WantedQuantity); err != nil {
			return fail(err)
		}
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.OutboundRequestRow, sortField string) cataloguepage.Position {
		switch sortField {
		case "requestId":
			return cataloguepage.Position{Value: item.RequestID, Identity: []string{item.RequestID}}
		default:
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.ReceivedAt), Identity: []string{item.RequestID}}
		}
	})
}

// 波次的一行连同它的出库请求与拣货任务在同一句里取出：状态、成员与任务是同一刻的，不会状态说拣货中、
// 任务却都已经拣完。成员与任务各聚成一个 JSON 数组，只对取到的这一页算。$1 是租户，与 waveListed 用的是同一个参数。
const waveRowSelection = `wave_id, warehouse_code, operator_id, prefetch, status, released_at, single_unit,
	(SELECT coalesce(jsonb_agg(jsonb_build_object(
	            'requestId', member.request_id, 'withdrawn', member.withdrawn_at IS NOT NULL
	        ) ORDER BY member.request_id), '[]'::jsonb)
	   FROM warehouse_fulfillment.wave_request member
	  WHERE member.tenant_id = $1 AND member.wave_id = listed.wave_id) AS members,
	(SELECT coalesce(jsonb_agg(jsonb_build_object(
	            'taskId', task.task_id, 'requestId', task.request_id, 'locationCode', task.location_code,
	            'stagingLocation', coalesce(task.staging_location, ''), 'goodsCode', task.goods_code, 'unit', task.unit_name,
	            'stockState', task.stock_state, 'quantity', task.quantity, 'picked', task.picked, 'status', task.status
	        ) ORDER BY task.task_id), '[]'::jsonb)
	   FROM warehouse_fulfillment.pick_task task
	  WHERE task.tenant_id = $1 AND task.wave_id = listed.wave_id) AS tasks`

const waveListed = `WITH listed AS (
	SELECT wave_id, warehouse_code, operator_id, prefetch, prefetch::text AS prefetch_word, status, released_at,
	       single_unit, single_unit::text AS single_unit_word
	  FROM warehouse_fulfillment.wave
	 WHERE tenant_id = $1
)`

var waveListBinding = ports.WavePages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"releasedAt": "released_at",
		"waveId":     "wave_id",
	},
	Identity: []string{"wave_id"},
	Filters: map[string]string{
		"warehouseCode": "warehouse_code",
		"prefetch":      "prefetch_word",
		"status":        "status",
		"singleUnit":    "single_unit_word",
	},
	Keyword: []string{"wave_id", "warehouse_code", "operator_id"},
})

// ListWaves 按分页条件列出本租户已放出的波次及其请求与拣货任务。
func (store *Waves) ListWaves(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.WaveRow], error) {
	const operation = "list waves"
	fail := func(err error) (cataloguepage.Result[ports.WaveRow], error) {
		return cataloguepage.Result[ports.WaveRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := waveListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, waveListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, waveListed+" "+plan.PageSQL(waveRowSelection, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.WaveRow
	for rows.Next() {
		item, err := scanWaveRow(rows)
		if err != nil {
			return fail(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.WaveRow, sortField string) cataloguepage.Position {
		switch sortField {
		case "waveId":
			return cataloguepage.Position{Value: item.WaveID, Identity: []string{item.WaveID}}
		default:
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.ReleasedAt), Identity: []string{item.WaveID}}
		}
	})
}

// LoadWaveRow 按波次号读一张，形状与列表的一行相同。没有这个波次时 found 为假。
func (store *Waves) LoadWaveRow(ctx context.Context, tenant domain.TenantID, waveID string) (ports.WaveRow, bool, error) {
	const operation = "load wave"
	if tenant.String() == "" || waveID == "" {
		return ports.WaveRow{}, false, fmt.Errorf("%s: tenant and wave are required", operation)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.WaveRow{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	item, err := scanWaveRow(querier.QueryRow(ctx,
		waveListed+" SELECT "+waveRowSelection+" FROM listed WHERE wave_id = $2",
		tenant.String(), waveID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.WaveRow{}, false, nil
	}
	if err != nil {
		return ports.WaveRow{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return item, true, nil
}

// scanWaveRow 读 waveRowSelection 取出的一行，把成员与任务两个 JSON 数组解开。
func scanWaveRow(row interface{ Scan(dest ...any) error }) (ports.WaveRow, error) {
	var item ports.WaveRow
	var members, tasks []byte
	if err := row.Scan(&item.WaveID, &item.WarehouseCode, &item.Operator, &item.Prefetch, &item.Status, &item.ReleasedAt, &item.SingleUnit, &members, &tasks); err != nil {
		return ports.WaveRow{}, err
	}
	var listed []struct {
		RequestID string `json:"requestId"`
		Withdrawn bool   `json:"withdrawn"`
	}
	if err := json.Unmarshal(members, &listed); err != nil {
		return ports.WaveRow{}, fmt.Errorf("wave members: %w", err)
	}
	for _, member := range listed {
		item.Requests = append(item.Requests, member.RequestID)
		if member.Withdrawn {
			item.Withdrawn = append(item.Withdrawn, member.RequestID)
		}
	}
	var picks []struct {
		TaskID          string `json:"taskId"`
		RequestID       string `json:"requestId"`
		LocationCode    string `json:"locationCode"`
		StagingLocation string `json:"stagingLocation"`
		GoodsCode       string `json:"goodsCode"`
		Unit            string `json:"unit"`
		StockState      string `json:"stockState"`
		Quantity        int64  `json:"quantity"`
		Picked          int64  `json:"picked"`
		Status          string `json:"status"`
	}
	if err := json.Unmarshal(tasks, &picks); err != nil {
		return ports.WaveRow{}, fmt.Errorf("wave tasks: %w", err)
	}
	for _, task := range picks {
		item.Tasks = append(item.Tasks, ports.WaveTaskRow{
			TaskID: task.TaskID, RequestID: task.RequestID, LocationCode: task.LocationCode, StagingLocation: task.StagingLocation,
			GoodsCode: task.GoodsCode, Unit: task.Unit, StockState: task.StockState,
			Quantity: task.Quantity, Picked: task.Picked, Status: task.Status,
		})
	}
	return item, nil
}

const pickDifferenceListColumns = `difference_id, status, quantity, task_id, wave_id, request_id, owner_account_id, warehouse_code,
	location_code, goods_code, unit_name, stock_state, task_quantity, picked, recorded_at, closed_at, closed_by_count, recorded_order`

const pickDifferenceListed = `WITH listed AS (
	SELECT difference.difference_id, difference.status, difference.quantity, difference.task_id, task.wave_id, task.request_id,
	       request.owner_account_id, request.warehouse_code, task.location_code, task.goods_code, task.unit_name, task.stock_state,
	       task.quantity AS task_quantity, task.picked, difference.recorded_at, difference.closed_at, difference.closed_by_count,
	       coalesce(difference.recorded_at, 'epoch'::timestamptz) AS recorded_order
	  FROM warehouse_fulfillment.pick_difference difference
	  JOIN warehouse_fulfillment.pick_task task
	    ON task.tenant_id = difference.tenant_id AND task.task_id = difference.task_id
	  JOIN warehouse_fulfillment.outbound_request request
	    ON request.tenant_id = task.tenant_id AND request.request_id = task.request_id
	 WHERE difference.tenant_id = $1
)`

var pickDifferenceListBinding = ports.PickDifferencePages.MustBind(cataloguepage.Columns{
	// 早先的差异没记时刻：排序用的那一列拿纪元起点顶上，它们排在最旧的一头；读出来的时刻仍是空的，不替它补。
	Sorts:    map[string]string{"recordedAt": "recorded_order", "differenceId": "difference_id"},
	Identity: []string{"difference_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"status":        "status",
	},
	Keyword: []string{"difference_id", "location_code", "goods_code", "request_id", "wave_id"},
})

// ListPickDifferences 按分页条件列出本租户少拣留下的拣货差异，连同它所在的任务、波次、出库请求与库位。
func (store *Waves) ListPickDifferences(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PickDifferenceRow], error) {
	const operation = "list pick differences"
	fail := func(err error) (cataloguepage.Result[ports.PickDifferenceRow], error) {
		return cataloguepage.Result[ports.PickDifferenceRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := pickDifferenceListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, pickDifferenceListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, pickDifferenceListed+" "+plan.PageSQL(pickDifferenceListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.PickDifferenceRow
	order := map[string]time.Time{}
	for rows.Next() {
		var item ports.PickDifferenceRow
		var closedBy *string
		var recordedOrder time.Time
		if err := rows.Scan(
			&item.DifferenceID, &item.Status, &item.Quantity, &item.TaskID, &item.WaveID, &item.RequestID, &item.OwnerAccount, &item.WarehouseCode,
			&item.LocationCode, &item.GoodsCode, &item.Unit, &item.StockState, &item.TaskQuantity, &item.Picked,
			&item.RecordedAt, &item.ClosedAt, &closedBy, &recordedOrder,
		); err != nil {
			return fail(err)
		}
		item.ClosedByCount = valueOrEmpty(closedBy)
		order[item.DifferenceID] = recordedOrder
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.PickDifferenceRow, sortField string) cataloguepage.Position {
		if sortField == "differenceId" {
			return cataloguepage.Position{Value: item.DifferenceID, Identity: []string{item.DifferenceID}}
		}
		return cataloguepage.Position{Value: cataloguepage.FormatInstant(order[item.DifferenceID]), Identity: []string{item.DifferenceID}}
	})
}

const chargeOccurrenceListColumns = `
	occurrence_id, version, supersedes, correction, kind, owner_account_id, warehouse_code,
	source_fact, business_time, measures, attributes, lines, weight_unit, volume_unit`

const chargeOccurrenceListed = `WITH listed AS (
	SELECT occurrence.occurrence_id, version.version, version.supersedes, version.correction,
	       occurrence.kind, occurrence.owner_account_id, occurrence.warehouse_code,
	       occurrence.source_fact, version.business_time, version.measures, version.attributes,
	       version.lines, version.weight_unit, version.volume_unit
	  FROM warehouse_fulfillment.charge_occurrence occurrence
	  JOIN warehouse_fulfillment.charge_occurrence_version version
	    ON version.tenant_id = occurrence.tenant_id
	   AND version.occurrence_id = occurrence.occurrence_id
	   AND version.version = occurrence.head_version
	 WHERE occurrence.tenant_id = $1
)`

var chargeOccurrenceListBinding = ports.ChargeOccurrencePages.MustBind(cataloguepage.Columns{
	Sorts: map[string]string{
		"businessTime": "business_time",
		"occurrenceId": "occurrence_id",
	},
	Identity: []string{"occurrence_id"},
	Filters: map[string]string{
		"ownerAccount":  "owner_account_id",
		"warehouseCode": "warehouse_code",
		"kind":          "kind",
	},
	Keyword: []string{"occurrence_id", "owner_account_id", "warehouse_code", "source_fact"},
})

// ListOccurrences 按分页条件列出本租户的仓储作业收费发生项（每项取最新一版）。
func (store *Charges) ListOccurrences(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ChargeOccurrenceRow], error) {
	const operation = "list charge occurrences"
	fail := func(err error) (cataloguepage.Result[ports.ChargeOccurrenceRow], error) {
		return cataloguepage.Result[ports.ChargeOccurrenceRow]{}, fmt.Errorf("%s: %w", operation, err)
	}
	if tenant.String() == "" || limit < 1 {
		return fail(errors.New("tenant and a positive limit are required"))
	}
	plan, err := chargeOccurrenceListBinding.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, chargeOccurrenceListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, chargeOccurrenceListed+" "+plan.PageSQL(chargeOccurrenceListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.ChargeOccurrenceRow
	for rows.Next() {
		var (
			item                   ports.ChargeOccurrenceRow
			supersedes             *int
			measures, attrs, lines []byte
			weightUnit, volumeUnit *string
			businessTime           time.Time
		)
		if err := rows.Scan(
			&item.OccurrenceID, &item.Version, &supersedes, &item.Correction, &item.Kind,
			&item.OwnerAccount, &item.Warehouse, &item.SourceFact, &businessTime,
			&measures, &attrs, &lines, &weightUnit, &volumeUnit,
		); err != nil {
			return fail(err)
		}
		item.BusinessTime = businessTime
		if supersedes != nil {
			item.Supersedes = *supersedes
		}
		item.WeightUnit = valueOrEmpty(weightUnit)
		item.VolumeUnit = valueOrEmpty(volumeUnit)
		if err := json.Unmarshal(measures, &item.Measures); err != nil {
			return fail(err)
		}
		if err := json.Unmarshal(attrs, &item.Attributes); err != nil {
			return fail(err)
		}
		var parsedLines []ports.ChargeLine
		if err := json.Unmarshal(lines, &parsedLines); err != nil {
			return fail(err)
		}
		item.LineCount = len(parsedLines)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	return cataloguepage.Finish(query, items, limit, total, func(item ports.ChargeOccurrenceRow, sortField string) cataloguepage.Position {
		switch sortField {
		case "occurrenceId":
			return cataloguepage.Position{Value: item.OccurrenceID, Identity: []string{item.OccurrenceID}}
		default:
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(item.BusinessTime), Identity: []string{item.OccurrenceID}}
		}
	})
}
