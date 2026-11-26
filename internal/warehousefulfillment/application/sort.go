package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// SortCommand 是分拣时的一次投放：合并拣货的波次拿回来的货里，把这张出库请求的这种货放进它的周转箱。
// 数量按基本单位，与拣货任务同口径。
type SortCommand struct {
	Tenant    domain.TenantID
	FactID    domain.PickFactID
	Wave      domain.WaveID
	Request   domain.OutboundRequestID
	Goods     domain.GoodsCode
	Quantity  int64
	Tote      domain.ToteCode
	Warehouse domain.WarehouseCode
}

// Sort 记下一次投放。它不动库存：货在拣货时已经记进待出库库位，分进箱里没有换库位。它只让「这张请求拣出来的货
// 分好了没有」答得出——分完才能复核。
//
// 投放的件数不能超过这张请求的这种货在这个波次里已经拣出来而还没投放的：拣货的人还没拿回来的，分拣台上没有。
// 箱照拣货绑箱的规矩绑到这张请求上：没登记、已停用、绑着别的请求的箱，这一笔整份不成立，货不该放进去。
// 一张请求可以用几个箱。箱到打完包才放开。
func (handler *WaveHandler) Sort(ctx context.Context, command SortCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.Quantity < 1 || command.Tote.String() == "" || command.Goods.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	// 与拣货排在同一道次序里：同一张请求的两次投放、或投放与拣货同时来时，各自读到的已拣与已投放才不是旧的。
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, request.Owner(), request.Warehouse()); err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	existing, found, err := handler.waves.LoadSortPut(ctx, command.Tenant, command.FactID.String())
	if err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	if found {
		if existing.Wave == command.Wave && existing.Request == command.Request && existing.Goods == command.Goods &&
			existing.Quantity == command.Quantity && existing.Tote == command.Tote {
			return WaveResult{outcome: WaveExisting}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	sorts, found, err := handler.waves.WaveSorts(ctx, command.Tenant, command.Wave)
	if err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if !sorts || (command.Warehouse.String() != "" && command.Warehouse != request.Warehouse()) {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	request, found, err = handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil || !found {
		return WaveResult{}, fmt.Errorf("sort: reload request: %w", err)
	}
	// 取消的请求拣出来的货走回库，复核过的请求已经分完：都不再往箱里分。
	switch request.Status() {
	case domain.OutboundAllocated, domain.OutboundReserved, domain.OutboundPicked:
	default:
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	tasks, err := handler.waves.TasksOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	sorted, err := handler.waves.SortedOf(ctx, command.Tenant, command.Wave, command.Request, command.Goods)
	if err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	if sorted+command.Quantity > pickedInWave(tasks, command.Wave, command.Goods) {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	refused, err := handler.bindTote(ctx, command.Tenant, request, command.Tote)
	if err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	if refused != WaveOutcomeInvalid {
		return WaveResult{outcome: refused}, nil
	}
	if err := handler.waves.SaveSortPut(ctx, command.Tenant, ports.SortPut{
		FactID: command.FactID.String(), Wave: command.Wave, Request: command.Request, Goods: command.Goods,
		Quantity: command.Quantity, Warehouse: request.Warehouse(), Tote: command.Tote, RecordedAt: handler.clock.Now(),
	}); err != nil {
		return WaveResult{}, fmt.Errorf("sort: %w", err)
	}
	return WaveResult{outcome: WaveSorted}, nil
}

// pickedInWave 是这张请求的这种货在这个波次里拣出来的件数：只数拣满与少拣的任务。
// 作废之后才拣出来的（迟到）不算，那几件走回库。
func pickedInWave(tasks []ports.PickTask, wave domain.WaveID, goods domain.GoodsCode) int64 {
	var picked int64
	for _, task := range tasks {
		if task.WaveID == wave && task.Goods == goods && (task.Status == "DONE" || task.Status == "SHORT") {
			picked += task.Picked
		}
	}
	return picked
}
