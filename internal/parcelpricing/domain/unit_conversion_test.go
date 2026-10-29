package domain_test

import (
	"errors"
	"math/big"
	"math/rand"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件的用例对 ADR-0346：评价里输入实重 / 尺寸的单位与方案声明的单位不同时，按产品内置目录的精确系数换算，
// 而不是答单位不一致；单位一致的路径一个字节不动。

type volumetricSpec struct {
	divisor    string
	length     domain.LengthUnit
	mode       domain.RoundingMode
	increment  string
	factorUnit domain.WeightUnit
}

// tieredPlan 造一张分档价表的合成价卡：[0,1)=5、[1,2)=8、[2,3)=12、[3,1000)=20，单位随 unit。
// 档位不同价，才能从金额上看出换算把包裹放进了哪一档。
func tieredPlan(t testing.TB, suffix string, unit domain.WeightUnit, mode domain.RoundingMode, increment string, factor *volumetricSpec) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	bands := [][3]string{{"0", "1", "5"}, {"1", "2", "8"}, {"2", "3", "12"}, {"3", "1000", "20"}}
	entries := make([]domain.RateEntry, 0, len(bands))
	for index, band := range bands {
		entry, err := domain.NewRateEntry(
			mustValue(t, domain.NewRateEntryID, "entry-"+suffix+"-"+string(rune('a'+index))),
			"Z1", weight(t, band[0], unit), weight(t, band[1], unit), money(t, band[2], currency),
		)
		if err != nil {
			t.Fatalf("rate entry: %v", err)
		}
		entries = append(entries, entry)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-"+suffix, "v1"),
		domain.RateTableFamilyWeightZone, currency, unit, effectivePeriod(t), entries,
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(mode, weight(t, increment, unit))
	if err != nil {
		t.Fatalf("rounding policy: %v", err)
	}
	method := domain.PricingWeightActualOnly
	var volumetric *domain.VolumetricFactor
	if factor != nil {
		method = domain.PricingWeightMax
		factorRounding, roundingErr := domain.NewWeightRoundingPolicy(factor.mode, weight(t, factor.increment, unit))
		if roundingErr != nil {
			t.Fatalf("volumetric rounding: %v", roundingErr)
		}
		declared, factorErr := domain.NewVolumetricFactor(decimal(t, factor.divisor), factor.length, factorRounding)
		if factorErr != nil {
			t.Fatalf("volumetric factor: %v", factorErr)
		}
		volumetric = &declared
	}
	policy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-"+suffix, "v1"), method, rounding, volumetric,
	)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-"+suffix, "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t), table, policy, nil, domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("pricing plan: %v", err)
	}
	return plan
}

func unitInput(t testing.TB, actual string, unit domain.WeightUnit, sides *domain.Dimensions) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"), "Z1", weight(t, actual, unit), sides,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("pricing input: %v", err)
	}
	return input
}

func totalOf(t testing.TB, evaluation domain.PricingEvaluation) string {
	t.Helper()
	total, ok := evaluation.Total()
	if !ok {
		t.Fatalf("evaluation %s has no total: status %s issues %v", evaluation.ID(), evaluation.Status(), evaluation.Issues())
	}
	return total.Amount().String()
}

func explanationHas(evaluation domain.PricingEvaluation, fragment string) bool {
	for _, line := range evaluation.Explanation() {
		if strings.Contains(line, fragment) {
			return true
		}
	}
	return false
}

func ratOf(t testing.TB, text string) *big.Rat {
	t.Helper()
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		t.Fatalf("not a rational: %q", text)
	}
	return value
}

