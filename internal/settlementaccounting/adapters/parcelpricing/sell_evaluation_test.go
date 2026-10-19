package parcelpricing_test

import (
	"errors"
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 SELL 读口在票 sell-customer-charge/04 里多交的几样：评价回指照 SELL 请求身份带过来，没有回指是零值；全部费用行的
// 计价费用代码按行序照抄；卡没声明金额取整不报错，交回「金额未声明」那一格并照带回指与代码，SA 才记得了`待判断`；
// BUY 评价答端口上那一枚哨兵。夹具全为 SYN 卡（S 级）。

// sellCard 造一张 CNY 的 SELL·客户费用卡：Z1 一行 100、一条固定附加费 REMOTE_AREA_SURCHARGE 5；rounded 时声明合计
// HALF_UP 到 0.01。附加费让评价有两条代码不同的费用行。
func sellCard(t *testing.T, direction ppdomain.PricingDirection, purpose ppdomain.PricingPurpose, rounded bool) ppdomain.PricingPlanVersion {
	t.Helper()
	var amountRounding *pptest.AmountRoundingSpec
	if rounded {
		amountRounding = &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		}
	}
	base := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-SELL-CARD", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-SELL-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-SELL-WEIGHT", "v1"),
		Scope:                 "scope-1",
		Direction:             direction,
		Purpose:               purpose,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "entry",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "100",
		WeightRounding:      ppdomain.RoundingNone,
		WeightStepKilograms: "1",
		AmountRounding:      amountRounding,
	})
	cny := must[ppdomain.Currency](t)(ppdomain.NewCurrency("CNY"))
	surcharge := must[ppdomain.FixedChargeRule](t)(ppdomain.NewFixedChargeRule("SYN-RULE-REMOTE",
		must[ppdomain.ChargeCode](t)(ppdomain.NewChargeCode("REMOTE_AREA_SURCHARGE")), "SYN 偏远附加", ppdomain.ChargeEffectAdd,
		must[ppdomain.Money](t)(ppdomain.NewMoney(decimal(t, "5"), cny)), 1))
	return must[ppdomain.PricingPlanVersion](t)(ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(),
		base.Purpose(), base.BaseChargeCode(), base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(),
		[]ppdomain.FixedChargeRule{surcharge}, base.Structures()))
}

// sellEvaluation 真算一份评价；request 非空时评价回指它。
func sellEvaluation(t *testing.T, id, request string, plan ppdomain.PricingPlanVersion, zone string) ppdomain.PricingEvaluation {
	t.Helper()
	evaluationRequest := must[ppdomain.EvaluationRequest](t)(ppdomain.NewEvaluationRequest(
		must[ppdomain.EvaluationID](t)(ppdomain.NewEvaluationID(id)), plan, packageInput(t, "tenant-1", zone), ppdomain.EvidenceSynthetic))
	if request != "" {
		evaluationRequest = must[ppdomain.EvaluationRequest](t)(evaluationRequest.WithRequestReference(
			must[ppdomain.EvaluationRequestReference](t)(ppdomain.NewEvaluationRequestReference(request))))
	}
	return ppdomain.EvaluatePricing(evaluationRequest)
}

func newSellAdapter(t *testing.T, evaluations ...ppdomain.PricingEvaluation) *adapter.SellEvaluationAdapter {
	t.Helper()
	source := &evaluationSourceDouble{byID: map[string]ppdomain.PricingEvaluation{}}
	for _, evaluation := range evaluations {
		source.byID[evaluation.ID().String()] = evaluation
	}
	return must[*adapter.SellEvaluationAdapter](t)(adapter.NewSellEvaluationAdapter(source))
}

func loadSell(t *testing.T, view saports.SellEvaluationView, tenant, id string) (saports.SellEvaluationAdoption, bool, error) {
	t.Helper()
	return view.LoadSellEvaluation(t.Context(),
		must[sadomain.TenantID](t)(sadomain.NewTenantID(tenant)),
		must[sadomain.SellEvaluationReference](t)(sadomain.NewSellEvaluationReference(id)))
}

func chargeCodes(adoption saports.SellEvaluationAdoption) []string {
	var codes []string
	for _, code := range adoption.ChargeCodes {
		codes = append(codes, code.String())
	}
	return codes
}

