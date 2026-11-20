package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterInternalJobOutcome 是库内作业登记的结果。对不上账仍是已登记，不是拒绝。
type RegisterInternalJobOutcome uint8

const (
	RegisterInternalJobOutcomeInvalid RegisterInternalJobOutcome = iota
	RegisterInternalJobRegistered
	RegisterInternalJobShortageRegistered
	RegisterInternalJobExisting
	RegisterInternalJobConflict
	RegisterInternalJobContentInvalid
	RegisterInternalJobWarehouseNotFound
	RegisterInternalJobWarehouseNotAccepting
)

func (outcome RegisterInternalJobOutcome) String() string {
	switch outcome {
	case RegisterInternalJobRegistered:
		return "REGISTERED"
	case RegisterInternalJobShortageRegistered:
		return "SHORTAGE_REGISTERED"
	case RegisterInternalJobExisting:
		return "EXISTING_RESULT"
	case RegisterInternalJobConflict:
		return "CONTENT_CONFLICT"
	case RegisterInternalJobContentInvalid:
		return "CONTENT_INVALID"
	case RegisterInternalJobWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case RegisterInternalJobWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	default:
		return ""
	}
}

// RegisterInternalJobResult 携带落点。事实只在已登记、短少已登记与已有结果上有。
type RegisterInternalJobResult struct {
	outcome RegisterInternalJobOutcome
	job     domain.RecordedInternalJob
	hasJob  bool
	issues  []domain.InternalJobIssue
}

func (result RegisterInternalJobResult) Outcome() RegisterInternalJobOutcome { return result.outcome }
func (result RegisterInternalJobResult) Job() (domain.RecordedInternalJob, bool) {
	return result.job, result.hasJob
}
func (result RegisterInternalJobResult) Issues() []domain.InternalJobIssue {
	return append([]domain.InternalJobIssue(nil), result.issues...)
}

// RegisterInternalJobCommand 是一次库内作业登记。租户、设备与操作者取自信封。种类由口决定，不从载荷另报。
type RegisterInternalJobCommand struct {
	Tenant         domain.TenantID
	FactID         domain.InternalJobFactID
	Kind           domain.InternalJobKind
	Device         domain.ReceiptDeviceID
	Operator       domain.ReceiptOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	Warehouse      domain.WarehouseCode
	Lines          []domain.InternalJobLine
}

func (command RegisterInternalJobCommand) submission() domain.InternalJobSubmission {
	return domain.InternalJobSubmission{
		Kind: command.Kind, Device: command.Device, Operator: command.Operator, OccurredAt: command.OccurredAt,
		DeviceSequence: command.DeviceSequence, Warehouse: command.Warehouse, Lines: command.Lines,
	}
}

func (command RegisterInternalJobCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.FactID.String() == "":
		return errors.New("缺事实身份")
	case command.Kind.String() == "":
		return errors.New("缺作业种类")
	case command.Device.String() == "":
		return errors.New("缺设备")
	case command.Operator.String() == "":
		return errors.New("缺操作者")
	case command.OccurredAt.IsZero():
		return errors.New("缺发生时间")
	case command.DeviceSequence < 1:
		return errors.New("缺设备序号")
	case command.Warehouse.String() == "":
		return errors.New("缺仓")
	case len(command.Lines) == 0:
		return errors.New("缺行")
	default:
		return nil
	}
}

// RegisterInternalJobHandler 把上架、移库、补货、形态转换、状态转换与短少照记入账。
// 事务边界在调用方。写口不因账上不够而拒：取得走的入账，其余成待处理的库存差异。
type RegisterInternalJobHandler struct {
	jobs          ports.InternalJobStore
	restores      ports.ConditionRestoreStore
	poster        *PostStockMovementHandler
	ledger        ports.StockLedger
	onHand        ports.StockOnHandRead
	cartons       ports.CartonLedger
	cartonIDs     ports.CartonIdentities
	warehouses    ports.WarehouseLookup
	locations     ports.StockLocationLookup
	zones         ports.ZoneLookup
	goods         ports.StockGoodsLookup
	holds         ports.StockHolds
	discrepancies ports.StockDiscrepancyIdentities
	clock         ports.Clock
}

