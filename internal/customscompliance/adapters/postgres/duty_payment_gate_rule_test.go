package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 「税费付款」那一道的三件真库往返（0019，票 sa-cc/06）：规则行写读、付款核对「当前版」读口、门禁记录
// 带读数各列。做法承 case_config_registry_test.go 头注：断言穿读口取回、写入一律进环境事务；新表与新列
// 的约束和 SQL 是新写的，先例的绿证不了它们，逐格钉。

func synGateKey(t *testing.T) (domain.TenantID, domain.DecisionScopeReference, domain.CustomsProcedureReference) {
	t.Helper()
	return viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT")
}

func registerSynGateCatalog(t *testing.T, fixture *viewFixture, registry *adapter.GateConditionRegistrations) {
	t.Helper()
	tenant, scope, boundary := synGateKey(t)
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, boundary, registryBaseAt)
	}); err != nil {
		t.Fatalf("登记门禁目录：%v", err)
	}
}

func synAcceptRule(t *testing.T) domain.DutyPaymentGateRule {
	t.Helper()
	rule, err := domain.AcceptDutyPaymentWhen(
		[]domain.DutyCoverage{domain.CoverageFull},
		[]domain.DutyDelta{domain.DeltaNone, domain.DeltaExcess},
		[]domain.DutyFactValidity{domain.FundsFactValid})
	if err != nil {
		t.Fatalf("构造接受集合规则：%v", err)
	}
	return rule
}

func registerDutyRule(t *testing.T, fixture *viewFixture, registry *adapter.GateConditionRegistrations, rule domain.DutyPaymentGateRule) (ports.CaseConfigurationSaveOutcome, error) {
	t.Helper()
	tenant, scope, boundary := synGateKey(t)
	return register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterDutyPaymentGateRule(ctx, tenant, scope, domain.OutboundRelease, boundary, rule)
	})
}

// Covers: 两形各自往返——接受集合逐格读回（去重排序后的形）、「不构成前置条件」读回为那一形；同键重登
// 交回`已登记`且不顶替；未登规则的目录读口答 found=false（门禁编排据以答「规则未配置」）。
func TestBothDutyPaymentGateRuleShapesRoundTripAndNeverReplace(t *testing.T) {
	registry, view, fixture := newGateRegistry(t)
	registerSynGateCatalog(t, fixture, registry)
	tenant, scope, boundary := synGateKey(t)

	if _, found, err := view.LoadDutyPaymentGateRule(t.Context(), tenant, scope, domain.OutboundRelease, boundary); err != nil || found {
		t.Fatalf("未登规则该 found=false：err=%v found=%v", err, found)
	}

	outcome, err := registerDutyRule(t, fixture, registry, synAcceptRule(t))
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首次登记：err=%v outcome=%v", err, outcome)
	}
	loaded, found, err := view.LoadDutyPaymentGateRule(t.Context(), tenant, scope, domain.OutboundRelease, boundary)
	if err != nil || !found || loaded.NotAPrecondition() {
		t.Fatalf("读不回接受集合规则：err=%v found=%v rule=%+v", err, found, loaded)
	}
	coverage, delta, validity := loaded.Accepts()
	if len(coverage) != 1 || coverage[0] != domain.CoverageFull ||
		len(delta) != 2 || delta[0] != domain.DeltaNone || delta[1] != domain.DeltaExcess ||
		len(validity) != 1 || validity[0] != domain.FundsFactValid {
		t.Fatalf("接受集合走样：%v %v %v", coverage, delta, validity)
	}

	outcome, err = registerDutyRule(t, fixture, registry, domain.DutyPaymentNotAPrecondition())
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键重登该交回`已登记`：err=%v outcome=%v", err, outcome)
	}
	loaded, _, err = view.LoadDutyPaymentGateRule(t.Context(), tenant, scope, domain.OutboundRelease, boundary)
	if err != nil || loaded.NotAPrecondition() {
		t.Fatalf("首版被顶替：err=%v rule=%+v", err, loaded)
	}

	otherBoundary := viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, otherBoundary, registryBaseAt)
	}); err != nil {
		t.Fatalf("登记第二个目录：%v", err)
	}
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterDutyPaymentGateRule(ctx, tenant, scope, domain.OutboundRelease, otherBoundary, domain.DutyPaymentNotAPrecondition())
	}); err != nil {
		t.Fatalf("登记不构成前置条件：%v", err)
	}
	waived, found, err := view.LoadDutyPaymentGateRule(t.Context(), tenant, scope, domain.OutboundRelease, otherBoundary)
	if err != nil || !found || !waived.NotAPrecondition() {
		t.Fatalf("「不构成前置条件」没读回那一形：err=%v found=%v rule=%+v", err, found, waived)
	}
}

