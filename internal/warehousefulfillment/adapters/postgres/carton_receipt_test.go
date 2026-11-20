package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 整箱收货走真库。箱单数量故意不是 1，用来看累计实收有没有被折成件。

func (world receiptWorld) adviseCartons(t *testing.T, key string, numbers ...string) domain.PreAdvice {
	t.Helper()
	cartons := make([]domain.Carton, 0, len(numbers))
	for _, number := range numbers {
		content, err := domain.NewCartonContentLine(valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 5)
		if err != nil {
			t.Fatal(err)
		}
		carton, err := domain.NewCarton(valueOf(t, domain.NewCartonNumber, number), []domain.CartonContentLine{content})
		if err != nil {
			t.Fatal(err)
		}
		cartons = append(cartons, carton)
	}
	accepted := world.accept(t, preAdviceAcceptCommand(t, key, func(command *application.AcceptPreAdviceCommand) {
		command.Inbound = domain.InboundByCarton
		command.GoodsLines = nil
		command.Cartons = cartons
	}))
	if accepted.Outcome() != application.PreAdviceOutcomeAdvised {
		t.Fatalf("整箱预报 %s：%s", key, accepted.Outcome())
	}
	advice, ok := accepted.PreAdvice()
	if !ok {
		t.Fatal("受理没有回显预报")
	}
	return advice
}

