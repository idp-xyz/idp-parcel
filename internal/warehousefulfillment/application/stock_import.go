package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 建账导入的提交与入账（UC-WF-006）。批准与驳回的口、批准职责规则在 UC-WF-011。
// 这里的 Post 与 Reject 是批准成立或驳回之后的状态迁移，调用方先判授予与职责规则。

type StockImportOutcome uint8

const (
	StockImportOutcomeInvalid StockImportOutcome = iota
	StockImportSubmitted
	StockImportExistingResult
	StockImportSourceConflict
	StockImportWarehouseNotFound
	StockImportWarehouseNotAccepting
	StockImportContentInvalid
	StockImportPosted
	StockImportNoLongerValid
	StockImportRejected
	StockImportStatusMismatch
	StockImportNotFound
)

func (outcome StockImportOutcome) String() string {
	switch outcome {
	case StockImportSubmitted:
		return "SUBMITTED"
	case StockImportExistingResult:
		return "EXISTING_RESULT"
	case StockImportSourceConflict:
		return "SOURCE_CONFLICT"
	case StockImportWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case StockImportWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	case StockImportContentInvalid:
		return "CONTENT_INVALID"
	case StockImportPosted:
		return "POSTED"
	case StockImportNoLongerValid:
		return "NO_LONGER_VALID"
	case StockImportRejected:
		return "REJECTED"
	case StockImportStatusMismatch:
		return "STATUS_MISMATCH"
	case StockImportNotFound:
		return "NOT_FOUND"
	default:
		return ""
	}
}

// StockImportIssueKind 是一行不成立的种类。整份要么都成立，要么逐行指出、一行都不落。
type StockImportIssueKind uint8

const (
	StockImportIssueInvalid StockImportIssueKind = iota
	StockImportIssueQuantityNotPositive
	StockImportIssueLocationNotRegistered
	StockImportIssueLocationDeactivated
	StockImportIssueZoneIncompatible
	StockImportIssueGoodsNotRegistered
	StockImportIssueUnitNotDeclared
	StockImportIssueBatchNumberRequired
	StockImportIssueBatchNumberNotTracked
	StockImportIssueExpiryRequired
	StockImportIssueExpiryNotTracked
	StockImportIssueLabelAliasNotValid
	StockImportIssueDeclaredInboundAfterSubmission
	StockImportIssueCartonContentsEmpty
	StockImportIssueCartonNumberRepeated
	StockImportIssueCartonNumberOnHand
	StockImportIssueCartonGoodsNotRegistered
	StockImportIssueCartonQuantityNotPositive
)

func (kind StockImportIssueKind) String() string {
	switch kind {
	case StockImportIssueQuantityNotPositive:
		return "QUANTITY_NOT_POSITIVE"
	case StockImportIssueLocationNotRegistered:
		return "LOCATION_NOT_REGISTERED"
	case StockImportIssueLocationDeactivated:
		return "LOCATION_DEACTIVATED"
	case StockImportIssueZoneIncompatible:
		return "ZONE_INCOMPATIBLE"
	case StockImportIssueGoodsNotRegistered:
		return "GOODS_NOT_REGISTERED"
	case StockImportIssueUnitNotDeclared:
		return "UNIT_NOT_DECLARED"
	case StockImportIssueBatchNumberRequired:
		return "BATCH_NUMBER_REQUIRED"
	case StockImportIssueBatchNumberNotTracked:
		return "BATCH_NUMBER_NOT_TRACKED"
	case StockImportIssueExpiryRequired:
		return "EXPIRY_REQUIRED"
	case StockImportIssueExpiryNotTracked:
		return "EXPIRY_NOT_TRACKED"
	case StockImportIssueLabelAliasNotValid:
		return "LABEL_ALIAS_NOT_VALID"
	case StockImportIssueDeclaredInboundAfterSubmission:
		return "DECLARED_INBOUND_AFTER_SUBMISSION"
	case StockImportIssueCartonContentsEmpty:
		return "CARTON_CONTENTS_EMPTY"
	case StockImportIssueCartonNumberRepeated:
		return "CARTON_NUMBER_REPEATED"
	case StockImportIssueCartonNumberOnHand:
		return "CARTON_NUMBER_ON_HAND"
	case StockImportIssueCartonGoodsNotRegistered:
		return "CARTON_GOODS_NOT_REGISTERED"
	case StockImportIssueCartonQuantityNotPositive:
		return "CARTON_QUANTITY_NOT_POSITIVE"
	default:
		return ""
	}
}

