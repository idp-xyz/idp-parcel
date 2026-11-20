package application_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// callLog 记下替身被调用的次序。记数必须先串行、再读，这个次序本身就是断言。
type callLog struct {
	mu  sync.Mutex
	seq []string
}

func (log *callLog) add(name string) {
	log.mu.Lock()
	log.seq = append(log.seq, name)
	log.mu.Unlock()
}

func (log *callLog) count(name string) int {
	log.mu.Lock()
	defer log.mu.Unlock()
	found := 0
	for _, item := range log.seq {
		if item == name {
			found++
		}
	}
	return found
}

func (log *callLog) names() []string {
	log.mu.Lock()
	defer log.mu.Unlock()
	out := make([]string, len(log.seq))
	copy(out, log.seq)
	return out
}

func (log *callLog) reset() {
	log.mu.Lock()
	log.seq = nil
	log.mu.Unlock()
}

// memoryStockLedger 是一本记在内存里的账。余额不存，按已落册的流水现算。
type memoryStockLedger struct {
	mu            sync.Mutex
	log           *callLog
	movements     []domain.StockMovement
	batches       map[string]domain.InboundBatch
	serializeErr  error
	loadErr       error
	loadBatchErr  error
	balanceErr    error
	appendErr     error
	appendOutcome ports.StockAppendOutcome
	skipLoads     int
}

func newMemoryStockLedger(log *callLog) *memoryStockLedger {
	return &memoryStockLedger{log: log, batches: map[string]domain.InboundBatch{}}
}

func (ledger *memoryStockLedger) SerializePosting(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode) error {
	ledger.log.add("SerializePosting")
	return ledger.serializeErr
}

