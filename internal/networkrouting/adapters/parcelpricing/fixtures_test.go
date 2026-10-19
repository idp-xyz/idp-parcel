package parcelpricing_test

import (
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是取数侧用例的合成夹具。价卡全为隔离合成（S 级纪律）：只钉装配规则与合成算术，
// 不冒充任何租户实例值。

func value[T any](t *testing.T, constructor func(string) (T, error), raw string) T {
	t.Helper()
	built, err := constructor(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// fxDailyRegistration 登一版 fx-daily 汇率序列：口径按 fx-caliber 政策报价，一期自卡的适用期起不封口、取值 rate。方案侧
// 要有同名汇率绑定才用得上它（syntheticBuyPlanWithRate 带）。
func fxDailyRegistration(t *testing.T, rate string) ppdomain.ReferenceSeriesRegistration {
	t.Helper()
	fxSeries, err := ppdomain.NewVersionReference(
		ppdomain.ArtifactReferenceSeries, "fx-daily", "v1", "sha256:syn-fx-1")
	if err != nil {
		t.Fatalf("fx series: %v", err)
	}
	caliber, err := ppdomain.NewVersionReference(
		ppdomain.ArtifactCommercialPolicy, "fx-caliber", "v1", "sha256:syn-caliber-1")
	if err != nil {
		t.Fatalf("caliber: %v", err)
	}
	period, err := ppdomain.NewSeriesPeriodValue(planPeriod, time.Time{}, decimalOf(t, rate), "SYN-EVIDENCE/fx-daily")
	if err != nil {
		t.Fatalf("fx period: %v", err)
	}
	registration, err := ppdomain.NewReferenceSeriesRegistration(ppdomain.ReferenceSeriesRegistrationSpec{
		Tenant:           value(t, ppdomain.NewTenantID, "tenant-1"),
		Kind:             ppdomain.ReferenceSeriesExchangeRate,
		Reference:        fxSeries,
		SourceIdentifier: "SYN-BANK/fx-daily",
		Registrant:       "SYN-OPS",
		QuoteBasis:       caliber,
		Periods:          []ppdomain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("fx registration: %v", err)
	}
	return registration
}

// zoneCatalogueRegistration 登一版分区目录：目的邮编前缀 018 落 Z1，与始发无关，自卡的适用期起不封口。目录标识要与卡上
// 的绑定同名（zoneCatalogueBoundBuyPlan 绑 zones-<卡标识>）。
func zoneCatalogueRegistration(t *testing.T, catalogueID string) ppdomain.ReferenceCatalogueRegistration {
	t.Helper()
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactReferenceCatalogue, catalogueID, "v1")
	if err != nil {
		t.Fatalf("catalogue reference: %v", err)
	}
	period, err := ppdomain.NewEffectivePeriod(planPeriod, time.Time{})
	if err != nil {
		t.Fatalf("catalogue period: %v", err)
	}
	entry, err := ppdomain.NewCatalogueEntry("018", "Z1")
	if err != nil {
		t.Fatalf("catalogue entry: %v", err)
	}
	registration, err := ppdomain.NewReferenceCatalogueRegistration(ppdomain.ReferenceCatalogueRegistrationSpec{
		Tenant:           value(t, ppdomain.NewTenantID, "tenant-1"),
		Kind:             ppdomain.CatalogueKindZone,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/zone-chart",
		Registrant:       "SYN-OPS",
		Origin:           ppdomain.NewIndependentCatalogueOrigin(),
		PrefixLength:     3,
		Entries:          []ppdomain.CatalogueEntry{entry},
		Period:           period,
	})
	if err != nil {
		t.Fatalf("catalogue registration: %v", err)
	}
	return registration
}

func decimalOf(t *testing.T, raw string) ppdomain.Decimal {
	t.Helper()
	decimal, err := ppdomain.ParseDecimal(raw)
	if err != nil {
		t.Fatalf("decimal %q: %v", raw, err)
	}
	return decimal
}

// syntheticBuyPlan 立一张 USD 计价的供应商采购价卡（带 fx-daily 汇率绑定）：Z1 区 0–10kg
// 每 kg 收 baseAmount。
func syntheticBuyPlan(t *testing.T, id, version, baseAmount string) ppdomain.PricingPlanVersion {
	t.Helper()
	return syntheticBuyPlanIn(t, id, version, baseAmount, "USD")
}

func syntheticBuyPlanIn(t *testing.T, id, version, baseAmount, currency string) ppdomain.PricingPlanVersion {
	t.Helper()
	return rateTablePlan(t, id, version, baseAmount, currency,
		ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, "Z1", false)
}

// syntheticBuyPlanWithRate 是带 fx-daily 汇率绑定的采购价卡：异币种比较用例用（绑定要
// 配上输入侧的汇率读数才解得出来）。
func syntheticBuyPlanWithRate(t *testing.T, id, version, baseAmount string) ppdomain.PricingPlanVersion {
	t.Helper()
	return rateTablePlan(t, id, version, baseAmount, "USD",
		ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, "Z1", true)
}

// syntheticInternalPlan 立一张内部标准成本价卡：方向 INTERNAL、用途 INTERNAL_PRICE。
func syntheticInternalPlan(t *testing.T, id, version, baseAmount string) ppdomain.PricingPlanVersion {
	t.Helper()
	return rateTablePlan(t, id, version, baseAmount, "USD",
		ppdomain.PricingDirectionInternal, ppdomain.PricingPurposeInternalPrice, "Z1", false)
}

// zoneLessBuyPlan 立一张对 Z1 没有档位的卡（档位只在 Z9）：评价落不可计价那一格。
func zoneLessBuyPlan(t *testing.T, id string, version int) ppdomain.PricingPlanVersion {
	t.Helper()
	return rateTablePlan(t, id, "v1", "3", "USD",
		ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost, "Z9", false)
}

// rateTablePlan 是合成价卡的最小配方：单档重量段表 + 实重计价策略 + fx-daily 汇率绑定，
// 无金额取整声明。entryZone 是价卡档位所在的分区。
func rateTablePlan(
	t *testing.T,
	id, version, baseAmount, currency string,
	direction ppdomain.PricingDirection,
	purpose ppdomain.PricingPurpose,
	entryZone string,
	bindFX bool,
) ppdomain.PricingPlanVersion {
	t.Helper()
	code := value(t, ppdomain.NewCurrency, currency)
	period, err := ppdomain.NewEffectivePeriod(planPeriod, planPeriod.AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	table, err := ppdomain.NewRateTableVersion(
		mustReference(t, ppdomain.ArtifactRateTable, "table-"+id, version),
		ppdomain.RateTableFamilyWeightZone, code, ppdomain.WeightUnitKilogram, period,
		[]ppdomain.RateEntry{rateEntry(t, entryZone, baseAmount, currency)},
	)
	if err != nil {
		t.Fatalf("rate table: %v", err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, weightOf(t, "0.5"))
	if err != nil {
		t.Fatalf("weight rounding: %v", err)
	}
	weightPolicy, err := ppdomain.NewPricingWeightPolicy(
		mustReference(t, ppdomain.ArtifactWeightPolicy, "weight-"+id, version),
		ppdomain.PricingWeightActualOnly, rounding, nil,
	)
	if err != nil {
		t.Fatalf("weight policy: %v", err)
	}
	var structures ppdomain.PricingPlanStructures
	if bindFX {
		binding, err := ppdomain.NewReferenceSeriesBinding(ppdomain.ReferenceSeriesExchangeRate, "fx-daily")
		if err != nil {
			t.Fatalf("fx binding: %v", err)
		}
		structures, err = ppdomain.NewPricingPlanStructures(nil, nil, []ppdomain.ReferenceSeriesBinding{binding})
		if err != nil {
			t.Fatalf("structures: %v", err)
		}
	}
	return rateTablePlanWith(t, id, version, direction, purpose, table, weightPolicy, period, structures)
}

// zoneCatalogueBoundBuyPlan 是档位在 Z1、却绑了分区目录的 BUY 卡：区域只能从目录读数来（ADR-0109 决定四）。
func zoneCatalogueBoundBuyPlan(t *testing.T, id, version, baseAmount string) ppdomain.PricingPlanVersion {
	t.Helper()
	plain := syntheticBuyPlan(t, id, version, baseAmount)
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "zones-"+id)
	if err != nil {
		t.Fatalf("catalogue link: %v", err)
	}
	structures, err := ppdomain.PricingPlanStructures{}.WithReferenceCatalogues(link)
	if err != nil {
		t.Fatalf("catalogue structures: %v", err)
	}
	return rateTablePlanWith(t, id, version, ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost,
		plain.RateTable(), plain.WeightPolicy(), plain.EffectivePeriod(), structures)
}

func rateTablePlanWith(
	t *testing.T,
	id, version string,
	direction ppdomain.PricingDirection,
	purpose ppdomain.PricingPurpose,
	table ppdomain.RateTableVersion,
	weightPolicy ppdomain.PricingWeightPolicy,
	period ppdomain.EffectivePeriod,
	structures ppdomain.PricingPlanStructures,
) ppdomain.PricingPlanVersion {
	t.Helper()
	plan, err := ppdomain.NewPricingPlanVersion(
		mustReference(t, ppdomain.ArtifactPricingPlan, id, version),
		value(t, ppdomain.NewPricingScopeID, "scope-1"),
		direction,
		purpose,
		value(t, ppdomain.NewChargeCode, "BASE_FREIGHT"),
		period,
		table,
		weightPolicy,
		nil,
		structures,
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

// rateEntry 造 0–10kg 的单档条目。
func rateEntry(t *testing.T, zone, amount, currency string) ppdomain.RateEntry {
	t.Helper()
	money, err := ppdomain.NewMoneyFromString(amount, value(t, ppdomain.NewCurrency, currency))
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	built, err := ppdomain.NewRateEntry(
		value(t, ppdomain.NewRateEntryID, "entry-"+zone),
		zone,
		weightOf(t, "0"),
		weightOf(t, "10"),
		money,
	)
	if err != nil {
		t.Fatalf("rate entry: %v", err)
	}
	return built
}

func mustReference(t *testing.T, kind ppdomain.ArtifactKind, id, version string) ppdomain.VersionReference {
	t.Helper()
	reference, err := ppdomain.NewVersionReference(kind, id, version, "sha256:syn-"+id)
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	return reference
}

func weightOf(t *testing.T, raw string) ppdomain.Weight {
	t.Helper()
	weight, err := ppdomain.NewWeightFromString(raw, ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("weight %q: %v", raw, err)
	}
	return weight
}
