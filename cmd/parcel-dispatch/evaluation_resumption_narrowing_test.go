package main

import (
	"context"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 在生产路由表上证 ADR-0229 续办的收窄：参数登记只续办这一次登记真解得开的那几件——序列按绑了
// 它的卡的范围与方向收窄、且解不开缺区；结算账户、计费时点采用由各自真生产方交信封，续办门按停因与户接着推；重跑答
// `采用了别的时点`的发起停办落终止。夹具全是 `SYN-` 合成值，证据只记 S。

// fxBoundBuyCard 造一张绑了演示汇率序列的 BUY·供应商成本卡，放在自己的计价范围上：要证的是「序列只推绑了它的卡上
// 停着的请求」，卡上的价不重要。
func fxBoundBuyCard(t *testing.T, planID, scope string) ppdomain.PriceCardRegistration {
	t.Helper()
	binding, err := ppdomain.NewReferenceSeriesBinding(ppdomain.ReferenceSeriesExchangeRate, "SYN-SERIES-FX-CNY-SGD")
	if err != nil {
		t.Fatal(err)
	}
	structures, err := ppdomain.NewPricingPlanStructures(nil, nil, []ppdomain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatal(err)
	}
	return syntheticBuyCard(t, planID, scope, "SGD", structures)
}

// syntheticBuyCard 造一张 BUY·供应商成本卡：Z1 一行 [0, 30] 公斤 10，卡上绑什么由 structures 给。
func syntheticBuyCard(
	t *testing.T,
	planID, scope, currency string,
	structures ppdomain.PricingPlanStructures,
) ppdomain.PriceCardRegistration {
	t.Helper()
	base := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, planID, "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, planID+"-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, planID+"-WEIGHT", "v1"),
		Scope:                 scope,
		Direction:             ppdomain.PricingDirectionBuy,
		Purpose:               ppdomain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              currency,
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         planID + "-Z1",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "30",
		RateAmount:          "10",
		WeightRounding:      ppdomain.RoundingNone,
		WeightStepKilograms: "1",
	})
	plan, err := ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(),
		base.BaseChargeCode(), base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("卡 %s：%v", planID, err)
	}
	source, err := ppdomain.NewSourceFileIdentity("SYN-"+planID+".xlsx", strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactCommercialAuthorization, "SYN-AUTH-"+planID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := ppdomain.NewPriceCardRegistration(ppValue(t, ppdomain.NewTenantID, demoTenant), plan, source,
		authorization, "SYN-PRICING-GOVERNOR-01")
	if err != nil {
		t.Fatalf("价卡登记：%v", err)
	}
	return registration
}

func registerPriceCardForTest(t *testing.T, db *bentopg.DB, registration ppdomain.PriceCardRegistration) {
	t.Helper()
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	handler := ppapplication.NewRegisterPriceCardHandler(ppapplication.RegisterPriceCardDeps{
		Catalog: cards, Registered: pricingParameterHandoffForTest(t, db),
	})
	var outcome ppapplication.RegisterPriceCardOutcome
	mustWithinTX(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		var err error
		outcome, err = handler.Handle(ctx, ppapplication.RegisterPriceCardCommand{Registration: registration})
		return err
	})
	if outcome != ppapplication.PriceCardRecorded {
		t.Fatalf("登记 %s = %s", registration.Plan().Reference().ID(), outcome)
	}
}

