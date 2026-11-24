package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 证票 07：多件委托的当前有效各件都完成有效网络收寄后，形成一份票级评价和一笔票级客户费用。
// 少一件不形成。证据只记 S。

func TestShipmentMembersIntakeFormsOneSellEvaluationAndOneCustomerCharge(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	submitTwoPieceShipment(t, ctx, fixture)
	recordReachabilityFor(t, ctx, fixture, "SYN-PARCEL-01", "SYN-NRJ-01")
	recordReachabilityFor(t, ctx, fixture, "SYN-PARCEL-02", "SYN-NRJ-02")
	recordHeldControl(t, ctx, fixture)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	seedSettledClosure(t, fixture)
	registerShipmentSellAccount(t, fixture)
	registerDemoZoneCatalogue(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, shipmentSellCard(t))
	registerShipmentFeeMapping(t, fixture)

	secondAt := sellBasisAt.Add(time.Hour)
	recordShipmentIntake(t, fixture, "SYN-PARCEL-02", "SYN-INTAKE-V2", "SYN-CMT-02", secondAt)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("只到第二件：%v", err)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request`); n != 0 {
		t.Fatalf("少一件却落了 %d 份委托级请求", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.customer_charge`); n != 0 {
		t.Fatalf("少一件却落了 %d 笔客户费用", n)
	}

	recordShipmentIntake(t, fixture, "SYN-PARCEL-01", sellChainIntakeVersion, "SYN-CMT-01", sellBasisAt)
	for step := 0; step < 8; step++ {
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("第 %d 拍：%v", step, err)
		}
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request`); n != 1 {
		t.Fatalf("委托级请求 %d 份, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("同一委托又有 %d 份包裹级销售请求", n)
	}
	requests, err := sapostgres.NewShipmentSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatalf("请求册：%v", err)
	}
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	record, found, err := requests.FindByNaturalKey(ctx, tenant, sadomain.ShipmentSellEvaluationRequestNaturalKey{
		Shipment: mustPS(t, sadomain.NewShipmentReference, fixture.requestID.String()),
		Moment:   sadomain.ShipmentMembersIntakeAdopted,
	})
	if err != nil || !found {
		t.Fatalf("读委托级请求 found=%v err=%v", found, err)
	}
	if !record.Request.PricingBasisAt().Equal(sellBasisAt) || record.State != sadomain.SellRequestFormed {
		t.Fatalf("请求状态 %s 基准 %s，想要已形成、基准取第一件 %s", record.State, record.Request.PricingBasisAt(), sellBasisAt)
	}
	evaluations, err := pppostgres.NewEvaluations(fixture.db)
	if err != nil {
		t.Fatalf("评价库：%v", err)
	}
	evaluation, found, err := evaluations.FindByRequestReference(ctx,
		ppValue(t, ppdomain.NewTenantID, fixture.identity.TenantID().String()),
		ppValue(t, ppdomain.NewEvaluationRequestReference, record.Key.Request.String()))
	if err != nil || !found {
		t.Fatalf("评价 found=%v err=%v", found, err)
	}
	subject, hasSubject := evaluation.Input().Subject()
	if !hasSubject || subject.Kind() != ppdomain.SubjectShipment || evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("评价主体 %s 状态 %s，想要 SHIPMENT / COMPLETED；issues=%v",
			subject.Kind(), evaluation.Status(), evaluation.Issues())
	}
	charges, err := sapostgres.NewCustomerCharges(fixture.db)
	if err != nil {
		t.Fatalf("费用册：%v", err)
	}
	charge, found, err := charges.FindByID(ctx, tenant, record.FormedCharge)
	if err != nil || !found {
		t.Fatalf("客户费用 found=%v err=%v id=%s", found, err, record.FormedCharge)
	}
	if charge.Stage() != sadomain.ChargeEstimated || charge.ID() != record.Request.EstimatedCustomerChargeID() {
		t.Fatalf("费用 %s 阶段 %s", charge.ID(), charge.Stage())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.customer_charge`); n != 1 {
		t.Fatalf("客户费用 %d 笔, want 1", n)
	}

	for step := 0; step < 2; step++ {
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("形成后再派：%v", err)
		}
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.customer_charge`); n != 1 {
		t.Fatalf("再派之后客户费用 %d 笔，不该重算", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request`); n != 1 {
		t.Fatalf("再派之后委托级请求 %d 份，不该另起一代", n)
	}
}

