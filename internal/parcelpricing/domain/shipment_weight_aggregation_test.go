package domain_test

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

// 本文件证 ADR-0364 决定三与决定五里票级计价重量的聚合方式：合计后派生与逐件求和各得各的计价重量，
// 进位点两种读法各算各的；缺尺寸待判断、不退回合计；旧卡仍是合计后派生、写旧版；主单级不开这一选项。

func shipmentPieces(t testing.TB, total string, volumetric *string, pieces ...domain.MemberPiece) domain.MemberManifest {
	t.Helper()
	ids := make([]string, 0, len(pieces))
	for _, piece := range pieces {
		ids = append(ids, piece.ID().String())
	}
	manifest := members(t, total, volumetric, ids...)
	if len(pieces) == 0 {
		return manifest
	}
	withPieces, err := manifest.WithPieces(pieces)
	if err != nil {
		t.Fatalf("逐件：%v", err)
	}
	return withPieces
}

func piece(t testing.TB, id, actual string, sides *domain.Dimensions) domain.MemberPiece {
	t.Helper()
	return pieceIn(t, id, actual, domain.WeightUnitKilogram, sides)
}

func pieceIn(t testing.TB, id, actual string, unit domain.WeightUnit, sides *domain.Dimensions) domain.MemberPiece {
	t.Helper()
	made, err := domain.NewMemberPiece(mustValue(t, domain.NewPackageID, id), weight(t, actual, unit), sides)
	if err != nil {
		t.Fatalf("一件：%v", err)
	}
	return made
}

func manifestIn(t testing.TB, total string, unit domain.WeightUnit, volumetric *domain.Weight, pieces ...domain.MemberPiece) domain.MemberManifest {
	t.Helper()
	ids := make([]domain.PackageID, 0, len(pieces))
	for _, item := range pieces {
		ids = append(ids, item.ID())
	}
	manifest, err := domain.NewMemberManifest(ids, weight(t, total, unit), volumetric)
	if err != nil {
		t.Fatalf("清单：%v", err)
	}
	if len(pieces) == 0 {
		return manifest
	}
	withPieces, err := manifest.WithPieces(pieces)
	if err != nil {
		t.Fatalf("逐件：%v", err)
	}
	return withPieces
}

