package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 收货确认的核心走真库。每个用例一个施加过全套迁移的全新库。入账和推进预报状态都包在已有的 WithinTransaction 里。

type receiptWorld struct {
	preAdviceWorld
	zones         *adapter.Zones
	locations     *adapter.Locations
	ledger        *adapter.StockLedger
	catalogue     *adapter.GoodsCatalogue
	preAdviceRead *adapter.PreAdviceCatalogue
	receipts      *adapter.Receipts
	closures      *adapter.PreAdviceClosures
	conclusions   *adapter.DiscrepancyConclusions
	stockIDs      *whidentity.StockIdentities
	cartons       *adapter.CartonLedger
	cartonIDs     *whidentity.CartonIdentities
	discrepancy   *whidentity.ReceiptDiscrepancyIdentities
	arrivals      *adapter.UnclaimedArrivals
	arrivalIDs    *whidentity.UnclaimedArrivalIdentities
	charges       *application.ChargeHandler
}

func newReceiptWorld(t *testing.T) receiptWorld {
	t.Helper()
	base := newPreAdviceWorld(t, preAdviceNumberingInstant)
	zones, err := adapter.NewZones(base.db)
	if err != nil {
		t.Fatal(err)
	}
	locations, err := adapter.NewLocations(base.db)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := adapter.NewStockLedger(base.db)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := adapter.NewGoodsCatalogue(base.db)
	if err != nil {
		t.Fatal(err)
	}
	preAdviceRead, err := adapter.NewPreAdviceCatalogue(base.db)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := adapter.NewReceipts(base.db)
	if err != nil {
		t.Fatal(err)
	}
	closures, err := adapter.NewPreAdviceClosures(base.db)
	if err != nil {
		t.Fatal(err)
	}
	conclusions, err := adapter.NewDiscrepancyConclusions(base.db)
	if err != nil {
		t.Fatal(err)
	}
	stockIDs, err := whidentity.NewStockIdentities()
	if err != nil {
		t.Fatal(err)
	}
	cartons, err := adapter.NewCartonLedger(base.db)
	if err != nil {
		t.Fatal(err)
	}
	cartonIDs, err := whidentity.NewCartonIdentities()
	if err != nil {
		t.Fatal(err)
	}
	discrepancy, err := whidentity.NewReceiptDiscrepancyIdentities()
	if err != nil {
		t.Fatal(err)
	}
	arrivals, err := adapter.NewUnclaimedArrivals(base.db)
	if err != nil {
		t.Fatal(err)
	}
	arrivalIDs, err := whidentity.NewUnclaimedArrivalIdentities()
	if err != nil {
		t.Fatal(err)
	}
	return receiptWorld{
		preAdviceWorld: base, zones: zones, locations: locations, ledger: ledger, catalogue: catalogue, preAdviceRead: preAdviceRead,
		receipts: receipts, closures: closures, conclusions: conclusions, stockIDs: stockIDs,
		cartons: cartons, cartonIDs: cartonIDs, discrepancy: discrepancy,
		arrivals: arrivals, arrivalIDs: arrivalIDs,
	}
}

func (world receiptWorld) confirmer() *application.ConfirmReceiptHandler {
	handler := application.NewConfirmReceiptHandler(
		world.receipts, world.preAdvices, world.handler(), world.poster(),
		world.cartons, world.cartonIDs,
		world.warehouses, world.locations, world.zones, world.catalogue, world.goods,
		world.discrepancy, world.arrivals, world.arrivalIDs, world.clock,
	)
	if world.charges != nil {
		handler = handler.WithCharges(world.charges)
	}
	return handler
}

func (world receiptWorld) poster() *application.PostStockMovementHandler {
	return application.NewPostStockMovementHandler(
		world.ledger, world.warehouses, world.locations, world.zones, world.goods, world.stockIDs, world.clock,
	)
}

func (world receiptWorld) queries() *application.QueryPreAdviceHandler {
	return application.NewQueryPreAdviceHandler(world.preAdviceRead, world.receipts, world.goods, world.closures, world.conclusions, world.arrivals)
}

func (world receiptWorld) seedReceiving(t *testing.T) {
	t.Helper()
	world.seedWarehouse(t, nil)
	world.seedGoods(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-RCV", 1, func(spec *domain.ZoneContentSpec) {
		spec.Name = valueOf(t, domain.NewZoneName, "收货暂存")
		spec.Usage = domain.ZoneUsageReceivingStaging
	}))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-RCV", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-RCV")
	}))
}

