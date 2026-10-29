package main

// 完整售价卡的合成种子：演示租户在基础运费之外带燃油、偏远、超尺寸、超重四项附加费的第二张 SELL 价卡，以及在它之上
// 超重费改按计价重量触发、另带住宅派送费的第三张（fullNatureSpec）。
//
// 它们与 sellCardSnapshot 那张 v1 并存而不顶替它：演示动线（提交、受理前估价、费用确认、对账）的客户费用金额
// 由 v1 产生，这几张卡各放在自己的计价范围（fullScopeID、fullNatureScopeID），评价按范围选卡，不选到它们，既有动线的
// 评价结果因此不变。
// 卡上的所有金额、门槛与费率系数都是演示租户的合成采用值，证据只记 S，不是产品默认，也不是任何承运商的真实资费。

import "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

const (
	// fullScopeID 是完整价卡独占的计价范围，不与 scopeID 重叠：试算页按这个范围查，只会看到这一张卡。
	fullScopeID = "SYN-SCOPE-02"
	// fullNatureScopeID 是第二张完整价卡独占的计价范围：它与 FULL-01 的金额、门槛与目录绑定逐项相同，只在超重费与住宅派送费上
	// 不同，放在同一个范围里试算页会并排出两张近似的卡；各占一个范围，各自的试算结果一眼分得开。
	fullNatureScopeID = "SYN-SCOPE-03"

	fullZoneCatalogueID = "SYN-CAT-ZONE-CN-SG-FULL"
	fullTierCatalogueID = "SYN-CAT-REMOTE-TIER-CN-SG"
	fullFuelSeriesID    = "SYN-SERIES-FUEL-FULL-01"
)

// fullFuelSeriesSnapshot 折装燃油费率序列 SYN-SERIES-FUEL-FULL-01 v1：取值是**百分点**（12 即 12%），三期与
// SYN-SERIES-FUEL-01 同一组日期。引擎按「基数 × 费率 × 卡上折扣系数 ÷ 100」算燃油，所以序列取值必须是百分点；
// 150 一节那条 SYN-SERIES-FUEL-01 登的是 0.12、0.135、0.11，读作百分点是 0.12%，从来没有哪条规则用过它而没被发现。
// 它已被 v1 售价卡绑定并复核在用，改它会动到 v1 评价清单里冻结的序列版本，所以这张卡另绑自己的一条而不去改那条。
func fullFuelSeriesSnapshot() []byte {
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant(),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        reference(domain.ArtifactReferenceSeries, fullFuelSeriesID, "v1"),
		SourceIdentifier: "SYN-CARRIER-FUEL-BULLETIN-FULL",
		Registrant:       registrant,
		Periods: []domain.SeriesPeriodValue{
			seriesPeriod("2026-01-01", "2026-04-01", "12", "SYN-EVID-FUEL-FULL-2601"),
			seriesPeriod("2026-04-01", "2026-07-01", "13.5", "SYN-EVID-FUEL-FULL-2604"),
			seriesPeriod("2026-07-01", "", "11", "SYN-EVID-FUEL-FULL-2607"),
		},
	})
	must("完整卡燃油序列登记", err)
	raw, err := domain.MarshalReferenceSeriesRegistration(registration)
	must("完整卡燃油序列折装", err)
	return raw
}