func mustCartonLine(t *testing.T, number, warehouse string, damaged bool) domain.CartonReceiptLine {
	t.Helper()
	var assigned domain.CartonNumber
	if warehouse != "" {
		assigned = valueOf(t, domain.NewCartonNumber, warehouse)
	}
	line, err := domain.NewCartonReceiptLine(
		valueOf(t, domain.NewCartonNumber, number), assigned, damaged,
		valueOf(t, domain.NewLocationCode, "SYN-LOC-RCV"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func cartonReceiptCommand(t *testing.T, fact, advice string, lines ...domain.CartonReceiptLine) application.ConfirmReceiptCommand {
	t.Helper()
	command := receiptCommand(t, fact, advice)
	command.Shape = domain.InboundByCarton
	command.Lines = nil
	command.CartonLines = lines
	return command
}

func (world receiptWorld) cartonBalance(t *testing.T, number string) int64 {
	t.Helper()
	var quantity int64
	err := world.pool.QueryRow(t.Context(),
		`SELECT coalesce(sum(quantity_delta), 0)
		   FROM warehouse_fulfillment.carton_movement_line
		  WHERE tenant_id = $1 AND carton_number = $2`, "SYN-TENANT-01", number).Scan(&quantity)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}

func (world receiptWorld) cartonMovementCount(t *testing.T) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.carton_movement WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) cartonBatchCount(t *testing.T) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.carton_inbound_batch WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (world receiptWorld) ownerCartonOf(t *testing.T, booked string) string {
	t.Helper()
	var owner *string
	err := world.pool.QueryRow(t.Context(),
		`SELECT owner_carton_number
		   FROM warehouse_fulfillment.carton_movement_line
		  WHERE tenant_id = $1 AND carton_number = $2`, "SYN-TENANT-01", booked).Scan(&owner)
	if err != nil {
		t.Fatal(err)
	}
	if owner == nil {
		return ""
	}
	return *owner
}

func (world receiptWorld) goodsMovementRows(t *testing.T) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.stock_movement_line WHERE tenant_id = $1`, "SYN-TENANT-01").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func cartonCumulativeQuantity(t *testing.T, body map[string]json.RawMessage, number string) int64 {
	t.Helper()
	var rows []struct {
		GoodsCode    string `json:"goodsCode"`
		CartonNumber string `json:"cartonNumber"`
		Quantity     int64  `json:"quantity"`
	}
	if err := json.Unmarshal(body["cumulativeReceived"], &rows); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.GoodsCode != "" {
			t.Fatalf("整箱累计实收带了商品 %s", body["cumulativeReceived"])
		}
		if row.CartonNumber == number {
			return row.Quantity
		}
	}
	t.Fatalf("累计实收没有 %s：%s", number, body["cumulativeReceived"])
	return 0
}

func receiptLineObject(t *testing.T, body map[string]json.RawMessage, index int) map[string]json.RawMessage {
	t.Helper()
	var lines []map[string]json.RawMessage
	if err := json.Unmarshal(body["lines"], &lines); err != nil || index >= len(lines) {
		t.Fatalf("行 %d：%v %s", index, err, body["lines"])
	}
	return lines[index]
}

// Covers: AT-WF-044。同一事实里每箱各形成一个入库批次。箱单不进商品库存。
func TestCartonReceiptPostsEachListedCartonAsItsOwnBatch(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-CARTON", "SYN-CARTON-1", "SYN-CARTON-2")
	command := cartonReceiptCommand(t, "SYN-FACT-CARTON", advice.ID().String(),
		mustCartonLine(t, "SYN-CARTON-1", "", false),
		mustCartonLine(t, "SYN-CARTON-2", "", false),
	)
	confirmed := world.confirm(t, command)
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.adviceStatus(t, advice.ID()) != "RECEIVING" {
		t.Fatalf("入账 %s 状态 %s", confirmed.Outcome(), world.adviceStatus(t, advice.ID()))
	}
	receipt := requireReceipt(t, confirmed)
	if receipt.Shape() != domain.InboundByCarton || len(receipt.CartonLines()) != 2 {
		t.Fatalf("行 %s %d", receipt.Shape(), len(receipt.CartonLines()))
	}
	for _, line := range receipt.CartonLines() {
		if line.Result() != domain.ReceiptLinePosted || line.Unposted().String() != "" {
			t.Fatalf("行结果 %s %s", line.Result(), line.Unposted())
		}
	}
	if _, has := receipt.Discrepancy(); has {
		t.Fatal("没有差异却签发了差异")
	}
	if world.cartonBalance(t, "SYN-CARTON-1") != 1 || world.cartonBalance(t, "SYN-CARTON-2") != 1 ||
		world.cartonMovementCount(t) != 1 || world.cartonBatchCount(t) != 2 {
		t.Fatalf("整箱账 余额 %d %d 流水 %d 批次 %d",
			world.cartonBalance(t, "SYN-CARTON-1"), world.cartonBalance(t, "SYN-CARTON-2"),
			world.cartonMovementCount(t), world.cartonBatchCount(t))
	}
	if world.goodsMovementRows(t) != 0 || world.stockOf(t, "SYN-GOODS-01") != 0 || world.movementCount(t) != 0 {
		t.Fatalf("商品库存被写入 行 %d 数量 %d 流水 %d", world.goodsMovementRows(t), world.stockOf(t, "SYN-GOODS-01"), world.movementCount(t))
	}
	replayed := world.confirm(t, command)
	if replayed.Outcome() != application.ConfirmReceiptExistingResult || world.cartonMovementCount(t) != 1 || world.cartonBatchCount(t) != 2 {
		t.Fatalf("重放 %s 流水 %d 批次 %d", replayed.Outcome(), world.cartonMovementCount(t), world.cartonBatchCount(t))
	}
	owner := objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice")
	if cartonCumulativeQuantity(t, owner, "SYN-CARTON-1") != 1 || cartonCumulativeQuantity(t, owner, "SYN-CARTON-2") != 1 {
		t.Fatalf("累计实收 %s", owner["cumulativeReceived"])
	}
	shown := receiptLineObject(t, firstObject(t, owner, "receipts"), 0)
	if jsonString(t, shown, "result") != "POSTED" || jsonString(t, shown, "cartonNumber") != "SYN-CARTON-1" {
		t.Fatalf("读口行 %s", shown)
	}
}

// Covers: AT-WF-044。没列的箱和已经收过的箱都不入账，记错货。
func TestCartonReceiptLeavesUnlistedAndRepeatCartonsUnposted(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-CARTON", "SYN-CARTON-1")
	first := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-1", advice.ID().String(),
		mustCartonLine(t, "SYN-CARTON-1", "", false),
		mustCartonLine(t, "SYN-CARTON-ALIEN", "", false),
	))
	if first.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal(first.Outcome())
	}
	posted := requireReceipt(t, first)
	lines := posted.CartonLines()
	if lines[0].Result() != domain.ReceiptLinePosted || lines[1].Unposted() != domain.ReceiptUnpostedWrongCarton {
		t.Fatalf("第一笔 %s %s", lines[0].Result(), lines[1].Unposted())
	}
	if kind, quantity, carton := discrepancyOf(t, posted, domain.ReceiptDiscrepancyWrongGoods); quantity != 1 || carton != "SYN-CARTON-ALIEN" || kind != domain.ReceiptDiscrepancyWrongGoods {
		t.Fatalf("错货 %s %d %s", kind, quantity, carton)
	}
	again := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-2", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false)))
	if again.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal(again.Outcome())
	}
	repeated := requireReceipt(t, again)
	if repeated.CartonLines()[0].Unposted() != domain.ReceiptUnpostedWrongCarton {
		t.Fatalf("再收 %s", repeated.CartonLines()[0].Unposted())
	}
	if _, quantity, carton := discrepancyOf(t, repeated, domain.ReceiptDiscrepancyWrongGoods); quantity != 1 || carton != "SYN-CARTON-1" {
		t.Fatalf("已收再记 %d %s", quantity, carton)
	}
	if world.cartonBalance(t, "SYN-CARTON-1") != 1 || world.cartonBalance(t, "SYN-CARTON-ALIEN") != 0 || world.goodsMovementRows(t) != 0 {
		t.Fatalf("余额 %d 外来 %d 商品行 %d", world.cartonBalance(t, "SYN-CARTON-1"), world.cartonBalance(t, "SYN-CARTON-ALIEN"), world.goodsMovementRows(t))
	}
	owner := objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice")
	if cartonCumulativeQuantity(t, owner, "SYN-CARTON-1") != 1 {
		t.Fatalf("累计 %s", owner["cumulativeReceived"])
	}
}

// Covers: AT-WF-044。货主箱号与在库重复、没带仓另编的箱号：不入账，不形成差异。
func TestCartonReceiptDuplicateOnHandNumberDoesNotPostOrDiffer(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	held := world.adviseCartons(t, "REQ-HELD", "SYN-CARTON-1")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-HELD", held.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("先入账")
	}
	advice := world.adviseCartons(t, "REQ-AGAIN", "SYN-CARTON-1")
	result := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-DUP", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false)))
	if result.Outcome() != application.ConfirmReceiptConfirmed || world.adviceStatus(t, advice.ID()) != "RECEIVING" {
		t.Fatalf("重复箱号 %s 状态 %s", result.Outcome(), world.adviceStatus(t, advice.ID()))
	}
	receipt := requireReceipt(t, result)
	line := receipt.CartonLines()[0]
	if line.Result().String() != "" || line.Unposted() != domain.ReceiptUnpostedCartonNumberInUse {
		t.Fatalf("行 %s %s", line.Result(), line.Unposted())
	}
	if _, has := receipt.Discrepancy(); has {
		t.Fatal("箱号重复形成了差异")
	}
	if world.cartonBalance(t, "SYN-CARTON-1") != 1 || world.cartonMovementCount(t) != 1 || world.goodsMovementRows(t) != 0 {
		t.Fatalf("账 余额 %d 流水 %d 商品行 %d", world.cartonBalance(t, "SYN-CARTON-1"), world.cartonMovementCount(t), world.goodsMovementRows(t))
	}
	owner := objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice")
	shown := receiptLineObject(t, firstObject(t, owner, "receipts"), 0)
	if jsonString(t, shown, "cartonNumber") != "SYN-CARTON-1" || jsonString(t, shown, "unpostedReason") != "CARTON_NUMBER_IN_USE" {
		t.Fatalf("读口 %s", shown)
	}
	if _, present := shown["result"]; present {
		t.Fatalf("没入账却有结果 %s", shown["result"])
	}
	if _, present := firstObject(t, owner, "receipts")["discrepancies"]; present {
		t.Fatal("读口带了差异")
	}
	if cartonCumulativeQuantity(t, owner, "SYN-CARTON-1") != 0 {
		t.Fatalf("没入账却计入累计 %s", owner["cumulativeReceived"])
	}
}

// Covers: AT-WF-044。货主箱号与在库重复、带了仓另编的箱号：以那个箱号入账，并记下货主原箱号。
func TestCartonReceiptPostsUnderAWarehouseAssignedNumber(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	held := world.adviseCartons(t, "REQ-HELD", "SYN-CARTON-1")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-HELD", held.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("先入账")
	}
	advice := world.adviseCartons(t, "REQ-RENUMBER", "SYN-CARTON-1")
	result := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-NEW", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "SYN-CARTON-W", false)))
	if result.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal(result.Outcome())
	}
	receipt := requireReceipt(t, result)
	line := receipt.CartonLines()[0]
	if line.Result() != domain.ReceiptLinePosted || line.Number().String() != "SYN-CARTON-1" || line.WarehouseNumber().String() != "SYN-CARTON-W" {
		t.Fatalf("行 %s %s %s", line.Result(), line.Number(), line.WarehouseNumber())
	}
	if _, has := receipt.Discrepancy(); has {
		t.Fatal("另编箱号形成了差异")
	}
	if world.cartonBalance(t, "SYN-CARTON-1") != 1 || world.cartonBalance(t, "SYN-CARTON-W") != 1 || world.ownerCartonOf(t, "SYN-CARTON-W") != "SYN-CARTON-1" {
		t.Fatalf("账 原箱 %d 另编 %d 货主原箱号 %s", world.cartonBalance(t, "SYN-CARTON-1"), world.cartonBalance(t, "SYN-CARTON-W"), world.ownerCartonOf(t, "SYN-CARTON-W"))
	}
	if world.cartonBatchCount(t) != 2 || world.goodsMovementRows(t) != 0 {
		t.Fatalf("批次 %d 商品行 %d", world.cartonBatchCount(t), world.goodsMovementRows(t))
	}
	owner := objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice")
	shown := receiptLineObject(t, firstObject(t, owner, "receipts"), 0)
	if jsonString(t, shown, "result") != "POSTED" || jsonString(t, shown, "warehouseCartonNumber") != "SYN-CARTON-W" || jsonString(t, shown, "cartonNumber") != "SYN-CARTON-1" {
		t.Fatalf("读口 %s", shown)
	}
	if cartonCumulativeQuantity(t, owner, "SYN-CARTON-1") != 1 {
		t.Fatalf("累计 %s", owner["cumulativeReceived"])
	}
}

// Covers: AT-WF-044。外箱破损仍入账，并记破损一箱。
func TestCartonReceiptPostsADamagedCartonAndRecordsDamage(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-DMG", "SYN-CARTON-1")
	result := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-DMG", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", true)))
	if result.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal(result.Outcome())
	}
	receipt := requireReceipt(t, result)
	line := receipt.CartonLines()[0]
	if line.Result() != domain.ReceiptLinePosted || !line.Damaged() {
		t.Fatalf("行 %s 破损 %v", line.Result(), line.Damaged())
	}
	if kind, quantity, carton := discrepancyOf(t, receipt, domain.ReceiptDiscrepancyDamage); kind != domain.ReceiptDiscrepancyDamage || quantity != 1 || carton != "SYN-CARTON-1" {
		t.Fatalf("破损 %s %d %s", kind, quantity, carton)
	}
	if world.cartonBalance(t, "SYN-CARTON-1") != 1 || world.cartonBatchCount(t) != 1 || world.goodsMovementRows(t) != 0 || world.stockOf(t, "SYN-GOODS-01") != 0 {
		t.Fatalf("账 余额 %d 批次 %d 商品行 %d 商品数量 %d", world.cartonBalance(t, "SYN-CARTON-1"), world.cartonBatchCount(t), world.goodsMovementRows(t), world.stockOf(t, "SYN-GOODS-01"))
	}
}

// Covers: AT-WF-046 的整箱一半。商品入库的预报交来整箱行，整条不落，不占事实身份。
func TestCartonReceiptGoodsAdviceWithCartonLinesDoesNotOccupyTheFact(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-GOODS", "SYN-GOODS-01", "件", 10)
	rejected := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-MIX", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false)))
	if rejected.Outcome() != application.ConfirmReceiptContentInvalid || world.receiptCount(t) != 0 || world.adviceStatus(t, advice.ID()) != "ADVISED" {
		t.Fatalf("形态不符 %s 事实 %d 状态 %s", rejected.Outcome(), world.receiptCount(t), world.adviceStatus(t, advice.ID()))
	}
	if world.cartonMovementCount(t) != 0 || world.goodsMovementRows(t) != 0 {
		t.Fatalf("不落却写了账 整箱 %d 商品 %d", world.cartonMovementCount(t), world.goodsMovementRows(t))
	}
	posted := world.confirm(t, receiptCommand(t, "SYN-FACT-MIX", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 2})))
	if posted.Outcome() != application.ConfirmReceiptConfirmed || world.receiptCount(t) != 1 || world.stockOf(t, "SYN-GOODS-01") != 2 {
		t.Fatalf("同一事实随后的商品收货 %s 事实 %d 库存 %d", posted.Outcome(), world.receiptCount(t), world.stockOf(t, "SYN-GOODS-01"))
	}
}

// Covers: 整箱账的写入在没有环境事务时拒绝。
func TestCartonLedgerRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	ctx := context.Background()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	if err := world.cartons.Serialize(ctx, tenant, owner, warehouse); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Serialize：%v", err)
	}
	if err := world.cartons.Append(ctx, domain.CartonMovement{}, nil); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Append：%v", err)
	}
}

func discrepancyOf(t *testing.T, receipt domain.RecordedReceipt, kind domain.ReceiptDiscrepancyKind) (domain.ReceiptDiscrepancyKind, int64, string) {
	t.Helper()
	for _, line := range receipt.Differences() {
		if line.Kind() == kind {
			return line.Kind(), line.Quantity(), line.Carton().String()
		}
	}
	t.Fatalf("没有 %s：%d 行差异", kind, len(receipt.Differences()))
	return 0, 0, ""
}