func (world receiptWorld) saveZone(t *testing.T, revision domain.ZoneRevision) {
	t.Helper()
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, saveErr := world.zones.SaveZoneRevision(ctx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (world receiptWorld) saveLocation(t *testing.T, revision domain.LocationRevision) {
	t.Helper()
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, saveErr := world.locations.SaveLocationRevision(ctx, revision)
		return saveErr
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (world receiptWorld) saveGoodsRevision(t *testing.T, revision domain.GoodsRevision) {
	t.Helper()
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		saved, saveErr := world.goods.SaveGoodsRevision(ctx, revision)
		if saveErr != nil {
			return saveErr
		}
		if saved.Outcome != ports.GoodsRevisionSaved {
			return errors.New(saved.Outcome.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (world receiptWorld) adviseGoods(t *testing.T, key, goods, unit string, quantity int64) domain.PreAdvice {
	t.Helper()
	line, err := domain.NewGoodsInboundLine(
		valueOf(t, domain.NewGoodsCode, goods), valueOf(t, domain.NewGoodsUnitName, unit),
		quantity, domain.BatchNumber{}, domain.ExpiryDate{}, domain.CartonCount{},
	)
	if err != nil {
		t.Fatal(err)
	}
	result := world.accept(t, preAdviceAcceptCommand(t, key, func(command *application.AcceptPreAdviceCommand) {
		command.GoodsLines = []domain.GoodsInboundLine{line}
	}))
	if result.Outcome() != application.PreAdviceOutcomeAdvised {
		t.Fatalf("受理 %s：%s", key, result.Outcome())
	}
	advice, ok := result.PreAdvice()
	if !ok {
		t.Fatal("受理没有回显预报")
	}
	return advice
}

func (world receiptWorld) confirm(t *testing.T, command application.ConfirmReceiptCommand) application.ConfirmReceiptResult {
	t.Helper()
	var result application.ConfirmReceiptResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var confirmErr error
		result, confirmErr = world.confirmer().Confirm(ctx, command)
		return confirmErr
	})
	if err != nil {
		t.Fatalf("收货：%v", err)
	}
	return result
}

type receiptLineSpec struct {
	observed string
	unit     string
	quantity int64
	state    domain.StockState
	batch    string
	expiry   string
	alias    string
	location string
}

func mustReceiptLine(t *testing.T, spec receiptLineSpec) domain.ReceiptLine {
	t.Helper()
	if spec.unit == "" {
		spec.unit = "件"
	}
	if spec.state == domain.StockStateInvalid {
		spec.state = domain.StockStateGood
	}
	if spec.location == "" {
		spec.location = "SYN-LOC-RCV"
	}
	var batch domain.BatchNumber
	if spec.batch != "" {
		batch = valueOf(t, domain.NewBatchNumber, spec.batch)
	}
	var expiry domain.ExpiryDate
	if spec.expiry != "" {
		expiry = valueOf(t, domain.NewExpiryDate, spec.expiry)
	}
	var alias domain.GoodsAliasCode
	if spec.alias != "" {
		alias = valueOf(t, domain.NewGoodsAliasCode, spec.alias)
	}
	line, err := domain.NewReceiptLine(
		spec.observed, valueOf(t, domain.NewGoodsUnitName, spec.unit), spec.state, spec.quantity,
		batch, expiry, alias, valueOf(t, domain.NewLocationCode, spec.location),
	)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func receiptCommand(t *testing.T, fact, advice string, lines ...domain.ReceiptLine) application.ConfirmReceiptCommand {
	t.Helper()
	return application.ConfirmReceiptCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), FactID: valueOf(t, domain.NewReceiptFactID, fact),
		Device: valueOf(t, domain.NewReceiptDeviceID, "SYN-DEVICE-01"), Operator: valueOf(t, domain.NewReceiptOperatorID, "operator-1"),
		OccurredAt: preAdviceNumberingInstant, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		PreAdvice: valueOf(t, domain.NewPreAdviceID, advice), Shape: domain.InboundByGoods, Lines: lines,
	}
}

func (world receiptWorld) stockOf(t *testing.T, goods string) int64 {
	t.Helper()
	var quantity int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT coalesce(sum(quantity_delta), 0)
		   FROM warehouse_fulfillment.stock_movement_line
		  WHERE tenant_id = $1 AND goods_code = $2`, "SYN-TENANT-01", goods).Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}

func (world receiptWorld) batchCount(t *testing.T, goods string) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.inbound_batch WHERE tenant_id = $1 AND goods_code = $2`,
		"SYN-TENANT-01", goods).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) movementCount(t *testing.T) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.stock_movement WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) receiptCount(t *testing.T) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.receipt_confirmation WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) adviceStatus(t *testing.T, id domain.PreAdviceID) string {
	t.Helper()
	var status string
	err := world.pool.QueryRow(t.Context(),
		`SELECT status FROM warehouse_fulfillment.inbound_pre_advice WHERE tenant_id = $1 AND pre_advice_id = $2`,
		"SYN-TENANT-01", id.String()).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func overageQuantity(receipt domain.RecordedReceipt) int64 {
	var quantity int64
	for _, line := range receipt.Differences() {
		if line.Kind() == domain.ReceiptDiscrepancyOverage {
			quantity += line.Quantity()
		}
	}
	return quantity
}

func requireReceipt(t *testing.T, result application.ConfirmReceiptResult) domain.RecordedReceipt {
	t.Helper()
	receipt, ok := result.Receipt()
	if !ok {
		t.Fatalf("%s 没有回显收货事实", result.Outcome())
	}
	return receipt
}

// Covers: AT-WF-037（收货确认的核心）
func TestReceiptFirstPostReplaysAndRejectsADifferentQuantity(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 100)
	command := receiptCommand(t, "SYN-FACT-1", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 40}))
	confirmed := world.confirm(t, command)
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.adviceStatus(t, advice.ID()) != "RECEIVING" ||
		world.stockOf(t, "SYN-GOODS-01") != 40 || world.batchCount(t, "SYN-GOODS-01") != 1 || world.movementCount(t) != 1 {
		t.Fatalf("第一次收货 %s 状态 %s 库存 %d 批次 %d 流水 %d",
			confirmed.Outcome(), world.adviceStatus(t, advice.ID()), world.stockOf(t, "SYN-GOODS-01"),
			world.batchCount(t, "SYN-GOODS-01"), world.movementCount(t))
	}
	replayed := world.confirm(t, command)
	if replayed.Outcome() != application.ConfirmReceiptExistingResult || world.stockOf(t, "SYN-GOODS-01") != 40 || world.movementCount(t) != 1 {
		t.Fatalf("原样重投 %s 库存 %d 流水 %d", replayed.Outcome(), world.stockOf(t, "SYN-GOODS-01"), world.movementCount(t))
	}
	conflict := receiptCommand(t, "SYN-FACT-1", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 41}))
	rejected := world.confirm(t, conflict)
	if rejected.Outcome() != application.ConfirmReceiptContentConflict || world.stockOf(t, "SYN-GOODS-01") != 40 || world.receiptCount(t) != 1 {
		t.Fatalf("换数量 %s 库存 %d 事实 %d", rejected.Outcome(), world.stockOf(t, "SYN-GOODS-01"), world.receiptCount(t))
	}
}