// fullZoneCatalogueSnapshot 折装分区目录 SYN-CAT-ZONE-CN-SG-FULL v1。分区目录 SYN-CAT-ZONE-CN-SG 只覆盖 018 与 238 两个
// 目的前缀，而偏远档位目录里的偏远前缀（098、627、638）也得有分区才算得出基础运费，所以这张卡绑一本覆盖五个前缀的新目录，
// 不去改那本已被 v1 价卡与成本卡绑住的目录。始发覆盖与 018 / 238 的分区照旧同值，同一目的邮编在两张卡上落同一个分区。
func fullZoneCatalogueSnapshot() []byte {
	origin, err := domain.NewPostalPrefixCatalogueOrigin([]string{"200", "510"})
	must("完整卡分区目录始发", err)
	entries := make([]domain.CatalogueEntry, 0, 5)
	for _, row := range []struct{ prefix, zone string }{
		{"018", "Z1"},
		{"098", "Z1"},
		{"238", "Z2"},
		{"627", "Z1"},
		{"638", "Z1"},
	} {
		entry, entryErr := domain.NewCatalogueEntry(row.prefix, domain.CategoryValue(row.zone))
		must("完整卡分区条目 "+row.prefix, entryErr)
		entries = append(entries, entry)
	}
	registration, err := domain.NewReferenceCatalogueRegistration(domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           tenant(),
		Kind:             domain.CatalogueKindZone,
		Reference:        reference(domain.ArtifactReferenceCatalogue, fullZoneCatalogueID, "v1"),
		SourceIdentifier: "SYN-CARRIER-ZONE-BULLETIN-FULL",
		Registrant:       registrant,
		Origin:           origin,
		PrefixLength:     3,
		Entries:          entries,
		Period:           period("2026-01-01"),
	})
	must("完整卡分区目录登记", err)
	raw, err := domain.MarshalReferenceCatalogueRegistration(registration)
	must("完整卡分区目录折装", err)
	return raw
}

// remoteTierCatalogueSnapshot 折装偏远档位目录 SYN-CAT-REMOTE-TIER-CN-SG v1。偏远档位表不区分始发（独立始发维度）；
// 前缀粒度 3 位，与分区目录同。
//
// **市区前缀也要登**，登成 T0（非偏远）：绑了档位目录的卡，目的邮编查不到条目一律落 REMOTE_TIER_UNRESOLVED 待判断，
// 不当作「不偏远」（ADR-0109 决定四：查不到即待判断，不给默认）。T0 没有任何规则按它收费，命中即不收，
// 与「没登」是两个答案。新加坡的合成前缀：Sentosa 098、大士 638 为一档（T1），裕廊岛 627 为二档（T2）。
func remoteTierCatalogueSnapshot() []byte {
	entries := make([]domain.CatalogueEntry, 0, 5)
	for _, row := range []struct{ prefix, tier string }{
		{"018", "T0"},
		{"098", "T1"},
		{"238", "T0"},
		{"627", "T2"},
		{"638", "T1"},
	} {
		entry, err := domain.NewCatalogueEntry(row.prefix, domain.CategoryValue(row.tier))
		must("偏远档位条目 "+row.prefix, err)
		entries = append(entries, entry)
	}
	registration, err := domain.NewReferenceCatalogueRegistration(domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           tenant(),
		Kind:             domain.CatalogueKindRemoteTier,
		Reference:        reference(domain.ArtifactReferenceCatalogue, fullTierCatalogueID, "v1"),
		SourceIdentifier: "SYN-CARRIER-REMOTE-TIER-BULLETIN",
		Registrant:       registrant,
		Origin:           domain.NewIndependentCatalogueOrigin(),
		PrefixLength:     3,
		Entries:          entries,
		Period:           period("2026-01-01"),
	})
	must("偏远档位目录登记", err)
	raw, err := domain.MarshalReferenceCatalogueRegistration(registration)
	must("偏远档位目录折装", err)
	return raw
}