func submitTwoPieceShipment(t *testing.T, ctx context.Context, fixture *synVerticalFixture) {
	t.Helper()
	command := fixture.submitCommand(t)
	command.DeclaredParcelIDs = []psdomain.DeclaredParcelID{
		mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-02"),
	}
	command.DeclaredProfiles = []psdomain.DeclaredParcelProfile{
		gapSplitProfile(t, "SYN-PARCEL-01", "2"),
		gapSplitProfile(t, "SYN-PARCEL-02", "3"),
	}
	command.DeclaredElements = gapSplitPostalElements(t, "200001", "018956")
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		result, err := fixture.submitHandler.Handle(txCtx, command)
		if err != nil {
			return err
		}
		outcome = result.Outcome()
		return nil
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("submit = %s, want SUBMITTED", outcome)
	}
}

func recordReachabilityFor(t *testing.T, ctx context.Context, fixture *synVerticalFixture, parcelID, judgmentID string) {
	t.Helper()
	parcel := mustPS(t, psdomain.NewDeclaredParcelID, parcelID)
	asOf := synJudgmentAsOf(t, psdomain.ReachabilityJudgmentKind, time.Now().UTC().Add(-time.Minute))
	reachable, err := psdomain.NewReachabilityJudgment(psdomain.ReachabilityJudgmentSpec{
		JudgmentID: mustPS(t, psdomain.NewReachabilityJudgmentID, judgmentID),
		ParcelID:   parcel,
		Value:      psdomain.ReachabilityReachable,
		AsOf:       asOf,
	})
	if err != nil {
		t.Fatalf("可达性：%v", err)
	}
	reachable = reachable.FormedUnder(mustPS(t, psdomain.NewCommercialResolutionID, "SYN-RES-01"))
	version := fixture.mustLoadRequest(t, ctx).CurrentSubmissionVersion().VersionID()
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.judgments.RecordReachabilityJudgment(txCtx, fixture.identity.TenantID(), fixture.requestID, version, reachable)
	})
}

func recordHeldControl(t *testing.T, ctx context.Context, fixture *synVerticalFixture) {
	t.Helper()
	controlAsOf := synJudgmentAsOf(t, psdomain.FinancialControlJudgmentKind, time.Now().UTC().Add(-time.Minute))
	held := synHeldControl(t, "SYN-SAC-01", controlAsOf)
	version := fixture.mustLoadRequest(t, ctx).CurrentSubmissionVersion().VersionID()
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.judgments.RecordFinancialControlResult(txCtx, fixture.identity.TenantID(), fixture.requestID, version, held)
	})
}

func registerShipmentSellAccount(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatalf("账户册：%v", err)
	}
	adoptions, err := sapostgres.NewShipmentBillingMomentAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("委托级采用册：%v", err)
	}
	account := synRReceivableAccount(t)
	adoption, err := sadomain.NewShipmentBillingMomentAdoption(account.ID(), sadomain.ShipmentMembersIntakeAdopted,
		mustPS(t, sadomain.NewEstimatePricingScope, sellChainPricingScope))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	at := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		if _, err := accounts.Save(txCtx, tenant, account, at); err != nil {
			return err
		}
		_, err := adoptions.SaveShipmentBillingMomentAdoption(txCtx, tenant, adoption, at)
		return err
	})
}

func registerShipmentFeeMapping(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	mappings, err := sapostgres.NewFeeItemMappings(fixture.db)
	if err != nil {
		t.Fatalf("映射册：%v", err)
	}
	mapping, err := sadomain.NewFeeItemMapping(
		mustPS(t, sadomain.NewPricingChargeCode, "BASE_FREIGHT"),
		mustPS(t, sadomain.NewFeeItemReference, "SYN-FEE-ITEM-FREIGHT"),
		mustPS(t, sadomain.NewFeeItemMappingBasisReference, "SYN-CLAUSE-FREIGHT-01"),
	)
	if err != nil {
		t.Fatalf("映射：%v", err)
	}
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := mappings.SaveFeeItemMapping(txCtx, tenant, mapping, time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC))
		return err
	})
}

