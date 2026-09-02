package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// volumetricPlan 造一张 `BUY` 合成价卡，体积系数与两档费率都由调用方给。既有的
// syntheticPlanWithBaseCode 把系数写死成 5000、只给一档费率，而本文件要证的正是
// **不同候选的系数不同会各自算出不同的计费重**，系数不可配就证不了。
func volumetricPlan(t testing.TB, suffix, divisor, lowBand, highBand string) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")

	entryLow, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-low-"+suffix),
		"Z1",
		weight(t, "0", domain.WeightUnitKilogram),
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, lowBand, currency),
	)
	if err != nil {
		t.Fatalf("构造低档费率段 %s：%v", suffix, err)
	}
	entryHigh, err := domain.NewOpenEndedRateEntry(
		mustValue(t, domain.NewRateEntryID, "entry-high-"+suffix),
		"Z1",
		weight(t, "10", domain.WeightUnitKilogram),
		money(t, highBand, currency),
	)
	if err != nil {
		t.Fatalf("构造高档费率段 %s：%v", suffix, err)
	}
	table, err := domain.NewRateTableVersion(
		versionReference(t, domain.ArtifactRateTable, "table-"+suffix, "v1"),
		domain.RateTableFamilyWeightZone,
		currency,
		domain.WeightUnitKilogram,
		effectivePeriod(t),
		[]domain.RateEntry{entryLow, entryHigh},
	)
	if err != nil {
		t.Fatalf("构造价表 %s：%v", suffix, err)
	}

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.5", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("构造取整策略 %s：%v", suffix, err)
	}
	factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight(t, "0.1", domain.WeightUnitKilogram))
	if err != nil {
		t.Fatalf("构造体积重取整 %s：%v", suffix, err)
	}
	factor, err := domain.NewVolumetricFactor(decimal(t, divisor), domain.LengthUnitCentimeter, factorRounding)
	if err != nil {
		t.Fatalf("构造体积系数 %s：%v", suffix, err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		versionReference(t, domain.ArtifactWeightPolicy, "weight-"+suffix, "v1"),
		domain.PricingWeightMax,
		rounding,
		&factor,
	)
	if err != nil {
		t.Fatalf("构造计价重量策略 %s：%v", suffix, err)
	}

	plan, err := domain.NewPricingPlanVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan-"+suffix, "v1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionBuy,
		domain.PricingPurposeSupplierCost,
		mustValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		effectivePeriod(t),
		table,
		weightPolicy,
		nil,
		domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("构造价卡 %s：%v", suffix, err)
	}
	return plan
}

func planTarget(t testing.TB, id string, plan domain.PricingPlanVersion) domain.PlanEvaluationTarget {
	t.Helper()

	target, err := domain.NewPlanEvaluationTarget(mustValue(t, domain.NewEvaluationID, id), plan)
	if err != nil {
		t.Fatalf("构造批量评价目标 %s：%v", id, err)
	}
	return target
}

func completedTotal(t testing.TB, evaluation domain.PricingEvaluation) domain.Money {
	t.Helper()

	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("评价 %s 结果 = %s，want %s（问题：%v）",
			evaluation.ID().String(), evaluation.Status(), domain.EvaluationCompleted, evaluation.Issues())
	}
	total, priced := evaluation.Total()
	if !priced {
		t.Fatalf("评价 %s 已完成却没有总价", evaluation.ID().String())
	}
	return total
}