func segmentedShipmentPlan(t testing.TB, method domain.PricingWeightMethod) domain.PricingPlanVersion {
	t.Helper()
	currency := usd(t)
	entry, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "entry-ship-agg"), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "100", domain.WeightUnitKilogram), money(t, "10", currency))
	if err != nil {
		t.Fatalf("价表行：%v", err)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "table-ship-agg", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	below, err := domain.NewWeightRoundingSegment(domain.RoundingCeiling, weight(t, "0.5", domain.WeightUnitKilogram), weight(t, "21", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("进位段：%v", err)
	}
	open, err := domain.NewOpenEndedWeightRoundingSegment(domain.RoundingCeiling, weight(t, "1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("开口段：%v", err)
	}
	rounding, err := domain.NewSegmentedWeightRoundingPolicy([]domain.WeightRoundingSegment{below, open})
	if err != nil {
		t.Fatalf("进位：%v", err)
	}
	var factor *domain.VolumetricFactor
	if method == domain.PricingWeightMax {
		factorRounding, roundingErr := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.1", domain.WeightUnitKilogram))
		if roundingErr != nil {
			t.Fatalf("体积进位：%v", roundingErr)
		}
		declared, factorErr := domain.NewVolumetricFactor(decimal(t, "5000"), domain.LengthUnitCentimeter, factorRounding)
		if factorErr != nil {
			t.Fatalf("体积系数：%v", factorErr)
		}
		factor = &declared
	}
	policy, err := domain.NewPricingWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "weight-ship-agg", "v1"), method, rounding, factor)
	if err != nil {
		t.Fatalf("重量策略：%v", err)
	}
	plan, err := domain.NewAggregatePricingPlanVersion(domain.AggregationPerShipment,
		versionReference(t, domain.ArtifactPricingPlan, "plan-ship-agg", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"), effectivePeriod(t), table, policy, nil, domain.PricingPlanStructures{})
	if err != nil {
		t.Fatalf("方案：%v", err)
	}
	return plan
}

func declareShipmentWeight(t testing.TB, plan domain.PricingPlanVersion, method domain.ShipmentWeightAggregation, point domain.ShipmentWeightRoundingPoint) domain.PricingPlanVersion {
	t.Helper()
	declared, err := plan.WithShipmentWeightAggregation(method, point)
	if err != nil {
		t.Fatalf("声明聚合方式：%v", err)
	}
	return declared
}

func roundedPricingWeight(t testing.TB, evaluation domain.PricingEvaluation) string {
	t.Helper()
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s，问题 %v", evaluation.Status(), issueCodes(evaluation))
	}
	pricingWeight, ok := evaluation.PricingWeight()
	if !ok {
		t.Fatal("没有计价重量")
	}
	return pricingWeight.RoundedWeight().Value().String()
}

// Covers: ADR-0364 决定三——同一组多件，合计后派生、逐件进位后求和、求和之后进位一次，三种读法各得各的计价重量。
// 21 千克以下按 0.5 千克进位、以上按 1 千克进位：10.3 与 10.7 逐件进位是 10.5 与 11，和是 21.5；未进位的和是 21，再进一次是 21。
// 合计实重另报 20，合计后派生按 20 进位，不拿逐件的和顶替。
func TestShipmentWeightAggregationReadingsDisagreeOnTheSamePieces(t *testing.T) {
	base := segmentedShipmentPlan(t, domain.PricingWeightActualOnly)
	pieces := shipmentPieces(t, "20", nil, piece(t, "pkg-a", "10.3", nil), piece(t, "pkg-b", "10.7", nil))
	input := aggregateInput(t, shipmentSubject(t, "SR-AGG"), pieces)

	undeclared := evaluate(t, "eval-agg-old", base, input)
	totals := evaluate(t, "eval-agg-totals", declareShipmentWeight(t, base, domain.ShipmentWeightTotalsThenDerive, ""), input)
	each := evaluate(t, "eval-agg-each", declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightRoundEachThenSum), input)
	once := evaluate(t, "eval-agg-once", declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightSumThenRoundOnce), input)

	if got := roundedPricingWeight(t, undeclared); got != "20" {
		t.Fatalf("未声明的票级卡计价重量 %s，应按合计 20 进位", got)
	}
	if got := roundedPricingWeight(t, totals); got != "20" {
		t.Fatalf("合计后派生 %s，应是 20", got)
	}
	if got := roundedPricingWeight(t, each); got != "21.5" {
		t.Fatalf("逐件进位后求和 %s，应是 21.5", got)
	}
	if got := roundedPricingWeight(t, once); got != "21" {
		t.Fatalf("求和之后进位一次 %s，应是 21，不该再对 21.5 进成 22", got)
	}
	if !explanationMentions(undeclared, "undeclared, read as totals then derive") {
		t.Fatalf("旧卡解释应记下按旧版读法：%v", undeclared.Explanation())
	}
	if !explanationMentions(totals, "TOTALS_THEN_DERIVE") || explanationMentions(totals, "undeclared") {
		t.Fatalf("合计后派生的解释：%v", totals.Explanation())
	}
	if !explanationMentions(each, "PER_PIECE_THEN_SUM") || !explanationMentions(each, "ROUND_EACH_THEN_SUM") {
		t.Fatalf("逐件进位的解释：%v", each.Explanation())
	}
	if !explanationMentions(once, "SUM_THEN_ROUND_ONCE") {
		t.Fatalf("求和后进位的解释：%v", once.Explanation())
	}
	if base.CanonicalizationVersion() != "PPC-5" || totals.PlanCanonicalizationVersion() != "PPC-8" {
		t.Fatalf("版本 %s / %s，未声明应仍是 PPC-5，声明了的写 PPC-8", base.CanonicalizationVersion(), totals.PlanCanonicalizationVersion())
	}
	if base.ContentDigest() == declareShipmentWeight(t, base, domain.ShipmentWeightTotalsThenDerive, "").ContentDigest() {
		t.Fatal("声明与否的两张卡摘要相同")
	}
	if undeclared.SemanticDigest() == totals.SemanticDigest() {
		t.Fatal("旧卡与声明了合计后派生的卡评出了同一个语义摘要")
	}
}

