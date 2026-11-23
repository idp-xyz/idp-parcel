package application

import (
	"context"
	"fmt"
	"math"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// OutboundOutcome 是受理出库请求或登记缺货处置的答案。
type OutboundOutcome uint8

const (
	OutboundOutcomeInvalid OutboundOutcome = iota
	OutboundReserved
	OutboundPartiallyReserved
	OutboundRejected
	OutboundPendingPolicy
	OutboundHandedOff
	OutboundExisting
	OutboundConflict
	OutboundWarehouseNotFound
	OutboundContentInvalid
	OutboundNumberingCollision
	OutboundPolicyRegistered
	OutboundSplitResult
	OutboundStatusMismatch
	OutboundNotFound
)

func (outcome OutboundOutcome) String() string {
	switch outcome {
	case OutboundReserved:
		return "RESERVED"
	case OutboundPartiallyReserved:
		return "PARTIALLY_RESERVED"
	case OutboundRejected:
		return "REJECTED"
	case OutboundPendingPolicy:
		return "NOT_CONFIGURED"
	case OutboundHandedOff:
		return "HANDED_OFF"
	case OutboundExisting:
		return "EXISTING_RESULT"
	case OutboundConflict:
		return "CONTENT_CONFLICT"
	case OutboundWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case OutboundContentInvalid:
		return "CONTENT_INVALID"
	case OutboundNumberingCollision:
		return "NUMBERING_RULE_COLLISION"
	case OutboundPolicyRegistered:
		return "REGISTERED"
	case OutboundSplitResult:
		return "SPLIT"
	case OutboundStatusMismatch:
		return "STATUS_MISMATCH"
	case OutboundNotFound:
		return "OUTBOUND_NOT_FOUND"
	default:
		return ""
	}
}

// OutboundResult 是一次受理或登记的结果。Request 只在落了册或重放时有。
type OutboundResult struct {
	outcome OutboundOutcome
	request domain.OutboundRequest
	has     bool
}

func (result OutboundResult) Outcome() OutboundOutcome { return result.outcome }
func (result OutboundResult) Request() (domain.OutboundRequest, bool) {
	return result.request, result.has
}

// AcceptOutboundCommand 是货主提交或运营代录一份出库请求。货主口的货主取自信封。
type AcceptOutboundCommand struct {
	Tenant    domain.TenantID
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	Origin    domain.OutboundOrigin
	Key       domain.RequestKey
	Dispatch  domain.DispatchMethod
	Service   string
	Recipient string
	Address   string
	Basis     string
	Operator  domain.Registrant
	Lines     []domain.OutboundLine
	Parent    domain.OutboundRequestID
}

// RegisterShortagePolicyCommand 登记某个货主的缺货处置。修订只追加。
type RegisterShortagePolicyCommand struct {
	Tenant   domain.TenantID
	Owner    domain.OwnerAccountID
	Revision int
	Kind     domain.ShortagePolicyKind
	Operator domain.Registrant
}

// OutboundHandler 受理出库请求并按可用量预占。预占不写库存流水。
type OutboundHandler struct {
	requests   ports.OutboundStore
	identities ports.OutboundIdentities
	numbers    ports.OutboundNumberIssuer
	ledger     interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
	}
	warehouses ports.WarehouseLookup
	locations  ports.StockLocationLookup
	zones      interface {
		LoadLatestZone(context.Context, domain.TenantID, domain.WarehouseCode, domain.ZoneCode) (domain.ZoneRevision, bool, error)
	}
	goods        ports.StockGoodsLookup
	holds        ports.StockHolds
	owners       ports.OwnerAccountLookup
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
}

func NewOutboundHandler(
	requests ports.OutboundStore,
	identities ports.OutboundIdentities,
	numbers ports.OutboundNumberIssuer,
	ledger interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
	},
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones interface {
		LoadLatestZone(context.Context, domain.TenantID, domain.WarehouseCode, domain.ZoneCode) (domain.ZoneRevision, bool, error)
	},
	goods ports.StockGoodsLookup,
	holds ports.StockHolds,
	owners ports.OwnerAccountLookup,
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
) *OutboundHandler {
	return &OutboundHandler{
		requests: requests, identities: identities, numbers: numbers, ledger: ledger, warehouses: warehouses,
		locations: locations, zones: zones, goods: goods, holds: holds, owners: owners, clock: clock, anyWarehouse: anyWarehouse,
	}
}