// Covers: 规则行挂在目录之下——目录不在场时外键拒（与认定行同判据）；库内再守一遍领域不变量：`待确认`
// 进接受集合、两形互斥（不构成前置条件却带集合）、集外词的旁路写入被 CHECK 挡在门外。
func TestTheDutyPaymentGateRuleTableRejectsWhatTheDomainRejects(t *testing.T) {
	registry, _, fixture := newGateRegistry(t)

	if _, err := registerDutyRule(t, fixture, registry, synAcceptRule(t)); err == nil {
		t.Fatal("目录不在场的规则被接受了")
	}
	registerSynGateCatalog(t, fixture, registry)

	insert := `INSERT INTO customs_compliance.gate_condition_duty_payment_rule
		(tenant_id, scope_ref, action, boundary_ref, not_a_precondition, accept_coverage, accept_delta, accept_validity, registered_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb, $9)`
	fixture.rejects(t, "差额待确认登为接受", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", false,
		`["COVERED"]`, `["PENDING"]`, `["VALID"]`, registryBaseAt)
	fixture.rejects(t, "有效性冲突登为接受", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", false,
		`["COVERED"]`, `["NO_DELTA"]`, `["CONFLICTING"]`, registryBaseAt)
	fixture.rejects(t, "不构成前置条件却带集合", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", true,
		`["COVERED"]`, `[]`, `[]`, registryBaseAt)
	fixture.rejects(t, "接受集合为空", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", false,
		`[]`, `["NO_DELTA"]`, `["VALID"]`, registryBaseAt)
	fixture.rejects(t, "覆盖集外词", insert,
		"tenant-a", "SYN-UNIT-01", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", false,
		`["MAYBE"]`, `["NO_DELTA"]`, `["VALID"]`, registryBaseAt)
}

// Covers: 付款核对「当前版」= 核对时刻最新那一版，不按到达顺序——先写晚核对的、后写早核对的，读口仍答
// 晚的那版并带它的版本指纹；没有任何一版时 found=false；空范围是调用方编程错误。
func TestTheCurrentDutyVerificationIsTheLatestByVerifiedAtNotByArrival(t *testing.T) {
	store, fixture := newDutyReconciliation(t)
	tenant, scope, _ := synGateKey(t)

	if _, found, err := store.LoadCurrentDutyVerification(t.Context(), tenant, scope); err != nil || found {
		t.Fatalf("无核对该 found=false：err=%v found=%v", err, found)
	}
	if _, _, err := store.LoadCurrentDutyVerification(t.Context(), tenant, domain.DecisionScopeReference{}); err == nil {
		t.Fatal("空范围被当成正常查询")
	}

	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return store.RegisterFundsFact(ctx, tenant, synFundsFact(t, 1000))
	}); err != nil {
		t.Fatalf("登记资金事实：%v", err)
	}
	later := synVerificationRecord(t, domain.CoverageFull, "SYN-DIGEST-LATER")
	earlier := synVerificationRecord(t, domain.CoveragePartial, "SYN-DIGEST-EARLIER")
	earlierVerification, err := domain.VerifyDutyPayment(
		earlier.Key.Duty, earlier.Key.Funds, earlier.Key.Scope, earlier.Verification.Procedure(),
		domain.CoveragePartial, domain.DeltaShort, domain.FundsFactValid, dutyRegistryBaseAt.Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("构造早一版核对：%v", err)
	}
	earlier.Verification = earlierVerification
	for _, record := range []ports.DutyVerificationRecord{later, earlier} {
		if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
			return store.SaveVerification(ctx, record)
		}); err != nil {
			t.Fatalf("登记核对 %s：%v", record.Key.Digest, err)
		}
	}

	current, found, err := store.LoadCurrentDutyVerification(t.Context(), tenant, scope)
	if err != nil || !found {
		t.Fatalf("读当前版：err=%v found=%v", err, found)
	}
	if current.Key.Digest != "SYN-DIGEST-LATER" || current.Verification.Coverage() != domain.CoverageFull ||
		current.Key.Duty != later.Key.Duty || current.Key.Funds != later.Key.Funds || current.Basis != later.Basis {
		t.Fatalf("当前版该是核对时刻最新那版：%+v", current)
	}
}