// Covers: ADR-0346 决定一——价表按 KG 查、输入实重按 LB 给：评价把 2.5 LB 精确换成 1.133980925 KG 再进位查表，
// 而不是答 WEIGHT_UNIT_MISMATCH；输入快照保留原值原单位，换算的原值、换算值、系数留在评价上与解释里。
func TestAPoundWeightIsConvertedExactlyToThePlansKilogramTableUnit(t *testing.T) {
	plan := tieredPlan(t, "lb-to-kg", domain.WeightUnitKilogram, domain.RoundingCeiling, "0.5", nil)
	input := unitInput(t, "2.5", domain.WeightUnitPound, nil)

	evaluation := evaluate(t, "eval-lb-to-kg", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %v", evaluation.Status(), evaluation.Issues())
	}
	// 1.133980925 KG 向上进到 0.5 KG 的整数倍是 1.5 KG，落在 [1,2) 档。
	if total := totalOf(t, evaluation); total != "8" {
		t.Fatalf("total = %s, want 8", total)
	}
	pricingWeight, _ := evaluation.PricingWeight()
	if pricingWeight.ActualWeight().Value().String() != "1.133980925" || pricingWeight.ActualWeight().Unit() != domain.WeightUnitKilogram ||
		pricingWeight.RoundedWeight().Value().String() != "1.5" {
		t.Fatalf("pricing weight = actual %s %s, rounded %s", pricingWeight.ActualWeight().Value(), pricingWeight.ActualWeight().Unit(), pricingWeight.RoundedWeight().Value())
	}
	if kept := evaluation.Input().ActualWeight(); kept.Value().String() != "2.5" || kept.Unit() != domain.WeightUnitPound {
		t.Fatalf("评价输入应保留原值与原单位，得到 %s %s", kept.Value(), kept.Unit())
	}

	steps := evaluation.UnitConversions()
	if len(steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(steps))
	}
	step := steps[0]
	if step.Quantity() != domain.ConvertedActualWeight || step.FromUnit() != "LB" || step.ToUnit() != "KG" || step.Factor() != "0.45359237" ||
		!step.Exact() || step.Original()[0].String() != "2.5" || step.Converted()[0].String() != "1.133980925" {
		t.Fatalf("step = %+v", step)
	}
	if !explanationHas(evaluation, "unit conversion ACTUAL_WEIGHT: 2.5 LB -> 1.133980925 KG at factor 0.45359237 (exact)") {
		t.Fatalf("解释里没有原值原单位、换算值目标单位与系数：%v", evaluation.Explanation())
	}
}

// Covers: ADR-0346 决定一——克折千克：2500 G 是 2.5 KG，正好整进位，落在 [2,3) 档。
func TestAGramWeightIsConvertedToTheKilogramTableUnit(t *testing.T) {
	plan := tieredPlan(t, "g-to-kg", domain.WeightUnitKilogram, domain.RoundingCeiling, "0.5", nil)
	evaluation := evaluate(t, "eval-g-to-kg", plan, unitInput(t, "2500", domain.WeightUnitGram, nil))
	if total := totalOf(t, evaluation); total != "12" {
		t.Fatalf("total = %s, want 12", total)
	}
	steps := evaluation.UnitConversions()
	if len(steps) != 1 || steps[0].Factor() != "0.001" || steps[0].Converted()[0].String() != "2.5" {
		t.Fatalf("steps = %+v", steps)
	}
}

// Covers: ADR-0346 决定一——体积重的尺寸以英寸给、卡的体积系数声明按厘米：三边按 2.54 精确换成 25.4 cm，
// 体积重与直接按 25.4 cm 给尺寸的评价逐字相同。
func TestInchSidesAreConvertedToTheVolumetricFactorsCentimetres(t *testing.T) {
	factor := &volumetricSpec{divisor: "5000", length: domain.LengthUnitCentimeter, mode: domain.RoundingCeiling, increment: "0.1"}
	plan := tieredPlan(t, "in-to-cm", domain.WeightUnitKilogram, domain.RoundingCeiling, "0.5", factor)

	inches := dimensions(t, "10", "10", "10", domain.LengthUnitInch)
	converted := evaluate(t, "eval-in-to-cm", plan, unitInput(t, "1", domain.WeightUnitKilogram, &inches))
	centimetres := dimensions(t, "25.4", "25.4", "25.4", domain.LengthUnitCentimeter)
	direct := evaluate(t, "eval-cm-direct", plan, unitInput(t, "1", domain.WeightUnitKilogram, &centimetres))

	// 25.4³ = 16387.064 cm³，除 5000 得 3.2774128，向上进到 0.1 是 3.3 KG；实重 1 KG 小，MAX 取 3.3，再进到 3.5 KG，落 [3,1000)。
	if totalOf(t, converted) != "20" || totalOf(t, direct) != "20" {
		t.Fatalf("totals = %s / %s, want 20", totalOf(t, converted), totalOf(t, direct))
	}
	convertedWeight, _ := converted.PricingWeight()
	directWeight, _ := direct.PricingWeight()
	volumetric, _ := convertedWeight.VolumetricWeight()
	if volumetric.Value().String() != "3.3" || convertedWeight.RoundedWeight().Value().String() != "3.5" {
		t.Fatalf("volumetric %s rounded %s", volumetric.Value(), convertedWeight.RoundedWeight().Value())
	}
	if convertedWeight.Explanation() != directWeight.Explanation() {
		t.Fatalf("换算后的计价重量解释与直接给厘米不同：\n%s\n%s", convertedWeight.Explanation(), directWeight.Explanation())
	}
	steps := converted.UnitConversions()
	if len(steps) != 1 || steps[0].Quantity() != domain.ConvertedDimensions || steps[0].Factor() != "2.54" || !steps[0].Exact() ||
		joinForTest(steps[0].Original()) != "10 10 10" || joinForTest(steps[0].Converted()) != "25.4 25.4 25.4" {
		t.Fatalf("steps = %+v", steps)
	}
	if !explanationHas(converted, "unit conversion DIMENSIONS: 10 x 10 x 10 IN -> 25.4 x 25.4 x 25.4 CM at factor 2.54 (exact)") {
		t.Fatalf("解释：%v", converted.Explanation())
	}
}

