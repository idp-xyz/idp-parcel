package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 票 routing-first-cut/20 的真库取证（ADR-0189 决定一第一条）：候选成本取数侧经生产装配走 parcel-pricing「对给定价卡形成
// 试算」的入口，补齐读数与正式评价、受理前估价同一种装法。三格：绑了分区目录的卡经包裹邮编路线读区出价；没绑目录、也没登段
// 计价区域的卡仍待判断；要按汇率序列换算的卡在序列有在用版本时出价。
//
// 卡都在用例里造，与演示成本卡同身份（演示线路三段都引 SYN-PLAN-CN-SG-COST-01/v1）、同档位，只换币种与方案结构：要证的是
// 「卡绑了什么、补齐读数补不补得上」，不随演示种子的取值走。

// demoShapedCostCard 造一张与演示成本卡同身份、同档位的 BUY·供应商成本卡：Z1 三档（[0,1) 30、[1,5) 85、[5,30) 260），
// 实重计费、按 0.5 KG 向上进位，总额按 0.01 取整。币种与额外的方案结构（目录或序列绑定）由用例给。
func demoShapedCostCard(t *testing.T, code string, structures ppdomain.PricingPlanStructures) ppdomain.PriceCardRegistration {
	t.Helper()
	identity := func(kind ppdomain.ArtifactKind, id string) ppdomain.VersionReference {
		reference, err := ppdomain.NewVersionReferenceIdentity(kind, id, "v1")
		if err != nil {
			t.Fatalf("引用 %s：%v", id, err)
		}
		return reference
	}
	kilogram := ppdomain.WeightUnitKilogram
	weight := func(raw string) ppdomain.Weight {
		value, err := ppdomain.NewWeightFromString(raw, kilogram)
		if err != nil {
			t.Fatalf("重量 %s：%v", raw, err)
		}
		return value
	}
	currency := ppValue(t, ppdomain.NewCurrency, code)
	money := func(raw string) ppdomain.Money {
		value, err := ppdomain.NewMoneyFromString(raw, currency)
		if err != nil {
			t.Fatalf("金额 %s：%v", raw, err)
		}
		return value
	}
	period, err := ppdomain.NewUnboundedEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("适用期：%v", err)
	}
	entries := make([]ppdomain.RateEntry, 0, 3)
	for _, bracket := range []struct{ id, lower, upper, amount string }{
		{"SYN-COST-Z1-A", "0", "1", "30"},
		{"SYN-COST-Z1-B", "1", "5", "85"},
		{"SYN-COST-Z1-C", "5", "30", "260"},
	} {
		entry, err := ppdomain.NewRateEntry(ppValue(t, ppdomain.NewRateEntryID, bracket.id), "Z1",
			weight(bracket.lower), weight(bracket.upper), money(bracket.amount))
		if err != nil {
			t.Fatalf("档位 %s：%v", bracket.id, err)
		}
		entries = append(entries, entry)
	}
	table, err := ppdomain.NewRateTableVersion(identity(ppdomain.ArtifactRateTable, "SYN-TABLE-CN-SG-COST-01"),
		ppdomain.RateTableFamilyWeightZone, currency, kilogram, period, entries)
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, weight("0.5"))
	if err != nil {
		t.Fatalf("计费重进位：%v", err)
	}
	weightPolicy, err := ppdomain.NewPricingWeightPolicy(identity(ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT-CN-SG-COST-01"),
		ppdomain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("计费重策略：%v", err)
	}
	amountRounding, err := ppdomain.NewAmountRoundingPolicy(
		ppdomain.RoundingHalfUp, money("0.01"), []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal})
	if err != nil {
		t.Fatalf("金额取整：%v", err)
	}
	if structures, err = structures.WithAmountRounding(amountRounding); err != nil {
		t.Fatalf("方案结构声明金额取整：%v", err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(identity(ppdomain.ArtifactPricingPlan, "SYN-PLAN-CN-SG-COST-01"),
		ppValue(t, ppdomain.NewPricingScopeID, "SYN-SCOPE-01"), ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost,
		ppValue(t, ppdomain.NewChargeCode, "BASE_FREIGHT"), period, table, weightPolicy, nil, structures)
	if err != nil {
		t.Fatalf("成本方案：%v", err)
	}
	sourceName := "SYN-CARD-CN-SG-COST-RFC20.xlsx"
	digest := sha256.Sum256([]byte(sourceName))
	source, err := ppdomain.NewSourceFileIdentity(sourceName, hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatalf("源文件身份：%v", err)
	}
	registration, err := ppdomain.NewPriceCardRegistration(ppValue(t, ppdomain.NewTenantID, demoTenant), plan, source,
		identity(ppdomain.ArtifactCommercialAuthorization, "SYN-AUTH-COST-DIR-01"), "SYN-PRICING-GOVERNOR-01")
	if err != nil {
		t.Fatalf("价卡登记：%v", err)
	}
	return registration
}

func zoneCatalogueBound(t *testing.T) ppdomain.PricingPlanStructures {
	t.Helper()
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	if err != nil {
		t.Fatalf("分区目录绑定：%v", err)
	}
	structures, err := ppdomain.PricingPlanStructures{}.WithReferenceCatalogues(link)
	if err != nil {
		t.Fatalf("方案结构绑分区目录：%v", err)
	}
	return structures
}

func exchangeRateBound(t *testing.T) ppdomain.PricingPlanStructures {
	t.Helper()
	binding, err := ppdomain.NewReferenceSeriesBinding(ppdomain.ReferenceSeriesExchangeRate, "SYN-SERIES-FX-CNY-SGD")
	if err != nil {
		t.Fatalf("汇率序列绑定：%v", err)
	}
	structures, err := ppdomain.NewPricingPlanStructures(nil, nil, []ppdomain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("方案结构绑汇率序列：%v", err)
	}
	return structures
}

// registerCostCard 走与 parcel-pricing-register 同一条登记用例。
func registerCostCard(t *testing.T, db *bentopg.DB, registration ppdomain.PriceCardRegistration) {
	t.Helper()
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡库：%v", err)
	}
	handler := ppapplication.NewRegisterPriceCardHandler(ppapplication.RegisterPriceCardDeps{Catalog: cards})
	var outcome ppapplication.RegisterPriceCardOutcome
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		result, registerErr := handler.Handle(ctx, ppapplication.RegisterPriceCardCommand{Registration: registration})
		outcome = result
		return registerErr
	})
	if err != nil || outcome != ppapplication.PriceCardRecorded {
		t.Fatalf("登记成本卡：outcome=%s err=%v", outcome, err)
	}
}

