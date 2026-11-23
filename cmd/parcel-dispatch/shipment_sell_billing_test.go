package main

import (
	"context"
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
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
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