// Covers: ADR-0364 决定三——卡要体积重而缺任一件的尺寸时评价待判断，不退回成员清单上的合计体积重。
func TestPerPieceVolumetricWeightStaysPendingWhenAnyPieceLacksDimensions(t *testing.T) {
	base := segmentedShipmentPlan(t, domain.PricingWeightMax)
	plan := declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightRoundEachThenSum)
	box := dimensions(t, "50", "40", "30", domain.LengthUnitCentimeter)
	volumetric := "50"
	missing := shipmentPieces(t, "8", &volumetric, piece(t, "pkg-a", "4", &box), piece(t, "pkg-b", "4", nil))
	evaluation := evaluate(t, "eval-agg-missing-side", plan, aggregateInput(t, shipmentSubject(t, "SR-AGG"), missing))
	if evaluation.Status() != domain.EvaluationPending || issueCodes(evaluation)[0] != "DIMENSIONS_REQUIRED" {
		t.Fatalf("缺一件尺寸：%s %v", evaluation.Status(), issueCodes(evaluation))
	}
	if _, ok := evaluation.Total(); ok {
		t.Fatal("缺尺寸却给出了合计，等于退回了合计体积重")
	}

	both := shipmentPieces(t, "8", &volumetric, piece(t, "pkg-a", "4", &box), piece(t, "pkg-b", "4", &box))
	priced := evaluate(t, "eval-agg-both-sides", plan, aggregateInput(t, shipmentSubject(t, "SR-AGG"), both))
	if priced.Status() != domain.EvaluationCompleted {
		t.Fatalf("两件都有尺寸应完成：%s %v", priced.Status(), issueCodes(priced))
	}
}

// Covers: ADR-0364 决定三——逐件求和没有逐件实重时待判断，不把合计实重拆开用。
func TestPerPieceSummingStaysPendingWithoutPieceWeights(t *testing.T) {
	plan := declareShipmentWeight(t, segmentedShipmentPlan(t, domain.PricingWeightActualOnly), domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightSumThenRoundOnce)
	evaluation := evaluate(t, "eval-agg-no-pieces", plan, aggregateInput(t, shipmentSubject(t, "SR-AGG"), members(t, "20", nil, "pkg-a", "pkg-b")))
	if evaluation.Status() != domain.EvaluationPending || issueCodes(evaluation)[0] != "PIECE_MEASUREMENTS_REQUIRED" {
		t.Fatalf("没有逐件实重：%s %v", evaluation.Status(), issueCodes(evaluation))
	}
}

// Covers: ADR-0364 决定三——条件最低计价重量作用在和上；判定条件读到的自身计价重量是进位之后、抬高之前的值。
// 两件各 1.2 千克、0.5 千克向上进位：逐件进位后是 3，求和后进位一次是 2.5。下限 4 千克把方案级计价重量抬到 4，不是每件抬到 4 再相加得 8。
// 读「自身计价重量大于 2.6」的条款只在逐件进位那一张上命中，尽管两张卡抬高之后的计价重量都是 4。
func TestConditionalMinimumRaisesTheSumAndConditionsReadTheRoundedSumBeforeTheRaise(t *testing.T) {
	chargeable := ownWeightTrigger(t, domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, "2.6", domain.WeightUnitKilogram)
	overweight := weightSurcharge(t, "over-chargeable", "OVERWEIGHT", chargeable, "7")
	floorTrigger := ownWeightTrigger(t, domain.FeatureActualWeight, domain.ComparisonGreaterThan, "0", domain.WeightUnitKilogram)
	floor, err := domain.NewConditionalMinimumWeight("sum-floor", floorTrigger, weight(t, "4", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("下限：%v", err)
	}
	carrier := weightSurcharge(t, "always", "HANDLING", floorTrigger, "1")
	carrier, err = carrier.WithConditionalMinimumWeight(floor)
	if err != nil {
		t.Fatalf("挂下限：%v", err)
	}
	base := mustOwnWeightPlan(t, ownWeightSpec{
		method: domain.PricingWeightActualOnly, aggregation: domain.AggregationPerShipment, structures: structuresOf(t, overweight, carrier),
	})
	pieces := shipmentPieces(t, "2.4", nil, piece(t, "pkg-a", "1.2", nil), piece(t, "pkg-b", "1.2", nil))
	input := aggregateInput(t, shipmentSubject(t, "SR-MIN"), pieces)
	each := declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightRoundEachThenSum)
	once := declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightSumThenRoundOnce)

	eachEval := evaluate(t, "eval-min-each", each, input)
	onceEval := evaluate(t, "eval-min-once", once, input)
	if got := roundedPricingWeight(t, eachEval); got != "4" {
		t.Fatalf("逐件进位后抬高 %s，应是方案级的 4，不是 8", got)
	}
	if got := roundedPricingWeight(t, onceEval); got != "4" {
		t.Fatalf("求和后进位再抬高 %s，应是 4", got)
	}
	if got := ownWeightTotal(t, eachEval); got != "18" {
		t.Fatalf("逐件进位的合计 %s，条件应看见抬高前的 3 从而收超重 7（10+1+7）", got)
	}
	if got := ownWeightTotal(t, onceEval); got != "11" {
		t.Fatalf("求和后进位的合计 %s，条件应看见抬高前的 2.5，不收超重（10+1）", got)
	}
}

