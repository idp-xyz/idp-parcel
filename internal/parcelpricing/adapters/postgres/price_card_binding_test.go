package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// boundCardPlan 造一张带计价范围、方向、适用期与方案结构的合成价卡，供「绑了这条序列或目录的卡在哪些范围上」用。
func boundCardPlan(
	t *testing.T,
	planID, planVersion, scope string,
	direction domain.PricingDirection,
	from time.Time,
	structures domain.PricingPlanStructures,
) domain.PricingPlanVersion {
	t.Helper()
	purpose := domain.PricingPurposeInternalPrice
	switch direction {
	case domain.PricingDirectionSell:
		purpose = domain.PricingPurposeCustomerCharge
	case domain.PricingDirectionBuy:
		purpose = domain.PricingPurposeSupplierCost
	}
	currency := evaluationValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(evaluationValue(t, domain.NewRateEntryID, "entry-"+planID+"-"+planVersion), "Z1",
		catalogWeight(t, "0"), catalogWeight(t, "10"), catalogMoney(t, "5.00", currency))
	if err != nil {
		t.Fatalf("构造费率段：%v", err)
	}
	period, err := domain.NewEffectivePeriod(from, from.AddDate(0, 6, 0))
	if err != nil {
		t.Fatalf("构造适用期：%v", err)
	}
	tableRef, err := domain.NewVersionReferenceIdentity(domain.ArtifactRateTable, "table-"+planID, planVersion)
	if err != nil {
		t.Fatalf("构造价表引用：%v", err)
	}
	table, err := domain.NewRateTableVersion(tableRef, domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram,
		period, []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("构造价表：%v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, catalogWeight(t, "0.5"))
	if err != nil {
		t.Fatalf("构造取整策略：%v", err)
	}
	weightRef, err := domain.NewVersionReferenceIdentity(domain.ArtifactWeightPolicy, "weight-"+planID, planVersion)
	if err != nil {
		t.Fatalf("构造计价重引用：%v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(weightRef, domain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("构造计价重策略：%v", err)
	}
	planRef, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, planID, planVersion)
	if err != nil {
		t.Fatalf("构造方案引用：%v", err)
	}
	plan, err := domain.NewPricingPlanVersion(planRef, evaluationValue(t, domain.NewPricingScopeID, scope), direction, purpose,
		evaluationValue(t, domain.NewChargeCode, "BASE_FREIGHT"), period, table, weightPolicy, nil, structures)
	if err != nil {
		t.Fatalf("构造价卡：%v", err)
	}
	return plan
}

func seriesBound(t *testing.T, seriesID string) domain.PricingPlanStructures {
	t.Helper()
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, seriesID)
	if err != nil {
		t.Fatalf("汇率序列绑定：%v", err)
	}
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("方案结构绑汇率序列：%v", err)
	}
	return structures
}

// Covers: AT-PP-004 的读口半边——一条序列或一份目录绑在哪些卡上：交回那几张卡的计价范围与方向，同一范围同一方向的
// 几版只交一次；没绑它的卡不交；别的租户的卡不交；内部方向的卡照实交回（不交信封是登记口那一步的事）；种类不是序列
// 或目录就报错。
func TestTheCardsBindingASeriesOrCatalogueAreFoundWithTheirScopes(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()
	january := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	zoneLink, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, "SYN-CAT-ZONE")
	if err != nil {
		t.Fatal(err)
	}
	seriesAndZone, err := seriesBound(t, "SYN-SERIES-FX").WithReferenceCatalogues(zoneLink)
	if err != nil {
		t.Fatal(err)
	}
	for _, registration := range []domain.PriceCardRegistration{
		cardRegistration(t, "tenant-a", boundCardPlan(t, "SYN-CARD-A", "v1", "SYN-SCOPE-A", domain.PricingDirectionBuy, january, seriesBound(t, "SYN-SERIES-FX"))),
		cardRegistration(t, "tenant-a", boundCardPlan(t, "SYN-CARD-A", "v2", "SYN-SCOPE-A", domain.PricingDirectionBuy, july, seriesBound(t, "SYN-SERIES-FX"))),
		cardRegistration(t, "tenant-a", boundCardPlan(t, "SYN-CARD-B", "v1", "SYN-SCOPE-B", domain.PricingDirectionSell, january, seriesAndZone)),
		cardRegistration(t, "tenant-a", boundCardPlan(t, "SYN-CARD-C", "v1", "SYN-SCOPE-C", domain.PricingDirectionBuy, january, domain.PricingPlanStructures{})),
		cardRegistration(t, "tenant-a", boundCardPlan(t, "SYN-CARD-D", "v1", "SYN-SCOPE-D", domain.PricingDirectionInternal, january, seriesBound(t, "SYN-SERIES-FX"))),
		cardRegistration(t, "tenant-b", boundCardPlan(t, "SYN-CARD-E", "v1", "SYN-SCOPE-E", domain.PricingDirectionBuy, january, seriesBound(t, "SYN-SERIES-FX"))),
	} {
		if outcome := registerCard(t, catalog, transactor, ctx, registration); outcome != ports.PriceCardRegistered {
			t.Fatalf("登记 %s = %d", registration.Plan().Reference().ID(), outcome)
		}
	}
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	describe := func(scopes []ports.PricedScope) []string {
		var faces []string
		for _, scope := range scopes {
			faces = append(faces, scope.Scope.String()+"/"+scope.Direction.String())
		}
		return faces
	}
	series, err := catalog.FindPricedScopesBinding(ctx, tenant, ports.PricingParameterReferenceSeries, "SYN-SERIES-FX")
	if err != nil {
		t.Fatal(err)
	}
	if got := describe(series); len(got) != 3 || got[0] != "SYN-SCOPE-A/BUY" || got[1] != "SYN-SCOPE-B/SELL" || got[2] != "SYN-SCOPE-D/INTERNAL" {
		t.Fatalf("绑序列的卡 = %v", got)
	}
	catalogue, err := catalog.FindPricedScopesBinding(ctx, tenant, ports.PricingParameterReferenceCatalogue, "SYN-CAT-ZONE")
	if err != nil {
		t.Fatal(err)
	}
	if got := describe(catalogue); len(got) != 1 || got[0] != "SYN-SCOPE-B/SELL" {
		t.Fatalf("绑目录的卡 = %v", got)
	}
	if unbound, err := catalog.FindPricedScopesBinding(ctx, tenant, ports.PricingParameterReferenceSeries, "SYN-SERIES-OTHER"); err != nil || len(unbound) != 0 {
		t.Fatalf("没有卡绑的序列 = %v err=%v", unbound, err)
	}
	if _, err := catalog.FindPricedScopesBinding(ctx, tenant, ports.PricingParameterPriceCard, "SYN-CARD-A"); err == nil {
		t.Fatal("价卡种类被当成绑定问了")
	}
}