// Covers: 票 13 最容易做错的那一格——`PAR-NET-16` 要求各候选按**自己**方案的体积系数与
// 进位算出自己的计费重，而不是算一次计费重再套不同费率。
//
// 判据取 `PAR-NET-16` 的原话「单价更低的候选可以因此总价更高」，它独立于实现：便宜卡两档
// 费率（10/30）都低于贵卡（18/50），但它的体积系数 2000 狠得多，把 60×50×40 厘米这一件的
// 体积重推到 60 千克、掉进高档；贵卡系数 24000 只算出 5 千克，留在低档。于是每档都更贵的
// 那张卡反而报出更低的总价。
//
// 「算一次计费重再套不同费率」那个写法在这里必然选错：无论那一次算出的是 60 还是 5，两卡
// 都会落进同一档，而便宜卡在任何同一档上都更便宜，于是它每次都赢。
func TestBatchEvaluationGivesEachCandidateItsOwnBillableWeight(t *testing.T) {
	t.Parallel()

	input := syntheticInputWithDimensions(t, "1", "Z1",
		dimensions(t, "60", "50", "40", domain.LengthUnitCentimeter))

	evaluations := domain.EvaluatePricingAcrossPlans(
		input,
		domain.EvidenceSynthetic,
		[]domain.PlanEvaluationTarget{
			planTarget(t, "eval-cheap-rate", volumetricPlan(t, "cheap-rate-harsh-divisor", "2000", "10", "30")),
			planTarget(t, "eval-dear-rate", volumetricPlan(t, "dear-rate-lenient-divisor", "24000", "18", "50")),
		},
	)

	if len(evaluations) != 2 {
		t.Fatalf("评价条数 = %d，want 2", len(evaluations))
	}
	cheapRate := completedTotal(t, evaluations[0])
	dearRate := completedTotal(t, evaluations[1])

	if cheapRate.Amount().String() != "30" {
		t.Fatalf("便宜卡总价 = %s，want 30（体积重 60 千克落高档）", cheapRate.Amount().String())
	}
	if dearRate.Amount().String() != "18" {
		t.Fatalf("贵卡总价 = %s，want 18（体积重 5 千克留低档）", dearRate.Amount().String())
	}
	if cheapRate.Amount().Cmp(dearRate.Amount()) <= 0 {
		t.Fatalf("便宜卡总价 %s 未高于贵卡 %s——计费重被算了一次然后套到两张卡上了",
			cheapRate.Amount().String(), dearRate.Amount().String())
	}
}

// Covers: 票 13「某个候选算不出不得中断其余候选」。算不出的那个摆在中间，是要钉住「遇错
// 就返回」那个写法——它会连**排在后面、本来算得出**的候选一起吞掉，而调用方只看到一批
// 没有价格，分不清是这批都不可计价还是有人半路把它掐了。
//
// 中间这张卡用既有合成卡（体积系数 5000、只声明 [0,10) 一档）：同一件 60×50×40 厘米算出
// 24 千克计费重，掉在它的档位之外，于是自然落进 `RATE_NOT_FOUND` 的`待判断`。
func TestBatchEvaluationKeepsPricingTheRestWhenOneCandidateCannotBePriced(t *testing.T) {
	t.Parallel()

	input := syntheticInputWithDimensions(t, "1", "Z1",
		dimensions(t, "60", "50", "40", domain.LengthUnitCentimeter))
	unratable := syntheticPlan(t, "batch-out-of-band",
		domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost, "12", domain.PricingWeightMax, nil)

	evaluations := domain.EvaluatePricingAcrossPlans(
		input,
		domain.EvidenceSynthetic,
		[]domain.PlanEvaluationTarget{
			planTarget(t, "eval-batch-cheap", volumetricPlan(t, "batch-cheap", "2000", "10", "30")),
			planTarget(t, "eval-batch-unratable", unratable),
			planTarget(t, "eval-batch-dear", volumetricPlan(t, "batch-dear", "24000", "18", "50")),
		},
	)

	if len(evaluations) != 3 {
		t.Fatalf("评价条数 = %d，want 3——批量口必须逐格作答，算不出的那一格也要占位", len(evaluations))
	}
	// 结果按入参次序对位交回，调用方据此把评价接回自己的候选；次序错位比少一条更难发现。
	wantIDs := []string{"eval-batch-cheap", "eval-batch-unratable", "eval-batch-dear"}
	for index, want := range wantIDs {
		if got := evaluations[index].ID().String(); got != want {
			t.Fatalf("第 %d 格评价标识 = %q，want %q", index, got, want)
		}
	}

	blocked := evaluations[1]
	if blocked.Status() != domain.EvaluationPending {
		t.Fatalf("算不出的候选结果 = %s，want %s", blocked.Status(), domain.EvaluationPending)
	}
	if _, priced := blocked.Total(); priced {
		t.Fatal("待判断的候选交出了总价")
	}
	if len(blocked.Issues()) == 0 || blocked.Issues()[0].Code() != "RATE_NOT_FOUND" {
		t.Fatalf("算不出的候选问题 = %v，want RATE_NOT_FOUND", blocked.Issues())
	}

	if got := completedTotal(t, evaluations[0]).Amount().String(); got != "30" {
		t.Fatalf("首个候选总价 = %s，want 30", got)
	}
	if got := completedTotal(t, evaluations[2]).Amount().String(); got != "18" {
		t.Fatalf("末个候选总价 = %s，want 18——它排在算不出的那个之后，被一并掐掉了", got)
	}
}
