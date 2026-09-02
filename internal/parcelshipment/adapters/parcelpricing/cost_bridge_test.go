package parcelpricing_test

import (
	"errors"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	bridge "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/parcelpricing"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证成本分值桥的翻译是全函数且逐格忠实：`parcel-pricing` 的五种评价结果各自译到
// 渠道择优侧的一格，一格不并、一格不漏。桥自己**不做出局判断**——那一条由票 `01` 裁给
// 择优比较器，桥只如实交出「算得出」或「算不出，是哪一格」。

const businessDay = "2026-08-07T10:00:00Z"

func mustValue[T any](t testing.TB, constructor func(string) (T, error), value string) T {
	t.Helper()
	result, err := constructor(value)
	if err != nil {
		t.Fatalf("构造 %q：%v", value, err)
	}
	return result
}

func at(t testing.TB, stamp string) time.Time {
	t.Helper()
	moment, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("解析时刻 %q：%v", stamp, err)
	}
	return moment
}

func kilograms(t testing.TB, value string) ppdomain.Weight {
	t.Helper()
	result, err := ppdomain.NewWeight(mustValue(t, ppdomain.ParseDecimal, value), ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("构造重量 %s：%v", value, err)
	}
	return result
}

func usd(t testing.TB, value string) ppdomain.Money {
	t.Helper()
	amount, err := ppdomain.NewMoneyFromString(value, mustValue(t, ppdomain.NewCurrency, "USD"))
	if err != nil {
		t.Fatalf("构造金额 %s：%v", value, err)
	}
	return amount
}

func reference(t testing.TB, kind ppdomain.ArtifactKind, id string) ppdomain.VersionReference {
	t.Helper()
	result, err := ppdomain.NewVersionReference(kind, id, "v1", "sha256:syn-"+id+"-v1")
	if err != nil {
		t.Fatalf("构造版本引用 %s：%v", id, err)
	}
	return result
}

func year2026(t testing.TB) ppdomain.EffectivePeriod {
	t.Helper()
	period, err := ppdomain.NewEffectivePeriod(at(t, "2026-01-01T00:00:00Z"), at(t, "2027-01-01T00:00:00Z"))
	if err != nil {
		t.Fatalf("构造生效区间：%v", err)
	}
	return period
}

// buyPlan 造一张 `BUY`/供应商成本方向的合成价卡：单档 [0,10) 千克，只按实重计价。
func buyPlan(
	t testing.TB,
	suffix, baseAmount string,
	rules []ppdomain.FixedChargeRule,
	structures ppdomain.PricingPlanStructures,
) ppdomain.PricingPlanVersion {
	t.Helper()
	return directedPlan(t, suffix, baseAmount, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, rules, structures)
}