// Covers: ADR-0364 决定五——新键写 PPC-8，旧卡快照不带这一键；记错版本的读回门拒。主单级与逐包裹声明不了。
func TestShipmentWeightAggregationCanonicalizationAndWhoMayDeclareIt(t *testing.T) {
	base := segmentedShipmentPlan(t, domain.PricingWeightActualOnly)
	if _, err := base.WithShipmentWeightAggregation(domain.ShipmentWeightPerPieceThenSum, ""); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("逐件求和没给进位点：%v", err)
	}
	if _, err := base.WithShipmentWeightAggregation(domain.ShipmentWeightTotalsThenDerive, domain.ShipmentWeightSumThenRoundOnce); !errors.Is(err, domain.ErrInvalidPricingPlan) {
		t.Fatalf("合计后派生却给了进位点：%v", err)
	}
	master := mustAggregatePlan(t, domain.AggregationPerMasterDocument, domain.PricingWeightActualOnly)
	if _, err := master.WithShipmentWeightAggregation(domain.ShipmentWeightTotalsThenDerive, ""); !errors.Is(err, domain.ErrShipmentWeightAggregationOnlyOnShipment) {
		t.Fatalf("主单级：%v", err)
	}
	perPackage := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly})
	if _, err := perPackage.WithShipmentWeightAggregation(domain.ShipmentWeightTotalsThenDerive, ""); !errors.Is(err, domain.ErrShipmentWeightAggregationOnlyOnShipment) {
		t.Fatalf("逐包裹：%v", err)
	}

	raw, rebuilt := planSnapshotRoundTrip(t, base)
	if strings.Contains(string(raw), "shipmentWeightAggregation") || rebuilt.CanonicalizationVersion() != "PPC-5" || rebuilt.ContentDigest() != base.ContentDigest() {
		t.Fatalf("未声明的卡快照不该带新键：%s", raw)
	}
	declared := declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightRoundEachThenSum)
	declaredRaw, declaredAgain := planSnapshotRoundTrip(t, declared)
	if declaredAgain.CanonicalizationVersion() != "PPC-8" || declaredAgain.ContentDigest() != declared.ContentDigest() {
		t.Fatalf("往返后 %s %s", declaredAgain.CanonicalizationVersion(), declaredAgain.ContentDigest())
	}
	method, ok := declaredAgain.ShipmentWeightAggregation()
	point, hasPoint := declaredAgain.ShipmentWeightRoundingPoint()
	if !ok || method != domain.ShipmentWeightPerPieceThenSum || !hasPoint || point != domain.ShipmentWeightRoundEachThenSum {
		t.Fatalf("读回的聚合方式 %s %v，进位点 %s %v", method, ok, point, hasPoint)
	}
	if !strings.Contains(string(declaredRaw), `"shipmentWeightAggregation":"PER_PIECE_THEN_SUM"`) || !strings.Contains(string(declaredRaw), `"shipmentWeightRoundingPoint":"ROUND_EACH_THEN_SUM"`) {
		t.Fatalf("快照没写下两格：%s", declaredRaw)
	}
	for name, mutate := range map[string]func(map[string]any){
		"带了键却记着 PPC-7": func(plan map[string]any) { plan["canonicalization"] = "PPC-7" },
		"记着 PPC-8 却去掉键": func(plan map[string]any) {
			delete(plan, "shipmentWeightAggregation")
			delete(plan, "shipmentWeightRoundingPoint")
		},
		"逐件求和却没有进位点": func(plan map[string]any) { delete(plan, "shipmentWeightRoundingPoint") },
	} {
		if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, declaredRaw, func(document map[string]any) {
			mutate(document["plan"].(map[string]any))
		})); err == nil {
			t.Fatalf("%s：读回门放行了", name)
		}
	}
	if _, err := domain.RehydratePriceCardRegistration(tamperedDocument(t, declaredRaw, func(document map[string]any) {
		document["plan"].(map[string]any)["canonicalization"] = "PPC-9"
	})); !errors.Is(err, domain.ErrCanonicalizationVersionUnsupported) {
		t.Fatalf("PPC-9：%v", err)
	}

	sell := mustOwnWeightPlan(t, ownWeightSpec{method: domain.PricingWeightActualOnly, aggregation: domain.AggregationPerShipment})
	both, err := declareShipmentWeight(t, sell, domain.ShipmentWeightTotalsThenDerive, "").WithServiceProducts(product(t, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("售价卡同时声明产品与聚合方式：%v", err)
	}
	if both.CanonicalizationVersion() != "PPC-8" || len(both.ServiceProducts()) != 1 {
		t.Fatalf("同时带产品的卡写 %s，应是 PPC-8", both.CanonicalizationVersion())
	}
}