func NewRegisterInternalJobHandler(
	jobs ports.InternalJobStore,
	restores ports.ConditionRestoreStore,
	poster *PostStockMovementHandler,
	ledger ports.StockLedger,
	onHand ports.StockOnHandRead,
	cartons ports.CartonLedger,
	cartonIDs ports.CartonIdentities,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	goods ports.StockGoodsLookup,
	holds ports.StockHolds,
	discrepancies ports.StockDiscrepancyIdentities,
	clock ports.Clock,
) *RegisterInternalJobHandler {
	return &RegisterInternalJobHandler{
		jobs: jobs, restores: restores, poster: poster, ledger: ledger, onHand: onHand, cartons: cartons, cartonIDs: cartonIDs,
		warehouses: warehouses, locations: locations, zones: zones, goods: goods, holds: holds,
		discrepancies: discrepancies, clock: clock,
	}
}

func (handler *RegisterInternalJobHandler) Register(ctx context.Context, command RegisterInternalJobCommand) (RegisterInternalJobResult, error) {
	if err := command.assemblyError(); err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	if err := handler.jobs.HoldFact(ctx, command.Tenant, command.FactID); err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	existing, found, err := handler.jobs.Load(ctx, command.Tenant, command.FactID)
	if err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	if found {
		if existing.SameContent(command.submission()) {
			return RegisterInternalJobResult{outcome: RegisterInternalJobExisting, job: existing, hasJob: true}, nil
		}
		return RegisterInternalJobResult{outcome: RegisterInternalJobConflict}, nil
	}
	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	if !found {
		return RegisterInternalJobResult{outcome: RegisterInternalJobWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return RegisterInternalJobResult{outcome: RegisterInternalJobWarehouseNotAccepting}, nil
	}
	issues, places, revisions, err := handler.screen(ctx, command)
	if err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	if len(issues) > 0 {
		return RegisterInternalJobResult{outcome: RegisterInternalJobContentInvalid, issues: issues}, nil
	}
	recorded, discrepancies, outbound, err := handler.post(ctx, command, places, revisions)
	if err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	if len(outbound) > 0 {
		return RegisterInternalJobResult{outcome: RegisterInternalJobContentInvalid, issues: outbound}, nil
	}
	job, err := domain.NewRecordedInternalJob(command.Tenant, command.FactID, command.submission(), handler.clock.Now(), recorded)
	if err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	if err := handler.jobs.Save(ctx, job, discrepancies); err != nil {
		return RegisterInternalJobResult{}, fmt.Errorf("register internal job: %w", err)
	}
	outcome := RegisterInternalJobRegistered
	if command.Kind == domain.InternalJobShortage {
		outcome = RegisterInternalJobShortageRegistered
	}
	return RegisterInternalJobResult{outcome: outcome, job: job, hasJob: true}, nil
}

type placeInfo struct {
	usage domain.ZoneUsage
}

func (handler *RegisterInternalJobHandler) screen(
	ctx context.Context,
	command RegisterInternalJobCommand,
) ([]domain.InternalJobIssue, map[string]placeInfo, map[string]domain.GoodsRevision, error) {
	places := map[string]placeInfo{}
	revisions := map[string]domain.GoodsRevision{}
	var issues []domain.InternalJobIssue
	for index, line := range command.Lines {
		more, err := handler.screenLine(ctx, command, index, line, places, revisions)
		if err != nil {
			return nil, nil, nil, err
		}
		issues = append(issues, more...)
	}
	return issues, places, revisions, nil
}

func (handler *RegisterInternalJobHandler) screenLine(
	ctx context.Context,
	command RegisterInternalJobCommand,
	index int,
	line domain.InternalJobLine,
	places map[string]placeInfo,
	revisions map[string]domain.GoodsRevision,
) ([]domain.InternalJobIssue, error) {
	var issues []domain.InternalJobIssue
	add := func(kind domain.InternalJobIssueKind) {
		issues = append(issues, domain.NewInternalJobIssue(index, kind))
	}
	if line.CartonLine() && line.Goods().String() != "" {
		add(domain.InternalJobIssueLineShape)
		return issues, nil
	}
	if line.CartonLine() {
		if command.Kind != domain.InternalJobMove && command.Kind != domain.InternalJobShortage {
			add(domain.InternalJobIssueLineShape)
			return issues, nil
		}
		if line.Target().String() == "" && command.Kind == domain.InternalJobMove {
			add(domain.InternalJobIssueLineShape)
			return issues, nil
		}
		if command.Kind == domain.InternalJobMove {
			usage, found, err := handler.place(ctx, command.Tenant, command.Warehouse, line.Target(), places)
			if err != nil {
				return nil, err
			}
			if !found {
				add(domain.InternalJobIssueLocationNotFound)
			} else if usage != domain.ZoneUsageReceivingStaging && usage != domain.ZoneUsageStorage {
				add(domain.InternalJobIssueCartonZone)
			}
		}
		return issues, nil
	}
	if line.Goods().String() == "" || line.Unit().String() == "" || line.State().String() == "" || line.Source().String() == "" {
		add(domain.InternalJobIssueLineShape)
		return issues, nil
	}
	if line.Quantity() <= 0 {
		add(domain.InternalJobIssueQuantityNotPositive)
	}
	revision, foundGoods, err := handler.goodsRevision(ctx, command.Tenant, line, revisions)
	if err != nil {
		return nil, err
	}
	if !foundGoods {
		add(domain.InternalJobIssueGoodsNotRegistered)
	} else {
		issues = append(issues, goodsIssues(index, revision, line)...)
	}
	sourceUsage, sourceFound, err := handler.place(ctx, command.Tenant, command.Warehouse, line.Source(), places)
	if err != nil {
		return nil, err
	}
	if !sourceFound {
		add(domain.InternalJobIssueLocationNotFound)
	} else if sourceUsage == domain.ZoneUsageOutboundStaging {
		add(domain.InternalJobIssueOutboundStaging)
	}
	switch command.Kind {
	case domain.InternalJobMove:
		more, err := handler.screenMove(ctx, command, index, line, places, revision, foundGoods)
		if err != nil {
			return nil, err
		}
		issues = append(issues, more...)
	case domain.InternalJobForm:
		if line.Target().String() != "" || line.ToUnit().String() == "" || line.ToQuantity() <= 0 {
			add(domain.InternalJobIssueLineShape)
			break
		}
		if foundGoods {
			issues = append(issues, conversionIssues(index, revision, line)...)
		}
	case domain.InternalJobCondition:
		if line.Target().String() != "" || line.ToState().String() == "" {
			add(domain.InternalJobIssueLineShape)
			break
		}
		if sourceFound && sourceUsage != domain.ZoneUsageReceivingStaging && sourceUsage != domain.ZoneUsageReturnStaging {
			add(domain.InternalJobIssueConditionNotAllowed)
		} else if sourceFound && !conditionAllowed(line.State(), line.ToState()) {
			add(domain.InternalJobIssueConditionNotAllowed)
		}
	case domain.InternalJobShortage:
		if line.Target().String() != "" || line.ToUnit().String() != "" || line.ToState().String() != "" {
			add(domain.InternalJobIssueLineShape)
		}
	default:
		add(domain.InternalJobIssueLineShape)
	}
	return issues, nil
}

func (handler *RegisterInternalJobHandler) screenMove(
	ctx context.Context,
	command RegisterInternalJobCommand,
	index int,
	line domain.InternalJobLine,
	places map[string]placeInfo,
	revision domain.GoodsRevision,
	foundGoods bool,
) ([]domain.InternalJobIssue, error) {
	var issues []domain.InternalJobIssue
	add := func(kind domain.InternalJobIssueKind) {
		issues = append(issues, domain.NewInternalJobIssue(index, kind))
	}
	if line.Target().String() == "" {
		add(domain.InternalJobIssueLineShape)
		return issues, nil
	}
	targetUsage, found, err := handler.place(ctx, command.Tenant, command.Warehouse, line.Target(), places)
	if err != nil {
		return nil, err
	}
	if !found {
		add(domain.InternalJobIssueLocationNotFound)
		return issues, nil
	}
	if targetUsage == domain.ZoneUsageOutboundStaging {
		add(domain.InternalJobIssueOutboundStaging)
	}
	destState := line.State()
	if targetUsage == domain.ZoneUsageDefective {
		destState = domain.StockStateDefective
	} else if !destState.AllowedIn(targetUsage) {
		add(domain.InternalJobIssueZoneIncompatible)
	}
	half := line.ToUnit().String() != "" || line.ToQuantity() != 0
	if half && (line.ToUnit().String() == "" || line.ToQuantity() <= 0) {
		add(domain.InternalJobIssueLineShape)
		return issues, nil
	}
	if line.ToUnit().String() == "" {
		return issues, nil
	}
	sourceUsage, sourceFound := places[line.Source().String()]
	if !sourceFound || sourceUsage.usage != domain.ZoneUsageStorage || targetUsage != domain.ZoneUsagePicking {
		add(domain.InternalJobIssueReplenishShape)
	}
	if foundGoods {
		issues = append(issues, conversionIssues(index, revision, line)...)
	}
	return issues, nil
}

func (handler *RegisterInternalJobHandler) place(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
	places map[string]placeInfo,
) (domain.ZoneUsage, bool, error) {
	if info, ok := places[code.String()]; ok {
		return info.usage, true, nil
	}
	location, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, code)
	if err != nil {
		return 0, false, err
	}
	if !found {
		return 0, false, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, location.Content().ZoneCode())
	if err != nil {
		return 0, false, err
	}
	if !found {
		return 0, false, fmt.Errorf("库位 %s 已登记，它的库区 %s 读不到", code, location.Content().ZoneCode())
	}
	usage := zone.Content().Usage()
	places[code.String()] = placeInfo{usage: usage}
	return usage, true, nil
}