// readDemoPricingSeed 读演示种子计价目录里的一份。
func readDemoPricingSeed(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(demoSellPricing, name))
	if err != nil {
		t.Fatalf("读演示种子 %s：%v", name, err)
	}
	return raw
}

// registerDemoZoneCatalogue 照演示种子登分区目录 SYN-CAT-ZONE-CN-SG 及其复核：不复核就不在用（ADR-0109）。
func registerDemoZoneCatalogue(t *testing.T, db *bentopg.DB) {
	t.Helper()
	catalogues, err := pppostgres.NewReferenceCatalogueVersions(db)
	if err != nil {
		t.Fatalf("构造目录登记册：%v", err)
	}
	reviews, err := pppostgres.NewReferenceCatalogueReviews(db)
	if err != nil {
		t.Fatalf("构造目录复核册：%v", err)
	}
	zones, err := ppdomain.RehydrateReferenceCatalogueRegistration(readDemoPricingSeed(t, "reference-catalogue-zone-cn-sg.json"))
	if err != nil {
		t.Fatalf("重建分区目录：%v", err)
	}
	register := ppapplication.NewRegisterReferenceCatalogueHandler(ppapplication.RegisterReferenceCatalogueDeps{Register: catalogues})
	var review struct {
		Tenant, CatalogueID, CatalogueVersion, Reviewer, Decision, Basis, ReviewedAt string
	}
	decodeSeedDocument(t, readDemoPricingSeed(t, "reference-catalogue-zone-cn-sg-review.json"), &review)
	reviewHandler := ppapplication.NewReviewReferenceCatalogueHandler(ppapplication.ReviewReferenceCatalogueDeps{
		Versions: catalogues, Reviews: reviews, Clock: systemClock{},
	})
	var registered ppapplication.RegisterReferenceCatalogueOutcome
	var reviewed ppapplication.ReviewReferenceCatalogueOutcome
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var err error
		if registered, err = register.Handle(ctx, ppapplication.RegisterReferenceCatalogueCommand{Registration: zones}); err != nil {
			return err
		}
		reviewed, err = reviewHandler.Handle(ctx, ppapplication.ReviewReferenceCatalogueCommand{
			Tenant:           ppValue(t, ppdomain.NewTenantID, review.Tenant),
			CatalogueID:      review.CatalogueID,
			CatalogueVersion: review.CatalogueVersion,
			Reviewer:         review.Reviewer,
			Decision:         ppdomain.SeriesReviewDecision(review.Decision),
			Basis:            review.Basis,
			ReviewedAt:       seedInstant(t, review.ReviewedAt),
		})
		return err
	})
	if err != nil || registered != ppapplication.ReferenceCatalogueRecorded || reviewed != ppapplication.CatalogueReviewRecorded {
		t.Fatalf("登记分区目录及复核：%s / %s err=%v", registered, reviewed, err)
	}
}

