package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// FreezeOutcome 是冻结与解冻的结果。冻结不改库存账。
type FreezeOutcome uint8

const (
	FreezeOutcomeInvalid FreezeOutcome = iota
	FreezeFrozen
	FreezeExisting
	FreezeConflict
	FreezeNotValid
	FreezeContentInvalid
	FreezeWarehouseNotFound
	FreezeWarehouseNotAccepting
	FreezeNotFound
	FreezeUnfrozen
	FreezeNotConfigured
)

func (outcome FreezeOutcome) String() string {
	switch outcome {
	case FreezeFrozen:
		return "FROZEN"
	case FreezeExisting:
		return "EXISTING_RESULT"
	case FreezeConflict:
		return "SOURCE_CONFLICT"
	case FreezeNotValid:
		return "NOT_VALID"
	case FreezeContentInvalid:
		return "CONTENT_INVALID"
	case FreezeWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case FreezeWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case FreezeNotFound:
		return "FREEZE_NOT_FOUND"
	case FreezeUnfrozen:
		return "UNFROZEN"
	case FreezeNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// FreezeRefusal 指出冻结不成立的哪一种。不冻够的那一部分。
type FreezeRefusal uint8

const (
	FreezeRefusalInvalid FreezeRefusal = iota
	FreezeNotAllocatable
	FreezeQuantityShort
	FreezeHoldConflict
)

func (refusal FreezeRefusal) String() string {
	switch refusal {
	case FreezeNotAllocatable:
		return "NOT_ALLOCATABLE"
	case FreezeQuantityShort:
		return "QUANTITY_SHORT"
	case FreezeHoldConflict:
		return "HOLD_CONFLICT"
	default:
		return ""
	}
}

// FreezeResult 携带落点。已冻结、已有结果与已解冻上有冻结。
type FreezeResult struct {
	outcome FreezeOutcome
	freeze  domain.Freeze
	has     bool
	issues  []domain.FreezeIssue
	refusal FreezeRefusal
}

func (result FreezeResult) Outcome() FreezeOutcome        { return result.outcome }
func (result FreezeResult) Freeze() (domain.Freeze, bool) { return result.freeze, result.has }
func (result FreezeResult) Issues() []domain.FreezeIssue {
	return append([]domain.FreezeIssue(nil), result.issues...)
}
func (result FreezeResult) Refusal() FreezeRefusal { return result.refusal }

// FreezeCommand 是运营冻结一格或一箱。整箱时格子是零值，数量由用例写成一箱。
type FreezeCommand struct {
	Tenant     domain.TenantID
	DecisionID domain.RequestKey
	Owner      domain.OwnerAccountID
	Warehouse  domain.WarehouseCode
	Target     domain.FreezeTarget
	Cell       domain.StockCell
	Carton     domain.CartonNumber
	Quantity   int64
	Reason     string
	Operator   domain.Registrant
}

// UnfreezeCommand 是整份解冻。说明可空。货主与格取自所指的冻结。
type UnfreezeCommand struct {
	Tenant   domain.TenantID
	ID       domain.FreezeID
	Operator domain.Registrant
	Note     string
}

// FreezeHandler 在库存账内原子判定冻结成不成立。
type FreezeHandler struct {
	freezes    ports.FreezeStore
	identities ports.FreezeIdentities
	ledger     ports.StockOnHandRead
	posting    interface {
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
		CellBalance(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) (int64, error)
	}
	cartons      ports.CartonLedger
	warehouses   ports.WarehouseLookup
	locations    ports.StockLocationLookup
	zones        ports.ZoneLookup
	goods        ports.StockGoodsLookup
	holds        ports.StockHolds
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
}

func NewFreezeHandler(
	freezes ports.FreezeStore,
	identities ports.FreezeIdentities,
	ledger interface {
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
		CellBalance(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.StockCell) (int64, error)
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
	},
	cartons ports.CartonLedger,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	goods ports.StockGoodsLookup,
	holds ports.StockHolds,
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
) *FreezeHandler {
	return &FreezeHandler{
		freezes: freezes, identities: identities, ledger: ledger, posting: ledger, cartons: cartons,
		warehouses: warehouses, locations: locations, zones: zones, goods: goods, holds: holds,
		clock: clock, anyWarehouse: anyWarehouse,
	}
}

func (handler *FreezeHandler) Freeze(ctx context.Context, command FreezeCommand) (FreezeResult, error) {
	if command.Tenant.String() == "" || command.DecisionID.String() == "" || command.Owner.String() == "" || command.Warehouse.String() == "" || command.Reason == "" || command.Operator.String() == "" {
		return FreezeResult{}, fmt.Errorf("freeze stock: 命令不完整")
	}
	if err := handler.freezes.HoldDecision(ctx, command.Tenant, command.DecisionID); err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	existing, found, err := handler.freezes.LoadByDecision(ctx, command.Tenant, command.DecisionID)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	spec := domain.FreezeSpec{
		Tenant: command.Tenant, DecisionID: command.DecisionID, Owner: command.Owner, Warehouse: command.Warehouse,
		Target: command.Target, Cell: command.Cell, Carton: command.Carton, Quantity: command.Quantity, Reason: command.Reason,
		Operator: command.Operator, FrozenAt: handler.clock.Now(),
	}
	if command.Target == domain.FreezeCarton {
		spec.Quantity = 1
	}
	if found {
		if existing.SameFreeze(spec) {
			return FreezeResult{outcome: FreezeExisting, freeze: existing, has: true}, nil
		}
		return FreezeResult{outcome: FreezeConflict}, nil
	}
	warehouse, foundWarehouse, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if !foundWarehouse {
		return FreezeResult{outcome: FreezeWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return FreezeResult{outcome: FreezeWarehouseNotAccepting}, nil
	}
	if err := handler.posting.SerializePosting(ctx, command.Tenant, command.Owner, command.Warehouse); err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if command.Target == domain.FreezeCarton {
		return handler.freezeCarton(ctx, command, spec)
	}
	return handler.freezeGoods(ctx, command, spec)
}

func (handler *FreezeHandler) freezeGoods(ctx context.Context, command FreezeCommand, spec domain.FreezeSpec) (FreezeResult, error) {
	if command.Cell.InboundBatch().String() == "" || command.Quantity <= 0 {
		return FreezeResult{}, fmt.Errorf("freeze stock: 商品冻结缺批次或数量")
	}
	location, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, command.Cell.Location())
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if !found {
		return FreezeResult{outcome: FreezeContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound)}}, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, location.Content().ZoneCode())
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if !found {
		return FreezeResult{outcome: FreezeContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound)}}, nil
	}
	revision, found, err := handler.goods.LoadGoodsAt(ctx, command.Tenant, command.Owner, command.Cell.Goods(), handler.clock.Now())
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if !found {
		return FreezeResult{outcome: FreezeContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueGoodsNotRegistered)}}, nil
	}
	if _, ok := unitFactor(revision, command.Cell.Unit()); !ok {
		return FreezeResult{outcome: FreezeContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueUnitNotDeclared)}}, nil
	}
	if !domain.AllocatableGoods(command.Cell.State(), zone.Content().Usage()) {
		return FreezeResult{outcome: FreezeNotValid, refusal: FreezeNotAllocatable}, nil
	}
	balance, err := handler.posting.CellBalance(ctx, command.Tenant, command.Owner, command.Warehouse, command.Cell)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if balance <= 0 {
		return FreezeResult{outcome: FreezeContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueInboundBatchNotOnCell)}}, nil
	}
	held, err := handler.holds.CellHold(ctx, command.Tenant, command.Owner, command.Warehouse, command.Cell)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if command.Quantity > balance {
		return FreezeResult{outcome: FreezeNotValid, refusal: FreezeQuantityShort}, nil
	}
	if command.Quantity > balance-held {
		return FreezeResult{outcome: FreezeNotValid, refusal: FreezeHoldConflict}, nil
	}
	reserved, err := handler.holds.UnassignedReservation(ctx, command.Tenant, command.Owner, command.Warehouse, command.Cell.Goods(), command.Cell.State(), command.Cell.Alias(), command.Cell.BatchNumber(), command.Cell.Expiry())
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	allocatable, err := handler.allocatable(ctx, command.Tenant, command.Owner, command.Warehouse, command.Cell.Goods(), command.Cell.Unit(), command.Cell.State())
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	already, err := handler.freezes.SummaryFreeze(ctx, command.Tenant, command.Owner, command.Warehouse, command.Cell.Goods(), command.Cell.Unit(), command.Cell.State())
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if allocatable < already+reserved+command.Quantity {
		return FreezeResult{outcome: FreezeNotValid, refusal: FreezeHoldConflict}, nil
	}
	return handler.saveFreeze(ctx, spec)
}

