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
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件把结算的施加编排、本适配器与计价的受理前试算入口接成一条，证票 product-strategy-boundary/25 完成判据二：绑燃油序列与
// 分区目录的售价卡、两件委托，冻结金额等于逐件试算合计；任一件缺测量、任一份评价没有合计、卡的币种不是作用域的币种时不冻结，
// 答未形成。计价那一侧是真入口、真补齐读数与纯函数，结算那一侧是真编排与真适配器；替身只在读数源（价卡在用解析、申报、分区
// 目录与燃油序列）与结算自己的几本账上。全在内存里，证据层级 `S`。

const estimateFuelSeries = "SYN-SERIES-FUEL-01"

// fuelAndZoneBoundCard 是 zoneBoundCard 再加一条燃油附加费：按基础运费的百分比计收，百分比取绑定的燃油序列、乘卡上系数 1。
// Z1 一行 0–30 千克每件 12.50；燃油序列当期 20（百分比），所以每件 12.50 + 2.50 = 15.00。
func fuelAndZoneBoundCard(t *testing.T, currency string) ppdomain.PricingPlanVersion {
	t.Helper()
	base := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-PLAN-SELL", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-TABLE-SELL", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT", "v1"),
		Scope:                 "SYN-SCOPE-SELL",
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE",
		Currency:              currency,
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "rate-z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "12.50",
		WeightRounding:        ppdomain.RoundingCeiling,
		WeightStepKilograms:   "1",
	})
	fuel := pptest.Value(t, ppdomain.NewChargeCode, "FUEL_SURCHARGE")
	condition, err := ppdomain.NewWeightFeatureCondition(ppdomain.FeatureActualWeight, ppdomain.ComparisonGreaterThan,
		pptest.Value(t, func(raw string) (ppdomain.Weight, error) {
			return ppdomain.NewWeightFromString(raw, ppdomain.WeightUnitKilogram)
		}, "0"))
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	trigger, err := ppdomain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	calculation, err := ppdomain.NewSeriesRateSurcharge(ppdomain.ReferenceSeriesFuelRate, pptest.Value(t, ppdomain.ParseDecimal, "1"), "dep-fuel")
	if err != nil {
		t.Fatalf("calculation: %v", err)
	}
	rule, err := ppdomain.NewSurchargeRule("fuel", fuel, "fuel surcharge", ppdomain.ChargeEffectAdd, trigger, calculation)
	if err == nil {
		rule, err = rule.Standalone()
	}
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	dependency, err := ppdomain.NewListedChargeDependency("dep-fuel", fuel, []ppdomain.ChargeCode{base.BaseChargeCode()}, nil)
	if err != nil {
		t.Fatalf("dependency: %v", err)
	}
	binding, err := ppdomain.NewReferenceSeriesBinding(ppdomain.ReferenceSeriesFuelRate, estimateFuelSeries)
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := ppdomain.NewPricingPlanStructures(
		[]ppdomain.SurchargeRule{rule}, []ppdomain.ChargeDependency{dependency}, []ppdomain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if structures, err = structures.WithReferenceCatalogues(link); err != nil {
		t.Fatalf("catalogues: %v", err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

type fuelInForceDouble struct{ reference ppdomain.VersionReference }

func (double fuelInForceDouble) ResolveInForce(context.Context, ppdomain.TenantID, ppdomain.ReferenceSeriesKind, string, time.Time) (ppdomain.VersionReference, ppports.InForceOutcome, error) {
	return double.reference, ppports.SeriesVersionInForce, nil
}

type fuelRegisterDouble struct {
	registration ppdomain.ReferenceSeriesRegistration
}

func (double fuelRegisterDouble) Register(context.Context, ppdomain.ReferenceSeriesRegistration) (ppports.ReferenceSeriesRegistrationOutcome, error) {
	return ppports.ReferenceSeriesRegistrationOutcomeInvalid, errors.New("not used")
}

func (double fuelRegisterDouble) ResolveAt(_ context.Context, _ ppdomain.TenantID, _ ppdomain.VersionReference, asOf time.Time) (ppdomain.ResolvedSeriesReading, bool, error) {
	reading, found := double.registration.ResolveAt(asOf)
	return reading, found, nil
}

// fuelSeries 是燃油序列那一对补齐口：在用 v1，自年初起一期、费率 20（百分比）。
func fuelSeries(t *testing.T) (fuelInForceDouble, fuelRegisterDouble) {
	t.Helper()
	reference, err := ppdomain.NewVersionReference(ppdomain.ArtifactReferenceSeries, estimateFuelSeries, "v1", "sha256:syn-fuel-v1")
	if err != nil {
		t.Fatalf("fuel reference: %v", err)
	}
	period, err := ppdomain.NewSeriesPeriodValue(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{},
		pptest.Value(t, ppdomain.ParseDecimal, "20"), "SYN-EVIDENCE/fuel")
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	registration, err := ppdomain.NewReferenceSeriesRegistration(ppdomain.ReferenceSeriesRegistrationSpec{
		Tenant: pptest.Value(t, ppdomain.NewTenantID, "SYN-T1"), Kind: ppdomain.ReferenceSeriesFuelRate, Reference: reference,
		SourceIdentifier: "SYN-CARRIER/fuel", Registrant: "SYN-OPS", Periods: []ppdomain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return fuelInForceDouble{reference: reference}, fuelRegisterDouble{registration: registration}
}

// measuredParcels 是两件 20 × 15 × 10 厘米、按给定千克数申报的包裹，同一条邮编路线（目的前缀 902 → Z1）。卡上有附加费时纯函数要
// 从输入派生包裹特征，所以带尺寸。weights 里的空串即那一件没报测量。
func measuredParcels(t *testing.T, weights ...string) ppports.SubmissionDeclaration {
	t.Helper()
	route, err := ppdomain.NewPostalRoute("200000", "90210")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	declaration := ppports.SubmissionDeclaration{Route: &route}
	for index, raw := range weights {
		parcel := ppports.DeclaredParcel{Parcel: []string{"P-1", "P-2"}[index]}
		if raw != "" {
			weight := pptest.Value(t, func(raw string) (ppdomain.Weight, error) {
				return ppdomain.NewWeightFromString(raw, ppdomain.WeightUnitKilogram)
			}, raw)
			sides, err := ppdomain.NewDimensions(pptest.Value(t, ppdomain.ParseDecimal, "20"), pptest.Value(t, ppdomain.ParseDecimal, "15"),
				pptest.Value(t, ppdomain.ParseDecimal, "10"), ppdomain.LengthUnitCentimeter)
			if err != nil {
				t.Fatalf("dimensions: %v", err)
			}
			parcel.Weight, parcel.Dimensions = &weight, &sides
		}
		declaration.Parcels = append(declaration.Parcels, parcel)
	}
	return declaration
}

// fuelTrials 是生产形状的试算口：本适配器包在计价真入口上，入口的补齐读数装分区目录与燃油序列两对。
func fuelTrials(t *testing.T, card ppdomain.PricingPlanVersion, declaration ppports.SubmissionDeclaration) *adapter.PreAcceptanceTrials {
	t.Helper()
	zone, catalogue := zoneCatalogue(t)
	inForce, series := fuelSeries(t)
	planGiven, err := ppapplication.NewFormPlanGivenEstimateEvaluationsHandler(ppapplication.FormPlanGivenEstimateEvaluationsDeps{
		Clock:            trialClock{},
		SeriesInForce:    inForce,
		SeriesVersions:   series,
		CatalogueInForce: zoneInForceDouble{reference: zone},
		Catalogues:       zoneRegisterDouble{registration: catalogue},
	})
	if err != nil {
		t.Fatalf("plan-given handler: %v", err)
	}
	estimates, err := ppapplication.NewFormPreAcceptanceEstimatesHandler(ppapplication.FormPreAcceptanceEstimatesDeps{
		PriceCards:   trialCardDouble{resolution: ppports.PriceCardInForceResolution{Outcome: ppports.PriceCardVersionInForce, Plan: card}},
		Declarations: trialDeclarationDouble{declaration: declaration, found: true},
		Estimates:    planGiven,
		Evidence:     ppdomain.EvidenceSynthetic,
	})
	if err != nil {
		t.Fatalf("pre-acceptance handler: %v", err)
	}
	trials, err := adapter.NewPreAcceptanceTrials(estimates)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	return trials
}

// 以下是结算自己的几本账的替身：策略要求一项预付冻结，这一户采用了逐件 SELL 试算合计、计价范围就是卡的范围，余额充足。
type requiredFreezePolicy struct {
	policy sadomain.PreAcceptanceControlPolicy
}

func (double requiredFreezePolicy) LoadControlPolicy(context.Context, sadomain.TenantID, sadomain.SettlementScope, sadomain.CommercialResolutionReference) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	return double.policy, true, nil
}

type adoptedSellScope struct {
	adoption sadomain.PreAcceptanceEstimateAdoption
}

func (double adoptedSellScope) FindAdoption(context.Context, sadomain.TenantID, sadomain.SettlementAccountID) (sadomain.PreAcceptanceEstimateAdoption, bool, error) {
	return double.adoption, true, nil
}

type keptTraces struct {
	traces []saports.PreAcceptanceEstimateTrace
}

func (double *keptTraces) RecordEstimate(_ context.Context, trace saports.PreAcceptanceEstimateTrace) (saports.PreAcceptanceEstimateTraceEffect, error) {
	double.traces = append(double.traces, trace)
	return saports.PreAcceptanceEstimateTraceRecorded, nil
}

type ampleBalance struct{ balance sadomain.OperationalBalance }

func (double ampleBalance) LoadBalance(context.Context, sadomain.TenantID, sadomain.SettlementScope) (sadomain.OperationalBalance, error) {
	return double.balance, nil
}

type inMemoryFreezes struct {
	ledger *sadomain.FreezeLedger
	saved  int
}

func (double *inMemoryFreezes) LoadForScope(context.Context, sadomain.TenantID, sadomain.SettlementScope) (*sadomain.FreezeLedger, error) {
	return double.ledger, nil
}

func (double *inMemoryFreezes) Save(context.Context, sadomain.TenantID, sadomain.SettlementScope, *sadomain.FreezeLedger) error {
	double.saved++
	return nil
}

type inMemoryExposures struct {
	ledger *sadomain.CreditExposureLedger
}

func (double inMemoryExposures) LoadForScope(context.Context, sadomain.TenantID, sadomain.SettlementScope) (*sadomain.CreditExposureLedger, error) {
	return double.ledger, nil
}

func (double inMemoryExposures) Save(context.Context, sadomain.TenantID, sadomain.SettlementScope, *sadomain.CreditExposureLedger) error {
	return nil
}

// unusedCredit 替信用那三口：本夹具的策略只要求预付冻结，它们不会被问到。
type unusedCredit struct{}

func (unusedCredit) LoadCreditStanding(context.Context, sadomain.TenantID, sadomain.SettlementScope) (sadomain.CreditStanding, error) {
	return sadomain.CreditStanding{}, nil
}

func (unusedCredit) LoadCreditBasis(context.Context, sadomain.TenantID, sadomain.SettlementScope, sadomain.CommercialResolutionReference) (sadomain.CreditBasis, bool, error) {
	return sadomain.CreditBasis{}, false, nil
}

func (unusedCredit) LoadCreditRatioBase(context.Context, sadomain.TenantID, sadomain.SettlementScope, sadomain.CreditRatioBase) (int64, bool, error) {
	return 0, false, nil
}

type estimateControl struct {
	handler *saapplication.ApplyPreAcceptanceControlHandler
	freezes *inMemoryFreezes
	traces  *keptTraces
}

func newEstimateControl(t *testing.T, trials saports.PreAcceptanceTrialSource) estimateControl {
	t.Helper()
	scope := estimateScope(t)
	item, err := sadomain.NewControlItem(sadomain.PrepaidFreezeControl, 1)
	if err != nil {
		t.Fatalf("control item: %v", err)
	}
	policy, err := sadomain.NewRequiredControlPolicy([]sadomain.ControlItem{item}, sadomain.AllControlsPass,
		mustSA(t, sadomain.NewControlPolicyReference, "SYN-CONTROL-POLICY/v1"), sadomain.PrepaidSettlement,
		mustSA(t, sadomain.NewAdoptedPolicyReference, "SYN-SETTLEMENT-POLICY/v1"))
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	adoption, err := sadomain.NewPreAcceptanceEstimateAdoption(scope.Account(), sadomain.PerParcelSellTrialSum,
		mustSA(t, sadomain.NewEstimatePricingScope, "SYN-SCOPE-SELL"))
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	balance, err := sadomain.NewOperationalBalance(scope, 100_000, 0, 0, 0)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	control := estimateControl{freezes: &inMemoryFreezes{ledger: sadomain.NewFreezeLedger()}, traces: &keptTraces{}}
	control.handler, err = saapplication.NewApplyPreAcceptanceControlHandler(saapplication.ApplyPreAcceptanceControlDeps{
		Policy:      requiredFreezePolicy{policy: policy},
		Adoptions:   adoptedSellScope{adoption: adoption},
		Trials:      trials,
		Traces:      control.traces,
		Balance:     ampleBalance{balance: balance},
		Freezes:     control.freezes,
		Credit:      unusedCredit{},
		CreditBasis: unusedCredit{},
		RatioBases:  unusedCredit{},
		Exposures:   inMemoryExposures{ledger: sadomain.NewCreditExposureLedger()},
		Clock:       trialClock{},
	})
	if err != nil {
		t.Fatalf("apply handler: %v", err)
	}
	return control
}

func estimateScope(t *testing.T) sadomain.SettlementScope {
	t.Helper()
	scope, err := sadomain.NewSettlementScope(mustSA(t, sadomain.NewLegalEntityReference, "SYN-LE-1"),
		mustSA(t, sadomain.NewSettlementAccountID, "ACCT-1"), mustSA(t, sadomain.NewCurrencyCode, "CNY"))
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	return scope
}

func estimateCommand(t *testing.T) saapplication.ApplyPreAcceptanceControlCommand {
	t.Helper()
	asOf, err := sadomain.NewControlAsOf(mustSA(t, sadomain.NewAsOfSemantic, "SUBMISSION_RECEIPT"), trialJudgedAt,
		mustSA(t, sadomain.NewAsOfStrategyVersion, "SYN-ASOF/v1"))
	if err != nil {
		t.Fatalf("as-of: %v", err)
	}
	return saapplication.ApplyPreAcceptanceControlCommand{
		TenantID:          mustSA(t, sadomain.NewTenantID, "SYN-T1"),
		RequestID:         mustSA(t, sadomain.NewControlRequestID, "SHR-SYN-01/SV-SYN-01"),
		Scope:             estimateScope(t),
		ShipmentRequest:   "SHR-SYN-01",
		SubmissionVersion: "SV-SYN-01",
		Association:       mustSA(t, sadomain.NewBusinessAssociationReference, "SHR-SYN-01/SV-SYN-01"),
		AsOf:              asOf,
		Resolution:        mustSA(t, sadomain.NewCommercialResolutionReference, "SYN-RES-1"),
	}
}

// Covers: 票 product-strategy-boundary/25 完成判据二（ADR-0179 决定二、三、四）——绑燃油序列与分区目录的售价卡、两件委托：每件
// 12.50 基础运费加 20% 燃油、经共用补齐读数取到当期费率，冻结金额等于两件试算之和 3000；留痕的版本清单里有价卡、分区目录与
// 燃油序列。
func TestAFuelBoundCardOnTwoParcelsFreezesTheSumOfTheirTrials(t *testing.T) {
	control := newEstimateControl(t, fuelTrials(t, fuelAndZoneBoundCard(t, "CNY"), measuredParcels(t, "1", "1")))

	result, err := control.handler.Handle(context.Background(), estimateCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != saapplication.ControlApplied {
		t.Fatalf("outcome/reason = %q/%q, want CONTROL_APPLIED", result.Outcome(), result.NotFormedReason())
	}
	freeze, held := result.Freeze()
	if !held || freeze.Status() != sadomain.FreezeHeld || freeze.AmountMinor() != 3_000 {
		t.Fatalf("freeze = %#v, want a HELD 3000——两件各 15.00 CNY", freeze)
	}
	if len(control.traces.traces) != 1 {
		t.Fatalf("留痕 %d 份, want 1", len(control.traces.traces))
	}
	trials := control.traces.traces[0].Estimate.Trials()
	if len(trials) != 2 {
		t.Fatalf("留痕逐件 %d 份, want 2", len(trials))
	}
	for index, trial := range trials {
		minor, _, present := trial.Total()
		manifest := map[string]bool{}
		for _, version := range trial.Versions() {
			for _, id := range []string{"SYN-PLAN-SELL@v1", "SYN-CAT-ZONE@v2", estimateFuelSeries + "@v1"} {
				if strings.Contains(version, id) {
					manifest[id] = true
				}
			}
		}
		if !present || minor != 1_500 || len(manifest) != 3 {
			t.Fatalf("第 %d 件 = %d（present=%t），版本清单 %v；want 1500，清单含价卡、分区目录与燃油序列", index+1, minor, present, trial.Versions())
		}
	}
}

// Covers: 票 product-strategy-boundary/25 完成判据二的停法（ADR-0179 决定二）——同一张燃油卡下：一件没报测量、一件超出费率表
// （那一份评价没有合计）、卡的币种不是作用域的币种，都不冻结，答未形成，账本一次都不写。币种那一格停在没有合计而不是币种不对：
// 结算币种随试算交给计价，卡的币种与它不同而卡上没有汇率序列时，计价那一侧就不出合计（CONTEXT-MAP「计价消费汇率」那一条），
// 结算的币种停法是在计价交回了别的币种时才轮得到的那一道。
func TestAFuelBoundCardStopsBeforeFreezingWhenAnyParcelCannotBeEstimated(t *testing.T) {
	cases := map[string]struct {
		card        ppdomain.PricingPlanVersion
		declaration ppports.SubmissionDeclaration
		want        saapplication.NotFormedReason
	}{
		"一件没报测量":     {card: fuelAndZoneBoundCard(t, "CNY"), declaration: measuredParcels(t, "1", ""), want: saapplication.EstimateDeclarationIncomplete},
		"一件超出费率表":    {card: fuelAndZoneBoundCard(t, "CNY"), declaration: measuredParcels(t, "1", "40"), want: saapplication.EstimateTrialWithoutTotal},
		"卡的币种不是作用域的": {card: fuelAndZoneBoundCard(t, "USD"), declaration: measuredParcels(t, "1", "1"), want: saapplication.EstimateTrialWithoutTotal},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			control := newEstimateControl(t, fuelTrials(t, tc.card, tc.declaration))
			result, err := control.handler.Handle(context.Background(), estimateCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != saapplication.ControlNotFormed || result.NotFormedReason() != tc.want {
				t.Fatalf("outcome/reason = %q/%q, want CONTROL_NOT_FORMED/%q", result.Outcome(), result.NotFormedReason(), tc.want)
			}
			if _, held := result.Freeze(); held || control.freezes.saved != 0 || len(control.traces.traces) != 0 {
				t.Fatalf("没形成估价却冻结了或写了账（saved %d、留痕 %d）", control.freezes.saved, len(control.traces.traces))
			}
		})
	}
}
