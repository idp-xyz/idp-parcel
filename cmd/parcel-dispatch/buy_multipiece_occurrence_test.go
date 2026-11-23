package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ppcommercial "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/partycommercial"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppapplication "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证票 14：一个带多件已受理包裹的运输收费发生项，在逐委托采购卡上形成一份 BUY 评价和一笔供应商预期成本。
// 计价重是逐件取实重与体积重的大者再相加。单件摘要与包裹主体构造器相同。逐包裹卡遇到多件仍不可得。邮编不一致待判断。
// 夹具全部 SYN-，证据只记 S。

const (
	multiBuyScope    = "SYN-SCOPE-CFMR14-SHIP"
	packageBuyScope  = "SYN-SCOPE-CFMR14-PKG"
	multiAgreement   = "SYN-AGR-CFMR14-SHIP"
	packageAgreement = "SYN-AGR-CFMR14-PKG"
	multiPlanID      = "SYN-PLAN-CFMR14-SHIP"
	packagePlanID    = "SYN-PLAN-CFMR14-PKG"
)

var occurrenceBusinessAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

func TestAMultiPieceChargeOccurrenceFormsOneBuyEvaluationAndOneExpectedCost(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	acceptPricedShipment(t, fixture, fixture.identity, fixture.requestID.String(), "SYN-DEC-01", "200001", "018956",
		pricedProfile(t, "SYN-PARCEL-01", "2", "30", "20", "10"),
		pricedProfile(t, "SYN-PARCEL-02", "1", "50", "40", "30"),
	)
	secondIdentity := sourceIdentity(t, "SYN-KEY-CFMR14")
	acceptPricedShipment(t, fixture, secondIdentity, "SYN-REQ-CFMR14", "SYN-DEC-CFMR14", "200001", "238001",
		pricedProfile(t, "SYN-PARCEL-03", "1.5", "20", "20", "20"),
	)
	for step := 0; step < 4; step++ {
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("受理链第 %d 拍：%v", step, err)
		}
	}

	registerDemoZoneCatalogue(t, fixture.db)
	registerPerShipmentBuyCard(t, fixture)
	registerPerPackageBuyCard(t, fixture)
	registerScopedAgreement(t, fixture.db, multiAgreement, multiBuyScope, multiPlanID)
	registerScopedAgreement(t, fixture.db, packageAgreement, packageBuyScope, packagePlanID)
	registerBuyTriggers(t, fixture.db, demoTenant)

	insertOccurrenceMembers(t, fixture, "SYN-OCC-MULTI", multiAgreement+"/v1", "SYN-PARCEL-01", "SYN-PARCEL-02")
	insertOccurrenceMembers(t, fixture, "SYN-OCC-ONE", packageAgreement+"/v1", "SYN-PARCEL-01")
	insertOccurrenceMembers(t, fixture, "SYN-OCC-PKG", packageAgreement+"/v1", "SYN-PARCEL-01", "SYN-PARCEL-02")
	insertOccurrenceMembers(t, fixture, "SYN-OCC-SPLIT", multiAgreement+"/v1", "SYN-PARCEL-01", "SYN-PARCEL-03")

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"SYN-OCC-MULTI", "SYN-OCC-ONE", "SYN-OCC-PKG", "SYN-OCC-SPLIT"} {
		enqueueForBeat(t, fixture.db, store, "formed-"+id, sainbox.ChargeOccurrenceFormedEventType,
			`{"tenantId":"`+demoTenant+`","occurrence":"`+id+`","version":"V1"}`)
	}
	for step := 0; step < 16; step++ {
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("第 %d 拍：%v", step, err)
		}
	}

	multiRequest, multiStop := buyRequestOf(t, fixture, "SYN-OCC-MULTI")
	if multiStop != "" {
		t.Fatalf("多件逐委托请求停在 %s", multiStop)
	}
	multiEval := evaluationOf(t, fixture, multiRequest)
	if multiEval.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("多件评价 = %s %v", multiEval.Status(), multiEval.Issues())
	}
	subject, ok := multiEval.Input().Subject()
	if !ok || subject.Kind() != ppdomain.SubjectTransportChargeOccurrence || subject.Reference() != "SYN-OCC-MULTI" {
		t.Fatalf("主体 = %+v %v", subject, ok)
	}
	if !multiEval.Input().BusinessAt().Equal(occurrenceBusinessAt) {
		t.Fatalf("业务时点 = %s，want 发生时刻 %s", multiEval.Input().BusinessAt(), occurrenceBusinessAt)
	}
	weight, weighed := multiEval.PricingWeight()
	if !weighed || weight.RoundedWeight().Value().String() != "14" {
		got := ""
		if weighed {
			got = weight.RoundedWeight().Value().String()
		}
		t.Fatalf("计价重量 = %s present=%v，want 逐件取大者之和 14（2 与 12）", got, weighed)
	}
	if n := costCount(t, fixture, "SYN-OCC-MULTI"); n != 1 {
		t.Fatalf("多件预期成本 %d 笔, want 1", n)
	}

	oneRequest, oneStop := buyRequestOf(t, fixture, "SYN-OCC-ONE")
	if oneStop != "" {
		t.Fatalf("单件请求停在 %s", oneStop)
	}
	oneEval := evaluationOf(t, fixture, oneRequest)
	if oneEval.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("单件评价 = %s %v", oneEval.Status(), oneEval.Issues())
	}
	oneSubject, _ := oneEval.Input().Subject()
	if oneSubject.Kind() != ppdomain.SubjectAcceptedPackage || oneSubject.Reference() != "SYN-PARCEL-01" {
		t.Fatalf("单件主体 = %+v", oneSubject)
	}
	if oneEval.SemanticDigest() != singlePieceDigest(t, oneEval) {
		t.Fatal("单件摘要与包裹主体构造器算出的那一份不同")
	}
	if n := costCount(t, fixture, "SYN-OCC-ONE"); n != 1 {
		t.Fatalf("单件预期成本 %d 笔, want 1", n)
	}

	_, packageStop := buyRequestOf(t, fixture, "SYN-OCC-PKG")
	if packageStop != "PRICING_INPUT_UNAVAILABLE" {
		t.Fatalf("逐包裹卡遇到多件停因 = %s, want PRICING_INPUT_UNAVAILABLE", packageStop)
	}
	if n := costCount(t, fixture, "SYN-OCC-PKG"); n != 0 {
		t.Fatalf("逐包裹卡仍形成了 %d 笔成本", n)
	}

	splitRequest, splitStop := buyRequestOf(t, fixture, "SYN-OCC-SPLIT")
	if splitStop != "EVALUATION_CATALOGUE_UNRESOLVED" {
		t.Fatalf("邮编不一致停因 = %s, want 分区目录解不开的待判断", splitStop)
	}
	splitEval := evaluationOf(t, fixture, splitRequest)
	if splitEval.Status() != ppdomain.EvaluationPending || len(splitEval.Issues()) == 0 || splitEval.Issues()[0].Code() != "ZONE_UNRESOLVED" {
		t.Fatalf("邮编不一致评价 = %s %v，want 待判断", splitEval.Status(), splitEval.Issues())
	}
	if _, present := splitEval.Input().PostalRoute(); present {
		t.Fatal("邮编不一致却挑了一条路线")
	}
	if n := costCount(t, fixture, "SYN-OCC-SPLIT"); n != 0 {
		t.Fatalf("待判断仍形成了 %d 笔成本", n)
	}
}