func (handler *FreezeHandler) freezeCarton(ctx context.Context, command FreezeCommand, spec domain.FreezeSpec) (FreezeResult, error) {
	if err := handler.cartons.Serialize(ctx, command.Tenant, command.Owner, command.Warehouse); err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	place, err := handler.cartons.Locate(ctx, command.Tenant, command.Owner, command.Warehouse, command.Carton)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if !place.Found {
		return FreezeResult{outcome: FreezeContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueCartonNotOnHand)}}, nil
	}
	location, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, place.Location)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if !found {
		return FreezeResult{outcome: FreezeContentInvalid, issues: []domain.FreezeIssue{domain.NewFreezeIssue(domain.FreezeIssueLocationNotFound)}}, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, location.Content().ZoneCode())
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if !found || !domain.AllocatableCarton(zone.Content().Usage()) {
		return FreezeResult{outcome: FreezeNotValid, refusal: FreezeNotAllocatable}, nil
	}
	held, err := handler.holds.CartonHeld(ctx, command.Tenant, command.Owner, command.Warehouse, command.Carton)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if held {
		return FreezeResult{outcome: FreezeNotValid, refusal: FreezeHoldConflict}, nil
	}
	return handler.saveFreeze(ctx, spec)
}

func (handler *FreezeHandler) saveFreeze(ctx context.Context, spec domain.FreezeSpec) (FreezeResult, error) {
	id, err := handler.identities.NextFreezeID()
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	spec.ID = id
	spec.FrozenAt = handler.clock.Now()
	freeze, err := domain.NewFreeze(spec)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	if err := handler.freezes.Save(ctx, freeze); err != nil {
		return FreezeResult{}, fmt.Errorf("freeze stock: %w", err)
	}
	return FreezeResult{outcome: FreezeFrozen, freeze: freeze, has: true}, nil
}