// feedExchangeRateSeries 照演示汇率序列的取值登一版新版本并复核通过，走与喂价口同一条复核用例：复核通过那一笔交「参数
// 已登记」，按绑了这条序列的卡各交一面。
func feedExchangeRateSeries(t *testing.T, db *bentopg.DB, version string) {
	t.Helper()
	demo, err := ppdomain.RehydrateReferenceSeriesRegistration(readDemoPricingSeed(t, "reference-series-fx-cny-sgd.json"))
	if err != nil {
		t.Fatal(err)
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactReferenceSeries, demo.Reference().ID(), version)
	if err != nil {
		t.Fatal(err)
	}
	quoteBasis, _ := demo.QuoteBasis()
	series, err := ppdomain.NewReferenceSeriesRegistration(ppdomain.ReferenceSeriesRegistrationSpec{
		Tenant: demo.Tenant(), Kind: demo.Kind(), Reference: reference, SourceIdentifier: demo.SourceIdentifier(),
		Registrant: demo.Registrant(), QuoteBasis: quoteBasis, Periods: demo.Periods(),
	})
	if err != nil {
		t.Fatalf("汇率序列 %s：%v", version, err)
	}
	versions, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatal(err)
	}
	reviews, err := pppostgres.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatal(err)
	}
	register := ppapplication.NewRegisterReferenceSeriesHandler(ppapplication.RegisterReferenceSeriesDeps{Register: versions})
	review := ppapplication.NewReviewReferenceSeriesHandler(ppapplication.ReviewReferenceSeriesDeps{
		Versions: versions, Reviews: reviews, Clock: systemClock{}, Registered: pricingParameterHandoffForTest(t, db),
		PricedScopes: pricedScopesForTest(t, db),
	})
	var registered ppapplication.RegisterReferenceSeriesOutcome
	var reviewed ppapplication.ReviewReferenceSeriesOutcome
	mustWithinTX(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		var err error
		if registered, err = register.Handle(ctx, ppapplication.RegisterReferenceSeriesCommand{Registration: series}); err != nil {
			return err
		}
		reviewed, err = review.Handle(ctx, ppapplication.ReviewReferenceSeriesCommand{
			Tenant: demo.Tenant(), SeriesID: reference.ID(), SeriesVersion: version, Reviewer: "SYN-PRICING-REVIEW-01",
			Decision: ppdomain.SeriesReviewApproved, Basis: "SYN-REVIEW/" + version, ReviewedAt: beatInstant(),
		})
		return err
	})
	if registered != ppapplication.ReferenceSeriesRecorded || reviewed != ppapplication.SeriesReviewRecorded {
		t.Fatalf("喂汇率序列 %s：%s / %s", version, registered, reviewed)
	}
}