func (handler *OutboundHandler) Accept(ctx context.Context, command AcceptOutboundCommand) (OutboundResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	if !configured {
		return OutboundResult{outcome: OutboundWarehouseNotFound}, nil
	}
	if command.Origin == domain.OutboundOriginOperator {
		registered, err := handler.owners.OwnerAccountRegistered(ctx, command.Tenant, command.Owner)
		if err != nil {
			return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
		}
		if !registered {
			return OutboundResult{}, ErrOwnerAccountNotInTenant
		}
		if command.Basis == "" || command.Operator.String() == "" {
			return OutboundResult{outcome: OutboundContentInvalid}, nil
		}
	}
	if command.Origin == domain.OutboundOriginCustomer && (command.Basis != "" || command.Operator.String() != "") {
		return OutboundResult{outcome: OutboundContentInvalid}, nil
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	if !found {
		return OutboundResult{outcome: OutboundWarehouseNotFound}, nil
	}
	if !commandComplete(command) {
		return OutboundResult{outcome: OutboundContentInvalid}, nil
	}
	existing, found, err := handler.requests.LoadByKey(ctx, command.Tenant, command.Origin, command.Key)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	if found {
		if sameOutboundSubmission(existing, command) {
			return OutboundResult{outcome: OutboundExisting, request: existing, has: true}, nil
		}
		return OutboundResult{outcome: OutboundConflict, request: existing, has: true}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return handler.save(ctx, command, warehouse, domain.OutboundHandedOff, nil)
	}
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, command.Owner, command.Warehouse); err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	expanded, ok, err := handler.expand(ctx, command)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	if !ok {
		return OutboundResult{outcome: OutboundContentInvalid}, nil
	}
	planned, err := handler.plan(ctx, command, expanded)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	status := domain.OutboundReserved
	short := false
	for _, item := range planned {
		if item.Quantity < item.Wanted {
			short = true
			break
		}
	}
	if short {
		policy, found, err := handler.requests.LoadPolicy(ctx, command.Tenant, command.Owner)
		if err != nil {
			return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
		}
		if !found {
			status = domain.OutboundPendingPolicy
			planned = nil
		} else if policy.Kind == domain.ShortagePolicyReject {
			status = domain.OutboundRejected
			planned = nil
		} else {
			status = domain.OutboundPartiallyReserved
		}
	}
	return handler.save(ctx, command, warehouse, status, planned)
}

func (handler *OutboundHandler) RegisterPolicy(ctx context.Context, command RegisterShortagePolicyCommand) (OutboundResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("register shortage policy: %w", err)
	}
	if !configured {
		return OutboundResult{outcome: OutboundWarehouseNotFound}, nil
	}
	if command.Revision < 1 || command.Kind == domain.ShortagePolicyInvalid || command.Operator.String() == "" {
		return OutboundResult{outcome: OutboundContentInvalid}, nil
	}
	current, found, err := handler.requests.LoadPolicy(ctx, command.Tenant, command.Owner)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("register shortage policy: %w", err)
	}
	if found && current.Revision == command.Revision && current.Kind == command.Kind {
		return OutboundResult{outcome: OutboundExisting}, nil
	}
	if found && command.Revision != current.Revision+1 {
		return OutboundResult{outcome: OutboundConflict}, nil
	}
	if err := handler.requests.SavePolicy(ctx, command.Tenant, command.Owner, ports.StoredShortagePolicy{
		Revision: command.Revision, Kind: command.Kind, Operator: command.Operator.String(), RegisteredAt: handler.clock.Now(),
	}); err != nil {
		return OutboundResult{}, fmt.Errorf("register shortage policy: %w", err)
	}
	return OutboundResult{outcome: OutboundPolicyRegistered}, nil
}

