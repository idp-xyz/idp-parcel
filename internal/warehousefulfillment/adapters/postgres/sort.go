package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

var _ ports.SortPlans = (*Waves)(nil)

func (store *Waves) WaveSorts(ctx context.Context, tenant domain.TenantID, id domain.WaveID) (bool, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return false, false, err
	}
	var sorts bool
	err = querier.QueryRow(ctx,
		`SELECT batch_pick FROM warehouse_fulfillment.wave WHERE tenant_id = $1 AND wave_id = $2`,
		tenant.String(), id.String()).Scan(&sorts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("wave sorts: %w", err)
	}
	return sorts, true, nil
}

func (store *Waves) LoadSortPut(ctx context.Context, tenant domain.TenantID, factID string) (ports.SortPut, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SortPut{}, false, err
	}
	var wave, request, goods, warehouse, tote string
	put := ports.SortPut{FactID: factID}
	err = querier.QueryRow(ctx,
		`SELECT wave_id, request_id, goods_code, quantity, warehouse_code, tote_code, recorded_at
		   FROM warehouse_fulfillment.sort_put
		  WHERE tenant_id = $1 AND fact_id = $2`,
		tenant.String(), factID).Scan(&wave, &request, &goods, &put.Quantity, &warehouse, &tote, &put.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SortPut{}, false, nil
	}
	if err != nil {
		return ports.SortPut{}, false, fmt.Errorf("load sort put: %w", err)
	}
	if put.Wave, err = domain.NewWaveID(wave); err != nil {
		return ports.SortPut{}, false, fmt.Errorf("load sort put: %w", err)
	}
	if put.Request, err = domain.NewOutboundRequestID(request); err != nil {
		return ports.SortPut{}, false, fmt.Errorf("load sort put: %w", err)
	}
	if put.Goods, err = domain.NewGoodsCode(goods); err != nil {
		return ports.SortPut{}, false, fmt.Errorf("load sort put: %w", err)
	}
	if put.Warehouse, err = domain.NewWarehouseCode(warehouse); err != nil {
		return ports.SortPut{}, false, fmt.Errorf("load sort put: %w", err)
	}
	if put.Tote, err = domain.NewToteCode(tote); err != nil {
		return ports.SortPut{}, false, fmt.Errorf("load sort put: %w", err)
	}
	return put, true, nil
}

func (store *Waves) SaveSortPut(ctx context.Context, tenant domain.TenantID, put ports.SortPut) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save sort put: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO warehouse_fulfillment.sort_put (
		    tenant_id, fact_id, wave_id, request_id, goods_code, quantity, warehouse_code, tote_code, recorded_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tenant.String(), put.FactID, put.Wave.String(), put.Request.String(), put.Goods.String(), put.Quantity,
		put.Warehouse.String(), put.Tote.String(), put.RecordedAt.UTC()); err != nil {
		return fmt.Errorf("save sort put: %w", err)
	}
	return nil
}

func (store *Waves) SortedOf(ctx context.Context, tenant domain.TenantID, wave domain.WaveID, request domain.OutboundRequestID, goods domain.GoodsCode) (int64, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var sorted int64
	if err := querier.QueryRow(ctx,
		`SELECT coalesce(sum(quantity), 0)::bigint FROM warehouse_fulfillment.sort_put
		  WHERE tenant_id = $1 AND request_id = $2 AND wave_id = $3 AND goods_code = $4`,
		tenant.String(), request.String(), wave.String(), goods.String()).Scan(&sorted); err != nil {
		return 0, fmt.Errorf("sorted of: %w", err)
	}
	return sorted, nil
}

// UnsortedOf 两头都只数合并拣货的波次：投放只会记在这种波次名下，拣出来的也只有这种波次的要分。
func (store *Waves) UnsortedOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (int64, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var unsorted int64
	if err := querier.QueryRow(ctx,
		`SELECT (SELECT coalesce(sum(task.picked), 0)
		           FROM warehouse_fulfillment.pick_task task
		           JOIN warehouse_fulfillment.wave wave
		             ON wave.tenant_id = task.tenant_id AND wave.wave_id = task.wave_id AND wave.batch_pick
		          WHERE task.tenant_id = $1 AND task.request_id = $2 AND task.status IN ('DONE', 'SHORT'))
		      - (SELECT coalesce(sum(put.quantity), 0)
		           FROM warehouse_fulfillment.sort_put put
		          WHERE put.tenant_id = $1 AND put.request_id = $2)`,
		tenant.String(), request.String()).Scan(&unsorted); err != nil {
		return 0, fmt.Errorf("unsorted of: %w", err)
	}
	return unsorted, nil
}

