package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ReturnOutcome 是退货预报、到仓与处置的结果。
type ReturnOutcome uint8

const (
	ReturnOutcomeInvalid ReturnOutcome = iota
	ReturnAccepted
	ReturnExisting
	ReturnConflict
	ReturnContentInvalid
	ReturnWarehouseNotFound
	ReturnWarehouseNotAccepting
	ReturnCounted
	ReturnUnclaimed
	ReturnDisposed
	ReturnNotConfigured
)

func (outcome ReturnOutcome) String() string {
	switch outcome {
	case ReturnAccepted:
		return "ACCEPTED"
	case ReturnExisting:
		return "EXISTING_RESULT"
	case ReturnConflict:
		return "CONTENT_CONFLICT"
	case ReturnContentInvalid:
		return "CONTENT_INVALID"
	case ReturnWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case ReturnWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case ReturnCounted:
		return "COUNTED"
	case ReturnUnclaimed:
		return "UNCLAIMED"
	case ReturnDisposed:
		return "DISPOSED"
	case ReturnNotConfigured:
		return "NOT_CONFIGURED"
	default:
		return ""
	}
}

// ReturnResult 携带预报或到仓结果。不受理时带原因。
type ReturnResult struct {
	outcome ReturnOutcome
	advice  ports.ReturnAdvice
	has     bool
	reason  WarehouseNotAcceptingReason
	arrival string
}

func (result ReturnResult) Outcome() ReturnOutcome { return result.outcome }
func (result ReturnResult) Advice() (ports.ReturnAdvice, bool) {
	return result.advice, result.has
}
func (result ReturnResult) NotAcceptingReason() (WarehouseNotAcceptingReason, bool) {
	return result.reason, result.outcome == ReturnWarehouseNotAccepting
}
func (result ReturnResult) Arrival() string { return result.arrival }

// AcceptReturnCommand 是一份退货预报。预计商品不是库存。
type AcceptReturnCommand struct {
	Tenant    domain.TenantID
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	Origin    domain.ReturnOrigin
	Key       string
	Tracking  string
	Basis     string
	Operator  domain.Registrant
	Lines     []ports.ReturnLine
}

// ReceiveReturnCommand 是退件到仓。认不出货主则建待认领，不入账。
type ReceiveReturnCommand struct {
	Tenant     domain.TenantID
	FactID     domain.ReturnFactID
	Warehouse  domain.WarehouseCode
	Location   domain.LocationCode
	Tracking   string
	Goods      domain.GoodsCode
	Unit       domain.GoodsUnitName
	State      domain.StockState
	Quantity   int64
	OccurredAt time.Time
	Operator   domain.ReceiptOperatorID
	Device     domain.ReceiptDeviceID
	Arrival    domain.UnclaimedArrivalID
}

// DisposeReturnCommand 是一件退货的去向。货已不在退货暂存时不入账。
type DisposeReturnCommand struct {
	Tenant      domain.TenantID
	Decision    domain.ReturnDecisionID
	Fact        domain.ReturnFactID
	Destination domain.ReturnDestination
	Origin      domain.ReturnOrigin
	Owner       domain.OwnerAccountID
	Basis       string
	Operator    domain.Registrant
}

// ReturnHandler 受理退货预报、退货入库与回库处置。原出库确认不在这里冲回。
type ReturnHandler struct {
	returns    ports.ReturnStore
	identities ports.ReturnIdentities
	numbers    ports.ReturnNumberIssuer
	warehouses ports.WarehouseLookup
	locations  ports.StockLocationLookup
	zones      interface {
		LoadLatestZone(context.Context, domain.TenantID, domain.WarehouseCode, domain.ZoneCode) (domain.ZoneRevision, bool, error)
	}
	anyWarehouse ports.TenantWarehouseLookup
	owners       ports.OwnerAccountLookup
	poster       *PostStockMovementHandler
	ledger       interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
	}
	unclaimed *RegisterUnclaimedArrivalHandler
	arrivals  ports.UnclaimedArrivalCatalogueRead
	clock     ports.Clock
	// charges 缺席时退货入库与处置照样成立。
	charges chargeFormer
}