func (handler *OutboundHandler) save(
	ctx context.Context,
	command AcceptOutboundCommand,
	warehouse domain.WarehouseRevision,
	status domain.OutboundStatus,
	planned []domain.OutboundReservation,
) (OutboundResult, error) {
	issued, err := handler.numbers.IssueOutboundNumber(ctx, command.Tenant, warehouse.Content().BusinessTimeZone())
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	number, absence, collided, err := numberFrom(issued)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	if collided {
		return OutboundResult{outcome: OutboundNumberingCollision}, nil
	}
	id, err := handler.identities.NextOutboundRequestID()
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	request, err := domain.NewOutboundRequest(domain.OutboundRequestSpec{
		Tenant: command.Tenant, ID: id, Owner: command.Owner, Warehouse: command.Warehouse, Origin: command.Origin,
		Key: command.Key, Dispatch: command.Dispatch, Service: command.Service, Recipient: command.Recipient,
		Address: command.Address, Basis: command.Basis, Operator: command.Operator.String(), Status: status,
		Parent: command.Parent, Number: number, Absence: absence, Lines: command.Lines, Reservations: planned,
		ReceivedAt: handler.clock.Now(),
	})
	if err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	if err := handler.requests.Save(ctx, request); err != nil {
		return OutboundResult{}, fmt.Errorf("accept outbound: %w", err)
	}
	return OutboundResult{outcome: statusOutcome(status), request: request, has: true}, nil
}

func statusOutcome(status domain.OutboundStatus) OutboundOutcome {
	switch status {
	case domain.OutboundReserved:
		return OutboundReserved
	case domain.OutboundPartiallyReserved:
		return OutboundPartiallyReserved
	case domain.OutboundRejected:
		return OutboundRejected
	case domain.OutboundPendingPolicy:
		return OutboundPendingPolicy
	case domain.OutboundHandedOff:
		return OutboundHandedOff
	default:
		return OutboundOutcomeInvalid
	}
}

func commandComplete(command AcceptOutboundCommand) bool {
	if command.Tenant.String() == "" || command.Owner.String() == "" || command.Warehouse.String() == "" ||
		command.Key.String() == "" || command.Dispatch == domain.DispatchMethodInvalid ||
		command.Origin == domain.OutboundOriginInvalid || command.Recipient == "" || command.Address == "" ||
		len(command.Lines) == 0 {
		return false
	}
	if command.Dispatch == domain.DispatchProductNetwork && command.Service == "" {
		return false
	}
	return true
}

func sameOutboundSubmission(existing domain.OutboundRequest, command AcceptOutboundCommand) bool {
	probe, err := domain.NewOutboundRequest(domain.OutboundRequestSpec{
		Tenant: existing.Tenant(), ID: existing.ID(), Owner: existing.Owner(), Warehouse: command.Warehouse,
		Origin: existing.Origin(), Key: existing.Key(), Dispatch: command.Dispatch, Service: command.Service,
		Recipient: command.Recipient, Address: command.Address, Status: existing.Status(), Lines: command.Lines,
		ReceivedAt: existing.ReceivedAt(),
	})
	if err != nil {
		return false
	}
	return existing.SameSubmission(probe)
}

type expandedDemand struct {
	goods  domain.GoodsCode
	state  domain.StockState
	alias  domain.GoodsAliasCode
	batch  domain.BatchNumber
	expiry domain.ExpiryDate
	wanted int64
}