func joinForTest(values []domain.Decimal) string {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, value.String())
	}
	return strings.Join(texts, " ")
}

// Covers: ADR-0346 决定四——1 KG 折多少磅是无限小数：2.2046226218487757…。换算保留 11 位（既约分母 45359237 有 8 位，
// 进位单位 0.5 的中点占 2 位，再加 1），截断如实写进记录与解释；2.20462262184 向上进到 0.5 LB 是 2.5 LB，
// 与不截断时逐一相同，落 [2,3) 档。
func TestAKilogramWeightToAPoundTableKeepsEnoughPlacesAndSaysItWasTruncated(t *testing.T) {
	plan := tieredPlan(t, "kg-to-lb", domain.WeightUnitPound, domain.RoundingCeiling, "0.5", nil)
	evaluation := evaluate(t, "eval-kg-to-lb", plan, unitInput(t, "1", domain.WeightUnitKilogram, nil))
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s, issues = %v", evaluation.Status(), evaluation.Issues())
	}
	if total := totalOf(t, evaluation); total != "12" {
		t.Fatalf("total = %s, want 12", total)
	}
	pricingWeight, _ := evaluation.PricingWeight()
	if pricingWeight.RoundedWeight().Value().String() != "2.5" {
		t.Fatalf("rounded = %s, want 2.5", pricingWeight.RoundedWeight().Value())
	}
	step := evaluation.UnitConversions()[0]
	if step.Factor() != "100000000/45359237" || step.Exact() || step.Places() != 11 || step.Converted()[0].String() != "2.20462262184" {
		t.Fatalf("step = factor %s exact %v places %d converted %s", step.Factor(), step.Exact(), step.Places(), step.Converted()[0])
	}
	if !explanationHas(evaluation, "1 KG -> 2.20462262184 LB at factor 100000000/45359237 (not a finite decimal: truncated toward zero at 11 decimal places") {
		t.Fatalf("解释没有如实写截断：%v", evaluation.Explanation())
	}
}

