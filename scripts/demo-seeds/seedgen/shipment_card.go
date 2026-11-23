package main

// 逐委托售价卡 SYN-PLAN-CN-SG-SHIP-23（范围 SYN-SCOPE-SHIP-23，票 07）：只给新开的那一户用。
// 公共卡、专属卡、完整卡仍是逐包裹，不改它们的范围，也不把它们改成逐委托。
//
// 分区目录与燃油序列绑公共卡那一份（steps/150 已登记并复核）。Z1、Z2 都写进运费表，收件邮编落在目录里就能算出一票。
// 价与公共卡刻意不同（首重 40、续重 12；公共卡是 55、18），演示上分得出用的是哪一张。
// 全部取值是演示租户的合成采用值，证据只记 S，不是产品默认。

import "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

const shipmentScopeID = "SYN-SCOPE-SHIP-23"

func shipmentSellCardSnapshot() []byte {
	cny := currency("CNY")
	kilogram := domain.WeightUnitKilogram

	firstHalfKilo := weight("0.5", kilogram)
	east, err := domain.NewFirstContinueRate(
		rateEntryID("SYN-RATE-CN-SG-SHIP-Z1"), "Z1",
		firstHalfKilo, money("40", cny),
		firstHalfKilo, money("12", cny),
	)
	must("逐委托卡 Z1 首续费率", err)
	south, err := domain.NewFirstContinueRate(
		rateEntryID("SYN-RATE-CN-SG-SHIP-Z2"), "Z2",
		firstHalfKilo, money("36", cny),
		firstHalfKilo, money("10", cny),
	)
	must("逐委托卡 Z2 首续费率", err)
	table, err := domain.NewFirstContinueRateTable(
		reference(domain.ArtifactRateTable, "SYN-TABLE-CN-SG-SHIP-23", "v1"),
		cny, kilogram, period("2026-01-01"),
		[]domain.FirstContinueRate{east, south},
	)
	must("逐委托卡售价表", err)

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, firstHalfKilo)
	must("逐委托卡计费重进位", err)
	// 委托级输入只交合计实重，不交合计体积重。计费重取大时缺了体积重就停在缺尺寸，估价没有合计，委托接受不了。
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, "SYN-WEIGHT-CN-SG-SHIP-23", "v1"),
		domain.PricingWeightActualOnly, rounding, nil,
	)
	must("逐委托卡计费重策略", err)

	remote, err := domain.NewFixedChargeRule(
		"SYN-RULE-REMOTE-AREA", chargeCode("REMOTE_AREA_SURCHARGE"),
		"SYN 偏远区域附加（合成演示值）", domain.ChargeEffectAdd, money("5", cny), 1,
	)
	must("逐委托卡偏远附加规则", err)

	fuelBinding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "SYN-SERIES-FUEL-01")
	must("逐委托卡燃油序列绑定", err)
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{fuelBinding})
	must("逐委托卡方案结构", err)
	zoneLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	must("逐委托卡分区目录绑定", err)
	structures, err = structures.WithReferenceCatalogues(zoneLink)
	must("逐委托卡方案结构绑分区目录", err)
	amountRounding, err := domain.NewAmountRoundingPolicy(
		domain.RoundingHalfUp, money("0.01", cny), []domain.AmountRoundingPoint{domain.AmountRoundingTotal})
	must("逐委托卡金额取整", err)
	structures, err = structures.WithAmountRounding(amountRounding)
	must("逐委托卡方案结构声明金额取整", err)

	scope, err := domain.NewPricingScopeID(shipmentScopeID)
	must("逐委托计价范围", err)
	plan, err := domain.NewAggregatePricingPlanVersion(
		domain.AggregationPerShipment,
		reference(domain.ArtifactPricingPlan, "SYN-PLAN-CN-SG-SHIP-23", "v1"),
		scope, domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		chargeCode("BASE_FREIGHT"), period("2026-01-01"),
		table, weightPolicy, []domain.FixedChargeRule{remote}, structures,
	)
	must("逐委托售价方案", err)

	return marshalCard(plan, "SYN-CARD-CN-SG-SHIP-23.xlsx", "SYN-AUTH-PRICE-DIR-01")
}