func poundActualPlan(t testing.TB) domain.PricingPlanVersion {
	t.Helper()
	currency := usd(t)
	entry, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "entry-ship-lb"), "Z1",
		weight(t, "0", domain.WeightUnitPound), weight(t, "100", domain.WeightUnitPound), money(t, "10", currency))
	if err != nil {
		t.Fatalf("价表行：%v", err)
	}
	table, err := domain.NewRateTableVersion(versionReference(t, domain.ArtifactRateTable, "table-ship-lb", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitPound, effectivePeriod(t), []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.5", domain.WeightUnitPound))
	if err != nil {
		t.Fatalf("进位：%v", err)
	}
	policy, err := domain.NewPricingWeightPolicy(versionReference(t, domain.ArtifactWeightPolicy, "weight-ship-lb", "v1"),
		domain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("重量策略：%v", err)
	}
	plan, err := domain.NewAggregatePricingPlanVersion(domain.AggregationPerShipment,
		versionReference(t, domain.ArtifactPricingPlan, "plan-ship-lb", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"), effectivePeriod(t), table, policy, nil, domain.PricingPlanStructures{})
	if err != nil {
		t.Fatalf("方案：%v", err)
	}
	return plan
}

func conversionText(step domain.UnitConversionStep) string {
	originals := make([]string, 0, len(step.Original()))
	for _, value := range step.Original() {
		originals = append(originals, value.String())
	}
	converted := make([]string, 0, len(step.Converted()))
	for _, value := range step.Converted() {
		converted = append(converted, value.String())
	}
	return fmt.Sprintf("%s %s %s -> %s %s factor %s exact %v places %d",
		step.Quantity(), strings.Join(originals, " x "), step.FromUnit(),
		strings.Join(converted, " x "), step.ToUnit(), step.Factor(), step.Exact(), step.Places())
}

func sameConversions(t testing.TB, left, right []domain.UnitConversionStep) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("换算步骤 %d 条对 %d 条", len(left), len(right))
	}
	for index := range left {
		if conversionText(left[index]) != conversionText(right[index]) {
			t.Fatalf("换算步骤 %d\n%s\n%s", index, conversionText(left[index]), conversionText(right[index]))
		}
	}
}

func samePricingWeight(t testing.TB, left, right domain.PricingEvaluation) {
	t.Helper()
	first, ok := left.PricingWeight()
	second, other := right.PricingWeight()
	if !ok || !other || first.Explanation() != second.Explanation() ||
		first.ActualWeight().Value().String() != second.ActualWeight().Value().String() ||
		first.ActualWeight().Unit() != second.ActualWeight().Unit() ||
		first.RawWeight().Value().String() != second.RawWeight().Value().String() ||
		first.RoundedWeight().Value().String() != second.RoundedWeight().Value().String() {
		t.Fatalf("计价重量\n%s\n%s", first.Explanation(), second.Explanation())
	}
}

// Covers: 逐件求和的件单位与卡不同时，评价按目录逐件换算再进原来的派生与进位。
// 合计实重另报 9 千克，不是两件之和，换算结果对得上逐件而不是合计。两件原值不同，截断位数可以不同；冻结重放按原值对上那一条。
func TestPerPieceSumConvertsEachPieceOnTheCatalogueAndReplaysByOriginalValue(t *testing.T) {
	plan := declareShipmentWeight(t, poundActualPlan(t), domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightSumThenRoundOnce)
	input := aggregateInput(t, shipmentSubject(t, "SR-LB"), manifestIn(t, "9", domain.WeightUnitKilogram, nil,
		pieceIn(t, "pkg-a", "1", domain.WeightUnitKilogram, nil),
		pieceIn(t, "pkg-b", "1.1", domain.WeightUnitKilogram, nil)))
	converted := evaluate(t, "eval-piece-convert", plan, input)
	if converted.Status() != domain.EvaluationCompleted {
		t.Fatalf("逐件换算：%s %v", converted.Status(), issueCodes(converted))
	}
	steps := converted.UnitConversions()
	factor, err := refdata.Convert(big.NewRat(1, 1), "KG", "LB")
	if err != nil {
		t.Fatalf("目录：%v", err)
	}
	if len(steps) != 2 || steps[0].Factor() != factor.String() || steps[1].Factor() != factor.String() ||
		steps[0].Original()[0].String() != "1" || steps[1].Original()[0].String() != "1.1" ||
		steps[0].Places() == steps[1].Places() || steps[0].Exact() || steps[1].Exact() {
		texts := make([]string, 0, len(steps))
		for _, step := range steps {
			texts = append(texts, conversionText(step))
		}
		t.Fatalf("换算步骤 %s，目录系数 %s", strings.Join(texts, " | "), factor)
	}
	already := aggregateInput(t, shipmentSubject(t, "SR-LB"), manifestIn(t, "9", domain.WeightUnitPound, nil,
		pieceIn(t, "pkg-a", steps[0].Converted()[0].String(), domain.WeightUnitPound, nil),
		pieceIn(t, "pkg-b", steps[1].Converted()[0].String(), domain.WeightUnitPound, nil)))
	direct := evaluate(t, "eval-piece-already", plan, already)
	if direct.Status() != domain.EvaluationCompleted {
		t.Fatalf("先换成卡的单位：%s %v", direct.Status(), issueCodes(direct))
	}
	samePricingWeight(t, converted, direct)
	if len(direct.UnitConversions()) != 0 {
		t.Fatalf("已经是卡的单位仍有换算：%s", conversionText(direct.UnitConversions()[0]))
	}

	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-piece-replay"), converted, plan, domain.EvidenceSynthetic)
	if err != nil || replay.Status() != domain.EvaluationCompleted {
		t.Fatalf("冻结重放：%v %s %v", err, replay.Status(), issueCodes(replay))
	}
	samePricingWeight(t, converted, replay)
	sameConversions(t, converted.UnitConversions(), replay.UnitConversions())

	raw, err := domain.MarshalEvaluationSnapshot(converted)
	if err != nil {
		t.Fatalf("折快照：%v", err)
	}
	rebuilt, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	again, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-piece-replay-2"), rebuilt, plan, domain.EvidenceSynthetic)
	if err != nil || again.Status() != domain.EvaluationCompleted {
		t.Fatalf("读回后重放：%v %s %v", err, again.Status(), issueCodes(again))
	}
	samePricingWeight(t, converted, again)
	sameConversions(t, converted.UnitConversions(), again.UnitConversions())
}

// Covers: 合计后派生与单位已经一致的逐件求和都不产生换算步骤。
func TestMatchingUnitsLeaveShipmentConversionStepsEmpty(t *testing.T) {
	base := segmentedShipmentPlan(t, domain.PricingWeightActualOnly)
	input := aggregateInput(t, shipmentSubject(t, "SR-AGG"), shipmentPieces(t, "20", nil, piece(t, "pkg-a", "10.3", nil), piece(t, "pkg-b", "10.7", nil)))
	plans := []domain.PricingPlanVersion{
		base,
		declareShipmentWeight(t, base, domain.ShipmentWeightTotalsThenDerive, ""),
		declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightSumThenRoundOnce),
	}
	for index, plan := range plans {
		evaluation := evaluate(t, fmt.Sprintf("eval-agg-same-unit-%d", index), plan, input)
		if evaluation.Status() != domain.EvaluationCompleted || len(evaluation.UnitConversions()) != 0 || explanationMentions(evaluation, "unit conversion") {
			t.Fatalf("单位一致 %d：%s 换算 %d %v", index, evaluation.Status(), len(evaluation.UnitConversions()), evaluation.Explanation())
		}
	}
}