func (handler *OutboundHandler) expand(ctx context.Context, command AcceptOutboundCommand) ([]expandedDemand, bool, error) {
	var demands []expandedDemand
	for _, line := range command.Lines {
		revision, found, err := handler.goods.LoadLatestGoods(ctx, command.Tenant, command.Owner, line.Goods())
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, nil
		}
		factor, ok := unitFactor(revision, line.Unit())
		if !ok {
			return nil, false, nil
		}
		base, ok := scaleQuantity(line.Quantity(), factor)
		if !ok {
			return nil, false, fmt.Errorf("accept outbound: 数量溢出")
		}
		mode, given := revision.Content().OutboundMode()
		if revision.Content().Composite() && given && mode == domain.GoodsOutboundByComponent {
			if len(revision.Content().Components()) == 0 {
				return nil, false, nil
			}
			for _, component := range revision.Content().Components() {
				wanted, ok := scaleQuantity(base, component.Quantity())
				if !ok || component.Quantity() <= 0 {
					return nil, false, nil
				}
				demands = append(demands, expandedDemand{
					goods: component.Code(), state: line.State(), wanted: wanted,
				})
			}
			continue
		}
		demands = append(demands, expandedDemand{
			goods: line.Goods(), state: line.State(), alias: line.Alias(), batch: line.Batch(), expiry: line.Expiry(), wanted: base,
		})
	}
	return demands, true, nil
}

func (handler *OutboundHandler) plan(ctx context.Context, command AcceptOutboundCommand, demands []expandedDemand) ([]domain.OutboundReservation, error) {
	rows, err := handler.ledger.ListOnHandCells(ctx, command.Tenant, command.Owner, command.Warehouse)
	if err != nil {
		return nil, err
	}
	existing, err := handler.requests.ListReservations(ctx, command.Tenant, command.Owner, command.Warehouse)
	if err != nil {
		return nil, err
	}
	var planned []domain.OutboundReservation
	for _, demand := range demands {
		labeled, unlabeled, err := handler.pool(ctx, command, rows, demand)
		if err != nil {
			return nil, err
		}
		booked := append(existing, plannedAsStored(planned)...)
		labeledUsed, unlabeledUsed := usedOf(booked, demand)
		if demand.batch.Given() || demand.expiry.Given() {
			open := demand
			open.batch = domain.BatchNumber{}
			open.expiry = domain.ExpiryDate{}
			allLabeled, allUnlabeled, err := handler.pool(ctx, command, rows, open)
			if err != nil {
				return nil, err
			}
			// 没点名的预占可以落到任一小批。点名这一格时，只再扣别的格子装不下的部分，总数才不会超过在库。
			unnamedLabeled, unnamedUnlabeled := usedOf(unnamedReservations(booked), open)
			labeledUsed += overflowOnto(unnamedLabeled, allLabeled-labeled)
			unlabeledUsed += overflowOnto(unnamedUnlabeled, allUnlabeled-unlabeled)
		}
		var takeLabeled, takeUnlabeled int64
		if demand.alias.String() == "" {
			room := unlabeled - unlabeledUsed
			if room < 0 {
				room = 0
			}
			takeUnlabeled = demand.wanted
			if takeUnlabeled > room {
				takeUnlabeled = room
			}
		} else {
			room := labeled - labeledUsed
			if room < 0 {
				room = 0
			}
			takeLabeled = demand.wanted
			if takeLabeled > room {
				takeLabeled = room
			}
			rest := demand.wanted - takeLabeled
			unlabeledRoom := unlabeled - unlabeledUsed
			if unlabeledRoom < 0 {
				unlabeledRoom = 0
			}
			takeUnlabeled = rest
			if takeUnlabeled > unlabeledRoom {
				takeUnlabeled = unlabeledRoom
			}
		}
		planned = append(planned, domain.OutboundReservation{
			Goods: demand.goods, State: demand.state, Alias: demand.alias, Batch: demand.batch, Expiry: demand.expiry,
			Quantity: takeLabeled + takeUnlabeled, FromUnlabeled: takeUnlabeled, Wanted: demand.wanted,
		})
	}
	return planned, nil
}

func plannedAsStored(planned []domain.OutboundReservation) []ports.StoredReservation {
	stored := make([]ports.StoredReservation, 0, len(planned))
	for _, item := range planned {
		stored = append(stored, ports.StoredReservation{
			Goods: item.Goods, State: item.State, Alias: item.Alias, Batch: item.Batch, Expiry: item.Expiry,
			Quantity: item.Quantity, FromUnlabeled: item.FromUnlabeled,
		})
	}
	return stored
}