// registerDemoExchangeRateSeries 照演示种子登汇率序列 SYN-SERIES-FX-CNY-SGD 及其复核：2026-07-01 起一期 5.35。
func registerDemoExchangeRateSeries(t *testing.T, db *bentopg.DB) {
	t.Helper()
	versions, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造序列登记册：%v", err)
	}
	reviews, err := pppostgres.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatalf("构造序列复核册：%v", err)
	}
	series, err := ppdomain.RehydrateReferenceSeriesRegistration(readDemoPricingSeed(t, "reference-series-fx-cny-sgd.json"))
	if err != nil {
		t.Fatalf("重建汇率序列：%v", err)
	}
	register := ppapplication.NewRegisterReferenceSeriesHandler(ppapplication.RegisterReferenceSeriesDeps{Register: versions})
	var review struct {
		Tenant, SeriesID, SeriesVersion, Reviewer, Decision, Basis, ReviewedAt string
	}
	decodeSeedDocument(t, readDemoPricingSeed(t, "reference-series-fx-cny-sgd-review.json"), &review)
	reviewHandler := ppapplication.NewReviewReferenceSeriesHandler(ppapplication.ReviewReferenceSeriesDeps{
		Versions: versions, Reviews: reviews, Clock: systemClock{},
	})
	var registered ppapplication.RegisterReferenceSeriesOutcome
	var reviewed ppapplication.ReviewReferenceSeriesOutcome
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var err error
		if registered, err = register.Handle(ctx, ppapplication.RegisterReferenceSeriesCommand{Registration: series}); err != nil {
			return err
		}
		reviewed, err = reviewHandler.Handle(ctx, ppapplication.ReviewReferenceSeriesCommand{
			Tenant:        ppValue(t, ppdomain.NewTenantID, review.Tenant),
			SeriesID:      review.SeriesID,
			SeriesVersion: review.SeriesVersion,
			Reviewer:      review.Reviewer,
			Decision:      ppdomain.SeriesReviewDecision(review.Decision),
			Basis:         review.Basis,
			ReviewedAt:    seedInstant(t, review.ReviewedAt),
		})
		return err
	})
	if err != nil || registered != ppapplication.ReferenceSeriesRecorded || reviewed != ppapplication.SeriesReviewRecorded {
		t.Fatalf("登记汇率序列及复核：%s / %s err=%v", registered, reviewed, err)
	}
}

// acceptDeclaredDemoParcel 让 SYN-PARCEL-01 带客户声明 2.5 KG 经受理链接受：包裹事实取数侧读的就是这份声明。
func acceptDeclaredDemoParcel(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	ctx := t.Context()
	command := fixture.submitCommand(t)
	command.DeclaredProfiles = []psdomain.DeclaredParcelProfile{declaredProfile(t, "SYN-PARCEL-01", "2.5", "KG")}
	var submitted psapplication.SubmitOutcome
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		result, err := fixture.submitHandler.Handle(txCtx, command)
		submitted = result.Outcome()
		return err
	})
	if submitted != psapplication.OutcomeSubmitted {
		t.Fatalf("submit outcome = %q, want SUBMITTED", submitted)
	}
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
}

// carriedPostalRoute 是随初始路由请求携带的投影：寄件 CN 200000，收件 SG 018000——目的前缀 018 落演示分区目录的 Z1，寄件
// 前缀 200 在目录的始发集里。
func carriedPostalRoute() nrports.RequestCarriedContent {
	return nrports.RequestCarriedContent{Geo: nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide("CN", true, "200000", true),
		nrdomain.NewGeoResolutionSide("SG", true, "018000", true))}
}

// demoCandidateCost 取演示候选恰一条的成本事实与出处条数。
func demoCandidateCost(t *testing.T, costs nrports.RouteCandidateCosts) nrdomain.CandidateCostFact {
	t.Helper()
	if len(costs.Facts) != 1 || costs.Facts[0].Candidate().String() != "SYN-LINE-CN-SG-01@1" {
		t.Fatalf("成本事实 = %+v，want 演示候选恰一条", costs.Facts)
	}
	return costs.Facts[0]
}