func shipmentSellCard(t *testing.T) ppdomain.PriceCardRegistration {
	t.Helper()
	base := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-PLAN-SHIP-SELL", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-PLAN-SHIP-SELL-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-PLAN-SHIP-SELL-WEIGHT", "v1"),
		Scope:                 sellChainPricingScope,
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              synRCurrency,
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-PLAN-SHIP-SELL-Z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "73",
		WeightRounding:        ppdomain.RoundingNone,
		WeightStepKilograms:   "1",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode: ppdomain.RoundingHalfUp, Increment: "0.01", Points: []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	if err != nil {
		t.Fatalf("目录绑定：%v", err)
	}
	structures, err := base.Structures().WithReferenceCatalogues(link)
	if err != nil {
		t.Fatalf("结构：%v", err)
	}
	plan, err := ppdomain.NewAggregatePricingPlanVersion(ppdomain.AggregationPerShipment,
		base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("票级卡：%v", err)
	}
	return pricingResumeRegistration(t, "SYN-PLAN-SHIP-SELL", plan)
}

func recordShipmentIntake(t *testing.T, fixture *synVerticalFixture, parcelID, version, commitmentID string, occurredAt time.Time) {
	t.Helper()
	ctx := t.Context()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("Outbox：%v", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("收寄采用：%v", err)
	}
	handoff, err := pspostgres.NewOutboxNetworkIntakeHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("交接：%v", err)
	}
	key := psports.IntakeAdoptionKey{
		TenantID: fixture.identity.TenantID(),
		Parcel:   mustPS(t, psdomain.NewDeclaredParcelID, parcelID),
		Kind:     psdomain.NodeIntakeSource,
		Version:  mustPS(t, psdomain.NewSourceResultVersion, version),
	}
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind: key.Kind, Object: mustPS(t, psdomain.NewSourceObjectReference, "SYN-UNIT-"+parcelID),
		Parcel: key.Parcel, Place: mustPS(t, psdomain.NewIntakePlaceReference, "SYN-NODE-01"),
		Control: mustPS(t, psdomain.NewIntakeControlReference, "SYN-CONTROL-"+parcelID),
		Version: key.Version, OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	baseline := fixture.mustLoadRequest(t, ctx).CurrentSubmissionVersion().VersionID()
	intake, err := psdomain.AdoptNetworkIntake(source, baseline)
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	commitment, err := psdomain.FormFormalCommitment(
		mustPS(t, psdomain.NewCommitmentVersionID, commitmentID), intake,
		mustPS(t, psdomain.NewExpectedCommitmentReference, "SYN-RES-01@2026-08-07T00:00:00Z"),
	)
	if err != nil {
		t.Fatalf("承诺：%v", err)
	}
	record := psports.IntakeAdoptionRecord{
		Key: key, CustomerAccountID: fixture.identity.CustomerAccountID(), ShipmentRequestID: fixture.requestID,
		ContentDigest: "SYN-ADOPTION-" + parcelID, Adopted: true, Intake: intake, Commitment: commitment,
		AdoptedAt: occurredAt.Add(time.Minute),
	}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		if _, err := adoptions.Save(txCtx, record); err != nil {
			return err
		}
		return handoff.HandOffNetworkIntake(txCtx, psports.NetworkIntakeHandoffIntent{Record: record})
	})
}

func TestShipmentSellBasisStaysTheEarliestResponsibilityStartAcrossACardChange(t *testing.T) {
	t0 := sellBasisAt
	cut := t0.Add(time.Hour)
	later := t0.Add(2 * time.Hour)
	fixture := acceptTwoPieceSellShipment(t, "200001", "018956",
		gapSplitProfile(t, "SYN-PARCEL-01", "2"), gapSplitProfile(t, "SYN-PARCEL-02", "3"))
	registerShipmentSellAccount(t, fixture)
	registerShipmentFeeMapping(t, fixture)
	registerDemoZoneCatalogue(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-SHIP-EARLY", scope: sellChainPricingScope, amount: "73",
		starts: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ends: cut, aggregate: true, zones: []string{"Z1"},
	}))
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-SHIP-LATE", scope: sellChainPricingScope, amount: "91",
		starts: cut, aggregate: true, zones: []string{"Z1"},
	}))

	recordShipmentIntake(t, fixture, "SYN-PARCEL-02", "SYN-INTAKE-V2", "SYN-CMT-02", t0)
	recordShipmentIntake(t, fixture, "SYN-PARCEL-01", sellChainIntakeVersion, "SYN-CMT-01", later)
	dispatchBeats(t, fixture.beat, 8)

	record := formedShipmentRequest(t, fixture)
	if !record.Request.PricingBasisAt().Equal(t0) {
		t.Fatalf("基准 %s，第二件 t0、第一件 t0+2h 时应取最早的 t0，不是位次第一件", record.Request.PricingBasisAt())
	}
	evaluation := shipmentEvaluation(t, fixture, record)
	if evaluation.Status() != ppdomain.EvaluationCompleted || evaluation.PlanReference().ID() != "SYN-PLAN-SHIP-EARLY" {
		t.Fatalf("评价 %s 卡 %s 问题 %v，应选用 t0 那一版", evaluation.Status(), evaluation.PlanReference().ID(), evaluation.Issues())
	}
}

