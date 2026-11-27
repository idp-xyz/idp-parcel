package application

import (
	"context"
	"fmt"
	"maps"
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
	WaveShortShipped
	WavePartialShipmentNotConfigured
	WavePartialShipmentNotAllowed
	WavePrefetchStands
	WaveTotesRegistered
	WaveToteRetired
	WaveToteNotFound
	WaveToteInUse
	WaveWarehouseNotFound
	WaveWarehouseNotAccepting
	WaveToteReleased
	WaveToteNotAccepting
	WaveToteRequired
	// WaveSorted 是分拣的一次投放记下了。
	WaveSorted
	// WaveSortPending 是这张请求在合并拣货的波次里拣出来的货还没分完，不能复核。
	WaveSortPending
	// WaveRuleNotFound 是这个仓没有登记过这条成波规则。
	WaveRuleNotFound
	// WaveRuleDeactivated 是这条成波规则最新的一版是停用的。
	WaveRuleDeactivated
	// WaveNothingToRelease 是按规则没有放出任何波次：没有圈到单，或圈到的都分配不出。
	WaveNothingToRelease
	// WaveAssigned 是波次派给了一个人（由人指派，或拣货的人自己领）。
	WaveAssigned
	// WaveUnassigned 是取消了指派。
	WaveUnassigned
	// WaveAssignedToOther 是这个波次派给了别人：领不了，它的任务也拣不了。
	WaveAssignedToOther
	// WaveDueRulesRun 是「放到点的规则」办了至少一个时点。
	WaveDueRulesRun
	// WaveNothingDue 是此刻没有到点而还没办过的时点。
	WaveNothingDue
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
	case WaveShortShipped:
		return "SHORT_SHIPPED"
	case WavePartialShipmentNotConfigured:
		return "PARTIAL_SHIPMENT_NOT_CONFIGURED"
	case WavePartialShipmentNotAllowed:
		return "PARTIAL_SHIPMENT_NOT_ALLOWED"
	case WavePrefetchStands:
		return "PREFETCH_STANDS"
	case WaveTotesRegistered:
		return "TOTES_REGISTERED"
	case WaveToteRetired:
		return "TOTE_RETIRED"
	case WaveToteNotFound:
		return "TOTE_NOT_FOUND"
	case WaveToteInUse:
		return "TOTE_IN_USE"
	case WaveWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case WaveWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case WaveToteReleased:
		return "TOTE_RELEASED"
	case WaveToteNotAccepting:
		return "TOTE_NOT_ACCEPTING"
	case WaveToteRequired:
		return "TOTE_REQUIRED"
	case WaveSorted:
		return "SORTED"
	case WaveSortPending:
		return "SORT_PENDING"
	case WaveRuleNotFound:
		return "WAVE_RULE_NOT_FOUND"
	case WaveRuleDeactivated:
		return "WAVE_RULE_DEACTIVATED"
	case WaveNothingToRelease:
		return "NOTHING_TO_RELEASE"
	case WaveAssigned:
		return "ASSIGNED"
	case WaveUnassigned:
		return "UNASSIGNED"
	case WaveAssignedToOther:
		return "WAVE_ASSIGNED_TO_OTHER"
	case WaveDueRulesRun:
		return "DUE_RULES_RUN"
	case WaveNothingDue:
		return "NOTHING_DUE"
	default:
		return ""
	}
}

// WaveResult 是一次放波次或拣货。Tasks 只在分配或重放时有。
type WaveResult struct {
	outcome WaveOutcome
	tasks   []ports.PickTask
	// exceptions 只在选了「放行能放的」的放波次里有：点名了而没放进波次的请求。
	exceptions []ReleaseException
	// assignee 只在派工、领波次与因派工被拒的答复里有：这个波次此刻派给了谁。
	assignee string
}

// Assignee 是这个波次此刻派给了谁；与派工无关的答复为空。
func (result WaveResult) Assignee() string { return result.assignee }

// Exceptions 是这一次放波次点名了而没放进波次的请求和各自的缘由。
func (result WaveResult) Exceptions() []ReleaseException {
	return append([]ReleaseException(nil), result.exceptions...)
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
	// BatchPick 是合并拣货：拣货的人按库位、商品把几张请求的货合在一起拿，拿回来再按请求分拣。
	// 不选是逐单拣。分配与任务不因它而变——一条任务仍是某张请求在某个库位取几件。
	BatchPick bool
	// Partial 是「放行能放的」：点名的请求里有分配不出、不是已预占或找不到的，跳过它们、放行其余的，
	// 跳过的在答复里逐张带出缘由。不选时有一张不行整个波次不放。
	Partial bool
}