func (ledger *memoryStockLedger) LoadMovementBySource(
	_ context.Context,
	tenant domain.TenantID,
	reason domain.StockMovementReason,
	source domain.SourceFactReference,
) (domain.StockMovement, bool, error) {
	ledger.log.add("LoadMovementBySource")
	if ledger.loadErr != nil {
		return domain.StockMovement{}, false, ledger.loadErr
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.skipLoads > 0 {
		ledger.skipLoads--
		return domain.StockMovement{}, false, nil
	}
	for _, movement := range ledger.movements {
		if movement.Tenant() == tenant && movement.Reason() == reason && movement.Source() == source {
			return movement, true, nil
		}
	}
	return domain.StockMovement{}, false, nil
}

func (ledger *memoryStockLedger) LoadInboundBatch(
	_ context.Context,
	tenant domain.TenantID,
	id domain.InboundBatchID,
) (domain.InboundBatch, bool, error) {
	ledger.log.add("LoadInboundBatch")
	if ledger.loadBatchErr != nil {
		return domain.InboundBatch{}, false, ledger.loadBatchErr
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	batch, ok := ledger.batches[tenant.String()+"/"+id.String()]
	return batch, ok, nil
}

func (ledger *memoryStockLedger) CellBalance(
	_ context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	cell domain.StockCell,
) (int64, error) {
	ledger.log.add("CellBalance")
	if ledger.balanceErr != nil {
		return 0, ledger.balanceErr
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.balanceLocked(tenant, owner, warehouse, cell)
}

func (ledger *memoryStockLedger) balanceLocked(
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	warehouse domain.WarehouseCode,
	cell domain.StockCell,
) (int64, error) {
	if cell.InboundBatch().String() == "" {
		return 0, errors.New("入库批次必有")
	}
	var balance int64
	for _, movement := range ledger.movements {
		if movement.Tenant() != tenant || movement.Owner() != owner || movement.Warehouse() != warehouse {
			continue
		}
		for _, line := range movement.Lines() {
			if line.Cell() != cell {
				continue
			}
			if line.Direction() == domain.StockIn {
				balance += line.Quantity()
			} else {
				balance -= line.Quantity()
			}
		}
	}
	return balance, nil
}

func (ledger *memoryStockLedger) AppendMovement(
	_ context.Context,
	movement domain.StockMovement,
	formed []domain.InboundBatch,
) (ports.StockAppendOutcome, error) {
	ledger.log.add("AppendMovement")
	if ledger.appendErr != nil {
		return ports.StockAppendOutcomeInvalid, ledger.appendErr
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.appendOutcome != ports.StockAppendOutcomeInvalid {
		return ledger.appendOutcome, nil
	}
	for _, existing := range ledger.movements {
		if existing.Tenant() == movement.Tenant() && existing.Reason() == movement.Reason() && existing.Source() == movement.Source() {
			return ports.StockAlreadyPosted, nil
		}
	}
	negative, err := ledger.negatesLocked(movement)
	if err != nil {
		return ports.StockAppendOutcomeInvalid, err
	}
	if negative {
		return ports.StockInsufficientBalance, nil
	}
	ledger.movements = append(ledger.movements, movement)
	for _, batch := range formed {
		ledger.batches[batch.Tenant().String()+"/"+batch.ID().String()] = batch
	}
	return ports.StockAppended, nil
}

func (ledger *memoryStockLedger) negatesLocked(movement domain.StockMovement) (bool, error) {
	nets := map[domain.StockCell]int64{}
	for _, line := range movement.Lines() {
		delta := line.Quantity()
		if line.Direction() == domain.StockOut {
			delta = -delta
		}
		nets[line.Cell()] += delta
	}
	for cell, net := range nets {
		if net >= 0 {
			continue
		}
		balance, err := ledger.balanceLocked(movement.Tenant(), movement.Owner(), movement.Warehouse(), cell)
		if err != nil {
			return false, err
		}
		if balance+net < 0 {
			return true, nil
		}
	}
	return false, nil
}

func (ledger *memoryStockLedger) seed(movement domain.StockMovement) {
	ledger.mu.Lock()
	ledger.movements = append(ledger.movements, movement)
	ledger.mu.Unlock()
}

func (ledger *memoryStockLedger) movementCount() int {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return len(ledger.movements)
}

func (ledger *memoryStockLedger) storedBatch(tenant domain.TenantID, id domain.InboundBatchID) (domain.InboundBatch, bool) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	batch, ok := ledger.batches[tenant.String()+"/"+id.String()]
	return batch, ok
}

type postingWarehouses struct {
	log  *callLog
	rows map[string]domain.WarehouseRevision
	err  error
}

func (lookup *postingWarehouses) put(revision domain.WarehouseRevision) {
	if lookup.rows == nil {
		lookup.rows = map[string]domain.WarehouseRevision{}
	}
	lookup.rows[revision.Tenant().String()+"/"+revision.Code().String()] = revision
}

func (lookup *postingWarehouses) LoadLatestWarehouse(
	_ context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
) (domain.WarehouseRevision, bool, error) {
	lookup.log.add("LoadLatestWarehouse")
	if lookup.err != nil {
		return domain.WarehouseRevision{}, false, lookup.err
	}
	revision, ok := lookup.rows[tenant.String()+"/"+code.String()]
	return revision, ok, nil
}

type postingLocations struct {
	log  *callLog
	rows map[string]domain.LocationRevision
	err  error
}

func (lookup *postingLocations) put(revision domain.LocationRevision) {
	if lookup.rows == nil {
		lookup.rows = map[string]domain.LocationRevision{}
	}
	lookup.rows[revision.Tenant().String()+"/"+revision.Warehouse().String()+"/"+revision.Code().String()] = revision
}

func (lookup *postingLocations) LoadLatestLocation(
	_ context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
) (domain.LocationRevision, bool, error) {
	lookup.log.add("LoadLatestLocation")
	if lookup.err != nil {
		return domain.LocationRevision{}, false, lookup.err
	}
	revision, ok := lookup.rows[tenant.String()+"/"+warehouse.String()+"/"+code.String()]
	return revision, ok, nil
}

type postingZones struct {
	log  *callLog
	rows map[string]domain.ZoneRevision
	err  error
}

func (lookup *postingZones) put(revision domain.ZoneRevision) {
	if lookup.rows == nil {
		lookup.rows = map[string]domain.ZoneRevision{}
	}
	lookup.rows[revision.Tenant().String()+"/"+revision.Warehouse().String()+"/"+revision.Code().String()] = revision
}

func (lookup *postingZones) LoadLatestZone(
	_ context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
) (domain.ZoneRevision, bool, error) {
	lookup.log.add("LoadLatestZone")
	if lookup.err != nil {
		return domain.ZoneRevision{}, false, lookup.err
	}
	revision, ok := lookup.rows[tenant.String()+"/"+warehouse.String()+"/"+code.String()]
	return revision, ok, nil
}

type postingGoods struct {
	log  *callLog
	rows map[string]domain.GoodsRevision
	err  error
}

func (lookup *postingGoods) put(revision domain.GoodsRevision) {
	if lookup.rows == nil {
		lookup.rows = map[string]domain.GoodsRevision{}
	}
	lookup.rows[revision.Tenant().String()+"/"+revision.Owner().String()+"/"+revision.Code().String()] = revision
}

func (lookup *postingGoods) LoadLatestGoods(
	_ context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (domain.GoodsRevision, bool, error) {
	lookup.log.add("LoadLatestGoods")
	if lookup.err != nil {
		return domain.GoodsRevision{}, false, lookup.err
	}
	revision, ok := lookup.rows[tenant.String()+"/"+owner.String()+"/"+code.String()]
	return revision, ok, nil
}

func (lookup *postingGoods) LoadGoodsAt(
	_ context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
	at time.Time,
) (domain.GoodsRevision, bool, error) {
	lookup.log.add("LoadGoodsAt")
	if lookup.err != nil {
		return domain.GoodsRevision{}, false, lookup.err
	}
	revision, ok := lookup.rows[tenant.String()+"/"+owner.String()+"/"+code.String()]
	if !ok || revision.RegisteredAt().After(at) {
		return domain.GoodsRevision{}, false, nil
	}
	return revision, true, nil
}

type postingIdentities struct {
	log         *callLog
	movementSeq int
	batchSeq    int
	movementErr error
	batchErr    error
}

func (ids *postingIdentities) NextStockMovementID() (domain.StockMovementID, error) {
	ids.log.add("NextStockMovementID")
	if ids.movementErr != nil {
		return domain.StockMovementID{}, ids.movementErr
	}
	ids.movementSeq++
	return domain.NewStockMovementID(fmt.Sprintf("SYN-MOVE-%02d", ids.movementSeq))
}

func (ids *postingIdentities) NextInboundBatchID() (domain.InboundBatchID, error) {
	ids.log.add("NextInboundBatchID")
	if ids.batchErr != nil {
		return domain.InboundBatchID{}, ids.batchErr
	}
	ids.batchSeq++
	return domain.NewInboundBatchID(fmt.Sprintf("SYN-BATCH-%02d", ids.batchSeq))
}

type postingWorld struct {
	t          *testing.T
	log        *callLog
	ledger     *memoryStockLedger
	warehouses *postingWarehouses
	locations  *postingLocations
	zones      *postingZones
	goods      *postingGoods
	identities *postingIdentities
	clock      *fixedClock
	tenant     domain.TenantID
	owner      domain.OwnerAccountID
	warehouse  domain.WarehouseCode
}

func newPostingWorld(t *testing.T) *postingWorld {
	t.Helper()
	log := &callLog{}
	world := &postingWorld{
		t:          t,
		log:        log,
		ledger:     newMemoryStockLedger(log),
		warehouses: &postingWarehouses{log: log},
		locations:  &postingLocations{log: log},
		zones:      &postingZones{log: log},
		goods:      &postingGoods{log: log},
		identities: &postingIdentities{log: log},
		clock:      &fixedClock{at: registrationInstant},
		tenant:     valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		owner:      valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		warehouse:  valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
	}
	world.warehouses.put(stockWarehouse(t, "SYN-WH-01", domain.WarehouseKindSelfOperated, false))
	world.zones.put(stockZone(t, "SYN-ZONE-01", domain.ZoneUsageStorage, false))
	world.locations.put(stockLocation(t, "SYN-WH-01", "SYN-LOC-A", "SYN-ZONE-01", false))
	world.locations.put(stockLocation(t, "SYN-WH-01", "SYN-LOC-B", "SYN-ZONE-01", false))
	world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", nil))
	return world
}

func (world *postingWorld) handler() *application.PostStockMovementHandler {
	return application.NewPostStockMovementHandler(
		world.ledger, world.warehouses, world.locations, world.zones, world.goods, world.identities, world.clock,
	)
}

func (world *postingWorld) post(request domain.StockMovementRequest) (application.PostStockMovementResult, error) {
	world.t.Helper()
	return world.handler().Post(world.t.Context(), application.PostStockMovementCommand{Tenant: world.tenant, Request: request})
}

func (world *postingWorld) mustPost(request domain.StockMovementRequest) application.PostStockMovementResult {
	world.t.Helper()
	result, err := world.post(request)
	if err != nil {
		world.t.Fatalf("Post: %v", err)
	}
	return result
}

func (world *postingWorld) balance(cell domain.StockCell) int64 {
	world.t.Helper()
	world.ledger.mu.Lock()
	defer world.ledger.mu.Unlock()
	balance, err := world.ledger.balanceLocked(world.tenant, world.owner, world.warehouse, cell)
	if err != nil {
		world.t.Fatal(err)
	}
	return balance
}

func (world *postingWorld) addPlace(location, zone string, usage domain.ZoneUsage) {
	world.t.Helper()
	world.zones.put(stockZone(world.t, zone, usage, false))
	world.locations.put(stockLocation(world.t, "SYN-WH-01", location, zone, false))
}

func stockWarehouse(t *testing.T, code string, kind domain.WarehouseKind, deactivated bool) domain.WarehouseRevision {
	t.Helper()
	content, err := domain.NewWarehouseContent(domain.WarehouseContentSpec{
		Name: valueOf(t, domain.NewWarehouseName, "合成仓"),
		Kind: kind,
		Address: domain.NewWarehouseAddress(
			valueOf(t, domain.NewCountryCode, "CN"),
			valueOf(t, domain.NewAddressText, "合成路 1 号"),
		),
		Deactivated: deactivated,
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, code),
		1, content, valueOf(t, domain.NewRegistrant, "operator-1"), registrationInstant,
	)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func stockZone(t *testing.T, code string, usage domain.ZoneUsage, deactivated bool) domain.ZoneRevision {
	t.Helper()
	content := zoneContentWith(t, func(spec *domain.ZoneContentSpec) {
		spec.Usage = usage
		spec.Deactivated = deactivated
	})
	revision, err := domain.NewZoneRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewZoneCode, code), 1, content, valueOf(t, domain.NewRegistrant, "operator-1"), registrationInstant,
	)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func stockLocation(t *testing.T, warehouse, code, zone string, deactivated bool) domain.LocationRevision {
	t.Helper()
	content, err := domain.NewLocationContent(domain.LocationContentSpec{
		ZoneCode: valueOf(t, domain.NewZoneCode, zone), Deactivated: deactivated,
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewLocationRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, warehouse),
		valueOf(t, domain.NewLocationCode, code), 1, content, valueOf(t, domain.NewRegistrant, "operator-1"), registrationInstant,
	)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func stockGoodsRevision(t *testing.T, code string, edit func(*domain.GoodsContentSpec)) domain.GoodsRevision {
	t.Helper()
	return goodsRevisionOf(t, goodsCommand(t, code, "SYN-REQ-"+code, domain.BasisRevision{}, edit), 1, registrationInstant)
}

func stockCell(t *testing.T, edit func(*domain.StockCellSpec)) domain.StockCell {
	t.Helper()
	spec := domain.StockCellSpec{
		Location: valueOf(t, domain.NewLocationCode, "SYN-LOC-A"),
		Goods:    valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		Unit:     valueOf(t, domain.NewGoodsUnitName, "件"),
		State:    domain.StockStateGood,
	}
	if edit != nil {
		edit(&spec)
	}
	cell, err := domain.NewStockCell(spec)
	if err != nil {
		t.Fatal(err)
	}
	return cell
}

func batchNo(t *testing.T, raw string) domain.BatchNumber {
	t.Helper()
	number, err := domain.NewBatchNumber(raw)
	if err != nil {
		t.Fatal(err)
	}
	return number
}

func expiryOn(t *testing.T, raw string) domain.ExpiryDate {
	t.Helper()
	date, err := domain.NewExpiryDate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return date
}

func inLine(t *testing.T, quantity int64, cell domain.StockCell) domain.StockLineRequest {
	t.Helper()
	return domain.NewStockLineRequest(domain.StockIn, quantity, cell, time.Time{})
}

func inLineAt(t *testing.T, quantity int64, cell domain.StockCell, declared time.Time) domain.StockLineRequest {
	t.Helper()
	return domain.NewStockLineRequest(domain.StockIn, quantity, cell, declared)
}

func outLine(t *testing.T, quantity int64, cell domain.StockCell) domain.StockLineRequest {
	t.Helper()
	return domain.NewStockLineRequest(domain.StockOut, quantity, cell, time.Time{})
}

func stockRequest(
	t *testing.T,
	warehouse string,
	reason domain.StockMovementReason,
	source string,
	at time.Time,
	lines ...domain.StockLineRequest,
) domain.StockMovementRequest {
	t.Helper()
	return domain.NewStockMovementRequest(
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewWarehouseCode, warehouse),
		reason,
		valueOf(t, domain.NewSourceFactReference, source),
		at,
		lines,
	)
}

func receiptOf(t *testing.T, source string, lines ...domain.StockLineRequest) domain.StockMovementRequest {
	t.Helper()
	return stockRequest(t, "SYN-WH-01", domain.StockReasonReceipt, source, registrationInstant, lines...)
}

func expectPosted(t *testing.T, result application.PostStockMovementResult) domain.StockMovement {
	t.Helper()
	if result.Outcome() != application.PostStockPosted {
		t.Fatalf("outcome = %s，不合 %v，want POSTED", result.Outcome(), result.Violations())
	}
	movement, ok := result.Movement()
	if !ok {
		t.Fatal("POSTED 没有流水")
	}
	if len(result.Violations()) != 0 {
		t.Fatalf("POSTED 还带了不合 %v", result.Violations())
	}
	return movement
}

func expectRefused(t *testing.T, result application.PostStockMovementResult, want ...application.StockViolation) {
	t.Helper()
	if result.Outcome() != application.PostStockRefused {
		t.Fatalf("outcome = %s，want REFUSED", result.Outcome())
	}
	if _, ok := result.Movement(); ok {
		t.Fatal("REFUSED 不该带流水")
	}
	if !slices.Equal(result.Violations(), want) {
		t.Fatalf("不合 = %v，want %v", result.Violations(), want)
	}
}

func violation(line int, kind application.StockViolationKind) application.StockViolation {
	return application.StockViolation{Line: line, Kind: kind}
}

func trackGoods(batch, expiry bool) func(*domain.GoodsContentSpec) {
	return func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = batch
		spec.TrackExpiry = expiry
	}
}

var (
	_ ports.StockLedger         = (*memoryStockLedger)(nil)
	_ ports.WarehouseLookup     = (*postingWarehouses)(nil)
	_ ports.StockLocationLookup = (*postingLocations)(nil)
	_ ports.ZoneLookup          = (*postingZones)(nil)
	_ ports.StockGoodsLookup    = (*postingGoods)(nil)
	_ ports.StockIdentities     = (*postingIdentities)(nil)
)

func lotCell(t *testing.T, lot, expiry, location string) domain.StockCell {
	t.Helper()
	return stockCell(t, func(spec *domain.StockCellSpec) {
		if location != "" {
			spec.Location = valueOf(t, domain.NewLocationCode, location)
		}
		if lot != "" {
			spec.BatchNumber = batchNo(t, lot)
		}
		if expiry != "" {
			spec.Expiry = expiryOn(t, expiry)
		}
	})
}

func expectNoBusinessAnswer(t *testing.T, result application.PostStockMovementResult, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("应当走 error，得到 %s %v", result.Outcome(), result.Violations())
	}
	if result.Outcome() != application.PostStockMovementOutcomeInvalid || len(result.Violations()) != 0 {
		t.Fatalf("出错时不该形成答复：%s %v", result.Outcome(), result.Violations())
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——同一次入账、同一商品、同一批号与效期的数量同属一个入库批次，入库时间取入账那一刻；批次按行里首次出现的先后签发。
func TestPostReceiptFormsOneInboundBatchPerGoodsLotAndExpiry(t *testing.T) {
	t.Run("同批号效期合成一个批次", func(t *testing.T) {
		world := newPostingWorld(t)
		world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", trackGoods(true, true)))
		cell := lotCell(t, "SYN-LOT-01", "2027-01-31", "")
		movement := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-SAME", inLine(t, 2, cell), inLine(t, 5, cell))))
		lines := movement.Lines()
		if len(lines) != 2 || lines[0].Quantity() != 2 || lines[1].Quantity() != 5 {
			t.Fatalf("行的先后 = %+v", lines)
		}
		if !lines[0].FormedInboundBatch() || lines[0].Cell().InboundBatch() != lines[1].Cell().InboundBatch() {
			t.Fatalf("两行应当同一个新入库批次，得到 %s 与 %s", lines[0].Cell().InboundBatch(), lines[1].Cell().InboundBatch())
		}
		if lines[0].Cell().InboundBatch().String() != "SYN-BATCH-01" {
			t.Fatalf("第一个批次 = %s", lines[0].Cell().InboundBatch())
		}
		batch, ok := world.ledger.storedBatch(world.tenant, lines[0].Cell().InboundBatch())
		if !ok || !batch.InboundAt().Equal(movement.PostedAt()) || batch.MovementID() != movement.ID() {
			t.Fatalf("入库时间 = %s，流水 = %s，有批次 %v", batch.InboundAt(), batch.MovementID(), ok)
		}
		if !movement.PostedAt().Equal(registrationInstant.Truncate(time.Microsecond)) {
			t.Fatalf("入账时刻 = %s", movement.PostedAt())
		}
		if world.log.names()[0] != "SerializePosting" || world.log.count("LoadLatestGoods") != 1 || world.log.count("LoadLatestLocation") != 1 || world.log.count("LoadLatestZone") != 1 || world.log.count("NextInboundBatchID") != 1 {
			t.Fatalf("调用 = %v", world.log.names())
		}
	})
	t.Run("不同批号各成一个批次且按出现先后", func(t *testing.T) {
		world := newPostingWorld(t)
		world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", trackGoods(true, false)))
		later := lotCell(t, "SYN-LOT-B", "", "")
		earlier := lotCell(t, "SYN-LOT-A", "", "")
		movement := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-LOTS", inLine(t, 1, later), inLine(t, 1, earlier))))
		lines := movement.Lines()
		if lines[0].Cell().InboundBatch().String() != "SYN-BATCH-01" || lines[1].Cell().InboundBatch().String() != "SYN-BATCH-02" {
			t.Fatalf("批次签发次序 = %s、%s", lines[0].Cell().InboundBatch(), lines[1].Cell().InboundBatch())
		}
		first, ok := world.ledger.storedBatch(world.tenant, lines[0].Cell().InboundBatch())
		if !ok || first.BatchNumber().String() != "SYN-LOT-B" {
			t.Fatalf("先出现的批号应当先签发，得到 %s", first.BatchNumber())
		}
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——建账导入可以带声明的入库时间，不得晚于入账那一刻；没声明的取入账那一刻。别的原因带了声明的入库时间不成立。
func TestPostTakeOnImportUsesTheDeclaredInboundTime(t *testing.T) {
	takeOn := func(t *testing.T, source string, lines ...domain.StockLineRequest) domain.StockMovementRequest {
		t.Helper()
		return stockRequest(t, "SYN-WH-01", domain.StockReasonTakeOnImport, source, registrationInstant, lines...)
	}
	t.Run("不同声明时间各成一个批次", func(t *testing.T) {
		world := newPostingWorld(t)
		day1 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
		day2 := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
		cell := stockCell(t, nil)
		movement := expectPosted(t, world.mustPost(takeOn(t, "SYN-FACT-DAYS", inLineAt(t, 1, cell, day1), inLineAt(t, 1, cell, day1), inLineAt(t, 1, cell, day2))))
		lines := movement.Lines()
		if lines[0].Cell().InboundBatch() != lines[1].Cell().InboundBatch() || lines[0].Cell().InboundBatch() == lines[2].Cell().InboundBatch() {
			t.Fatalf("批次 = %s、%s、%s", lines[0].Cell().InboundBatch(), lines[1].Cell().InboundBatch(), lines[2].Cell().InboundBatch())
		}
		if lines[0].Cell().InboundBatch().String() != "SYN-BATCH-01" || lines[2].Cell().InboundBatch().String() != "SYN-BATCH-02" {
			t.Fatalf("签发次序 = %s、%s", lines[0].Cell().InboundBatch(), lines[2].Cell().InboundBatch())
		}
		first, _ := world.ledger.storedBatch(world.tenant, lines[0].Cell().InboundBatch())
		second, _ := world.ledger.storedBatch(world.tenant, lines[2].Cell().InboundBatch())
		if !first.InboundAt().Equal(day1) || !second.InboundAt().Equal(day2) {
			t.Fatalf("入库时间 = %s、%s", first.InboundAt(), second.InboundAt())
		}
	})
	t.Run("同一瞬间的不同时区写法是一个批次", func(t *testing.T) {
		world := newPostingWorld(t)
		utc := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		plus8 := time.Date(2026, 3, 1, 8, 0, 0, 0, time.FixedZone("SYN+8", 8*3600))
		cell := stockCell(t, nil)
		movement := expectPosted(t, world.mustPost(takeOn(t, "SYN-FACT-ZONE-TIME", inLineAt(t, 1, cell, utc), inLineAt(t, 1, cell, plus8))))
		if movement.Lines()[0].Cell().InboundBatch() != movement.Lines()[1].Cell().InboundBatch() {
			t.Fatal("同一瞬间被拆成了两个入库批次")
		}
		batch, _ := world.ledger.storedBatch(world.tenant, movement.Lines()[0].Cell().InboundBatch())
		if !batch.InboundAt().Equal(utc) {
			t.Fatalf("入库时间 = %s", batch.InboundAt())
		}
	})
	t.Run("声明时间与入账时刻同一微秒", func(t *testing.T) {
		world := newPostingWorld(t)
		movement := expectPosted(t, world.mustPost(takeOn(t, "SYN-FACT-EQUAL", inLineAt(t, 3, stockCell(t, nil), registrationInstant))))
		batch, ok := world.ledger.storedBatch(world.tenant, movement.Lines()[0].Cell().InboundBatch())
		if !ok || !batch.InboundAt().Equal(movement.PostedAt()) {
			t.Fatalf("入库时间 = %s，入账 = %s，有批次 %v", batch.InboundAt(), movement.PostedAt(), ok)
		}
	})
	t.Run("晚于入账时刻", func(t *testing.T) {
		world := newPostingWorld(t)
		result := world.mustPost(takeOn(t, "SYN-FACT-LATE", inLineAt(t, 1, stockCell(t, nil), registrationInstant.Add(time.Hour))))
		expectRefused(t, result, violation(0, application.StockViolationDeclaredInboundTimeInvalid))
		if world.ledger.movementCount() != 0 {
			t.Fatal("不合的建账导入不该落册")
		}
	})
	t.Run("收货带了声明的入库时间", func(t *testing.T) {
		world := newPostingWorld(t)
		past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		result := world.mustPost(receiptOf(t, "SYN-FACT-DECLARED", inLineAt(t, 1, stockCell(t, nil), past)))
		expectRefused(t, result, violation(0, application.StockViolationDeclaredInboundTimeInvalid))
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——一种来源事实、一种原因至多一笔流水。内容相同的重交答已在册，不同的答来源冲突，在册的不变。重放排在校验之前。
func TestPostReplayReturnsThePostedMovementAndADifferentBodyConflicts(t *testing.T) {
	world := newPostingWorld(t)
	request := receiptOf(t, "SYN-FACT-REPLAY", inLine(t, 4, stockCell(t, nil)))
	first := expectPosted(t, world.mustPost(request))
	again := world.mustPost(request)
	if again.Outcome() != application.PostStockAlreadyPosted {
		t.Fatalf("重投 = %s", again.Outcome())
	}
	echoed, ok := again.Movement()
	if !ok || echoed.ID() != first.ID() || world.ledger.movementCount() != 1 {
		t.Fatalf("回显 %s，在册 %d 笔", echoed.ID(), world.ledger.movementCount())
	}
	if world.log.count("AppendMovement") != 1 {
		t.Fatal("原样重投又落了一次")
	}

	changed := receiptOf(t, "SYN-FACT-REPLAY", inLine(t, 9, stockCell(t, nil)))
	conflict := world.mustPost(changed)
	if conflict.Outcome() != application.PostStockSourceConflict {
		t.Fatalf("换内容 = %s", conflict.Outcome())
	}
	if _, ok := conflict.Movement(); ok || world.ledger.movementCount() != 1 || world.balance(first.Lines()[0].Cell()) != 4 {
		t.Fatalf("冲突之后在册 %d 笔，余额 %d", world.ledger.movementCount(), world.balance(first.Lines()[0].Cell()))
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——重放的是已经形成的答案，不因为此后商品修订改了是否跟踪批号就改口。
func TestPostReplayDoesNotRevalidateAgainstLaterMasterData(t *testing.T) {
	world := newPostingWorld(t)
	request := receiptOf(t, "SYN-FACT-LATER", inLine(t, 2, stockCell(t, nil)))
	first := expectPosted(t, world.mustPost(request))
	world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", trackGoods(true, false)))
	again := world.mustPost(request)
	if again.Outcome() != application.PostStockAlreadyPosted {
		t.Fatalf("主数据变了之后的原样重投 = %s %v", again.Outcome(), again.Violations())
	}
	echoed, _ := again.Movement()
	if echoed.ID() != first.ID() || world.ledger.movementCount() != 1 {
		t.Fatal("重放不该另落一笔")
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——开箱这一段不收；仓必须是本租户已登记的自营仓；行的形状要与原因的类相符。这三格都不再往下看。
func TestPostRefusesUnboxingAnUnregisteredWarehouseAndABadShape(t *testing.T) {
	t.Run("开箱", func(t *testing.T) {
		world := newPostingWorld(t)
		cell := stockCell(t, nil)
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonUnboxing, "SYN-FACT-UNBOX", registrationInstant, outLine(t, 1, cell), inLine(t, 1, cell))
		expectRefused(t, world.mustPost(request), violation(-1, application.StockViolationReasonNotSupported))
		if world.log.count("LoadLatestWarehouse") != 0 || world.ledger.movementCount() != 0 {
			t.Fatal("开箱不该再去看仓")
		}
	})
	t.Run("仓没登记", func(t *testing.T) {
		world := newPostingWorld(t)
		request := stockRequest(t, "SYN-WH-MISSING", domain.StockReasonReceipt, "SYN-FACT-WH", registrationInstant, inLine(t, 1, stockCell(t, nil)))
		expectRefused(t, world.mustPost(request), violation(-1, application.StockViolationWarehouseNotRegistered))
		if world.log.count("LoadLatestGoods") != 0 {
			t.Fatal("仓不成立不该再看行")
		}
	})
	t.Run("不是自营仓", func(t *testing.T) {
		world := newPostingWorld(t)
		world.warehouses.put(stockWarehouse(t, "SYN-WH-AGENT", domain.WarehouseKindAgent, false))
		request := stockRequest(t, "SYN-WH-AGENT", domain.StockReasonReceipt, "SYN-FACT-AGENT", registrationInstant, inLine(t, 1, stockCell(t, nil)))
		expectRefused(t, world.mustPost(request), violation(-1, application.StockViolationWarehouseNotSelfOperated))
		if world.log.count("LoadLatestGoods") != 0 {
			t.Fatal("代理仓不该再看行")
		}
	})
	t.Run("形状不符", func(t *testing.T) {
		world := newPostingWorld(t)
		cell := stockCell(t, nil)
		request := receiptOf(t, "SYN-FACT-SHAPE", inLine(t, 1, cell), outLine(t, 1, cell))
		expectRefused(t, world.mustPost(request), violation(-1, application.StockViolationShapeNotAllowed))
		if world.log.count("LoadLatestWarehouse") != 1 || world.log.count("LoadLatestGoods") != 0 {
			t.Fatalf("形状应当在看行之前挡住，调用 = %v", world.log.names())
		}
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——停用的仓、库区、库位不拒已经发生的作业事实。
func TestPostAcceptsADeactivatedWarehouseZoneAndLocation(t *testing.T) {
	world := newPostingWorld(t)
	world.warehouses.put(stockWarehouse(t, "SYN-WH-01", domain.WarehouseKindSelfOperated, true))
	world.zones.put(stockZone(t, "SYN-ZONE-01", domain.ZoneUsageStorage, true))
	world.locations.put(stockLocation(t, "SYN-WH-01", "SYN-LOC-A", "SYN-ZONE-01", true))
	if !world.warehouses.rows["SYN-TENANT-01/SYN-WH-01"].Content().Deactivated() ||
		!world.zones.rows["SYN-TENANT-01/SYN-WH-01/SYN-ZONE-01"].Content().Deactivated() ||
		!world.locations.rows["SYN-TENANT-01/SYN-WH-01/SYN-LOC-A"].Content().Deactivated() {
		t.Fatal("夹具没有把仓、库区、库位停用")
	}
	expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-OFF", inLine(t, 1, stockCell(t, nil)))))
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——记数逐行指出全部不合的格，不只指第一处；有任何一处不合，整笔不落。
func TestPostCollectsEveryLineViolation(t *testing.T) {
	cases := []struct {
		name    string
		prepare func(*postingWorld)
		request func() domain.StockMovementRequest
		want    application.StockViolation
	}{
		{
			name: "库位没登记",
			request: func() domain.StockMovementRequest {
				cell := stockCell(t, func(spec *domain.StockCellSpec) {
					spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-NOWHERE")
				})
				return receiptOf(t, "SYN-FACT-LOC", inLine(t, 1, cell))
			},
			want: violation(0, application.StockViolationLocationNotRegistered),
		},
		{
			name: "库区不相容",
			prepare: func(world *postingWorld) {
				world.addPlace("SYN-LOC-DEF", "SYN-ZONE-DEF", domain.ZoneUsageDefective)
			},
			request: func() domain.StockMovementRequest {
				cell := stockCell(t, func(spec *domain.StockCellSpec) {
					spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-DEF")
				})
				return receiptOf(t, "SYN-FACT-ZONE", inLine(t, 1, cell))
			},
			want: violation(0, application.StockViolationZoneIncompatible),
		},
		{
			name: "商品没登记",
			request: func() domain.StockMovementRequest {
				cell := stockCell(t, func(spec *domain.StockCellSpec) {
					spec.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-MISSING")
				})
				return receiptOf(t, "SYN-FACT-GOODS", inLine(t, 1, cell))
			},
			want: violation(0, application.StockViolationGoodsNotRegistered),
		},
		{
			name: "计量单位没声明",
			request: func() domain.StockMovementRequest {
				cell := stockCell(t, func(spec *domain.StockCellSpec) {
					spec.Unit = valueOf(t, domain.NewGoodsUnitName, "托")
				})
				return receiptOf(t, "SYN-FACT-UNIT", inLine(t, 1, cell))
			},
			want: violation(0, application.StockViolationUnitNotDeclared),
		},
		{
			name: "跟踪批号却没带",
			prepare: func(world *postingWorld) {
				world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", trackGoods(true, false)))
			},
			request: func() domain.StockMovementRequest {
				return receiptOf(t, "SYN-FACT-LOT-NEED", inLine(t, 1, stockCell(t, nil)))
			},
			want: violation(0, application.StockViolationBatchNumberRequired),
		},
		{
			name: "不跟踪却带了批号",
			request: func() domain.StockMovementRequest {
				return receiptOf(t, "SYN-FACT-LOT-EXTRA", inLine(t, 1, lotCell(t, "SYN-LOT-01", "", "")))
			},
			want: violation(0, application.StockViolationBatchNumberNotTracked),
		},
		{
			name: "跟踪效期却没带",
			prepare: func(world *postingWorld) {
				world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", trackGoods(false, true)))
			},
			request: func() domain.StockMovementRequest {
				return receiptOf(t, "SYN-FACT-EXP-NEED", inLine(t, 1, stockCell(t, nil)))
			},
			want: violation(0, application.StockViolationExpiryRequired),
		},
		{
			name: "不跟踪却带了效期",
			request: func() domain.StockMovementRequest {
				return receiptOf(t, "SYN-FACT-EXP-EXTRA", inLine(t, 1, lotCell(t, "", "2027-01-31", "")))
			},
			want: violation(0, application.StockViolationExpiryNotTracked),
		},
		{
			name: "贴标别名不是平台商品码",
			prepare: func(world *postingWorld) {
				world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", func(spec *domain.GoodsContentSpec) {
					spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-BAR-01", domain.GoodsAliasBarcode, "", time.Time{}, time.Time{})}
				}))
			},
			request: func() domain.StockMovementRequest {
				cell := stockCell(t, func(spec *domain.StockCellSpec) {
					spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-BAR-01")
				})
				return receiptOf(t, "SYN-FACT-BAR", inLine(t, 1, cell))
			},
			want: violation(0, application.StockViolationLabelAliasNotValid),
		},
		{
			name: "取走没点名入库批次",
			request: func() domain.StockMovementRequest {
				return stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, "SYN-FACT-NEED-BATCH", registrationInstant, outLine(t, 1, stockCell(t, nil)))
			},
			want: violation(0, application.StockViolationInboundBatchRequired),
		},
		{
			name: "形成批次的行点了名",
			request: func() domain.StockMovementRequest {
				cell := stockCell(t, func(spec *domain.StockCellSpec) {
					spec.InboundBatch = valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-NAMED")
				})
				return receiptOf(t, "SYN-FACT-NAMED", inLine(t, 1, cell))
			},
			want: violation(0, application.StockViolationInboundBatchNotAllowed),
		},
		{
			name: "点名的入库批次不存在",
			request: func() domain.StockMovementRequest {
				cell := stockCell(t, func(spec *domain.StockCellSpec) {
					spec.InboundBatch = valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-MISSING")
				})
				return stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, "SYN-FACT-MISSING", registrationInstant, outLine(t, 1, cell))
			},
			want: violation(0, application.StockViolationInboundBatchNotFound),
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			world := newPostingWorld(t)
			if item.prepare != nil {
				item.prepare(world)
			}
			expectRefused(t, world.mustPost(item.request()), item.want)
			if world.ledger.movementCount() != 0 {
				t.Fatal("有不合不该落册")
			}
		})
	}

	t.Run("一次收齐多行多种不合", func(t *testing.T) {
		world := newPostingWorld(t)
		world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", func(spec *domain.GoodsContentSpec) {
			spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-BAR-01", domain.GoodsAliasBarcode, "", time.Time{}, time.Time{})}
		}))
		world.addPlace("SYN-LOC-DEF", "SYN-ZONE-DEF", domain.ZoneUsageDefective)
		missing := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-MISSING")
			spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-NOWHERE")
		})
		bad := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-DEF")
			spec.Unit = valueOf(t, domain.NewGoodsUnitName, "托")
			spec.BatchNumber = batchNo(t, "SYN-LOT-X")
			spec.Expiry = expiryOn(t, "2027-05-01")
			spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-BAR-01")
			spec.InboundBatch = valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-NAMED")
		})
		past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		request := receiptOf(t, "SYN-FACT-MANY", inLineAt(t, 1, missing, past), inLineAt(t, 1, bad, past))
		expectRefused(t, world.mustPost(request),
			violation(0, application.StockViolationGoodsNotRegistered),
			violation(0, application.StockViolationLocationNotRegistered),
			violation(0, application.StockViolationDeclaredInboundTimeInvalid),
			violation(1, application.StockViolationZoneIncompatible),
			violation(1, application.StockViolationUnitNotDeclared),
			violation(1, application.StockViolationBatchNumberNotTracked),
			violation(1, application.StockViolationExpiryNotTracked),
			violation(1, application.StockViolationLabelAliasNotValid),
			violation(1, application.StockViolationDeclaredInboundTimeInvalid),
			violation(1, application.StockViolationInboundBatchNotAllowed),
		)
		if world.ledger.movementCount() != 0 {
			t.Fatal("收齐了不合仍不该落册")
		}
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——点名的入库批次须是这一笔这一行的货主、仓、商品，批号与效期还要与行上的一致。
func TestPostInboundBatchMustMatchTheLine(t *testing.T) {
	t.Run("批号不一致", func(t *testing.T) {
		world := newPostingWorld(t)
		world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", trackGoods(true, true)))
		cell := lotCell(t, "SYN-LOT-01", "2027-01-31", "")
		posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-SEED", inLine(t, 4, cell))))
		wrong := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.InboundBatch = posted.Lines()[0].Cell().InboundBatch()
			spec.BatchNumber = batchNo(t, "SYN-LOT-02")
			spec.Expiry = expiryOn(t, "2027-01-31")
		})
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, "SYN-FACT-MISMATCH", registrationInstant, outLine(t, 1, wrong))
		expectRefused(t, world.mustPost(request), violation(0, application.StockViolationInboundBatchMismatch))
		if world.balance(posted.Lines()[0].Cell()) != 4 {
			t.Fatal("对不上的取走不该改余额")
		}
	})
	t.Run("批次属于另一个商品", func(t *testing.T) {
		world := newPostingWorld(t)
		world.goods.put(stockGoodsRevision(t, "SYN-GOODS-02", nil))
		posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-SEED", inLine(t, 4, stockCell(t, nil)))))
		other := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-02")
			spec.InboundBatch = posted.Lines()[0].Cell().InboundBatch()
		})
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, "SYN-FACT-OTHER", registrationInstant, outLine(t, 1, other))
		expectRefused(t, world.mustPost(request), violation(0, application.StockViolationInboundBatchNotFound))
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——存储与拣选只放良品，次品库区只放次品，收货暂存三种状态都可以放，待检只在暂存库区。
func TestPostChecksZoneCompatibilityWithTheStockState(t *testing.T) {
	postAt := func(t *testing.T, location string, state domain.StockState, usage domain.ZoneUsage) application.PostStockMovementResult {
		t.Helper()
		world := newPostingWorld(t)
		if location != "SYN-LOC-A" {
			world.addPlace(location, "SYN-ZONE-X", usage)
		}
		cell := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Location = valueOf(t, domain.NewLocationCode, location)
			spec.State = state
		})
		return world.mustPost(receiptOf(t, "SYN-FACT-PLACE", inLine(t, 1, cell)))
	}
	expectPosted(t, postAt(t, "SYN-LOC-A", domain.StockStateGood, domain.ZoneUsageStorage))
	expectRefused(t, postAt(t, "SYN-LOC-DEF", domain.StockStateGood, domain.ZoneUsageDefective), violation(0, application.StockViolationZoneIncompatible))
	expectPosted(t, postAt(t, "SYN-LOC-RCV", domain.StockStatePendingInspection, domain.ZoneUsageReceivingStaging))
	expectRefused(t, postAt(t, "SYN-LOC-A", domain.StockStatePendingInspection, domain.ZoneUsageStorage), violation(0, application.StockViolationZoneIncompatible))
	expectPosted(t, postAt(t, "SYN-LOC-BAD", domain.StockStateDefective, domain.ZoneUsageDefective))
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——贴标别名要么为空，要么是这个商品当前修订里的平台商品码，且事实发生时刻落在有效区间内。库内移动不重判已经贴上的别名。
func TestPostChecksTheLabelAliasOnlyWhenTheLineSticksANewOne(t *testing.T) {
	from := registrationInstant
	to := registrationInstant.Add(time.Hour)
	withAlias := func(t *testing.T) *postingWorld {
		t.Helper()
		world := newPostingWorld(t)
		world.goods.put(stockGoodsRevision(t, "SYN-GOODS-01", func(spec *domain.GoodsContentSpec) {
			spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-FNSKU-01", domain.GoodsAliasPlatformCode, "SYN-PLATFORM", from, to)}
		}))
		return world
	}
	labeled := func(t *testing.T, location, batch string) domain.StockCell {
		t.Helper()
		return stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-01")
			if location != "" {
				spec.Location = valueOf(t, domain.NewLocationCode, location)
			}
			if batch != "" {
				spec.InboundBatch = valueOf(t, domain.NewInboundBatchID, batch)
			}
		})
	}
	t.Run("区间内", func(t *testing.T) {
		world := withAlias(t)
		movement := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-IN-WINDOW", inLine(t, 2, labeled(t, "", "")))))
		if movement.Lines()[0].Cell().Alias().String() != "SYN-FNSKU-01" {
			t.Fatal("贴上的别名没有留在格上")
		}
	})
	t.Run("没贴", func(t *testing.T) {
		world := withAlias(t)
		expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-BARE", inLine(t, 1, stockCell(t, nil)))))
	})
	t.Run("区间之外", func(t *testing.T) {
		world := withAlias(t)
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonReceipt, "SYN-FACT-OUT", to, inLine(t, 1, labeled(t, "", "")))
		expectRefused(t, world.mustPost(request), violation(0, application.StockViolationLabelAliasNotValid))
	})
	t.Run("还没生效", func(t *testing.T) {
		world := withAlias(t)
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonReceipt, "SYN-FACT-EARLY", from.Add(-time.Second), inLine(t, 1, labeled(t, "", "")))
		expectRefused(t, world.mustPost(request), violation(0, application.StockViolationLabelAliasNotValid))
	})
	t.Run("不是这个商品的别名", func(t *testing.T) {
		world := withAlias(t)
		cell := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-OTHER")
		})
		expectRefused(t, world.mustPost(receiptOf(t, "SYN-FACT-OTHER-ALIAS", inLine(t, 1, cell))), violation(0, application.StockViolationLabelAliasNotValid))
	})
	t.Run("过期之后的库内移动", func(t *testing.T) {
		world := withAlias(t)
		posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-STICK", inLine(t, 4, labeled(t, "", "")))))
		source := posted.Lines()[0].Cell()
		dest := labeled(t, "SYN-LOC-B", source.InboundBatch().String())
		move := stockRequest(t, "SYN-WH-01", domain.StockReasonInternalMove, "SYN-FACT-MOVE-ALIAS", to.Add(time.Hour), outLine(t, 4, source), inLine(t, 4, dest))
		expectPosted(t, world.mustPost(move))
		if world.balance(source) != 0 || world.balance(dest) != 4 {
			t.Fatalf("移动后余额 源 %d 的 %d", world.balance(source), world.balance(dest))
		}
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——取走之后每一格的余额不为负。同一笔里同一格的取走合并了再判；取到恰好为零成立。
func TestPostTakeDecreasesTheBalanceAndRefusesAnOverdraw(t *testing.T) {
	world := newPostingWorld(t)
	posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-IN", inLine(t, 10, stockCell(t, nil)))))
	cell := posted.Lines()[0].Cell()
	take := func(source string, quantity int64) application.PostStockMovementResult {
		t.Helper()
		return world.mustPost(stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, source, registrationInstant, outLine(t, quantity, cell)))
	}
	expectPosted(t, take("SYN-FACT-OUT-4", 4))
	if world.balance(cell) != 6 {
		t.Fatalf("取走 4 之后余额 = %d", world.balance(cell))
	}
	expectPosted(t, take("SYN-FACT-OUT-6", 6))
	if world.balance(cell) != 0 {
		t.Fatalf("取到零之后余额 = %d", world.balance(cell))
	}
	before := world.ledger.movementCount()
	expectRefused(t, take("SYN-FACT-OUT-1", 1), violation(0, application.StockViolationInsufficientBalance))
	if world.balance(cell) != 0 || world.ledger.movementCount() != before {
		t.Fatal("不够的取走不该落册")
	}

	t.Run("同一格两行取走合并", func(t *testing.T) {
		world := newPostingWorld(t)
		posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-IN", inLine(t, 10, stockCell(t, nil)))))
		cell := posted.Lines()[0].Cell()
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, "SYN-FACT-BOTH", registrationInstant, outLine(t, 6, cell), outLine(t, 6, cell))
		expectRefused(t, world.mustPost(request),
			violation(0, application.StockViolationInsufficientBalance),
			violation(1, application.StockViolationInsufficientBalance),
		)
		if world.balance(cell) != 10 || world.ledger.movementCount() != 1 {
			t.Fatal("合并后不够，不该落册")
		}
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——库内移动不改数量与入库批次，只改位置；组内不相抵则整笔不落。
func TestPostInternalMoveKeepsTheInboundBatchAndRefusesAnUnbalancedMove(t *testing.T) {
	world := newPostingWorld(t)
	posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-IN", inLine(t, 8, stockCell(t, nil)))))
	source := posted.Lines()[0].Cell()
	destOf := func(t *testing.T, quantity int64) domain.StockLineRequest {
		t.Helper()
		dest := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-B")
			spec.InboundBatch = source.InboundBatch()
		})
		return inLine(t, quantity, dest)
	}
	unbalanced := stockRequest(t, "SYN-WH-01", domain.StockReasonInternalMove, "SYN-FACT-SHORT", registrationInstant, outLine(t, 8, source), destOf(t, 7))
	expectRefused(t, world.mustPost(unbalanced), violation(-1, application.StockViolationShapeNotAllowed))
	if world.balance(source) != 8 {
		t.Fatal("不相抵的移动不该改余额")
	}
	world.log.reset()
	destLine := destOf(t, 8)
	moved := expectPosted(t, world.mustPost(stockRequest(t, "SYN-WH-01", domain.StockReasonInternalMove, "SYN-FACT-MOVE", registrationInstant, outLine(t, 8, source), destLine)))
	if moved.Lines()[0].Cell().InboundBatch() != source.InboundBatch() || moved.Lines()[1].Cell().InboundBatch() != source.InboundBatch() {
		t.Fatal("库内移动不该另起入库批次")
	}
	if world.balance(source) != 0 || world.balance(destLine.Cell()) != 8 {
		t.Fatalf("移动后余额 源 %d 的 %d", world.balance(source), world.balance(destLine.Cell()))
	}
	if world.log.count("LoadInboundBatch") != 1 {
		t.Fatalf("同一个入库批次读了 %d 次", world.log.count("LoadInboundBatch"))
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——形态转换折成基本单位守恒，折合数取商品当前修订的包装层级。
func TestPostUnitConversionBalancesInBaseUnits(t *testing.T) {
	world := newPostingWorld(t)
	box := stockCell(t, func(spec *domain.StockCellSpec) {
		spec.Unit = valueOf(t, domain.NewGoodsUnitName, "箱")
	})
	posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-BOX", inLine(t, 2, box))))
	boxCell := posted.Lines()[0].Cell()
	piece := func(quantity int64) domain.StockLineRequest {
		t.Helper()
		cell := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.InboundBatch = boxCell.InboundBatch()
		})
		return inLine(t, quantity, cell)
	}
	short := stockRequest(t, "SYN-WH-01", domain.StockReasonUnitConversion, "SYN-FACT-SHORT", registrationInstant, outLine(t, 1, boxCell), piece(23))
	expectRefused(t, world.mustPost(short), violation(-1, application.StockViolationShapeNotAllowed))
	convert := stockRequest(t, "SYN-WH-01", domain.StockReasonUnitConversion, "SYN-FACT-CONV", registrationInstant, outLine(t, 1, boxCell), piece(24))
	expectPosted(t, world.mustPost(convert))
	if world.balance(boxCell) != 1 || world.balance(piece(24).Cell()) != 24 {
		t.Fatalf("转换后 箱 %d 件 %d", world.balance(boxCell), world.balance(piece(24).Cell()))
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——落册再判一次判重位置与余额，那是并发下的定论。
func TestPostAppendOutcomeIsTheConcurrentVerdict(t *testing.T) {
	seededFrom := func(t *testing.T, request domain.StockMovementRequest) domain.StockMovement {
		t.Helper()
		batchID := valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-SEED")
		lines := make([]domain.StockMovementLine, 0, len(request.Lines()))
		for _, line := range request.Lines() {
			lines = append(lines, domain.NewStockMovementLine(line.Direction(), line.Quantity(), line.Cell().WithInboundBatch(batchID), line.DeclaredInboundAt(), true))
		}
		movement, err := domain.NewStockMovement(
			valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewStockMovementID, "SYN-MOVE-SEED"),
			request.Owner(), request.Warehouse(), request.Reason(), request.Source(), request.OccurredAt(), registrationInstant, lines,
		)
		if err != nil {
			t.Fatal(err)
		}
		return movement
	}
	t.Run("别人先落了相同内容", func(t *testing.T) {
		world := newPostingWorld(t)
		request := receiptOf(t, "SYN-FACT-RACE", inLine(t, 5, stockCell(t, nil)))
		world.ledger.seed(seededFrom(t, request))
		world.ledger.skipLoads = 1
		world.ledger.appendOutcome = ports.StockAlreadyPosted
		result := world.mustPost(request)
		if result.Outcome() != application.PostStockAlreadyPosted {
			t.Fatalf("并发重放 = %s", result.Outcome())
		}
		echoed, _ := result.Movement()
		if echoed.ID().String() != "SYN-MOVE-SEED" || world.ledger.movementCount() != 1 {
			t.Fatalf("回显 %s，在册 %d 笔", echoed.ID(), world.ledger.movementCount())
		}
	})
	t.Run("别人先落了不同内容", func(t *testing.T) {
		world := newPostingWorld(t)
		world.ledger.seed(seededFrom(t, receiptOf(t, "SYN-FACT-RACE", inLine(t, 5, stockCell(t, nil)))))
		world.ledger.skipLoads = 1
		world.ledger.appendOutcome = ports.StockAlreadyPosted
		result := world.mustPost(receiptOf(t, "SYN-FACT-RACE", inLine(t, 9, stockCell(t, nil))))
		if result.Outcome() != application.PostStockSourceConflict || world.ledger.movementCount() != 1 {
			t.Fatalf("并发冲突 = %s，在册 %d 笔", result.Outcome(), world.ledger.movementCount())
		}
	})
	t.Run("落册说已在册却取不回", func(t *testing.T) {
		world := newPostingWorld(t)
		world.ledger.skipLoads = 2
		world.ledger.appendOutcome = ports.StockAlreadyPosted
		result, err := world.post(receiptOf(t, "SYN-FACT-GONE", inLine(t, 1, stockCell(t, nil))))
		expectNoBusinessAnswer(t, result, err)
	})
	t.Run("别人先取走了", func(t *testing.T) {
		world := newPostingWorld(t)
		world.ledger.appendOutcome = ports.StockInsufficientBalance
		result := world.mustPost(receiptOf(t, "SYN-FACT-LOST", inLine(t, 1, stockCell(t, nil))))
		expectRefused(t, result, violation(-1, application.StockViolationInsufficientBalance))
		if world.ledger.movementCount() != 0 {
			t.Fatal("并发余额不足不该落册")
		}
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——记数先把这一货主这一仓的账串起来，再读。读不动或落册失败是技术失败，不形成业务答复。
func TestPostSerializePostingHappensBeforeAnyReadAndFailuresStayErrors(t *testing.T) {
	t.Run("串行在任何读之前", func(t *testing.T) {
		world := newPostingWorld(t)
		expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-ORDER", inLine(t, 1, stockCell(t, nil)), inLine(t, 1, stockCell(t, nil)))))
		names := world.log.names()
		if len(names) == 0 || names[0] != "SerializePosting" {
			t.Fatalf("调用 = %v", names)
		}
		reads := map[string]bool{
			"LoadMovementBySource": true, "LoadLatestWarehouse": true, "LoadLatestGoods": true,
			"LoadLatestLocation": true, "LoadLatestZone": true, "LoadInboundBatch": true, "CellBalance": true,
		}
		for index, name := range names {
			if reads[name] && index == 0 {
				t.Fatalf("读 %s 发生在串行之前", name)
			}
		}
	})
	t.Run("缺租户", func(t *testing.T) {
		world := newPostingWorld(t)
		result, err := world.handler().Post(t.Context(), application.PostStockMovementCommand{})
		expectNoBusinessAnswer(t, result, err)
		if world.log.count("SerializePosting") != 0 {
			t.Fatal("装配错不该去串行")
		}
	})

	plain := func(t *testing.T) domain.StockMovementRequest {
		t.Helper()
		return receiptOf(t, "SYN-FACT-ERR", inLine(t, 1, stockCell(t, nil)))
	}
	failures := []struct {
		name   string
		inject func(*postingWorld)
	}{
		{"串行失败", func(world *postingWorld) { world.ledger.serializeErr = errors.New("serialize down") }},
		{"判重读失败", func(world *postingWorld) { world.ledger.loadErr = errors.New("load down") }},
		{"仓读失败", func(world *postingWorld) { world.warehouses.err = errors.New("warehouse down") }},
		{"商品读失败", func(world *postingWorld) { world.goods.err = errors.New("goods down") }},
		{"库位读失败", func(world *postingWorld) { world.locations.err = errors.New("location down") }},
		{"库区读失败", func(world *postingWorld) { world.zones.err = errors.New("zone down") }},
		{"流水标识失败", func(world *postingWorld) { world.identities.movementErr = errors.New("movement id down") }},
		{"批次标识失败", func(world *postingWorld) { world.identities.batchErr = errors.New("batch id down") }},
		{"落册失败", func(world *postingWorld) { world.ledger.appendErr = errors.New("append down") }},
	}
	for _, item := range failures {
		t.Run(item.name, func(t *testing.T) {
			world := newPostingWorld(t)
			item.inject(world)
			result, err := world.post(plain(t))
			expectNoBusinessAnswer(t, result, err)
			if world.ledger.movementCount() != 0 {
				t.Fatal("失败不该落册")
			}
		})
	}
	t.Run("入库批次读失败", func(t *testing.T) {
		world := newPostingWorld(t)
		world.ledger.loadBatchErr = errors.New("batch down")
		cell := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.InboundBatch = valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-01")
		})
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, "SYN-FACT-BATCH-ERR", registrationInstant, outLine(t, 1, cell))
		result, err := world.post(request)
		expectNoBusinessAnswer(t, result, err)
	})
	t.Run("库位的库区不在册", func(t *testing.T) {
		world := newPostingWorld(t)
		world.locations.put(stockLocation(t, "SYN-WH-01", "SYN-LOC-GHOST", "SYN-ZONE-MISSING", false))
		cell := stockCell(t, func(spec *domain.StockCellSpec) {
			spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-GHOST")
		})
		result, err := world.post(receiptOf(t, "SYN-FACT-GHOST", inLine(t, 1, cell)))
		expectNoBusinessAnswer(t, result, err)
		if world.ledger.movementCount() != 0 {
			t.Fatal("册不自洽不该落成拒绝")
		}
	})
	t.Run("余额读失败", func(t *testing.T) {
		world := newPostingWorld(t)
		posted := expectPosted(t, world.mustPost(receiptOf(t, "SYN-FACT-IN", inLine(t, 4, stockCell(t, nil)))))
		world.ledger.balanceErr = errors.New("balance down")
		before := world.ledger.movementCount()
		request := stockRequest(t, "SYN-WH-01", domain.StockReasonOutboundConfirmation, "SYN-FACT-BAL", registrationInstant, outLine(t, 1, posted.Lines()[0].Cell()))
		result, err := world.post(request)
		expectNoBusinessAnswer(t, result, err)
		if world.ledger.movementCount() != before {
			t.Fatal("余额读失败不该落册")
		}
	})
	t.Run("串行失败时还没有读", func(t *testing.T) {
		world := newPostingWorld(t)
		world.ledger.serializeErr = errors.New("serialize down")
		_, err := world.post(plain(t))
		if err == nil || world.log.count("LoadMovementBySource") != 0 || world.log.count("LoadLatestWarehouse") != 0 {
			t.Fatalf("err = %v，调用 = %v", err, world.log.names())
		}
	})
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——事实发生时刻没给是装配错，在读账之前就交 error，不形成答复。
func TestPostRejectsAMissingFactInstantAsAnAssemblyError(t *testing.T) {
	world := newPostingWorld(t)
	request := domain.NewStockMovementRequest(
		world.owner, world.warehouse, domain.StockReasonReceipt,
		valueOf(t, domain.NewSourceFactReference, "SYN-FACT-NOTIME"), time.Time{},
		[]domain.StockLineRequest{inLine(t, 1, stockCell(t, nil))},
	)
	result, err := world.post(request)
	expectNoBusinessAnswer(t, result, err)
	if !strings.Contains(err.Error(), "缺事实发生时刻") || world.log.count("SerializePosting") != 0 {
		t.Fatalf("err = %v，调用 = %v", err, world.log.names())
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——同一格的记入与取走合并、以及余额加净额，相加回绕时交 error，不靠回绕答够。
func TestPostReturnsAnErrorWhenQuantitiesWrap(t *testing.T) {
	world := newPostingWorld(t)
	quantity := int64(math.MaxInt64)/2 + 1
	cell := stockCell(t, nil)
	result, err := world.post(receiptOf(t, "SYN-FACT-WRAP", inLine(t, quantity, cell), inLine(t, quantity, cell)))
	expectNoBusinessAnswer(t, result, err)
	if world.ledger.movementCount() != 0 {
		t.Fatal("回绕不该落册")
	}
}