func (handler *RegisterInternalJobHandler) goodsRevision(
	ctx context.Context,
	tenant domain.TenantID,
	line domain.InternalJobLine,
	revisions map[string]domain.GoodsRevision,
) (domain.GoodsRevision, bool, error) {
	key := line.Owner().String() + "\x1f" + line.Goods().String()
	if revision, ok := revisions[key]; ok {
		return revision, true, nil
	}
	revision, found, err := handler.goods.LoadLatestGoods(ctx, tenant, line.Owner(), line.Goods())
	if err != nil || !found {
		return domain.GoodsRevision{}, found, err
	}
	revisions[key] = revision
	return revision, true, nil
}

func goodsIssues(index int, revision domain.GoodsRevision, line domain.InternalJobLine) []domain.InternalJobIssue {
	var issues []domain.InternalJobIssue
	add := func(kind domain.InternalJobIssueKind) {
		issues = append(issues, domain.NewInternalJobIssue(index, kind))
	}
	content := revision.Content()
	if _, ok := unitFactor(revision, line.Unit()); !ok && line.Unit().String() != "" {
		add(domain.InternalJobIssueUnitNotDeclared)
	}
	switch {
	case content.TrackBatch() && !line.Batch().Given():
		add(domain.InternalJobIssueBatchRequired)
	case !content.TrackBatch() && line.Batch().Given():
		add(domain.InternalJobIssueBatchNotTracked)
	}
	switch {
	case content.TrackExpiry() && !line.Expiry().Given():
		add(domain.InternalJobIssueExpiryRequired)
	case !content.TrackExpiry() && line.Expiry().Given():
		add(domain.InternalJobIssueExpiryNotTracked)
	}
	if line.Alias().String() != "" && !platformAlias(content, line.Alias()) {
		add(domain.InternalJobIssueLabelAliasNotPlatform)
	}
	return issues
}