func TestShipmentSellBasisDoesNotSplitSelectionFromEvaluationAcrossACardChange(t *testing.T) {
	t0 := sellBasisAt
	cut := t0.Add(time.Hour)
	later := t0.Add(2 * time.Hour)
	fixture := acceptTwoPieceSellShipment(t, "200001", "018956",
		gapSplitProfile(t, "SYN-PARCEL-01", "2"), gapSplitProfile(t, "SYN-PARCEL-02", "3"))
	registerShipmentSellAccount(t, fixture)
	registerShipmentFeeMapping(t, fixture)
	registerDemoZoneCatalogue(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-SHIP-AT-T0", scope: sellChainPricingScope, amount: "73",
		starts: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ends: cut, aggregate: true, zones: []string{"Z1"},
	}))
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-PKG-AFTER", scope: sellChainPricingScope, amount: "91",
		starts: cut, aggregate: false, zones: []string{"Z1"},
	}))

	recordShipmentIntake(t, fixture, "SYN-PARCEL-01", sellChainIntakeVersion, "SYN-CMT-01", t0)
	recordShipmentIntake(t, fixture, "SYN-PARCEL-02", "SYN-INTAKE-V2", "SYN-CMT-02", later)
	dispatchBeats(t, fixture.beat, 8)

	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("选卡与基准都在 t0 的逐委托卡上，却落了 %d 份包裹级请求", n)
	}
	record := formedShipmentRequest(t, fixture)
	if !record.Request.PricingBasisAt().Equal(t0) {
		t.Fatalf("基准 %s，want %s", record.Request.PricingBasisAt(), t0)
	}
	evaluation := shipmentEvaluation(t, fixture, record)
	if evaluation.Status() != ppdomain.EvaluationCompleted || evaluation.PlanReference().ID() != "SYN-PLAN-SHIP-AT-T0" {
		t.Fatalf("评价 %s 卡 %s 问题 %v", evaluation.Status(), evaluation.PlanReference().ID(), evaluation.Issues())
	}
	for _, issue := range evaluation.Issues() {
		if issue.Code() == "AGGREGATION_SUBJECT_MISMATCH" {
			t.Fatal("选卡时刻与计价基准错位，答了聚合方式不符")
		}
	}
}