func synGateWithReading(t *testing.T, version string) (domain.ReleaseGateVerification, ports.GateVerificationKey) {
	t.Helper()
	tenant, scope, boundary := synGateKey(t)
	findings := []domain.PreconditionFinding{
		{Precondition: viewValue(t, domain.NewPreconditionReference, "RESTRICTION/released"), State: domain.PreconditionMet},
		{Precondition: domain.DutyPaymentPrecondition, State: domain.PreconditionMet},
	}
	preconditions := []domain.PreconditionReference{findings[0].Precondition, findings[1].Precondition}
	gate, err := domain.VerifyReleaseGate(scope, domain.OutboundRelease, boundary, preconditions, domain.GateMet, registryBaseAt)
	if err != nil {
		t.Fatalf("构造门禁核对：%v", err)
	}
	reading := domain.DutyPaymentGateReading{
		State:    domain.PreconditionMet,
		Coverage: domain.CoverageFull,
		Delta:    domain.DeltaExcess,
		Validity: domain.FundsFactValid,
		Verification: domain.DutyVerificationReference{
			Duty:    viewValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
			Funds:   viewValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
			Version: version,
		},
	}
	gate, err = gate.WithDutyPayment(reading)
	if err != nil {
		t.Fatalf("挂读数：%v", err)
	}
	return gate, ports.GateVerificationKey{
		TenantID: tenant, Scope: scope, Action: domain.OutboundRelease, Boundary: boundary,
		Digest: ports.GateVersionDigest(findings, reading, true),
	}
}

// Covers: 票 sa-cc/06 判据 3「门禁记录往返带引用列」——读数各列（判断、三态原值、税费引用、资金事实引用、
// 版本指纹）逐格如实读回；核对换版指纹变即另一行（引用指向新版，旧版原样在册）；没挂读数的版本各列
// 皆空、读口答「无」。
func TestAGateVerificationRoundTripsItsDutyPaymentReadingByReference(t *testing.T) {
	fixture := newCRGFixture(t)
	ctx := t.Context()

	gate, key := synGateWithReading(t, "SYN-DIGEST-V1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.gates.Save(txCtx, key, gate)
		return err
	})
	found, exists, err := fixture.gates.FindByKey(ctx, key)
	if err != nil || !exists {
		t.Fatalf("读回门禁核对：%v exists=%v", err, exists)
	}
	reading, has := found.DutyPayment()
	want, _ := gate.DutyPayment()
	if !has || reading != want || found.Conclusion() != domain.GateMet || len(found.Preconditions()) != 2 {
		t.Fatalf("读数没原样读回：has=%v got=%+v want=%+v", has, reading, want)
	}

	rebased, rebasedKey := synGateWithReading(t, "SYN-DIGEST-V2")
	if rebasedKey == key {
		t.Fatal("核对换版没有换门禁指纹")
	}
	var saved ports.GateVerificationSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		outcome, err := fixture.gates.Save(txCtx, rebasedKey, rebased)
		saved = outcome
		return err
	})
	if saved != ports.GateVerificationSaved {
		t.Fatalf("指向新核对版的门禁该另成一行：%v", saved)
	}
	first, _, err := fixture.gates.FindByKey(ctx, key)
	if err != nil {
		t.Fatalf("读回首版：%v", err)
	}
	if firstReading, _ := first.DutyPayment(); firstReading.Verification.Version != "SYN-DIGEST-V1" {
		t.Fatalf("首版引用被改写：%+v", firstReading)
	}

	plainFindings := []domain.PreconditionFinding{
		{Precondition: viewValue(t, domain.NewPreconditionReference, "RESTRICTION/released"), State: domain.PreconditionMet},
	}
	tenant, scope, boundary := synGateKey(t)
	plain, err := domain.VerifyReleaseGate(scope, domain.OutboundRelease, boundary,
		[]domain.PreconditionReference{plainFindings[0].Precondition}, domain.GateMet, registryBaseAt)
	if err != nil {
		t.Fatalf("构造无读数门禁：%v", err)
	}
	plainKey := ports.GateVerificationKey{
		TenantID: tenant, Scope: scope, Action: domain.OutboundRelease, Boundary: boundary,
		Digest: ports.GateVersionDigest(plainFindings, domain.DutyPaymentGateReading{}, false),
	}
	if plainKey.Digest != ports.FindingsDigest(plainFindings) {
		t.Fatal("没挂读数时门禁指纹该与逐项判断指纹逐字节相同——旧行照旧能撞上")
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.gates.Save(txCtx, plainKey, plain)
		return err
	})
	loadedPlain, _, err := fixture.gates.FindByKey(ctx, plainKey)
	if err != nil {
		t.Fatalf("读回无读数门禁：%v", err)
	}
	if _, has := loadedPlain.DutyPayment(); has {
		t.Fatal("没挂读数的版本读出了读数")
	}
}