// StockImportLineSide 指出不合落在商品行还是整箱行。
type StockImportLineSide uint8

const (
	StockImportLineSideInvalid StockImportLineSide = iota
	StockImportLineGoods
	StockImportLineCarton
)

func (side StockImportLineSide) String() string {
	switch side {
	case StockImportLineGoods:
		return "GOODS"
	case StockImportLineCarton:
		return "CARTON"
	default:
		return ""
	}
}

type StockImportIssue struct {
	Side StockImportLineSide
	Line int
	Kind StockImportIssueKind
}

type StockImportResult struct {
	outcome   StockImportOutcome
	item      domain.StockImport
	hasItem   bool
	reason    WarehouseNotAcceptingReason
	status    domain.StockImportStatus
	hasStatus bool
	issues    []StockImportIssue
}

func (result StockImportResult) Outcome() StockImportOutcome { return result.outcome }
func (result StockImportResult) Import() (domain.StockImport, bool) {
	return result.item, result.hasItem
}
func (result StockImportResult) NotAcceptingReason() (WarehouseNotAcceptingReason, bool) {
	return result.reason, result.outcome == StockImportWarehouseNotAccepting
}
func (result StockImportResult) Status() (domain.StockImportStatus, bool) {
	return result.status, result.hasStatus
}
func (result StockImportResult) Issues() []StockImportIssue {
	out := make([]StockImportIssue, len(result.issues))
	copy(out, result.issues)
	return out
}

type SubmitStockImportCommand struct {
	Tenant    domain.TenantID
	ID        domain.StockImportID
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	Reason    domain.StockImportReason
	Goods     []domain.StockImportGoodsLine
	Cartons   []domain.StockImportCartonLine
	Submitter domain.Registrant
}

func (command SubmitStockImportCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.ID.String() == "":
		return errors.New("缺导入标识")
	case command.Owner.String() == "":
		return errors.New("缺货主")
	case command.Warehouse.String() == "":
		return errors.New("缺仓")
	case command.Reason.String() == "":
		return errors.New("缺原因")
	case command.Submitter.String() == "":
		return errors.New("缺提交操作者")
	case len(command.Goods) == 0 && len(command.Cartons) == 0:
		return errors.New("至少一行")
	default:
		return nil
	}
}

type PostStockImportCommand struct {
	Tenant domain.TenantID
	ID     domain.StockImportID
	Poster domain.Registrant
}

type RejectStockImportCommand struct {
	Tenant   domain.TenantID
	ID       domain.StockImportID
	Rejector domain.Registrant
	Note     string
}

// StockImportHandler 提交建账导入，并在批准成立或驳回时改状态。事务边界在调用方。
type StockImportHandler struct {
	imports    ports.StockImportStore
	owners     ports.OwnerAccountLookup
	warehouses ports.WarehouseLookup
	locations  ports.StockLocationLookup
	zones      ports.ZoneLookup
	goods      ports.StockGoodsLookup
	cartons    ports.CartonLedger
	identities ports.CartonIdentities
	stock      *PostStockMovementHandler
	clock      ports.Clock
}

func NewStockImportHandler(
	imports ports.StockImportStore,
	owners ports.OwnerAccountLookup,
	warehouses ports.WarehouseLookup,
	locations ports.StockLocationLookup,
	zones ports.ZoneLookup,
	goods ports.StockGoodsLookup,
	cartons ports.CartonLedger,
	identities ports.CartonIdentities,
	stock *PostStockMovementHandler,
	clock ports.Clock,
) *StockImportHandler {
	return &StockImportHandler{
		imports: imports, owners: owners, warehouses: warehouses, locations: locations, zones: zones,
		goods: goods, cartons: cartons, identities: identities, stock: stock, clock: clock,
	}
}