// Covers: AT-WF-038（收货确认的核心）
func TestReceiptSecondPostRecordsTheOverage(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 100)
	first := world.confirm(t, receiptCommand(t, "SYN-FACT-60", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 60})))
	second := world.confirm(t, receiptCommand(t, "SYN-FACT-50", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 50})))
	if first.Outcome() != application.ConfirmReceiptConfirmed || second.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatalf("两次收货 %s %s", first.Outcome(), second.Outcome())
	}
	if overageQuantity(requireReceipt(t, first)) != 0 || overageQuantity(requireReceipt(t, second)) != 10 || world.stockOf(t, "SYN-GOODS-01") != 110 {
		t.Fatalf("多收 %d、%d 库存 %d", overageQuantity(requireReceipt(t, first)), overageQuantity(requireReceipt(t, second)), world.stockOf(t, "SYN-GOODS-01"))
	}
}

// Covers: AT-WF-038（收货确认的核心）——后一笔卡在预报锁上，多收记在后到的那一次。
func TestReceiptConcurrentPostsPutOverageOnTheLaterOne(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 100)
	firstCommand := receiptCommand(t, "SYN-FACT-LEFT", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 60}))
	secondCommand := receiptCommand(t, "SYN-FACT-RIGHT", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 50}))
	observer := newBlockedSessionObserver(t, world.pool)
	ctx := t.Context()
	inserted := make(chan struct{})
	release := make(chan struct{})
	var earlier, later application.ConfirmReceiptResult
	var earlierErr, laterErr error
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		earlierErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			earlier, err = world.confirmer().Confirm(txCtx, firstCommand)
			close(inserted)
			if err == nil {
				<-release
			}
			return err
		})
	}()
	<-inserted
	wait.Add(1)
	go func() {
		defer wait.Done()
		laterErr = world.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			later, err = world.confirmer().Confirm(txCtx, secondCommand)
			return err
		})
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()
	if earlierErr != nil || laterErr != nil {
		t.Fatalf("并发收货：%v、%v", earlierErr, laterErr)
	}
	if !blocked {
		t.Fatal("后一笔没有卡在锁上")
	}
	if earlier.Outcome() != application.ConfirmReceiptConfirmed || later.Outcome() != application.ConfirmReceiptConfirmed ||
		overageQuantity(requireReceipt(t, earlier)) != 0 || overageQuantity(requireReceipt(t, later)) != 10 ||
		world.stockOf(t, "SYN-GOODS-01") != 110 {
		t.Fatalf("先到多收 %d 后到多收 %d 库存 %d", overageQuantity(requireReceipt(t, earlier)), overageQuantity(requireReceipt(t, later)), world.stockOf(t, "SYN-GOODS-01"))
	}
}

// Covers: AT-WF-039（收货确认的核心）
func TestReceiptOverageConvertsCartonsAndPiecesToBaseUnits(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "箱", 10)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-1", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 250})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.stockOf(t, "SYN-GOODS-01") != 250 || overageQuantity(requireReceipt(t, confirmed)) != 10 {
		t.Fatalf("按件收 250：%s 库存 %d 多收 %d", confirmed.Outcome(), world.stockOf(t, "SYN-GOODS-01"), overageQuantity(requireReceipt(t, confirmed)))
	}
}

