package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证 ADR-0260 决定一至三在计价侧的那一半：一张 SELL 卡的体积系数、金额取整与换算序列都只从卡上读，试算与正式
// 评价同一读法。评价的输入面上没有商业价格政策或已解析商业依据的位置，所以「政策上另有一份口径」在这里连递进来的
// 地方都没有；断言落在它留下的痕迹上——版本清单里只有卡、卡的各件与卡绑定的序列。取值全是 SYN，证据只记 S。

// sellCardWithOwnTerms 是一张 SELL·客户费用卡：Z1 两档（0–2 KG 10 USD、2–10 KG 20 USD），MAX 计费重、体积系数除数
// 5000（立方厘米 → 千克，体积重进位 0.1 KG），计费重进位 0.5 KG；合计 HALF_UP 到 0.01；绑汇率序列 SYN-FX-USD-CNY。
func sellCardWithOwnTerms(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	currency := mustValue(t, domain.NewCurrency, "USD")
	kg := func(raw string) domain.Weight {
		weight, err := domain.NewWeightFromString(raw, domain.WeightUnitKilogram)
		if err != nil {
			t.Fatalf("weight %s: %v", raw, err)
		}
		return weight
	}
	usd := func(raw string) domain.Money {
		amount, err := domain.NewMoneyFromString(raw, currency)
		if err != nil {
			t.Fatalf("money %s: %v", raw, err)
		}
		return amount
	}
	reference := func(kind domain.ArtifactKind, id string) domain.VersionReference {
		built, err := domain.NewVersionReference(kind, id, "v1", "sha256:"+id)
		if err != nil {
			t.Fatalf("reference %s: %v", id, err)
		}
		return built
	}
	light, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "sell-terms-light"), "Z1", kg("0"), kg("2"), usd("10"))
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	heavy, err := domain.NewRateEntry(mustValue(t, domain.NewRateEntryID, "sell-terms-heavy"), "Z1", kg("2"), kg("10"), usd("20"))
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	base := minimalPlan(t)
	table, err := domain.NewRateTableVersion(
		reference(domain.ArtifactRateTable, "SYN-TABLE-SELL-TERMS"), domain.RateTableFamilyWeightZone, currency,
		domain.WeightUnitKilogram, base.RateTable().EffectivePeriod(), []domain.RateEntry{light, heavy})
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	volumetricRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, kg("0.1"))
	if err != nil {
		t.Fatalf("volumetric rounding: %v", err)
	}
	factor, err := domain.NewVolumetricFactor(mustValue(t, domain.ParseDecimal, "5000"), domain.LengthUnitCentimeter, volumetricRounding)
	if err != nil {
		t.Fatalf("volumetric factor: %v", err)
	}
	weightRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, kg("0.5"))
	if err != nil {
		t.Fatalf("weight rounding: %v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, "SYN-WEIGHT-SELL-TERMS"), domain.PricingWeightMax, weightRounding, &factor)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, "SYN-FX-USD-CNY")
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	rounding, err := domain.NewAmountRoundingPolicy(domain.RoundingHalfUp, usd("0.01"), []domain.AmountRoundingPoint{domain.AmountRoundingTotal})
	if err != nil {
		t.Fatalf("amount rounding: %v", err)
	}
	if structures, err = structures.WithAmountRounding(rounding); err != nil {
		t.Fatalf("with amount rounding: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		reference(domain.ArtifactPricingPlan, "SYN-PLAN-SELL-TERMS"), base.Scope(),
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, base.BaseChargeCode(),
		base.EffectivePeriod(), table, weightPolicy, nil, structures,
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

// sellTermsSides 是 40×30×20 CM：按卡上的除数 5000 得体积重 4.8 KG，大于实重 1 KG。
func sellTermsSides(t *testing.T) domain.Dimensions {
	t.Helper()
	sides, err := domain.NewDimensions(
		mustValue(t, domain.ParseDecimal, "40"), mustValue(t, domain.ParseDecimal, "30"),
		mustValue(t, domain.ParseDecimal, "20"), domain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("dimensions: %v", err)
	}
	return sides
}

// assertSellCardTerms 是两条路共同的期望：计费重 5 KG（体积重 4.8 → 进位 0.5），落 2–10 KG 那一档 20 USD；按在用版本 v2 的
// 牌价 7.23456 换成 144.6912 CNY，再按卡上的 HALF_UP 0.01 合计成 144.69 CNY；版本清单里没有商业政策或商业授权的引用，
// 只有卡、卡的各件与卡绑定的那串序列。
func assertSellCardTerms(t *testing.T, path string, evaluation domain.PricingEvaluation) {
	t.Helper()
	if evaluation.Status() != domain.EvaluationCompleted || evaluation.Direction() != domain.PricingDirectionSell {
		t.Fatalf("%s: status = %s direction = %s issues = %#v", path, evaluation.Status(), evaluation.Direction(), evaluation.Issues())
	}
	weight, ok := evaluation.PricingWeight()
	if !ok {
		t.Fatalf("%s: no pricing weight", path)
	}
	volumetric, derived := weight.VolumetricWeight()
	if !derived || volumetric.Value().String() != "4.8" {
		t.Fatalf("%s: volumetric = %s derived=%v, want 4.8 KG from the card's divisor 5000", path, volumetric.Value(), derived)
	}
	if rate, matched := evaluation.MatchedRate(); !matched || rate.Amount().Amount().String() != "20" {
		t.Fatalf("%s: matched rate = %#v, want the 2–10 KG band (20 USD) reached through the volumetric weight", path, rate)
	}
	step, converted := evaluation.ConversionStep()
	if !converted || step.SeriesReference().Version() != "v2" || step.Rate().String() != "7.23456" {
		t.Fatalf("%s: conversion = %#v, want the bound series' in-force v2 at 7.23456", path, step)
	}
	total, _ := evaluation.Total()
	if total.Currency().String() != "CNY" || total.Amount().String() != "144.69" {
		t.Fatalf("%s: total = %s %s, want 144.69 CNY（144.6912 按卡上的 HALF_UP 到 0.01）", path, total.Amount(), total.Currency())
	}
	if rounded := evaluation.AmountRounding(); len(rounded) != 1 || rounded[0].Point() != domain.AmountRoundingTotal {
		t.Fatalf("%s: amount rounding = %#v, want the card's TOTAL rounding only", path, rounded)
	}
	cited := map[domain.ArtifactKind]string{}
	for _, reference := range evaluation.Manifest().References() {
		switch reference.Kind() {
		case domain.ArtifactCommercialPolicy, domain.ArtifactCommercialAuthorization:
			t.Fatalf("%s: manifest cites %s %s——评价不引用商业价格政策或已解析商业依据", path, reference.Kind(), reference.ID())
		}
		cited[reference.Kind()] = reference.ID() + "@" + reference.Version()
	}
	if cited[domain.ArtifactPricingPlan] != "SYN-PLAN-SELL-TERMS@v1" || cited[domain.ArtifactReferenceSeries] != "SYN-FX-USD-CNY@v2" {
		t.Fatalf("%s: manifest = %v, want the card and its bound series' in-force version", path, cited)
	}
}

// Covers: AT-PP-013 的编排半边——一张在用 SELL 卡声明了自己的体积系数与金额取整、绑了一串汇率序列：正式评价与试算都按
// 卡上的除数算体积重、按卡上的策略取整、用卡绑定序列的在用版本按计价基准时点换算；版本清单里没有任何商业依据引用
// （ADR-0260 决定一至三）。前提里「同范围的销售方向价格政策另有一份口径」在真库上构造，见 cmd/parcel-api 的
// TestAnEstimateReadsOnlyTheCardWhileASellPolicyDeclaresOtherwiseAgainstARealDatabase。
func TestASellCardIsPricedOnlyOnItsOwnTermsOnBothPaths(t *testing.T) {
	plan := sellCardWithOwnTerms(t)
	readings := map[string]domain.ResolvedSeriesReading{"v2": fxReading(t, "v2", "7.23456")}

	t.Run("formal evaluation", func(t *testing.T) {
		input, err := domain.NewPricingInputSnapshot(
			mustValue(t, domain.NewTenantID, "tenant-1"), plan.Scope(), packageSubjectOf(t, "SYN-PARCEL-SELL-TERMS"),
			"Z1", mustWeight(t, "1"), ptr(sellTermsSides(t)), minimalInput(t, "Z1").BusinessAt())
		if err != nil {
			t.Fatalf("input: %v", err)
		}
		if input, err = input.WithSettlementCurrency(mustValue(t, domain.NewCurrency, "CNY")); err != nil {
			t.Fatalf("settlement currency: %v", err)
		}
		request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, "eval-sell-terms"), plan, input, domain.EvidenceSynthetic)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		inForce := &inForceDouble{reference: fxReference(t, "v2"), outcome: ports.SeriesVersionInForce}
		register := &seriesReadingDouble{readings: readings}
		handler := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
			Store:          &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
			Downstream:     &evaluationDownstreamDouble{},
			Clock:          fixedClock{at: formedAt},
			InForce:        inForce,
			SeriesVersions: register,
		})
		result, err := handler.Handle(context.Background(), application.EvaluatePricingCommand{Request: request})
		if err != nil || result.Outcome() != application.EvaluationRecorded {
			t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
		}
		evaluation, _ := result.Evaluation()
		assertSellCardTerms(t, "formal", evaluation)
		if inForce.lastID != "SYN-FX-USD-CNY" || !inForce.lastAt.Equal(formedAt) || !register.lastAsOf.Equal(input.BusinessAt()) {
			t.Fatalf("series asked as %s at %s, period at %s; want the card's binding, the forming moment, the pricing basis time",
				inForce.lastID, inForce.lastAt, register.lastAsOf)
		}
	})

	t.Run("estimate", func(t *testing.T) {
		inForce := &inForceDouble{reference: fxReference(t, "v2"), outcome: ports.SeriesVersionInForce}
		register := &seriesReadingDouble{readings: readings}
		handler, err := application.NewFormEstimateEvaluationsHandler(application.FormEstimateEvaluationsDeps{
			PriceCards:     &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{plan}},
			InForce:        &priceCardResolverDouble{},
			Clock:          fixedClock{at: estimateBasisAt},
			SeriesInForce:  inForce,
			SeriesVersions: register,
		})
		if err != nil {
			t.Fatalf("handler: %v", err)
		}
		command := estimateCommand(t)
		command.Dimensions = ptr(sellTermsSides(t))
		command.Settlement = ptr(mustValue(t, domain.NewCurrency, "CNY"))
		result, err := handler.Handle(context.Background(), command)
		if err != nil || result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
			t.Fatalf("result = %+v err = %v", result, err)
		}
		assertSellCardTerms(t, "estimate", result.Candidates[0].Evaluation)
		if inForce.lastID != "SYN-FX-USD-CNY" || !register.lastAsOf.Equal(estimateBasisAt) {
			t.Fatalf("series asked as %s, period at %s; want the card's binding at the declared basis time", inForce.lastID, register.lastAsOf)
		}
	})
}

func packageSubjectOf(t *testing.T, id string) domain.EvaluationSubject {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, id))
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	return subject
}

func ptr[T any](value T) *T { return &value }
