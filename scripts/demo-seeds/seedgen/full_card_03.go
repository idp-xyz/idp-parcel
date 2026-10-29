package main

// 完整售价卡的第三张合成种子 SYN-PLAN-CN-SG-FULL-03（范围 SYN-SCOPE-04）：基础运费、燃油、超尺寸、超重照
// SYN-PLAN-CN-SG-FULL-01 抄，其上演示两处新机制（ADR-0348）：
//
//   - 偏远地区费改成**一条规则一个费用代码**，按偏远档位查附加费价表：T1 收 12、T2 收 28，T0（市区）触发条件不成立、
//     不收；FULL-01 因附加费价表读不到偏远档位，只能一档一条规则、各占一个费用代码（REMOTE_AREA_SURCHARGE 与
//     REMOTE_AREA_EXTENDED_SURCHARGE）。
//   - 单票合计最低收费 120.99 CNY。这个下限取 FULL-01 小件市区一票（2 kg）的合计——基础运费 109 加燃油 11.99——
//     好让「一票刚好等于下限」有一票真实可造的包裹；基础价最低的一档（0.5 kg）合计也有 61.05，造不出低于 30 的合计，
//     所以不用 30。它落在金额取整进位单位（0.01）的整数倍上，是卡声明金额取整时的必要条件。最高收费不加：
//     机制随产品发布并有单元用例，演示卡只演示最低收费。
//
// 它与 FULL-01 并存、互不改动：放在自己的计价范围，评价按范围选卡，FULL-01 的评价结果因此不变。分区目录、偏远档位
// 目录与燃油序列绑 FULL-01 那一节（steps/160）已登记并复核的同一批，不另登。卡上的所有金额、门槛与费率系数都是演示租户
// 的合成采用值，证据只记 S，不是产品默认，也不是任何承运商的真实资费。

import "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

const (
	// full03ScopeID 是第三张完整价卡独占的计价范围，不与 SYN-SCOPE-01、SYN-SCOPE-02 重叠。
	full03ScopeID = "SYN-SCOPE-04"
)