// Covers: AT-WF-040（收货确认的核心）
func TestReceiptTrackingRejectsAMissingOrUnexpectedBatch(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-TRACK-01", 1, "REQ-TRACK", func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
		spec.TrackExpiry = true
		spec.Aliases = nil
	}))
	advice := world.adviseGoods(t, "REQ-TRACK", "SYN-TRACK-01", "件", 20)
	missing := world.confirm(t, receiptCommand(t, "SYN-FACT-MISS", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-TRACK-01", quantity: 2})))
	if missing.Outcome() != application.ConfirmReceiptContentInvalid || world.receiptCount(t) != 0 || world.adviceStatus(t, advice.ID()) != "ADVISED" || world.stockOf(t, "SYN-TRACK-01") != 0 {
		t.Fatalf("没带批号 %s 事实 %d 状态 %s", missing.Outcome(), world.receiptCount(t), world.adviceStatus(t, advice.ID()))
	}
	posted := world.confirm(t, receiptCommand(t, "SYN-FACT-OK", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-TRACK-01", quantity: 2, batch: "LOT-1", expiry: "2027-01-31",
	})))
	if posted.Outcome() != application.ConfirmReceiptConfirmed || world.batchCount(t, "SYN-TRACK-01") != 1 || world.stockOf(t, "SYN-TRACK-01") != 2 {
		t.Fatalf("带齐 %s 批次 %d 库存 %d", posted.Outcome(), world.batchCount(t, "SYN-TRACK-01"), world.stockOf(t, "SYN-TRACK-01"))
	}
	plain := world.adviseGoods(t, "REQ-PLAIN", "SYN-GOODS-01", "件", 10)
	unexpected := world.confirm(t, receiptCommand(t, "SYN-FACT-EXTRA", plain.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1, batch: "LOT-X",
	})))
	if unexpected.Outcome() != application.ConfirmReceiptContentInvalid || world.receiptCount(t) != 1 || world.stockOf(t, "SYN-GOODS-01") != 0 || world.adviceStatus(t, plain.ID()) != "ADVISED" {
		t.Fatalf("不跟踪却带了批号 %s 事实 %d", unexpected.Outcome(), world.receiptCount(t))
	}
	two := world.confirm(t, receiptCommand(t, "SYN-FACT-TWO", advice.ID().String(),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-TRACK-01", quantity: 1, batch: "LOT-A", expiry: "2027-02-01"}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-TRACK-01", quantity: 1, batch: "LOT-B", expiry: "2027-03-01"}),
	))
	if two.Outcome() != application.ConfirmReceiptConfirmed || world.batchCount(t, "SYN-TRACK-01") != 3 {
		t.Fatalf("两个批号 %s 批次 %d", two.Outcome(), world.batchCount(t, "SYN-TRACK-01"))
	}
}

// Covers: AT-WF-041（收货确认的核心）
func TestReceiptDamageCountsDefectiveAndSkipsInspection(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 100)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-1", advice.ID().String(),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 90, state: domain.StockStateGood}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 6, state: domain.StockStateDefective}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 4, state: domain.StockStatePendingInspection}),
	))
	receipt := requireReceipt(t, confirmed)
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.batchCount(t, "SYN-GOODS-01") != 1 || world.stockOf(t, "SYN-GOODS-01") != 100 {
		t.Fatalf("三种状态 %s 批次 %d 库存 %d", confirmed.Outcome(), world.batchCount(t, "SYN-GOODS-01"), world.stockOf(t, "SYN-GOODS-01"))
	}
	var damage, other int64
	for _, line := range receipt.Differences() {
		if line.Kind() == domain.ReceiptDiscrepancyDamage {
			damage += line.Quantity()
			continue
		}
		other += line.Quantity()
	}
	if damage != 6 || other != 0 {
		t.Fatalf("破损 %d 其他差异 %d", damage, other)
	}
}

// Covers: AT-WF-042（收货确认的核心）
func TestReceiptPostsKnownGoodsOffTheAdviceAndKeepsUnknownCodes(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-02", 1, "REQ-GOODS-2", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = nil
	}))
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-02", "SYN-FOREIGN-01", 1, "REQ-FOREIGN", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-FOREIGN"), domain.GoodsAliasPlatformCode,
			valueOf(t, domain.NewPlatformName, "SYN-PLATFORM"), domain.AliasValidity{},
		)}
	}))
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 100)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-MIX", advice.ID().String(),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-02", quantity: 3}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-NO-SUCH", quantity: 4}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-FOREIGN", quantity: 5}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 1}),
	))
	receipt := requireReceipt(t, confirmed)
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.stockOf(t, "SYN-GOODS-02") != 3 || world.stockOf(t, "SYN-GOODS-01") != 1 || world.stockOf(t, "SYN-FOREIGN-01") != 0 {
		t.Fatalf("错货入账 %s 货二 %d 货一 %d 别的货主 %d", confirmed.Outcome(), world.stockOf(t, "SYN-GOODS-02"), world.stockOf(t, "SYN-GOODS-01"), world.stockOf(t, "SYN-FOREIGN-01"))
	}
	unknown := map[string]int64{}
	var wrongGoods int64
	for _, line := range receipt.Lines() {
		if line.Result() == domain.ReceiptLineUnknownGoods {
			unknown[line.Observed()] = line.Quantity()
		}
	}
	for _, line := range receipt.Differences() {
		if line.Kind() != domain.ReceiptDiscrepancyWrongGoods {
			continue
		}
		if line.Goods().String() == "SYN-GOODS-02" {
			wrongGoods = line.Quantity()
		}
		if line.Observed() != "" {
			unknown[line.Observed()] = line.Quantity()
		}
	}
	if wrongGoods != 3 || unknown["SYN-NO-SUCH"] != 4 || unknown["SYN-FOREIGN"] != 5 {
		t.Fatalf("错货 %d 认不出 %v", wrongGoods, unknown)
	}
	none := world.confirm(t, receiptCommand(t, "SYN-FACT-NONE", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-ALSO-UNKNOWN", quantity: 7})))
	if none.Outcome() != application.ConfirmReceiptConfirmed || world.movementCount(t) != 1 || world.adviceStatus(t, advice.ID()) != "RECEIVING" {
		t.Fatalf("全部认不出 %s 流水 %d 状态 %s", none.Outcome(), world.movementCount(t), world.adviceStatus(t, advice.ID()))
	}
}

// Covers: AT-WF-043（收货确认的核心）
func TestReceiptLabelAliasUsesTheFactInstant(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-GOODS-01", 2, "REQ-GOODS-2", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = append(spec.Aliases, domain.NewGoodsAlias(
			valueOf(t, domain.NewGoodsAliasCode, "SYN-PLAT-01"), domain.GoodsAliasPlatformCode,
			valueOf(t, domain.NewPlatformName, "SYN-PLATFORM"), domain.AliasValidity{},
		))
	}))
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 10)
	posted := world.confirm(t, receiptCommand(t, "SYN-FACT-ALIAS", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1, alias: "SYN-PLAT-01",
	})))
	if posted.Outcome() != application.ConfirmReceiptConfirmed || world.stockOf(t, "SYN-GOODS-01") != 1 {
		t.Fatalf("有效平台商品码 %s 库存 %d", posted.Outcome(), world.stockOf(t, "SYN-GOODS-01"))
	}
	var alias *string
	err := world.pool.QueryRow(t.Context(),
		`SELECT label_alias FROM warehouse_fulfillment.stock_movement_line WHERE tenant_id = $1 AND goods_code = $2`,
		"SYN-TENANT-01", "SYN-GOODS-01").Scan(&alias)
	if err != nil || alias == nil || *alias != "SYN-PLAT-01" {
		t.Fatalf("贴标别名 %v %v", alias, err)
	}
	rejected := world.confirm(t, receiptCommand(t, "SYN-FACT-BAR", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1, alias: "SYN-BAR-01",
	})))
	if rejected.Outcome() != application.ConfirmReceiptContentInvalid || world.receiptCount(t) != 1 || world.stockOf(t, "SYN-GOODS-01") != 1 {
		t.Fatalf("商品条码别名 %s 事实 %d 库存 %d", rejected.Outcome(), world.receiptCount(t), world.stockOf(t, "SYN-GOODS-01"))
	}
}