func unnamedReservations(rows []ports.StoredReservation) []ports.StoredReservation {
	var unnamed []ports.StoredReservation
	for _, row := range rows {
		if row.Batch.Given() || row.Expiry.Given() {
			continue
		}
		unnamed = append(unnamed, row)
	}
	return unnamed
}

func overflowOnto(claimed, elsewhere int64) int64 {
	if elsewhere < 0 {
		elsewhere = 0
	}
	if claimed <= elsewhere {
		return 0
	}
	return claimed - elsewhere
}

func usedOf(rows []ports.StoredReservation, demand expandedDemand) (labeled, unlabeled int64) {
	for _, row := range rows {
		if row.Goods != demand.goods || row.State != demand.state {
			continue
		}
		if (demand.batch.Given() || demand.expiry.Given()) && (row.Batch != demand.batch || row.Expiry != demand.expiry) {
			continue
		}
		unlabeled += row.FromUnlabeled
		if demand.alias.String() != "" && row.Alias == demand.alias {
			labeled += row.Quantity - row.FromUnlabeled
		}
	}
	return labeled, unlabeled
}

func (handler *OutboundHandler) pool(ctx context.Context, command AcceptOutboundCommand, rows []ports.StockBalanceRow, demand expandedDemand) (labeled, unlabeled int64, err error) {
	revision, found, err := handler.goods.LoadLatestGoods(ctx, command.Tenant, command.Owner, demand.goods)
	if err != nil || !found {
		return 0, 0, err
	}
	zones := map[string]domain.ZoneUsage{}
	for _, row := range rows {
		if row.GoodsCode != demand.goods.String() || row.State != demand.state.String() {
			continue
		}
		// 没点名批号或效期时不收成「只要空的那一格」：点名了才只看满足条件的部分，先到期先出才选得到各批次。
		if demand.batch.Given() && textOrEmpty(row.BatchNumber) != demand.batch.String() {
			continue
		}
		if demand.expiry.Given() && textOrEmpty(row.Expiry) != demand.expiry.String() {
			continue
		}
		alias := textOrEmpty(row.Alias)
		if alias != "" && alias != demand.alias.String() {
			continue
		}
		if demand.alias.String() == "" && alias != "" {
			continue
		}
		location, err := domain.NewLocationCode(row.LocationCode)
		if err != nil {
			return 0, 0, err
		}
		usage, ok := zones[row.LocationCode]
		if !ok {
			place, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, location)
			if err != nil {
				return 0, 0, err
			}
			if !found {
				continue
			}
			zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, place.Content().ZoneCode())
			if err != nil {
				return 0, 0, err
			}
			if !found {
				continue
			}
			usage = zone.Content().Usage()
			zones[row.LocationCode] = usage
		}
		if !domain.AllocatableGoods(demand.state, usage) {
			continue
		}
		unit, err := domain.NewGoodsUnitName(row.Unit)
		if err != nil {
			return 0, 0, err
		}
		factor, ok := unitFactor(revision, unit)
		if !ok {
			continue
		}
		cell, err := cellFromBalance(row)
		if err != nil {
			return 0, 0, err
		}
		held, err := handler.holds.CellHold(ctx, command.Tenant, command.Owner, command.Warehouse, cell)
		if err != nil {
			return 0, 0, err
		}
		allocated, err := handler.holds.Allocated(ctx, command.Tenant, command.Owner, command.Warehouse, cell)
		if err != nil {
			return 0, 0, err
		}
		// 分配是已经落到格上的预占，下面按出库请求再扣，这里只留下冻结和作业锁定。
		free := row.Quantity - holdWithoutAllocation(held, allocated)
		if free <= 0 {
			continue
		}
		base, ok := scaleQuantity(free, factor)
		if !ok {
			return 0, 0, fmt.Errorf("accept outbound: 可分配数量溢出")
		}
		if alias == "" {
			unlabeled += base
		} else {
			labeled += base
		}
	}
	return labeled, unlabeled, nil
}

func scaleQuantity(quantity, factor int64) (int64, bool) {
	if quantity < 0 || factor <= 0 || (factor > 1 && quantity > math.MaxInt64/factor) {
		return 0, false
	}
	return quantity * factor, true
}

func textOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