// Covers: 已完成、声明了取整的 SELL 评价带回指与全部费用代码，金额按合计 0.01 换写（100 + 5 = 105 元 → 10500 分）。
func TestACompletedSellEvaluationCarriesItsRequestReferenceAndEveryChargeCode(t *testing.T) {
	evaluation := sellEvaluation(t, "eval-sell-1", "SELLREQ-SYN-1",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), "Z1")
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("夹具状态 = %s %#v", evaluation.Status(), evaluation.Issues())
	}
	adoption, found, err := loadSell(t, newSellAdapter(t, evaluation), "tenant-1", "eval-sell-1")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if adoption.Outcome != saports.SellEvaluationCompleted || adoption.Request.String() != "SELLREQ-SYN-1" {
		t.Fatalf("结果 = %s 回指 = %q", adoption.Outcome, adoption.Request)
	}
	if codes := chargeCodes(adoption); len(codes) != 2 || codes[0] != "BASE_FREIGHT" || codes[1] != "REMOTE_AREA_SURCHARGE" {
		t.Fatalf("费用代码 = %v，want 按行序 BASE_FREIGHT、REMOTE_AREA_SURCHARGE", codes)
	}
	if adoption.SettlementCurrency.String() != "CNY" || adoption.SettlementMinor != 10500 || adoption.OriginalMinor != 10500 {
		t.Fatalf("金额 = %s %d / %d，want CNY 10500", adoption.SettlementCurrency, adoption.SettlementMinor, adoption.OriginalMinor)
	}
}

// Covers: 没有回指的评价交回零值回指，由形成编排答「不是 SELL 请求」，读口不替它判。
func TestASellEvaluationWithoutARequestReferenceCarriesAZeroReference(t *testing.T) {
	evaluation := sellEvaluation(t, "eval-sell-2", "",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), "Z1")
	adoption, found, err := loadSell(t, newSellAdapter(t, evaluation), "tenant-1", "eval-sell-2")
	if err != nil || !found || adoption.Request.String() != "" || adoption.Outcome != saports.SellEvaluationCompleted {
		t.Fatalf("回指 = %q 结果 = %s found=%v err=%v", adoption.Request, adoption.Outcome, found, err)
	}
}

// Covers: 卡没声明金额取整——不报错、不补取整，交回「金额未声明」那一格、不带金额，回指与费用代码照带。
func TestAnUndeclaredAmountPrecisionIsAnOutcomeThatKeepsTheReference(t *testing.T) {
	evaluation := sellEvaluation(t, "eval-sell-3", "SELLREQ-SYN-3",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, false), "Z1")
	adoption, found, err := loadSell(t, newSellAdapter(t, evaluation), "tenant-1", "eval-sell-3")
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if adoption.Outcome != saports.SellEvaluationAmountUndeclared || adoption.Request.String() != "SELLREQ-SYN-3" || len(adoption.ChargeCodes) != 2 {
		t.Fatalf("结果 = %s 回指 = %q 代码 = %v", adoption.Outcome, adoption.Request, chargeCodes(adoption))
	}
	if adoption.SettlementMinor != 0 || adoption.OriginalMinor != 0 || adoption.SettlementCurrency.String() != "" {
		t.Fatalf("金额未声明却带了金额：%s %d / %d", adoption.SettlementCurrency, adoption.SettlementMinor, adoption.OriginalMinor)
	}
}

// Covers: 分区落空的评价是待判断，交回 PENDING、回指照带、不带费用代码；BUY 评价答端口上的哨兵；别的租户的评价不存在。
func TestTheSellViewBoundaries(t *testing.T) {
	pending := sellEvaluation(t, "eval-sell-4", "SELLREQ-SYN-4",
		sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true), "Z9")
	buy := sellEvaluation(t, "eval-buy-4", "EVREQ-SYN-4",
		sellCard(t, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, true), "Z1")
	view := newSellAdapter(t, pending, buy)

	adoption, found, err := loadSell(t, view, "tenant-1", "eval-sell-4")
	if err != nil || !found || adoption.Outcome != saports.SellEvaluationPending || adoption.Request.String() != "SELLREQ-SYN-4" || len(adoption.ChargeCodes) != 0 {
		t.Fatalf("分区落空：结果 = %s 代码 = %v found=%v err=%v", adoption.Outcome, chargeCodes(adoption), found, err)
	}
	if _, _, err := loadSell(t, view, "tenant-1", "eval-buy-4"); !errors.Is(err, saports.ErrNotASellEvaluation) || !errors.Is(err, adapter.ErrNotASellEvaluation) {
		t.Fatalf("BUY 评价 err = %v，want 端口上的 ErrNotASellEvaluation", err)
	}
	if _, found, err := loadSell(t, view, "tenant-2", "eval-sell-4"); err != nil || found {
		t.Fatalf("别的租户读到了评价：found=%v err=%v", found, err)
	}
}