// sortPlanRequests 在同一句里读出波次里各张请求的箱与各种货的拣出、投放、待拣件数，是同一刻的。
// 只算待执行、拣满、少拣三种任务：作废与迟到的货不往箱里分。打完包之后的请求不列，它的箱已经放开。
const sortPlanRequests = `(
	SELECT coalesce(jsonb_agg(jsonb_build_object(
	            'requestId', request.request_id, 'documentNumber', coalesce(request.document_number, ''),
	            'ownerAccount', request.owner_account_id, 'status', request.status,
	            'totes', (SELECT coalesce(jsonb_agg(binding.tote_code ORDER BY binding.tote_code), '[]'::jsonb)
	                        FROM warehouse_fulfillment.tote_binding binding
	                       WHERE binding.tenant_id = request.tenant_id AND binding.request_id = request.request_id
	                         AND binding.released_at IS NULL),
	            'lines', (SELECT coalesce(jsonb_agg(jsonb_build_object(
	                                 'goodsCode', line.goods_code, 'unit', line.unit_name, 'picked', line.picked, 'open', line.open,
	                                 'sorted', (SELECT coalesce(sum(put.quantity), 0)
	                                              FROM warehouse_fulfillment.sort_put put
	                                             WHERE put.tenant_id = request.tenant_id AND put.request_id = request.request_id
	                                               AND put.wave_id = wave.wave_id AND put.goods_code = line.goods_code)
	                             ) ORDER BY line.goods_code), '[]'::jsonb)
	                        FROM (SELECT task.goods_code, min(task.unit_name) AS unit_name,
	                                     coalesce(sum(task.picked) FILTER (WHERE task.status IN ('DONE', 'SHORT')), 0) AS picked,
	                                     coalesce(sum(task.quantity) FILTER (WHERE task.status = 'OPEN'), 0) AS open
	                                FROM warehouse_fulfillment.pick_task task
	                               WHERE task.tenant_id = request.tenant_id AND task.request_id = request.request_id
	                                 AND task.wave_id = wave.wave_id AND task.status IN ('OPEN', 'DONE', 'SHORT')
	                               GROUP BY task.goods_code) line)
	        ) ORDER BY request.received_at, request.request_id), '[]'::jsonb)
	  FROM warehouse_fulfillment.wave_request member
	  JOIN warehouse_fulfillment.outbound_request request
	    ON request.tenant_id = member.tenant_id AND request.request_id = member.request_id
	 WHERE member.tenant_id = wave.tenant_id AND member.wave_id = wave.wave_id AND member.withdrawn_at IS NULL
	   AND request.status IN ('ALLOCATED', 'RESERVED', 'PICKED', 'CHECKED'))`

func (store *Waves) LoadSortPlan(ctx context.Context, tenant domain.TenantID, wave string) (ports.SortPlan, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return ports.SortPlan{}, false, err
	}
	plan := ports.SortPlan{Wave: wave}
	var requests []byte
	err = querier.QueryRow(ctx,
		`SELECT wave.warehouse_code, wave.status, wave.batch_pick,
		        CASE WHEN wave.batch_pick THEN `+sortPlanRequests+` ELSE '[]'::jsonb END
		   FROM warehouse_fulfillment.wave wave
		  WHERE wave.tenant_id = $1 AND wave.wave_id = $2`,
		tenant.String(), wave).Scan(&plan.Warehouse, &plan.Status, &plan.Sorts, &requests)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.SortPlan{}, false, nil
	}
	if err != nil {
		return ports.SortPlan{}, false, fmt.Errorf("load sort plan: %w", err)
	}
	var decoded []struct {
		Request        string   `json:"requestId"`
		DocumentNumber string   `json:"documentNumber"`
		Owner          string   `json:"ownerAccount"`
		Status         string   `json:"status"`
		Totes          []string `json:"totes"`
		Lines          []struct {
			Goods  string `json:"goodsCode"`
			Unit   string `json:"unit"`
			Picked int64  `json:"picked"`
			Sorted int64  `json:"sorted"`
			Open   int64  `json:"open"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(requests, &decoded); err != nil {
		return ports.SortPlan{}, false, fmt.Errorf("load sort plan: %w", err)
	}
	for _, item := range decoded {
		request := ports.SortPlanRequest{
			Request: item.Request, DocumentNumber: item.DocumentNumber, Owner: item.Owner, Status: item.Status, Totes: item.Totes,
		}
		pending := false
		for _, line := range item.Lines {
			request.Lines = append(request.Lines, ports.SortPlanLine{
				Goods: line.Goods, Unit: line.Unit, Picked: line.Picked, Sorted: line.Sorted, Open: line.Open,
			})
			pending = pending || line.Picked > 0 || line.Open > 0
		}
		// 在这个波次里一件都没拣到、也没有任务等着拣的请求不列：它的格都按 0 件记了短拣，没有货要分，
		// 列出来只会让这个波次永远显得「没分完」。它回到了已预占，等有货了再放。
		if !pending {
			continue
		}
		plan.Requests = append(plan.Requests, request)
	}
	return plan, true, nil
}