// Submit 落一份待批准的导入，不改账。判定次序（UC-WF-006）：
//  1. 核货主在本租户：查不到交 ErrOwnerAccountNotInTenant，读不动交 error；
//  2. 按导入标识认重放：内容相同答 EXISTING_RESULT 并回显此刻状态，内容不同答 SOURCE_CONFLICT；
//  3. 仓没登记答 WAREHOUSE_NOT_FOUND；停用或代理仓答 WAREHOUSE_NOT_ACCEPTING；
//  4. 逐行收齐不合，任一行不成立整份不落；
//  5. 落成待批准。
func (handler *StockImportHandler) Submit(ctx context.Context, command SubmitStockImportCommand) (StockImportResult, error) {
	if err := command.assemblyError(); err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	registered, err := handler.owners.OwnerAccountRegistered(ctx, command.Tenant, command.Owner)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	if !registered {
		return StockImportResult{}, ErrOwnerAccountNotInTenant
	}
	if err := handler.imports.Hold(ctx, command.Tenant, command.ID); err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	existing, found, err := handler.imports.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	if found {
		if existing.SameContent(command.Owner, command.Warehouse, command.Reason, command.Goods, command.Cartons) {
			return StockImportResult{outcome: StockImportExistingResult, item: existing, hasItem: true}, nil
		}
		return StockImportResult{outcome: StockImportSourceConflict}, nil
	}
	warehouse, warehouseFound, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	if !warehouseFound {
		return StockImportResult{outcome: StockImportWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() != domain.WarehouseKindSelfOperated {
		return StockImportResult{outcome: StockImportWarehouseNotAccepting, reason: WarehouseNotAcceptingAgent}, nil
	}
	if warehouse.Content().Deactivated() {
		return StockImportResult{outcome: StockImportWarehouseNotAccepting, reason: WarehouseNotAcceptingDeactivated}, nil
	}
	now := handler.clock.Now()
	issues, err := handler.screen(ctx, command.Tenant, command.Owner, command.Warehouse, command.Goods, command.Cartons, now, now)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	if len(issues) > 0 {
		return StockImportResult{outcome: StockImportContentInvalid, issues: issues}, nil
	}
	pending, err := domain.NewPendingStockImport(
		command.Tenant, command.ID, command.Owner, command.Warehouse, command.Reason,
		command.Goods, command.Cartons, command.Submitter, now,
	)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	if err := handler.imports.SavePending(ctx, pending); err != nil {
		return StockImportResult{}, fmt.Errorf("submit stock import: %w", err)
	}
	return StockImportResult{outcome: StockImportSubmitted, item: pending, hasItem: true}, nil
}

// Post 在批准成立时重判各行并同笔记账。停用的仓不挡已经提交的导入。
// 有行已不成立答 NO_LONGER_VALID，导入留在待批准。
func (handler *StockImportHandler) Post(ctx context.Context, command PostStockImportCommand) (StockImportResult, error) {
	if command.Tenant.String() == "" || command.ID.String() == "" || command.Poster.String() == "" {
		return StockImportResult{}, errors.New("post stock import: 缺租户、导入标识或批准人")
	}
	if err := handler.imports.Hold(ctx, command.Tenant, command.ID); err != nil {
		return StockImportResult{}, fmt.Errorf("post stock import: %w", err)
	}
	item, found, err := handler.imports.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("post stock import: %w", err)
	}
	if !found {
		return StockImportResult{outcome: StockImportNotFound}, nil
	}
	switch item.Status() {
	case domain.StockImportPosted:
		return StockImportResult{outcome: StockImportExistingResult, item: item, hasItem: true}, nil
	case domain.StockImportRejected:
		return StockImportResult{outcome: StockImportStatusMismatch, status: item.Status(), hasStatus: true, item: item, hasItem: true}, nil
	case domain.StockImportPending:
	default:
		return StockImportResult{}, fmt.Errorf("post stock import: unexpected status %s", item.Status())
	}
	if len(item.CartonLines()) > 0 {
		if err := handler.cartons.Serialize(ctx, command.Tenant, item.Owner(), item.Warehouse()); err != nil {
			return StockImportResult{}, fmt.Errorf("post stock import: %w", err)
		}
	}
	now := handler.clock.Now()
	issues, err := handler.screen(ctx, command.Tenant, item.Owner(), item.Warehouse(), item.GoodsLines(), item.CartonLines(), now, item.SubmittedAt())
	if err != nil {
		return StockImportResult{}, fmt.Errorf("post stock import: %w", err)
	}
	if len(issues) > 0 {
		return StockImportResult{outcome: StockImportNoLongerValid, issues: issues, item: item, hasItem: true}, nil
	}
	var goodsMove domain.StockMovementID
	if len(item.GoodsLines()) > 0 {
		movement, err := handler.postGoods(ctx, command.Tenant, item, now)
		if err != nil {
			return StockImportResult{}, fmt.Errorf("post stock import: %w", err)
		}
		goodsMove = movement
	}
	var cartonMove domain.CartonMovementID
	if len(item.CartonLines()) > 0 {
		movement, err := handler.postCartons(ctx, item, now)
		if err != nil {
			return StockImportResult{}, err
		}
		cartonMove = movement
	}
	posted, err := item.Post(command.Poster, now, goodsMove, cartonMove)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("post stock import: %w", err)
	}
	if err := handler.imports.MarkPosted(ctx, posted); err != nil {
		return StockImportResult{}, fmt.Errorf("post stock import: %w", err)
	}
	return StockImportResult{outcome: StockImportPosted, item: posted, hasItem: true}, nil
}

