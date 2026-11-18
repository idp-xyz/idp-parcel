package main

// 专属售价卡 SYN-PLAN-CN-SG-EXCL-01（范围 SYN-SCOPE-EXCL-01，票 commercial-four-module-requirements/21）：客户合同把这个
// 范围排在公共范围 SYN-SCOPE-01 之前的那一位客户，先问这张卡（ADR-0380 决定一、三）。
//
// 只写谈下来的 Z1，没谈的 Z2 不写。分区目录与公共卡绑同一本 SYN-CAT-ZONE-CN-SG，Z1、Z2 都解得出：走 Z2 的包裹在这张
// 卡上是「基础运费表没有这一格」，评价落到合同声明的后一个范围；目录解不出分区是另一回事，停在这一格、不回退（ADR-0380
// 越权风险点 4）。
//
// 价与公共卡刻意不同，让同一件包裹在两张卡上的金额一眼分得开：首重 50、续重 16（公共卡是 55、18）。其余口径照公共卡——
// 同一条燃油序列、同样的偏远附加、体积系数、计费重进位与金额取整——差别只在价表这一处，演示上看到的差额就是谈下来的那一截。
// 全部取值是演示租户的合成采用值，证据只记 S，不是产品默认。

import "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

const exclusiveScopeID = "SYN-SCOPE-EXCL-01"

func exclusiveSellCardSnapshot() []byte {
	cny := currency("CNY")
	kilogram := domain.WeightUnitKilogram

	firstHalfKilo := weight("0.5", kilogram)
	east, err := domain.NewFirstContinueRate(
		rateEntryID("SYN-RATE-CN-SG-EXCL-Z1"), "Z1",
		firstHalfKilo, money("50", cny),
		firstHalfKilo, money("16", cny),
	)
	must("专属卡 Z1 首续费率", err)
	table, err := domain.NewFirstContinueRateTable(
		reference(domain.ArtifactRateTable, "SYN-TABLE-CN-SG-EXCL-01", "v1"),
		cny, kilogram, period("2026-01-01"),
		[]domain.FirstContinueRate{east},
	)
	must("专属卡售价表", err)

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, firstHalfKilo)
	must("专属卡计费重进位", err)
	factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.1", kilogram))
	must("专属卡体积重进位", err)
	factor, err := domain.NewVolumetricFactor(decimal("5000"), domain.LengthUnitCentimeter, factorRounding)
	must("专属卡体积系数", err)
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, "SYN-WEIGHT-CN-SG-EXCL-01", "v1"),
		domain.PricingWeightMax, rounding, &factor,
	)
	must("专属卡计费重策略", err)

	remote, err := domain.NewFixedChargeRule(
		"SYN-RULE-REMOTE-AREA", chargeCode("REMOTE_AREA_SURCHARGE"),
		"SYN 偏远区域附加（合成演示值）", domain.ChargeEffectAdd, money("5", cny), 1,
	)
	must("专属卡偏远附加规则", err)

	fuelBinding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "SYN-SERIES-FUEL-01")
	must("专属卡燃油序列绑定", err)
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{fuelBinding})
	must("专属卡方案结构", err)
	zoneLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	must("专属卡分区目录绑定", err)
	structures, err = structures.WithReferenceCatalogues(zoneLink)
	must("专属卡方案结构绑分区目录", err)
	amountRounding, err := domain.NewAmountRoundingPolicy(
		domain.RoundingHalfUp, money("0.01", cny), []domain.AmountRoundingPoint{domain.AmountRoundingTotal})
	must("专属卡金额取整", err)
	structures, err = structures.WithAmountRounding(amountRounding)
	must("专属卡方案结构声明金额取整", err)

	scope, err := domain.NewPricingScopeID(exclusiveScopeID)
	must("专属计价范围", err)
	plan, err := domain.NewPricingPlanVersion(
		reference(domain.ArtifactPricingPlan, "SYN-PLAN-CN-SG-EXCL-01", "v1"),
		scope, domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		chargeCode("BASE_FREIGHT"), period("2026-01-01"),
		table, weightPolicy, []domain.FixedChargeRule{remote}, structures,
	)
	must("专属售价方案", err)

	return marshalCard(plan, "SYN-CARD-CN-SG-EXCL-01.xlsx", "SYN-AUTH-PRICE-DIR-01")
}
