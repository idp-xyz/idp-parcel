package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const outboundRequestListColumns = `
	request_id, owner_account_id, warehouse_code, origin, request_key, dispatch,
	service_product, recipient, address, basis, operator_id, status, document_number,
	numbering_absence, received_at, parent_request_id, platform_stocking,
	prefetch_shipment_id, prefetch_declared_parcel_id`

const outboundRequestListed = `WITH listed AS (
	SELECT ` + outboundRequestListColumns + `
	  FROM warehouse_fulfillment.outbound_request
	 WHERE tenant_id = $1
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
			prefetchShipment, prefetchDeclared                *string
		)
		if err := rows.Scan(
			&item.RequestID, &item.OwnerAccount, &item.WarehouseCode, &item.Origin, &item.RequestKey, &item.Dispatch,
			&service, &item.Recipient, &item.Address, &basis, &operator, &item.Status, &number,
			&absence, &item.ReceivedAt, &parent, &item.PlatformStocking,
			&prefetchShipment, &prefetchDeclared,
		); err != nil {
			return fail(err)
		}
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

const waveListColumns = `wave_id, warehouse_code, operator_id, prefetch, released_at`

const waveListed = `WITH listed AS (
	SELECT wave_id, warehouse_code, operator_id, prefetch, prefetch::text AS prefetch_word, released_at
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
	rows, err := querier.Query(ctx, waveListed+" "+plan.PageSQL(waveListColumns, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var items []ports.WaveRow
	for rows.Next() {
		var item ports.WaveRow
		if err := rows.Scan(&item.WaveID, &item.WarehouseCode, &item.Operator, &item.Prefetch, &item.ReleasedAt); err != nil {
			return fail(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	for index := range items {
		reqRows, err := querier.Query(ctx,
			`SELECT request_id FROM warehouse_fulfillment.wave_request
			  WHERE tenant_id = $1 AND wave_id = $2
			  ORDER BY request_id`,
			tenant.String(), items[index].WaveID)
		if err != nil {
			return fail(err)
		}
		for reqRows.Next() {
			var id string
			if err := reqRows.Scan(&id); err != nil {
				reqRows.Close()
				return fail(err)
			}
			items[index].Requests = append(items[index].Requests, id)
		}
		if err := reqRows.Err(); err != nil {
			reqRows.Close()
			return fail(err)
		}
		reqRows.Close()

		taskRows, err := querier.Query(ctx,
			`SELECT task_id, request_id, location_code, staging_location, goods_code, unit_name, stock_state, quantity, picked, status
			   FROM warehouse_fulfillment.pick_task
			  WHERE tenant_id = $1 AND wave_id = $2
			  ORDER BY task_id`,
			tenant.String(), items[index].WaveID)
		if err != nil {
			return fail(err)
		}
		for taskRows.Next() {
			var task ports.WaveTaskRow
			var staging *string
			if err := taskRows.Scan(
				&task.TaskID, &task.RequestID, &task.LocationCode, &staging,
				&task.GoodsCode, &task.Unit, &task.StockState, &task.Quantity, &task.Picked, &task.Status,
			); err != nil {
				taskRows.Close()
				return fail(err)
			}
			task.StagingLocation = valueOrEmpty(staging)
			items[index].Tasks = append(items[index].Tasks, task)
		}
		if err := taskRows.Err(); err != nil {
			taskRows.Close()
			return fail(err)
		}
		taskRows.Close()
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
