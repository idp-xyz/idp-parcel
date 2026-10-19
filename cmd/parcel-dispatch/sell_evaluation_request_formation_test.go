package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppsettlement "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/settlementaccounting"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件钉路由表的 SA SELL 评价请求已提交一条（ADR-0190 决定四；票 sell-customer-charge/03 完成判据「真库装配」）：在生产
// 依赖图上，演示租户一件申报了重量、尺寸与寄收邮编的包裹经受理链接受，演示售价卡 SYN-PLAN-CN-SG-01 连同它绑的燃油序列与
// 分区目录 SYN-CAT-ZONE-CN-SG 照演示种子登记并复核；SA 用真请求册与真 Outbox 适配器落一份 SELL 请求、交一封信封。一拍之后
// PP 的 SELL 消费者按标识回读请求，按包裹建快照、选到那张卡，形成一份回指请求的 SELL·客户费用评价，并交出评价已记录信封。
// 信封类型由 SA 的交接口自己写，路由表按 PP 消费方自写的常量认，两串相等由这一拍在真库上钉住。证据只记 S。

// sellBasisAt 是两份请求的计价基准时点：落在演示售价卡、燃油序列与分区目录的在用期里。
var sellBasisAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

// demoSellPricing 是演示种子的计价目录。SELL 这一半要登的是卡、它绑的燃油序列与分区目录，以及两者的复核：不复核就
// 不在用，评价会如实挂起——这里要的是一份能成形的评价，所以复核一并登。
var demoSellPricing = filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")

// registerDemoSellPricing 照演示种子走与 parcel-pricing-register 同一条重建与登记用例。
func registerDemoSellPricing(t *testing.T, db *bentopg.DB) {
	t.Helper()
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(demoSellPricing, name))
		if err != nil {
			t.Fatalf("读演示种子 %s：%v", name, err)
		}
		return raw
	}
	within := func(name string, register func(ctx context.Context) (string, error), want string) {
		t.Helper()
		var outcome string
		err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			var registerErr error
			outcome, registerErr = register(ctx)
			return registerErr
		})
		if err != nil || outcome != want {
			t.Fatalf("登记 %s：outcome=%s err=%v", name, outcome, err)
		}
	}

	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡库：%v", err)
	}
	series, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造序列登记册：%v", err)
	}
	seriesReviews, err := pppostgres.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatalf("构造序列复核册：%v", err)
	}
	catalogues, err := pppostgres.NewReferenceCatalogueVersions(db)
	if err != nil {
		t.Fatalf("构造目录登记册：%v", err)
	}
	catalogueReviews, err := pppostgres.NewReferenceCatalogueReviews(db)
	if err != nil {
		t.Fatalf("构造目录复核册：%v", err)
	}

	card, err := ppdomain.RehydratePriceCardRegistration(read("price-card-cn-sg.json"))
	if err != nil {
		t.Fatalf("重建演示售价卡：%v", err)
	}
	registerCard := ppapplication.NewRegisterPriceCardHandler(ppapplication.RegisterPriceCardDeps{Catalog: cards})
	within("演示售价卡", func(ctx context.Context) (string, error) {
		outcome, err := registerCard.Handle(ctx, ppapplication.RegisterPriceCardCommand{Registration: card})
		return outcome.String(), err
	}, ppapplication.PriceCardRecorded.String())

	fuel, err := ppdomain.RehydrateReferenceSeriesRegistration(read("reference-series-fuel.json"))
	if err != nil {
		t.Fatalf("重建燃油序列：%v", err)
	}
	registerSeries := ppapplication.NewRegisterReferenceSeriesHandler(ppapplication.RegisterReferenceSeriesDeps{Register: series})
	within("燃油序列", func(ctx context.Context) (string, error) {
		outcome, err := registerSeries.Handle(ctx, ppapplication.RegisterReferenceSeriesCommand{Registration: fuel})
		return outcome.String(), err
	}, ppapplication.ReferenceSeriesRecorded.String())

	var seriesReview struct {
		Tenant, SeriesID, SeriesVersion, Reviewer, Decision, Basis, ReviewedAt string
	}
	decodeSeedDocument(t, read("reference-series-fuel-review.json"), &seriesReview)
	reviewSeries := ppapplication.NewReviewReferenceSeriesHandler(ppapplication.ReviewReferenceSeriesDeps{
		Versions: series, Reviews: seriesReviews, Clock: systemClock{},
	})
	within("燃油序列复核", func(ctx context.Context) (string, error) {
		outcome, err := reviewSeries.Handle(ctx, ppapplication.ReviewReferenceSeriesCommand{
			Tenant:        ppValue(t, ppdomain.NewTenantID, seriesReview.Tenant),
			SeriesID:      seriesReview.SeriesID,
			SeriesVersion: seriesReview.SeriesVersion,
			Reviewer:      seriesReview.Reviewer,
			Decision:      ppdomain.SeriesReviewDecision(seriesReview.Decision),
			Basis:         seriesReview.Basis,
			ReviewedAt:    seedInstant(t, seriesReview.ReviewedAt),
		})
		return outcome.String(), err
	}, ppapplication.SeriesReviewRecorded.String())

	zones, err := ppdomain.RehydrateReferenceCatalogueRegistration(read("reference-catalogue-zone-cn-sg.json"))
	if err != nil {
		t.Fatalf("重建分区目录：%v", err)
	}
	registerCatalogue := ppapplication.NewRegisterReferenceCatalogueHandler(ppapplication.RegisterReferenceCatalogueDeps{Register: catalogues})
	within("分区目录", func(ctx context.Context) (string, error) {
		outcome, err := registerCatalogue.Handle(ctx, ppapplication.RegisterReferenceCatalogueCommand{Registration: zones})
		return outcome.String(), err
	}, ppapplication.ReferenceCatalogueRecorded.String())

	var catalogueReview struct {
		Tenant, CatalogueID, CatalogueVersion, Reviewer, Decision, Basis, ReviewedAt string
	}
	decodeSeedDocument(t, read("reference-catalogue-zone-cn-sg-review.json"), &catalogueReview)
	reviewCatalogue := ppapplication.NewReviewReferenceCatalogueHandler(ppapplication.ReviewReferenceCatalogueDeps{
		Versions: catalogues, Reviews: catalogueReviews, Clock: systemClock{},
	})
	within("分区目录复核", func(ctx context.Context) (string, error) {
		outcome, err := reviewCatalogue.Handle(ctx, ppapplication.ReviewReferenceCatalogueCommand{
			Tenant:           ppValue(t, ppdomain.NewTenantID, catalogueReview.Tenant),
			CatalogueID:      catalogueReview.CatalogueID,
			CatalogueVersion: catalogueReview.CatalogueVersion,
			Reviewer:         catalogueReview.Reviewer,
			Decision:         ppdomain.SeriesReviewDecision(catalogueReview.Decision),
			Basis:            catalogueReview.Basis,
			ReviewedAt:       seedInstant(t, catalogueReview.ReviewedAt),
		})
		return outcome.String(), err
	}, ppapplication.CatalogueReviewRecorded.String())
}

