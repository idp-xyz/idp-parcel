package application

import (
	"context"
	"math"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// reallocateShort 把一条任务少拣的数量当场在同一个波次里换别的格再分配，交回添出来的待执行任务。
// 这张请求不因为一格少了就退出波次：它别的任务照常拣，少的这几件换一格接着拣，拣货的人不必等人重新放波次。
// 少拣的那个库位不再考虑——账上说它还有，手上刚确认过没有；这张请求先前少拣过这种货的库位同样不考虑，
// 不然换出来的任务会绕回刚确认过没货的地方。避开的是库位，不只是那一格：同一个库位上另一批入库的同种货，
// 拣货的人站在那里也一并看过了。别的格照放波次时同一套次序挑，扣掉冻结、盘点锁定与已经分给待执行任务的数量。
// 分不出、或只分得出一部分时，没分出的那部分留在预占里，等这张请求的任务都有了结果再说。
// 调用方已经拿到这位货主在这个仓的入账次序。siblings 是这张请求的全部任务。
func (handler *WaveHandler) reallocateShort(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequest, task ports.PickTask, siblings []ports.PickTask, short int64) ([]ports.PickTask, error) {
	rows, err := handler.ledger.ListOnHandCells(ctx, tenant, request.Owner(), request.Warehouse())
	if err != nil {
		return nil, err
	}
	reserved, found := reservationOfTask(request.Reservations(), task, rows)
	if !found {
		return nil, nil
	}
	emptied := map[string]struct{}{task.Location.String(): {}}
	for _, sibling := range siblings {
		if sibling.Status == "SHORT" && sibling.Goods == task.Goods {
			emptied[sibling.Location.String()] = struct{}{}
		}
	}
	avoided := map[string]int64{}
	for _, row := range rows {
		if _, skip := emptied[row.LocationCode]; skip && row.GoodsCode == task.Goods.String() {
			avoided[row.LocationCode+"\x1f"+row.GoodsCode+"\x1f"+row.Unit+"\x1f"+row.State+"\x1f"+textOrEmpty(row.Alias)+"\x1f"+row.InboundBatchID] = math.MaxInt64 / 2
		}
	}
	command := ReleaseWaveCommand{Tenant: tenant, ID: task.WaveID, Warehouse: request.Warehouse()}
	cells, err := handler.rankedCells(ctx, command, request, rows, reserved, avoided)
	if err != nil {
		return nil, err
	}
	var added []ports.PickTask
	need := short
	for _, cell := range cells {
		if need == 0 {
			break
		}
		take := cell.free
		if take > need {
			take = need
		}
		if take <= 0 {
			continue
		}
		id, err := handler.tasks.NextPickTaskID()
		if err != nil {
			return nil, err
		}
		added = append(added, ports.PickTask{
			ID: id, WaveID: task.WaveID, Request: task.Request, Location: cell.location, Goods: task.Goods,
			Unit: cell.unit, State: task.State, Alias: cell.alias, Inbound: cell.inbound, Quantity: take, Status: "OPEN",
		})
		need -= take
	}
	return added, nil
}

// reservationOfTask 认出一条任务是替哪一行预占拣的：商品、库存状态与标签别名相同。同一种货有几行预占、各指了不同的
// 批次或效期时，取任务那一格满足的那一行；那一格的账已经不在了就取头一行相同的货。
func reservationOfTask(reservations []domain.OutboundReservation, task ports.PickTask, rows []ports.StockBalanceRow) (domain.OutboundReservation, bool) {
	var batch, expiry string
	for _, row := range rows {
		if row.LocationCode == task.Location.String() && row.InboundBatchID == task.Inbound.String() && row.GoodsCode == task.Goods.String() &&
			row.Unit == task.Unit.String() && row.State == task.State.String() && textOrEmpty(row.Alias) == task.Alias.String() {
			batch, expiry = textOrEmpty(row.BatchNumber), textOrEmpty(row.Expiry)
			break
		}
	}
	var fallback domain.OutboundReservation
	matched := false
	for _, reserved := range reservations {
		if reserved.Goods != task.Goods || reserved.State != task.State || reserved.Alias != task.Alias {
			continue
		}
		if (!reserved.Batch.Given() || reserved.Batch.String() == batch) && (!reserved.Expiry.Given() || reserved.Expiry.String() == expiry) {
			return reserved, true
		}
		if !matched {
			fallback, matched = reserved, true
		}
	}
	return fallback, matched
}