func fullSellCard03Snapshot() []byte {
	cny := currency("CNY")
	kilogram := domain.WeightUnitKilogram

	firstHalfKilo := weight("0.5", kilogram)
	east, err := domain.NewFirstContinueRate(
		rateEntryID("SYN-RATE-CN-SG-FULL3-Z1"), "Z1",
		firstHalfKilo, money("55", cny),
		firstHalfKilo, money("18", cny),
	)
	must("完整卡三 Z1 首续费率", err)
	south, err := domain.NewFirstContinueRate(
		rateEntryID("SYN-RATE-CN-SG-FULL3-Z2"), "Z2",
		firstHalfKilo, money("48", cny),
		firstHalfKilo, money("15", cny),
	)
	must("完整卡三 Z2 首续费率", err)
	table, err := domain.NewFirstContinueRateTable(
		reference(domain.ArtifactRateTable, "SYN-TABLE-CN-SG-FULL-03", "v1"),
		cny, kilogram, period("2026-01-01"),
		[]domain.FirstContinueRate{east, south},
	)
	must("完整卡三售价表", err)

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, firstHalfKilo)
	must("完整卡三计费重进位", err)
	factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.1", kilogram))
	must("完整卡三体积重进位", err)
	factor, err := domain.NewVolumetricFactor(decimal("5000"), domain.LengthUnitCentimeter, factorRounding)
	must("完整卡三体积系数", err)
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, "SYN-WEIGHT-CN-SG-FULL-03", "v1"),
		domain.PricingWeightMax, rounding, &factor,
	)
	must("完整卡三计费重策略", err)

	trigger := func(condition domain.FeatureCondition, conditionErr error) domain.TriggerCondition {
		must("触发条件", conditionErr)
		value, triggerErr := domain.NewTrigger(condition)
		must("触发条件包装", triggerErr)
		return value
	}
	lengthOver := func(source domain.FeatureSource, centimeters string) domain.TriggerCondition {
		threshold, lengthErr := domain.NewLength(decimal(centimeters), domain.LengthUnitCentimeter)
		must("长度阈值 "+centimeters, lengthErr)
		return trigger(domain.NewLengthFeatureCondition(source, domain.ComparisonGreaterThan, threshold))
	}
	actualOver := func(kilograms string) domain.TriggerCondition {
		return trigger(domain.NewWeightFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThan, weight(kilograms, kilogram)))
	}
	tierIs := func(tier string) domain.TriggerCondition {
		return trigger(domain.NewCategoryFeatureCondition(domain.FeatureAddressType, domain.CategoryValue(tier)))
	}
	fixed := func(amount string) domain.SurchargeCalculation {
		value, fixedErr := domain.NewFixedAmountSurcharge(money(amount, cny))
		must("固定金额 "+amount, fixedErr)
		return value
	}

	// 燃油对每票都收，触发条件没有「无条件」的写法，所以写实重大于 0（同 FULL-01）。
	fuelCalculation, err := domain.NewSeriesRateSurcharge(domain.ReferenceSeriesFuelRate, decimal("1"), "SYN-DEP-FULL3-FUEL")
	must("完整卡三燃油费率计算", err)
	fuel, err := domain.NewSurchargeRule("SYN-RULE-FULL3-FUEL", chargeCode("FUEL_SURCHARGE"),
		"燃油附加费（合成演示值）", domain.ChargeEffectAdd, actualOver("0"), fuelCalculation)
	must("完整卡三燃油规则", err)
	fuel, err = fuel.Standalone()
	must("完整卡三燃油并列计收", err)

	// 偏远地区费：一条规则、一个费用代码，金额按偏远档位查价表。价表条目的类别键是档位，每档一条从 0 起的开口重量段
	// （金额与重量无关）。触发条件决定哪些档位收费，价表决定各档收多少：T0 不在触发条件里，所以市区不收。
	remoteTable := func() domain.RateTableVersion {
		entries := make([]domain.RateEntry, 0, 2)
		for _, row := range []struct{ tier, amount string }{{"T1", "12"}, {"T2", "28"}} {
			entry, entryErr := domain.NewOpenEndedRateEntry(
				rateEntryID("SYN-RATE-CN-SG-FULL3-REMOTE-"+row.tier), row.tier, weight("0", kilogram), money(row.amount, cny))
			must("偏远档位条目 "+row.tier, entryErr)
			entries = append(entries, entry)
		}
		built, tableErr := domain.NewRateTableVersion(
			reference(domain.ArtifactRateTable, "SYN-TABLE-CN-SG-FULL-03-REMOTE", "v1"),
			domain.RateTableFamilyWeightZone, cny, kilogram, period("2026-01-01"), entries)
		must("偏远档位价表", tableErr)
		return built
	}()
	remoteCalculation, err := domain.NewRemoteTierTableLookupSurcharge(remoteTable)
	must("偏远档位查表计算", err)
	remoteTrigger, err := domain.NewAnyOfTrigger(tierIs("T1"), tierIs("T2"))
	must("偏远触发条件", err)
	remote, err := domain.NewSurchargeRule("SYN-RULE-FULL3-REMOTE", chargeCode("REMOTE_AREA_SURCHARGE"),
		"偏远地区费（合成演示值）", domain.ChargeEffectAdd, remoteTrigger, remoteCalculation)
	must("偏远规则", err)
	remote, err = remote.Standalone()
	must("偏远并列计收", err)

	oversizeTrigger, err := domain.NewAnyOfTrigger(
		lengthOver(domain.FeatureLongestSide, "100"),
		lengthOver(domain.FeatureLengthAndGirth, "300"),
	)
	must("完整卡三超尺寸触发", err)
	oversize, err := domain.NewSurchargeRule("SYN-RULE-FULL3-OVERSIZE", chargeCode("OVERSIZE_SURCHARGE"),
		"超尺寸费（合成演示值）", domain.ChargeEffectAdd, oversizeTrigger, fixed("35"))
	must("完整卡三超尺寸规则", err)
	oversize, err = oversize.InExclusivityGroup("SYN-GROUP-FULL3-OVERSIZE-OVERWEIGHT", 1)
	must("完整卡三超尺寸互斥组", err)
	overweight, err := domain.NewSurchargeRule("SYN-RULE-FULL3-OVERWEIGHT", chargeCode("OVERWEIGHT_SURCHARGE"),
		"超重费（合成演示值）", domain.ChargeEffectAdd, actualOver("20"), fixed("25"))
	must("完整卡三超重规则", err)
	overweight, err = overweight.InExclusivityGroup("SYN-GROUP-FULL3-OVERSIZE-OVERWEIGHT", 1)
	must("完整卡三超重互斥组", err)

	fuelBasis, err := domain.NewListedChargeDependency("SYN-DEP-FULL3-FUEL", chargeCode("FUEL_SURCHARGE"),
		[]domain.ChargeCode{chargeCode("BASE_FREIGHT")}, nil)
	must("完整卡三燃油基数", err)
	fuelBinding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, fullFuelSeriesID)
	must("完整卡三燃油序列绑定", err)
	structures, err := domain.NewPricingPlanStructures(
		[]domain.SurchargeRule{fuel, remote, oversize, overweight},
		[]domain.ChargeDependency{fuelBasis},
		[]domain.ReferenceSeriesBinding{fuelBinding},
	)
	must("完整卡三方案结构", err)

	zoneLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, fullZoneCatalogueID)
	must("完整卡三分区目录绑定", err)
	tierLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindRemoteTier, fullTierCatalogueID)
	must("完整卡三偏远档位目录绑定", err)
	structures, err = structures.WithReferenceCatalogues(zoneLink, tierLink)
	must("完整卡三方案结构绑目录", err)
	amountRounding, err := domain.NewAmountRoundingPolicy(
		domain.RoundingHalfUp, money("0.01", cny),
		[]domain.AmountRoundingPoint{domain.AmountRoundingPerLine, domain.AmountRoundingTotal})
	must("完整卡三金额取整", err)
	structures, err = structures.WithAmountRounding(amountRounding)
	must("完整卡三方案结构声明金额取整", err)

	minimum := money("120.99", cny)
	bounds, err := domain.NewTotalChargeBounds(&minimum, nil)
	must("完整卡三总额下限", err)
	structures, err = structures.WithTotalChargeBounds(bounds)
	must("完整卡三方案结构声明总额下限", err)

	scope, err := domain.NewPricingScopeID(full03ScopeID)
	must("完整卡三计价范围", err)
	plan, err := domain.NewPricingPlanVersion(
		reference(domain.ArtifactPricingPlan, "SYN-PLAN-CN-SG-FULL-03", "v1"),
		scope, domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		chargeCode("BASE_FREIGHT"), period("2026-01-01"),
		table, weightPolicy, nil, structures,
	)
	must("完整售价方案三", err)

	return marshalCard(plan, "SYN-CARD-CN-SG-FULL-03.xlsx", "SYN-AUTH-PRICE-DIR-01")
}