// Covers: ADR-0346 决定四——截断不改变取整结果的论证（guardPlaces 注释）：用独立的有理数参考实现，在 KG→LB
// 的临界点（k × 0.226796185 KG 恰是 k × 0.5 LB）两侧各取 1e-12、1e-9、1e-6 的偏差，加一批随机重量，
// CEILING 与 HALF_UP 下评价算出的进位后重量都逐一等于对精确商取整的结果。
func TestTruncatedConversionNeverChangesTheRoundedWeight(t *testing.T) {
	lbPerKg := new(big.Rat).Inv(ratOf(t, "0.45359237"))
	cases := []struct {
		mode      domain.RoundingMode
		increment string
	}{{domain.RoundingCeiling, "0.5"}, {domain.RoundingHalfUp, "0.1"}, {domain.RoundingCeiling, "0.001"}}
	for _, c := range cases {
		plan := tieredPlan(t, "ref-"+string(c.mode)+c.increment, domain.WeightUnitPound, c.mode, c.increment, nil)
		increment := ratOf(t, c.increment)
		var weights []string
		for k := 1; k <= 40; k++ {
			boundary := new(big.Rat).Mul(big.NewRat(int64(k), 1), ratOf(t, "0.226796185"))
			for _, delta := range []string{"0", "0.000000000001", "0.000000001", "0.000001"} {
				for _, sign := range []int{1, -1} {
					shifted := new(big.Rat).Add(boundary, new(big.Rat).Mul(big.NewRat(int64(sign), 1), ratOf(t, delta)))
					if shifted.Sign() > 0 {
						weights = append(weights, shifted.FloatString(15))
					}
				}
			}
		}
		generator := rand.New(rand.NewSource(7))
		for range 300 {
			weights = append(weights, new(big.Rat).SetFrac64(generator.Int63n(30000)+1, 1000).FloatString(3))
		}
		for _, text := range weights {
			text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
			evaluation := evaluate(t, "eval-ref", plan, unitInput(t, text, domain.WeightUnitKilogram, nil))
			pricingWeight, ok := evaluation.PricingWeight()
			if !ok {
				t.Fatalf("%s kg: no pricing weight, status %s %v", text, evaluation.Status(), evaluation.Issues())
			}
			exact := new(big.Rat).Mul(ratOf(t, text), lbPerKg)
			multiples := new(big.Rat).Quo(exact, increment)
			whole, remainder := new(big.Int).QuoRem(multiples.Num(), multiples.Denom(), new(big.Int))
			up := remainder.Sign() != 0
			if c.mode == domain.RoundingHalfUp {
				up = new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(multiples.Denom()) >= 0
			}
			if up {
				whole.Add(whole, big.NewInt(1))
			}
			want := new(big.Rat).Mul(new(big.Rat).SetInt(whole), increment)
			if got := ratOf(t, pricingWeight.RoundedWeight().Value().String()); got.Cmp(want) != 0 {
				t.Fatalf("%s %s %s kg: rounded %s lb, exact quotient rounds to %s lb", c.mode, c.increment, text, got.FloatString(12), want.FloatString(12))
			}
		}
	}
}

// Covers: ADR-0346 决定四——取整策略含 NONE 时不收截断：1 KG 折磅不是有限小数，截断的值会直接进查表，
// 那是静默改金额，所以仍答原来的单位不一致（冲突，问题项 WEIGHT_UNIT_MISMATCH）；
// 同一策略下能精确换算的（LB→KG）照常评价。
func TestANonTerminatingConversionIsRefusedWhenTheRoundingCannotAbsorbTruncation(t *testing.T) {
	poundPlan := tieredPlan(t, "none-lb", domain.WeightUnitPound, domain.RoundingNone, "1", nil)
	refused := evaluate(t, "eval-none-lb", poundPlan, unitInput(t, "1", domain.WeightUnitKilogram, nil))
	if refused.Status() != domain.EvaluationConflict || len(refused.Issues()) != 1 || refused.Issues()[0].Code() != "WEIGHT_UNIT_MISMATCH" ||
		refused.Issues()[0].Message() != domain.ErrWeightUnitMismatch.Error() || len(refused.UnitConversions()) != 0 {
		t.Fatalf("evaluation = %s %v conversions %d", refused.Status(), refused.Issues(), len(refused.UnitConversions()))
	}

	kilogramPlan := tieredPlan(t, "none-kg", domain.WeightUnitKilogram, domain.RoundingNone, "1", nil)
	exact := evaluate(t, "eval-none-kg", kilogramPlan, unitInput(t, "2.5", domain.WeightUnitPound, nil))
	if totalOf(t, exact) != "8" {
		t.Fatalf("exact conversion under NONE: total %s", totalOf(t, exact))
	}
}

