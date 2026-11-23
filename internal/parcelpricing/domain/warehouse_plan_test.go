package domain

import (
	"strings"
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

func TestAMissingAttributeStaysPendingWhenTheGroupWouldNotCharge(t *testing.T) {
	sole, issue := NewWarehousePlan(conditionedPlan("NOT_CHARGED", "INBOUND_BOX", "MATERIAL", "BOX"))
	if issue != "" {
		t.Fatal(issue)
	}
	missing := EvaluateWarehousePlan(sole, WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}},
	})
	if missing.Status != "PENDING" || !strings.Contains(missing.Reason, "MATERIAL") || !strings.Contains(missing.Reason, "INBOUND") {
		t.Fatalf("缺属性 %+v", missing)
	}
	covered := conditionedPlan("NOT_CHARGED", "INBOUND_BOX", "MATERIAL", "BOX")
	covered.Rules = append([]WarehouseChargeRule{warehouseChargeRule("INBOUND_ANY", 2, nil)}, covered.Rules...)
	plan, issue := NewWarehousePlan(covered)
	if issue != "" {
		t.Fatal(issue)
	}
	shadowed := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}},
	})
	if shadowed.Status != "PENDING" || !strings.Contains(shadowed.Reason, "MATERIAL") || !strings.Contains(shadowed.Reason, "INBOUND") {
		t.Fatalf("另一条能命中仍应待判断 %+v", shadowed)
	}
}

func TestAPresentButDifferentAttributeIsNotCharged(t *testing.T) {
	plan, issue := NewWarehousePlan(conditionedPlan("NOT_CHARGED", "INBOUND_BOX", "MATERIAL", "BOX"))
	if issue != "" {
		t.Fatal(issue)
	}
	occurrence := WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}},
		Attributes: []WarehouseAttribute{{Name: "MATERIAL", Value: "无"}},
	}
	got := EvaluateWarehousePlan(plan, occurrence)
	if got.Status != "ZERO" || got.TotalMinor != 0 || len(got.Codes) != 0 || got.Reason != "" {
		t.Fatalf("值不同不计收 %+v", got)
	}
	matched, issue := NewWarehousePlan(conditionedPlan("NOT_CHARGED", "INBOUND_NONE", "MATERIAL", "无"))
	if issue != "" {
		t.Fatal(issue)
	}
	charged := EvaluateWarehousePlan(matched, occurrence)
	if charged.Status != "COMPLETED" || charged.TotalMinor != 400 {
		t.Fatalf("「无」是在场的值 %+v", charged)
	}
}

func TestAPresentButDifferentAttributeStaysPendingWhenTheGroupSaysSo(t *testing.T) {
	plan, issue := NewWarehousePlan(conditionedPlan("PENDING", "INBOUND_BOX", "MATERIAL", "BOX"))
	if issue != "" {
		t.Fatal(issue)
	}
	got := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}},
		Attributes: []WarehouseAttribute{{Name: "MATERIAL", Value: "PALLET"}},
	})
	if got.Status != "PENDING" || got.Reason != "MISS:INBOUND" {
		t.Fatalf("未命中待判断 %+v", got)
	}
}

func TestAnAttributeOnTheOccurrenceSatisfiesALineRule(t *testing.T) {
	plan, issue := NewWarehousePlan(linePlan("NOT_CHARGED"))
	if issue != "" {
		t.Fatal(issue)
	}
	fromOccurrence := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "PACKING", Attributes: []WarehouseAttribute{{Name: "MATERIAL", Value: "BOX"}},
		Lines: []WarehouseOccurrenceLine{{
			Kind: "PARCEL", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "2"}},
		}},
	})
	if fromOccurrence.Status != "COMPLETED" || fromOccurrence.TotalMinor != 200 || len(fromOccurrence.Codes) != 1 {
		t.Fatalf("发生项上的属性 %+v", fromOccurrence)
	}
	lineWins := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "PACKING", Attributes: []WarehouseAttribute{{Name: "MATERIAL", Value: "BOX"}},
		Lines: []WarehouseOccurrenceLine{{
			Kind: "PARCEL", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "2"}},
			Attributes: []WarehouseAttribute{{Name: "MATERIAL", Value: "无"}},
		}},
	})
	if lineWins.Status != "ZERO" || lineWins.TotalMinor != 0 {
		t.Fatalf("这一层自己的值 %+v", lineWins)
	}
}