func TestShipmentSellUndeclaredMaxWeightPricesFromPieceDimensions(t *testing.T) {
	fixture := acceptTwoPieceSellShipment(t, "200001", "018956",
		pricedProfile(t, "SYN-PARCEL-01", "2", "30", "20", "10"),
		pricedProfile(t, "SYN-PARCEL-02", "1", "50", "40", "30"),
	)
	registerShipmentSellAccount(t, fixture)
	registerShipmentFeeMapping(t, fixture)
	registerDemoZoneCatalogue(t, fixture.db)
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-SHIP-VOL", scope: sellChainPricingScope, amount: "40",
		starts: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), aggregate: true, zones: []string{"Z1"}, volumetric: true,
	}))

	trials, err := preAcceptanceTrials(fixture.db, systemClock{})
	if err != nil {
		t.Fatalf("试算：%v", err)
	}
	request := fixture.mustLoadRequest(t, t.Context())
	answer, err := trials.FormPreAcceptanceTrials(t.Context(), saports.PreAcceptanceTrialQuery{
		Tenant:            mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String()),
		PricingScope:      mustPS(t, sadomain.NewEstimatePricingScope, sellChainPricingScope),
		ShipmentRequest:   fixture.requestID.String(),
		SubmissionVersion: request.CurrentSubmissionVersion().VersionID().String(),
		At:                sellBasisAt,
		Currency:          mustPS(t, sadomain.NewCurrencyCode, synRCurrency),
	})
	if err != nil {
		t.Fatalf("受理前估价：%v", err)
	}
	if answer.Outcome != saports.PreAcceptanceTrialsFormed || len(answer.Trials) != 1 {
		t.Fatalf("试算 %s 份数 %d", answer.Outcome, len(answer.Trials))
	}
	weight, weighed := answer.Trials[0].PricingWeight()
	if !weighed || weight.Value() != "13.2" || weight.Unit() != "KG" {
		t.Fatalf("试算计价重量 %s %s present=%v，want 13.2 KG", weight.Value(), weight.Unit(), weighed)
	}

	recordShipmentIntake(t, fixture, "SYN-PARCEL-01", sellChainIntakeVersion, "SYN-CMT-01", sellBasisAt)
	recordShipmentIntake(t, fixture, "SYN-PARCEL-02", "SYN-INTAKE-V2", "SYN-CMT-02", sellBasisAt.Add(time.Hour))
	dispatchBeats(t, fixture.beat, 8)
	evaluation := shipmentEvaluation(t, fixture, formedShipmentRequest(t, fixture))
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("正式评价 %s %v", evaluation.Status(), evaluation.Issues())
	}
	priced, ok := evaluation.PricingWeight()
	volumetric, derived := priced.VolumetricWeight()
	if !ok || !derived || volumetric.Value().String() != "13.2" {
		t.Fatalf("体积重 present=%v derived=%v value=%s", ok, derived, volumetric.Value())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.customer_charge`); n != 1 {
		t.Fatalf("客户费用 %d 笔, want 1", n)
	}
}

func TestShipmentSellPackageCardMissFallsThroughToTheShipmentCard(t *testing.T) {
	const exclusive = "SYN-SCOPE-CFMR-EXCL"
	const public = "SYN-SCOPE-CFMR-PUBLIC"
	fixture := acceptTwoPieceSellShipment(t, "200001", "238956",
		gapSplitProfile(t, "SYN-PARCEL-01", "1"), gapSplitProfile(t, "SYN-PARCEL-02", "1"))
	registerShipmentSellAccount(t, fixture)
	registerShipmentFeeMapping(t, fixture)
	registerDemoZoneCatalogue(t, fixture.db)
	saveSellPriceSources(t, fixture, exclusive, public)
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-EXCL-PKG", scope: exclusive, amount: "11",
		starts: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), aggregate: false, zones: []string{"Z1"},
	}))
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-PUBLIC-SHIP", scope: public, amount: "22",
		starts: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), aggregate: true, zones: []string{"Z1", "Z2"},
	}))

	recordShipmentIntake(t, fixture, "SYN-PARCEL-01", sellChainIntakeVersion, "SYN-CMT-01", sellBasisAt)
	dispatchBeats(t, fixture.beat, 4)
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("第一件应收寄等齐，却落了 %d 份包裹请求", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request`); n != 0 {
		t.Fatalf("少一件却落了 %d 份委托级请求", n)
	}

	recordShipmentIntake(t, fixture, "SYN-PARCEL-02", "SYN-INTAKE-V2", "SYN-CMT-02", sellBasisAt.Add(time.Hour))
	dispatchBeats(t, fixture.beat, 8)
	record := formedShipmentRequest(t, fixture)
	evaluation := shipmentEvaluation(t, fixture, record)
	if evaluation.Status() != ppdomain.EvaluationCompleted || evaluation.PlanReference().ID() != "SYN-PLAN-PUBLIC-SHIP" {
		t.Fatalf("评价 %s 卡 %s 问题 %v", evaluation.Status(), evaluation.PlanReference().ID(), evaluation.Issues())
	}
	if !explanationHasSellSource(evaluation, exclusive, "BASE_RATE_NOT_IN_TABLE") || !explanationHasSellSource(evaluation, public, "SELECTED") {
		t.Fatalf("解释 = %v", evaluation.Explanation())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.customer_charge`); n != 1 {
		t.Fatalf("客户费用 %d 笔, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("票级费用之外还有 %d 份包裹请求", n)
	}
}

func TestShipmentSellShipmentCardMissFallsThroughToPerPackageBilling(t *testing.T) {
	const exclusive = "SYN-SCOPE-CFMR-EXCL-SHIP"
	const public = "SYN-SCOPE-CFMR-PUBLIC-PKG"
	fixture := acceptTwoPieceSellShipment(t, "200001", "238956",
		gapSplitProfile(t, "SYN-PARCEL-01", "1"), gapSplitProfile(t, "SYN-PARCEL-02", "1"))
	registerParcelSellAccount(t, fixture)
	registerShipmentFeeMapping(t, fixture)
	registerDemoZoneCatalogue(t, fixture.db)
	saveSellPriceSources(t, fixture, exclusive, public)
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-EXCL-SHIP", scope: exclusive, amount: "11",
		starts: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), aggregate: true, zones: []string{"Z1"},
	}))
	registerPriceCardForTest(t, fixture.db, catalogueSellCard(t, sellCardSpec{
		planID: "SYN-PLAN-PUBLIC-PKG", scope: public, amount: "22",
		starts: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), aggregate: false, zones: []string{"Z1", "Z2"},
	}))

	recordShipmentIntake(t, fixture, "SYN-PARCEL-01", sellChainIntakeVersion, "SYN-CMT-01", sellBasisAt)
	dispatchBeats(t, fixture.beat, 8)
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request`); n != 0 {
		t.Fatalf("逐包裹卡上不该有委托级请求，有 %d 份", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 1 {
		t.Fatalf("第一件包裹请求 %d 份, want 1", n)
	}
	first := parcelSellEvaluation(t, fixture, "SYN-PARCEL-01")
	if first.Status() != ppdomain.EvaluationCompleted || first.PlanReference().ID() != "SYN-PLAN-PUBLIC-PKG" {
		t.Fatalf("第一件评价 %s 卡 %s 问题 %v", first.Status(), first.PlanReference().ID(), first.Issues())
	}
	if !explanationHasSellSource(first, exclusive, "BASE_RATE_NOT_IN_TABLE") || !explanationHasSellSource(first, public, "SELECTED") {
		t.Fatalf("第一件解释 = %v", first.Explanation())
	}

	recordShipmentIntake(t, fixture, "SYN-PARCEL-02", "SYN-INTAKE-V2", "SYN-CMT-02", sellBasisAt.Add(time.Hour))
	dispatchBeats(t, fixture.beat, 8)
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.shipment_sell_evaluation_request`); n != 0 {
		t.Fatalf("两件之后仍不该有委托级请求，有 %d 份", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 2 {
		t.Fatalf("包裹请求 %d 份, want 2", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.customer_charge`); n != 2 {
		t.Fatalf("客户费用 %d 笔, want 2", n)
	}
	second := parcelSellEvaluation(t, fixture, "SYN-PARCEL-02")
	if second.Status() != ppdomain.EvaluationCompleted || !explanationHasSellSource(second, exclusive, "BASE_RATE_NOT_IN_TABLE") ||
		!explanationHasSellSource(second, public, "SELECTED") {
		t.Fatalf("第二件评价 %s 解释 %v 问题 %v", second.Status(), second.Explanation(), second.Issues())
	}
}