// Covers: ADR-0346 决定四——厘米折英寸的系数 50/127 也是无限小数：体积重由精确有理数一步算到进位单位，
// 不先把三边各自截成十进制；与独立的有理数参考实现逐一相同。
func TestCentimetreSidesToAnInchFactorAreRoundedFromTheExactProduct(t *testing.T) {
	factor := &volumetricSpec{divisor: "250", length: domain.LengthUnitInch, mode: domain.RoundingCeiling, increment: "1"}
	plan := tieredPlan(t, "cm-to-in", domain.WeightUnitPound, domain.RoundingCeiling, "1", factor)
	inchesPerCm := new(big.Rat).Inv(ratOf(t, "2.54"))
	for _, sides := range [][3]string{{"30", "30", "30"}, {"41.3", "27.9", "15.2"}, {"100", "50", "25"}, {"25.4", "25.4", "25.4"}} {
		measured := dimensions(t, sides[0], sides[1], sides[2], domain.LengthUnitCentimeter)
		evaluation := evaluate(t, "eval-cm-in", plan, unitInput(t, "0.1", domain.WeightUnitPound, &measured))
		pricingWeight, ok := evaluation.PricingWeight()
		if !ok {
			t.Fatalf("%v: no pricing weight: %s %v", sides, evaluation.Status(), evaluation.Issues())
		}
		volume := big.NewRat(1, 1)
		for _, side := range sides {
			volume.Mul(volume, new(big.Rat).Mul(ratOf(t, side), inchesPerCm))
		}
		quotient := new(big.Rat).Quo(volume, big.NewRat(250, 1))
		whole, remainder := new(big.Int).QuoRem(quotient.Num(), quotient.Denom(), new(big.Int))
		if remainder.Sign() != 0 {
			whole.Add(whole, big.NewInt(1))
		}
		volumetric, _ := pricingWeight.VolumetricWeight()
		if volumetric.Value().String() != whole.String() {
			t.Fatalf("%v cm: volumetric %s lb, exact reference %s lb", sides, volumetric.Value(), whole)
		}
	}
	// 25.4 cm 立方恰是 10 in 立方：体积 1000 in³ ÷ 250 = 4 整，是精确落在整磅上的临界例，不能多进一磅。
	exactCube := dimensions(t, "25.4", "25.4", "25.4", domain.LengthUnitCentimeter)
	evaluation := evaluate(t, "eval-cube", plan, unitInput(t, "0.1", domain.WeightUnitPound, &exactCube))
	pricingWeight, _ := evaluation.PricingWeight()
	if volumetric, _ := pricingWeight.VolumetricWeight(); volumetric.Value().String() != "4" {
		t.Fatalf("25.4 cm 立方体积重 = %s lb, want 4", volumetric.Value())
	}
	// 系数虽是 50/127，25.4 cm 乘它正好是 10：换算后的三边是有限小数，记录写精确；30 cm 乘它不是，记录写截断位数。
	exactStep := evaluation.UnitConversions()[0]
	if exactStep.Factor() != "50/127" || !exactStep.Exact() || exactStep.Places() != 0 || joinForTest(exactStep.Converted()) != "10 10 10" {
		t.Fatalf("exact step = factor %s exact %v places %d converted %s", exactStep.Factor(), exactStep.Exact(), exactStep.Places(), joinForTest(exactStep.Converted()))
	}
	thirty := dimensions(t, "30", "30", "30", domain.LengthUnitCentimeter)
	inexact := evaluate(t, "eval-thirty", plan, unitInput(t, "0.1", domain.WeightUnitPound, &thirty)).UnitConversions()[0]
	if inexact.Exact() || inexact.Places() != 16 || joinForTest(inexact.Converted()) != "11.811023622047244 11.811023622047244 11.811023622047244" {
		t.Fatalf("inexact step = exact %v places %d converted %s", inexact.Exact(), inexact.Places(), joinForTest(inexact.Converted()))
	}
}

// Covers: ADR-0346 决定二——单位本来就一致的路径换算为恒等：不记换算、解释不多一行、不出 unitConversions 键，
// 评价的语义摘要与换算接入之前逐字节相同（摘要对照见票面完成记录里在 origin/main 上取的同一组值）。
func TestAnEvaluationInTheTableUnitIsUntouchedByConversion(t *testing.T) {
	plan := tieredPlan(t, "identity", domain.WeightUnitKilogram, domain.RoundingCeiling, "0.5", nil)
	evaluation := evaluate(t, "eval-identity", plan, unitInput(t, "1.2", domain.WeightUnitKilogram, nil))
	if len(evaluation.UnitConversions()) != 0 || explanationHas(evaluation, "unit conversion") {
		t.Fatalf("单位一致的评价不该有换算：%v %v", evaluation.UnitConversions(), evaluation.Explanation())
	}
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil || strings.Contains(string(raw), "unitConversions") {
		t.Fatalf("快照 err=%v，不该带 unitConversions：%s", err, raw)
	}
	again := evaluate(t, "eval-identity", plan, unitInput(t, "1.2", domain.WeightUnitKilogram, nil))
	if again.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatal("同输入同方案的两次评价摘要不同")
	}
}