func TestAMissingMeasureStaysPending(t *testing.T) {
	plan, _ := NewWarehousePlan(warehousePlan())
	got := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{Kind: "INBOUND_HANDLING"})
	if got.Status != "PENDING" || got.Reason != "MISSING:PIECE" {
		t.Fatalf("缺计量 %+v", got)
	}
}

func TestAClassedMeasureNamesTheClassWhenTheUnitDiffersOrTheClassIsAbsent(t *testing.T) {
	plan, issue := NewWarehousePlan(volumePlan("REFUSE"))
	if issue != "" {
		t.Fatal(issue)
	}
	mismatch := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING", Measures: []WarehouseMeasure{{Class: "VOLUME", Unit: "CM3", Quantity: "2"}},
	})
	if mismatch.Status != "CONFLICT" || mismatch.Reason != "UNIT:VOLUME" {
		t.Fatalf("单位不一致 %+v", mismatch)
	}
	absent := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "2"}},
	})
	if absent.Status != "PENDING" || absent.Reason != "MISSING:VOLUME" {
		t.Fatalf("缺这一类 %+v", absent)
	}
	priced := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING", Measures: []WarehouseMeasure{{Class: "VOLUME", Unit: "M3", Quantity: "2"}},
	})
	if priced.Status != "COMPLETED" || priced.TotalMinor != 200 {
		t.Fatalf("对得上的体积 %+v", priced)
	}
	condition := volumePlan("REFUSE")
	condition.Rules[0].MeasureClass = ""
	condition.Rules[0].Unit = "PIECE"
	condition.Rules[0].When = []WarehouseCondition{{MeasureClass: "VOLUME", MeasureUnit: "M3", Operator: "GT", Threshold: "0"}}
	conditioned, issue := NewWarehousePlan(condition)
	if issue != "" {
		t.Fatal(issue)
	}
	wrongUnit := EvaluateWarehousePlan(conditioned, WarehouseOccurrenceSnapshot{
		Kind: "INBOUND_HANDLING",
		Measures: []WarehouseMeasure{
			{Unit: "PIECE", Quantity: "4"},
			{Class: "VOLUME", Unit: "CM3", Quantity: "2"},
		},
	})
	if wrongUnit.Status != "CONFLICT" || wrongUnit.Reason != "UNIT:VOLUME" {
		t.Fatalf("条件上的单位不一致 %+v", wrongUnit)
	}
}

func TestADeclaredValueInAnotherUnitStaysPendingUnderUse(t *testing.T) {
	plan, issue := NewWarehousePlan(volumePlan("USE"))
	if issue != "" {
		t.Fatal(issue)
	}
	other := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind:     "INBOUND_HANDLING",
		Measures: []WarehouseMeasure{{Class: "VOLUME", Unit: "M3", Unmeasured: true, Declared: "2", DeclaredUnit: "CM3"}},
	})
	if other.Status != "PENDING" || other.Reason != "DECLARED:VOLUME" || other.TotalMinor != 0 {
		t.Fatalf("另一个体积单位 %+v", other)
	}
	blank := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind:     "INBOUND_HANDLING",
		Measures: []WarehouseMeasure{{Class: "VOLUME", Unit: "M3", Unmeasured: true, Declared: "2"}},
	})
	if blank.Status != "PENDING" || blank.Reason != "DECLARED:VOLUME" {
		t.Fatalf("重量体积不接受空的声明单位 %+v", blank)
	}
	same := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind:     "INBOUND_HANDLING",
		Measures: []WarehouseMeasure{{Class: "VOLUME", Unit: "M3", Unmeasured: true, Declared: "2", DeclaredUnit: "M3"}},
	})
	if same.Status != "COMPLETED" || same.TotalMinor != 200 {
		t.Fatalf("单位相同的声明值 %+v", same)
	}
	pieces, issue := NewWarehousePlan(warehousePlan())
	if issue != "" {
		t.Fatal(issue)
	}
	pieces.DeclaredValue = "USE"
	carton := EvaluateWarehousePlan(pieces, WarehouseOccurrenceSnapshot{
		Kind:     "INBOUND_HANDLING",
		Measures: []WarehouseMeasure{{Unit: "PIECE", Unmeasured: true, Declared: "2", DeclaredUnit: "CARTON"}},
	})
	if carton.Status != "PENDING" || carton.Reason != "DECLARED:PIECE" {
		t.Fatalf("计数类的另一个单位 %+v", carton)
	}
	matched := EvaluateWarehousePlan(pieces, WarehouseOccurrenceSnapshot{
		Kind:     "INBOUND_HANDLING",
		Measures: []WarehouseMeasure{{Unit: "PIECE", Unmeasured: true, Declared: "2", DeclaredUnit: "PIECE"}},
	})
	if matched.Status != "COMPLETED" || matched.TotalMinor != 200 {
		t.Fatalf("计数类单位相同 %+v", matched)
	}
}

