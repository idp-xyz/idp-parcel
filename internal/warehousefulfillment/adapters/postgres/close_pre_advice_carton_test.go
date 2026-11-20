package postgres_test

import (
	"encoding/json"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestClosePreAdviceCartonRecordsTheCartonsThatDidNotArrive(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-CARTON-SHORT", "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")
	confirmed := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-ONE", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false)))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.adviceStatus(t, advice.ID()) != "RECEIVING" {
		t.Fatalf("收货 %s 状态 %s", confirmed.Outcome(), world.adviceStatus(t, advice.ID()))
	}
	before := world.cartonBalances(t, "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")
	goodsRows := world.goodsMovementRows(t)
	closed := world.closeAdvice(t, advice.ID(), "")
	closure, ok := closed.Closure()
	id, has := closure.Discrepancy()
	if closed.Outcome() != application.ClosePreAdviceClosed || !ok || !has || world.adviceStatus(t, advice.ID()) != "CLOSED" ||
		world.goodsMovementRows(t) != goodsRows || !sameBalances(before, world.cartonBalances(t, "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")) {
		t.Fatalf("完结 %s 有差异 %t 状态 %s 商品行 %d", closed.Outcome(), has, world.adviceStatus(t, advice.ID()), world.goodsMovementRows(t))
	}
	if got := cartonShortage(closure.Shortages()); len(got) != 2 || got["SYN-CARTON-2"] != 1 || got["SYN-CARTON-3"] != 1 || got["SYN-CARTON-1"] != 0 {
		t.Fatalf("少收 %v，差异 %s", got, id)
	}
	owner := objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice")
	operator := objectField(t, receiptReadJSON(t, world, advice.ID().String(), true), "preAdvice")
	assertCartonShortageJSON(t, objectField(t, owner, "closure"), false)
	assertCartonShortageJSON(t, objectField(t, operator, "closure"), true)
}

func TestClosePreAdviceCartonWhenEveryCartonArrivedRecordsNoShortage(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-CARTON-FULL", "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")
	confirmed := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-ALL", advice.ID().String(),
		mustCartonLine(t, "SYN-CARTON-1", "", false),
		mustCartonLine(t, "SYN-CARTON-2", "", false),
		mustCartonLine(t, "SYN-CARTON-3", "", false),
	))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal(confirmed.Outcome())
	}
	before := world.cartonBalances(t, "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")
	closed := world.closeAdvice(t, advice.ID(), "")
	closure, ok := closed.Closure()
	_, has := closure.Discrepancy()
	if closed.Outcome() != application.ClosePreAdviceClosed || !ok || has || len(closure.Shortages()) != 0 ||
		world.shortageCount(t) != 0 || !sameBalances(before, world.cartonBalances(t, "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")) {
		t.Fatalf("到齐 %s 少收 %d 签了差异 %t", closed.Outcome(), len(closure.Shortages()), has)
	}
	body := objectField(t, objectField(t, receiptReadJSON(t, world, advice.ID().String(), true), "preAdvice"), "closure")
	if hasKey(body, "shortages") || hasKey(body, "discrepancyId") {
		t.Fatalf("到齐仍带少收 %s", body)
	}
}

func TestClosePreAdviceCartonTreatsAWarehouseAssignedNumberAsArrived(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	held := world.adviseCartons(t, "REQ-HELD", "SYN-CARTON-1")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-HELD", held.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("先入账")
	}
	advice := world.adviseCartons(t, "REQ-RENUMBER", "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")
	confirmed := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-NEW", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "SYN-CARTON-W", false)))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal(confirmed.Outcome())
	}
	before := world.cartonBalances(t, "SYN-CARTON-1", "SYN-CARTON-W", "SYN-CARTON-2", "SYN-CARTON-3")
	closed := world.closeAdvice(t, advice.ID(), "")
	closure, ok := closed.Closure()
	if closed.Outcome() != application.ClosePreAdviceClosed || !ok ||
		!sameBalances(before, world.cartonBalances(t, "SYN-CARTON-1", "SYN-CARTON-W", "SYN-CARTON-2", "SYN-CARTON-3")) {
		t.Fatal(closed.Outcome())
	}
	if got := cartonShortage(closure.Shortages()); len(got) != 2 || got["SYN-CARTON-2"] != 1 || got["SYN-CARTON-3"] != 1 || got["SYN-CARTON-1"] != 0 || got["SYN-CARTON-W"] != 0 {
		t.Fatalf("另编箱号仍被记少收 %v", got)
	}
}

func TestClosePreAdviceCartonIgnoresAnUnlistedCartonThatWasNotPosted(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-FOREIGN", "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")
	confirmed := world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-MIX", advice.ID().String(),
		mustCartonLine(t, "SYN-CARTON-1", "", false),
		mustCartonLine(t, "SYN-CARTON-9", "", false),
	))
	if confirmed.Outcome() != application.ConfirmReceiptConfirmed || world.cartonBalance(t, "SYN-CARTON-9") != 0 {
		t.Fatalf("错箱 %s 余额 %d", confirmed.Outcome(), world.cartonBalance(t, "SYN-CARTON-9"))
	}
	closed := world.closeAdvice(t, advice.ID(), "")
	closure, ok := closed.Closure()
	if closed.Outcome() != application.ClosePreAdviceClosed || !ok {
		t.Fatal(closed.Outcome())
	}
	if got := cartonShortage(closure.Shortages()); len(got) != 2 || got["SYN-CARTON-2"] != 1 || got["SYN-CARTON-3"] != 1 || got["SYN-CARTON-9"] != 0 || got["SYN-CARTON-1"] != 0 {
		t.Fatalf("没列的箱抵消了少收 %v", got)
	}
	if world.cartonBalance(t, "SYN-CARTON-1") != 1 || world.cartonBalance(t, "SYN-CARTON-9") != 0 {
		t.Fatalf("完结改了整箱账 %d %d", world.cartonBalance(t, "SYN-CARTON-1"), world.cartonBalance(t, "SYN-CARTON-9"))
	}
}

