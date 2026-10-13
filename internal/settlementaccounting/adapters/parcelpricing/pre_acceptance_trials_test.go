package parcelpricing_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证结算对计价受理前试算的消费侧适配器（票 product-strategy-boundary/25 第 4 条；ADR-0179 决定一、四、六）：经真入口、真补齐
// 读数算出的评价，按 ISO 4217 最小币单位译成整数合计、带上评价标识与版本清单；计价那一侧的各格原样分开；位数多于币种的合计不截断，
// 当没有合计交回。

var trialJudgedAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

type trialCardDouble struct {
	resolution ppports.PriceCardInForceResolution
	err        error
}

func (double trialCardDouble) ResolveInForce(context.Context, ppdomain.TenantID, ppdomain.PricingScopeID,
	ppdomain.PricingDirection, ppdomain.PricingPurpose, time.Time) (ppports.PriceCardInForceResolution, error) {
	return double.resolution, double.err
}

type trialDeclarationDouble struct {
	declaration ppports.SubmissionDeclaration
	found       bool
}

func (double trialDeclarationDouble) LoadSubmissionDeclaration(context.Context, ppdomain.TenantID, string, string) (ppports.SubmissionDeclaration, bool, error) {
	return double.declaration, double.found, nil
}

type zoneInForceDouble struct{ reference ppdomain.VersionReference }

func (double zoneInForceDouble) ResolveInForce(context.Context, ppdomain.TenantID, ppdomain.CatalogueKind, string, time.Time) (ppdomain.VersionReference, ppports.CatalogueInForceOutcome, error) {
	return double.reference, ppports.CatalogueVersionInForce, nil
}

type zoneRegisterDouble struct {
	registration ppdomain.ReferenceCatalogueRegistration
}

func (double zoneRegisterDouble) Register(context.Context, ppdomain.ReferenceCatalogueRegistration) (ppports.ReferenceCatalogueRegistrationOutcome, error) {
	return ppports.ReferenceCatalogueRegistrationOutcomeInvalid, errors.New("not used")
}

func (double zoneRegisterDouble) ResolveAt(_ context.Context, _ ppdomain.TenantID, _ ppdomain.VersionReference, asOf time.Time, route ppdomain.PostalRoute) (ppdomain.ResolvedCatalogueValue, bool, error) {
	reading, applicable := double.registration.ResolveAt(asOf, route)
	return reading, applicable, nil
}

