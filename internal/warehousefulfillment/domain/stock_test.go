package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func mustBatchNumber(t *testing.T, raw string) domain.BatchNumber {
	t.Helper()
	number, err := domain.NewBatchNumber(raw)
	if err != nil {
		t.Fatalf("NewBatchNumber(%q): %v", raw, err)
	}
	return number
}

func mustExpiry(t *testing.T, raw string) domain.ExpiryDate {
	t.Helper()
	date, err := domain.NewExpiryDate(raw)
	if err != nil {
		t.Fatalf("NewExpiryDate(%q): %v", raw, err)
	}
	return date
}

func stockSpec(t *testing.T) domain.StockCellSpec {
	t.Helper()
	return domain.StockCellSpec{
		Location:     valueOf(t, domain.NewLocationCode, "SYN-LOC-A"),
		Goods:        valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"),
		Unit:         valueOf(t, domain.NewGoodsUnitName, "件"),
		State:        domain.StockStateGood,
		Alias:        valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-01"),
		InboundBatch: valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-01"),
		BatchNumber:  mustBatchNumber(t, "SYN-LOT-01"),
		Expiry:       mustExpiry(t, "2027-01-31"),
	}
}

func stockCell(t *testing.T, edit func(*domain.StockCellSpec)) domain.StockCell {
	t.Helper()
	spec := stockSpec(t)
	if edit != nil {
		edit(&spec)
	}
	cell, err := domain.NewStockCell(spec)
	if err != nil {
		t.Fatalf("NewStockCell: %v", err)
	}
	return cell
}

func stockLine(t *testing.T, direction domain.StockDirection, quantity int64, edit func(*domain.StockCellSpec)) domain.StockLineRequest {
	t.Helper()
	return domain.NewStockLineRequest(direction, quantity, stockCell(t, edit), time.Time{})
}

func stockRequest(
	t *testing.T,
	reason domain.StockMovementReason,
	lines ...domain.StockLineRequest,
) domain.StockMovementRequest {
	t.Helper()
	return domain.NewStockMovementRequest(
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		reason,
		valueOf(t, domain.NewSourceFactReference, "SYN-FACT-01"),
		time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
		lines,
	)
}

