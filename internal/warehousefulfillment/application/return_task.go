package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ReturnCommand 完成一条回库任务：货从待出库移回可分配库区。
type ReturnCommand struct {
	Tenant    domain.TenantID
	FactID    domain.PickFactID
	ReturnID  string
	Target    domain.LocationCode
	Warehouse domain.WarehouseCode
}

func (handler *WaveHandler) Return(ctx context.Context, command ReturnCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("return stock: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.FactID.String() == "" || command.ReturnID == "" || command.Target.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	task, found, err := handler.waves.LoadReturnTask(ctx, command.Tenant, command.ReturnID)
	if err != nil {
		return WaveResult{}, fmt.Errorf("return stock: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if task.Status == "DONE" {
		if task.Fact == command.FactID.String() {
			return WaveResult{outcome: WaveExisting}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, task.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("return stock: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if command.Warehouse.String() != "" && command.Warehouse != request.Warehouse() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	place, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, request.Warehouse(), command.Target)
	if err != nil || !found || place.Content().Deactivated() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, request.Warehouse(), place.Content().ZoneCode())
	if err != nil || !found || zone.Content().Deactivated() || !domain.AllocatableGoods(task.State, zone.Content().Usage()) {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	if err := handler.moveReturn(ctx, request, task, command); err != nil {
		return WaveResult{}, err
	}
	if err := handler.waves.CompleteReturn(ctx, command.Tenant, task.ID, command.FactID.String()); err != nil {
		return WaveResult{}, fmt.Errorf("return stock: %w", err)
	}
	open, err := handler.waves.OpenReturns(ctx, command.Tenant, task.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("return stock: %w", err)
	}
	if open == 0 && request.Status() == domain.OutboundAwaitingReturn {
		if err := handler.waves.SetRequestStatus(ctx, command.Tenant, task.Request, domain.OutboundCancelled); err != nil {
			return WaveResult{}, fmt.Errorf("return stock: %w", err)
		}
		// 截单的货都回了库，箱空出来。
		if err := handler.waves.ReleaseTotes(ctx, command.Tenant, task.Request, handler.clock.Now()); err != nil {
			return WaveResult{}, fmt.Errorf("return stock: %w", err)
		}
	}
	return WaveResult{outcome: WaveRecorded}, nil
}

func (handler *WaveHandler) moveReturn(ctx context.Context, request domain.OutboundRequest, task ports.ReturnTask, command ReturnCommand) error {
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, request.Owner(), request.Warehouse()); err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	rows, err := handler.ledger.ListOnHandCells(ctx, command.Tenant, request.Owner(), request.Warehouse())
	if err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	var source domain.StockCell
	found := false
	for _, row := range rows {
		if row.LocationCode != task.Location.String() || row.InboundBatchID != task.Inbound.String() || row.GoodsCode != task.Goods.String() || row.Quantity < task.Quantity {
			continue
		}
		source, err = cellFromBalance(row)
		if err != nil {
			return fmt.Errorf("return stock: %w", err)
		}
		found = true
		break
	}
	if !found {
		return fmt.Errorf("return stock: staged goods are gone")
	}
	dest, err := retarget(source, command.Target, task.Unit, task.State)
	if err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	out, err := stockLine(domain.StockOut, task.Quantity, source)
	if err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	in, err := stockLine(domain.StockIn, task.Quantity, dest)
	if err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	reference, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant:  command.Tenant,
		Request: domain.NewStockMovementRequest(request.Owner(), request.Warehouse(), domain.StockReasonInternalMove, reference, handler.clock.Now(), []domain.StockLineRequest{out, in}),
	})
	if err != nil {
		return fmt.Errorf("return stock: %w", err)
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return fmt.Errorf("return stock: movement was not posted")
	}
	return nil
}
