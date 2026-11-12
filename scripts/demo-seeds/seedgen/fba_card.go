package main

// FBA 头程售价卡 SYN-PLAN-CN-SG-FBA-01（范围 SYN-SCOPE-01，票 declared-goods-lines/17）：指名服务产品 SYN-PROD-CN-SG-FBA
// （ADR-0382 决定一），申报这个产品的委托由它计价，其余产品照旧由同一范围里不声明产品的 SYN-PLAN-CN-SG-01 计价。两张卡同在
// 一个计价范围，结算账户照旧只采用这一个范围（ADR-0280）。
//
// 价表与快递卡刻意不同，让两张卡算出的价一眼分得开：
//
//   - 按公斤单价、不收首重（计费重乘单价）：头程大箱按重量卖，没有小包那种首重一口价。
//   - 体积重按 6000 折算、计费重按整公斤进位：头程按箱的常见口径；快递卡是 5000、0.5 kg 进位。
//   - 没有固定附加费规则，也不绑燃油序列：演示只看基础运费分得开。
//
// 接受前估价与正式 SELL 评价都逐件评价（ADR-0382 越权风险点 8），这张卡因此是逐箱计价，每箱各算各的计费重——头程整票汇总
// 计费重的卖法今天写不进售价。带适用产品的卡写规范化版本 PPC-7，登记要部署开关（ADR-0352，见 steps/170）。
//
// 分区目录与快递卡绑同一本 SYN-CAT-ZONE-CN-SG（只绑标识，ADR-0109）：受理前估价对没绑分区目录的卡答未形成（ADR-0179
// 决定四）；金额取整同快递卡，结算由评价形成客户费用时要这一格（ADR-0190）。费用代码沿用 BASE_FREIGHT，结算的费目映射
// 按费用代码配，不必另登。全部单价与口径都是演示租户的合成采用值，证据只记 S，不是产品默认，也不是任何承运商的真实资费。

import "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

func fbaSellCardSnapshot() []byte {
	cny := currency("CNY")
	kilogram := domain.WeightUnitKilogram

	east, err := domain.NewUnitPriceRate(rateEntryID("SYN-RATE-CN-SG-FBA-Z1"), "Z1", money("24", cny))
	must("FBA 卡 Z1 单价", err)
	south, err := domain.NewUnitPriceRate(rateEntryID("SYN-RATE-CN-SG-FBA-Z2"), "Z2", money("21", cny))
	must("FBA 卡 Z2 单价", err)
	table, err := domain.NewUnitPriceRateTable(
		reference(domain.ArtifactRateTable, "SYN-TABLE-CN-SG-FBA-01", "v1"),
		cny, kilogram, period("2026-01-01"),
		[]domain.UnitPriceRate{east, south},
	)
	must("FBA 卡价表", err)

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("1", kilogram))
	must("FBA 卡计费重进位", err)
	factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.1", kilogram))
	must("FBA 卡体积重进位", err)
	factor, err := domain.NewVolumetricFactor(decimal("6000"), domain.LengthUnitCentimeter, factorRounding)
	must("FBA 卡体积系数", err)
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, "SYN-WEIGHT-CN-SG-FBA-01", "v1"),
		domain.PricingWeightMax, rounding, &factor,
	)
	must("FBA 卡计费重策略", err)

	structures, err := domain.NewPricingPlanStructures(nil, nil, nil)
	must("FBA 卡方案结构", err)
	zoneLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	must("FBA 卡分区目录绑定", err)
	structures, err = structures.WithReferenceCatalogues(zoneLink)
	must("FBA 卡方案结构绑分区目录", err)
	amountRounding, err := domain.NewAmountRoundingPolicy(
		domain.RoundingHalfUp, money("0.01", cny), []domain.AmountRoundingPoint{domain.AmountRoundingTotal})
	must("FBA 卡金额取整", err)
	structures, err = structures.WithAmountRounding(amountRounding)
	must("FBA 卡方案结构声明金额取整", err)

	plan, err := domain.NewPricingPlanVersion(
		reference(domain.ArtifactPricingPlan, "SYN-PLAN-CN-SG-FBA-01", "v1"),
		pricingScope(), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		chargeCode("BASE_FREIGHT"), period("2026-01-01"),
		table, weightPolicy, nil, structures,
	)
	must("FBA 卡售价方案", err)
	product, err := domain.NewServiceProductReference("SYN-PROD-CN-SG-FBA")
	must("FBA 卡适用服务产品", err)
	plan, err = plan.WithServiceProducts(product)
	must("FBA 卡声明适用服务产品", err)

	// 方向授权照演示的其余售价卡引 SYN-AUTH-PRICE-DIR-01。那份授权挂的价格规则 SYN-PRICE-RULE-CN-SG 正文指名的是
	// SYN-PLAN-CN-SG-01 与快递产品；计价只引授权、从不解析，登记只核它是一份商业授权引用，所以这里的引用背后没有一份指名
	// FBA 卡的价格规则——引得进来不等于授权覆盖了这张卡。
	return marshalCard(plan, "SYN-CARD-CN-SG-FBA-01.xlsx", "SYN-AUTH-PRICE-DIR-01")
}