func NewReturnHandler(
	returns ports.ReturnStore,
	identities ports.ReturnIdentities,
	numbers ports.ReturnNumberIssuer,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones interface {
		LoadLatestZone(context.Context, domain.TenantID, domain.WarehouseCode, domain.ZoneCode) (domain.ZoneRevision, bool, error)
	},
	anyWarehouse ports.TenantWarehouseLookup,
	owners ports.OwnerAccountLookup,
	poster *PostStockMovementHandler,
	ledger interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
	},
	unclaimed *RegisterUnclaimedArrivalHandler,
	arrivals ports.UnclaimedArrivalCatalogueRead,
	clock ports.Clock,
) *ReturnHandler {
	return &ReturnHandler{
		returns: returns, identities: identities, numbers: numbers, warehouses: warehouses, locations: locations,
		zones: zones, anyWarehouse: anyWarehouse, owners: owners, poster: poster, ledger: ledger,
		unclaimed: unclaimed, arrivals: arrivals, clock: clock,
	}
}

func (handler *ReturnHandler) Accept(ctx context.Context, command AcceptReturnCommand) (ReturnResult, error) {
	if command.Key == "" || command.Tracking == "" || len(command.Lines) == 0 || command.Origin.String() == "" {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	if command.Origin == domain.ReturnOriginOperator && (command.Basis == "" || command.Operator.String() == "") {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	if command.Origin == domain.ReturnOriginCustomer && (command.Basis != "" || command.Operator.String() != "") {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	for _, line := range command.Lines {
		if line.Quantity < 1 || line.Goods.String() == "" || line.Unit.String() == "" {
			return ReturnResult{outcome: ReturnContentInvalid}, nil
		}
	}
	if command.Origin == domain.ReturnOriginOperator {
		registered, err := handler.owners.OwnerAccountRegistered(ctx, command.Tenant, command.Owner)
		if err != nil {
			return ReturnResult{}, fmt.Errorf("accept return: %w", err)
		}
		if !registered {
			return ReturnResult{}, ErrOwnerAccountNotInTenant
		}
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("accept return: %w", err)
	}
	if !found {
		return ReturnResult{outcome: ReturnWarehouseNotFound}, nil
	}
	existing, found, err := handler.returns.LoadAdviceByKey(ctx, command.Tenant, command.Origin, command.Key)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("accept return: %w", err)
	}
	if found {
		if sameReturnAdvice(existing, command) {
			return ReturnResult{outcome: ReturnExisting, advice: existing, has: true}, nil
		}
		return ReturnResult{outcome: ReturnConflict, advice: existing, has: true}, nil
	}
	if warehouse.Content().Deactivated() {
		return ReturnResult{outcome: ReturnWarehouseNotAccepting, reason: WarehouseNotAcceptingDeactivated}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return ReturnResult{outcome: ReturnWarehouseNotAccepting, reason: WarehouseNotAcceptingAgent}, nil
	}
	issue, err := handler.numbers.IssueReturnNumber(ctx, command.Tenant, warehouse.Content().BusinessTimeZone())
	if err != nil {
		return ReturnResult{}, fmt.Errorf("accept return: %w", err)
	}
	adviceID, err := handler.identities.NextReturnAdviceID()
	if err != nil {
		return ReturnResult{}, fmt.Errorf("accept return: %w", err)
	}
	advice := ports.ReturnAdvice{
		ID: adviceID, Owner: command.Owner, Warehouse: command.Warehouse, Origin: command.Origin, Key: command.Key,
		Tracking: command.Tracking, Basis: command.Basis, Operator: command.Operator.String(), Lines: command.Lines,
		Received: handler.clock.Now(),
	}
	switch issue.Outcome {
	case ports.PreAdviceNumberAssigned:
		advice.Number = issue.Number.String()
	case ports.PreAdviceNumberTimeZoneNotConfigured:
		advice.Absence = domain.NumberingTimeZoneNotConfigured.String()
	default:
		return ReturnResult{}, fmt.Errorf("accept return: numbering %s", issue.Outcome)
	}
	if err := handler.returns.SaveAdvice(ctx, command.Tenant, advice); err != nil {
		return ReturnResult{}, fmt.Errorf("accept return: %w", err)
	}
	return ReturnResult{outcome: ReturnAccepted, advice: advice, has: true}, nil
}

func (handler *ReturnHandler) Receive(ctx context.Context, command ReceiveReturnCommand) (ReturnResult, error) {
	if command.FactID.String() == "" || command.Tracking == "" || command.Quantity < 1 || command.Location.String() == "" {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	if !found {
		return ReturnResult{outcome: ReturnWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return ReturnResult{outcome: ReturnWarehouseNotAccepting, reason: WarehouseNotAcceptingAgent}, nil
	}
	existing, found, err := handler.returns.LoadReceipt(ctx, command.Tenant, command.FactID)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	if found {
		if sameReturnReceipt(existing, command) {
			return ReturnResult{outcome: ReturnExisting, arrival: existing.Arrival}, nil
		}
		return ReturnResult{outcome: ReturnConflict}, nil
	}
	staging, err := handler.returnStaging(ctx, command.Tenant, command.Warehouse, command.Location)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	if !staging {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	occurred := command.OccurredAt
	if occurred.IsZero() {
		occurred = handler.clock.Now()
	}
	if command.Arrival.String() != "" {
		return handler.receiveClaimed(ctx, command, occurred)
	}
	owner, advice, recognized, err := handler.returns.FindAdviceByTracking(ctx, command.Tenant, command.Warehouse, command.Tracking)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	adviceID := ""
	if recognized {
		adviceID = advice.String()
	}
	if !recognized {
		packed, foundOwner, err := handler.returns.FindOwnerByExternalNumber(ctx, command.Tenant, command.Tracking)
		if err != nil {
			return ReturnResult{}, fmt.Errorf("receive return: %w", err)
		}
		if foundOwner {
			owner = packed
			recognized = true
		}
	}
	if !recognized {
		return handler.leaveUnclaimed(ctx, command, occurred)
	}
	if err := handler.postReturn(ctx, command, owner, occurred); err != nil {
		return ReturnResult{}, err
	}
	receipt := receiptOf(command, owner.String(), adviceID, "", "COUNTED", occurred)
	if err := handler.returns.SaveReceipt(ctx, command.Tenant, receipt); err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	if err := handler.formReturnReceiptCharge(ctx, command, owner, occurred); err != nil {
		return ReturnResult{}, err
	}
	return ReturnResult{outcome: ReturnCounted}, nil
}

func (handler *ReturnHandler) receiveClaimed(ctx context.Context, command ReceiveReturnCommand, occurred time.Time) (ReturnResult, error) {
	detail, found, err := handler.arrivals.LoadDetail(ctx, command.Tenant, command.Arrival)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	claim, claimed := detail.ActiveClaim()
	if !found || !claimed || claim.Owner().String() == "" {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	if err := handler.postReturn(ctx, command, claim.Owner(), occurred); err != nil {
		return ReturnResult{}, err
	}
	receipt := receiptOf(command, claim.Owner().String(), "", command.Arrival.String(), "COUNTED", occurred)
	if err := handler.returns.SaveReceipt(ctx, command.Tenant, receipt); err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	if err := handler.formReturnReceiptCharge(ctx, command, claim.Owner(), occurred); err != nil {
		return ReturnResult{}, err
	}
	return ReturnResult{outcome: ReturnCounted, arrival: command.Arrival.String()}, nil
}

func (handler *ReturnHandler) leaveUnclaimed(ctx context.Context, command ReceiveReturnCommand, occurred time.Time) (ReturnResult, error) {
	fact, err := domain.NewUnclaimedArrivalFactID(command.FactID.String())
	if err != nil {
		return ReturnResult{}, err
	}
	observation, err := domain.NewUnclaimedArrivalObservation(command.Tracking)
	if err != nil {
		return ReturnResult{}, err
	}
	storage, err := domain.NewUnclaimedArrivalStorageLocation(command.Location.String())
	if err != nil {
		return ReturnResult{}, err
	}
	registered, err := handler.unclaimed.Register(ctx, RegisterUnclaimedArrivalCommand{
		Tenant: command.Tenant, FactID: fact, Device: command.Device, Operator: command.Operator, OccurredAt: occurred,
		DeviceSequence: 1, Warehouse: command.Warehouse, Kind: domain.UnclaimedArrivalKindReturn,
		Observation: observation, StorageLocation: storage,
	})
	if err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	if registered.Outcome() != RegisterUnclaimedRegistered && registered.Outcome() != RegisterUnclaimedExistingResult {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	arrival, ok := registered.Arrival()
	if !ok {
		return ReturnResult{}, fmt.Errorf("receive return: unclaimed arrival was not recorded")
	}
	receipt := receiptOf(command, "", "", arrival.ID().String(), "UNCLAIMED", occurred)
	if err := handler.returns.SaveReceipt(ctx, command.Tenant, receipt); err != nil {
		return ReturnResult{}, fmt.Errorf("receive return: %w", err)
	}
	return ReturnResult{outcome: ReturnUnclaimed, arrival: arrival.ID().String()}, nil
}

func (handler *ReturnHandler) Dispose(ctx context.Context, command DisposeReturnCommand) (ReturnResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("dispose return: %w", err)
	}
	if !configured {
		return ReturnResult{outcome: ReturnNotConfigured}, nil
	}
	if command.Decision.String() == "" || command.Fact.String() == "" || command.Destination.String() == "" {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	if command.Origin == domain.ReturnOriginOperator && (command.Basis == "" || command.Operator.String() == "") {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	existing, found, err := handler.returns.LoadDisposition(ctx, command.Tenant, command.Decision)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("dispose return: %w", err)
	}
	if found {
		if existing.Fact == command.Fact && existing.Destination == command.Destination {
			return ReturnResult{outcome: ReturnExisting}, nil
		}
		return ReturnResult{outcome: ReturnConflict}, nil
	}
	receipt, found, err := handler.returns.LoadReceipt(ctx, command.Tenant, command.Fact)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("dispose return: %w", err)
	}
	if !found || receipt.Outcome != "COUNTED" {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	if command.Origin == domain.ReturnOriginCustomer && command.Owner.String() != receipt.Owner {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	owner, err := domain.NewOwnerAccountID(receipt.Owner)
	if err != nil {
		return ReturnResult{}, err
	}
	rows, err := handler.ledger.ListOnHandCells(ctx, command.Tenant, owner, receipt.Warehouse)
	if err != nil {
		return ReturnResult{}, fmt.Errorf("dispose return: %w", err)
	}
	held := int64(0)
	for _, row := range rows {
		if row.LocationCode == receipt.Location.String() && row.GoodsCode == receipt.Goods.String() && row.Unit == receipt.Unit.String() && row.State == receipt.State.String() {
			held += row.Quantity
		}
	}
	if held < receipt.Quantity {
		return ReturnResult{outcome: ReturnContentInvalid}, nil
	}
	decided := handler.clock.Now()
	if err := handler.returns.SaveDisposition(ctx, command.Tenant, ports.ReturnDisposition{
		Decision: command.Decision, Fact: command.Fact, Destination: command.Destination, Basis: command.Basis,
		Operator: command.Operator.String(), Origin: command.Origin, Decided: decided,
	}); err != nil {
		return ReturnResult{}, fmt.Errorf("dispose return: %w", err)
	}
	if err := handler.formDispositionCharge(ctx, command, receipt, decided); err != nil {
		return ReturnResult{}, err
	}
	return ReturnResult{outcome: ReturnDisposed}, nil
}

func (handler *ReturnHandler) returnStaging(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, location domain.LocationCode) (bool, error) {
	place, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, location)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, place.Content().ZoneCode())
	if err != nil {
		return false, err
	}
	return found && zone.Content().Usage() == domain.ZoneUsageReturnStaging, nil
}

func (handler *ReturnHandler) postReturn(ctx context.Context, command ReceiveReturnCommand, owner domain.OwnerAccountID, occurred time.Time) error {
	source, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return err
	}
	cell, err := domain.NewStockCell(domain.StockCellSpec{
		Location: command.Location, Goods: command.Goods, Unit: command.Unit, State: command.State,
	})
	if err != nil {
		return err
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant: command.Tenant,
		Request: domain.NewStockMovementRequest(
			owner, command.Warehouse, domain.StockReasonReturnReceipt, source, occurred,
			[]domain.StockLineRequest{domain.NewStockLineRequest(domain.StockIn, command.Quantity, cell, time.Time{})},
		),
	})
	if err != nil {
		return fmt.Errorf("receive return: %w", err)
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return fmt.Errorf("receive return: stock %s", posted.Outcome())
	}
	return nil
}

func receiptOf(command ReceiveReturnCommand, owner, advice, arrival, outcome string, occurred time.Time) ports.ReturnReceipt {
	return ports.ReturnReceipt{
		Fact: command.FactID, Warehouse: command.Warehouse, Location: command.Location, Tracking: command.Tracking,
		Owner: owner, Advice: advice, Arrival: arrival, Goods: command.Goods, Unit: command.Unit, State: command.State,
		Quantity: command.Quantity, Outcome: outcome, Occurred: occurred,
	}
}

func sameReturnAdvice(existing ports.ReturnAdvice, command AcceptReturnCommand) bool {
	if existing.Owner != command.Owner || existing.Warehouse != command.Warehouse || existing.Tracking != command.Tracking || len(existing.Lines) != len(command.Lines) {
		return false
	}
	for index, line := range existing.Lines {
		other := command.Lines[index]
		if line.Goods != other.Goods || line.Unit != other.Unit || line.Quantity != other.Quantity {
			return false
		}
	}
	return true
}

func sameReturnReceipt(existing ports.ReturnReceipt, command ReceiveReturnCommand) bool {
	return existing.Warehouse == command.Warehouse && existing.Location == command.Location && existing.Tracking == command.Tracking &&
		existing.Goods == command.Goods && existing.Unit == command.Unit && existing.State == command.State && existing.Quantity == command.Quantity
}
