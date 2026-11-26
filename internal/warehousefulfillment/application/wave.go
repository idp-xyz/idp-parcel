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
	WaveConfirmed
	WaveRecorded
	WaveWithheld
	WaveCorrected
	WavePending
	WavePosted
	WaveDutyBlocked
	WaveSubmitted
	WaveUnresolved
	WavePaused
	WaveOtherAuthority
	WaveInputRejected
	WaveWithdrawn
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
	case WaveConfirmed:
		return "CONFIRMED"
	case WaveRecorded:
		return "RECORDED"
	case WaveWithheld:
		return "WITHHELD"
	case WaveCorrected:
		return "CORRECTED"
	case WavePending:
		return "PENDING"
	case WavePosted:
		return "POSTED"
	case WaveDutyBlocked:
		return "DUTY_BLOCKED"
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
	case WaveWithdrawn:
		return "WITHDRAWN"
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

// ReleaseWaveCommand 把已预占的请求放进一个波次。标了预取并且选了包材时，本产品网络、非平台备货、仓测齐、代办已登记的请求按包裹序号 1 交一份估算委托。
type ReleaseWaveCommand struct {
	Tenant    domain.TenantID
	ID        domain.WaveID
	Warehouse domain.WarehouseCode
	Operator  domain.Registrant
	Requests  []domain.OutboundRequestID
	Prefetch  bool
	Packaging domain.PackagingMaterialCode
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
		LoadMovementBySource(context.Context, domain.TenantID, domain.StockMovementReason, domain.SourceFactReference) (domain.StockMovement, bool, error)
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
	// confirmation 缺席时出库确认照样落册，只是不把事实交给全程追踪。
	confirmation outboundConfirmationHandoff
	// duties 缺席时，判不成立的回补批准答未配置。
	duties ports.ApprovalDutyStore
	// releaser 缺席时，已经提交委托的截单答未配置，不先拆包。
	releaser shipmentReleaser
	// charges 缺席时出库确认照样成立。重量体积单位没登记时出库作业发生项不形成。
	charges chargeFormer
	// prefetch 缺席时，标了预取也只分配，打包后再交委托。
	prefetch *wavePrefetch
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
		LoadMovementBySource(context.Context, domain.TenantID, domain.StockMovementReason, domain.SourceFactReference) (domain.StockMovement, bool, error)
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

// WithConfirmationHandoff 接上交给全程追踪的出库确认。没接时确认照样落册。
func (handler *WaveHandler) WithConfirmationHandoff(handoff outboundConfirmationHandoff) *WaveHandler {
	handler.confirmation = handoff
	return handler
}

// WithApprovalDuties 接上批准职责规则。没接时回补批准答未配置。
func (handler *WaveHandler) WithApprovalDuties(duties ports.ApprovalDutyStore) *WaveHandler {
	handler.duties = duties
	return handler
}

// WithShipmentReleaser 接上询问小包托运放不放行撤回。没接时，已提交委托的截单答未配置。
func (handler *WaveHandler) WithShipmentReleaser(releaser shipmentReleaser) *WaveHandler {
	handler.releaser = releaser
	return handler
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
	// 同一波次里先分走的数量还没落成拣货任务，后一份请求再读占用账看不见。
	taken := map[string]int64{}
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
		allocated, err := handler.allocate(ctx, command, request, taken)
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
	if err := handler.prefetchRequests(ctx, command, tasks); err != nil {
		return WaveResult{}, err
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
	// 先拿到这位货主在这个仓的入账次序，再读任务与请求来判断。同一张请求的两条任务可以被两个人同时拣：
	// 各自在排队之前读到的预占与任务状态都是旧的，照旧的判，拣齐的请求会停在已预占，或一条任务被记两遍。
	owning, found, err := handler.requests.LoadByID(ctx, command.Tenant, task.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, owning.Owner(), owning.Warehouse()); err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	task, found, err = handler.waves.LoadTask(ctx, command.Tenant, command.TaskID)
	if err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if task.Status == "CANCELLED" || task.Status == "LATE" {
		if task.Fact == command.FactID.String() && task.Picked == command.Quantity {
			return WaveResult{outcome: WaveExisting, tasks: []ports.PickTask{task}}, nil
		}
		if task.Status == "LATE" || command.Quantity <= 0 || command.Quantity > task.Quantity {
			return WaveResult{outcome: WaveConflict}, nil
		}
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, task.Request)
		if err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		if !found {
			return WaveResult{outcome: WaveNotFound}, nil
		}
		task.Picked = command.Quantity
		task.Staging = command.Target
		if err := handler.postPick(ctx, request, task, command); err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		if err := handler.waves.SaveLatePick(ctx, command.Tenant, task, command.FactID.String()); err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		if err := handler.queueReturns(ctx, command.Tenant, task.Request, 0, []ports.PickTask{task}); err != nil {
			return WaveResult{}, err
		}
		return WaveResult{outcome: WaveRecorded, tasks: []ports.PickTask{task}}, nil
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
	if err != nil || !found || place.Content().Deactivated() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, request.Warehouse(), place.Content().ZoneCode())
	if err != nil || !found || zone.Content().Deactivated() || zone.Content().Usage() != domain.ZoneUsageOutboundStaging {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	if err := handler.postPick(ctx, request, task, command); err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	task.Picked = command.Quantity
	task.Staging = command.Target
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
	var replacements []ports.PickTask
	differenceID := ""
	if short > 0 {
		id, err := handler.differences.NextStockDiscrepancyID()
		if err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		differenceID = id.String()
		replacements, err = handler.reallocateShort(ctx, command.Tenant, request, task, siblings, short)
		if err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
	}
	// 这张请求还有任务等着拣——别的任务，或刚替少拣添出来的——就仍是已放波次，不退出波次：退回已预占会让它带着
	// 没拣的任务被再放一次，那几件就分了两遍。任务都有了结果之后，预占全拣走的是已拣货；还剩预占没拣到的
	// （少了又换不出别的格）回到已预占，等有货了再放。
	status := domain.OutboundAllocated
	if !open && len(replacements) == 0 {
		var remaining int64
		for _, reserved := range request.Reservations() {
			remaining += reserved.Quantity
		}
		status = domain.OutboundPicked
		if remaining > command.Quantity {
			status = domain.OutboundReserved
		}
	}
	if err := handler.waves.SavePick(ctx, command.Tenant, task, status, differenceID, command.FactID.String(), short); err != nil {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	for _, added := range replacements {
		if err := handler.waves.InsertOpenTask(ctx, command.Tenant, added); err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
	}
	return WaveResult{outcome: WavePicked, tasks: append([]ports.PickTask{task}, replacements...)}, nil
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
	for _, task := range tasks {
		if task.Status == "OPEN" {
			return WaveResult{outcome: WaveStatusMismatch}, nil
		}
	}
	picked := pickedQuantity(tasks)
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
	if !parcelsConserve(command.Parcels, pickedQuantity(tasks)) {
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

func (handler *WaveHandler) allocate(ctx context.Context, command ReleaseWaveCommand, request domain.OutboundRequest, taken map[string]int64) ([]ports.PickTask, error) {
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
		cells, err := handler.rankedCells(ctx, command, request, rows, reserved, taken)
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
				Unit: cell.unit, State: reserved.State, Alias: cell.alias, Inbound: cell.inbound, Quantity: take, Status: "OPEN",
			})
			need -= take
			taken[allocationKey(cell.location, reserved.Goods, cell.unit, reserved.State, cell.alias, cell.inbound)] += take
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
	alias    domain.GoodsAliasCode
	inbound  domain.InboundBatchID
	expiry   string
	at       time.Time
	picking  bool
	priority domain.LocationPriority
	free     int64
}

func allocationKey(location domain.LocationCode, goods domain.GoodsCode, unit domain.GoodsUnitName, state domain.StockState, alias domain.GoodsAliasCode, inbound domain.InboundBatchID) string {
	return location.String() + "\x1f" + goods.String() + "\x1f" + unit.String() + "\x1f" + state.String() + "\x1f" + alias.String() + "\x1f" + inbound.String()
}

func (handler *WaveHandler) rankedCells(ctx context.Context, command ReleaseWaveCommand, request domain.OutboundRequest, rows []ports.StockBalanceRow, reserved domain.OutboundReservation, taken map[string]int64) ([]rankedCell, error) {
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
		if reserved.Batch.Given() && textOrEmpty(row.BatchNumber) != reserved.Batch.String() {
			continue
		}
		if reserved.Expiry.Given() && textOrEmpty(row.Expiry) != reserved.Expiry.String() {
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
		inbound, err := domain.NewInboundBatchID(row.InboundBatchID)
		if err != nil {
			return nil, err
		}
		var alias domain.GoodsAliasCode
		if row.Alias != nil {
			alias, err = domain.NewGoodsAliasCode(*row.Alias)
			if err != nil {
				return nil, err
			}
		}
		free := row.Quantity - held - taken[allocationKey(location, reserved.Goods, unit, reserved.State, alias, inbound)]
		if free <= 0 {
			continue
		}
		batch, found, err := handler.ledger.LoadInboundBatch(ctx, command.Tenant, inbound)
		if err != nil || !found {
			return nil, err
		}
		cells = append(cells, rankedCell{
			location: location, unit: unit, alias: alias, inbound: inbound, expiry: batch.Expiry().String(), at: batch.InboundAt(),
			picking: zone.Content().Usage() == domain.ZoneUsagePicking, priority: place.Content().Priority(), free: free,
		})
	}
	// 参考配置：当前修订跟踪效期的先到期先出，其余按入库时间先进先出。批次顺序在库区与库位优先级之前。
	tracksExpiry := revision.Content().TrackExpiry()
	slices.SortFunc(cells, func(left, right rankedCell) int {
		return compareRankedCells(left, right, tracksExpiry)
	})
	return cells, nil
}

func compareRankedCells(left, right rankedCell, tracksExpiry bool) int {
	if tracksExpiry {
		if order := compareExpiry(left.expiry, right.expiry); order != 0 {
			return order
		}
	}
	if order := compareInbound(left.at, right.at); order != 0 {
		return order
	}
	if left.picking != right.picking {
		if left.picking {
			return -1
		}
		return 1
	}
	if order := compareLocationPriority(left.priority, right.priority); order != 0 {
		return order
	}
	if left.location.String() != right.location.String() {
		if left.location.String() < right.location.String() {
			return -1
		}
		return 1
	}
	if left.inbound.String() < right.inbound.String() {
		return -1
	}
	if left.inbound.String() > right.inbound.String() {
		return 1
	}
	return 0
}

func compareExpiry(left, right string) int {
	switch {
	case left == "" && right != "":
		return 1
	case right == "" && left != "":
		return -1
	case left == right:
		return 0
	case left < right:
		return -1
	default:
		return 1
	}
}

func compareInbound(left, right time.Time) int {
	if left.Before(right) {
		return -1
	}
	if right.Before(left) {
		return 1
	}
	return 0
}

// 没给优先级不当成任何数字，只排在每一条写明的优先级之后：不给就是没有，不是 0。
func compareLocationPriority(left, right domain.LocationPriority) int {
	if left.Given() != right.Given() {
		if left.Given() {
			return -1
		}
		return 1
	}
	if !left.Given() || left.Value() == right.Value() {
		return 0
	}
	if left.Value() > right.Value() {
		return -1
	}
	return 1
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
		if row.LocationCode != task.Location.String() || row.InboundBatchID != task.Inbound.String() || row.GoodsCode != task.Goods.String() ||
			row.Unit != task.Unit.String() || row.State != task.State.String() || textOrEmpty(row.Alias) != task.Alias.String() {
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