func (handler *FreezeHandler) allocatable(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, goods domain.GoodsCode, unit domain.GoodsUnitName, state domain.StockState) (int64, error) {
	rows, err := handler.ledger.ListOnHandCells(ctx, tenant, owner, warehouse)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, row := range rows {
		if row.GoodsCode != goods.String() || row.Unit != unit.String() || row.State != state.String() {
			continue
		}
		location, err := domain.NewLocationCode(row.LocationCode)
		if err != nil {
			return 0, err
		}
		revision, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, location)
		if err != nil {
			return 0, err
		}
		if !found {
			continue
		}
		zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, revision.Content().ZoneCode())
		if err != nil {
			return 0, err
		}
		if found && domain.AllocatableGoods(state, zone.Content().Usage()) {
			total += row.Quantity
		}
	}
	return total, nil
}

func (handler *FreezeHandler) Unfreeze(ctx context.Context, command UnfreezeCommand) (FreezeResult, error) {
	if command.Tenant.String() == "" || command.ID.String() == "" || command.Operator.String() == "" {
		return FreezeResult{}, fmt.Errorf("unfreeze stock: 命令不完整")
	}
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("unfreeze stock: %w", err)
	}
	if !configured {
		return FreezeResult{outcome: FreezeNotConfigured}, nil
	}
	if err := handler.freezes.HoldFreeze(ctx, command.Tenant, command.ID); err != nil {
		return FreezeResult{}, fmt.Errorf("unfreeze stock: %w", err)
	}
	freeze, found, err := handler.freezes.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("unfreeze stock: %w", err)
	}
	if !found {
		return FreezeResult{outcome: FreezeNotFound}, nil
	}
	if freeze.Status() == domain.FreezeUnfrozen {
		return FreezeResult{outcome: FreezeExisting, freeze: freeze, has: true}, nil
	}
	unfrozen, err := freeze.Unfreeze(command.Operator, handler.clock.Now(), command.Note)
	if err != nil {
		return FreezeResult{}, fmt.Errorf("unfreeze stock: %w", err)
	}
	if err := handler.freezes.MarkUnfrozen(ctx, unfrozen); err != nil {
		return FreezeResult{}, fmt.Errorf("unfreeze stock: %w", err)
	}
	return FreezeResult{outcome: FreezeUnfrozen, freeze: unfrozen, has: true}, nil
}