// stallBuyRequest 在 BUY 请求册上落一份首代请求并记停因：只要它的计价范围与停因，续办怎么推它由收窄口径定。
func stallBuyRequest(t *testing.T, db *bentopg.DB, id, scope, occurrence string, cause sadomain.EvaluationStopCause) {
	t.Helper()
	formed, err := sadomain.NewTransportChargeOccurrence(saValue(t, sadomain.NewChargeOccurrenceID, occurrence),
		saValue(t, sadomain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"), saValue(t, sadomain.NewOccurrenceVersion, "V1"),
		time.Date(2026, 10, 18, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	request, err := sadomain.SubmitEvaluationRequest(sadomain.EvaluationRequestSpec{
		ID:      saValue(t, sadomain.NewEvaluationRequestID, id),
		Scope:   saValue(t, sadomain.NewPrimaryScopeReference, scope),
		Purpose: sadomain.BuySupplierCost,
		Sources: sadomain.EligibleSourceReferences{
			Occurrence: formed,
			FeeItem:    saValue(t, sadomain.NewFeeItemReference, trunkFeeItem),
			Agreement:  saValue(t, sadomain.NewSupplierAgreementReference, trunkAgreementObject+"/v1"),
		},
		RequestedAt: beatInstant(),
		RequestedBy: saValue(t, sadomain.NewRequesterReference, "SYN-SETTLEMENT-JOB"),
	})
	if err != nil {
		t.Fatal(err)
	}
	requests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	key := saports.EvaluationRequestKey{TenantID: saValue(t, sadomain.NewTenantID, demoTenant), Request: request.ID()}
	mustWithinTX(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		if _, err := requests.Save(ctx, saports.EvaluationRequestRecord{Key: key, Request: request, RecordedAt: beatInstant()}); err != nil {
			return err
		}
		_, err := requests.RecordStop(ctx, key, cause, beatInstant())
		return err
	})
}

// Covers: AT-SA-189、AT-SA-193、AT-PP-004——序列喂进一期，结算只续办绑了这条序列的卡（同一路）上停着的、且是序列解得开的
// 请求：绑了汇率序列的那张 BUY 卡所在范围上缺换算步骤的请求续办出下一代；同一停因、范围上的卡没绑它的不动；缺区的请求
// 连着两期汇率喂进来都不动（序列解不开缺区）。
func TestAnExchangeRateFeedResumesOnlyWhatItsBindingCardsCanUnblock(t *testing.T) {
	fixture, beat, _ := buyResumptionFixture(t)
	registerDemoCostCard(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, fxBoundBuyCard(t, "SYN-PLAN-FX-COST-01", "SYN-SCOPE-FX"))
	drainBeats(t, beat)

	stallBuyRequest(t, fixture.db, "EVREQ-SYN-ZONE", "SYN-SCOPE-01", "SYN-OCC-ZONE", sadomain.StopEvaluationCatalogueUnresolved)
	stallBuyRequest(t, fixture.db, "EVREQ-SYN-FX", "SYN-SCOPE-FX", "SYN-OCC-FX", sadomain.StopConversionStepMissing)
	stallBuyRequest(t, fixture.db, "EVREQ-SYN-UNBOUND", "SYN-SCOPE-01", "SYN-OCC-UNBOUND", sadomain.StopConversionStepMissing)

	feedExchangeRateSeries(t, fixture.db, "v1")
	drainBeats(t, beat)
	if lineage := buyRequestsOf(t, fixture.db, "SYN-OCC-FX"); len(lineage) < 2 || lineage[1].previous != "EVREQ-SYN-FX" ||
		!strings.HasPrefix(lineage[1].basis, "PARAMREG/REFERENCE_SERIES/") {
		t.Fatalf("绑了汇率序列的范围上缺换算步骤的请求 = %+v，want 凭序列登记续办出下一代", lineage)
	}

	feedExchangeRateSeries(t, fixture.db, "v2")
	drainBeats(t, beat)
	for _, occurrence := range []string{"SYN-OCC-ZONE", "SYN-OCC-UNBOUND"} {
		if lineage := buyRequestsOf(t, fixture.db, occurrence); len(lineage) != 1 {
			t.Fatalf("%s 连着两期汇率喂进来之后 = %+v，want 仍只有停着的第一代", occurrence, lineage)
		}
	}
	assertNothingAbandoned(t, fixture.db, "parcel-pricing.pricing-parameter.registered")
}

// sellChainWithoutAccount 铺一件已接受、已固定闭包的包裹，结算账户与计费时点采用都还没登。
func sellChainWithoutAccount(t *testing.T) *synVerticalFixture {
	t.Helper()
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	seedSettledClosure(t, fixture)
	return fixture
}

func registerSellAdoptionForTest(t *testing.T, fixture *synVerticalFixture, store *outbox.Store, moment sadomain.ParcelBillingMoment) {
	t.Helper()
	handoff, err := sapostgres.NewOutboxEvaluationParameterRegisteredHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := sapostgres.NewSellBillingMomentAdoptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	register, err := saapplication.NewRegisterSellBillingMomentAdoptionHandler(adoptions, handoff, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	adoption, err := sadomain.NewSellBillingMomentAdoption(synRReceivableAccount(t).ID(), moment,
		mustPS(t, sadomain.NewEstimatePricingScope, sellChainPricingScope))
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustPS(t, sadomain.NewTenantID, demoTenant)
	mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
		_, err := register.Register(ctx, saapplication.RegisterSellBillingMomentAdoptionCommand{Tenant: tenant, Adoption: adoption})
		return err
	})
}

// Covers: AT-SA-194——「结算账户已登记」的真生产方驱动续办门：包裹计费时点到来时账户还没登，这一拍停在账户没登；登记账户
// 的编排同笔交信封，信封扇出到小包托运与结算两路、各记各的 inbox 账，结算那一路按停因重跑，链走到账户、停在计费时点
// 没采用并记下这一户；再登采用（按户收窄），发出首代请求。
func TestTheSettlementAccountRegisteredEnvelopeDrivesTheEvaluationResumption(t *testing.T) {
	fixture := sellChainWithoutAccount(t)
	recordAdoptedNetworkIntake(t, fixture)
	drainBeats(t, fixture.beat)
	tenant := mustPS(t, sadomain.NewTenantID, demoTenant)
	stalls, err := sapostgres.NewEvaluationResumptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	key := sadomain.SellEvaluationRequestNaturalKey{
		Parcel: mustPS(t, sadomain.NewParcelReference, "SYN-PARCEL-01"), Moment: sadomain.BillingMomentNetworkIntakeAdopted,
	}
	if stall, found, err := stalls.FindSellInitiationStall(t.Context(), tenant, key); err != nil || !found ||
		stall.Cause() != sadomain.StopSettlementAccountNotRegistered {
		t.Fatalf("账户没登时的发起停办 = %+v found=%v err=%v", stall, found, err)
	}

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := sapostgres.NewOutboxSettlementAccountRegisteredHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	register, err := saapplication.NewRegisterSettlementAccountHandler(accounts, handoff, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
		_, err := register.Register(ctx, saapplication.RegisterSettlementAccountCommand{Tenant: tenant, Account: synRReceivableAccount(t)})
		return err
	})
	drainBeats(t, fixture.beat)

	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(), `SELECT i.consumer, i.status FROM `+migrate.SchemaBento+`.inbox i
		JOIN `+migrate.SchemaBento+`.outbox o ON o.event_id = i.event_id
		WHERE o.event_type = $1`, string(sainbox.SettlementAccountRegisteredResumption.EventType))
	if err != nil {
		t.Fatal(err)
	}
	lanes := map[string]string{}
	for rows.Next() {
		var consumer, status string
		if err := rows.Scan(&consumer, &status); err != nil {
			t.Fatal(err)
		}
		lanes[consumer] = status
	}
	rows.Close()
	if len(lanes) != 2 || lanes[sainbox.SettlementAccountRegisteredResumption.ConsumerName] != "PROCESSED" {
		t.Fatalf("「结算账户已登记」的 inbox = %v，want 小包托运与结算两路各一行、结算那一路处理完毕", lanes)
	}
	stall, found, err := stalls.FindSellInitiationStall(t.Context(), tenant, key)
	account, reached := stall.Account()
	if err != nil || !found || stall.Cause() != sadomain.StopSellBillingMomentNotAdopted || !reached ||
		account != synRReceivableAccount(t).ID() || !strings.HasPrefix(stall.Basis().String(), "PARAMREG/SETTLEMENT_ACCOUNT/") {
		t.Fatalf("登了账户之后的发起停办 = %+v found=%v err=%v，want 停在计费时点没采用、记下这一户与账户登记的依据", stall, found, err)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("采用没登却落了 %d 份请求", n)
	}

	registerSellAdoptionForTest(t, fixture, store, sadomain.BillingMomentNetworkIntakeAdopted)
	drainBeats(t, fixture.beat)
	requests, err := sapostgres.NewSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	record, found, err := requests.FindByNaturalKey(t.Context(), tenant, key)
	if err != nil || !found || record.Request.Generation() != 1 ||
		!strings.HasPrefix(record.Request.ResumeBasis().String(), "PARAMREG/SELL_BILLING_MOMENT_ADOPTION/") {
		t.Fatalf("登了采用之后的请求 found=%v err=%v 依据 %q", found, err, record.Request.ResumeBasis())
	}
	assertNothingAbandoned(t, fixture.db,
		string(sainbox.SettlementAccountRegisteredResumption.EventType), "settlement-accounting.evaluation-parameter.registered")
}