func singlePieceDigest(t *testing.T, stored ppdomain.PricingEvaluation) string {
	t.Helper()
	subject, err := ppdomain.NewAcceptedPackageSubject(ppValue(t, ppdomain.NewPackageID, "SYN-PARCEL-01"))
	if err != nil {
		t.Fatal(err)
	}
	route, err := ppdomain.NewPostalRoute("200001", "018956")
	if err != nil {
		t.Fatal(err)
	}
	weight, err := ppdomain.NewWeightFromString("2", ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	sides, err := ppdomain.NewDimensionsFromStrings("30", "20", "10", ppdomain.LengthUnitCentimeter)
	if err != nil {
		t.Fatal(err)
	}
	input, err := ppdomain.NewPostalPricingInputSnapshot(
		ppValue(t, ppdomain.NewTenantID, demoTenant),
		ppValue(t, ppdomain.NewPricingScopeID, packageBuyScope),
		subject, route, weight, &sides, occurrenceBusinessAt,
		versionedFact(t, ppdomain.ArtifactDeclaredMeasurement, "SYN-PARCEL-01", "declaration@baseline"),
		versionedFact(t, ppdomain.ArtifactAddressElements, "SYN-PARCEL-01", "address@baseline"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if readings := stored.Input().CatalogueReadings(); len(readings) > 0 {
		input, err = input.WithReferenceCatalogues(readings...)
		if err != nil {
			t.Fatal(err)
		}
	}
	packagePlan := perPackagePlan(t)
	return ppdomain.EvaluatePricing(mustEvaluationRequest(t, packagePlan, input)).SemanticDigest()
}

func mustEvaluationRequest(t *testing.T, plan ppdomain.PricingPlanVersion, input ppdomain.PricingInputSnapshot) ppdomain.EvaluationRequest {
	t.Helper()
	request, err := ppdomain.NewEvaluationRequest(ppValue(t, ppdomain.NewEvaluationID, "SYN-EVAL-DIGEST"), plan, input, ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func versionedFact(t *testing.T, kind ppdomain.ArtifactKind, id, version string) ppdomain.VersionedFactReference {
	t.Helper()
	reference, err := ppdomain.NewVersionReferenceIdentity(kind, id, version)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ppdomain.NewVersionedFactReference(reference)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func buyRequestOf(t *testing.T, fixture *synVerticalFixture, occurrence string) (string, string) {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var requestID, stop string
	err = querier.QueryRow(t.Context(), `
		SELECT request_id, COALESCE(stop_cause, '')
		  FROM settlement_accounting.evaluation_request
		 WHERE tenant_id = $1 AND occurrence_id = $2`, demoTenant, occurrence).Scan(&requestID, &stop)
	if err != nil {
		t.Fatalf("读 %s 的请求：%v", occurrence, err)
	}
	return requestID, stop
}

func evaluationOf(t *testing.T, fixture *synVerticalFixture, requestID string) ppdomain.PricingEvaluation {
	t.Helper()
	evaluations, err := pppostgres.NewEvaluations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, found, err := evaluations.FindByRequestReference(t.Context(),
		ppValue(t, ppdomain.NewTenantID, demoTenant),
		ppValue(t, ppdomain.NewEvaluationRequestReference, requestID))
	if err != nil || !found {
		t.Fatalf("评价 %s found=%v err=%v", requestID, found, err)
	}
	return evaluation
}

func costCount(t *testing.T, fixture *synVerticalFixture, occurrence string) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := querier.QueryRow(t.Context(), `
		SELECT count(*) FROM settlement_accounting.supplier_expected_cost
		 WHERE tenant_id = $1 AND occurrence_id = $2`, demoTenant, occurrence).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func acceptPricedShipment(
	t *testing.T,
	fixture *synVerticalFixture,
	identity psdomain.SourceIdentity,
	requestID, decisionID, sender, delivery string,
	profiles ...psdomain.DeclaredParcelProfile,
) {
	t.Helper()
	ctx := t.Context()
	command := fixture.submitCommand(t)
	command.Identity = identity
	command.ShipmentRequestID = mustPS(t, psdomain.NewShipmentRequestID, requestID)
	command.PayloadDigest = mustPS(t, psdomain.NewPayloadDigest, "SYN-DIGEST-"+requestID)
	command.BatchID = mustPS(t, psdomain.NewSubmissionBatchID, "SYN-BATCH-"+requestID)
	command.DeclaredParcelIDs = make([]psdomain.DeclaredParcelID, 0, len(profiles))
	for _, profile := range profiles {
		command.DeclaredParcelIDs = append(command.DeclaredParcelIDs, profile.Parcel())
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
		t.Fatalf("%s submit = %s", requestID, outcome)
	}
	request, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("读回 %s：%v found=%v", requestID, err, found)
	}
	version := request.CurrentSubmissionVersion().VersionID()
	asOf := synJudgmentAsOf(t, psdomain.ReachabilityJudgmentKind, time.Now().UTC().Add(-time.Minute))
	controlAsOf := synJudgmentAsOf(t, psdomain.FinancialControlJudgmentKind, time.Now().UTC().Add(-time.Minute))
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		for _, profile := range profiles {
			reachable, err := psdomain.NewReachabilityJudgment(psdomain.ReachabilityJudgmentSpec{
				JudgmentID: mustPS(t, psdomain.NewReachabilityJudgmentID, requestID+"-NRJ-"+profile.Parcel().String()),
				ParcelID:   profile.Parcel(),
				Value:      psdomain.ReachabilityReachable,
				AsOf:       asOf,
			})
			if err != nil {
				return err
			}
			reachable = reachable.FormedUnder(mustPS(t, psdomain.NewCommercialResolutionID, "SYN-RES-01"))
			if err := fixture.judgments.RecordReachabilityJudgment(txCtx, identity.TenantID(), command.ShipmentRequestID, version, reachable); err != nil {
				return err
			}
		}
		return fixture.judgments.RecordFinancialControlResult(txCtx, identity.TenantID(), command.ShipmentRequestID, version,
			synHeldControl(t, requestID+"-SAC", controlAsOf))
	})
	form := acceptanceFormer(t, fixture, decisionID)
	var result psapplication.FormAcceptanceDecisionResult
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = form.Handle(txCtx, psapplication.FormAcceptanceDecisionCommand{
			Identity: identity, ShipmentRequestID: command.ShipmentRequestID, SubmissionVersion: version,
		})
		return err
	})
	if result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("%s state = %s pending = %s", requestID, result.State(), result.PendingReason())
	}
}