func pieceFactor(unit string) (int64, bool) {
	switch unit {
	case "件":
		return 1, true
	case "箱":
		return 12, true
	default:
		return 0, false
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——库存状态与流水原因都是封闭集，原因分成增项、减项、不改汇总、相抵，只有收货、退货入库、建账导入、加工与拆解的记入形成入库批次。
func TestStockWordsRoundTripAndClassifyEveryReason(t *testing.T) {
	states := []domain.StockState{domain.StockStateGood, domain.StockStatePendingInspection, domain.StockStateDefective}
	for _, state := range states {
		parsed, err := domain.NewStockState(state.String())
		if err != nil || parsed != state || state.String() == "" {
			t.Fatalf("库存状态 %s 往返 = %s，%v", state, parsed, err)
		}
	}
	if _, err := domain.NewStockState("good"); !errors.Is(err, domain.ErrUnknownStockWord) {
		t.Fatalf("未知库存状态 err = %v，want ErrUnknownStockWord", err)
	}
	if _, err := domain.NewStockState(""); !errors.Is(err, domain.ErrUnknownStockWord) {
		t.Fatalf("空库存状态 err = %v，want ErrUnknownStockWord", err)
	}

	cases := []struct {
		reason domain.StockMovementReason
		class  domain.StockReasonClass
		forms  bool
	}{
		{domain.StockReasonReceipt, domain.StockReasonIncrease, true},
		{domain.StockReasonReturnReceipt, domain.StockReasonIncrease, true},
		{domain.StockReasonTakeOnImport, domain.StockReasonIncrease, true},
		{domain.StockReasonCountGain, domain.StockReasonIncrease, false},
		{domain.StockReasonAdjustmentIncrease, domain.StockReasonIncrease, false},
		{domain.StockReasonOutboundConfirmation, domain.StockReasonDecrease, false},
		{domain.StockReasonCountLoss, domain.StockReasonDecrease, false},
		{domain.StockReasonAdjustmentDecrease, domain.StockReasonDecrease, false},
		{domain.StockReasonDestruction, domain.StockReasonDecrease, false},
		{domain.StockReasonInternalMove, domain.StockReasonConserving, false},
		{domain.StockReasonStateChange, domain.StockReasonConserving, false},
		{domain.StockReasonRelabel, domain.StockReasonConserving, false},
		{domain.StockReasonUnitConversion, domain.StockReasonConserving, false},
		{domain.StockReasonProcessing, domain.StockReasonOffsetting, true},
		{domain.StockReasonDisassembly, domain.StockReasonOffsetting, true},
		{domain.StockReasonUnboxing, domain.StockReasonOffsetting, false},
	}
	if len(cases) != 16 {
		t.Fatalf("原因 %d 种，封闭集应当被这一张表逐一断言", len(cases))
	}
	seen := map[string]bool{}
	for _, item := range cases {
		word := item.reason.String()
		if word == "" || seen[word] {
			t.Fatalf("原因 %s 的字面量空了或重复", item.reason)
		}
		seen[word] = true
		parsed, err := domain.NewStockMovementReason(word)
		if err != nil || parsed != item.reason {
			t.Fatalf("原因 %s 往返 = %s，%v", word, parsed, err)
		}
		if item.reason.Class() != item.class || item.reason.FormsInboundBatch() != item.forms {
			t.Fatalf("%s 类 = %d（want %d），形成批次 = %v（want %v）", word, item.reason.Class(), item.class, item.reason.FormsInboundBatch(), item.forms)
		}
	}
	if _, err := domain.NewStockMovementReason("RECEIVE"); !errors.Is(err, domain.ErrUnknownStockWord) {
		t.Fatalf("未知原因 err = %v，want ErrUnknownStockWord", err)
	}
	if domain.StockMovementReasonInvalid.Class() != domain.StockReasonClassInvalid || domain.StockMovementReasonInvalid.FormsInboundBatch() {
		t.Fatal("零值原因不该落入任何一类，也不形成入库批次")
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——商品库存只放在与状态相容的库区：存储与拣选只放良品，次品库区只放次品，收货暂存、退货暂存与待出库三种状态都可以放。
func TestStockStateIsAllowedOnlyInACompatibleZone(t *testing.T) {
	want := map[domain.StockState]map[domain.ZoneUsage]bool{
		domain.StockStateGood: {
			domain.ZoneUsageReceivingStaging: true,
			domain.ZoneUsageStorage:          true,
			domain.ZoneUsagePicking:          true,
			domain.ZoneUsageDefective:        false,
			domain.ZoneUsageReturnStaging:    true,
			domain.ZoneUsageOutboundStaging:  true,
		},
		domain.StockStatePendingInspection: {
			domain.ZoneUsageReceivingStaging: true,
			domain.ZoneUsageStorage:          false,
			domain.ZoneUsagePicking:          false,
			domain.ZoneUsageDefective:        false,
			domain.ZoneUsageReturnStaging:    true,
			domain.ZoneUsageOutboundStaging:  true,
		},
		domain.StockStateDefective: {
			domain.ZoneUsageReceivingStaging: true,
			domain.ZoneUsageStorage:          false,
			domain.ZoneUsagePicking:          false,
			domain.ZoneUsageDefective:        true,
			domain.ZoneUsageReturnStaging:    true,
			domain.ZoneUsageOutboundStaging:  true,
		},
	}
	for _, state := range []domain.StockState{domain.StockStateGood, domain.StockStatePendingInspection, domain.StockStateDefective} {
		for _, usage := range domain.ZoneUsages() {
			allowed, listed := want[state][usage]
			if !listed {
				t.Fatalf("缺了 %s × %s 这一格", state, usage)
			}
			if state.AllowedIn(usage) != allowed {
				t.Errorf("%s 放进 %s = %v，want %v", state, usage, state.AllowedIn(usage), allowed)
			}
		}
	}
	if domain.StockStateInvalid.AllowedIn(domain.ZoneUsageStorage) || domain.StockStateGood.AllowedIn(domain.ZoneUsageInvalid) {
		t.Fatal("零值状态或零值用途不该相容")
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——效期是失效日期，只到日；不是真实存在的公历日就没有这一维，零值就是没给。
func TestExpiryDateAcceptsARealDayAndRejectsAnythingElse(t *testing.T) {
	date := mustExpiry(t, "2026-11-20")
	if date.String() != "2026-11-20" || !date.Given() {
		t.Fatalf("合法效期 = %q，given %v", date, date.Given())
	}
	leap := mustExpiry(t, "2024-02-29")
	if leap.String() != "2024-02-29" {
		t.Fatalf("闰日 = %s", leap)
	}
	for _, raw := range []string{"2026-02-30", "2026-02-29", "2026-1-20", "2026/11/20", " 2026-11-20", ""} {
		_, err := domain.NewExpiryDate(raw)
		if err == nil {
			t.Fatalf("效期 %q 不该被收下", raw)
		}
	}
	_, err := domain.NewExpiryDate("2026-02-30")
	if !errors.Is(err, domain.ErrInvalidExpiryDate) || strings.Contains(err.Error(), "2026-02-30") {
		t.Fatalf("2 月 30 日 err = %v，正文不该带回传入的串", err)
	}
	var zero domain.ExpiryDate
	if zero.Given() || zero.String() != "" {
		t.Fatalf("零值效期 = %q，given %v", zero, zero.Given())
	}
	number := mustBatchNumber(t, " SYN-LOT ")
	if number.String() != " SYN-LOT " || !number.Given() {
		t.Fatalf("批号要逐字保存，得到 %q", number)
	}
	if _, err := domain.NewBatchNumber("  "); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空白批号 err = %v，want ErrBlankValue", err)
	}
	var noNumber domain.BatchNumber
	if noNumber.Given() {
		t.Fatal("零值批号不该算给了")
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——增项只有记入，减项只有取走，不改汇总与相抵两类记入与取走都要有；数量为正。
func TestStockMovementShapeFollowsTheReasonClass(t *testing.T) {
	in := func(quantity int64) domain.StockLineRequest {
		t.Helper()
		return stockLine(t, domain.StockIn, quantity, nil)
	}
	out := func(quantity int64) domain.StockLineRequest {
		t.Helper()
		return stockLine(t, domain.StockOut, quantity, nil)
	}
	cases := []struct {
		name   string
		reason domain.StockMovementReason
		lines  []domain.StockLineRequest
		defect bool
	}{
		{"收货只有记入", domain.StockReasonReceipt, []domain.StockLineRequest{in(2)}, false},
		{"收货夹了取走", domain.StockReasonReceipt, []domain.StockLineRequest{in(2), out(2)}, true},
		{"出库确认只有取走", domain.StockReasonOutboundConfirmation, []domain.StockLineRequest{out(2)}, false},
		{"出库确认夹了记入", domain.StockReasonOutboundConfirmation, []domain.StockLineRequest{out(2), in(2)}, true},
		{"库内移动有记入有取走", domain.StockReasonInternalMove, []domain.StockLineRequest{out(2), in(2)}, false},
		{"库内移动只有记入", domain.StockReasonInternalMove, []domain.StockLineRequest{in(2)}, true},
		{"库内移动只有取走", domain.StockReasonInternalMove, []domain.StockLineRequest{out(2)}, true},
		{"加工有记入有取走", domain.StockReasonProcessing, []domain.StockLineRequest{out(2), in(1)}, false},
		{"加工只有记入", domain.StockReasonProcessing, []domain.StockLineRequest{in(1)}, true},
		{"零行", domain.StockReasonReceipt, nil, true},
		{"数量为零", domain.StockReasonReceipt, []domain.StockLineRequest{in(0)}, true},
		{"数量为负", domain.StockReasonReceipt, []domain.StockLineRequest{in(-1)}, true},
		{"方向无效", domain.StockReasonReceipt, []domain.StockLineRequest{domain.NewStockLineRequest(domain.StockDirectionInvalid, 1, stockCell(t, nil), time.Time{})}, true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := stockRequest(t, item.reason, item.lines...).ShapeDefect()
			if got != item.defect {
				t.Fatalf("ShapeDefect = %v，want %v", got, item.defect)
			}
		})
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——库内移动、状态转换、商品换标与形态转换不改按商品折成基本单位的汇总；组内不相抵就是有缺陷。加工这类原因不在这里判。
func TestConservingMovementsMustBalanceWithinTheirGroup(t *testing.T) {
	factor := func(_ domain.GoodsCode, unit domain.GoodsUnitName) (int64, bool) {
		return pieceFactor(unit.String())
	}
	move := func(lines ...domain.StockLineRequest) bool {
		t.Helper()
		return stockRequest(t, domain.StockReasonInternalMove, lines...).ConservationDefect(factor)
	}
	outAt := func(quantity int64, edit func(*domain.StockCellSpec)) domain.StockLineRequest {
		t.Helper()
		return stockLine(t, domain.StockOut, quantity, edit)
	}
	inAt := func(quantity int64, edit func(*domain.StockCellSpec)) domain.StockLineRequest {
		t.Helper()
		return stockLine(t, domain.StockIn, quantity, edit)
	}
	otherLocation := func(spec *domain.StockCellSpec) {
		spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-B")
	}
	if move(outAt(4, nil), inAt(4, otherLocation)) {
		t.Fatal("库内移动数量相抵，不该有缺陷")
	}
	if !move(outAt(4, nil), inAt(3, otherLocation)) {
		t.Fatal("库内移动数量不相抵")
	}
	if !move(outAt(4, nil), inAt(4, func(spec *domain.StockCellSpec) {
		otherLocation(spec)
		spec.Unit = valueOf(t, domain.NewGoodsUnitName, "箱")
	})) {
		t.Fatal("库内移动换了计量单位，组不同，不该算相抵")
	}
	if !move(outAt(4, nil), inAt(4, func(spec *domain.StockCellSpec) {
		otherLocation(spec)
		spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-02")
	})) {
		t.Fatal("库内移动换了贴标别名，组不同，不该算相抵")
	}

	state := stockRequest(t, domain.StockReasonStateChange,
		outAt(4, nil),
		inAt(4, func(spec *domain.StockCellSpec) { spec.State = domain.StockStateDefective }),
	)
	if state.ConservationDefect(factor) {
		t.Fatal("状态转换换了状态、数量相抵，不该有缺陷")
	}

	relabel := stockRequest(t, domain.StockReasonRelabel,
		outAt(4, nil),
		inAt(4, func(spec *domain.StockCellSpec) { spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-02") }),
	)
	if relabel.ConservationDefect(factor) {
		t.Fatal("商品换标换了别名、数量相抵，不该有缺陷")
	}
	relabelUnit := stockRequest(t, domain.StockReasonRelabel,
		outAt(4, nil),
		inAt(4, func(spec *domain.StockCellSpec) {
			spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-02")
			spec.Unit = valueOf(t, domain.NewGoodsUnitName, "箱")
		}),
	)
	if !relabelUnit.ConservationDefect(factor) {
		t.Fatal("商品换标连计量单位也换了，不该算相抵")
	}

	box := func(spec *domain.StockCellSpec) { spec.Unit = valueOf(t, domain.NewGoodsUnitName, "箱") }
	convert := func(outQty, inQty int64, factor func(domain.GoodsCode, domain.GoodsUnitName) (int64, bool)) bool {
		t.Helper()
		request := stockRequest(t, domain.StockReasonUnitConversion, outAt(outQty, box), inAt(inQty, nil))
		return request.ConservationDefect(factor)
	}
	if convert(1, 12, factor) {
		t.Fatal("1 箱折合 12 件换成 12 件，应当相抵")
	}
	if !convert(1, 11, factor) {
		t.Fatal("1 箱换成 11 件，不该相抵")
	}
	missing := func(_ domain.GoodsCode, unit domain.GoodsUnitName) (int64, bool) {
		if unit.String() == "箱" {
			return 12, true
		}
		return 0, false
	}
	if !convert(1, 12, missing) {
		t.Fatal("件的折合数答不出，应当有缺陷")
	}
	overflow := func(_ domain.GoodsCode, _ domain.GoodsUnitName) (int64, bool) { return 4, true }
	// 2^62 × 4 = 2^64，静默回绕成 0 会把这一组算成相抵。
	if !convert(1<<62, 1<<62, overflow) {
		t.Fatal("折合乘法溢出，应当有缺陷")
	}

	receipt := stockRequest(t, domain.StockReasonReceipt, inAt(1, nil), inAt(3, nil))
	called := false
	if receipt.ConservationDefect(func(domain.GoodsCode, domain.GoodsUnitName) (int64, bool) {
		called = true
		return 0, false
	}) || called {
		t.Fatal("收货不判组内相抵，也不该去问折合数")
	}
	processing := stockRequest(t, domain.StockReasonProcessing, outAt(5, nil), inAt(1, func(spec *domain.StockCellSpec) {
		spec.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-02")
	}))
	if processing.ConservationDefect(factor) {
		t.Fatal("加工不要求按商品相抵")
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——同一判重位置上，内容相同才是重放。时刻按同一瞬间比，时区写法和同一微秒里的纳秒差都还是同一份。
func TestStockMovementRequestSameAsIgnoresTimeZoneAndSubMicrosecondNoise(t *testing.T) {
	occurred := time.Date(2026, 11, 20, 8, 0, 0, 1500, time.UTC)
	declared := time.Date(2026, 1, 2, 0, 0, 0, 1800, time.UTC)
	baseLine := func(quantity int64, declaredAt time.Time, edit func(*domain.StockCellSpec)) domain.StockLineRequest {
		t.Helper()
		return domain.NewStockLineRequest(domain.StockIn, quantity, stockCell(t, edit), declaredAt)
	}
	build := func(owner, warehouse, source string, reason domain.StockMovementReason, at time.Time, lines ...domain.StockLineRequest) domain.StockMovementRequest {
		t.Helper()
		return domain.NewStockMovementRequest(
			valueOf(t, domain.NewOwnerAccountID, owner),
			valueOf(t, domain.NewWarehouseCode, warehouse),
			reason,
			valueOf(t, domain.NewSourceFactReference, source),
			at,
			lines,
		)
	}
	lineA := baseLine(2, declared, nil)
	lineB := baseLine(3, time.Time{}, func(spec *domain.StockCellSpec) {
		spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-B")
	})
	base := build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, lineA, lineB)
	if !base.SameAs(build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, lineA, lineB)) {
		t.Fatal("原样相同应当 SameAs")
	}
	plus8 := time.FixedZone("SYN+8", 8*3600)
	sameInstant := occurred.In(plus8)
	if !occurred.Equal(sameInstant) {
		t.Fatal("测试前提：两个写法不是同一瞬间")
	}
	if !base.SameAs(build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, sameInstant, lineA, lineB)) {
		t.Fatal("同一瞬间的不同时区写法应当 SameAs")
	}
	sameMicro := occurred.Add(300 * time.Nanosecond)
	if !base.SameAs(build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, sameMicro, lineA, lineB)) {
		t.Fatal("同一微秒里的纳秒差应当 SameAs")
	}
	later := occurred.Add(time.Microsecond)
	if base.SameAs(build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, later, lineA, lineB)) {
		t.Fatal("差了一微秒不该 SameAs")
	}
	if base.SameAs(build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, lineB, lineA)) {
		t.Fatal("行的先后不同不该 SameAs")
	}

	otherDeclared := domain.NewStockLineRequest(domain.StockIn, 2, lineA.Cell(), declared.Add(time.Microsecond))
	differing := []struct {
		name    string
		request domain.StockMovementRequest
	}{
		{"货主", build("SYN-OWNER-02", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, lineA, lineB)},
		{"仓", build("SYN-OWNER-01", "SYN-WH-02", "SYN-FACT-01", domain.StockReasonReceipt, occurred, lineA, lineB)},
		{"原因", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonCountGain, occurred, lineA, lineB)},
		{"来源事实", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-02", domain.StockReasonReceipt, occurred, lineA, lineB)},
		{"数量", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(9, declared, nil), lineB)},
		{"方向", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, domain.NewStockLineRequest(domain.StockOut, 2, lineA.Cell(), declared), lineB)},
		{"声明的入库时间", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, otherDeclared, lineB)},
		{"库位", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.Location = valueOf(t, domain.NewLocationCode, "SYN-LOC-C")
		}), lineB)},
		{"商品", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-02")
		}), lineB)},
		{"计量单位", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.Unit = valueOf(t, domain.NewGoodsUnitName, "箱")
		}), lineB)},
		{"库存状态", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.State = domain.StockStatePendingInspection
		}), lineB)},
		{"贴标别名", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.Alias = valueOf(t, domain.NewGoodsAliasCode, "SYN-FNSKU-09")
		}), lineB)},
		{"入库批次", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.InboundBatch = valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-09")
		}), lineB)},
		{"批号", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.BatchNumber = mustBatchNumber(t, "SYN-LOT-09")
		}), lineB)},
		{"效期", build("SYN-OWNER-01", "SYN-WH-01", "SYN-FACT-01", domain.StockReasonReceipt, occurred, baseLine(2, declared, func(spec *domain.StockCellSpec) {
			spec.Expiry = mustExpiry(t, "2028-01-31")
		}), lineB)},
	}
	for _, item := range differing {
		t.Run(item.name, func(t *testing.T) {
			if base.SameAs(item.request) {
				t.Fatalf("只差%s 不该 SameAs", item.name)
			}
		})
	}
}

