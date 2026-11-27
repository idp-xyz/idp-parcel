package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ShipPickedCommand 按已拣数量发货：一张请求少拣之后换不出别的库位，照货主「少发」的规则，拣到的发走，没拣到的不再发。
type ShipPickedCommand struct {
	Tenant   domain.TenantID
	Request  domain.OutboundRequestID
	Operator domain.Registrant
}

// ShipPicked 只在货主许了少发时成立：规则没写明答部分发货未配置，写的是整单发或拆单发答不许少发——
// 少发是替货主决定对他的客户少履约，操作员只在货主许的范围里执行。成立之后没拣到的预占放开、请求进已拣货，
// 往下复核、打包、出库确认都按已拣数量走；当时照的是哪一层的规则、要发与实发各多少一并记下。
func (handler *WaveHandler) ShipPicked(ctx context.Context, command ShipPickedCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.Request.String() == "" || command.Operator.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	// 拣货入账排在同一道次序里：拿到之后再读请求与任务，读到的已拣数量不会再被一笔在途的拣货改掉。
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, request.Owner(), request.Warehouse()); err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	disposal, found, err := handler.waves.LoadShortPickDisposal(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	if found {
		if disposal.Rule == domain.PartialShipmentShortShip.String() {
			return WaveResult{outcome: WaveExisting}, nil
		}
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	request, found, err = handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	tasks, err := handler.waves.TasksOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	if !AwaitsShortPickDisposal(request.Status(), tasks) {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	// 放波次时预取过估算委托的请求照办：那份委托按全量申报了件数，少发之后打包时按实际装的件数提货物明细修订
	// （CommissionHandler 的 revisePrefetch），面单号不变。只有早先预取、没留下申报件数的不办——比不出差了多少，
	// 办了就是以货主名义交出去一份件数不对的委托。它等有货了再放，或截单（截单会把那份委托撤回）。
	if record, prefetched, err := handler.waves.LoadPrefetch(ctx, command.Tenant, command.Request); err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	} else if prefetched && record.Quantity == 0 {
		return WaveResult{outcome: WavePrefetchStands}, nil
	}
	rule, source, err := partialShipmentOf(ctx, handler.requests, command.Tenant, request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	if !rule.Given() {
		return WaveResult{outcome: WavePartialShipmentNotConfigured}, nil
	}
	if rule != domain.PartialShipmentShortShip {
		return WaveResult{outcome: WavePartialShipmentNotAllowed}, nil
	}
	wanted, shipped := shortPickQuantities(request, tasks)
	if shipped <= 0 || shipped >= wanted {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if err := handler.waves.SaveShortPickDisposal(ctx, command.Tenant, ports.ShortPickDisposal{
		Request: command.Request, Rule: rule.String(), Source: source.String(), Wanted: wanted, Shipped: shipped,
		Operator: command.Operator.String(), RecordedAt: handler.clock.Now(),
	}); err != nil {
		return WaveResult{}, fmt.Errorf("ship picked: %w", err)
	}
	return WaveResult{outcome: WaveShortShipped}, nil
}

// shortPickQuantities 交回一张请求要发与已拣的数量，两个都按基本单位。要发取预占行上的需求合计，不取请求行的数量：
// 请求行按下单的单位写，按组件出的套装也没展开，与按基本单位记的已拣数量不可比——3 套各 2 件拣到 4 件，
// 是要发 6 拣到 4，不是要发 3 拣到 4。
func shortPickQuantities(request domain.OutboundRequest, tasks []ports.PickTask) (wanted, shipped int64) {
	for _, reserved := range request.Reservations() {
		wanted += reserved.Wanted
	}
	return wanted, pickedQuantity(tasks)
}

// AwaitsShortPickDisposal 看一张请求是不是停在「拣不齐、等处置」：它是已预占，任务都有了结果（没有一条待执行），
// 并且已有货拣进了待出库。少拣之后换不出别的库位的请求就停在这里；从波次撤出的、还没放过波次的没有拣过的货，不算。
// tasks 是这张请求的全部任务。
func AwaitsShortPickDisposal(status domain.OutboundStatus, tasks []ports.PickTask) bool {
	if status != domain.OutboundReserved {
		return false
	}
	for _, task := range tasks {
		if task.Status == "OPEN" {
			return false
		}
	}
	return pickedQuantity(tasks) > 0
}