func acceptanceFormer(t *testing.T, fixture *synVerticalFixture, decisionID string) *psapplication.FormAcceptanceDecisionHandler {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := pspostgres.NewOutboxAcceptanceDecisionHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	return psapplication.NewFormAcceptanceDecisionHandler(psapplication.FormAcceptanceDecisionDeps{
		Requests:     fixture.requests,
		Commercial:   &synSCommercialBasis{t: t},
		Reachability: &synSReachabilityRevalidator{},
		Judgments:    fixture.judgments,
		Recorder:     fixture.judgments,
		Release:      &synSControlRelease{},
		Downstream:   handoff,
		Identities:   fixedDecisionID(decisionID),
		Clock:        systemClock{},
	})
}

type fixedDecisionID string

func (id fixedDecisionID) NextAcceptanceDecisionID(context.Context) (psdomain.AcceptanceDecisionID, error) {
	return psdomain.NewAcceptanceDecisionID(string(id))
}

func sourceIdentity(t *testing.T, key string) psdomain.SourceIdentity {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		mustPS(t, psdomain.NewTenantID, demoTenant),
		mustPS(t, psdomain.NewCustomerAccountID, "SYN-CUSTOMER-01"),
		mustPS(t, psdomain.NewSource, "SYN-SOURCE-01"),
		mustPS(t, psdomain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func pricedProfile(t *testing.T, parcel, kilograms, length, width, height string) psdomain.DeclaredParcelProfile {
	t.Helper()
	weight, err := psdomain.NewDeclaredWeight(
		mustPS(t, psdomain.NewMeasurementValue, kilograms),
		mustPS(t, psdomain.NewMeasurementUnitReference, "KG"))
	if err != nil {
		t.Fatal(err)
	}
	sides, err := psdomain.NewDeclaredDimensions(
		mustPS(t, psdomain.NewMeasurementValue, length),
		mustPS(t, psdomain.NewMeasurementValue, width),
		mustPS(t, psdomain.NewMeasurementValue, height),
		mustPS(t, psdomain.NewMeasurementUnitReference, "CM"))
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, sides)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := psdomain.NewDeclaredParcelProfile(mustPS(t, psdomain.NewDeclaredParcelID, parcel), measurement)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func insertOccurrenceMembers(t *testing.T, fixture *synVerticalFixture, id, agreement string, parcels ...string) {
	t.Helper()
	spec := tfdomain.TransportChargeOccurrenceSpec{Reason: tfdomain.ActualFulfillmentOccurrence, Quantity: 1, OccurredAt: occurrenceBusinessAt}
	var err error
	if spec.TenantID, err = tfdomain.NewTenantID(demoTenant); err != nil {
		t.Fatal(err)
	}
	if spec.Occurrence, err = tfdomain.NewChargeOccurrenceReference(id); err != nil {
		t.Fatal(err)
	}
	if spec.Journey, err = tfdomain.NewJourneyReference("SYN-JRN-" + id); err != nil {
		t.Fatal(err)
	}
	if spec.LegalEntity, err = tfdomain.NewProcurementLegalEntityReference("SYN-LE-01"); err != nil {
		t.Fatal(err)
	}
	if spec.Provider, err = tfdomain.NewServiceProviderReference("SYN-PARTY-CARRIER-TRUNK-01"); err != nil {
		t.Fatal(err)
	}
	if spec.Agreement, err = tfdomain.NewAgreementSnapshotReference(agreement); err != nil {
		t.Fatal(err)
	}
	if spec.FactBasis, err = tfdomain.NewOccurrenceBasisReference("SYN-BASIS-" + id); err != nil {
		t.Fatal(err)
	}
	if spec.Scope, err = tfdomain.NewOccurrenceScopeReference("SYN-SEGMENT-" + id); err != nil {
		t.Fatal(err)
	}
	for _, parcel := range parcels {
		member, err := tfdomain.NewCarriedObjectReference(parcel)
		if err != nil {
			t.Fatal(err)
		}
		spec.Members = append(spec.Members, member)
	}
	if spec.Unit, err = tfdomain.NewQuantityUnitReference("SEGMENT"); err != nil {
		t.Fatal(err)
	}
	if spec.Validity, err = tfdomain.NewOccurrenceValidityVersion("V1"); err != nil {
		t.Fatal(err)
	}
	registrant, err := tfdomain.NewRegistrantReference("https://dex.syn.example#SYN-OPERATOR-CONFIG-01")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Condition, err = tfdomain.NewAssertedOccurrenceCondition(registrant, "SYN 断言："+agreement+" 的发生条件已满足"); err != nil {
		t.Fatal(err)
	}
	occurrence, err := tfdomain.FormTransportChargeOccurrence(spec)
	if err != nil {
		t.Fatal(err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := occurrences.Save(txCtx, tfports.ChargeOccurrenceRecord{
			Key:        tfports.ChargeOccurrenceKey{TenantID: spec.TenantID, Occurrence: spec.Occurrence, Validity: spec.Validity},
			Occurrence: occurrence,
			RecordedAt: beatInstant(),
		})
		if err == nil && outcome != tfports.ChargeOccurrenceSaved {
			err = errors.New("charge occurrence was not saved fresh")
		}
		return err
	})
}

func registerScopedAgreement(t *testing.T, db *bentopg.DB, object, scope, plan string) {
	t.Helper()
	version := sellChainVersion(t, demoTenant, pcdomain.SupplierAgreementObject, object)
	agreement, err := pcdomain.NewSupplierAgreement(
		version,
		mustPC(t, pcdomain.NewPartyID, "SYN-PARTY-CARRIER-TRUNK-01"),
		mustPC(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
		mustPC(t, pcdomain.NewCommercialScopeReference, scope),
		mustPC(t, pcdomain.NewPricingPlanReference, plan),
		version.Effective(),
	)
	if err != nil {
		t.Fatal(err)
	}
	perPiece, err := pcdomain.NewChargedOccurrenceCondition(pcdomain.ActualFulfillmentChargeReason, pcdomain.PerPieceQuantity)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := pcdomain.NewChargeOccurrenceConditions(perPiece)
	if err != nil {
		t.Fatal(err)
	}
	if agreement, err = agreement.WithChargeConditions(conditions); err != nil {
		t.Fatal(err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if outcome, err := publications.SaveVersion(txCtx, version); err != nil || outcome != pcports.PublicationSaved {
			return errors.Join(err, errors.New(outcome.String()))
		}
		if outcome, err := publications.SaveSupplierAgreement(txCtx, agreement); err != nil || outcome != pcports.SupplierAgreementSaved {
			return errors.Join(err, errors.New(outcome.String()))
		}
		return nil
	})
}

func registerPerShipmentBuyCard(t *testing.T, fixture *synVerticalFixture) ppdomain.PricingPlanVersion {
	t.Helper()
	base := buyCardBase(t, multiPlanID, multiBuyScope)
	factorRounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, kilograms(t, "0.1"))
	if err != nil {
		t.Fatal(err)
	}
	factor, err := ppdomain.NewVolumetricFactor(ppValue(t, ppdomain.ParseDecimal, "5000"), ppdomain.LengthUnitCentimeter, factorRounding)
	if err != nil {
		t.Fatal(err)
	}
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingNone, kilograms(t, "1"))
	if err != nil {
		t.Fatal(err)
	}
	weight, err := ppdomain.NewPricingWeightPolicy(
		pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, multiPlanID+"-WEIGHT", "v1"),
		ppdomain.PricingWeightMax, rounding, &factor)
	if err != nil {
		t.Fatal(err)
	}
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	if err != nil {
		t.Fatal(err)
	}
	structures, err := base.Structures().WithReferenceCatalogues(link)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ppdomain.NewAggregatePricingPlanVersion(ppdomain.AggregationPerShipment,
		base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), weight, nil, structures)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = plan.WithShipmentWeightAggregation(ppdomain.ShipmentWeightPerPieceThenSum, ppdomain.ShipmentWeightSumThenRoundOnce)
	if err != nil {
		t.Fatal(err)
	}
	registerNewerPriceCard(t, fixture.db, pricingResumeRegistration(t, multiPlanID, plan))
	return plan
}