// decodeSeedDocument 按键名不分大小写解复核文档：种子由 seedgen 写成小驼峰，与匿名结构的字段名同词。
func decodeSeedDocument(t *testing.T, raw []byte, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("解复核文档：%v", err)
	}
}

func seedInstant(t *testing.T, raw string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("复核时刻 %q：%v", raw, err)
	}
	return at
}

func ppValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// acceptedSellParcel 让演示租户的 SYN-PARCEL-01 经受理链接受：申报 1.2 KG、30×20×10 CM，寄件 200001、收件 018956。
// 收件邮编前三位落分区目录的 Z1，寄件前缀在目录的始发集里；单位用计价单位表认的大写原词。
func acceptedSellParcel(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	ctx := t.Context()
	weight, err := psdomain.NewDeclaredWeight(
		mustPS(t, psdomain.NewMeasurementValue, "1.2"), mustPS(t, psdomain.NewMeasurementUnitReference, "KG"))
	if err != nil {
		t.Fatalf("申报重量：%v", err)
	}
	sides, err := psdomain.NewDeclaredDimensions(
		mustPS(t, psdomain.NewMeasurementValue, "30"), mustPS(t, psdomain.NewMeasurementValue, "20"),
		mustPS(t, psdomain.NewMeasurementValue, "10"), mustPS(t, psdomain.NewMeasurementUnitReference, "CM"))
	if err != nil {
		t.Fatalf("申报尺寸：%v", err)
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, sides)
	if err != nil {
		t.Fatalf("申报测量：%v", err)
	}
	profile, err := psdomain.NewDeclaredParcelProfile(mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"), measurement)
	if err != nil {
		t.Fatalf("申报画像：%v", err)
	}
	postal := func(group psdomain.SourceDataGroupReference, code string) psdomain.AddressElements {
		entry, err := psdomain.NewCanonicalContentEntry(psdomain.AddressElementEntryName(group, psdomain.PostalCodeElement), code)
		if err != nil {
			t.Fatalf("邮编要素：%v", err)
		}
		return psdomain.AddressElementsOf(group, []psdomain.CanonicalContentEntry{entry})
	}

	command := fixture.submitCommand(t)
	command.DeclaredProfiles = []psdomain.DeclaredParcelProfile{profile}
	command.DeclaredElements = psdomain.NewDeclaredAddressElements(
		postal(psdomain.SenderPlaceDataGroup(), "200001"),
		postal(psdomain.DeliveryPlaceDataGroup(), "018956"),
	)
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

// sellRequestRecord 造一份演示租户在`有效网络收寄已采用`那一拍的 SELL 评价请求记录。
func sellRequestRecord(t *testing.T, id, parcel, scope string) saports.SellEvaluationRequestRecord {
	t.Helper()
	request, err := sadomain.SubmitSellEvaluationRequest(sadomain.SellEvaluationRequestSpec{
		ID:                 saValue(t, sadomain.NewSellEvaluationRequestID, id),
		Parcel:             saValue(t, sadomain.NewParcelReference, parcel),
		Account:            saValue(t, sadomain.NewSettlementAccountID, "SYN-SA-SHIPPER-01-RECEIVABLE"),
		SettlementCurrency: saValue(t, sadomain.NewCurrencyCode, "CNY"),
		PricingScope:       saValue(t, sadomain.NewEstimatePricingScope, scope),
		Moment:             sadomain.BillingMomentNetworkIntakeAdopted,
		Source:             saValue(t, sadomain.NewSourceFactReference, "SYN-SOURCE-FACT-"+id),
		PricingBasisAt:     sellBasisAt,
	})
	if err != nil {
		t.Fatalf("SELL 评价请求：%v", err)
	}
	return saports.SellEvaluationRequestRecord{
		Key:        saports.SellEvaluationRequestKey{TenantID: saValue(t, sadomain.NewTenantID, demoTenant), Request: request.ID()},
		Request:    request,
		RecordedAt: beatInstant(),
		State:      sadomain.SellRequestSubmitted,
	}
}

// Covers: 一封 SELL 请求信封经生产路由表形成一份评价、发出评价已记录信封（正）；另一份请求的计价范围下没有卡，停在
// 未配置、未决、零写入，原文说的是缺价卡（反）。
func TestASellEvaluationRequestFormsACustomerChargeEvaluationThroughTheRouteTable(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	acceptedSellParcel(t, fixture)

	observed := map[string]error{}
	purpose, err := nrdomain.NewServicePurpose("NETWORK_SERVICE")
	if err != nil {
		t.Fatalf("服务目的：%v", err)
	}
	beat, err := wireDispatcher(fixture.db, dispatchSettings{
		purpose:         purpose,
		deliveryTimeout: 5 * time.Second,
		config:          dispatch.Config{Limit: 20, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: time.Hour},
	}, dispatch.WithDeliveryFailureObserver(func(delivery eventing.Delivery, _ eventing.FailureCode, err error) {
		observed[string(delivery.Envelope.ID)] = err
	}))
	if err != nil {
		t.Fatalf("装配派发器：%v", err)
	}
	// 先让受理链交出的信封走一拍，未决的一小时内不再到期。第二拍断言的只是 SA 那两封各自的去向，同一批里还有没有
	// 别的信封不影响结论。
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("受理链那一拍：%v", err)
	}

	registerDemoSellPricing(t, fixture.db)
	requests, err := sapostgres.NewSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatalf("SA SELL 请求册：%v", err)
	}
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("Outbox Store：%v", err)
	}
	handoff, err := sapostgres.NewOutboxSellEvaluationRequestHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("SA SELL 请求交接：%v", err)
	}
	// 反例换一件包裹：SELL 请求信封按（租户、包裹）分区，同一件包裹的两封在一拍里只走得到排头那一封。它停在选卡，
	// 用不着这件包裹真在委托上。
	carded := sellRequestRecord(t, "SELLREQ-SYN-CARDED", "SYN-PARCEL-01", "SYN-SCOPE-01")
	uncarded := sellRequestRecord(t, "SELLREQ-SYN-NOCARD", "SYN-PARCEL-02", "SYN-SCOPE-NOCARD")
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		for _, record := range []saports.SellEvaluationRequestRecord{carded, uncarded} {
			if _, err := requests.Save(txCtx, record); err != nil {
				return err
			}
			if err := handoff.HandOffSellEvaluationRequest(txCtx, saports.SellEvaluationRequestIntent{Record: record}); err != nil {
				return err
			}
		}
		return nil
	})
	cardedID := string(outboxintent.FingerprintEventID("sell-evaluation-request", demoTenant, "SELLREQ-SYN-CARDED", "submitted"))
	uncardedID := string(outboxintent.FingerprintEventID("sell-evaluation-request", demoTenant, "SELLREQ-SYN-NOCARD", "submitted"))
	if ids := fixture.outboxIDsOfType(t, string(ppinbox.SellEvaluationRequestSubmittedEventType)); len(ids) != 2 {
		t.Fatalf("SA 交出的 SELL 请求信封 = %v, want 两封", ids)
	}

	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("SELL 请求那一拍：%v", err)
	}

	if status := outboxStatus(t, fixture.db, cardedID); status != "PUBLISHED" {
		t.Fatalf("有卡的 SELL 请求信封 status = %q, want PUBLISHED；失败码 = %q 原文 = %v",
			status, recordedFailureCode(t, fixture.db, cardedID), observed[cardedID])
	}
	if n := fixture.countInbox(t, "parcel-pricing/form-sell-evaluation-from-request", cardedID); n != 1 {
		t.Fatalf("SELL 消费者 inbox 行数 = %d, want 1", n)
	}

	evaluations, err := pppostgres.NewEvaluations(fixture.db)
	if err != nil {
		t.Fatalf("PP 评价库：%v", err)
	}
	evaluation, found, err := evaluations.FindByRequestReference(t.Context(),
		ppValue(t, ppdomain.NewTenantID, demoTenant), ppValue(t, ppdomain.NewEvaluationRequestReference, "SELLREQ-SYN-CARDED"))
	if err != nil || !found {
		t.Fatalf("按回指读评价：found=%v err=%v", found, err)
	}
	if evaluation.Direction() != ppdomain.PricingDirectionSell || evaluation.Purpose() != ppdomain.PricingPurposeCustomerCharge {
		t.Fatalf("评价 = %s/%s, want SELL/CUSTOMER_CHARGE", evaluation.Direction(), evaluation.Purpose())
	}
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("评价状态 = %s, want COMPLETED；issues = %v", evaluation.Status(), evaluation.Issues())
	}
	if plan := evaluation.PlanReference(); plan.ID() != "SYN-PLAN-CN-SG-01" {
		t.Fatalf("选到的卡 = %s, want 演示售价卡 SYN-PLAN-CN-SG-01", plan.ID())
	}
	if total, ok := evaluation.Total(); !ok || total.Currency().String() != "CNY" {
		t.Fatalf("合计 = %v present=%v, want CNY", total, ok)
	}
	input := evaluation.Input()
	if settlement, declared := input.SettlementCurrency(); !declared || settlement.String() != "CNY" {
		t.Fatalf("快照结算币种 = %q declared=%v, want 请求上快照的 CNY", settlement, declared)
	}
	if !input.BusinessAt().Equal(sellBasisAt) {
		t.Fatalf("快照业务时点 = %s, want 请求的计价基准时点 %s", input.BusinessAt(), sellBasisAt)
	}
	declarationTagged := false
	for _, fact := range input.FactReferences() {
		if fact.Reference().Kind() == ppdomain.ArtifactDeclaredMeasurement && fact.Reference().Version() == "declaration@baseline" {
			declarationTagged = true
		}
	}
	if !declarationTagged {
		t.Fatalf("快照事实引用 = %+v, want 一条标明来源是基线上的客户申报", input.FactReferences())
	}
	if eventType := outboxEventType(t, fixture.db, evaluation.ID().String()); eventType != "parcel-pricing.evaluation.recorded" {
		t.Fatalf("评价标识那封信 = %q, want parcel-pricing.evaluation.recorded", eventType)
	}

	if got := recordedFailureCode(t, fixture.db, uncardedID); got != "dispatch.consumer_undecided" {
		t.Fatalf("没卡的 SELL 请求 failure_code = %q, want dispatch.consumer_undecided；原文 = %v", got, observed[uncardedID])
	}
	if err := observed[uncardedID]; !errors.Is(err, ppsettlement.ErrPriceCardNotConfigured) {
		t.Fatalf("没卡的 SELL 请求没停在「未配置」：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-pricing/form-sell-evaluation-from-request", uncardedID); n != 0 {
		t.Fatalf("未决必须回滚：inbox 行数 = %d, want 0", n)
	}
	if _, found, err := evaluations.FindByRequestReference(t.Context(),
		ppValue(t, ppdomain.NewTenantID, demoTenant), ppValue(t, ppdomain.NewEvaluationRequestReference, "SELLREQ-SYN-NOCARD")); err != nil || found {
		t.Fatalf("没卡的请求形成了评价：found=%v err=%v", found, err)
	}
	if n := fixture.countOutboxOfType(t, "parcel-pricing.evaluation.recorded"); n != 1 {
		t.Fatalf("评价已记录信封 %d 封, want 1", n)
	}
}

// outboxEventType 读回一封信在 outbox 里的事件类型。
func outboxEventType(t *testing.T, db *bentopg.DB, eventID string) string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var eventType string
	if err := querier.QueryRow(t.Context(),
		`SELECT event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID,
	).Scan(&eventType); err != nil {
		t.Fatalf("读回 %s 的事件类型：%v", eventID, err)
	}
	return eventType
}