// Covers: UC-WF-002 与 AT-WF-043——迟到事实的识别码与贴标别名按发生时刻生效的修订解析，
// 计量单位与是否跟踪批号按入账时的当前修订判。
func TestReceiptLateFactResolvesOldRevisionAliasAndChecksCurrentTracking(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	// rev 1 登记于 baseInstant+1m，带着旧条码与旧平台别名（有效期在事实到达之前已结束）。
	rev1At := baseInstant.Add(time.Minute)
	rev2At := baseInstant.Add(2 * time.Minute)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-LATE-01", 1, "REQ-LATE-1", func(spec *domain.GoodsContentSpec) {
		spec.PackagingLevels = []domain.PackagingLevel{goodsLevelFixture(t, "件", 1)}
		spec.Aliases = []domain.GoodsAlias{
			domain.NewGoodsAlias(
				valueOf(t, domain.NewGoodsAliasCode, "SYN-OLD-SCAN"), domain.GoodsAliasBarcode, domain.PlatformName{},
				domain.NewAliasValidity(rev1At, time.Time{}),
			),
			domain.NewGoodsAlias(
				valueOf(t, domain.NewGoodsAliasCode, "SYN-OLD-PLAT"), domain.GoodsAliasPlatformCode,
				valueOf(t, domain.NewPlatformName, "SYN-PLATFORM"),
				domain.NewAliasValidity(rev1At, rev1At.Add(45*time.Second)),
			),
		}
	}))
	// rev 2 登记于 baseInstant+2m，删掉旧别名，新加包装层级「箱」并改成跟踪批号。
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-LATE-01", 2, "REQ-LATE-2", func(spec *domain.GoodsContentSpec) {
		spec.Aliases = nil
		spec.TrackBatch = true
		spec.PackagingLevels = []domain.PackagingLevel{goodsLevelFixture(t, "件", 1), goodsLevelFixture(t, "箱", 12)}
	}))
	advice := world.adviseGoods(t, "REQ-LATE-ADV", "SYN-LATE-01", "件", 24)
	occurredDuringRev1 := rev1At.Add(30 * time.Second)

	// 发生于 rev 1 期间但没带批号：跟踪批号按入账时当前修订（rev 2）判，答 CONTENT_INVALID。
	missingBatch := receiptCommand(t, "SYN-FACT-LATE-NOBATCH", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-OLD-SCAN", unit: "箱", quantity: 1, alias: "SYN-OLD-PLAT",
	}))
	missingBatch.OccurredAt = occurredDuringRev1
	refused := world.confirm(t, missingBatch)
	if refused.Outcome() != application.ConfirmReceiptContentInvalid || len(refused.Issues()) != 1 ||
		refused.Issues()[0].Kind() != domain.ReceiptIssueBatchRequired || refused.Issues()[0].Line() != 0 {
		t.Fatalf("入账时已跟踪批号：%s %+v", refused.Outcome(), refused.Issues())
	}

	// 补齐批号：扫描旧条码、贴标旧平台别名（有效区间在到达时已过期但在发生时刻有效）、用 rev 2 新加的「箱」入账。
	validLate := receiptCommand(t, "SYN-FACT-LATE-OK", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-OLD-SCAN", unit: "箱", quantity: 1, batch: "LOT-REV2", alias: "SYN-OLD-PLAT",
	}))
	validLate.OccurredAt = occurredDuringRev1
	confirmed := world.confirm(t, validLate)
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.stockOf(t, "SYN-LATE-01") != 1 {
		t.Fatalf("迟到事实按发生时刻解析：%s 库存 %d", confirmed.Outcome(), world.stockOf(t, "SYN-LATE-01"))
	}
	var labelAlias *string
	err := world.pool.QueryRow(t.Context(),
		`SELECT label_alias FROM warehouse_fulfillment.stock_movement_line WHERE tenant_id = $1 AND goods_code = $2`,
		"SYN-TENANT-01", "SYN-LATE-01").Scan(&labelAlias)
	if err != nil || labelAlias == nil || *labelAlias != "SYN-OLD-PLAT" {
		t.Fatalf("贴标别名 %v %v", labelAlias, err)
	}

	// 发生于 rev 2 之后再带已删除的旧平台别名：答 CONTENT_INVALID。
	afterRev2 := receiptCommand(t, "SYN-FACT-AFTER-REV2", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-LATE-01", unit: "件", quantity: 1, batch: "LOT-REV2", alias: "SYN-OLD-PLAT",
	}))
	afterRev2.OccurredAt = rev2At.Add(time.Minute)
	rejectedAfter := world.confirm(t, afterRev2)
	if rejectedAfter.Outcome() != application.ConfirmReceiptContentInvalid || len(rejectedAfter.Issues()) != 1 ||
		rejectedAfter.Issues()[0].Kind() != domain.ReceiptIssueLabelAliasNotValid {
		t.Fatalf("新修订已删旧别名：%s %+v", rejectedAfter.Outcome(), rejectedAfter.Issues())
	}
}