// registerNewerPriceCard 打开较新规范化版本的登记门。逐件求和的卡写 PPC-8，关着的门会拒。
func registerNewerPriceCard(t *testing.T, db *bentopg.DB, registration ppdomain.PriceCardRegistration) {
	t.Helper()
	if err := publishDirectionAuthorization(t.Context(), db, registration); err != nil {
		t.Fatal(err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	authorizations, err := ppcommercial.NewDirectionAuthorizations(publications)
	if err != nil {
		t.Fatal(err)
	}
	handler := ppapplication.NewRegisterPriceCardHandler(ppapplication.RegisterPriceCardDeps{
		Catalog: cards, Registered: pricingParameterHandoffForTest(t, db),
		DirectionAuthorizations: authorizations, RegisteredContent: cards, Clock: systemClock{},
		NewerCanonicalizationEnabled: true,
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

func registerPerPackageBuyCard(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	registerPriceCardForTest(t, fixture.db, pricingResumeRegistration(t, packagePlanID, perPackagePlan(t)))
}

func perPackagePlan(t *testing.T) ppdomain.PricingPlanVersion {
	t.Helper()
	base := buyCardBase(t, packagePlanID, packageBuyScope)
	link, err := ppdomain.NewReferenceCatalogueLink(ppdomain.CatalogueKindZone, "SYN-CAT-ZONE-CN-SG")
	if err != nil {
		t.Fatal(err)
	}
	structures, err := base.Structures().WithReferenceCatalogues(link)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ppdomain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(),
		base.BaseChargeCode(), base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func kilograms(t *testing.T, raw string) ppdomain.Weight {
	t.Helper()
	weight, err := ppdomain.NewWeight(ppValue(t, ppdomain.ParseDecimal, raw), ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	return weight
}

func buyCardBase(t *testing.T, planID, scope string) ppdomain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, planID, "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, planID+"-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, planID+"-WEIGHT", "v1"),
		Scope:                 scope,
		Direction:             ppdomain.PricingDirectionBuy,
		Purpose:               ppdomain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
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
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode: ppdomain.RoundingHalfUp, Increment: "0.01", Points: []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
}
