package application

import (
	"context"
	"fmt"
	"slices"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// WaveOutcome 是放波次、采用分配策略或拣货的答案。
type WaveOutcome uint8

const (
	WaveOutcomeInvalid WaveOutcome = iota
	WaveAllocated
	WaveNotConfigured
	WaveExisting
	WaveConflict
	WaveStatusMismatch
	WaveNotFound
	WaveContentInvalid
	WavePicked
	WaveStrategyRegistered
	WaveChecked
	WaveMismatch
	WavePacked
	WaveSubmitted
	WaveUnresolved
	WavePaused
	WaveOtherAuthority
	WaveInputRejected
)

func (outcome WaveOutcome) String() string {
	switch outcome {
	case WaveAllocated:
		return "ALLOCATED"
	case WaveNotConfigured:
		return "NOT_CONFIGURED"
	case WaveExisting:
		return "EXISTING_RESULT"
	case WaveConflict:
		return "CONTENT_CONFLICT"
	case WaveStatusMismatch:
		return "STATUS_MISMATCH"
	case WaveNotFound:
		return "WAVE_NOT_FOUND"
	case WaveContentInvalid:
		return "CONTENT_INVALID"
	case WavePicked:
		return "PICKED"
	case WaveStrategyRegistered:
		return "REGISTERED"
	case WaveChecked:
		return "CHECKED"
	case WaveMismatch:
		return "CHECK_MISMATCH"
	case WavePacked:
		return "PACKED"
	case WaveSubmitted:
		return "SUBMITTED"
	case WaveUnresolved:
		return "OWNERSHIP_UNRESOLVED"
	case WavePaused:
		return "ADMISSION_PAUSED"
	case WaveOtherAuthority:
		return "OTHER_PRODUCTION_AUTHORITY"
	case WaveInputRejected:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// WaveResult 是一次放波次或拣货。Tasks 只在分配或重放时有。
type WaveResult struct {
	outcome WaveOutcome
	tasks   []ports.PickTask
}

func (result WaveResult) Outcome() WaveOutcome { return result.outcome }
func (result WaveResult) Tasks() []ports.PickTask {
	return append([]ports.PickTask(nil), result.tasks...)
}

// ReleaseWaveCommand 把已预占的请求放进一个波次。预取只记下选择，提交委托不在这一段。
type ReleaseWaveCommand struct {
	Tenant    domain.TenantID
	ID        domain.WaveID
	Warehouse domain.WarehouseCode
	Operator  domain.Registrant
	Requests  []domain.OutboundRequestID
	Prefetch  bool
}

// PickCommand 是一条拣货事实。实拣可以少于任务数量。
type PickCommand struct {
	Tenant    domain.TenantID
	FactID    domain.PickFactID
	TaskID    domain.PickTaskID
	Quantity  int64
	Target    domain.LocationCode
	Warehouse domain.WarehouseCode
}

// AdoptStrategyCommand 采用随产品发布的参考分配策略。没采用时放波次答未配置。
type AdoptStrategyCommand struct {
	Tenant   domain.TenantID
	Revision int
	Operator domain.Registrant
}

// WaveHandler 放波次并拣货。分配不超过预占，拣货才把货移进待出库。
type WaveHandler struct {
	waves    ports.WaveStore
	requests ports.OutboundStore
	tasks    interface {
		NextPickTaskID() (domain.PickTaskID, error)
	}
	differences interface {
		NextStockDiscrepancyID() (domain.StockDiscrepancyID, error)
	}
	poster *PostStockMovementHandler
	ledger interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
		LoadInboundBatch(context.Context, domain.TenantID, domain.InboundBatchID) (domain.InboundBatch, bool, error)
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
	}
	locations ports.StockLocationLookup
	zones     interface {
		LoadLatestZone(context.Context, domain.TenantID, domain.WarehouseCode, domain.ZoneCode) (domain.ZoneRevision, bool, error)
	}
	goods        ports.StockGoodsLookup
	holds        ports.StockHolds
	clock        ports.Clock
	anyWarehouse ports.TenantWarehouseLookup
}

func NewWaveHandler(
	waves ports.WaveStore,
	requests ports.OutboundStore,
	tasks interface {
		NextPickTaskID() (domain.PickTaskID, error)
	},
	differences interface {
		NextStockDiscrepancyID() (domain.StockDiscrepancyID, error)
	},
	poster *PostStockMovementHandler,
	ledger interface {
		ListOnHandCells(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) ([]ports.StockBalanceRow, error)
		LoadInboundBatch(context.Context, domain.TenantID, domain.InboundBatchID) (domain.InboundBatch, bool, error)
		SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error
	},
	locations ports.StockLocationLookup,
	zones interface {
		LoadLatestZone(context.Context, domain.TenantID, domain.WarehouseCode, domain.ZoneCode) (domain.ZoneRevision, bool, error)
	},
	goods ports.StockGoodsLookup,
	holds ports.StockHolds,
	clock ports.Clock,
	anyWarehouse ports.TenantWarehouseLookup,
) *WaveHandler {
	return &WaveHandler{
		waves: waves, requests: requests, tasks: tasks, differences: differences, poster: poster, ledger: ledger,
		locations: locations, zones: zones, goods: goods, holds: holds, clock: clock, anyWarehouse: anyWarehouse,
	}
}

func (handler *WaveHandler) Adopt(ctx context.Context, command AdoptStrategyCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("adopt allocation strategy: %w", err)
	}
	if !configured || command.Revision < 1 || command.Operator.String() == "" {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	current, found, err := handler.waves.LoadStrategy(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("adopt allocation strategy: %w", err)
	}
	if found && current.Revision == command.Revision {
		return WaveResult{outcome: WaveExisting}, nil
	}
	if found && command.Revision != current.Revision+1 {
		return WaveResult{outcome: WaveConflict}, nil
	}
	if err := handler.waves.SaveStrategy(ctx, command.Tenant, ports.AllocationStrategy{
		Revision: command.Revision, Operator: command.Operator.String(), RegisteredAt: handler.clock.Now(),
	}); err != nil {
		return WaveResult{}, fmt.Errorf("adopt allocation strategy: %w", err)
	}
	return WaveResult{outcome: WaveStrategyRegistered}, nil
}

func (handler *WaveHandler) Release(ctx context.Context, command ReleaseWaveCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("release wave: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if len(command.Requests) == 0 || command.Operator.String() == "" || command.ID.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	existing, found, err := handler.waves.LoadWave(ctx, command.Tenant, command.ID)
	if err != nil {
		return WaveResult{}, fmt.Errorf("release wave: %w", err)
	}
	if found {
		if sameRequests(existing, command.Requests) {
			tasks := []ports.PickTask{}
			for _, request := range command.Requests {
				listed, err := handler.waves.TasksOf(ctx, command.Tenant, request)
				if err != nil {
					return WaveResult{}, fmt.Errorf("release wave: %w", err)
				}
				tasks = append(tasks, listed...)
			}
			return WaveResult{outcome: WaveExisting, tasks: tasks}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	if _, adopted, err := handler.waves.LoadStrategy(ctx, command.Tenant); err != nil {
		return WaveResult{}, fmt.Errorf("release wave: %w", err)
	} else if !adopted {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	var tasks []ports.PickTask
	owners := map[string]domain.OwnerAccountID{}
	for _, id := range command.Requests {
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
		if !found || request.Warehouse() != command.Warehouse {
			return WaveResult{outcome: WaveNotFound}, nil
		}
		if request.Status() != domain.OutboundReserved {
			return WaveResult{outcome: WaveStatusMismatch}, nil
		}
		owners[request.Owner().String()] = request.Owner()
		allocated, err := handler.allocate(ctx, command, request)
		if err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
		if allocated == nil {
			return WaveResult{outcome: WaveContentInvalid}, nil
		}
		tasks = append(tasks, allocated...)
	}
	for _, owner := range owners {
		if err := handler.ledger.SerializePosting(ctx, command.Tenant, owner, command.Warehouse); err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
	}
	if err := handler.waves.SaveWave(ctx, command.Tenant, command.ID, command.Warehouse, command.Operator, command.Prefetch, handler.clock.Now(), command.Requests, tasks); err != nil {
		return WaveResult{}, fmt.Errorf("release wave: %w", err)
	}
	return WaveResult{outcome: WaveAllocated, tasks: tasks}, nil
}

func (handler *WaveHandler) Pick(ctx context.Context, command PickCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	task, found, err := handler.waves.LoadTask(ctx, command.Tenant, command.TaskID)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if task.Status != "OPEN" {
		if task.Fact == command.FactID.String() && task.Picked == command.Quantity {
			return WaveResult{outcome: WaveExisting, tasks: []ports.PickTask{task}}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	if command.Quantity <= 0 || command.Quantity > task.Quantity-task.Picked {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, task.Request)
	if err != nil || !found {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	if command.Warehouse.String() != "" && command.Warehouse != request.Warehouse() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	place, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, request.Warehouse(), command.Target)
	if err != nil || !found {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, request.Warehouse(), place.Content().ZoneCode())
	if err != nil || !found || zone.Content().Usage() != domain.ZoneUsageOutboundStaging {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	if err := handler.postPick(ctx, request, task, command); err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	task.Picked = command.Quantity
	task.Status = "DONE"
	short := task.Quantity - command.Quantity
	if short > 0 {
		task.Status = "SHORT"
	}
	siblings, err := handler.waves.TasksOf(ctx, command.Tenant, task.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	open := false
	for _, sibling := range siblings {
		if sibling.ID == task.ID {
			continue
		}
		if sibling.Status == "OPEN" {
			open = true
		}
	}
	status := domain.OutboundPicked
	if short > 0 || open {
		status = domain.OutboundReserved
		if open && short == 0 {
			status = domain.OutboundAllocated
		}
	}
	differenceID := ""
	if short > 0 {
		id, err := handler.differences.NextStockDiscrepancyID()
		if err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		differenceID = id.String()
	}
	if err := handler.waves.SavePick(ctx, command.Tenant, task, status, differenceID, command.FactID.String(), short); err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	return WaveResult{outcome: WavePicked, tasks: []ports.PickTask{task}}, nil
}

// CheckCommand 是一次出库复核。观察到的数量必须等于已拣数量，否则不进入可打包。
type CheckCommand struct {
	Tenant    domain.TenantID
	FactID    domain.PickFactID
	Request   domain.OutboundRequestID
	Observed  int64
	Warehouse domain.WarehouseCode
}

// PackParcelCommand 是打包时的一个包裹。
type PackParcelCommand struct {
	Sequence   int
	Packaging  string
	Weight     string
	WeightUnit string
	Length     string
	Width      string
	Height     string
	Dimension  string
	Goods      domain.GoodsCode
	Unit       domain.GoodsUnitName
	Quantity   int64
}

// PackCommand 写下装箱记录。委托还不在这一段提交。
type PackCommand struct {
	Tenant    domain.TenantID
	FactID    domain.PickFactID
	Request   domain.OutboundRequestID
	Parcels   []PackParcelCommand
	Warehouse domain.WarehouseCode
}

func (handler *WaveHandler) Check(ctx context.Context, command CheckCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("check outbound: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	observed, matched, found, err := handler.waves.LoadCheck(ctx, command.Tenant, command.FactID)
	if err != nil {
		return WaveResult{}, fmt.Errorf("check outbound: %w", err)
	}
	if found {
		if observed == command.Observed {
			if matched {
				return WaveResult{outcome: WaveExisting}, nil
			}
			return WaveResult{outcome: WaveMismatch}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("check outbound: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if request.Status() != domain.OutboundPicked {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if command.Warehouse.String() != "" && command.Warehouse != request.Warehouse() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	tasks, err := handler.waves.TasksOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("check outbound: %w", err)
	}
	var picked int64
	for _, task := range tasks {
		if task.Status == "OPEN" {
			return WaveResult{outcome: WaveStatusMismatch}, nil
		}
		picked += task.Picked
	}
	matched = command.Observed == picked && command.Observed > 0
	next := request.Status()
	outcome := WaveMismatch
	if matched {
		next = domain.OutboundChecked
		outcome = WaveChecked
	}
	if err := handler.waves.SaveCheck(ctx, command.Tenant, command.FactID, command.Request, command.Observed, matched, handler.clock.Now(), next); err != nil {
		return WaveResult{}, fmt.Errorf("check outbound: %w", err)
	}
	return WaveResult{outcome: outcome}, nil
}

func (handler *WaveHandler) Pack(ctx context.Context, command PackCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pack outbound: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	found, err := handler.waves.LoadPack(ctx, command.Tenant, command.FactID)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pack outbound: %w", err)
	}
	if found {
		return WaveResult{outcome: WaveExisting}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pack outbound: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if request.Status() != domain.OutboundChecked {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if command.Warehouse.String() != "" && command.Warehouse != request.Warehouse() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	tasks, err := handler.waves.TasksOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pack outbound: %w", err)
	}
	var picked int64
	for _, task := range tasks {
		picked += task.Picked
	}
	if !parcelsConserve(command.Parcels, picked) {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	stored := make([]ports.PackParcel, 0, len(command.Parcels))
	for _, parcel := range command.Parcels {
		stored = append(stored, ports.PackParcel{
			Sequence: parcel.Sequence, Packaging: parcel.Packaging, Weight: parcel.Weight, WeightUnit: parcel.WeightUnit,
			Length: parcel.Length, Width: parcel.Width, Height: parcel.Height, Dimension: parcel.Dimension,
			Goods: parcel.Goods, Unit: parcel.Unit, Quantity: parcel.Quantity,
		})
	}
	if err := handler.waves.SavePack(ctx, command.Tenant, command.FactID, command.Request, stored, handler.clock.Now()); err != nil {
		return WaveResult{}, fmt.Errorf("pack outbound: %w", err)
	}
	return WaveResult{outcome: WavePacked}, nil
}

func parcelsConserve(parcels []PackParcelCommand, picked int64) bool {
	if len(parcels) == 0 || picked <= 0 {
		return false
	}
	seen := map[int]struct{}{}
	var total int64
	for _, parcel := range parcels {
		if parcel.Sequence < 1 || parcel.Quantity <= 0 || parcel.Packaging == "" || parcel.Weight == "" || parcel.WeightUnit == "" ||
			parcel.Length == "" || parcel.Width == "" || parcel.Height == "" || parcel.Dimension == "" ||
			parcel.Goods.String() == "" || parcel.Unit.String() == "" {
			return false
		}
		if _, ok := seen[parcel.Sequence]; ok {
			return false
		}
		seen[parcel.Sequence] = struct{}{}
		total += parcel.Quantity
	}
	return total == picked
}

func (handler *WaveHandler) allocate(ctx context.Context, command ReleaseWaveCommand, request domain.OutboundRequest) ([]ports.PickTask, error) {
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, request.Owner(), command.Warehouse); err != nil {
		return nil, err
	}
	rows, err := handler.ledger.ListOnHandCells(ctx, command.Tenant, request.Owner(), command.Warehouse)
	if err != nil {
		return nil, err
	}
	var tasks []ports.PickTask
	for _, reserved := range request.Reservations() {
		need := reserved.Quantity
		cells, err := handler.rankedCells(ctx, command, request, rows, reserved)
		if err != nil {
			return nil, err
		}
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
			tasks = append(tasks, ports.PickTask{
				ID: id, WaveID: command.ID, Request: request.ID(), Location: cell.location, Goods: reserved.Goods,
				Unit: cell.unit, State: reserved.State, Inbound: cell.inbound, Quantity: take, Status: "OPEN",
			})
			need -= take
			cell.free -= take
		}
		if need != 0 {
			return nil, nil
		}
	}
	return tasks, nil
}

type rankedCell struct {
	location domain.LocationCode
	unit     domain.GoodsUnitName
	inbound  domain.InboundBatchID
	at       time.Time
	picking  bool
	free     int64
}

func (handler *WaveHandler) rankedCells(ctx context.Context, command ReleaseWaveCommand, request domain.OutboundRequest, rows []ports.StockBalanceRow, reserved domain.OutboundReservation) ([]rankedCell, error) {
	revision, found, err := handler.goods.LoadLatestGoods(ctx, command.Tenant, request.Owner(), reserved.Goods)
	if err != nil || !found {
		return nil, err
	}
	var cells []rankedCell
	for _, row := range rows {
		if row.GoodsCode != reserved.Goods.String() || row.State != reserved.State.String() {
			continue
		}
		if textOrEmpty(row.Alias) != reserved.Alias.String() {
			continue
		}
		unit, err := domain.NewGoodsUnitName(row.Unit)
		if err != nil {
			return nil, err
		}
		factor, ok := unitFactor(revision, unit)
		if !ok || factor != 1 {
			continue
		}
		location, err := domain.NewLocationCode(row.LocationCode)
		if err != nil {
			return nil, err
		}
		place, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, command.Warehouse, location)
		if err != nil || !found {
			return nil, err
		}
		zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, command.Warehouse, place.Content().ZoneCode())
		if err != nil || !found || !domain.AllocatableGoods(reserved.State, zone.Content().Usage()) {
			continue
		}
		stockCell, err := cellFromBalance(row)
		if err != nil {
			return nil, err
		}
		held, err := handler.holds.CellHold(ctx, command.Tenant, request.Owner(), command.Warehouse, stockCell)
		if err != nil {
			return nil, err
		}
		free := row.Quantity - held
		if free <= 0 {
			continue
		}
		inbound, err := domain.NewInboundBatchID(row.InboundBatchID)
		if err != nil {
			return nil, err
		}
		batch, found, err := handler.ledger.LoadInboundBatch(ctx, command.Tenant, inbound)
		if err != nil || !found {
			return nil, err
		}
		cells = append(cells, rankedCell{
			location: location, unit: unit, inbound: inbound, at: batch.InboundAt(),
			picking: zone.Content().Usage() == domain.ZoneUsagePicking, free: free,
		})
	}
	slices.SortFunc(cells, func(left, right rankedCell) int {
		if left.picking != right.picking {
			if left.picking {
				return -1
			}
			return 1
		}
		if left.at.Before(right.at) {
			return -1
		}
		if right.at.Before(left.at) {
			return 1
		}
		return 0
	})
	return cells, nil
}

func (handler *WaveHandler) postPick(ctx context.Context, request domain.OutboundRequest, task ports.PickTask, command PickCommand) error {
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, request.Owner(), request.Warehouse()); err != nil {
		return err
	}
	rows, err := handler.ledger.ListOnHandCells(ctx, command.Tenant, request.Owner(), request.Warehouse())
	if err != nil {
		return err
	}
	var source domain.StockCell
	found := false
	for _, row := range rows {
		if row.LocationCode != task.Location.String() || row.InboundBatchID != task.Inbound.String() || row.GoodsCode != task.Goods.String() {
			continue
		}
		source, err = cellFromBalance(row)
		if err != nil {
			return err
		}
		found = true
		break
	}
	if !found {
		return fmt.Errorf("pick: 任务所在的格没有货")
	}
	dest, err := retarget(source, command.Target, task.Unit, task.State)
	if err != nil {
		return err
	}
	out, err := stockLine(domain.StockOut, command.Quantity, source)
	if err != nil {
		return err
	}
	in, err := stockLine(domain.StockIn, command.Quantity, dest)
	if err != nil {
		return err
	}
	reference, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return err
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant:  command.Tenant,
		Request: domain.NewStockMovementRequest(request.Owner(), request.Warehouse(), domain.StockReasonInternalMove, reference, handler.clock.Now(), []domain.StockLineRequest{out, in}),
	})
	if err != nil {
		return err
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return fmt.Errorf("pick: 没有入账")
	}
	return nil
}

func sameRequests(left, right []domain.OutboundRequestID) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]int{}
	for _, id := range left {
		seen[id.String()]++
	}
	for _, id := range right {
		seen[id.String()]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}