// zoneBoundCard 是一张 SELL·客户费用卡：Z1 一行、每件 rate 元，绑一份「目的前缀 902 → Z1」的分区目录。
func zoneBoundCard(t *testing.T, rate string) ppdomain.PricingPlanVersion {
	t.Helper()
	period := pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	base := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-PLAN-SELL", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-TABLE-SELL", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT", "v1"),
		Scope:                 "SYN-SCOPE-SELL",
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE",
		Currency:              "CNY",
		Period:                period,
		RateEntryID:           "rate-z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            rate,
		WeightRounding:        ppdomain.RoundingCeiling,
		WeightStepKilograms:   "1",
	})
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	structures, err := base.Structures().WithReferenceCatalogues(link)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func zoneCatalogue(t *testing.T) (ppdomain.VersionReference, ppdomain.ReferenceCatalogueRegistration) {
	t.Helper()
	reference := pptest.IdentityReference(t, ppdomain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE", "v2")
	period, err := ppdomain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	entry, err := ppdomain.NewCatalogueEntry("902", "Z1")
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	registration, err := ppdomain.NewReferenceCatalogueRegistration(ppdomain.ReferenceCatalogueRegistrationSpec{
		Tenant: pptest.Value(t, ppdomain.NewTenantID, "SYN-T1"), Kind: ppdomain.CatalogueKindZone, Reference: reference,
		SourceIdentifier: "SYN-CARRIER/zone-chart", Registrant: "SYN-OPS", Origin: ppdomain.NewIndependentCatalogueOrigin(),
		PrefixLength: 3, Entries: []ppdomain.CatalogueEntry{entry}, Period: period,
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return reference, registration
}

func trialAdapter(t *testing.T, card ppports.PriceCardInForceResolution, declaration ppports.SubmissionDeclaration) *adapter.PreAcceptanceTrials {
	t.Helper()
	reference, registration := zoneCatalogue(t)
	estimates, err := ppapplication.NewFormPlanGivenEstimateEvaluationsHandler(ppapplication.FormPlanGivenEstimateEvaluationsDeps{
		Clock:            trialClock{},
		CatalogueInForce: zoneInForceDouble{reference: reference},
		Catalogues:       zoneRegisterDouble{registration: registration},
	})
	if err != nil {
		t.Fatalf("plan-given handler: %v", err)
	}
	handler, err := ppapplication.NewFormPreAcceptanceEstimatesHandler(ppapplication.FormPreAcceptanceEstimatesDeps{
		PriceCards:   trialCardDouble{resolution: card},
		Declarations: trialDeclarationDouble{declaration: declaration, found: true},
		Estimates:    estimates,
		Evidence:     ppdomain.EvidenceSynthetic,
	})
	if err != nil {
		t.Fatalf("pre-acceptance handler: %v", err)
	}
	trials, err := adapter.NewPreAcceptanceTrials(handler)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	return trials
}

type trialClock struct{}

func (trialClock) Now() time.Time { return trialJudgedAt.Add(time.Minute) }

func twoParcels(t *testing.T) ppports.SubmissionDeclaration {
	t.Helper()
	route, err := ppdomain.NewPostalRoute("200000", "90210")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	one := pptest.Value(t, func(raw string) (ppdomain.Weight, error) {
		return ppdomain.NewWeightFromString(raw, ppdomain.WeightUnitKilogram)
	}, "1")
	return ppports.SubmissionDeclaration{
		Parcels: []ppports.DeclaredParcel{{Parcel: "P-1", Weight: &one}, {Parcel: "P-2", Weight: &one}},
		Route:   &route,
	}
}

func trialQuery(t *testing.T) saports.PreAcceptanceTrialQuery {
	t.Helper()
	return saports.PreAcceptanceTrialQuery{
		Tenant:            mustSA(t, sadomain.NewTenantID, "SYN-T1"),
		PricingScope:      mustSA(t, sadomain.NewEstimatePricingScope, "SYN-SCOPE-SELL"),
		ShipmentRequest:   "SHR-SYN-01",
		SubmissionVersion: "SV-SYN-01",
		At:                trialJudgedAt,
		Currency:          mustSA(t, sadomain.NewCurrencyCode, "CNY"),
	}
}

func mustSA[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// 逐件译、与申报同序：12.50 CNY 按 ISO 4217 两位译成 1250；评价标识与版本清单（含价卡与分区目录）随试算带回，价卡引用随答复带回。
// 本上下文的合计复算出 2500。
func TestPreAcceptanceTrialsCarryMinorUnitTotalsAndTheirVersionManifest(t *testing.T) {
	card := zoneBoundCard(t, "12.50")
	answer, err := trialAdapter(t, ppports.PriceCardInForceResolution{Outcome: ppports.PriceCardVersionInForce, Plan: card}, twoParcels(t)).
		FormPreAcceptanceTrials(context.Background(), trialQuery(t))
	if err != nil {
		t.Fatalf("trials: %v", err)
	}
	if answer.Outcome != saports.PreAcceptanceTrialsFormed || len(answer.Trials) != 2 || !strings.Contains(answer.PriceCard, "SYN-PLAN-SELL@v1") {
		t.Fatalf("answer = %+v, 想要已形成、两份、带价卡引用", answer)
	}
	for index, parcel := range []string{"P-1", "P-2"} {
		trial := answer.Trials[index]
		minor, currency, present := trial.Total()
		if trial.Parcel() != parcel || !present || minor != 1250 || currency.String() != "CNY" || trial.Evaluation() == "" {
			t.Fatalf("第 %d 份 = %+v（%d %s %t）, 想要 %s 1250 CNY 带评价标识", index+1, trial, minor, currency, present, parcel)
		}
		versions := strings.Join(trial.Versions(), " ")
		if !strings.Contains(versions, "SYN-PLAN-SELL@v1") || !strings.Contains(versions, "SYN-CAT-ZONE@v2") {
			t.Fatalf("第 %d 份版本清单 = %s, 想要含价卡与分区目录", index+1, versions)
		}
	}
	adoption, err := sadomain.NewPreAcceptanceEstimateAdoption(mustSA(t, sadomain.NewSettlementAccountID, "ACCT-1"),
		sadomain.PerParcelSellTrialSum, mustSA(t, sadomain.NewEstimatePricingScope, "SYN-SCOPE-SELL"))
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	estimate, gap, err := sadomain.FormPerParcelSellTrialSum(adoption, answer.PriceCard, mustSA(t, sadomain.NewCurrencyCode, "CNY"), answer.Trials)
	if err != nil || gap != sadomain.PreAcceptanceEstimateFormed || estimate.TotalMinor() != 2500 {
		t.Fatalf("estimate = %d gap = %s err = %v, 想要 2500", estimate.TotalMinor(), gap, err)
	}
}

// 合计的小数位多于币种位数（12.505 CNY）不截断：那一份当没有合计交回，估价随之停在没有合计那一格。
func TestATotalFinerThanTheCurrencyIsNotTruncated(t *testing.T) {
	card := zoneBoundCard(t, "12.505")
	answer, err := trialAdapter(t, ppports.PriceCardInForceResolution{Outcome: ppports.PriceCardVersionInForce, Plan: card}, twoParcels(t)).
		FormPreAcceptanceTrials(context.Background(), trialQuery(t))
	if err != nil || answer.Outcome != saports.PreAcceptanceTrialsFormed || len(answer.Trials) != 2 {
		t.Fatalf("answer = %+v err = %v", answer, err)
	}
	if _, _, present := answer.Trials[0].Total(); present {
		t.Fatalf("trial = %+v, 想要当没有合计", answer.Trials[0])
	}
}

// 计价那一侧的格原样分开：选卡未配置、适用冲突、缺件；计价未决是可重试的不可用，不是未配置。
func TestPricingCellsStaySeparateAndUndecidedIsUnavailable(t *testing.T) {
	notConfigured, err := trialAdapter(t, ppports.PriceCardInForceResolution{Outcome: ppports.PriceCardNotConfigured}, twoParcels(t)).
		FormPreAcceptanceTrials(context.Background(), trialQuery(t))
	if err != nil || notConfigured.Outcome != saports.PreAcceptanceTrialPriceCardNotConfigured {
		t.Fatalf("未配置：%+v err = %v", notConfigured, err)
	}
	conflict, err := trialAdapter(t, ppports.PriceCardInForceResolution{Outcome: ppports.PriceCardApplicabilityConflict}, twoParcels(t)).
		FormPreAcceptanceTrials(context.Background(), trialQuery(t))
	if err != nil || conflict.Outcome != saports.PreAcceptanceTrialPriceCardConflict {
		t.Fatalf("冲突：%+v err = %v", conflict, err)
	}
	unmeasured := twoParcels(t)
	unmeasured.Parcels[1].Weight = nil
	incomplete, err := trialAdapter(t, ppports.PriceCardInForceResolution{Outcome: ppports.PriceCardVersionInForce, Plan: zoneBoundCard(t, "12.50")}, unmeasured).
		FormPreAcceptanceTrials(context.Background(), trialQuery(t))
	if err != nil || incomplete.Outcome != saports.PreAcceptanceTrialsIncomplete || len(incomplete.Trials) != 0 {
		t.Fatalf("缺件：%+v err = %v", incomplete, err)
	}

	unreachable, err := ppapplication.NewFormPreAcceptanceEstimatesHandler(ppapplication.FormPreAcceptanceEstimatesDeps{
		PriceCards:   trialCardDouble{err: errors.New("price card register unreachable")},
		Declarations: trialDeclarationDouble{found: true},
		Estimates:    mustPlanGiven(t),
		Evidence:     ppdomain.EvidenceSynthetic,
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	broken, err := adapter.NewPreAcceptanceTrials(unreachable)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if _, err := broken.FormPreAcceptanceTrials(context.Background(), trialQuery(t)); !errors.Is(err, saports.ErrPreAcceptanceTrialsUnavailable) {
		t.Fatalf("未决 err = %v, 想要 ErrPreAcceptanceTrialsUnavailable", err)
	}
	if _, err := adapter.NewPreAcceptanceTrials(nil); err == nil {
		t.Fatal("缺入口却装配成功")
	}
}

func mustPlanGiven(t *testing.T) *ppapplication.FormPlanGivenEstimateEvaluationsHandler {
	t.Helper()
	handler, err := ppapplication.NewFormPlanGivenEstimateEvaluationsHandler(ppapplication.FormPlanGivenEstimateEvaluationsDeps{Clock: trialClock{}})
	if err != nil {
		t.Fatalf("plan-given handler: %v", err)
	}
	return handler
}