// Covers: CONTEXT「库存账与守恒」（库存账的核心）——重放比的是当初的请求。形成入库批次的行当时没有批次标识，还原时清空；点名的批次原样留下。
func TestStockMovementRequestRestoresFormedBatchesAsUnnamed(t *testing.T) {
	outCell := stockCell(t, nil)
	inCell := stockCell(t, func(spec *domain.StockCellSpec) {
		spec.InboundBatch = valueOf(t, domain.NewInboundBatchID, "SYN-BATCH-02")
		spec.Goods = valueOf(t, domain.NewGoodsCode, "SYN-GOODS-02")
	})
	declared := time.Date(2026, 5, 1, 10, 0, 0, 1500, time.FixedZone("SYN+8", 8*3600))
	occurred := time.Date(2026, 11, 20, 8, 0, 0, 1800, time.UTC)
	posted := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	lines := []domain.StockMovementLine{
		domain.NewStockMovementLine(domain.StockOut, 2, outCell, time.Time{}, false),
		domain.NewStockMovementLine(domain.StockIn, 1, inCell, declared, true),
	}
	movement, err := domain.NewStockMovement(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewStockMovementID, "SYN-MOVE-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		domain.StockReasonProcessing,
		valueOf(t, domain.NewSourceFactReference, "SYN-FACT-01"),
		occurred, posted, lines,
	)
	if err != nil {
		t.Fatalf("NewStockMovement: %v", err)
	}
	restored := movement.Request()
	got := restored.Lines()
	if len(got) != 2 {
		t.Fatalf("还原了 %d 行", len(got))
	}
	if got[0].Direction() != domain.StockOut || got[0].Quantity() != 2 || got[0].Cell() != outCell {
		t.Fatalf("取走行应当原样留下，得到方向 %d 数量 %d", got[0].Direction(), got[0].Quantity())
	}
	if got[0].Cell().InboundBatch() != outCell.InboundBatch() {
		t.Fatalf("点名的入库批次被清空了：%s", got[0].Cell().InboundBatch())
	}
	cleared := inCell.WithInboundBatch(domain.InboundBatchID{})
	if got[1].Cell() != cleared || got[1].Quantity() != 1 || got[1].Direction() != domain.StockIn {
		t.Fatalf("形成批次的行应当还原成没有入库批次，得到 %s", got[1].Cell().InboundBatch())
	}
	if !got[1].DeclaredInboundAt().Equal(declared.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("声明的入库时间 = %s", got[1].DeclaredInboundAt())
	}
	if !restored.OccurredAt().Equal(occurred.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("事实发生时刻 = %s", restored.OccurredAt())
	}
	original := domain.NewStockMovementRequest(
		movement.Owner(), movement.Warehouse(), movement.Reason(), movement.Source(), occurred,
		[]domain.StockLineRequest{
			domain.NewStockLineRequest(domain.StockOut, 2, outCell, time.Time{}),
			domain.NewStockLineRequest(domain.StockIn, 1, cleared, declared),
		},
	)
	if !restored.SameAs(original) {
		t.Fatal("还原出的请求应当与当初那份 SameAs，包括带纳秒的时刻")
	}

	withoutBatch := inCell.WithInboundBatch(domain.InboundBatchID{})
	if _, err := domain.NewStockMovement(
		movement.Tenant(), movement.ID(), movement.Owner(), movement.Warehouse(), movement.Reason(), movement.Source(),
		occurred, posted,
		[]domain.StockMovementLine{domain.NewStockMovementLine(domain.StockIn, 1, withoutBatch, time.Time{}, true)},
	); err == nil {
		t.Fatal("已入账的行没有入库批次，不该构造得成")
	}
}
