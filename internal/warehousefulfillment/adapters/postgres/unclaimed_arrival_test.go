package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 收货事实到达时预报已不能收货：原事实保全成一件待认领到货。认领与处置不在这一段。

const (
	unclaimedScan     = "SYN-SCAN-UNCLAIMED"
	unclaimedQuantity = int64(47)
)

type ledgerSnapshot struct {
	goodsLines      int
	goodsMovements  int
	cartonLines     int
	cartonMovements int
	goodsBatches    int
	cartonBatches   int
}

func (world receiptWorld) ledgerNow(t *testing.T) ledgerSnapshot {
	t.Helper()
	return ledgerSnapshot{
		goodsLines:      world.goodsMovementRows(t),
		goodsMovements:  world.movementCount(t),
		cartonLines:     world.cartonLineCount(t),
		cartonMovements: world.cartonMovementCount(t),
		goodsBatches:    world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.inbound_batch WHERE tenant_id = $1`),
		cartonBatches:   world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.carton_inbound_batch WHERE tenant_id = $1`),
	}
}

func (world receiptWorld) cartonLineCount(t *testing.T) int {
	t.Helper()
	return world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.carton_movement_line WHERE tenant_id = $1`)
}

func (world receiptWorld) unclaimedCount(t *testing.T) int {
	t.Helper()
	return world.countRows(t, `SELECT count(*) FROM warehouse_fulfillment.unclaimed_arrival WHERE tenant_id = $1`)
}

func (world receiptWorld) withdrawAdvice(t *testing.T, id domain.PreAdviceID, key string) {
	t.Helper()
	var withdrawal application.AcceptPreAdviceResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var withdrawErr error
		withdrawal, withdrawErr = world.handler().Withdraw(ctx, preAdviceWithdrawCommand(t, id, key))
		return withdrawErr
	})
	if err != nil || withdrawal.Outcome() != application.PreAdviceOutcomeCancelled {
		t.Fatalf("撤回 %s %v", withdrawal.Outcome(), err)
	}
}

func (world receiptWorld) onlyUnclaimedArrival(t *testing.T) (id, fact, advice, owner string) {
	t.Helper()
	rows, err := world.pool.Query(t.Context(),
		`SELECT arrival_id, fact_id, pre_advice_id, owner_account_id
		   FROM warehouse_fulfillment.unclaimed_arrival
		  WHERE tenant_id = $1`, "SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		found++
		if err := rows.Scan(&id, &fact, &advice, &owner); err != nil {
			t.Fatal(err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if found != 1 || !strings.HasPrefix(id, "UCA-") {
		t.Fatalf("待认领到货件数 %d 标识 %s", found, id)
	}
	return id, fact, advice, owner
}

func (before ledgerSnapshot) sameAs(t *testing.T, after ledgerSnapshot) {
	t.Helper()
	if before != after {
		t.Fatalf("库存行变了：之前 %+v 之后 %+v", before, after)
	}
}

func lateGoodsCommand(t *testing.T, fact, advice, scan string, quantity int64) application.ConfirmReceiptCommand {
	t.Helper()
	command := receiptCommand(t, fact, advice, mustReceiptLine(t, receiptLineSpec{observed: scan, quantity: quantity}))
	// 设备记下的发生时刻早于撤回或完结。先后只看事实到达本上下文的次序。
	command.OccurredAt = preAdviceNumberingInstant.Add(-2 * time.Hour)
	return command
}

// Covers: AT-WF-047、AT-WF-260。已取消的预报上来收货：不入账，预报仍是已取消，保全一件待认领到货。
// 原样重投不另建；换内容答内容冲突，在册的不变。读口只标有无。
func TestUnclaimedArrivalCancelledPreAdviceStaysOffTheBooks(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	target := world.adviseGoods(t, "REQ-CANCEL", "SYN-GOODS-01", "件", 10)
	clean := world.adviseGoods(t, "REQ-CLEAN", "SYN-GOODS-01", "件", 10)
	world.withdrawAdvice(t, target.ID(), "REQ-W")
	before := world.ledgerNow(t)
	fact := "SYN-FACT-CANCELLED"
	command := lateGoodsCommand(t, fact, target.ID().String(), unclaimedScan, unclaimedQuantity)
	late := world.confirm(t, command)
	if late.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.adviceStatus(t, target.ID()) != "CANCELLED" {
		t.Fatalf("已取消 %s 状态 %s", late.Outcome(), world.adviceStatus(t, target.ID()))
	}
	before.sameAs(t, world.ledgerNow(t))
	arrivalID, arrivalFact, arrivalAdvice, owner := world.onlyUnclaimedArrival(t)
	if arrivalFact != fact || arrivalAdvice != target.ID().String() || owner != "SYN-OWNER-01" {
		t.Fatalf("到货指向 %s %s %s", arrivalFact, arrivalAdvice, owner)
	}
	outcome, observed, quantity := world.divertedGoodsLine(t, fact)
	if outcome != "DIVERTED_TO_UNCLAIMED" || observed != unclaimedScan || quantity != unclaimedQuantity {
		t.Fatalf("原事实 %s %s %d", outcome, observed, quantity)
	}
	occurred, received := world.receiptInstants(t, fact)
	if !occurred.Equal(command.OccurredAt) || !received.Equal(preAdviceNumberingInstant) || !occurred.Before(received) {
		t.Fatalf("发生 %s 接收 %s", occurred, received)
	}
	hidden := []string{arrivalID, unclaimedScan, fact, fmt.Sprintf(`"quantity":%d`, unclaimedQuantity)}
	assertArrivalReads(t, world, target.ID().String(), true, hidden)
	assertArrivalReads(t, world, clean.ID().String(), false, hidden)

	again := world.confirm(t, command)
	replayID, _, _, _ := world.onlyUnclaimedArrival(t)
	if again.Outcome() != application.ConfirmReceiptExistingResult || replayID != arrivalID {
		t.Fatalf("原样重投 %s 到货 %s", again.Outcome(), replayID)
	}
	before.sameAs(t, world.ledgerNow(t))

	conflict := command
	conflict.Lines = []domain.ReceiptLine{mustReceiptLine(t, receiptLineSpec{observed: unclaimedScan, quantity: unclaimedQuantity + 1})}
	rejected := world.confirm(t, conflict)
	keptID, _, _, _ := world.onlyUnclaimedArrival(t)
	_, keptObserved, keptQuantity := world.divertedGoodsLine(t, fact)
	if rejected.Outcome() != application.ConfirmReceiptContentConflict || keptID != arrivalID ||
		keptObserved != unclaimedScan || keptQuantity != unclaimedQuantity {
		t.Fatalf("换内容 %s 到货 %s 事实 %s %d", rejected.Outcome(), keptID, keptObserved, keptQuantity)
	}
	before.sameAs(t, world.ledgerNow(t))
	if world.adviceStatus(t, target.ID()) != "CANCELLED" || world.receiptCount(t) != 1 {
		t.Fatalf("冲突后状态 %s 事实 %d", world.adviceStatus(t, target.ID()), world.receiptCount(t))
	}
}

// Covers: AT-WF-047、AT-WF-049。已完结的预报同样：不入账，预报仍是已完结，读口只标有无。
func TestUnclaimedArrivalClosedPreAdviceStaysOffTheBooks(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-CLOSE", "SYN-GOODS-01", "件", 100)
	if world.confirm(t, receiptCommand(t, "SYN-FACT-EARLY", advice.ID().String(), mustReceiptLine(t, receiptLineSpec{observed: "SYN-GOODS-01", quantity: 40}))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("没收成")
	}
	if world.closeAdvice(t, advice.ID(), "").Outcome() != application.ClosePreAdviceClosed {
		t.Fatal("没完结")
	}
	before := world.ledgerNow(t)
	stock := world.stockOf(t, "SYN-GOODS-01")
	fact := "SYN-FACT-CLOSED"
	command := lateGoodsCommand(t, fact, advice.ID().String(), unclaimedScan, unclaimedQuantity)
	late := world.confirm(t, command)
	if late.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.adviceStatus(t, advice.ID()) != "CLOSED" ||
		world.stockOf(t, "SYN-GOODS-01") != stock || world.shortageCount(t) != 1 {
		t.Fatalf("已完结 %s 状态 %s 库存 %d", late.Outcome(), world.adviceStatus(t, advice.ID()), world.stockOf(t, "SYN-GOODS-01"))
	}
	before.sameAs(t, world.ledgerNow(t))
	arrivalID, arrivalFact, arrivalAdvice, owner := world.onlyUnclaimedArrival(t)
	if arrivalFact != fact || arrivalAdvice != advice.ID().String() || owner != "SYN-OWNER-01" {
		t.Fatalf("到货指向 %s %s %s", arrivalFact, arrivalAdvice, owner)
	}
	outcome, observed, quantity := world.divertedGoodsLine(t, fact)
	if outcome != "DIVERTED_TO_UNCLAIMED" || observed != unclaimedScan || quantity != unclaimedQuantity {
		t.Fatalf("原事实 %s %s %d", outcome, observed, quantity)
	}
	assertArrivalReads(t, world, advice.ID().String(), true, []string{
		arrivalID, unclaimedScan, fact, fmt.Sprintf(`"quantity":%d`, unclaimedQuantity),
	})
}

// Covers: UC-WF-004——已取消的预报直接把原事实保全为待认领到货，坏库位不挡转待认领；还能收货时同一坏库位才答 CONTENT_INVALID。
func TestUnclaimedArrivalIgnoresContentDefectsThatWouldBlockALiveAdvice(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	live := world.adviseGoods(t, "REQ-LIVE-BAD-LOC", "SYN-GOODS-01", "件", 10)
	cancelled := world.adviseGoods(t, "REQ-CANCEL-BAD-LOC", "SYN-GOODS-01", "件", 10)
	world.withdrawAdvice(t, cancelled.ID(), "REQ-W-BAD-LOC")
	badLine := mustReceiptLine(t, receiptLineSpec{observed: unclaimedScan, quantity: unclaimedQuantity, location: "SYN-LOC-MISSING"})

	refused := world.confirm(t, receiptCommand(t, "SYN-FACT-LIVE-BAD-LOC", live.ID().String(), badLine))
	if refused.Outcome() != application.ConfirmReceiptContentInvalid {
		t.Fatalf("还能收货 %s", refused.Outcome())
	}
	issues := refused.Issues()
	if len(issues) != 1 || issues[0].Kind() != domain.ReceiptIssueLocationNotReceiving || issues[0].Line() != 0 {
		t.Fatalf("逐行指出 = %+v", issues)
	}
	if world.unclaimedCount(t) != 0 || world.receiptCount(t) != 0 {
		t.Fatalf("坏库位不该落册：到货 %d 事实 %d", world.unclaimedCount(t), world.receiptCount(t))
	}

	before := world.ledgerNow(t)
	command := lateGoodsCommand(t, "SYN-FACT-CANCEL-BAD-LOC", cancelled.ID().String(), unclaimedScan, unclaimedQuantity)
	command.Lines = []domain.ReceiptLine{badLine}
	late := world.confirm(t, command)
	if late.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.adviceStatus(t, cancelled.ID()) != "CANCELLED" {
		t.Fatalf("已取消坏库位 %s 状态 %s", late.Outcome(), world.adviceStatus(t, cancelled.ID()))
	}
	before.sameAs(t, world.ledgerNow(t))
	if world.unclaimedCount(t) != 1 {
		t.Fatalf("到货 %d", world.unclaimedCount(t))
	}

	// 整箱入库：还能收货时多行坏库位逐行指出；已取消时坏库位同样直接转待认领。
	liveCarton := world.adviseCartons(t, "REQ-LIVE-CARTON-BAD", "SYN-CARTON-B1", "SYN-CARTON-B2")
	badCarton1, err := domain.NewCartonReceiptLine(valueOf(t, domain.NewCartonNumber, "SYN-CARTON-B1"), domain.CartonNumber{}, true, valueOf(t, domain.NewLocationCode, "SYN-LOC-MISSING-1"))
	if err != nil {
		t.Fatal(err)
	}
	badCarton2, err := domain.NewCartonReceiptLine(valueOf(t, domain.NewCartonNumber, "SYN-CARTON-B2"), domain.CartonNumber{}, true, valueOf(t, domain.NewLocationCode, "SYN-LOC-MISSING-2"))
	if err != nil {
		t.Fatal(err)
	}
	cartonRefused := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-LIVE-CARTON-BAD", liveCarton.ID().String(), badCarton1, badCarton2))
	if cartonRefused.Outcome() != application.ConfirmReceiptContentInvalid || len(cartonRefused.Issues()) != 2 ||
		cartonRefused.Issues()[0] != domain.ReceiptLineIssue(0, domain.ReceiptIssueLocationNotReceiving) ||
		cartonRefused.Issues()[1] != domain.ReceiptLineIssue(1, domain.ReceiptIssueLocationNotReceiving) {
		t.Fatalf("整箱多行坏库位：%s %+v", cartonRefused.Outcome(), cartonRefused.Issues())
	}
	world.withdrawAdvice(t, liveCarton.ID(), "REQ-W-CARTON-BAD")
	cartonLate := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-CANCEL-CARTON-BAD", liveCarton.ID().String(), badCarton1))
	if cartonLate.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.unclaimedCount(t) != 2 {
		t.Fatalf("整箱已取消坏库位：%s 到货 %d", cartonLate.Outcome(), world.unclaimedCount(t))
	}
}

// Covers: AT-WF-047。撤回先成立，随后到达的收货走待认领；设备上更早的发生时刻不把预报重新打开。
func TestUnclaimedArrivalAfterWithdrawalFollowsArrivalOrder(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseGoods(t, "REQ-ORDER", "SYN-GOODS-01", "件", 10)
	world.withdrawAdvice(t, advice.ID(), "REQ-W-ORDER")
	before := world.ledgerNow(t)
	fact := "SYN-FACT-ORDER"
	command := lateGoodsCommand(t, fact, advice.ID().String(), unclaimedScan, unclaimedQuantity)
	late := world.confirm(t, command)
	if late.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.adviceStatus(t, advice.ID()) != "CANCELLED" {
		t.Fatalf("撤回之后 %s 状态 %s", late.Outcome(), world.adviceStatus(t, advice.ID()))
	}
	before.sameAs(t, world.ledgerNow(t))
	occurred, received := world.receiptInstants(t, fact)
	if !occurred.Equal(command.OccurredAt) || !occurred.Before(received) || world.unclaimedCount(t) != 1 {
		t.Fatalf("发生 %s 接收 %s 到货 %d", occurred, received, world.unclaimedCount(t))
	}
	arrivalID, _, _, _ := world.onlyUnclaimedArrival(t)
	assertArrivalReads(t, world, advice.ID().String(), true, []string{arrivalID, unclaimedScan, fact})
}

// Covers: AT-WF-047。整箱入库的预报取消后到达的收货同样不入商品库存，也不入整箱库存。
func TestUnclaimedArrivalCartonPreAdviceStaysOffBothLedgers(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-CARTON-LATE", "SYN-CARTON-1")
	world.withdrawAdvice(t, advice.ID(), "REQ-W-CARTON")
	before := world.ledgerNow(t)
	fact := "SYN-FACT-CARTON-LATE"
	command := cartonReceiptCommand(t, fact, advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", true))
	command.OccurredAt = preAdviceNumberingInstant.Add(-2 * time.Hour)
	late := world.confirm(t, command)
	if late.Outcome() != application.ConfirmReceiptDivertedToUnclaimed || world.adviceStatus(t, advice.ID()) != "CANCELLED" {
		t.Fatalf("整箱 %s 状态 %s", late.Outcome(), world.adviceStatus(t, advice.ID()))
	}
	before.sameAs(t, world.ledgerNow(t))
	arrivalID, arrivalFact, arrivalAdvice, owner := world.onlyUnclaimedArrival(t)
	if arrivalFact != fact || arrivalAdvice != advice.ID().String() || owner != "SYN-OWNER-01" {
		t.Fatalf("到货指向 %s %s %s", arrivalFact, arrivalAdvice, owner)
	}
	var outcome, reason, number string
	var damaged bool
	err := world.pool.QueryRow(t.Context(),
		`SELECT c.outcome, l.unposted_reason, l.damaged, l.carton_number
		   FROM warehouse_fulfillment.receipt_confirmation c
		   JOIN warehouse_fulfillment.receipt_confirmation_carton_line l
		     ON l.tenant_id = c.tenant_id AND l.fact_id = c.fact_id
		  WHERE c.tenant_id = $1 AND c.fact_id = $2`, "SYN-TENANT-01", fact).Scan(&outcome, &reason, &damaged, &number)
	if err != nil || outcome != "DIVERTED_TO_UNCLAIMED" || reason != "NOT_RECEIVABLE" || !damaged || number != "SYN-CARTON-1" {
		t.Fatalf("整箱原事实 %s %s %v %s %v", outcome, reason, damaged, number, err)
	}
	assertArrivalReads(t, world, advice.ID().String(), true, []string{arrivalID, fact, `"damaged":true`})
}

// Covers: 待认领到货的写入在没有环境事务时拒绝。
func TestUnclaimedArrivalRefusesSaveOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	if _, err := world.arrivals.Save(context.Background(), domain.UnclaimedArrival{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Save：%v", err)
	}
}

func (world receiptWorld) divertedGoodsLine(t *testing.T, fact string) (outcome, observed string, quantity int64) {
	t.Helper()
	var goods *string
	var result *string
	err := world.pool.QueryRow(t.Context(),
		`SELECT c.outcome, l.observed_text, l.quantity, l.goods_code, l.result
		   FROM warehouse_fulfillment.receipt_confirmation c
		   JOIN warehouse_fulfillment.receipt_confirmation_line l
		     ON l.tenant_id = c.tenant_id AND l.fact_id = c.fact_id
		  WHERE c.tenant_id = $1 AND c.fact_id = $2`, "SYN-TENANT-01", fact).Scan(&outcome, &observed, &quantity, &goods, &result)
	if err != nil {
		t.Fatal(err)
	}
	if goods != nil || result != nil {
		t.Fatalf("未入账的行却有商品或结果 %v %v", goods, result)
	}
	return outcome, observed, quantity
}

func (world receiptWorld) receiptInstants(t *testing.T, fact string) (occurred, received time.Time) {
	t.Helper()
	err := world.pool.QueryRow(t.Context(),
		`SELECT occurred_at, received_at
		   FROM warehouse_fulfillment.receipt_confirmation
		  WHERE tenant_id = $1 AND fact_id = $2`, "SYN-TENANT-01", fact).Scan(&occurred, &received)
	if err != nil {
		t.Fatal(err)
	}
	return occurred, received
}

func assertArrivalReads(t *testing.T, world receiptWorld, adviceID string, wantFlag bool, hidden []string) {
	t.Helper()
	for _, operator := range []bool{false, true} {
		raw := readPreAdviceRaw(t, world, adviceID, operator)
		advice := preAdviceObject(t, raw)
		_, present := advice["unclaimedArrival"]
		switch {
		case wantFlag && (!present || string(advice["unclaimedArrival"]) != "true"):
			t.Fatalf("运营=%v 未标出待认领 %s", operator, raw)
		case !wantFlag && (present || strings.Contains(raw, `"unclaimedArrival"`)):
			t.Fatalf("运营=%v 不该出现这一格 %s", operator, raw)
		}
		for _, needle := range hidden {
			if strings.Contains(raw, needle) {
				t.Fatalf("运营=%v 读口带出了 %s：%s", operator, needle, raw)
			}
		}
	}
}

func readPreAdviceRaw(t *testing.T, world receiptWorld, adviceID string, operator bool) string {
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
	return response.Body.String()
}

func preAdviceObject(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	return objectField(t, body, "preAdvice")
}