func acceptTwoPieceSellShipment(t *testing.T, sender, delivery string, profiles ...psdomain.DeclaredParcelProfile) *synVerticalFixture {
	t.Helper()
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	command := fixture.submitCommand(t)
	command.DeclaredParcelIDs = []psdomain.DeclaredParcelID{
		mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-02"),
	}
	command.DeclaredProfiles = profiles
	command.DeclaredElements = gapSplitPostalElements(t, sender, delivery)
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		result, err := fixture.submitHandler.Handle(txCtx, command)
		if err != nil {
			return err
		}
		outcome = result.Outcome()
		return nil
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("submit = %s", outcome)
	}
	recordReachabilityFor(t, ctx, fixture, "SYN-PARCEL-01", "SYN-NRJ-01")
	recordReachabilityFor(t, ctx, fixture, "SYN-PARCEL-02", "SYN-NRJ-02")
	recordHeldControl(t, ctx, fixture)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q pending = %q", result.State(), result.PendingReason())
	}
	seedSettledClosure(t, fixture)
	return fixture
}

func formedShipmentRequest(t *testing.T, fixture *synVerticalFixture) saports.ShipmentSellEvaluationRequestRecord {
	t.Helper()
	requests, err := sapostgres.NewShipmentSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatalf("请求册：%v", err)
	}
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	record, found, err := requests.FindByNaturalKey(t.Context(), tenant, sadomain.ShipmentSellEvaluationRequestNaturalKey{
		Shipment: mustPS(t, sadomain.NewShipmentReference, fixture.requestID.String()),
		Moment:   sadomain.ShipmentMembersIntakeAdopted,
	})
	if err != nil || !found || record.State != sadomain.SellRequestFormed {
		t.Fatalf("委托级请求 found=%v state=%s err=%v", found, record.State, err)
	}
	return record
}