// Covers: 库内再守一遍——读数各列同生同灭（只填一半拒）、`待确认` / `冲突` 进不了门禁记录（那一格是未决、
// 不入册）。
func TestTheGateVerificationTableRejectsHalfReadingsAndPendingAxes(t *testing.T) {
	fixture := newCRGFixture(t)
	insert := `INSERT INTO customs_compliance.gate_verification
		(tenant_id, scope_ref, action, boundary_ref, findings_digest, preconditions, conclusion, verified_at,
		 duty_state, duty_coverage, duty_delta, duty_validity, duty_ref, funds_ref, duty_version_digest)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11, $12, $13, $14, $15)`
	base := []any{"tenant-a", "SYN-UNIT-X", "OUTBOUND_RELEASE", "SYN-PROC-IMPORT", "digest-x", `["DUTY_PAYMENT"]`, "MET", registryBaseAt}

	if _, err := fixture.pool.Exec(t.Context(), insert, append(append([]any{}, base...),
		"MET", nil, nil, nil, nil, nil, nil)...); err == nil {
		t.Fatal("库接受了只填一半的读数")
	}
	if _, err := fixture.pool.Exec(t.Context(), insert, append(append([]any{}, base...),
		"MET", "COVERED", "PENDING", "VALID", "SYN-DUTY-01/v1", "SYN-FUNDS-01", "digest-v1")...); err == nil {
		t.Fatal("库接受了差额`待确认`的读数")
	}
	if _, err := fixture.pool.Exec(t.Context(), insert, append(append([]any{}, base...),
		"MET", "COVERED", "NO_DELTA", "CONFLICTING", "SYN-DUTY-01/v1", "SYN-FUNDS-01", "digest-v1")...); err == nil {
		t.Fatal("库接受了有效性`冲突`的读数")
	}
}

// Covers: 目录上列带规则一格——登了规则的目录行 DutyPaymentRule 非空且与登记同形，未登的为 nil（如实透出
// 空位，不替它填）。
func TestTheGateCatalogueListsTheDutyPaymentRuleBesideFindings(t *testing.T) {
	registry, _, fixture := newGateRegistry(t)
	registerSynGateCatalog(t, fixture, registry)
	tenant, scope, _ := synGateKey(t)
	otherBoundary := viewValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")
	if _, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterGateCatalog(ctx, tenant, scope, domain.OutboundRelease, otherBoundary, registryBaseAt)
	}); err != nil {
		t.Fatalf("登记第二个目录：%v", err)
	}
	if _, err := registerDutyRule(t, fixture, registry, synAcceptRule(t)); err != nil {
		t.Fatalf("登记规则：%v", err)
	}
	catalogue, err := adapter.NewGateConditionCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造目录读口：%v", err)
	}

	entries, err := catalogue.ListGateConditions(t.Context(), tenant, 10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("上列：err=%v entries=%d", err, len(entries))
	}
	byBoundary := map[string]*domain.DutyPaymentGateRule{}
	for _, entry := range entries {
		byBoundary[entry.Boundary.String()] = entry.DutyPaymentRule
	}
	if byBoundary["SYN-PROC-EXPORT"] != nil {
		t.Fatal("未登规则的目录行凭空带了规则")
	}
	listed := byBoundary["SYN-PROC-IMPORT"]
	if listed == nil || listed.NotAPrecondition() {
		t.Fatalf("登了规则的目录行没带回规则：%+v", listed)
	}
	if _, delta, _ := listed.Accepts(); len(delta) != 2 {
		t.Fatalf("上列的规则走样：%v", delta)
	}
}

// Covers: 规则写口在无事务上下文一律被 RequireExecutor 拒绝（ErrTransactionRequired）——判据同包
// transaction_guard_test.go 那族，随适配器逐个成立。
func TestDutyPaymentGateRuleWritesRefuseToRunOutsideATransaction(t *testing.T) {
	registry, _, _ := newGateRegistry(t)
	tenant, scope, boundary := synGateKey(t)

	if _, err := registry.RegisterDutyPaymentGateRule(t.Context(), tenant, scope, domain.OutboundRelease, boundary,
		synAcceptRule(t)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记规则应返回 ErrTransactionRequired，实得：%v", err)
	}
}
