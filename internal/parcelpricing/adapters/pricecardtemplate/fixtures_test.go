package pricecardtemplate

import (
	"bytes"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

type rows = []map[string]string

// filled 是一份填好的模板：card 的字段值，加上其余各表的数据行（按列键）。
type filled struct {
	card map[string]string
	data map[string]rows
}

func (card filled) clone() filled {
	copied := filled{card: map[string]string{}, data: map[string]rows{}}
	for key, value := range card.card {
		copied.card[key] = value
	}
	for sheet, lines := range card.data {
		for _, line := range lines {
			row := map[string]string{}
			for key, value := range line {
				row[key] = value
			}
			copied.data[sheet] = append(copied.data[sheet], row)
		}
	}
	return copied
}

// sheets 按模板写成工作表；card 表每个字段一行、按模板行序。
func (card filled) sheets(t *testing.T) []spreadsheet.WriteSheet {
	t.Helper()
	out := make([]spreadsheet.WriteSheet, 0, len(sheets))
	for _, spec := range sheets {
		sheet := spreadsheet.WriteSheet{Name: spec.name, Columns: textColumns(spec.columns)}
		if spec.name == sheetCard {
			for _, entry := range cardFields {
				sheet.Rows = append(sheet.Rows, []string{entry.key, card.card[entry.key]})
			}
		}
		for _, line := range card.data[spec.name] {
			for key := range line {
				if _, _, known := spec.column(key); !known {
					t.Fatalf("夹具写错：表 %s 没有列 %s", spec.name, key)
				}
			}
			cells := make([]string, len(spec.columns))
			for index, entry := range spec.columns {
				cells[index] = line[entry.key]
			}
			sheet.Rows = append(sheet.Rows, cells)
		}
		out = append(out, sheet)
	}
	return out
}

func (card filled) bytes(t *testing.T) []byte {
	t.Helper()
	return writeSheets(t, card.sheets(t))
}

// book 读回成工作簿，给要改格类型的用例用（写出件只写文本格）。
func (card filled) book(t *testing.T) spreadsheet.Workbook {
	t.Helper()
	book, err := spreadsheet.Read(card.bytes(t), spreadsheet.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return book
}

func writeSheets(t *testing.T, sheets []spreadsheet.WriteSheet) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := spreadsheet.Write(&buffer, sheets); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

const synStart = "2026-01-01T00:00:00Z"

// costCard 是种子 SYN-PLAN-CN-SG-COST-01 v1 的模板写法（规范第八节）。
func costCard() filled {
	return filled{
		card: map[string]string{
			"templateVersion": TemplateVersion, "planId": "SYN-PLAN-CN-SG-COST-01", "planVersion": "v1",
			"scope": "SYN-SCOPE-01", "direction": "BUY", "purpose": "SUPPLIER_COST", "aggregation": "PER_PACKAGE",
			"baseChargeCode": "BASE_FREIGHT", "periodStartsAt": synStart, "rateTableId": "SYN-TABLE-CN-SG-COST-01",
			"weightPolicyId": "SYN-WEIGHT-CN-SG-COST-01", "weightPolicyVersion": "v1", "weightMethod": "ACTUAL_ONLY",
			"directionAuthorizationId": "SYN-AUTH-COST-DIR-01", "directionAuthorizationVersion": "v1",
		},
		data: map[string]rows{
			sheetTables: {{"tableId": "SYN-TABLE-CN-SG-COST-01", "tableVersion": "v1", "family": "WEIGHT_ZONE",
				"currency": "CNY", "weightUnit": "KG", "periodStartsAt": synStart}},
			sheetRatesWeightZone: {
				{"tableId": "SYN-TABLE-CN-SG-COST-01", "entryId": "SYN-COST-Z1-A", "zone": "Z1", "minimum": "0", "maximum": "1", "amount": "30"},
				{"tableId": "SYN-TABLE-CN-SG-COST-01", "entryId": "SYN-COST-Z1-B", "zone": "Z1", "minimum": "1", "maximum": "5", "amount": "85"},
				{"tableId": "SYN-TABLE-CN-SG-COST-01", "entryId": "SYN-COST-Z1-C", "zone": "Z1", "minimum": "5", "maximum": "30", "amount": "260"},
			},
			sheetWeightRounding: {{"policy": "CHARGEABLE", "mode": "CEILING", "increment": "0.5", "unit": "KG"}},
		},
	}
}

// sellCard 是种子 SYN-PLAN-CN-SG-01 v1 的模板写法。
func sellCard() filled {
	return filled{
		card: map[string]string{
			"templateVersion": TemplateVersion, "planId": "SYN-PLAN-CN-SG-01", "planVersion": "v1",
			"scope": "SYN-SCOPE-01", "direction": "SELL", "purpose": "CUSTOMER_CHARGE", "aggregation": "PER_PACKAGE",
			"baseChargeCode": "BASE_FREIGHT", "periodStartsAt": synStart, "rateTableId": "SYN-TABLE-CN-SG-01",
			"weightPolicyId": "SYN-WEIGHT-CN-SG-01", "weightPolicyVersion": "v1", "weightMethod": "MAX",
			"volumetricDivisor": "5000", "volumetricLengthUnit": "CM",
			"directionAuthorizationId": "SYN-AUTH-PRICE-DIR-01", "directionAuthorizationVersion": "v1",
		},
		data: map[string]rows{
			sheetTables: {{"tableId": "SYN-TABLE-CN-SG-01", "tableVersion": "v1", "family": "FIRST_CONTINUE",
				"currency": "CNY", "weightUnit": "KG", "periodStartsAt": synStart}},
			sheetRatesFirstContinue: {
				{"tableId": "SYN-TABLE-CN-SG-01", "entryId": "SYN-RATE-CN-SG-Z1", "zone": "Z1",
					"firstWeight": "0.5", "firstAmount": "55", "step": "0.5", "stepAmount": "18"},
				{"tableId": "SYN-TABLE-CN-SG-01", "entryId": "SYN-RATE-CN-SG-Z2", "zone": "Z2",
					"firstWeight": "0.5", "firstAmount": "48", "step": "0.5", "stepAmount": "15"},
			},
			sheetWeightRounding: {
				{"policy": "CHARGEABLE", "mode": "CEILING", "increment": "0.5", "unit": "KG"},
				{"policy": "VOLUMETRIC", "mode": "CEILING", "increment": "0.1", "unit": "KG"},
			},
			sheetFixedCharges: {{"ruleId": "SYN-RULE-REMOTE-AREA", "chargeCode": "REMOTE_AREA_SURCHARGE",
				"description": "SYN 偏远区域附加（合成演示值）", "effect": "ADD", "amount": "5", "order": "1"}},
			sheetReferenceSeries: {{"kind": "FUEL_RATE", "seriesId": "SYN-SERIES-FUEL-01"}},
		},
	}
}

// fullCard 用到模板的每一张表：分段进位与体积重、首续重与重量段两种价表、固定费用、互斥分组与
// 并列计收、条件最低计价重量、三种条件来源与组合、取较大值套定额与百分比、查表、按序列费率、
// 两种费用依赖、两条序列绑定、排除条款、参考目录、金额取整与版本清单依赖。
func fullCard() filled {
	const end = "2027-01-01T00:00:00Z"
	return filled{
		card: map[string]string{
			"templateVersion": TemplateVersion, "planId": "SYN-PPT-PLAN-FULL", "planVersion": "v1",
			"scope": "SYN-SCOPE-01", "direction": "BUY", "purpose": "SUPPLIER_COST", "aggregation": "PER_PACKAGE",
			"baseChargeCode": "BASE_FREIGHT", "periodStartsAt": synStart, "periodEndsAt": end,
			"rateTableId": "SYN-PPT-TABLE-BASE", "weightPolicyId": "SYN-PPT-WEIGHT", "weightPolicyVersion": "v1",
			"weightMethod": "MAX", "volumetricDivisor": "5000", "volumetricLengthUnit": "CM",
			"amountRoundingMode": "HALF_UP", "amountRoundingIncrement": "0.01", "amountRoundingPoints": "PER_LINE; TOTAL",
			"directionAuthorizationId": "SYN-PPT-AUTH", "directionAuthorizationVersion": "v1",
		},
		data: map[string]rows{
			sheetTables: {
				{"tableId": "SYN-PPT-TABLE-BASE", "tableVersion": "v1", "family": "WEIGHT_ZONE", "currency": "USD",
					"weightUnit": "KG", "periodStartsAt": synStart, "periodEndsAt": end},
				{"tableId": "SYN-PPT-TABLE-REMOTE", "tableVersion": "v1", "family": "FIRST_CONTINUE", "currency": "USD",
					"weightUnit": "KG", "periodStartsAt": synStart, "periodEndsAt": end},
			},
			sheetRatesWeightZone: {
				{"tableId": "SYN-PPT-TABLE-BASE", "entryId": "SYN-PPT-ENTRY-LOW", "zone": "Z1", "minimum": "0", "maximum": "10", "amount": "10"},
				{"tableId": "SYN-PPT-TABLE-BASE", "entryId": "SYN-PPT-ENTRY-OPEN", "zone": "Z1", "minimum": "10", "amount": "20"},
			},
			sheetRatesFirstContinue: {
				{"tableId": "SYN-PPT-TABLE-REMOTE", "entryId": "SYN-PPT-REMOTE-Z1", "zone": "Z1",
					"firstWeight": "1", "firstAmount": "3", "step": "0.5", "stepAmount": "1"},
			},
			sheetWeightRounding: {
				{"policy": "CHARGEABLE", "mode": "CEILING", "increment": "0.1", "maximum": "5", "unit": "KG"},
				{"policy": "VOLUMETRIC", "mode": "CEILING", "increment": "0.1", "unit": "KG"},
				{"policy": "CHARGEABLE", "mode": "CEILING", "increment": "0.5", "unit": "KG"},
			},
			sheetFixedCharges: {
				{"ruleId": "handling", "chargeCode": "RULE_HANDLING", "description": "handling", "effect": "ADD", "amount": "2", "order": "1"},
				{"ruleId": "rebate", "chargeCode": "RULE_REBATE", "description": "rebate", "effect": "DEDUCT", "amount": "1", "order": "2"},
			},
			sheetSurcharges: {
				{"ruleId": "SYN-PPT-SUR-OVERSIZE", "chargeCode": "OVERSIZE_FEE", "description": "oversize handling", "effect": "ADD",
					"conditionId": "C-OVERSIZE", "calculationId": "K-OVERSIZE", "exclusivity": "GROUPED", "exclusivityGroup": "OVERSIZE",
					"priority": "2", "minimumWeightId": "SYN-PPT-MIN-OVERSIZE", "minimumWeightConditionId": "C-MIN", "minimumWeight": "20"},
				{"ruleId": "SYN-PPT-SUR-REMOTE", "chargeCode": "REMOTE_AREA", "description": "remote area delivery", "effect": "ADD",
					"conditionId": "C-REMOTE", "calculationId": "K-REMOTE", "exclusivity": "STANDALONE"},
				{"ruleId": "SYN-PPT-SUR-FUEL", "chargeCode": "FUEL_SURCHARGE", "description": "fuel surcharge", "effect": "ADD",
					"conditionId": "C-FUEL", "calculationId": "K-FUEL", "exclusivity": "STANDALONE"},
			},
			sheetConditions: {
				{"nodeId": "C-OVERSIZE", "kind": "ANY_OF"},
				{"nodeId": "C-OVERSIZE-LEN", "parentId": "C-OVERSIZE", "kind": "PREDICATE", "source": "LONGEST_SIDE", "operator": "GT", "threshold": "100", "unit": "CM"},
				{"nodeId": "C-OVERSIZE-WT", "parentId": "C-OVERSIZE", "kind": "PREDICATE", "source": "ACTUAL_WEIGHT", "operator": "GT", "threshold": "20", "unit": "KG"},
				{"nodeId": "C-OVERSIZE-VOL", "parentId": "C-OVERSIZE", "kind": "PREDICATE", "source": "VOLUME", "operator": "GT", "threshold": "1000", "unit": "CM"},
				{"nodeId": "C-MIN", "kind": "PREDICATE", "source": "LENGTH_AND_GIRTH", "operator": "GT", "threshold": "300", "unit": "CM"},
				{"nodeId": "C-REMOTE", "kind": "PREDICATE", "source": "ZONE", "category": "Z1"},
				{"nodeId": "C-FUEL", "kind": "PREDICATE", "source": "ACTUAL_WEIGHT", "operator": "GT", "threshold": "0", "unit": "KG"},
				{"nodeId": "C-EXCL", "kind": "PREDICATE", "source": "ACTUAL_WEIGHT", "operator": "GT", "threshold": "150", "unit": "KG"},
			},
			sheetCalculations: {
				{"nodeId": "K-OVERSIZE", "method": "GREATER_OF"},
				{"nodeId": "K-OVERSIZE-FIXED", "parentId": "K-OVERSIZE", "method": "FIXED_AMOUNT", "amount": "5"},
				{"nodeId": "K-OVERSIZE-PCT", "parentId": "K-OVERSIZE", "method": "PERCENT_OF_BASIS", "percentage": "0.1", "basisDependencyId": "SYN-PPT-DEP-ALL"},
				{"nodeId": "K-REMOTE", "method": "TABLE_LOOKUP", "tableId": "SYN-PPT-TABLE-REMOTE"},
				{"nodeId": "K-FUEL", "method": "PERCENT_OF_BASIS", "seriesKind": "FUEL_RATE", "seriesFactor": "0.8", "basisDependencyId": "SYN-PPT-DEP-FUEL"},
			},
			sheetChargeDependencies: {
				{"dependencyId": "SYN-PPT-DEP-ALL", "chargeCode": "OVERSIZE_FEE", "composition": "ALL_CHARGES", "excludes": "REMOTE_AREA"},
				{"dependencyId": "SYN-PPT-DEP-FUEL", "chargeCode": "FUEL_SURCHARGE", "composition": "LISTED_CHARGES", "includes": "BASE_FREIGHT"},
			},
			sheetReferenceSeries: {
				{"kind": "FUEL_RATE", "seriesId": "SYN-PPT-FUEL-WEEKLY"},
				{"kind": "EXCHANGE_RATE", "seriesId": "SYN-PPT-USD-CNY"},
			},
			sheetExclusions:           {{"exclusionId": "SYN-PPT-EXCL-OVERLIMIT", "clause": "L12 over-limit refusal", "conditionId": "C-EXCL"}},
			sheetReferenceCatalogues:  {{"kind": "ZONE", "catalogueId": "SYN-PPT-CAT-ZONE"}},
			sheetManifestDependencies: {{"kind": "commercial-policy", "id": "SYN-PPT-FX-POLICY", "version": "v3"}},
		},
	}
}

// fullPlan 用领域构造函数直接立出 fullCard 描述的那一张方案，是模板翻译的独立对照。
func fullPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	usd := value(t, domain.NewCurrency, "USD")
	kg := domain.WeightUnitKilogram
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	period := try(domain.NewEffectivePeriod(start, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)))
	ref := func(kind domain.ArtifactKind, id, version string) domain.VersionReference {
		return try(domain.NewVersionReferenceIdentity(kind, id, version))
	}
	weight := func(text string) domain.Weight { return try(domain.NewWeightFromString(text, kg)) }
	money := func(text string) domain.Money { return try(domain.NewMoneyFromString(text, usd)) }
	decimal := func(text string) domain.Decimal { return try(domain.ParseDecimal(text)) }
	code := func(text string) domain.ChargeCode { return value(t, domain.NewChargeCode, text) }
	entryID := func(text string) domain.RateEntryID { return value(t, domain.NewRateEntryID, text) }
	trigger := func(condition domain.FeatureCondition) domain.TriggerCondition {
		return try(domain.NewTrigger(condition))
	}
	length := func(source domain.FeatureSource, threshold string) domain.TriggerCondition {
		return trigger(try(domain.NewLengthFeatureCondition(source, domain.ComparisonGreaterThan,
			try(domain.NewLength(decimal(threshold), domain.LengthUnitCentimeter)))))
	}
	actual := func(threshold string) domain.TriggerCondition {
		return trigger(try(domain.NewWeightFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThan, weight(threshold))))
	}

	low := try(domain.NewRateEntry(entryID("SYN-PPT-ENTRY-LOW"), "Z1", weight("0"), weight("10"), money("10")))
	open := try(domain.NewOpenEndedRateEntry(entryID("SYN-PPT-ENTRY-OPEN"), "Z1", weight("10"), money("20")))
	base := try(domain.NewRateTableVersion(ref(domain.ArtifactRateTable, "SYN-PPT-TABLE-BASE", "v1"),
		domain.RateTableFamilyWeightZone, usd, kg, period, []domain.RateEntry{low, open}))
	remoteRate := try(domain.NewFirstContinueRate(entryID("SYN-PPT-REMOTE-Z1"), "Z1", weight("1"), money("3"), weight("0.5"), money("1")))
	remoteTable := try(domain.NewFirstContinueRateTable(ref(domain.ArtifactRateTable, "SYN-PPT-TABLE-REMOTE", "v1"),
		usd, kg, period, []domain.FirstContinueRate{remoteRate}))

	fine := try(domain.NewWeightRoundingSegment(domain.RoundingCeiling, weight("0.1"), weight("5")))
	coarse := try(domain.NewOpenEndedWeightRoundingSegment(domain.RoundingCeiling, weight("0.5")))
	rounding := try(domain.NewSegmentedWeightRoundingPolicy([]domain.WeightRoundingSegment{fine, coarse}))
	factor := try(domain.NewVolumetricFactor(decimal("5000"), domain.LengthUnitCentimeter,
		try(domain.NewWeightRoundingPolicy(domain.RoundingCeiling, weight("0.1")))))
	policy := try(domain.NewPricingWeightPolicy(ref(domain.ArtifactWeightPolicy, "SYN-PPT-WEIGHT", "v1"), domain.PricingWeightMax, rounding, &factor))

	oversizeTrigger := try(domain.NewAnyOfTrigger(
		length(domain.FeatureLongestSide, "100"),
		actual("20"),
		trigger(try(domain.NewVolumeFeatureCondition(domain.FeatureVolume, domain.ComparisonGreaterThan,
			try(domain.NewVolume(decimal("1000"), domain.LengthUnitCentimeter))))),
	))
	greater := try(domain.NewGreaterOfSurcharge(
		try(domain.NewFixedAmountSurcharge(money("5"))),
		try(domain.NewPercentOfBasisSurcharge(decimal("0.1"), "SYN-PPT-DEP-ALL")),
	))
	oversize := try(domain.NewSurchargeRule("SYN-PPT-SUR-OVERSIZE", code("OVERSIZE_FEE"), "oversize handling", domain.ChargeEffectAdd, oversizeTrigger, greater))
	oversize = try(oversize.InExclusivityGroup("OVERSIZE", 2))
	oversize = try(oversize.WithConditionalMinimumWeight(try(domain.NewConditionalMinimumWeight(
		"SYN-PPT-MIN-OVERSIZE", length(domain.FeatureLengthAndGirth, "300"), weight("20")))))
	remote := try(domain.NewSurchargeRule("SYN-PPT-SUR-REMOTE", code("REMOTE_AREA"), "remote area delivery", domain.ChargeEffectAdd,
		trigger(try(domain.NewCategoryFeatureCondition(domain.FeatureZone, domain.CategoryValue("Z1")))),
		try(domain.NewTableLookupSurcharge(remoteTable))))
	remote = try(remote.Standalone())
	fuel := try(domain.NewSurchargeRule("SYN-PPT-SUR-FUEL", code("FUEL_SURCHARGE"), "fuel surcharge", domain.ChargeEffectAdd,
		actual("0"), try(domain.NewSeriesRateSurcharge(domain.ReferenceSeriesFuelRate, decimal("0.8"), "SYN-PPT-DEP-FUEL"))))
	fuel = try(fuel.Standalone())

	dependencies := []domain.ChargeDependency{
		try(domain.NewAllChargesDependency("SYN-PPT-DEP-ALL", code("OVERSIZE_FEE"), []domain.ChargeCode{code("REMOTE_AREA")})),
		try(domain.NewListedChargeDependency("SYN-PPT-DEP-FUEL", code("FUEL_SURCHARGE"), []domain.ChargeCode{code("BASE_FREIGHT")}, nil)),
	}
	series := []domain.ReferenceSeriesBinding{
		try(domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "SYN-PPT-FUEL-WEEKLY")),
		try(domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, "SYN-PPT-USD-CNY")),
	}
	structures := try(domain.NewPricingPlanStructures([]domain.SurchargeRule{oversize, remote, fuel}, dependencies, series))
	structures = try(structures.WithExclusionRules(try(domain.NewExclusionRule("SYN-PPT-EXCL-OVERLIMIT", "L12 over-limit refusal", actual("150")))))
	structures = try(structures.WithAmountRounding(try(domain.NewAmountRoundingPolicy(domain.RoundingHalfUp, money("0.01"),
		[]domain.AmountRoundingPoint{domain.AmountRoundingPerLine, domain.AmountRoundingTotal}))))
	structures = try(structures.WithReferenceCatalogues(try(domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, "SYN-PPT-CAT-ZONE"))))

	rules := []domain.FixedChargeRule{
		try(domain.NewFixedChargeRule("handling", code("RULE_HANDLING"), "handling", domain.ChargeEffectAdd, money("2"), 1)),
		try(domain.NewFixedChargeRule("rebate", code("RULE_REBATE"), "rebate", domain.ChargeEffectDeduct, money("1"), 2)),
	}
	return try(domain.NewPricingPlanVersion(ref(domain.ArtifactPricingPlan, "SYN-PPT-PLAN-FULL", "v1"),
		value(t, domain.NewPricingScopeID, "SYN-SCOPE-01"), domain.PricingDirectionBuy, domain.PricingPurposeSupplierCost,
		code("BASE_FREIGHT"), period, base, policy, rules, structures,
		ref(domain.ArtifactCommercialPolicy, "SYN-PPT-FX-POLICY", "v3")))
}

func value[T any](t *testing.T, constructor func(string) (T, error), text string) T {
	t.Helper()
	built, err := constructor(text)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return built
}

// try 把「值, 错」两返回收成一个值；夹具构造错直接让测试崩——那是夹具自己的错，不是被测对象的。
func try[T any](built T, err error) T {
	if err != nil {
		panic(err)
	}
	return built
}
