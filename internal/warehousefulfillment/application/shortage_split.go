package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// shortPickStore 是出库请求这一侧看得到的拣货结果：一张请求拣了多少、拣不齐的处置办过没有、预取过没有。
type shortPickStore interface {
	TasksOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PickTask, error)
	LoadShortPickDisposal(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (ports.ShortPickDisposal, bool, error)
	SaveShortPickDisposal(ctx context.Context, tenant domain.TenantID, disposal ports.ShortPickDisposal) error
	ReleaseReservations(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) error
	LoadPrefetch(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (ports.PrefetchRecord, bool, error)
}

// WithShortPicks 接上拣货结果。没接时拆出缺货部分答状态不符，拆单也不看有没有货已经拣出来。
func (handler *OutboundHandler) WithShortPicks(picks shortPickStore) *OutboundHandler {
	handler.shortPicks = picks
	return handler
}

// SplitShortageCommand 拆出缺货部分：一张请求少拣之后换不出别的库位，照货主「拆单发」的规则，拣到的发走，
// 没拣到的拆成另一张请求继续等货。拆多少由没拣到的预占定，不由这一次提交说。
type SplitShortageCommand struct {
	Tenant   domain.TenantID
	Request  domain.OutboundRequestID
	Operator domain.Registrant
}

// SplitShortage 只在货主许了拆单发时成立，口径与 ShipPicked 对称：规则没写明答部分发货未配置，写的是整单发或少发答不许。
// 成立之后原请求没拣到的预占放开、进已拣货，往下按已拣数量走；没拣到的那部分另成一张请求，重新受理、重新预占，
// 库存不够时照货主「受理时库存不够怎么办」那一格办。交回的 Remainder 是拆出的那一张。
func (handler *OutboundHandler) SplitShortage(ctx context.Context, command SplitShortageCommand) (SplitResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	if !configured {
		return SplitResult{outcome: OutboundWarehouseNotFound}, nil
	}
	if command.Request.String() == "" || command.Operator.String() == "" {
		return SplitResult{outcome: OutboundContentInvalid}, nil
	}
	parent, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	if !found {
		return SplitResult{outcome: OutboundNotFound}, nil
	}
	if handler.shortPicks == nil {
		return SplitResult{outcome: OutboundStatusMismatch, parent: parent, has: true}, nil
	}
	// 拣货入账排在同一道次序里：拿到之后再读请求、任务与预占，读到的不会再被一笔在途的拣货改掉。
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, parent.Owner(), parent.Warehouse()); err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	parent, found, err = handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	if !found {
		return SplitResult{outcome: OutboundNotFound}, nil
	}
	disposal, found, err := handler.shortPicks.LoadShortPickDisposal(ctx, command.Tenant, command.Request)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	if found {
		if disposal.Rule != domain.PartialShipmentSplitShip.String() {
			return SplitResult{outcome: OutboundStatusMismatch, parent: parent, has: true}, nil
		}
		child, found, err := handler.remainderOf(ctx, command.Tenant, disposal)
		if err != nil {
			return SplitResult{}, fmt.Errorf("split shortage: %w", err)
		}
		return SplitResult{outcome: OutboundExisting, parent: parent, has: true, remainder: child, hasRemainder: found}, nil
	}
	tasks, err := handler.shortPicks.TasksOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	if !AwaitsShortPickDisposal(parent.Status(), tasks) {
		return SplitResult{outcome: OutboundStatusMismatch, parent: parent, has: true}, nil
	}
	// 预取过估算委托的请求照办，口径同 ShipPicked：原请求照样是少发，打包时按实际装的件数提货物明细修订。
	// 只有早先预取、没留下申报件数的不办。
	if record, prefetched, err := handler.shortPicks.LoadPrefetch(ctx, command.Tenant, command.Request); err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	} else if prefetched && record.Quantity == 0 {
		return SplitResult{outcome: OutboundPrefetchStands, parent: parent, has: true}, nil
	}
	rule, source, err := partialShipmentOf(ctx, handler.requests, command.Tenant, parent)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	if !rule.Given() {
		return SplitResult{outcome: OutboundPartialShipmentNotConfigured, parent: parent, has: true}, nil
	}
	if rule != domain.PartialShipmentSplitShip {
		return SplitResult{outcome: OutboundPartialShipmentNotAllowed, parent: parent, has: true}, nil
	}
	wanted, shipped := shortPickQuantities(parent, tasks)
	if shipped <= 0 || shipped >= wanted {
		return SplitResult{outcome: OutboundStatusMismatch, parent: parent, has: true}, nil
	}
	lines, ok, err := handler.remainderLines(ctx, command.Tenant, parent)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	if !ok {
		return SplitResult{outcome: OutboundContentInvalid, parent: parent, has: true}, nil
	}
	key, err := domain.NewRequestKey(shortageRemainderKey + parent.ID().String())
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	said, err := handler.saidAboutOrder(ctx, command.Tenant, parent)
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	// 先放开原请求没拣到的预占，新请求才占得回这批货。往下任何一步不成，整笔回滚，不留一张没了预占的原请求。
	if err := handler.shortPicks.ReleaseReservations(ctx, command.Tenant, parent.ID()); err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	opened, err := handler.Accept(ctx, AcceptOutboundCommand{
		Tenant: command.Tenant, Owner: parent.Owner(), Warehouse: parent.Warehouse(), Origin: domain.OutboundOriginOperator,
		Key: key, Dispatch: parent.Dispatch(), Service: parent.Service(), Recipient: parent.Recipient(), Address: parent.Address(),
		Basis: shortageRemainderBasis + parent.ID().String(), Operator: command.Operator, Lines: lines, Parent: parent.ID(),
		PlatformStocking: parent.PlatformStocking(), PartialShipment: said,
	})
	if err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	child, saved := opened.Request()
	switch opened.Outcome() {
	case OutboundReserved, OutboundPartiallyReserved, OutboundPendingPolicy, OutboundRejected:
	default:
		saved = false
	}
	if !saved {
		return SplitResult{}, fmt.Errorf("split shortage: 缺货部分没有落册（%s）", opened.Outcome())
	}
	if err := handler.shortPicks.SaveShortPickDisposal(ctx, command.Tenant, ports.ShortPickDisposal{
		Request: parent.ID(), Rule: rule.String(), Source: source.String(), Wanted: wanted, Shipped: shipped,
		Remainder: child.ID().String(), Operator: command.Operator.String(), RecordedAt: handler.clock.Now(),
	}); err != nil {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	parent, found, err = handler.requests.LoadByID(ctx, command.Tenant, parent.ID())
	if err != nil || !found {
		return SplitResult{}, fmt.Errorf("split shortage: %w", err)
	}
	return SplitResult{outcome: OutboundRemainderSplit, parent: parent, has: true, remainder: child, hasRemainder: true}, nil
}