// Reject 把待批准的导入收成已驳回，不改账。已驳回的再驳回答 EXISTING_RESULT。
func (handler *StockImportHandler) Reject(ctx context.Context, command RejectStockImportCommand) (StockImportResult, error) {
	if command.Tenant.String() == "" || command.ID.String() == "" || command.Rejector.String() == "" {
		return StockImportResult{}, errors.New("reject stock import: 缺租户、导入标识或驳回人")
	}
	if err := handler.imports.Hold(ctx, command.Tenant, command.ID); err != nil {
		return StockImportResult{}, fmt.Errorf("reject stock import: %w", err)
	}
	item, found, err := handler.imports.Load(ctx, command.Tenant, command.ID)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("reject stock import: %w", err)
	}
	if !found {
		return StockImportResult{outcome: StockImportNotFound}, nil
	}
	switch item.Status() {
	case domain.StockImportRejected:
		return StockImportResult{outcome: StockImportExistingResult, item: item, hasItem: true}, nil
	case domain.StockImportPosted:
		return StockImportResult{outcome: StockImportStatusMismatch, status: item.Status(), hasStatus: true, item: item, hasItem: true}, nil
	case domain.StockImportPending:
	default:
		return StockImportResult{}, fmt.Errorf("reject stock import: unexpected status %s", item.Status())
	}
	rejected, err := item.Reject(command.Rejector, handler.clock.Now(), command.Note)
	if err != nil {
		return StockImportResult{}, fmt.Errorf("reject stock import: %w", err)
	}
	if err := handler.imports.MarkRejected(ctx, rejected); err != nil {
		return StockImportResult{}, fmt.Errorf("reject stock import: %w", err)
	}
	return StockImportResult{outcome: StockImportRejected, item: rejected, hasItem: true}, nil
}

func (handler *StockImportHandler) postGoods(ctx context.Context, tenant domain.TenantID, item domain.StockImport, postedAt time.Time) (domain.StockMovementID, error) {
	source, err := domain.NewSourceFactReference(item.ID().String())
	if err != nil {
		return domain.StockMovementID{}, err
	}
	lines := make([]domain.StockLineRequest, 0, len(item.GoodsLines()))
	for _, line := range item.GoodsLines() {
		cell, err := domain.NewStockCell(domain.StockCellSpec{
			Location: line.Location(), Goods: line.Goods(), Unit: line.Unit(), State: line.State(),
			Alias: line.Alias(), BatchNumber: line.BatchNumber(), Expiry: line.Expiry(),
		})
		if err != nil {
			return domain.StockMovementID{}, err
		}
		lines = append(lines, domain.NewStockLineRequest(domain.StockIn, line.Quantity(), cell, line.DeclaredInboundAt()))
	}
	request := domain.NewStockMovementRequest(item.Owner(), item.Warehouse(), domain.StockReasonTakeOnImport, source, postedAt, lines)
	posted, err := handler.stock.Post(ctx, PostStockMovementCommand{Tenant: tenant, Request: request})
	if err != nil {
		return domain.StockMovementID{}, err
	}
	switch posted.Outcome() {
	case PostStockPosted, PostStockAlreadyPosted:
		movement, ok := posted.Movement()
		if !ok {
			return domain.StockMovementID{}, errors.New("post stock import: 商品流水已入账却读不回")
		}
		return movement.ID(), nil
	case PostStockRefused:
		return domain.StockMovementID{}, errors.New("post stock import: 重判已通过的商品行被库存账拒绝")
	default:
		return domain.StockMovementID{}, fmt.Errorf("post stock import: stock outcome %s", posted.Outcome())
	}
}

