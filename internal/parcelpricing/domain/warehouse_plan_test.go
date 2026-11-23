package domain

import (
	"testing"
	"time"
)

func TestAWarehousePlanPricesPiecesAndStaysPendingWithoutARule(t *testing.T) {
	plan, issue := NewWarehousePlan(warehousePlan())
	if issue != "" || plan.Digest() == "" {
		t.Fatalf("方案 %s", issue)
	}
	priced := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		ID: "CHO-1", Version: 1, Kind: "INBOUND_HANDLING", Occurred: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}},
	})
	if priced.Status != "COMPLETED" || priced.TotalMinor != 400 || len(priced.Codes) != 1 {
		t.Fatalf("评价 %+v", priced)
	}
	pending := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{Kind: "STORAGE"})
	if pending.Status != "PENDING" {
		t.Fatalf("没规则 %s", pending.Status)
	}
	again, _ := NewWarehousePlan(warehousePlan())
	if again.Digest() != plan.Digest() {
		t.Fatal("同一张卡摘要变了")
	}
}

func TestAnUnmeasuredMeasureStaysPendingUntilTheCardAdoptsTheDeclaredValue(t *testing.T) {
	plan, _ := NewWarehousePlan(warehousePlan())
	occurrence := WarehouseOccurrenceSnapshot{
		Kind:     "INBOUND_HANDLING",
		Measures: []WarehouseMeasure{{Unit: "PIECE", Unmeasured: true, Declared: "2"}},
	}
	if got := EvaluateWarehousePlan(plan, occurrence); got.Status != "PENDING" {
		t.Fatalf("拒用声明值 %s", got.Status)
	}
	plan.DeclaredValue = "USE"
	if got := EvaluateWarehousePlan(plan, occurrence); got.Status != "COMPLETED" || got.TotalMinor != 200 {
		t.Fatalf("采用声明值 %+v", got)
	}
}

func warehousePlan() WarehousePlan {
	return WarehousePlan{
		ID: "SYN-PLAN-WH-01", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []WarehouseChargeGroup{{
			Name: "INBOUND", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", OnMiss: "PENDING",
		}},
		Rules: []WarehouseChargeRule{{
			Code: "INBOUND_PIECE", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", Group: "INBOUND", Priority: 1,
			Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT",
		}},
	}
}