// Covers: ADR-0346 决定三——回放按评价记下的系数重算得同结果；换算后才待判断的评价（实重已换算、MAX 的尺寸还没到）
// 也要能重放到同一个待判断，所以换算记录失败路径也留着。
func TestReplayOfAConvertedEvaluationReproducesTheSameResult(t *testing.T) {
	plan := tieredPlan(t, "replay", domain.WeightUnitPound, domain.RoundingCeiling, "0.5", nil)
	original := evaluate(t, "eval-replay", plan, unitInput(t, "1", domain.WeightUnitKilogram, nil))
	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-replay-2"), original, plan, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Status() != domain.EvaluationCompleted || replay.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("replay status %s, digest equal %v, issues %v", replay.Status(), replay.SemanticDigest() == original.SemanticDigest(), replay.Issues())
	}
	if len(replay.UnitConversions()) != 1 || replay.UnitConversions()[0].Factor() != original.UnitConversions()[0].Factor() {
		t.Fatalf("重放没带上记下的换算：%+v", replay.UnitConversions())
	}

	factor := &volumetricSpec{divisor: "5000", length: domain.LengthUnitCentimeter, mode: domain.RoundingCeiling, increment: "0.1"}
	maxPlan := tieredPlan(t, "replay-max", domain.WeightUnitKilogram, domain.RoundingCeiling, "0.5", factor)
	pending := evaluate(t, "eval-pending", maxPlan, unitInput(t, "2.5", domain.WeightUnitPound, nil))
	if pending.Status() != domain.EvaluationPending || pending.Issues()[0].Code() != "DIMENSIONS_REQUIRED" || len(pending.UnitConversions()) != 1 {
		t.Fatalf("pending = %s %v conversions %d", pending.Status(), pending.Issues(), len(pending.UnitConversions()))
	}
	replayedPending, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-pending-2"), pending, maxPlan, domain.EvidenceSynthetic)
	if err != nil || replayedPending.Status() != domain.EvaluationPending || replayedPending.SemanticDigest() != pending.SemanticDigest() {
		t.Fatalf("pending replay = %v %v", replayedPending.Status(), err)
	}
}

// Covers: ADR-0346 决定三——换算记录随评价快照往返，读回门逐条重验：被改动的换算值、非规范写法的系数、
// 目录外或跨类别的单位、与输入对不上的原值，都在读回时被拒。
func TestUnitConversionRecordsSurviveTheSnapshotAndAreRevalidatedOnReadBack(t *testing.T) {
	plan := tieredPlan(t, "snapshot", domain.WeightUnitPound, domain.RoundingCeiling, "0.5", nil)
	evaluation := evaluate(t, "eval-snapshot", plan, unitInput(t, "1", domain.WeightUnitKilogram, nil))
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rebuilt, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if len(rebuilt.UnitConversions()) != 1 || rebuilt.UnitConversions()[0].Factor() != "100000000/45359237" || rebuilt.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatalf("round trip lost the record: %+v", rebuilt.UnitConversions())
	}
	if replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-snapshot-2"), rebuilt, plan, domain.EvidenceSynthetic); err != nil || replay.SemanticDigest() != evaluation.SemanticDigest() {
		t.Fatalf("读回的评价重放 = %v %v", replay.Status(), err)
	}

	text := string(raw)
	tampered := map[string]string{
		"换算值被改":    strings.Replace(text, `"converted":[{"coefficient":"220462262184","scale":11}]`, `"converted":[{"coefficient":"220462262185","scale":11}]`, 1),
		"系数写法不规范":  strings.Replace(text, `"factor":"100000000/45359237"`, `"factor":"200000000/90718474"`, 1),
		"目的单位在目录外": strings.Replace(text, `"to":"LB"`, `"to":"STONE"`, 1),
		"目的单位跨类别":  strings.Replace(text, `"to":"LB"`, `"to":"CM"`, 1),
		"原值与输入对不上": strings.Replace(text, `"original":[{"coefficient":"1","scale":0}]`, `"original":[{"coefficient":"2","scale":0}]`, 1),
		"精确标记被改":   strings.Replace(text, `"exact":false`, `"exact":true`, 1),
	}
	for name, document := range tampered {
		if document == text {
			t.Fatalf("%s: 篡改没有生效，快照形状变了：%s", name, text)
		}
		if _, err := domain.RehydrateEvaluationSnapshot([]byte(document)); !errors.Is(err, domain.ErrEvaluationSnapshotInvalid) {
			t.Errorf("%s: 读回应被拒，得到 %v", name, err)
		}
	}
}

