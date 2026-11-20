package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func weightTolerance(t *testing.T, delta, unit string) domain.WeightDiscrepancyGateRule {
	t.Helper()
	rule, err := domain.AllowWeightDeltaUpTo(delta, unit)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func registerWeightRule(
	t *testing.T,
	fixture *viewFixture,
	registry *adapter.GateConditionRegistrations,
	rule domain.WeightDiscrepancyGateRule,
) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	tenant, scope, boundary := synGateKey(t)
	return register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterWeightDiscrepancyGateRule(ctx, tenant, scope, domain.OutboundRelease, boundary, rule)
	})
}

func TestBothWeightDiscrepancyGateRuleShapesRoundTrip(t *testing.T) {
	registry, view, fixture := newGateRegistry(t)
	registerSynGateCatalog(t, fixture, registry)
	tenant, scope, boundary := synGateKey(t)

	if _, found, err := view.LoadWeightDiscrepancyGateRule(t.Context(), tenant, scope, domain.OutboundRelease, boundary); err != nil || found {
		t.Fatalf("未登规则该 found=false：err=%v found=%v", err, found)
	}
	outcome, err := registerWeightRule(t, fixture, registry, weightTolerance(t, "0.50", "KG"))
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首次登记：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := view.LoadWeightDiscrepancyGateRule(t.Context(), tenant, scope, domain.OutboundRelease, boundary)
	if err != nil || !found {
		t.Fatalf("读不回：err=%v found=%v", err, found)
	}
	delta, unit, ok := loaded.MaxDelta()
	if !ok || delta != "0.5" || unit != "KG" || loaded.NotAPrecondition() {
		t.Fatalf("容差走样：%q %q ok=%v", delta, unit, ok)
	}
	outcome, err = registerWeightRule(t, fixture, registry, domain.WeightDiscrepancyNotAPrecondition())
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键重登：err=%v outcome=%v", err, outcome)
	}
	loaded, _, err = view.LoadWeightDiscrepancyGateRule(t.Context(), tenant, scope, domain.OutboundRelease, boundary)
	if err != nil || loaded.NotAPrecondition() {
		t.Fatalf("首版被顶替：err=%v %+v", err, loaded)
	}

	other := viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, other, registryBaseAt)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterWeightDiscrepancyGateRule(ctx, tenant, scope, domain.OutboundRelease, other, domain.WeightDiscrepancyNotAPrecondition())
	}); err != nil {
		t.Fatal(err)
	}
	waived, found, err := view.LoadWeightDiscrepancyGateRule(t.Context(), tenant, scope, domain.OutboundRelease, other)
	if err != nil || !found || !waived.NotAPrecondition() {
		t.Fatalf("不构成前置条件没读回：err=%v found=%v", err, found)
	}
}