const (
	// shortageRemainderKey 给拆出的请求起键：一张请求只拆一次，原请求的标识就够它不重。
	shortageRemainderKey = "SHORTAGE-REMAINDER-"
	// shortageRemainderBasis 是拆出的请求上「代录依据」那一格的开头，后面跟原请求的标识。
	shortageRemainderBasis = "缺货部分拆自 "
)

// remainderOf 读回一次拆单发处置拆出的那张请求。
func (handler *OutboundHandler) remainderOf(ctx context.Context, tenant domain.TenantID, disposal ports.ShortPickDisposal) (domain.OutboundRequest, bool, error) {
	id, err := domain.NewOutboundRequestID(disposal.Remainder)
	if err != nil {
		return domain.OutboundRequest{}, false, err
	}
	return handler.requests.LoadByID(ctx, tenant, id)
}

// remainderLines 把原请求还没拣到的预占写成新请求的行，一行预占一行，数量就是没拣到的数。
// 行按基本单位写、组件出的套装写成缺的那几种组件：预占与拣货都按基本单位记，缺的是几件就是几件——
// 原请求按箱下、只拣了半箱时，缺的写不成「半箱」；套装缺了其中一种组件时，缺的也写不成「几套」。
// 交回 false 是有一种货在册上认不出基本单位，这时不拆。
func (handler *OutboundHandler) remainderLines(ctx context.Context, tenant domain.TenantID, parent domain.OutboundRequest) ([]domain.OutboundLine, bool, error) {
	var lines []domain.OutboundLine
	for _, reserved := range parent.Reservations() {
		if reserved.Quantity <= 0 {
			continue
		}
		revision, found, err := handler.goods.LoadLatestGoods(ctx, tenant, parent.Owner(), reserved.Goods)
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, nil
		}
		unit := revision.Content().BaseUnit()
		if factor, ok := unitFactor(revision, unit); !ok || factor != 1 {
			return nil, false, nil
		}
		line, err := domain.NewOutboundLine(domain.OutboundLineSpec{
			Goods: reserved.Goods, Unit: unit, State: reserved.State, Alias: reserved.Alias,
			Batch: reserved.Batch, Expiry: reserved.Expiry, Quantity: reserved.Quantity,
		})
		if err != nil {
			return nil, false, err
		}
		lines = append(lines, line)
	}
	return lines, len(lines) > 0, nil
}

// saidAboutOrder 是货主对这一张请求说过的部分发货规则：受理之后代录的同意盖过提交时带的。
// 货主的默认不算在内——那不是对这一张说的。拆出的请求按它记，货主对这张单的交代跟着货走。
func (handler *OutboundHandler) saidAboutOrder(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequest) (domain.PartialShipment, error) {
	consent, consented, err := handler.requests.LatestConsent(ctx, tenant, request.ID())
	if err != nil {
		return domain.PartialShipmentUnset, err
	}
	if consented {
		return consent.Rule, nil
	}
	return request.PartialShipment(), nil
}