// Covers: `AT-NR-070` 真库那一格——绑了分区目录 SYN-CAT-ZONE-CN-SG 的 BUY 成本卡，经随请求携带的邮编路线 200000 → 018000 从
// 在用目录读出 Z1，三段各 85 CNY、合 25500 分，三段各一条出处。演示网络照种子登了三段各 Z1 的段计价区域，反事实那一半证
// 它不是出价的来由：请求不带收件邮编时同一张卡三段都待判断（`AT-NR-071`），登的区没有顶上。
func TestABoundCostCardIsPricedThroughThePostalRouteInProductionWiring(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	acceptDeclaredDemoParcel(t, fixture)
	registerDemoNetworkSeeds(t, fixture.db)
	registerCostCard(t, fixture.db, demoShapedCostCard(t, "CNY", zoneCatalogueBound(t)))
	registerDemoZoneCatalogue(t, fixture.db)
	seedWiringCustomsCatalog(t, fixture.db, demoTenant, true)
	evidence := loadWiringEvidence(t, fixture.db, demoTenant)
	key := wiringRouteKey(t, demoTenant)

	costs, err := routeCosts(fixture.db, systemClock{}).LoadCandidateCosts(t.Context(), key, evidence, carriedPostalRoute())
	if err != nil {
		t.Fatalf("候选成本取数侧：err = %v，want 交回成本事实", err)
	}
	if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 25500 {
		t.Fatalf("演示候选成本 = %s %d minor，want 已出价 25500（目录解出 Z1，三段各 85 CNY）", fact.State(), fact.AmountMinor())
	}
	if len(costs.Citations) != 3 {
		t.Fatalf("出处 %d 条，want 三段各一条：%+v", len(costs.Citations), costs.Citations)
	}
	for _, citation := range costs.Citations {
		if citation.PlanReference() != "SYN-PLAN-CN-SG-COST-01/v1" || !strings.HasPrefix(citation.EvaluationID(), "EST-") {
			t.Fatalf("出处 = %+v，want 指那张绑了目录的卡、评价标识出自 PP 的试算入口", citation)
		}
	}

	costs, err = routeCosts(fixture.db, systemClock{}).LoadCandidateCosts(t.Context(), key, evidence, wiringCarried())
	if err != nil {
		t.Fatalf("候选成本取数侧（不带收件邮编）：%v", err)
	}
	if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPending || len(costs.Citations) != 0 {
		t.Fatalf("不带收件邮编：成本 = %s，出处 %d 条，want 待判断、没有出处（登的 Z1 不顶上）", fact.State(), len(costs.Citations))
	}
}

// Covers: `AT-NR-071` 真库那一格（判据「没绑目录、也没登段计价区域的卡仍待判断」）——演示网络照种子登，只略过段计价区域那一份；卡没绑目录。
// 请求带了邮编路线也一样：路线只给绑了目录的卡解区，候选缺成本依据，不整判断停下。
func TestAnUnboundCostCardWithoutALegZoneStaysPendingInProductionWiring(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	acceptDeclaredDemoParcel(t, fixture)
	registerDemoNetworkSeedsKeeping(t, fixture.db, func(name string) bool {
		return !strings.Contains(name, "-leg-pricing-zones-")
	})
	registerCostCard(t, fixture.db, demoShapedCostCard(t, "CNY", ppdomain.PricingPlanStructures{}))
	registerDemoZoneCatalogue(t, fixture.db)
	seedWiringCustomsCatalog(t, fixture.db, demoTenant, true)
	evidence := loadWiringEvidence(t, fixture.db, demoTenant)

	costs, err := routeCosts(fixture.db, systemClock{}).LoadCandidateCosts(
		t.Context(), wiringRouteKey(t, demoTenant), evidence, carriedPostalRoute())
	if err != nil {
		t.Fatalf("候选成本取数侧：err = %v，want 交回成本事实", err)
	}
	if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPending || len(costs.Citations) != 0 {
		t.Fatalf("演示候选成本 = %s，出处 %d 条，want 待判断、没有出处", fact.State(), len(costs.Citations))
	}
}