// fullCardSpec 是两张完整售价卡之间唯一不同的几格：卡与价表、重量策略、费率行的标识，计价范围，登记用的源文件名，
// 以及两项随引擎能力而来的差别——超重费按哪个特征触发、有没有住宅派送费。其余一切（基础运费、体积重、燃油、偏远、超尺寸、
// 金额取整、两本目录与燃油序列的绑定）两张卡逐字相同，所以共用 buildFullSellCard 一份构造，而不是复制一遍。
type fullCardSpec struct {
	planID         string
	tableID        string
	weightPolicyID string
	// rateIDPrefix 后接 -Z1 / -Z2 即两条费率行的标识。
	rateIDPrefix   string
	scope          string
	sourceFileName string
	// overweightOnChargeable 为真时超重费读自身计价重量（CHARGEABLE_WEIGHT），否则读实重。
	overweightOnChargeable bool
	// residentialDelivery 为真时多一条住宅派送费：收件地址性质为住宅则加收 6 CNY，与偏远档位是两个独立维度，同时计收。
	residentialDelivery bool
}

// fullCardOne 是 SYN-PLAN-CN-SG-FULL-01 的几格。
var fullCardOne = fullCardSpec{
	planID:         "SYN-PLAN-CN-SG-FULL-01",
	tableID:        "SYN-TABLE-CN-SG-FULL-01",
	weightPolicyID: "SYN-WEIGHT-CN-SG-FULL-01",
	rateIDPrefix:   "SYN-RATE-CN-SG-FULL",
	scope:          fullScopeID,
	sourceFileName: "SYN-CARD-CN-SG-FULL-01.xlsx",
}

// fullSellCardSnapshot 折装完整售价卡 SYN-PLAN-CN-SG-FULL-01 v1（范围 SYN-SCOPE-02，SELL / CUSTOMER_CHARGE，CNY，KG，
// 每件计收）。基础运费的首重续重价与体积重规则抄 v1，便于两张卡并排对比；其上带四项附加费：
//
//   - 燃油附加费：基础运费乘燃油序列 SYN-SERIES-FUEL-FULL-01 当期费率，卡上折扣系数 1（即照公布费率收）。
//   - 偏远地区费：按偏远档位收。这张卡成形时附加费价表（TABLE_LOOKUP）只按基础运费的分区与计价重量查档，读不到偏远
//     档位，所以档位走触发条件（地址类型特征承载偏远档位，ADR-0109），一档一条规则、一档一个费用代码——一张卡里同一费用
//     代码只能出现一条附加费规则。两档同属一个互斥组，一票至多收一档。按档位查表的一条规则写法见 full_card_03.go。
//   - 超尺寸费：最长边超过 100 厘米，或长加围超过 300 厘米，任一成立收一次。
//   - 超重费：实重超过 20 千克。这一张读实重——落成之时评价器还不把体积重与计价重量填进判定特征；之后补上了
//     （ADR-0347），按计价重量收超重费的写法见 SYN-PLAN-CN-SG-FULL-02，本卡一字不动。
//
// 超尺寸与超重同一互斥组、同一优先级，组内同级取金额较高者；两者并存时收超尺寸（35 > 25），超重留痕不计收。
// 金额取整逐行加合计：每一行先取整到分，合计是各行之和，页面上看到的各行加起来就是合计。
//
// 住宅派送费与最低收费这一张都没有做：落成之时计价输入快照不带地址性质，引擎也没有对单票合计设下限的结构。地址性质
// 之后有了输入与独立的特征槽（ADR-0347），住宅派送费见 SYN-PLAN-CN-SG-FULL-02；总额下限现已有，演示见 full_card_03.go。
func fullSellCardSnapshot() []byte {
	return buildFullSellCard(fullCardOne)
}