func directedPlan(
	t testing.TB,
	suffix, baseAmount string,
	direction ppdomain.PricingDirection,
	purpose ppdomain.PricingPurpose,
	rules []ppdomain.FixedChargeRule,
	structures ppdomain.PricingPlanStructures,
) ppdomain.PricingPlanVersion {
	t.Helper()

	entry, err := ppdomain.NewRateEntry(
		mustValue(t, ppdomain.NewRateEntryID, "entry-"+suffix),
		"Z1",
		kilograms(t, "0"),
		kilograms(t, "10"),
		usd(t, baseAmount),
	)
	if err != nil {
		t.Fatalf("构造费率段 %s：%v", suffix, err)
	}
	table, err := ppdomain.NewRateTableVersion(
		reference(t, ppdomain.ArtifactRateTable, "table-"+suffix),
		ppdomain.RateTableFamilyWeightZone,
		mustValue(t, ppdomain.NewCurrency, "USD"),
		ppdomain.WeightUnitKilogram,
		year2026(t),
		[]ppdomain.RateEntry{entry},
	)
	if err != nil {
		t.Fatalf("构造价表 %s：%v", suffix, err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, kilograms(t, "0.5"))
	if err != nil {
		t.Fatalf("构造取整策略 %s：%v", suffix, err)
	}
	weightPolicy, err := ppdomain.NewPricingWeightPolicy(
		reference(t, ppdomain.ArtifactWeightPolicy, "weight-"+suffix),
		ppdomain.PricingWeightActualOnly,
		rounding,
		nil,
	)
	if err != nil {
		t.Fatalf("构造计价重量策略 %s：%v", suffix, err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(
		reference(t, ppdomain.ArtifactPricingPlan, "plan-"+suffix),
		mustValue(t, ppdomain.NewPricingScopeID, "scope-1"),
		direction,
		purpose,
		mustValue(t, ppdomain.NewChargeCode, "BASE_FREIGHT"),
		year2026(t),
		table,
		weightPolicy,
		rules,
		structures,
	)
	if err != nil {
		t.Fatalf("构造价卡 %s：%v", suffix, err)
	}
	return plan
}

func pricingInput(t testing.TB, zone string, businessAt time.Time, sides *ppdomain.Dimensions) ppdomain.PricingInputSnapshot {
	t.Helper()

	subject, err := ppdomain.NewAcceptedPackageSubject(mustValue(t, ppdomain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("构造评价对象：%v", err)
	}
	input, err := ppdomain.NewPricingInputSnapshot(
		mustValue(t, ppdomain.NewTenantID, "tenant-1"),
		mustValue(t, ppdomain.NewPricingScopeID, "scope-1"),
		subject,
		zone,
		kilograms(t, "1"),
		sides,
		businessAt,
	)
	if err != nil {
		t.Fatalf("构造计价输入：%v", err)
	}
	return input
}

func evaluate(t testing.TB, id string, plan ppdomain.PricingPlanVersion, input ppdomain.PricingInputSnapshot) ppdomain.PricingEvaluation {
	t.Helper()

	request, err := ppdomain.NewEvaluationRequest(
		mustValue(t, ppdomain.NewEvaluationID, id),
		plan,
		input,
		ppdomain.EvidenceSynthetic,
	)
	if err != nil {
		t.Fatalf("构造评价请求 %s：%v", id, err)
	}
	return ppdomain.EvaluatePricing(request)
}

// oversizeRefusalStructures 造一张带拒收条款的结构集合：最长边超过 20 厘米即拒。
func oversizeRefusalStructures(t testing.TB) ppdomain.PricingPlanStructures {
	t.Helper()

	limit, err := ppdomain.NewLength(mustValue(t, ppdomain.ParseDecimal, "20"), ppdomain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("构造长度上限：%v", err)
	}
	condition, err := ppdomain.NewLengthFeatureCondition(ppdomain.FeatureLongestSide, ppdomain.ComparisonGreaterThan, limit)
	if err != nil {
		t.Fatalf("构造判定条件：%v", err)
	}
	trigger, err := ppdomain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("构造触发条件：%v", err)
	}
	rule, err := ppdomain.NewExclusionRule("oversize-refusal", "over the last size tier the card carries", trigger)
	if err != nil {
		t.Fatalf("构造拒收条款：%v", err)
	}
	structures, err := ppdomain.PricingPlanStructures{}.WithExclusionRules(rule)
	if err != nil {
		t.Fatalf("挂载拒收条款：%v", err)
	}
	return structures
}

func oversizeSides(t testing.TB) *ppdomain.Dimensions {
	t.Helper()

	sides, err := ppdomain.NewDimensions(
		mustValue(t, ppdomain.ParseDecimal, "60"),
		mustValue(t, ppdomain.ParseDecimal, "50"),
		mustValue(t, ppdomain.ParseDecimal, "40"),
		ppdomain.LengthUnitCentimeter,
	)
	if err != nil {
		t.Fatalf("构造外廓：%v", err)
	}
	return &sides
}

func candidate(t testing.TB, id string) psdomain.ChannelCandidateID {
	t.Helper()
	return mustValue(t, psdomain.NewChannelCandidateID, id)
}

// Covers: 算得出价的候选译成已确立成本，金额与币种照评价原样过来，不折不换。
//
// 断言取评价自己交出的总价而不是写死 "12.00"：金额是**由那张卡算出来的**，桥的职责是
// 不改动它。写死一个字面量就变成在证价卡算得对，那是计价上下文自己的测试在管的事。
func TestAPricedEvaluationCrossesAsAnEstablishedCost(t *testing.T) {
	t.Parallel()

	evaluation := evaluate(t, "eval-priced",
		buyPlan(t, "priced", "12.00", nil, ppdomain.PricingPlanStructures{}),
		pricingInput(t, "Z1", at(t, businessDay), nil))
	total, priced := evaluation.Total()
	if !priced {
		t.Fatalf("前置：合成卡没算出总价，结果 = %s，问题 = %v", evaluation.Status(), evaluation.Issues())
	}

	cost, err := bridge.ChannelCandidateCostOf(candidate(t, "cand-priced"), evaluation)
	if err != nil {
		t.Fatalf("翻译已定价评价：%v", err)
	}

	amount, established := cost.Amount()
	if !established {
		t.Fatal("算得出价的候选被译成了未确立成本")
	}
	if amount.String() != total.Amount().String() {
		t.Fatalf("成本金额 = %q，want %q（评价原样）", amount.String(), total.Amount().String())
	}
	currency, carried := cost.Currency()
	if !carried || currency.String() != total.Currency().String() {
		t.Fatalf("成本币种 = %q，want %q", currency.String(), total.Currency().String())
	}
	if _, unavailable := cost.Unavailability(); unavailable {
		t.Fatal("算得出价的候选带上了出局格")
	}
}

// Covers: 四种非完成结果各自译到自己那一格，不互相顶替（`parcel-pricing` CONTEXT「依据
// 不足形成待判断，明确排除形成不可计价，互斥候选形成冲突，请求不合法或计算失败形成未
// 形成；四者不得互相替代」）。
//
// 四种评价都由真实的评价器产出而不是手搭：这一格要证的正是「提供方给出某种结果时桥译成
// 什么」，用一个假的评价对象绕过评价器，就把待证的那半换成了我自己的假设。
func TestEachUnpriceableOutcomeCrossesIntoItsOwnGrade(t *testing.T) {
	t.Parallel()

	deduction, err := ppdomain.NewFixedChargeRule(
		"discount-over-subtotal",
		mustValue(t, ppdomain.NewChargeCode, "RULE_OVER_DISCOUNT"),
		"discount larger than the base",
		ppdomain.ChargeEffectDeduct,
		usd(t, "99.00"),
		1,
	)
	if err != nil {
		t.Fatalf("构造超额抵扣规则：%v", err)
	}

	cases := []struct {
		name       string
		evaluation ppdomain.PricingEvaluation
		wantStatus ppdomain.EvaluationStatus
		wantGrade  psdomain.ChannelCostUnavailability
	}{
		{
			// 区域查不到费率是依据不足：补上这个区域的费率段就能出价。
			name: "待判断",
			evaluation: evaluate(t, "eval-pending",
				buyPlan(t, "pending", "12.00", nil, ppdomain.PricingPlanStructures{}),
				pricingInput(t, "UNKNOWN", at(t, businessDay), nil)),
			wantStatus: ppdomain.EvaluationPending,
			wantGrade:  psdomain.ChannelCostPendingEvidence,
		},
		{
			// 卡上的拒收条款明确排除这一票，是确定结论，补事实不会改变它。
			name: "不可计价",
			evaluation: evaluate(t, "eval-unratable",
				buyPlan(t, "unratable", "12.00", nil, oversizeRefusalStructures(t)),
				pricingInput(t, "Z1", at(t, businessDay), oversizeSides(t))),
			wantStatus: ppdomain.EvaluationUnratable,
			wantGrade:  psdomain.ChannelCostRatecardExclusion,
		},
		{
			// 业务时点落在版本生效区间之外：这份卡对这一票不适用，要治理侧处置。
			name: "冲突",
			evaluation: evaluate(t, "eval-conflict",
				buyPlan(t, "conflict", "12.00", nil, ppdomain.PricingPlanStructures{}),
				pricingInput(t, "Z1", at(t, "2025-08-07T10:00:00Z"), nil)),
			wantStatus: ppdomain.EvaluationConflict,
			wantGrade:  psdomain.ChannelCostConflict,
		},
		{
			// 抵扣超过基础费用，算不下去：技术失败，不是业务结论。
			name: "未形成",
			evaluation: evaluate(t, "eval-not-formed",
				buyPlan(t, "not-formed", "12.00", []ppdomain.FixedChargeRule{deduction}, ppdomain.PricingPlanStructures{}),
				pricingInput(t, "Z1", at(t, businessDay), nil)),
			wantStatus: ppdomain.EvaluationFailed,
			wantGrade:  psdomain.ChannelCostNotFormed,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.evaluation.Status() != testCase.wantStatus {
				t.Fatalf("前置：合成卡产出的结果 = %s，want %s（问题 = %v）",
					testCase.evaluation.Status(), testCase.wantStatus, testCase.evaluation.Issues())
			}

			cost, err := bridge.ChannelCandidateCostOf(candidate(t, "cand-"+testCase.name), testCase.evaluation)
			if err != nil {
				t.Fatalf("翻译 %s 评价：%v", testCase.wantStatus, err)
			}
			if _, established := cost.Amount(); established {
				t.Fatalf("%s 的候选交出了成本金额——零金额顶替就是从这里开始的", testCase.wantStatus)
			}
			grade, unavailable := cost.Unavailability()
			if !unavailable || grade != testCase.wantGrade {
				t.Fatalf("出局格 = %s，want %s", grade, testCase.wantGrade)
			}
		})
	}
}

// Covers: 价格方向隔离（`parcel-pricing` CONTEXT「`BUY` 不可计价不推导 `SELL` 不可计价，
// 反之亦然」）。桥服务的是供应商成本这一维，拿一份 `SELL`/客户收费的评价当成本用，得出的
// 是「哪家渠道向客户收得少」，而择优要问的是「哪家渠道让运营企业花得少」——两者可以指向
// 不同的供应商，而错的那个会被真的采购下去。
//
// 这一格必须拒而不是译：`SELL` 评价在结构上完全合法、金额齐备，译过去一路绿，没有任何
// 下游还能看出这个数字答的是另一个问题。
func TestASellEvaluationIsRefusedRatherThanReadAsASupplierCost(t *testing.T) {
	t.Parallel()

	evaluation := evaluate(t, "eval-sell",
		directedPlan(t, "sell", "12.00",
			ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge,
			nil, ppdomain.PricingPlanStructures{}),
		pricingInput(t, "Z1", at(t, businessDay), nil))
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("前置：SELL 合成卡没算出价，结果 = %s，问题 = %v", evaluation.Status(), evaluation.Issues())
	}

	if _, err := bridge.ChannelCandidateCostOf(candidate(t, "cand-sell"), evaluation); !errors.Is(err, bridge.ErrNotASupplierCostEvaluation) {
		t.Fatalf("翻译 SELL 评价 err = %v，want %v", err, bridge.ErrNotASupplierCostEvaluation)
	}
}