// Covers: 卡要体积重而件的单位与卡不同时，缺任一件尺寸仍待判断，不退回合计体积重。
func TestPerPieceVolumetricStaysPendingWhenAnotherUnitLacksDimensions(t *testing.T) {
	plan := declareShipmentWeight(t, segmentedShipmentPlan(t, domain.PricingWeightMax), domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightRoundEachThenSum)
	box := dimensions(t, "10", "10", "10", domain.LengthUnitInch)
	volumetric := weight(t, "50", domain.WeightUnitPound)
	manifest := manifestIn(t, "20", domain.WeightUnitPound, &volumetric,
		pieceIn(t, "pkg-a", "10", domain.WeightUnitPound, &box),
		pieceIn(t, "pkg-b", "10", domain.WeightUnitPound, nil))
	evaluation := evaluate(t, "eval-agg-lb-missing-side", plan, aggregateInput(t, shipmentSubject(t, "SR-LB"), manifest))
	if evaluation.Status() != domain.EvaluationPending || issueCodes(evaluation)[0] != "DIMENSIONS_REQUIRED" {
		t.Fatalf("缺一件尺寸：%s %v", evaluation.Status(), issueCodes(evaluation))
	}
	if _, ok := evaluation.Total(); ok {
		t.Fatal("缺尺寸却给出了合计，等于退回了合计体积重")
	}
}