func shipmentEvaluation(t *testing.T, fixture *synVerticalFixture, record saports.ShipmentSellEvaluationRequestRecord) ppdomain.PricingEvaluation {
	t.Helper()
	return evaluationByRequest(t, fixture, record.Key.Request.String())
}

func parcelSellEvaluation(t *testing.T, fixture *synVerticalFixture, parcelID string) ppdomain.PricingEvaluation {
	t.Helper()
	requests, err := sapostgres.NewSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatalf("包裹请求册：%v", err)
	}
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	record, found, err := requests.FindByNaturalKey(t.Context(), tenant, sadomain.SellEvaluationRequestNaturalKey{
		Parcel: mustPS(t, sadomain.NewParcelReference, parcelID),
		Moment: sadomain.BillingMomentNetworkIntakeAdopted,
	})
	if err != nil || !found || record.State != sadomain.SellRequestFormed {
		t.Fatalf("包裹 %s 请求 found=%v state=%s err=%v", parcelID, found, record.State, err)
	}
	return evaluationByRequest(t, fixture, record.Key.Request.String())
}

func evaluationByRequest(t *testing.T, fixture *synVerticalFixture, requestID string) ppdomain.PricingEvaluation {
	t.Helper()
	evaluations, err := pppostgres.NewEvaluations(fixture.db)
	if err != nil {
		t.Fatalf("评价库：%v", err)
	}
	evaluation, found, err := evaluations.FindByRequestReference(t.Context(),
		ppValue(t, ppdomain.NewTenantID, fixture.identity.TenantID().String()),
		ppValue(t, ppdomain.NewEvaluationRequestReference, requestID))
	if err != nil || !found {
		t.Fatalf("评价 %s found=%v err=%v", requestID, found, err)
	}
	return evaluation
}

func explanationHasSellSource(evaluation ppdomain.PricingEvaluation, scope, kind string) bool {
	want := "sell price source " + scope + ": " + kind
	for _, line := range evaluation.Explanation() {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func registerParcelSellAccount(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatalf("账户册：%v", err)
	}
	adoptions, err := sapostgres.NewSellBillingMomentAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("包裹采用册：%v", err)
	}
	account := synRReceivableAccount(t)
	adoption, err := sadomain.NewSellBillingMomentAdoption(account.ID(), sadomain.BillingMomentNetworkIntakeAdopted,
		mustPS(t, sadomain.NewEstimatePricingScope, sellChainPricingScope))
	if err != nil {
		t.Fatalf("采用：%v", err)
	}
	at := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		if _, err := accounts.Save(txCtx, tenant, account, at); err != nil {
			return err
		}
		_, err := adoptions.SaveSellBillingMomentAdoption(txCtx, tenant, adoption, at)
		return err
	})
}

func saveSellPriceSources(t *testing.T, fixture *synVerticalFixture, scopes ...string) {
	t.Helper()
	contract := sellChainVersion(t, fixture.identity.TenantID().String(), pcdomain.CustomerContractObject, sellChainContract)
	sources := make([]pcdomain.PricingScopeReference, 0, len(scopes))
	for _, scope := range scopes {
		sources = append(sources, mustPC(t, pcdomain.NewPricingScopeReference, scope))
	}
	arrangement, err := pcdomain.DeclareContractSellPriceArrangement(contract, sources, pcdomain.NoSellPriceBasis{})
	if err != nil {
		t.Fatalf("销售价格约定：%v", err)
	}
	publications, err := pcpostgres.NewCommercialPublications(fixture.db)
	if err != nil {
		t.Fatalf("发布册：%v", err)
	}
	var outcome pcports.DeclarationSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = publications.SaveContractSellPriceArrangement(txCtx, arrangement)
		return err
	})
	if outcome != pcports.DeclarationSaved {
		t.Fatalf("保存销售价格来源 = %s", outcome)
	}
}