func TestTheWeightDiscrepancyGateRuleTableRejectsAMissingCatalogAndAMixedShape(t *testing.T) {
	registry, _, fixture := newGateRegistry(t)
	if _, err := registerWeightRule(t, fixture, registry, weightTolerance(t, "0.5", "KG")); err == nil {
		t.Fatal("目录不在场的规则被接受了")
	}
	registerSynGateCatalog(t, fixture, registry)
	insert := `INSERT INTO customs_compliance.gate_condition_weight_discrepancy_rule
		(tenant_id, scope_ref, action, boundary_ref, not_a_precondition, max_delta, weight_unit, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	fixture.rejects(t, "不构成前置条件却带容差", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", true, "0.5", "KG", registryBaseAt)
	fixture.rejects(t, "容差缺单位", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", false, "0.5", nil, registryBaseAt)
	fixture.rejects(t, "单位不在重量词表", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", false, "0.5", "CM", registryBaseAt)
	fixture.rejects(t, "容差空白", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", false, "  ", "KG", registryBaseAt)
}

func TestGateConditionCatalogueListsTheWeightDiscrepancyRule(t *testing.T) {
	registry, _, fixture := newGateRegistry(t)
	registerSynGateCatalog(t, fixture, registry)
	tenant, scope, _ := synGateKey(t)
	if _, err := registerWeightRule(t, fixture, registry, weightTolerance(t, "0.50", "KG")); err != nil {
		t.Fatal(err)
	}
	other := viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, other, registryBaseAt)
	}); err != nil {
		t.Fatal(err)
	}
	catalogue, err := adapter.NewGateConditionCatalogue(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := rowsOf(catalogue.ListGateConditions(t.Context(), tenant, 10, firstPage(t, ports.GateConditionPages)))
	if err != nil || len(entries) != 2 {
		t.Fatalf("上列：err=%v entries=%d", err, len(entries))
	}
	byBoundary := map[string]*domain.WeightDiscrepancyGateRule{}
	for _, entry := range entries {
		byBoundary[entry.Boundary.String()] = entry.WeightDiscrepancyRule
	}
	if byBoundary["SYN-PROC-EXPORT"] != nil {
		t.Fatal("未登规则的目录行凭空带了规则")
	}
	listed := byBoundary["SYN-PROC-IMPORT"]
	if listed == nil {
		t.Fatal("登了规则的目录行没带回规则")
	}
	delta, unit, ok := listed.MaxDelta()
	if !ok || delta != "0.5" || unit != "KG" {
		t.Fatalf("上列的规则走样：%q %q ok=%v", delta, unit, ok)
	}
}

func TestAGateVerificationRoundTripsItsWeightDiscrepancyReading(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()
	gate, key := weightGate(t, "2.40", "SYN-WEIGH-IN")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.gates.Save(txCtx, key, gate)
		return err
	})
	found, exists, err := fixture.gates.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("读回：%v exists=%v", err, exists)
	}
	got, has := found.WeightDiscrepancy()
	want, _ := gate.WeightDiscrepancy()
	if !has || got.State != want.State || got.MaxDelta != want.MaxDelta || len(got.Members) != 1 ||
		got.Members[0].MeasuredWeight != "2.40" || got.Members[0].MeasurementSource != "SYN-WEIGH-IN" ||
		!got.Members[0].WithinTolerance {
		t.Fatalf("读数没原样读回：%+v", got)
	}

	plain, plainKey := synGateWithReading(t, "SYN-DIGEST-PLAIN")
	if _, has := plain.WeightDiscrepancy(); has {
		t.Fatal("没挂毛重读数的判断带着读数")
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.gates.Save(txCtx, plainKey, plain)
		return err
	})
	loadedPlain, _, err := fixture.gates.FindByKey(ctx, plainKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := loadedPlain.WeightDiscrepancy(); has {
		t.Fatal("不带毛重列的记录读出了读数")
	}

	changed, changedKey := weightGate(t, "3.00", "SYN-WEIGH-OVER")
	if changedKey.Digest == key.Digest {
		t.Fatal("换了实测没有换指纹")
	}
	var saved ports.GateVerificationSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		outcome, err := fixture.gates.Save(txCtx, changedKey, changed)
		saved = outcome
		return err
	})
	if saved != ports.GateVerificationSaved {
		t.Fatalf("换实测该另成一行：%v", saved)
	}
	var replay ports.GateVerificationSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		outcome, err := fixture.gates.Save(txCtx, key, gate)
		replay = outcome
		return err
	})
	if replay != ports.GateVerificationAlreadyRecorded {
		t.Fatalf("同实测重放：%v", replay)
	}
}

func weightGate(t *testing.T, measured, source string) (domain.ReleaseGateVerification, ports.GateVerificationKey) {
	t.Helper()
	tenant, scope, boundary := synGateKey(t)
	rule := weightTolerance(t, "0.5", "KG")
	version := weightSubmission(t)
	reading, err := rule.Judge(version, map[domain.DeclaredParcelReference]domain.WeightDiscrepancyMemberActual{
		version.Members()[0]: {GrossWeight: measured, GrossWeightUnit: "KG", MeasurementSource: source},
	})
	if err != nil {
		t.Fatal(err)
	}
	findings := []domain.PreconditionFinding{{
		Precondition: domain.WeightDiscrepancyPrecondition,
		State:        reading.State,
	}}
	gate, err := domain.VerifyReleaseGate(scope, domain.OutboundRelease, boundary,
		[]domain.PreconditionReference{domain.WeightDiscrepancyPrecondition},
		domain.GateUnmet, registryBaseAt)
	if reading.State == domain.PreconditionMet {
		gate, err = domain.VerifyReleaseGate(scope, domain.OutboundRelease, boundary,
			[]domain.PreconditionReference{domain.WeightDiscrepancyPrecondition},
			domain.GateMet, registryBaseAt)
	}
	if err != nil {
		t.Fatal(err)
	}
	gate, err = gate.WithWeightDiscrepancy(reading)
	if err != nil {
		t.Fatal(err)
	}
	return gate, ports.GateVerificationKey{
		TenantID: tenant, Scope: scope, Action: domain.OutboundRelease, Boundary: boundary,
		Digest: ports.GateVersionDigest(findings, domain.DutyPaymentGateReading{}, false, reading, true),
	}
}

func weightSubmission(t *testing.T) domain.CustomsSubmissionVersion {
	t.Helper()
	parcel := viewValue(t, domain.NewDeclaredParcelReference, "parcel-a")
	unit, err := domain.FormDeclarationUnit(
		viewValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		viewValue(t, domain.NewCustomsCaseID, "case-1"),
		viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		[]domain.DeclaredParcelReference{parcel})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)
	readiness, err := domain.JudgeReady(unit.ID(), viewValue(t, domain.NewReadinessBasisReference, "readiness-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := domain.GrantSubmissionAuthority(unit.ID(), viewValue(t, domain.NewSubmissionAuthorityReference, "authority-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewMemberMeasuredWeight(parcel, "2.10", "KG", "SYN-WEIGH-FIX", at)
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:            viewValue(t, domain.NewSubmissionVersionID, "submission-1"),
		Unit:          unit,
		Dossier:       viewValue(t, domain.NewDossierSnapshotReference, "dossier-1"),
		Roles:         viewValue(t, domain.NewRoleSnapshotReference, "roles-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       at.Add(time.Hour),
		MemberWeights: []domain.MemberMeasuredWeight{weight},
	})
	if err != nil {
		t.Fatal(err)
	}
	return version
}