func (handler *StockImportHandler) postCartons(ctx context.Context, item domain.StockImport, postedAt time.Time) (domain.CartonMovementID, error) {
	source, err := domain.NewSourceFactReference(item.ID().String())
	if err != nil {
		return domain.CartonMovementID{}, err
	}
	movementID, err := handler.identities.NextCartonMovementID()
	if err != nil {
		return domain.CartonMovementID{}, err
	}
	lines := make([]domain.CartonMovementLine, 0, len(item.CartonLines()))
	formed := make([]domain.CartonInboundBatch, 0, len(item.CartonLines()))
	for _, line := range item.CartonLines() {
		batchID, err := handler.identities.NextCartonInboundBatchID()
		if err != nil {
			return domain.CartonMovementID{}, err
		}
		inboundAt := line.DeclaredInboundAt()
		if inboundAt.IsZero() {
			inboundAt = postedAt
		}
		batch, err := domain.NewCartonInboundBatch(item.Tenant(), batchID, item.Owner(), item.Warehouse(), line.Number(), inboundAt, movementID)
		if err != nil {
			return domain.CartonMovementID{}, err
		}
		movementLine, err := domain.NewCartonMovementLine(line.Location(), line.Number(), domain.CartonNumber{}, batchID)
		if err != nil {
			return domain.CartonMovementID{}, err
		}
		formed = append(formed, batch)
		lines = append(lines, movementLine)
	}
	movement, err := domain.NewCartonMovement(
		item.Tenant(), movementID, item.Owner(), item.Warehouse(), domain.CartonMovementTakeOnImport,
		source, postedAt, postedAt, lines,
	)
	if err != nil {
		return domain.CartonMovementID{}, err
	}
	if err := handler.cartons.Append(ctx, movement, formed); err != nil {
		return domain.CartonMovementID{}, fmt.Errorf("post stock import: %w", err)
	}
	return movementID, nil
}

// screen 收齐全部不合。aliasAt 是贴标别名按哪一刻的册判；inboundDeadline 是声明的入库时间不得晚于的那一刻（提交时刻）。
// 仓后来停用不在这里判：提交时已经挡过，已经提交的导入照常可以入账。
func (handler *StockImportHandler) screen(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	goods []domain.StockImportGoodsLine,
	cartons []domain.StockImportCartonLine,
	aliasAt time.Time,
	inboundDeadline time.Time,
) ([]StockImportIssue, error) {
	var issues []StockImportIssue
	for index, line := range goods {
		more, err := handler.screenGoods(ctx, tenant, owner, warehouse, index, line, aliasAt, inboundDeadline)
		if err != nil {
			return nil, err
		}
		issues = append(issues, more...)
	}
	seen := map[string]struct{}{}
	onHand := map[string]bool{}
	if len(cartons) > 0 {
		numbers, err := handler.cartons.OnHandCartons(ctx, tenant, owner, warehouse)
		if err != nil {
			return nil, err
		}
		for _, number := range numbers {
			onHand[number.String()] = true
		}
	}
	for index, line := range cartons {
		if _, duplicate := seen[line.Number().String()]; duplicate {
			issues = append(issues, StockImportIssue{Side: StockImportLineCarton, Line: index, Kind: StockImportIssueCartonNumberRepeated})
		}
		seen[line.Number().String()] = struct{}{}
		if onHand[line.Number().String()] {
			issues = append(issues, StockImportIssue{Side: StockImportLineCarton, Line: index, Kind: StockImportIssueCartonNumberOnHand})
		}
		more, err := handler.screenCarton(ctx, tenant, owner, warehouse, index, line, inboundDeadline)
		if err != nil {
			return nil, err
		}
		issues = append(issues, more...)
	}
	return issues, nil
}