// Covers: AT-SA-194 的终止那一格——停在计费时点没采用的那一拍，这一户补登的采用是另一种时点：重跑答`采用了别的时点`，这一拍
// 本就不计费，发起停办落终止、不发请求；运营人员再点名这件包裹续办，终止的那一拍不再进队列。
func TestAStallWhoseAccountAdoptsAnotherMomentIsClosed(t *testing.T) {
	fixture := newSellChainFixture(t, false)
	recordAdoptedNetworkIntake(t, fixture)
	drainBeats(t, fixture.beat)
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	registerSellAdoptionForTest(t, fixture, store, sadomain.BillingMomentEffectiveDeliveryFinalized)
	drainBeats(t, fixture.beat)

	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("采用的是别的时点却落了 %d 份请求", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_initiation_stall
		WHERE parcel_id = 'SYN-PARCEL-01' AND billing_moment = 'NETWORK_INTAKE_ADOPTED' AND closed_at IS NOT NULL`); n != 1 {
		t.Fatalf("落了终止的发起停办 %d 行，want 1", n)
	}
	assertNothingAbandoned(t, fixture.db, "settlement-accounting.evaluation-parameter.registered")

	stalls, err := sapostgres.NewEvaluationResumptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if items, err := stalls.ListStalled(t.Context(), mustPS(t, sadomain.NewTenantID, demoTenant), saports.StalledEvaluationQuery{
		Causes: sadomain.AllEvaluationStopCauses(), Parcel: mustPS(t, sadomain.NewParcelReference, "SYN-PARCEL-01"),
	}, 10); err != nil || len(items) != 0 {
		t.Fatalf("运营点名这件包裹 = %+v err=%v，want 终止的那一拍不再进队列", items, err)
	}
}

// Covers: AT-SA-189、AT-SA-195——卡没绑分区目录、发生项也没带分区，计价答目录读不出：这一格只有绑了目录的新一版卡解得开
// （没有目录信封会落到这张卡的范围上）。租户在同一范围上另登一个方案、绑了分区目录，价卡登记的信封按范围与方向续办出
// 第二代；第一代原样；第二代因旧卡退不了场停在适用冲突——两张卡是不同的定价方案，收尾口只在同一方案的两版之间收尾
// （ADR-0234 决定七）。登成同一方案的新一版再收尾就续办得出评价，见 TestARequestStalledOnAMissingWeightBandResumesOnceTheOldCardIsClosed。
func TestARequestStalledOnAnUnboundZoneCatalogueResumesOnACardThatBindsIt(t *testing.T) {
	fixture, beat, store := buyResumptionFixture(t)
	registerBuyTriggers(t, fixture.db, demoTenant)
	registerPriceCardForTest(t, fixture.db, syntheticBuyCard(t, "SYN-PLAN-UNBOUND-COST-01", "SYN-SCOPE-01", "CNY",
		ppdomain.PricingPlanStructures{}))
	drainBeats(t, beat)
	insertChargeOccurrence(t, fixture.db, demoTenant, "SYN-OCC-TRUNK-ZONE", tfdomain.ActualFulfillmentOccurrence, trunkAgreementObject+"/v1")
	enqueueOccurrenceFormed(t, fixture.db, store, "occ-trunk-zone", "SYN-OCC-TRUNK-ZONE")
	drainBeats(t, beat)
	stalled := buyRequestsOf(t, fixture.db, "SYN-OCC-TRUNK-ZONE")
	if len(stalled) != 1 || stalled[0].stopCause != "EVALUATION_CATALOGUE_UNRESOLVED" {
		t.Fatalf("卡没绑目录时的 BUY 请求册 = %+v，want 一份第一代、停在 EVALUATION_CATALOGUE_UNRESOLVED", stalled)
	}

	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	if err != nil {
		t.Fatal(err)
	}
	zoned, err := ppdomain.PricingPlanStructures{}.WithReferenceCatalogues(link)
	if err != nil {
		t.Fatal(err)
	}
	registerPriceCardForTest(t, fixture.db, syntheticBuyCard(t, "SYN-PLAN-ZONED-COST-01", "SYN-SCOPE-01", "CNY", zoned))
	drainBeats(t, beat)
	lineage := buyRequestsOf(t, fixture.db, "SYN-OCC-TRUNK-ZONE")
	if len(lineage) < 2 || lineage[0].stopCause != "EVALUATION_CATALOGUE_UNRESOLVED" || lineage[1].previous != lineage[0].id ||
		!strings.HasPrefix(lineage[1].basis, "PARAMREG/PRICE_CARD/") {
		t.Fatalf("登了绑目录的卡之后 = %+v，want 第一代原样、凭价卡登记续办出第二代", lineage)
	}
	// 旧卡退不了场：同一范围上两个方案同时适用，第二代照实停在适用冲突；让位给另一方案的退役归 ADR-0234 越权风险点 5 记下
	// 的后继票。
	if lineage[1].stopCause != "PRICE_CARD_APPLICABILITY_CONFLICT" {
		t.Fatalf("第二代停因 = %q，want PRICE_CARD_APPLICABILITY_CONFLICT", lineage[1].stopCause)
	}
	assertNothingAbandoned(t, fixture.db, "parcel-pricing.pricing-parameter.registered", "parcel-pricing.evaluation-request.unformed")
}