func TestAnAgeTierWaivesYoungBatchesAndChargesTheRestPerPiece(t *testing.T) {
	plan, issue := NewWarehousePlan(ageTierPlan("PENDING"))
	if issue != "" {
		t.Fatal(issue)
	}
	got := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "STORAGE",
		Lines: []WarehouseOccurrenceLine{
			{Kind: "INBOUND_BATCH", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}, {Unit: "DAY", Quantity: "29"}}},
			{Kind: "INBOUND_BATCH", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "3"}, {Unit: "DAY", Quantity: "30"}}},
		},
	})
	if got.Status != "COMPLETED" || got.TotalMinor != 300 || len(got.Codes) != 1 || got.Codes[0] != "STORAGE_PIECE" {
		t.Fatalf("两行库龄 %+v", got)
	}
}

func TestAConditionOnAnAbsentMeasureStaysPending(t *testing.T) {
	plan, issue := NewWarehousePlan(ageTierPlan("NOT_CHARGED"))
	if issue != "" {
		t.Fatal(issue)
	}
	missing := EvaluateWarehousePlan(plan, WarehouseOccurrenceSnapshot{
		Kind: "STORAGE",
		Lines: []WarehouseOccurrenceLine{{
			Kind: "INBOUND_BATCH", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}},
		}},
	})
	if missing.Status != "PENDING" || missing.Reason != "MISSING:DAY" {
		t.Fatalf("条件缺计量 %+v", missing)
	}
	covered := ageTierPlan("NOT_CHARGED")
	covered.Rules = append(covered.Rules, WarehouseChargeRule{
		Code: "STORAGE_OPEN", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 5,
		Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT",
	})
	shadowed, issue := NewWarehousePlan(covered)
	if issue != "" {
		t.Fatal(issue)
	}
	got := EvaluateWarehousePlan(shadowed, WarehouseOccurrenceSnapshot{
		Kind: "STORAGE",
		Lines: []WarehouseOccurrenceLine{{
			Kind: "INBOUND_BATCH", Measures: []WarehouseMeasure{{Unit: "PIECE", Quantity: "4"}},
		}},
	})
	if got.Status != "PENDING" || got.Reason != "MISSING:DAY" {
		t.Fatalf("另一条能命中仍应待判断 %+v", got)
	}
}

func TestAnUnmeasuredDayFollowsTheDeclaredUnit(t *testing.T) {
	card := ageTierPlan("PENDING")
	card.DeclaredValue = "USE"
	plan, issue := NewWarehousePlan(card)
	if issue != "" {
		t.Fatal(issue)
	}
	waived := EvaluateWarehousePlan(plan, storageDay("10", "DAY", "4"))
	if waived.Status != "ZERO" || waived.TotalMinor != 0 {
		t.Fatalf("声明库龄未满免收 %+v", waived)
	}
	charged := EvaluateWarehousePlan(plan, storageDay("40", "", "3"))
	if charged.Status != "COMPLETED" || charged.TotalMinor != 300 {
		t.Fatalf("计数类不带声明单位 %+v", charged)
	}
	other := EvaluateWarehousePlan(plan, storageDay("40", "PIECE", "3"))
	if other.Status != "PENDING" || other.Reason != "DECLARED:DAY" {
		t.Fatalf("声明单位不同 %+v", other)
	}
	refused, issue := NewWarehousePlan(ageTierPlan("NOT_CHARGED"))
	if issue != "" {
		t.Fatal(issue)
	}
	pending := EvaluateWarehousePlan(refused, storageDay("40", "DAY", "3"))
	if pending.Status != "PENDING" || pending.Reason != "UNMEASURED:DAY" {
		t.Fatalf("拒用声明值 %+v", pending)
	}
}