// Covers: ADR-0346 决定一——换算只发生在评价这一层：直接调 CalculatePricingWeight 的人单位对不上仍然答
// 单位不一致，不会被悄悄换算（TestVolumetricFactorRefusesSidesMeasuredInAnotherUnit 钉长度那一半）。
func TestCalculatePricingWeightItselfStillRefusesAnotherWeightUnit(t *testing.T) {
	plan := tieredPlan(t, "bare", domain.WeightUnitKilogram, domain.RoundingCeiling, "0.5", nil)
	input := unitInput(t, "2.5", domain.WeightUnitPound, nil)
	if _, err := domain.CalculatePricingWeight(input, plan.WeightPolicy(), domain.WeightUnitKilogram); !errors.Is(err, domain.ErrWeightUnitMismatch) {
		t.Fatalf("err = %v, want ErrWeightUnitMismatch", err)
	}
}

// Covers: ADR-0346 决定一——票级主体：成员合计实重与合计体积重都以磅给、价表按千克查，两者用同一个系数换算，
// 换算记录各记一条，评价照常完成。
func TestAnAggregateEvaluationConvertsTheMemberTotals(t *testing.T) {
	factor := &volumetricSpec{divisor: "5000", length: domain.LengthUnitCentimeter, mode: domain.RoundingCeiling, increment: "0.1"}
	plan := aggregatePlanForConversion(t, factor)
	members := []domain.PackageID{mustValue(t, domain.NewPackageID, "package-1"), mustValue(t, domain.NewPackageID, "package-2")}
	volumetric := weight(t, "4", domain.WeightUnitPound)
	manifest, err := domain.NewMemberManifest(members, weight(t, "2", domain.WeightUnitPound), &volumetric)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	subject, err := domain.NewShipmentSubject("shipment-1")
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	input, err := domain.NewAggregatePricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"), mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "Z1", manifest, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("aggregate input: %v", err)
	}
	evaluation := evaluate(t, "eval-aggregate", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status %s %v", evaluation.Status(), evaluation.Issues())
	}
	// 合计体积重 4 LB = 1.81436948 KG 大于合计实重 2 LB = 0.90718474 KG，MAX 取前者，进到 0.5 是 2 KG，落 [2,3)。
	if total := totalOf(t, evaluation); total != "12" {
		t.Fatalf("total = %s, want 12", total)
	}
	steps := evaluation.UnitConversions()
	if len(steps) != 2 || steps[0].Quantity() != domain.ConvertedActualWeight || steps[1].Quantity() != domain.ConvertedTotalVolumetricWeight ||
		steps[1].Converted()[0].String() != "1.81436948" {
		t.Fatalf("steps = %+v", steps)
	}
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := domain.RehydrateEvaluationSnapshot(raw); err != nil {
		t.Fatalf("聚合主体的换算记录读回：%v", err)
	}
}

func aggregatePlanForConversion(t testing.TB, factor *volumetricSpec) domain.PricingPlanVersion {
	t.Helper()
	base := tieredPlan(t, "aggregate", domain.WeightUnitKilogram, domain.RoundingCeiling, "0.5", factor)
	currency := mustValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-aggregate"), "Z1",
		weight(t, "0", domain.WeightUnitKilogram), weight(t, "1000", domain.WeightUnitKilogram), money(t, "12", currency),
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-aggregate-2", "v1"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram, effectivePeriod(t), []domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	plan, err := domain.NewAggregatePricingPlanVersion(
		domain.AggregationPerShipment,
		versionReference(t, domain.ArtifactPricingPlan, "plan-aggregate-2", "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t), table, base.WeightPolicy(), nil, domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("aggregate plan: %v", err)
	}
	return plan
}