type sellCardSpec struct {
	planID     string
	scope      string
	amount     string
	starts     time.Time
	ends       time.Time
	aggregate  bool
	zones      []string
	volumetric bool
}

func catalogueSellCard(t *testing.T, spec sellCardSpec) ppdomain.PriceCardRegistration {
	t.Helper()
	currency := mustPS(t, ppdomain.NewCurrency, synRCurrency)
	entries := make([]ppdomain.RateEntry, 0, len(spec.zones))
	for index, zone := range spec.zones {
		entry, err := ppdomain.NewRateEntry(
			ppValue(t, ppdomain.NewRateEntryID, spec.planID+"-"+zone),
			zone, kilograms(t, "0"), kilograms(t, "30"),
			ppValue(t, func(raw string) (ppdomain.Money, error) {
				return ppdomain.NewMoney(ppValue(t, ppdomain.ParseDecimal, raw), currency)
			}, spec.amount),
		)
		if err != nil {
			t.Fatalf("价表行 %d：%v", index, err)
		}
		entries = append(entries, entry)
	}
	period, err := ppdomain.NewEffectivePeriod(spec.starts, spec.ends)
	if err != nil {
		t.Fatalf("有效期：%v", err)
	}
	table, err := ppdomain.NewRateTableVersion(
		pptest.IdentityReference(t, ppdomain.ArtifactRateTable, spec.planID+"-TABLE", "v1"),
		ppdomain.RateTableFamilyWeightZone, currency, ppdomain.WeightUnitKilogram, period, entries,
	)
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingNone, kilograms(t, "1"))
	if err != nil {
		t.Fatalf("进位：%v", err)
	}
	var factor *ppdomain.VolumetricFactor
	method := ppdomain.PricingWeightActualOnly
	if spec.volumetric {
		method = ppdomain.PricingWeightMax
		factorRounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, kilograms(t, "0.1"))
		if err != nil {
			t.Fatalf("体积进位：%v", err)
		}
		declared, err := ppdomain.NewVolumetricFactor(ppValue(t, ppdomain.ParseDecimal, "5000"), ppdomain.LengthUnitCentimeter, factorRounding)
		if err != nil {
			t.Fatalf("体积系数：%v", err)
		}
		factor = &declared
	}
	policy, err := ppdomain.NewPricingWeightPolicy(
		pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, spec.planID+"-WEIGHT", "v1"),
		method, rounding, factor,
	)
	if err != nil {
		t.Fatalf("重量策略：%v", err)
	}
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	if err != nil {
		t.Fatalf("目录：%v", err)
	}
	structures, err := ppdomain.PricingPlanStructures{}.WithReferenceCatalogues(link)
	if err != nil {
		t.Fatalf("结构：%v", err)
	}
	increment, err := ppdomain.NewMoney(ppValue(t, ppdomain.ParseDecimal, "0.01"), currency)
	if err != nil {
		t.Fatalf("金额进位：%v", err)
	}
	amountRounding, err := ppdomain.NewAmountRoundingPolicy(ppdomain.RoundingHalfUp, increment, []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal})
	if err != nil {
		t.Fatalf("金额取整：%v", err)
	}
	structures, err = structures.WithAmountRounding(amountRounding)
	if err != nil {
		t.Fatalf("金额取整结构：%v", err)
	}
	reference := pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, spec.planID, "v1")
	scope := ppValue(t, ppdomain.NewPricingScopeID, spec.scope)
	charge := ppValue(t, ppdomain.NewChargeCode, "BASE_FREIGHT")
	var plan ppdomain.PricingPlanVersion
	if spec.aggregate {
		plan, err = ppdomain.NewAggregatePricingPlanVersion(ppdomain.AggregationPerShipment, reference, scope,
			ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, charge, period, table, policy, nil, structures)
	} else {
		plan, err = ppdomain.NewPricingPlanVersion(reference, scope, ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge,
			charge, period, table, policy, nil, structures)
	}
	if err != nil {
		t.Fatalf("价卡：%v", err)
	}
	return pricingResumeRegistration(t, spec.planID, plan)
}
