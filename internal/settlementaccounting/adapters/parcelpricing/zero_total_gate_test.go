package parcelpricing_test

import (
	"testing"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉 ADR-0306 的跨上下文那一段（票 demo-chain-gaps/133，复核报告 M12）：计价对扣减恰好扣到零照样落成已完成，结算经真
// SELL 读口与真形成编排把它记成`负向结束（零额）`、入账不重投，不再作两侧矛盾上抛。夹具全为 SYN 卡（S 级）。

// zeroTotalSellCard 在 sellCard 那张卡上再挂一条恰好扣掉全部金额的折扣：100 + 5 − 105 = 0。
func zeroTotalSellCard(t *testing.T) ppdomain.PricingPlanVersion {
	t.Helper()
	base := sellCard(t, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, true)
	cny := must[ppdomain.Currency](t)(ppdomain.NewCurrency("CNY"))
	discount := must[ppdomain.FixedChargeRule](t)(ppdomain.NewFixedChargeRule("SYN-RULE-PROMO",
		must[ppdomain.ChargeCode](t)(ppdomain.NewChargeCode("SYN_PROMO_DISCOUNT")), "SYN 全额促销", ppdomain.ChargeEffectDeduct,
		must[ppdomain.Money](t)(ppdomain.NewMoney(decimal(t, "105"), cny)), 2))
	return must[ppdomain.PricingPlanVersion](t)(ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(),
		base.Purpose(), base.BaseChargeCode(), base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(),
		append(base.Rules(), discount), base.Structures()))
}

// Covers: AT-SA-215 的 SELL 半边——计价真算出一份合计为零的已完成评价，门入账、不报错；请求记`负向结束（零额）`、不带停因，
// 费用库零写入；同一封再投一次仍入账、仍零写入。
func TestTheSellGateEndsTheRequestWithoutAChargeForAZeroTotal(t *testing.T) {
	evaluation := sellEvaluation(t, "eval-sell-zero", "SELLREQ-SYN-1", zeroTotalSellCard(t), "Z1")
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("扣到零的评价 = %s，want 已完成（计价只在合计小于零时答失败）", evaluation.Status())
	}
	if total, ok := evaluation.Total(); !ok || !total.Amount().IsZero() {
		t.Fatalf("合计 = %v %v，want 零", total, ok)
	}
	mappings := gateMappings{
		"BASE_FREIGHT": "SYN-FEE-FREIGHT", "REMOTE_AREA_SURCHARGE": "SYN-FEE-FREIGHT", "SYN_PROMO_DISCOUNT": "SYN-FEE-FREIGHT",
	}
	fixture := newGateFixture(t, mappings, evaluation)

	for _, round := range []string{"首投", "重投"} {
		if err := fixture.handle(t, "eval-sell-zero"); err != nil {
			t.Fatalf("%s：零额被当成了错误：%v", round, err)
		}
		request := fixture.request()
		if request.State != sadomain.SellRequestZeroTotal || request.StopCause != sadomain.EvaluationStopCauseInvalid ||
			request.FormedCharge.String() != "" {
			t.Fatalf("%s：请求册 = %s / %s / %q，want ZERO_TOTAL 不带停因与费用", round, request.State, request.StopCause, request.FormedCharge)
		}
		if len(fixture.charges.saved) != 0 {
			t.Fatalf("%s：零额建了 %d 笔费用", round, len(fixture.charges.saved))
		}
	}
}