// Covers: ADR-0346 决定三、四——冻结模式连截断位数也只认记下的：摘要里的实重是截断后的十进制串，日后调整位数公式
// 不能让既有评价重放出另一串。记下 20 位（大于当前公式的 11 位）的评价，重放与读回后再重放都得同一个摘要；
// 记下的位数小于当前公式推出的下限（8 位）不静默改用较小位数，按答不出处理，仍是原来的单位不一致。
func TestFrozenReplayKeepsTheRecordedTruncationPlaces(t *testing.T) {
	plan := tieredPlan(t, "places", domain.WeightUnitPound, domain.RoundingCeiling, "0.5", nil)
	input := unitInput(t, "1", domain.WeightUnitKilogram, nil)
	fresh := evaluate(t, "eval-places-fresh", plan, input)
	if fresh.UnitConversions()[0].Places() != 11 {
		t.Fatalf("当前公式下限 = %d 位，want 11", fresh.UnitConversions()[0].Places())
	}

	asRecordedBy := func(places uint32) domain.PricingEvaluation {
		request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, "eval-places"), plan, input, domain.EvidenceSynthetic)
		if err != nil {
			t.Fatalf("evaluation request: %v", err)
		}
		recorded := domain.RecordedTruncatedWeightConversionForTest(domain.ConvertedActualWeight, "KG", "LB", "100000000/45359237", "1", places)
		return domain.EvaluatePricing(request.WithRecordedUnitConversions([]domain.UnitConversionStep{recorded}))
	}

	wide := asRecordedBy(20)
	if wide.Status() != domain.EvaluationCompleted || wide.UnitConversions()[0].Places() != 20 {
		t.Fatalf("记 20 位的评价 = %s %v", wide.Status(), wide.Issues())
	}
	freshWeight, _ := fresh.PricingWeight()
	wideWeight, _ := wide.PricingWeight()
	if wideWeight.ActualWeight().Value().String() == freshWeight.ActualWeight().Value().String() || wide.SemanticDigest() == fresh.SemanticDigest() {
		t.Fatal("位数不同，摘要里的实重串应当不同——这正是位数要冻结的原因")
	}
	if wideWeight.RoundedWeight().Value().String() != "2.5" || totalOf(t, wide) != totalOf(t, fresh) {
		t.Fatalf("位数只是更细，进位后的重量与金额不变：%s / %s", wideWeight.RoundedWeight().Value(), totalOf(t, wide))
	}

	replay, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-places-replay"), wide, plan, domain.EvidenceSynthetic)
	if err != nil || replay.Status() != domain.EvaluationCompleted || replay.SemanticDigest() != wide.SemanticDigest() || replay.UnitConversions()[0].Places() != 20 {
		t.Fatalf("记 20 位的评价重放 = %s %v digest 相同 %v", replay.Status(), err, replay.SemanticDigest() == wide.SemanticDigest())
	}
	raw, err := domain.MarshalEvaluationSnapshot(wide)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rebuilt, err := domain.RehydrateEvaluationSnapshot(raw)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	again, err := domain.ReplayPricingEvaluation(mustValue(t, domain.NewEvaluationID, "eval-places-replay-2"), rebuilt, plan, domain.EvidenceSynthetic)
	if err != nil || again.SemanticDigest() != wide.SemanticDigest() {
		t.Fatalf("读回后重放 = %s %v", again.Status(), err)
	}

	atFloor := asRecordedBy(11)
	if atFloor.Status() != domain.EvaluationCompleted || atFloor.SemanticDigest() != fresh.SemanticDigest() {
		t.Fatalf("记下的位数恰等于下限应照常评价且同摘要：%s", atFloor.Status())
	}
	short := asRecordedBy(8)
	if short.Status() != domain.EvaluationConflict || len(short.Issues()) != 1 || short.Issues()[0].Code() != "WEIGHT_UNIT_MISMATCH" ||
		short.Issues()[0].Message() != domain.ErrWeightUnitMismatch.Error() || len(short.UnitConversions()) != 0 {
		t.Fatalf("记 8 位（小于下限）= %s %v conversions %d", short.Status(), short.Issues(), len(short.UnitConversions()))
	}
}