// Covers: 票 routing-first-cut/18 判据与票 20 完成判据二（ADR-0189 决定一第二条）——卡没绑目录，演示网络照种子登了三段各 Z1
// 的段计价区域：三段各 85 CNY、合 25500 分，三段各一条出处。与上一条只差段计价区域那一份登没登，没绑目录的卡就从待判断变成
// 出价。请求带不带收件邮编都一样出价：这张卡的区出自登的段计价区域，邮编路线与在用目录对它不起作用。
func TestAnUnboundCostCardIsPricedThroughItsRegisteredLegZonesInProductionWiring(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	acceptDeclaredDemoParcel(t, fixture)
	registerDemoNetworkSeeds(t, fixture.db)
	registerCostCard(t, fixture.db, demoShapedCostCard(t, "CNY", ppdomain.PricingPlanStructures{}))
	registerDemoZoneCatalogue(t, fixture.db)
	seedWiringCustomsCatalog(t, fixture.db, demoTenant, true)
	evidence := loadWiringEvidence(t, fixture.db, demoTenant)
	key := wiringRouteKey(t, demoTenant)

	for name, carried := range map[string]nrports.RequestCarriedContent{
		"带邮编路线":  carriedPostalRoute(),
		"不带收件邮编": wiringCarried(),
	} {
		costs, err := routeCosts(fixture.db, systemClock{}).LoadCandidateCosts(t.Context(), key, evidence, carried)
		if err != nil {
			t.Fatalf("%s：候选成本取数侧 err = %v，want 交回成本事实", name, err)
		}
		if fact := demoCandidateCost(t, costs); fact.State() != nrdomain.CandidateCostPriced || fact.AmountMinor() != 25500 {
			t.Fatalf("%s：演示候选成本 = %s %d minor，want 已出价 25500（段计价区域三段各 Z1，各 85 CNY）",
				name, fact.State(), fact.AmountMinor())
		}
		if len(costs.Citations) != 3 {
			t.Fatalf("%s：出处 %d 条，want 三段各一条：%+v", name, len(costs.Citations), costs.Citations)
		}
		for _, citation := range costs.Citations {
			if citation.PlanReference() != "SYN-PLAN-CN-SG-COST-01/v1" || !strings.HasPrefix(citation.EvaluationID(), "EST-") {
				t.Fatalf("%s：出处 = %+v，want 指那张没绑目录的卡、评价标识出自 PP 的试算入口", name, citation)
			}
		}
	}
}

// Covers: `AT-NR-072` 真库那一格——卡以 SGD 计价、绑汇率序列 SYN-SERIES-FX-CNY-SGD，比较币种 CNY。序列登了且复核过（在用），
// 补齐读数按计价时点取 2026-07-01 起那一期 5.35：三段各 85 SGD 折 454.75 CNY，合 1364.25 CNY，即 136425 分。序列没登时
// 同一张卡三段都待判断，不编汇率。比较币种是路由策略版本上的租户取值，演示策略没声明，用例在证据上直接给。
func TestAnExchangeRateBoundCostCardIsPricedWhenTheSeriesIsInForce(t *testing.T) {
	for name, test := range map[string]struct {
		registerSeries bool
		state          nrdomain.CandidateCostState
		amountMinor    int64
	}{
		"序列在用": {true, nrdomain.CandidateCostPriced, 136425},
		"序列没登": {false, nrdomain.CandidateCostPending, 0},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newSYNVerticalFixture(t)
			acceptDeclaredDemoParcel(t, fixture)
			registerDemoNetworkSeeds(t, fixture.db)
			registerCostCard(t, fixture.db, demoShapedCostCard(t, "SGD", exchangeRateBound(t)))
			if test.registerSeries {
				registerDemoExchangeRateSeries(t, fixture.db)
			}
			seedWiringCustomsCatalog(t, fixture.db, demoTenant, true)
			evidence := loadWiringEvidence(t, fixture.db, demoTenant)
			evidence.HasComparisonCurrency, evidence.ComparisonCurrency = true, "CNY"

			costs, err := routeCosts(fixture.db, systemClock{}).LoadCandidateCosts(
				t.Context(), wiringRouteKey(t, demoTenant), evidence, carriedPostalRoute())
			if err != nil {
				t.Fatalf("候选成本取数侧：err = %v，want 交回成本事实", err)
			}
			fact := demoCandidateCost(t, costs)
			if fact.State() != test.state || (test.state == nrdomain.CandidateCostPriced && fact.AmountMinor() != test.amountMinor) {
				t.Fatalf("演示候选成本 = %s %d minor，want %s %d", fact.State(), fact.AmountMinor(), test.state, test.amountMinor)
			}
			if test.state == nrdomain.CandidateCostPriced {
				for _, citation := range costs.Citations {
					if citation.ComparisonCurrency() != "CNY" || citation.ComparisonAmount() != "454.75" {
						t.Fatalf("出处 = %+v，want 每段全精度比较金额 454.75 CNY", citation)
					}
				}
			}
		})
	}
}