func buildFullSellCard(spec fullCardSpec) []byte {
	cny := currency("CNY")
	kilogram := domain.WeightUnitKilogram

	firstHalfKilo := weight("0.5", kilogram)
	east, err := domain.NewFirstContinueRate(
		rateEntryID(spec.rateIDPrefix+"-Z1"), "Z1",
		firstHalfKilo, money("55", cny),
		firstHalfKilo, money("18", cny),
	)
	must("完整卡 Z1 首续费率", err)
	south, err := domain.NewFirstContinueRate(
		rateEntryID(spec.rateIDPrefix+"-Z2"), "Z2",
		firstHalfKilo, money("48", cny),
		firstHalfKilo, money("15", cny),
	)
	must("完整卡 Z2 首续费率", err)
	table, err := domain.NewFirstContinueRateTable(
		reference(domain.ArtifactRateTable, spec.tableID, "v1"),
		cny, kilogram, period("2026-01-01"),
		[]domain.FirstContinueRate{east, south},
	)
	must("完整卡售价表", err)

	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, firstHalfKilo)
	must("完整卡计费重进位", err)
	factorRounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.1", kilogram))
	must("完整卡体积重进位", err)
	factor, err := domain.NewVolumetricFactor(decimal("5000"), domain.LengthUnitCentimeter, factorRounding)
	must("完整卡体积系数", err)
	weightPolicy, err := domain.NewPricingWeightPolicy(
		reference(domain.ArtifactWeightPolicy, spec.weightPolicyID, "v1"),
		domain.PricingWeightMax, rounding, &factor,
	)
	must("完整卡计费重策略", err)

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
	chargeableOver := func(kilograms string) domain.TriggerCondition {
		return trigger(domain.NewWeightFeatureCondition(domain.FeatureChargeableWeight, domain.ComparisonGreaterThan, weight(kilograms, kilogram)))
	}
	tierIs := func(tier string) domain.TriggerCondition {
		return trigger(domain.NewCategoryFeatureCondition(domain.FeatureAddressType, domain.CategoryValue(tier)))
	}
	fixed := func(amount string) domain.SurchargeCalculation {
		value, fixedErr := domain.NewFixedAmountSurcharge(money(amount, cny))
		must("固定金额 "+amount, fixedErr)
		return value
	}

	// 实重大于 0 对任何一件包裹都成立：触发条件没有「无条件」的写法，空组合又不当恒真（CONTEXT「触发条件」），
	// 燃油对每票都收，只能这样表达。
	fuelCalculation, err := domain.NewSeriesRateSurcharge(domain.ReferenceSeriesFuelRate, decimal("1"), "SYN-DEP-FULL-FUEL")
	must("燃油费率计算", err)
	fuel, err := domain.NewSurchargeRule("SYN-RULE-FULL-FUEL", chargeCode("FUEL_SURCHARGE"),
		"燃油附加费（合成演示值）", domain.ChargeEffectAdd, actualOver("0"), fuelCalculation)
	must("燃油规则", err)
	fuel, err = fuel.Standalone()
	must("燃油并列计收", err)

	remoteFirst, err := domain.NewSurchargeRule("SYN-RULE-FULL-REMOTE-T1", chargeCode("REMOTE_AREA_SURCHARGE"),
		"偏远地区费·一档（合成演示值）", domain.ChargeEffectAdd, tierIs("T1"), fixed("12"))
	must("偏远一档规则", err)
	remoteFirst, err = remoteFirst.InExclusivityGroup("SYN-GROUP-FULL-REMOTE", 2)
	must("偏远一档互斥组", err)
	remoteSecond, err := domain.NewSurchargeRule("SYN-RULE-FULL-REMOTE-T2", chargeCode("REMOTE_AREA_EXTENDED_SURCHARGE"),
		"偏远地区费·二档（合成演示值）", domain.ChargeEffectAdd, tierIs("T2"), fixed("28"))
	must("偏远二档规则", err)
	remoteSecond, err = remoteSecond.InExclusivityGroup("SYN-GROUP-FULL-REMOTE", 1)
	must("偏远二档互斥组", err)

	oversizeTrigger, err := domain.NewAnyOfTrigger(
		lengthOver(domain.FeatureLongestSide, "100"),
		lengthOver(domain.FeatureLengthAndGirth, "300"),
	)
	must("超尺寸触发", err)
	oversize, err := domain.NewSurchargeRule("SYN-RULE-FULL-OVERSIZE", chargeCode("OVERSIZE_SURCHARGE"),
		"超尺寸费（合成演示值）", domain.ChargeEffectAdd, oversizeTrigger, fixed("35"))
	must("超尺寸规则", err)
	oversize, err = oversize.InExclusivityGroup("SYN-GROUP-FULL-OVERSIZE-OVERWEIGHT", 1)
	must("超尺寸互斥组", err)
	overweightTrigger := actualOver("20")
	if spec.overweightOnChargeable {
		overweightTrigger = chargeableOver("20")
	}
	overweight, err := domain.NewSurchargeRule("SYN-RULE-FULL-OVERWEIGHT", chargeCode("OVERWEIGHT_SURCHARGE"),
		"超重费（合成演示值）", domain.ChargeEffectAdd, overweightTrigger, fixed("25"))
	must("超重规则", err)
	overweight, err = overweight.InExclusivityGroup("SYN-GROUP-FULL-OVERSIZE-OVERWEIGHT", 1)
	must("超重互斥组", err)

	fuelBasis, err := domain.NewListedChargeDependency("SYN-DEP-FULL-FUEL", chargeCode("FUEL_SURCHARGE"),
		[]domain.ChargeCode{chargeCode("BASE_FREIGHT")}, nil)
	must("燃油基数", err)
	fuelBinding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, fullFuelSeriesID)
	must("燃油序列绑定", err)
	surcharges := []domain.SurchargeRule{fuel, remoteFirst, remoteSecond, oversize, overweight}
	if spec.residentialDelivery {
		// 住宅派送费按收件地址性质收，与偏远档位是两个独立维度（CONTEXT「地址分类」），所以独立计收、不进任何互斥组：
		// 同一票既是住宅又落在偏远档位时两笔都收。
		residential, residentialErr := domain.NewCategoryFeatureCondition(domain.FeatureAddressNature, domain.CategoryValue(domain.AddressNatureResidential))
		residentialTrigger := trigger(residential, residentialErr)
		rule, ruleErr := domain.NewSurchargeRule("SYN-RULE-FULL-RESIDENTIAL", chargeCode("RESIDENTIAL_DELIVERY_SURCHARGE"),
			"住宅派送费（合成演示值）", domain.ChargeEffectAdd, residentialTrigger, fixed("6"))
		must("住宅派送规则", ruleErr)
		rule, ruleErr = rule.Standalone()
		must("住宅派送并列计收", ruleErr)
		surcharges = append(surcharges, rule)
	}
	structures, err := domain.NewPricingPlanStructures(
		surcharges,
		[]domain.ChargeDependency{fuelBasis},
		[]domain.ReferenceSeriesBinding{fuelBinding},
	)
	must("完整卡方案结构", err)

	zoneLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, fullZoneCatalogueID)
	must("完整卡分区目录绑定", err)
	tierLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindRemoteTier, fullTierCatalogueID)
	must("完整卡偏远档位目录绑定", err)
	structures, err = structures.WithReferenceCatalogues(zoneLink, tierLink)
	must("完整卡方案结构绑目录", err)
	amountRounding, err := domain.NewAmountRoundingPolicy(
		domain.RoundingHalfUp, money("0.01", cny),
		[]domain.AmountRoundingPoint{domain.AmountRoundingPerLine, domain.AmountRoundingTotal})
	must("完整卡金额取整", err)
	structures, err = structures.WithAmountRounding(amountRounding)
	must("完整卡方案结构声明金额取整", err)

	scope, err := domain.NewPricingScopeID(spec.scope)
	must("完整卡计价范围", err)
	plan, err := domain.NewPricingPlanVersion(
		reference(domain.ArtifactPricingPlan, spec.planID, "v1"),
		scope, domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		chargeCode("BASE_FREIGHT"), period("2026-01-01"),
		table, weightPolicy, nil, structures,
	)
	must("完整售价方案", err)

	return marshalCard(plan, spec.sourceFileName, "SYN-AUTH-PRICE-DIR-01")
}