func twoPieceBox(t testing.TB, unit domain.LengthUnit) domain.MemberManifest {
	t.Helper()
	small := dimensions(t, "30", "20", "10", unit)
	large := dimensions(t, "50", "40", "30", unit)
	return shipmentPieces(t, "3", nil, piece(t, "pkg-a", "2", &small), piece(t, "pkg-b", "1", &large))
}

func volumetricOf(t testing.TB, evaluation domain.PricingEvaluation) string {
	t.Helper()
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s，问题 %v", evaluation.Status(), issueCodes(evaluation))
	}
	pricingWeight, ok := evaluation.PricingWeight()
	if !ok {
		t.Fatal("没有计价重量")
	}
	volumetric, derived := pricingWeight.VolumetricWeight()
	if !derived {
		t.Fatal("没有体积重")
	}
	return volumetric.Value().String()
}

// Covers: 未声明聚合与合计后派生的 MAX 票级卡，没声明合计体积重但各件尺寸齐全时，按总体积一次除体积系数。
// 30×20×10 与 50×40×30 立方厘米合计 66000，除以 5000 是 13.2 千克；逐件取较大者再相加是 14，两条路必须分开。
// 缺任一件尺寸仍待判断。调用方声明了合计体积重时仍用声明值，不改用各件尺寸。
func TestTotalsThenDeriveUsesTheSummedPieceVolumeWhenTheTotalVolumetricWeightIsAbsent(t *testing.T) {
	base := segmentedShipmentPlan(t, domain.PricingWeightMax)
	input := aggregateInput(t, shipmentSubject(t, "SR-VOL"), twoPieceBox(t, domain.LengthUnitCentimeter))
	totals := declareShipmentWeight(t, base, domain.ShipmentWeightTotalsThenDerive, "")

	undeclared := evaluate(t, "eval-vol-old", base, input)
	derived := evaluate(t, "eval-vol-totals", totals, input)
	if got := volumetricOf(t, undeclared); got != "13.2" {
		t.Fatalf("未声明票级卡体积重 %s，应是 66000/5000 = 13.2", got)
	}
	if got := volumetricOf(t, derived); got != "13.2" {
		t.Fatalf("合计后派生体积极重 %s，应是 13.2", got)
	}
	if base.CanonicalizationVersion() != "PPC-5" || totals.CanonicalizationVersion() != "PPC-8" {
		t.Fatalf("版本 %s / %s", base.CanonicalizationVersion(), totals.CanonicalizationVersion())
	}
	each := evaluate(t, "eval-vol-each", declareShipmentWeight(t, base, domain.ShipmentWeightPerPieceThenSum, domain.ShipmentWeightRoundEachThenSum), input)
	if got := roundedPricingWeight(t, each); got != "14" {
		t.Fatalf("逐件求和 %s，应是 2 与 12 之和 14，不能与合计体积重 13.2 混成同一个数", got)
	}
	if len(derived.UnitConversions()) != 0 {
		t.Fatalf("尺寸单位与系数一致，不该记换算：%v", derived.UnitConversions())
	}

	small := dimensions(t, "30", "20", "10", domain.LengthUnitCentimeter)
	missing := aggregateInput(t, shipmentSubject(t, "SR-VOL"), shipmentPieces(t, "3", nil, piece(t, "pkg-a", "2", &small), piece(t, "pkg-b", "1", nil)))
	pending := evaluate(t, "eval-vol-missing", totals, missing)
	if pending.Status() != domain.EvaluationPending || issueCodes(pending)[0] != "DIMENSIONS_REQUIRED" {
		t.Fatalf("缺一件尺寸：%s %v", pending.Status(), issueCodes(pending))
	}

	declaredTotal := "9"
	declared := aggregateInput(t, shipmentSubject(t, "SR-VOL"), shipmentPieces(t, "3", &declaredTotal, piece(t, "pkg-a", "2", &small), piece(t, "pkg-b", "1", dimensionsPtr(t, "50", "40", "30"))))
	kept := evaluate(t, "eval-vol-declared", totals, declared)
	if got := volumetricOf(t, kept); got != "9" {
		t.Fatalf("声明了合计体积重却用了各件尺寸：%s", got)
	}
}