// Covers: UC-WF-004 结果语义——收货内容不合时整条不落，逐行指出全部不合之处，不止于第一处。
func TestReceiptContentInvalidListsEveryLineDefect(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveGoodsRevision(t, goodsRevisionFixture(t, "SYN-OWNER-01", "SYN-STRICT-01", 1, "REQ-STRICT", func(spec *domain.GoodsContentSpec) {
		spec.TrackBatch = true
		spec.TrackExpiry = true
		spec.Aliases = nil
	}))
	advice := world.adviseGoods(t, "REQ-MULTI-DEFECT", "SYN-STRICT-01", "件", 10)
	refused := world.confirm(t, receiptCommand(t, "SYN-FACT-MULTI-DEFECT", advice.ID().String(),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-STRICT-01", unit: "托", quantity: 1, location: "SYN-LOC-MISSING"}),
		mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", unit: "件", quantity: 1, batch: "UNEXPECTED-LOT", alias: "SYN-BAR-01"}),
	))
	if refused.Outcome() != application.ConfirmReceiptContentInvalid || world.receiptCount(t) != 0 || world.adviceStatus(t, advice.ID()) != "ADVISED" {
		t.Fatalf("多行不合：%s 事实 %d 状态 %s", refused.Outcome(), world.receiptCount(t), world.adviceStatus(t, advice.ID()))
	}
	want := []domain.ReceiptContentIssue{
		domain.ReceiptLineIssue(0, domain.ReceiptIssueLocationNotReceiving),
		domain.ReceiptLineIssue(0, domain.ReceiptIssueUnitNotPackagingLevel),
		domain.ReceiptLineIssue(0, domain.ReceiptIssueBatchRequired),
		domain.ReceiptLineIssue(0, domain.ReceiptIssueExpiryRequired),
		domain.ReceiptLineIssue(1, domain.ReceiptIssueBatchNotTracked),
		domain.ReceiptLineIssue(1, domain.ReceiptIssueLabelAliasNotValid),
	}
	got := refused.Issues()
	if len(got) != len(want) {
		t.Fatalf("逐行指出 = %+v，want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("第 %d 处 = %v，want %v", index, got[index], want[index])
		}
	}
}

// Covers: AT-WF-045（收货确认的核心）
func TestReceiptLocationMustBeReceivingStaging(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	world.saveZone(t, zoneFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-ZONE-STORE", 1, nil))
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-STORE", 1, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-STORE")
	}))
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 10)
	missing := world.confirm(t, receiptCommand(t, "SYN-FACT-MISS", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1, location: "SYN-LOC-MISSING",
	})))
	stored := world.confirm(t, receiptCommand(t, "SYN-FACT-STORE", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{
		observed: "SYN-GOODS-01", quantity: 1, location: "SYN-LOC-STORE",
	})))
	if missing.Outcome() != application.ConfirmReceiptContentInvalid || stored.Outcome() != application.ConfirmReceiptContentInvalid ||
		world.receiptCount(t) != 0 || world.adviceStatus(t, advice.ID()) != "ADVISED" || world.stockOf(t, "SYN-GOODS-01") != 0 {
		t.Fatalf("库位不合 %s %s 事实 %d 状态 %s", missing.Outcome(), stored.Outcome(), world.receiptCount(t), world.adviceStatus(t, advice.ID()))
	}
	world.saveLocation(t, locationFixture(t, "SYN-TENANT-01", "SYN-WH-01", "SYN-LOC-RCV", 2, func(spec *domain.LocationContentSpec) {
		spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-RCV")
		spec.Deactivated = true
	}))
	posted := world.confirm(t, receiptCommand(t, "SYN-FACT-OFF", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 2})))
	if posted.Outcome() != application.ConfirmReceiptContentInvalid || world.stockOf(t, "SYN-GOODS-01") != 0 || world.adviceStatus(t, advice.ID()) != "ADVISED" || world.receiptCount(t) != 0 {
		t.Fatalf("停用库位仍收 %s 库存 %d 状态 %s", posted.Outcome(), world.stockOf(t, "SYN-GOODS-01"), world.adviceStatus(t, advice.ID()))
	}
}