// PickCommand 是一条拣货事实。实拣可以少于任务数量。
type PickCommand struct {
	Tenant    domain.TenantID
	FactID    domain.PickFactID
	TaskID    domain.PickTaskID
	Quantity  int64
	Target    domain.LocationCode
	Warehouse domain.WarehouseCode
	// Tote 是拣货的人把这一格的货放进去的周转箱。没带就是没用箱。
	Tote domain.ToteCode
	// Operator 是拣货的人的操作者标识，取自作业事实的认证。波次派给了别人时，他拣这个波次里待执行的任务不成立。
	// 为空是调用方没有交出是谁在拣，这时不按派工拦。
	Operator string
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
	// toteWarehouses 缺席时周转箱登记答未配置。
	toteWarehouses ports.WarehouseLookup
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
		if command.Partial {
			return handler.replayPartialRelease(ctx, command, existing)
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	if _, adopted, err := handler.waves.LoadStrategy(ctx, command.Tenant); err != nil {
		return WaveResult{}, fmt.Errorf("release wave: %w", err)
	} else if !adopted {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	var tasks []ports.PickTask
	// members 是放进这个波次的请求。不选「放行能放的」时它就是点名的全部：有一张不行整个波次不放。
	// 选了，不行的那几张记进 exceptions、留在原处，放行其余的。
	var members []domain.OutboundRequestID
	var exceptions []ReleaseException
	owners := map[string]domain.OwnerAccountID{}
	// 同一波次里先分走的数量还没落成拣货任务，后一份请求再读占用账看不见。
	taken := map[string]int64{}
	for _, id := range command.Requests {
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
		if !found || request.Warehouse() != command.Warehouse {
			if !command.Partial {
				return WaveResult{outcome: WaveNotFound}, nil
			}
			exceptions = append(exceptions, ReleaseException{Request: id, Reason: ReleaseNotFound})
			continue
		}
		if request.Status() != domain.OutboundReserved {
			if !command.Partial {
				return WaveResult{outcome: WaveStatusMismatch}, nil
			}
			exceptions = append(exceptions, ReleaseException{Request: id, Reason: ReleaseNotReserved})
			continue
		}
		// 分配不出时 allocate 已经往 taken 里记了一半，放行其余的之前照分配之前的那一份还原。
		before := maps.Clone(taken)
		allocated, err := handler.allocate(ctx, command, request, taken)
		if err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
		if allocated == nil {
			if !command.Partial {
				return WaveResult{outcome: WaveContentInvalid}, nil
			}
			taken = before
			exceptions = append(exceptions, ReleaseException{Request: id, Reason: ReleaseNotAllocatable})
			continue
		}
		owners[request.Owner().String()] = request.Owner()
		members = append(members, id)
		tasks = append(tasks, allocated...)
	}
	at := handler.clock.Now()
	for _, exception := range exceptions {
		if exception.Reason != ReleaseNotAllocatable {
			continue
		}
		if err := handler.waves.SaveAllocationException(ctx, command.Tenant, exception.Request, ports.AllocationExceptionFromWave, command.ID.String(), at); err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
	}
	if len(members) == 0 {
		return WaveResult{outcome: WaveNothingToRelease, exceptions: exceptions}, nil
	}
	for _, owner := range owners {
		if err := handler.ledger.SerializePosting(ctx, command.Tenant, owner, command.Warehouse); err != nil {
			return WaveResult{}, fmt.Errorf("release wave: %w", err)
		}
	}
	// 预取只对放进了波次的请求做：例外的那几张没有任务。
	command.Requests = members
	if err := handler.waves.SaveWave(ctx, command.Tenant, command.ID, command.Warehouse, command.Operator, command.Prefetch, command.BatchPick, at, members, tasks); err != nil {
		return WaveResult{}, fmt.Errorf("release wave: %w", err)
	}
	if err := handler.prefetchRequests(ctx, command, tasks); err != nil {
		return WaveResult{}, err
	}
	return WaveResult{outcome: WaveAllocated, tasks: tasks, exceptions: exceptions}, nil
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
	// 派工只管待执行的任务：作废之后才拣出来的（上面那一支）照记——货已经在手里了，要有回库任务接住它。
	// 任务所在的库区单独派了人就认那个人，没有才认整个波次派给的人。
	if command.Operator != "" {
		assignee, err := handler.pickAssignee(ctx, command.Tenant, owning.Warehouse(), task)
		if err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		if assignee != "" && assignee != command.Operator {
			return WaveResult{outcome: WaveAssignedToOther, assignee: assignee}, nil
		}
	}
	// 拣到 0 件也是一个结果：人到了库位、一件都没有。它照少拣办——记差异、换别的库位补、库位等盘点——只是没有货
	// 移进待出库，所以下面放货位、周转箱、入账三步都不走。不收它，这条任务就只能一直留在队里。
	if command.Quantity < 0 || command.Quantity > task.Quantity-task.Picked {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	empty := command.Quantity == 0
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, task.Request)
	if err != nil || !found {
		return WaveResult{}, fmt.Errorf("pick: %w", err)
	}
	if command.Warehouse.String() != "" && command.Warehouse != request.Warehouse() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	if !empty {
		place, found, err := handler.locations.LoadLatestLocation(ctx, command.Tenant, request.Warehouse(), command.Target)
		if err != nil || !found || place.Content().Deactivated() {
			return WaveResult{outcome: WaveContentInvalid}, nil
		}
		zone, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, request.Warehouse(), place.Content().ZoneCode())
		if err != nil || !found || zone.Content().Deactivated() || zone.Content().Usage() != domain.ZoneUsageOutboundStaging {
			return WaveResult{outcome: WaveContentInvalid}, nil
		}
	}
	// 箱在拣货入账之前绑：认不得的箱、绑着别的单的箱，这一笔拣货整份不成立，货不记进待出库。
	// 同一张单可以用几个箱，一个箱同一时刻只装一张单。
	switch {
	case empty:
		// 一件都没拣到时不绑箱、也不要求箱：没有货放进去，绑了只是白占一个箱。
	case command.Tote.String() != "":
		refused, err := handler.bindTote(ctx, command.Tenant, request, command.Tote)
		if err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		if refused != WaveOutcomeInvalid {
			return WaveResult{outcome: refused}, nil
		}
	default:
		// 租户给这个仓登记了「拣货必须用周转箱」时，不带箱的这一笔不成立。没登记这条规则就不要求。
		// 货合在一起拿的波次不受这一条管：箱是用来把各张单的货分开的，而单品单件波次与合并拣货的波次，
		// 拣货时一个容器里本来就装着好几张单的货，绑不到其中哪一张上——前者到复核打包扫货配单，后者到分拣才按单分箱。
		required, err := handler.toteRequired(ctx, command.Tenant, request.Warehouse())
		if err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		if required {
			merged, err := handler.waves.WavePicksMerged(ctx, command.Tenant, task.WaveID)
			if err != nil {
				return WaveResult{}, fmt.Errorf("pick: %w", err)
			}
			if !merged {
				return WaveResult{outcome: WaveToteRequired}, nil
			}
		}
	}
	if !empty {
		if err := handler.postPick(ctx, request, task, command); err != nil {
			return WaveResult{}, fmt.Errorf("pick: %w", err)
		}
		task.Staging = command.Target
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
	if err := handler.waves.SavePick(ctx, command.Tenant, task, status, differenceID, command.FactID.String(), short, handler.clock.Now()); err != nil {
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
	// 合并拣货的波次拣出来的货是几张请求混在一起的：没按请求分进箱之前，复核台上数不出「这张请求的货」。
	// 这一次复核不记：分完再来，用的还是这个事实号。
	unsorted, err := handler.waves.UnsortedOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("check outbound: %w", err)
	}
	if unsorted > 0 {
		return WaveResult{outcome: WaveSortPending}, nil
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
	// 货都装进包裹了，箱空出来。
	if err := handler.waves.ReleaseTotes(ctx, command.Tenant, command.Request, handler.clock.Now()); err != nil {
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
	// 少拣留下的差异还没了结的库位，这种货不再分：账上说还有，拣货的人在那里确认过没有，等盘点盘过再说。
	spots, err := handler.waves.ShortSpots(ctx, command.Tenant, request.Owner(), command.Warehouse)
	if err != nil {
		return nil, err
	}
	short := map[string]struct{}{}
	for _, spot := range spots {
		if spot.Goods == reserved.Goods {
			short[spot.Location.String()] = struct{}{}
		}
	}
	var cells []rankedCell
	for _, row := range rows {
		if row.GoodsCode != reserved.Goods.String() || row.State != reserved.State.String() {
			continue
		}
		if _, emptied := short[row.LocationCode]; emptied {
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