func conversionIssues(index int, revision domain.GoodsRevision, line domain.InternalJobLine) []domain.InternalJobIssue {
	var issues []domain.InternalJobIssue
	add := func(kind domain.InternalJobIssueKind) {
		issues = append(issues, domain.NewInternalJobIssue(index, kind))
	}
	if line.ToUnit() == line.Unit() {
		add(domain.InternalJobIssueSameUnit)
		return issues
	}
	from, fromOK := unitFactor(revision, line.Unit())
	to, toOK := unitFactor(revision, line.ToUnit())
	if !toOK {
		add(domain.InternalJobIssueUnitNotDeclared)
		return issues
	}
	if fromOK && line.Quantity() > 0 && line.ToQuantity() > 0 && !basesEqual(line.Quantity(), from, line.ToQuantity(), to) {
		add(domain.InternalJobIssueFactorsUnequal)
	}
	return issues
}

func conditionAllowed(from, to domain.StockState) bool {
	switch from {
	case domain.StockStatePendingInspection:
		return to == domain.StockStateGood || to == domain.StockStateDefective
	case domain.StockStateGood:
		return to == domain.StockStateDefective
	default:
		return false
	}
}

func platformAlias(content domain.GoodsContent, code domain.GoodsAliasCode) bool {
	for _, alias := range content.Aliases() {
		if alias.Code() == code && alias.Kind() == domain.GoodsAliasPlatformCode {
			return true
		}
	}
	return false
}