func dimensionsPtr(t testing.TB, length, width, height string) *domain.Dimensions {
	t.Helper()
	sides := dimensions(t, length, width, height, domain.LengthUnitCentimeter)
	return &sides
}

// Covers: 尺寸是英寸、体积系数按厘米时，各件三边换算后把精确体积加总再进位一次，每件各记一条尺寸换算。
// 快照读回与冻结重放的换算记录、摘要与原评价一致。不另记一条调用方没声明的合计体积重换算。
func TestPieceDimensionsInInchesDeriveOneShipmentVolumetricWeight(t *testing.T) {
	base := segmentedShipmentPlan(t, domain.PricingWeightMax)
	plan := declareShipmentWeight(t, base, domain.ShipmentWeightTotalsThenDerive, "")
	first := dimensions(t, "10", "10", "10", domain.LengthUnitInch)
	second := dimensions(t, "5", "5", "5", domain.LengthUnitInch)
	input := aggregateInput(t, shipmentSubject(t, "SR-IN"), shipmentPieces(t, "3", nil, piece(t, "pkg-a", "2", &first), piece(t, "pkg-b", "1", &second)))
	evaluation := evaluate(t, "eval-vol-in", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("状态 %s %v", evaluation.Status(), issueCodes(evaluation))
	}
	steps := evaluation.UnitConversions()
	if len(steps) != 2 || steps[0].Quantity() != domain.ConvertedDimensions || steps[1].Quantity() != domain.ConvertedDimensions {
		t.Fatalf("应各记一件尺寸换算：%v", steps)
	}
	if steps[0].FromUnit() != "IN" || steps[0].ToUnit() != "CM" || steps[0].Factor() != "2.54" || !steps[0].Exact() ||
		steps[1].Factor() != "2.54" || !steps[1].Exact() {
		t.Fatalf("换算记录：%+v %+v", steps[0], steps[1])
	}
	for _, step := range steps {
		if step.Quantity() == domain.ConvertedTotalVolumetricWeight {
			t.Fatalf("由系数派生的体积重不该记合计体积重换算：%+v", step)
		}
	}
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("快照：%v", err)
	}
	restored, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if restored.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatal("快照读回的摘要与原评价不同")
	}
	sameConversions(t, evaluation.UnitConversions(), restored.UnitConversions())
	replayed, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-vol-in-2"), restored, plan, domain.EvidenceSynthetic)
	if err != nil || replayed.Status() != domain.EvaluationCompleted || replayed.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatalf("冻结重放：%v %s %v", err, replayed.Status(), issueCodes(replayed))
	}
	sameConversions(t, evaluation.UnitConversions(), replayed.UnitConversions())
	if joinDecimalsForTest(evaluation.Explanation()) != joinDecimalsForTest(replayed.Explanation()) {
		t.Fatalf("重放解释与原评价不同：\n%v\n%v", evaluation.Explanation(), replayed.Explanation())
	}
}

func joinDecimalsForTest(lines []string) string {
	return strings.Join(lines, "\n")
}