// Covers: AT-WF-046（收货确认的核心）
func TestReceiptCartonPreAdviceDoesNotLand(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	content, err := domain.NewCartonContentLine(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 1)
	if err != nil {
		t.Fatal(err)
	}
	carton, err := domain.NewCarton(valueOf(t, domain.NewCartonNumber, "SYN-CARTON-1"), []domain.CartonContentLine{content})
	if err != nil {
		t.Fatal(err)
	}
	accepted := world.accept(t, preAdviceAcceptCommand(t, "REQ-CARTON", func(command *application.AcceptPreAdviceCommand) {
		command.Inbound = domain.InboundByCarton
		command.GoodsLines = nil
		command.Cartons = []domain.Carton{carton}
	}))
	if accepted.Outcome() != application.PreAdviceOutcomeAdvised {
		t.Fatalf("整箱预报 %s", accepted.Outcome())
	}
	advice, _ := accepted.PreAdvice()
	result := world.confirm(t, receiptCommand(t, "SYN-FACT-CARTON", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 1})))
	if result.Outcome() != application.ConfirmReceiptContentInvalid || world.receiptCount(t) != 0 || world.adviceStatus(t, advice.ID()) != "ADVISED" || world.stockOf(t, "SYN-GOODS-01") != 0 {
		t.Fatalf("整箱预报配商品行 %s 事实 %d 状态 %s", result.Outcome(), world.receiptCount(t), world.adviceStatus(t, advice.ID()))
	}
}

// Covers: AT-WF-047（收货确认的核心）——已取消之后到达的收货转成待认领到货，不入账。
func TestReceiptCancelledPreAdviceIsRecordedWithoutPosting(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 10)
	var withdrawal application.AcceptPreAdviceResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var withdrawErr error
		withdrawal, withdrawErr = world.handler().Withdraw(ctx, preAdviceWithdrawCommand(t, advice.ID(), "REQ-W"))
		return withdrawErr
	})
	if err != nil || withdrawal.Outcome() != application.PreAdviceOutcomeCancelled {
		t.Fatalf("撤回 %s %v", withdrawal.Outcome(), err)
	}
	goodsLines := world.goodsMovementRows(t)
	cartonLines := world.cartonLineCount(t)
	command := receiptCommand(t, "SYN-FACT-LATE", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 2}))
	late := world.confirm(t, command)
	if late.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.stockOf(t, "SYN-GOODS-01") != 0 ||
		world.goodsMovementRows(t) != goodsLines || world.cartonLineCount(t) != cartonLines || world.unclaimedCount(t) != 1 ||
		world.adviceStatus(t, advice.ID()) != "CANCELLED" || world.receiptCount(t) != 1 {
		t.Fatalf("已取消 %s 库存 %d 行 %d %d 到货 %d 状态 %s 事实 %d",
			late.Outcome(), world.stockOf(t, "SYN-GOODS-01"), world.goodsMovementRows(t), world.cartonLineCount(t),
			world.unclaimedCount(t), world.adviceStatus(t, advice.ID()), world.receiptCount(t))
	}
	again := world.confirm(t, command)
	if again.Outcome() != application.ConfirmReceiptExistingResult || world.receiptCount(t) != 1 || world.unclaimedCount(t) != 1 || world.stockOf(t, "SYN-GOODS-01") != 0 {
		t.Fatalf("再投 %s 事实 %d 到货 %d 库存 %d", again.Outcome(), world.receiptCount(t), world.unclaimedCount(t), world.stockOf(t, "SYN-GOODS-01"))
	}
}

// Covers: AT-WF-051（收货确认的核心）
func TestReceiptReadsShowCumulativeAndHideActorsFromTheOwner(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 100)
	confirmed := world.confirm(t, receiptCommand(t, "SYN-FACT-1", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 60})))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal(confirmed.Outcome())
	}
	ownerBody := receiptReadJSON(t, world, advice.ID().String(), false)
	operatorBody := receiptReadJSON(t, world, advice.ID().String(), true)
	ownerAdvice := objectField(t, ownerBody, "preAdvice")
	operatorAdvice := objectField(t, operatorBody, "preAdvice")
	if quantityField(t, ownerAdvice, "cumulativeReceived", "SYN-GOODS-01") != 60 || quantityField(t, operatorAdvice, "cumulativeReceived", "SYN-GOODS-01") != 60 {
		t.Fatalf("累计实收 货主 %s 运营 %s", ownerAdvice["cumulativeReceived"], operatorAdvice["cumulativeReceived"])
	}
	ownerReceipt := firstObject(t, ownerAdvice, "receipts")
	operatorReceipt := firstObject(t, operatorAdvice, "receipts")
	if _, present := ownerReceipt["device"]; present || hasKey(ownerReceipt, "operator") {
		t.Fatalf("货主读口带了设备或操作者：%s", ownerAdvice["receipts"])
	}
	if string(operatorReceipt["device"]) == "" || string(operatorReceipt["operator"]) == "" {
		t.Fatalf("运营查阅没有设备或操作者：%s", operatorAdvice["receipts"])
	}
	for _, body := range []map[string]json.RawMessage{ownerAdvice, operatorAdvice} {
		for _, absent := range []string{"unclaimedArrival", "anotherArrivalPendingClaim", "conclusion"} {
			if _, present := body[absent]; present {
				t.Fatalf("编了 %s", absent)
			}
		}
	}
}