func TestAWarehouseConditionMustBeOneShape(t *testing.T) {
	cases := []WarehouseCondition{
		{},
		{Name: "MATERIAL", Equals: "BOX", MeasureUnit: "DAY", Operator: "LT", Threshold: "30"},
		{MeasureUnit: "DAY", Operator: "EQ", Threshold: "30"},
		{MeasureUnit: "DAY", Operator: "lt", Threshold: "30"},
		{MeasureUnit: "DAY", Operator: "LT", Threshold: "thirty"},
		{MeasureUnit: "DAY", Operator: "LT"},
		{Equals: "BOX"},
		{MeasureClass: "MASS", MeasureUnit: "KG", Operator: "GT", Threshold: "1"},
	}
	for _, condition := range cases {
		plan := warehousePlan()
		plan.Rules[0].When = []WarehouseCondition{condition}
		if _, issue := NewWarehousePlan(plan); issue != "CONTENT_INVALID" {
			t.Fatalf("应拒 %+v 得 %q", condition, issue)
		}
	}
	unknown := warehousePlan()
	unknown.Rules[0].MeasureClass = "MASS"
	if _, issue := NewWarehousePlan(unknown); issue != "CONTENT_INVALID" {
		t.Fatalf("规则上的类 %q", issue)
	}
}

func conditionedPlan(onMiss, code, name, equals string) WarehousePlan {
	plan := warehousePlan()
	plan.Groups[0].OnMiss = onMiss
	plan.Rules = []WarehouseChargeRule{warehouseChargeRule(code, 1, []WarehouseCondition{{Name: name, Equals: equals}})}
	return plan
}

func linePlan(onMiss string) WarehousePlan {
	return WarehousePlan{
		ID: "SYN-PLAN-WH-01", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []WarehouseChargeGroup{{
			Name: "PACK", Kind: "PACKING", Level: "LINE", Line: "PARCEL", OnMiss: onMiss,
		}},
		Rules: []WarehouseChargeRule{warehouseChargeRule("PACK_PIECE", 1, []WarehouseCondition{{Name: "MATERIAL", Equals: "BOX"}})},
	}
}

func warehouseChargeRule(code string, priority int, when []WarehouseCondition) WarehouseChargeRule {
	level, line, kind, group := "OCCURRENCE", "", "INBOUND_HANDLING", "INBOUND"
	if code == "PACK_PIECE" {
		level, line, kind, group = "LINE", "PARCEL", "PACKING", "PACK"
	}
	return WarehouseChargeRule{
		Code: code, Kind: kind, Level: level, Line: line, Group: group, Priority: priority,
		Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT", When: when,
	}
}

func volumePlan(declared string) WarehousePlan {
	plan := warehousePlan()
	plan.DeclaredValue = declared
	plan.Rules = []WarehouseChargeRule{{
		Code: "INBOUND_VOLUME", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", Group: "INBOUND", Priority: 1,
		Method: "CHARGE", MeasureClass: "VOLUME", Unit: "M3", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT",
	}}
	return plan
}

func ageTierPlan(onMiss string) WarehousePlan {
	return WarehousePlan{
		ID: "SYN-PLAN-WH-01", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []WarehouseChargeGroup{{
			Name: "STORAGE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", OnMiss: onMiss,
		}},
		Rules: []WarehouseChargeRule{
			{
				Code: "STORAGE_FREE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 1,
				Method: "WAIVE",
				When:   []WarehouseCondition{{MeasureUnit: "DAY", Operator: "LT", Threshold: "30"}},
			},
			{
				Code: "STORAGE_PIECE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 1,
				Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT",
				When: []WarehouseCondition{{MeasureUnit: "DAY", Operator: "GE", Threshold: "30"}},
			},
		},
	}
}

func storageDay(declared, declaredUnit, pieces string) WarehouseOccurrenceSnapshot {
	return WarehouseOccurrenceSnapshot{
		Kind: "STORAGE",
		Lines: []WarehouseOccurrenceLine{{
			Kind: "INBOUND_BATCH",
			Measures: []WarehouseMeasure{
				{Unit: "PIECE", Quantity: pieces},
				{Unit: "DAY", Unmeasured: true, Declared: declared, DeclaredUnit: declaredUnit},
			},
		}},
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