func (handler *StockImportHandler) screenGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	index int,
	line domain.StockImportGoodsLine,
	aliasAt time.Time,
	inboundDeadline time.Time,
) ([]StockImportIssue, error) {
	var issues []StockImportIssue
	add := func(kind StockImportIssueKind) {
		issues = append(issues, StockImportIssue{Side: StockImportLineGoods, Line: index, Kind: kind})
	}
	if line.Quantity() <= 0 {
		add(StockImportIssueQuantityNotPositive)
	}
	if !line.DeclaredInboundAt().IsZero() && line.DeclaredInboundAt().After(inboundDeadline) {
		add(StockImportIssueDeclaredInboundAfterSubmission)
	}
	location, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, line.Location())
	if err != nil {
		return nil, err
	}
	if !found {
		add(StockImportIssueLocationNotRegistered)
	} else if location.Content().Deactivated() {
		add(StockImportIssueLocationDeactivated)
	} else if err := handler.screenZone(ctx, tenant, warehouse, location.Content().ZoneCode(), line.State(), false, add); err != nil {
		return nil, err
	}
	revision, goodsFound, err := handler.goods.LoadLatestGoods(ctx, tenant, owner, line.Goods())
	if err != nil {
		return nil, err
	}
	if !goodsFound {
		add(StockImportIssueGoodsNotRegistered)
		return issues, nil
	}
	if !unitDeclared(revision, line.Unit()) {
		add(StockImportIssueUnitNotDeclared)
	}
	content := revision.Content()
	switch {
	case content.TrackBatch() && !line.BatchNumber().Given():
		add(StockImportIssueBatchNumberRequired)
	case !content.TrackBatch() && line.BatchNumber().Given():
		add(StockImportIssueBatchNumberNotTracked)
	}
	switch {
	case content.TrackExpiry() && !line.Expiry().Given():
		add(StockImportIssueExpiryRequired)
	case !content.TrackExpiry() && line.Expiry().Given():
		add(StockImportIssueExpiryNotTracked)
	}
	if line.Alias().String() != "" {
		atRevision, atFound, err := handler.goods.LoadGoodsAt(ctx, tenant, owner, line.Goods(), aliasAt)
		if err != nil {
			return nil, err
		}
		if !atFound || !labelValid(atRevision, line.Alias(), aliasAt) {
			add(StockImportIssueLabelAliasNotValid)
		}
	}
	return issues, nil
}

func (handler *StockImportHandler) screenCarton(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	index int,
	line domain.StockImportCartonLine,
	inboundDeadline time.Time,
) ([]StockImportIssue, error) {
	var issues []StockImportIssue
	add := func(kind StockImportIssueKind) {
		issues = append(issues, StockImportIssue{Side: StockImportLineCarton, Line: index, Kind: kind})
	}
	if len(line.Contents()) == 0 {
		add(StockImportIssueCartonContentsEmpty)
	}
	if !line.DeclaredInboundAt().IsZero() && line.DeclaredInboundAt().After(inboundDeadline) {
		add(StockImportIssueDeclaredInboundAfterSubmission)
	}
	location, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, line.Location())
	if err != nil {
		return nil, err
	}
	if !found {
		add(StockImportIssueLocationNotRegistered)
	} else if location.Content().Deactivated() {
		add(StockImportIssueLocationDeactivated)
	} else if err := handler.screenZone(ctx, tenant, warehouse, location.Content().ZoneCode(), domain.StockStateInvalid, true, add); err != nil {
		return nil, err
	}
	for _, content := range line.Contents() {
		if content.Quantity() <= 0 {
			add(StockImportIssueCartonQuantityNotPositive)
		}
		_, goodsFound, err := handler.goods.LoadLatestGoods(ctx, tenant, owner, content.Goods())
		if err != nil {
			return nil, err
		}
		if !goodsFound {
			add(StockImportIssueCartonGoodsNotRegistered)
		}
	}
	return issues, nil
}

func (handler *StockImportHandler) screenZone(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	zoneCode domain.ZoneCode,
	state domain.StockState,
	carton bool,
	add func(StockImportIssueKind),
) error {
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, zoneCode)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("库位已登记，它的库区 %s 读不到", zoneCode)
	}
	usage := zone.Content().Usage()
	if carton {
		if !domain.CartonStockZoneAllowed(usage) {
			add(StockImportIssueZoneIncompatible)
		}
		return nil
	}
	if !state.AllowedIn(usage) {
		add(StockImportIssueZoneIncompatible)
	}
	return nil
}