// Covers: 收货事实的写入在没有环境事务时拒绝（收货确认的核心）
func TestReceiptsRefuseToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	factID := valueOf(t, domain.NewReceiptFactID, "SYN-FACT-1")
	if err := world.receipts.HoldFact(ctx, tenant, factID); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldFact：%v", err)
	}
	if _, err := world.receipts.Save(ctx, domain.RecordedReceipt{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
}

// Covers: 任一成立的行落不下，整笔回滚，预报不得留在收货中（收货确认的核心）
func TestReceiptPostingFailureRollsThePreAdviceBack(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-1", "SYN-GOODS-01", "件", 10)
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		posted, postErr := world.poster().Post(ctx, application.PostStockMovementCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
			Request: stockRequest(t, domain.StockReasonReceipt, "SYN-FACT-ROLL", domain.NewStockLineRequest(
				domain.StockIn, 1, stockCell(t, "SYN-GOODS-01", func(spec *domain.StockCellSpec) {
					spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV")
				}), time.Time{},
			)),
		})
		if postErr != nil {
			return postErr
		}
		if posted.Outcome() != application.PostStockPosted {
			return errors.New(posted.Outcome().String())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("先写入一笔来源相同的流水：%v", err)
	}
	command := receiptCommand(t, "SYN-FACT-ROLL", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 2}))
	err = world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, confirmErr := world.confirmer().Confirm(ctx, command)
		return confirmErr
	})
	if err == nil {
		t.Fatal("入账冲突却形成了答案")
	}
	if world.adviceStatus(t, advice.ID()) != "ADVISED" || world.receiptCount(t) != 0 || world.stockOf(t, "SYN-GOODS-01") != 1 || world.movementCount(t) != 1 {
		t.Fatalf("回滚后状态 %s 事实 %d 库存 %d 流水 %d", world.adviceStatus(t, advice.ID()), world.receiptCount(t), world.stockOf(t, "SYN-GOODS-01"), world.movementCount(t))
	}
}

func receiptReadJSON(t *testing.T, world receiptWorld, adviceID string, operator bool) map[string]json.RawMessage {
	t.Helper()
	var handler http.Handler
	var target string
	if operator {
		intake, err := warehousehttp.NewOperatorOperationsReadIntake(receiptOperatorAuth{})
		if err != nil {
			t.Fatal(err)
		}
		handler = warehousehttp.NewQueryPreAdviceEndpoint(intake, world.queries())
		target = "/warehouse-fulfillment-inbound-pre-advice?ownerAccount=SYN-OWNER-01&preAdviceId=" + adviceID
	} else {
		intake, err := warehousehttp.NewCustomerInboundPreAdviceIntake(receiptCustomerAuth{})
		if err != nil {
			t.Fatal(err)
		}
		handler = warehousehttp.NewGetCustomerInboundPreAdviceEndpoint(intake, world.queries())
		target = "/v1/warehouse/reads/inbound-pre-advice?preAdviceId=" + adviceID
	}
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer presented")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("读口 %d %s", response.Code, response.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func objectField(t *testing.T, body map[string]json.RawMessage, key string) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body[key], &object); err != nil {
		t.Fatalf("%s：%v %s", key, err, body[key])
	}
	return object
}

func firstObject(t *testing.T, body map[string]json.RawMessage, key string) map[string]json.RawMessage {
	t.Helper()
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body[key], &rows); err != nil || len(rows) == 0 {
		t.Fatalf("%s：%v %s", key, err, body[key])
	}
	return rows[0]
}

func quantityField(t *testing.T, body map[string]json.RawMessage, key, goods string) int64 {
	t.Helper()
	var rows []struct {
		GoodsCode string `json:"goodsCode"`
		Quantity  int64  `json:"quantity"`
	}
	if err := json.Unmarshal(body[key], &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.GoodsCode == goods {
			return row.Quantity
		}
	}
	t.Fatalf("%s 里没有 %s：%s", key, goods, body[key])
	return 0
}

func hasKey(object map[string]json.RawMessage, key string) bool {
	_, present := object[key]
	return present
}

type receiptOperatorAuth struct{}

func (receiptOperatorAuth) AuthenticateOperationsRead(context.Context, string) (domain.TenantID, error) {
	return domain.NewTenantID("SYN-TENANT-01")
}

type receiptCustomerAuth struct{}

func (receiptCustomerAuth) AuthenticateWarehouseRead(context.Context, warehousehttp.PresentedCustomerCredential) (warehousehttp.CustomerReadIdentity, error) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return warehousehttp.CustomerReadIdentity{}, err
	}
	owner, err := domain.NewOwnerAccountID("SYN-OWNER-01")
	if err != nil {
		return warehousehttp.CustomerReadIdentity{}, err
	}
	return warehousehttp.CustomerReadIdentity{Tenant: tenant, Owner: owner}, nil
}

func (receiptCustomerAuth) AuthenticateInboundPreAdviceSubmission(context.Context, warehousehttp.PresentedCustomerCredential, func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return warehousehttp.CustomerCommandIdentity{}, errors.New("not used")
}

func (receiptCustomerAuth) AuthenticateInboundPreAdviceArrivalNotice(context.Context, warehousehttp.PresentedCustomerCredential, func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return warehousehttp.CustomerCommandIdentity{}, errors.New("not used")
}

func (receiptCustomerAuth) AuthenticateInboundPreAdviceWithdrawal(context.Context, warehousehttp.PresentedCustomerCredential, func() (string, error)) (warehousehttp.CustomerCommandIdentity, error) {
	return warehousehttp.CustomerCommandIdentity{}, errors.New("not used")
}