func TestClosePreAdviceCartonAgainReturnsTheExistingShortage(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-AGAIN", "SYN-CARTON-1", "SYN-CARTON-2", "SYN-CARTON-3")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-ONE", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("没收成")
	}
	first := world.closeAdvice(t, advice.ID(), "第一次")
	second := world.closeAdvice(t, advice.ID(), "第二次")
	again, ok := second.Closure()
	if first.Outcome() != application.ClosePreAdviceClosed || second.Outcome() != application.ClosePreAdviceExistingResult || !ok ||
		again.Note().String() != "第一次" || world.closureCount(t) != 1 || world.shortageCount(t) != 2 {
		t.Fatalf("重放 %s 说明 %s 完结 %d 少收 %d", second.Outcome(), again.Note().String(), world.closureCount(t), world.shortageCount(t))
	}
	if got := cartonShortage(again.Shortages()); got["SYN-CARTON-2"] != 1 || got["SYN-CARTON-3"] != 1 {
		t.Fatalf("回显 %v", got)
	}
}

func TestClosePreAdviceCartonShortageTakesAnOwnerConclusion(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	advice := world.adviseCartons(t, "REQ-CONCLUSION", "SYN-CARTON-1", "SYN-CARTON-2")
	if world.confirm(t, cartonReceiptCommand(t, "SYN-FACT-ONE", advice.ID().String(), mustCartonLine(t, "SYN-CARTON-1", "", false))).Outcome() != application.ConfirmReceiptConfirmed {
		t.Fatal("没收成")
	}
	closed := world.closeAdvice(t, advice.ID(), "")
	closure, ok := closed.Closure()
	id, has := closure.Discrepancy()
	if !ok || !has {
		t.Fatal("少收没有差异标识")
	}
	recorded := world.conclude(t, ownerConclusion(t, id, domain.ConclusionConfirm, "SYN-KEY-CARTON", ""))
	if recorded.Outcome() != application.ConcludeDiscrepancyRecorded || world.conclusionRows(t, id) != 1 {
		t.Fatalf("结论 %s 行 %d", recorded.Outcome(), world.conclusionRows(t, id))
	}
	owner := objectField(t, objectField(t, receiptReadJSON(t, world, advice.ID().String(), false), "preAdvice"), "closure")
	operator := objectField(t, objectField(t, receiptReadJSON(t, world, advice.ID().String(), true), "preAdvice"), "closure")
	if jsonString(t, objectField(t, owner, "conclusion"), "conclusion") != "CONFIRM" || hasKey(owner, "operator") {
		t.Fatalf("货主读口 %s", owner)
	}
	if jsonString(t, objectField(t, operator, "conclusion"), "conclusion") != "CONFIRM" {
		t.Fatalf("运营查阅 %s", operator)
	}
}

func cartonShortage(lines []domain.PreAdviceShortage) map[string]int64 {
	got := map[string]int64{}
	for _, line := range lines {
		if line.Kind() != "SHORTAGE" || line.Goods().String() != "" || line.Quantity() != 1 {
			got["bad:"+line.Goods().String()+line.Carton().String()] = line.Quantity()
			continue
		}
		got[line.Carton().String()] = line.Quantity()
	}
	return got
}

func (world receiptWorld) cartonBalances(t *testing.T, numbers ...string) map[string]int64 {
	t.Helper()
	got := map[string]int64{}
	for _, number := range numbers {
		got[number] = world.cartonBalance(t, number)
	}
	return got
}

func sameBalances(left, right map[string]int64) bool {
	if len(left) != len(right) {
		return false
	}
	for key, quantity := range left {
		if right[key] != quantity {
			return false
		}
	}
	return true
}

func assertCartonShortageJSON(t *testing.T, closure map[string]json.RawMessage, withOperator bool) {
	t.Helper()
	lines := shortageBodies(t, closure)
	if len(lines) != 2 {
		t.Fatalf("读口少收 %d %s", len(lines), closure["shortages"])
	}
	seen := map[string]bool{}
	for _, line := range lines {
		if line.GoodsCode != "" || line.CartonNumber == "" || line.Kind != "SHORTAGE" || line.Quantity != 1 {
			t.Fatalf("少收行 %+v", line)
		}
		seen[line.CartonNumber] = true
	}
	if !seen["SYN-CARTON-2"] || !seen["SYN-CARTON-3"] || seen["SYN-CARTON-1"] {
		t.Fatalf("箱号 %v", seen)
	}
	if _, present := closure["operator"]; present != withOperator {
		t.Fatalf("操作者 with=%t %s", withOperator, closure["operator"])
	}
}
