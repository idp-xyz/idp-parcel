package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件证登记口的编排：命令路由、事务包裹与退出码翻译。冲突判定的派生逻辑在应用
// 层已证，这里接真 handler 配册面替身，证的是本口把它们接对——译装出的领域值原样
// 到册（保真），用例答案原样到退出码（不增不减）。

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

type failingTransactor struct{ err error }

func (transactor failingTransactor) WithinTransaction(context.Context, bentoapp.TxFunc) error {
	return transactor.err
}

// fakeReadinessBook 同一本替身册充当写口与读口两半：用例的冲突判定正要求两半看
// 同一份册面。
type fakeReadinessBook struct {
	byKey       map[string]domain.ReadinessJudgment
	registerErr error
}

func readinessKey(tenant domain.TenantID, unit domain.DeclarationUnitID) string {
	return tenant.String() + "/" + unit.String()
}

func (book *fakeReadinessBook) RegisterReadiness(
	_ context.Context,
	tenant domain.TenantID,
	judgment domain.ReadinessJudgment,
) (ports.CaseConfigurationSaveOutcome, error) {
	if book.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, book.registerErr
	}
	key := readinessKey(tenant, judgment.Unit())
	if _, exists := book.byKey[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.byKey[key] = judgment
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeReadinessBook) RevokeReadiness(
	_ context.Context,
	tenant domain.TenantID,
	judgment domain.ReadinessJudgment,
) error {
	book.byKey[readinessKey(tenant, judgment.Unit())] = judgment
	return nil
}

func (book *fakeReadinessBook) LoadReadiness(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.ReadinessJudgment, bool, error) {
	judgment, found := book.byKey[readinessKey(tenant, unit)]
	return judgment, found, nil
}

type fakeAuthorityBook struct {
	byKey map[string]domain.SubmissionAuthorization
}

func (book *fakeAuthorityBook) GrantSubmissionAuthority(
	_ context.Context,
	tenant domain.TenantID,
	authorization domain.SubmissionAuthorization,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := tenant.String() + "/" + authorization.Unit().String()
	if _, exists := book.byKey[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.byKey[key] = authorization
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeAuthorityBook) RevokeSubmissionAuthority(
	_ context.Context,
	tenant domain.TenantID,
	authorization domain.SubmissionAuthorization,
) error {
	book.byKey[tenant.String()+"/"+authorization.Unit().String()] = authorization
	return nil
}

func (book *fakeAuthorityBook) LoadSubmissionAuthority(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.SubmissionAuthorization, bool, error) {
	authorization, found := book.byKey[tenant.String()+"/"+unit.String()]
	return authorization, found, nil
}

type fakeRuleBook struct {
	byKey map[string]domain.InterpretationRuleReference
}

func (book *fakeRuleBook) RegisterInterpretationRule(
	_ context.Context,
	tenant domain.TenantID,
	layer domain.ResultLayer,
	rule domain.InterpretationRuleReference,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := tenant.String() + "/" + layer.String()
	if _, exists := book.byKey[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.byKey[key] = rule
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeRuleBook) LoadInterpretationRule(
	_ context.Context,
	tenant domain.TenantID,
	layer domain.ResultLayer,
) (domain.InterpretationRuleReference, bool, error) {
	rule, found := book.byKey[tenant.String()+"/"+layer.String()]
	return rule, found, nil
}

type fakeObligationBook struct {
	catalogs map[string]bool
	items    map[string]ports.ObligationRegistration
}

func obligationCaseKey(tenant domain.TenantID, caseRef domain.CustomsCaseID) string {
	return tenant.String() + "/" + caseRef.String()
}

func (book *fakeObligationBook) RegisterObligationCatalog(
	_ context.Context,
	tenant domain.TenantID,
	caseRef domain.CustomsCaseID,
	_ time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := obligationCaseKey(tenant, caseRef)
	if book.catalogs[key] {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.catalogs[key] = true
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeObligationBook) RegisterObligationItem(
	_ context.Context,
	tenant domain.TenantID,
	caseRef domain.CustomsCaseID,
	registration ports.ObligationRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := obligationCaseKey(tenant, caseRef) + "/" + registration.Item.Obligation
	if _, exists := book.items[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.items[key] = registration
	return ports.CaseConfigurationRegistered, nil
}

// LoadObligationItems 按半开区间盘（applies_from <= cutoff < applies_until），与真
// 读口同一口径——用例按区间起点读回比对，替身口径歪了冲突判定就会歪。
func (book *fakeObligationBook) LoadObligationItems(
	_ context.Context,
	tenant domain.TenantID,
	caseRef domain.CustomsCaseID,
	cutoffAt time.Time,
) ([]domain.ClosureObligationItem, bool, error) {
	if !book.catalogs[obligationCaseKey(tenant, caseRef)] {
		return nil, false, nil
	}
	prefix := obligationCaseKey(tenant, caseRef) + "/"
	var items []domain.ClosureObligationItem
	for key, registration := range book.items {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if cutoffAt.Before(registration.AppliesFrom) {
			continue
		}
		if !registration.AppliesUntil.IsZero() && !registration.AppliesUntil.After(cutoffAt) {
			continue
		}
		items = append(items, registration.Item)
	}
	return items, true, nil
}

type fakeGateBook struct {
	catalogs map[string]bool
	findings map[string]domain.PreconditionFinding
}

func gateKey(
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
) string {
	return tenant.String() + "/" + scope.String() + "/" + action.String() + "/" + boundary.String()
}

func (book *fakeGateBook) RegisterGateCatalog(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
	_ time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := gateKey(tenant, scope, action, boundary)
	if book.catalogs[key] {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.catalogs[key] = true
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeGateBook) RegisterGateFinding(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
	finding domain.PreconditionFinding,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := gateKey(tenant, scope, action, boundary) + "/" + finding.Precondition.String()
	if _, exists := book.findings[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.findings[key] = finding
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeGateBook) LoadPreconditionFindings(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
) ([]domain.PreconditionFinding, bool, error) {
	key := gateKey(tenant, scope, action, boundary)
	if !book.catalogs[key] {
		return nil, false, nil
	}
	var findings []domain.PreconditionFinding
	for stored, finding := range book.findings {
		if strings.HasPrefix(stored, key+"/") {
			findings = append(findings, finding)
		}
	}
	return findings, true, nil
}

// fakeRequirementBook 第六本册子的替身，同样一身两半。
type fakeRequirementBook struct {
	byKey map[string]ports.CaseRequirementJudgment
}

func requirementBookKey(
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
) string {
	return tenant.String() + "/" + jurisdiction.String() + "/" + direction.String() + "/" + procedure.String()
}

func (book *fakeRequirementBook) RegisterCaseRequirementRule(
	_ context.Context,
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
	judgment ports.CaseRequirementJudgment,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := requirementBookKey(tenant, jurisdiction, direction, procedure)
	if _, exists := book.byKey[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.byKey[key] = judgment
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeRequirementBook) JudgeCaseRequirement(
	_ context.Context,
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
) (ports.CaseRequirementJudgment, bool, error) {
	judgment, found := book.byKey[requirementBookKey(tenant, jurisdiction, direction, procedure)]
	return judgment, found, nil
}

type executeFixture struct {
	registrar    registrar
	readiness    *fakeReadinessBook
	authorities  *fakeAuthorityBook
	rules        *fakeRuleBook
	obligations  *fakeObligationBook
	gates        *fakeGateBook
	requirements *fakeRequirementBook
}

func newExecuteFixture() *executeFixture {
	readiness := &fakeReadinessBook{byKey: map[string]domain.ReadinessJudgment{}}
	authorities := &fakeAuthorityBook{byKey: map[string]domain.SubmissionAuthorization{}}
	rules := &fakeRuleBook{byKey: map[string]domain.InterpretationRuleReference{}}
	obligations := &fakeObligationBook{
		catalogs: map[string]bool{},
		items:    map[string]ports.ObligationRegistration{},
	}
	gates := &fakeGateBook{
		catalogs: map[string]bool{},
		findings: map[string]domain.PreconditionFinding{},
	}
	requirements := &fakeRequirementBook{byKey: map[string]ports.CaseRequirementJudgment{}}
	configurations := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness:      readiness,
		ReadinessView:  readiness,
		Authorities:    authorities,
		AuthorityView:  authorities,
		Rules:          rules,
		RuleView:       rules,
		Obligations:    obligations,
		ObligationView: obligations,
		Gates:          gates,
		GateView:       gates,
	})
	requirementHandler := application.NewRegisterCaseRequirementRuleHandler(
		application.RegisterCaseRequirementRuleDeps{Rules: requirements, View: requirements})
	return &executeFixture{
		registrar: registrar{
			configurations: configurations,
			requirements:   requirementHandler,
			transactor:     passthroughTransactor{},
		},
		readiness:    readiness,
		authorities:  authorities,
		rules:        rules,
		obligations:  obligations,
		gates:        gates,
		requirements: requirements,
	}
}

func readinessInput(basis string) []byte {
	return []byte(`{
		"tenantId": "SYN-T1",
		"unitId": "SYN-UNIT-1",
		"basisRef": "` + basis + `",
		"judgedAt": "2026-08-24T01:00:00Z"
	}`)
}

// TestExecuteRegisterReadinessLandsWithFidelity 证绿路径与译装保真：登记答 0，册上
// 那一份的单元、依据与形成时间就是输入里那三个值，一字不多一格不偏。
func TestExecuteRegisterReadinessLandsWithFidelity(t *testing.T) {
	fixture := newExecuteFixture()

	message, code := execute(context.Background(), commandReadinessRegister,
		readinessInput("SYN-BASIS-1"), fixture.registrar)
	if code != exitRegistered {
		t.Fatalf("退出码 = %d（%s），要 %d", code, message, exitRegistered)
	}
	if !strings.Contains(message, "REGISTERED") {
		t.Fatalf("答复 = %q，要含 REGISTERED", message)
	}
	judgment, found := fixture.readiness.byKey["SYN-T1/SYN-UNIT-1"]
	if !found {
		t.Fatalf("就绪判断没落册")
	}
	if judgment.Basis().String() != "SYN-BASIS-1" {
		t.Fatalf("册上依据 = %q，要输入原值", judgment.Basis())
	}
	wantJudgedAt := time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)
	if !judgment.JudgedAt().Equal(wantJudgedAt) {
		t.Fatalf("册上形成时间 = %s，要 %s", judgment.JudgedAt(), wantJudgedAt)
	}
}

// TestExecuteReadinessReplayAndConflictSplit 证冲突判定接对了：同一份重放答已存在
// （0），换依据答内容冲突（2）且册面纹丝不动——绝不覆盖。
func TestExecuteReadinessReplayAndConflictSplit(t *testing.T) {
	fixture := newExecuteFixture()
	ctx := context.Background()

	if _, code := execute(ctx, commandReadinessRegister, readinessInput("SYN-BASIS-1"), fixture.registrar); code != exitRegistered {
		t.Fatalf("首登退出码 = %d", code)
	}
	message, code := execute(ctx, commandReadinessRegister, readinessInput("SYN-BASIS-1"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "EXISTING") {
		t.Fatalf("重放 = %d（%s），要 0 且含 EXISTING", code, message)
	}
	message, code = execute(ctx, commandReadinessRegister, readinessInput("SYN-BASIS-2"), fixture.registrar)
	if code != exitConflict || !strings.Contains(message, "CONTENT_CONFLICT") {
		t.Fatalf("换依据 = %d（%s），要 %d 且含 CONTENT_CONFLICT", code, message, exitConflict)
	}
	if fixture.readiness.byKey["SYN-T1/SYN-UNIT-1"].Basis().String() != "SYN-BASIS-1" {
		t.Fatalf("冲突把册面顶掉了——绝不覆盖被破")
	}
}

// TestExecuteRevocationLifecycle 证撤销三格：撤销落地 0、再撤已撤销 0（意图已达成，
// 册面原因归首撤者）、撤销不在册的单元 1（改请求，不是重试）。
func TestExecuteRevocationLifecycle(t *testing.T) {
	fixture := newExecuteFixture()
	ctx := context.Background()

	if _, code := execute(ctx, commandReadinessRegister, readinessInput("SYN-BASIS-1"), fixture.registrar); code != exitRegistered {
		t.Fatalf("首登退出码 = %d", code)
	}
	revoke := []byte(`{
		"tenantId": "SYN-T1", "unitId": "SYN-UNIT-1",
		"cause": "SYN-RULE-CHANGE", "at": "2026-08-24T02:00:00Z"
	}`)
	message, code := execute(ctx, commandReadinessRevoke, revoke, fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "REVOKED") {
		t.Fatalf("撤销 = %d（%s），要 0 且含 REVOKED", code, message)
	}
	message, code = execute(ctx, commandReadinessRevoke, revoke, fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "ALREADY_REVOKED") {
		t.Fatalf("再撤 = %d（%s），要 0 且含 ALREADY_REVOKED", code, message)
	}
	missing := []byte(`{
		"tenantId": "SYN-T1", "unitId": "SYN-UNIT-NEVER",
		"cause": "SYN-RULE-CHANGE", "at": "2026-08-24T02:00:00Z"
	}`)
	message, code = execute(ctx, commandReadinessRevoke, missing, fixture.registrar)
	if code != exitUsage || !strings.Contains(message, "NOT_REGISTERED") {
		t.Fatalf("撤销无对象 = %d（%s），要 %d 且含 NOT_REGISTERED", code, message, exitUsage)
	}
}

// TestExecuteObligationItemCarriesTheInterval 证义务项译装保真：适用区间两端原样
// 到册，缺席的 appliesUntil 是「尚无终点」而不是某个时刻。
func TestExecuteObligationItemCarriesTheInterval(t *testing.T) {
	fixture := newExecuteFixture()
	ctx := context.Background()

	catalog := []byte(`{
		"tenantId": "SYN-T1", "caseRef": "SYN-CASE-1",
		"registeredAt": "2026-08-24T01:00:00Z"
	}`)
	if message, code := execute(ctx, commandObligationCatalog, catalog, fixture.registrar); code != exitRegistered {
		t.Fatalf("目录登记 = %d（%s）", code, message)
	}
	item := []byte(`{
		"tenantId": "SYN-T1", "caseRef": "SYN-CASE-1",
		"obligation": "SYN-DUTY-SETTLE", "scope": "SYN-SCOPE-1",
		"state": "HANDED_OVER", "basis": "SYN-BASIS-1", "handedTo": "SYN-BROKER-1",
		"appliesFrom": "2026-08-24T01:00:00Z"
	}`)
	if message, code := execute(ctx, commandObligationItem, item, fixture.registrar); code != exitRegistered {
		t.Fatalf("义务项登记 = %d（%s）", code, message)
	}
	registration, found := fixture.obligations.items["SYN-T1/SYN-CASE-1/SYN-DUTY-SETTLE"]
	if !found {
		t.Fatalf("义务项没落册")
	}
	if registration.Item.HandedTo != "SYN-BROKER-1" || registration.Item.State != domain.ObligationHandedOver {
		t.Fatalf("承接内容失真：%+v", registration.Item)
	}
	if !registration.AppliesUntil.IsZero() {
		t.Fatalf("缺席的 appliesUntil 变成了 %s，要零值（尚无终点）", registration.AppliesUntil)
	}
}

// TestExecuteGateFindingConflictIsAGovernanceAnswer 证门禁判断的冲突格：同键同判断
// 重放答已存在（0），同键异判断答内容冲突（2）——改判断走复核，不顶原判断。
func TestExecuteGateFindingConflictIsAGovernanceAnswer(t *testing.T) {
	fixture := newExecuteFixture()
	ctx := context.Background()

	catalog := []byte(`{
		"tenantId": "SYN-T1", "scopeRef": "SYN-SCOPE-1", "action": "OUTBOUND_RELEASE",
		"boundaryRef": "SYN-PROC-EXPORT", "registeredAt": "2026-08-24T01:00:00Z"
	}`)
	if message, code := execute(ctx, commandGateCatalog, catalog, fixture.registrar); code != exitRegistered {
		t.Fatalf("门禁目录登记 = %d（%s）", code, message)
	}
	finding := func(state string) []byte {
		return []byte(`{
			"tenantId": "SYN-T1", "scopeRef": "SYN-SCOPE-1", "action": "OUTBOUND_RELEASE",
			"boundaryRef": "SYN-PROC-EXPORT", "preconditionRef": "SYN-PRE-DUTY-PAID",
			"state": "` + state + `"
		}`)
	}
	if message, code := execute(ctx, commandGateFinding, finding("MET"), fixture.registrar); code != exitRegistered {
		t.Fatalf("判断登记 = %d（%s）", code, message)
	}
	message, code := execute(ctx, commandGateFinding, finding("MET"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "EXISTING") {
		t.Fatalf("判断重放 = %d（%s），要 0 且含 EXISTING", code, message)
	}
	message, code = execute(ctx, commandGateFinding, finding("UNMET"), fixture.registrar)
	if code != exitConflict || !strings.Contains(message, "CONTENT_CONFLICT") {
		t.Fatalf("换判断 = %d（%s），要 %d 且含 CONTENT_CONFLICT", code, message, exitConflict)
	}
}

// TestExecuteCaseRequirementFidelityAndConflict 证第十命令接对了：required=false 原样
// 到册（「不要求」带依据是合法且必须登得出的一格），翻面答内容冲突且册面不动。
func TestExecuteCaseRequirementFidelityAndConflict(t *testing.T) {
	fixture := newExecuteFixture()
	ctx := context.Background()

	requirement := func(required string) []byte {
		return []byte(`{
			"tenantId": "SYN-T1", "jurisdictionRef": "SYN-JURIS-DE", "direction": "EXPORT",
			"procedureRef": "SYN-PROC-EXPORT", "required": ` + required + `,
			"basis": "SYN-CONTRACT-NO-CASE-V1"
		}`)
	}
	message, code := execute(ctx, commandCaseRequirement, requirement("false"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "REGISTERED") {
		t.Fatalf("登记 = %d（%s）", code, message)
	}
	if len(fixture.requirements.byKey) != 1 {
		t.Fatalf("册上行数 = %d", len(fixture.requirements.byKey))
	}
	for _, judgment := range fixture.requirements.byKey {
		if judgment.Required || judgment.Basis != "SYN-CONTRACT-NO-CASE-V1" {
			t.Fatalf("落册内容失真：%+v", judgment)
		}
	}
	message, code = execute(ctx, commandCaseRequirement, requirement("false"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "EXISTING") {
		t.Fatalf("重放 = %d（%s）", code, message)
	}
	message, code = execute(ctx, commandCaseRequirement, requirement("true"), fixture.registrar)
	if code != exitConflict || !strings.Contains(message, "CONTENT_CONFLICT") {
		t.Fatalf("翻面 = %d（%s），要 %d", code, message, exitConflict)
	}
	for _, judgment := range fixture.requirements.byKey {
		if judgment.Required {
			t.Fatalf("冲突顶掉了册面")
		}
	}
}

// TestExecuteWriterFailureIsUndecided 证依赖故障答未决：写口报错被用例折成 UNDECIDED，
// 本口译成 3——登记与否未知，重跑同一命令续办。
func TestExecuteWriterFailureIsUndecided(t *testing.T) {
	fixture := newExecuteFixture()
	fixture.readiness.registerErr = errors.New("register store unavailable")

	message, code := execute(context.Background(), commandReadinessRegister,
		readinessInput("SYN-BASIS-1"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "UNDECIDED") {
		t.Fatalf("写口故障 = %d（%s），要 %d 且含 UNDECIDED", code, message, exitUndecided)
	}
}

// TestExecuteTransactorFailureIsUndecided 证环境事务给不出来也答未决：连开笔都没开，
// 登记与否同样未知。
func TestExecuteTransactorFailureIsUndecided(t *testing.T) {
	fixture := newExecuteFixture()
	fixture.registrar.transactor = failingTransactor{err: errors.New("no transaction")}

	message, code := execute(context.Background(), commandReadinessRegister,
		readinessInput("SYN-BASIS-1"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "未决") {
		t.Fatalf("事务故障 = %d（%s），要 %d 且含 未决", code, message, exitUndecided)
	}
}

// TestExecuteTranslationRejectionIsUsageAndTouchesNothing 证译装拒绝在入库前：坏词表
// 答 1，五本册子一格未动。
func TestExecuteTranslationRejectionIsUsageAndTouchesNothing(t *testing.T) {
	fixture := newExecuteFixture()

	raw := []byte(`{
		"tenantId": "SYN-T1", "resultLayer": "CLEARANCE_DONE", "ruleRef": "SYN-RULE-1"
	}`)
	message, code := execute(context.Background(), commandInterpretationRule, raw, fixture.registrar)
	if code != exitUsage {
		t.Fatalf("词表外取值 = %d（%s），要 %d", code, message, exitUsage)
	}
	if len(fixture.rules.byKey) != 0 {
		t.Fatalf("被拒的输入不得落册")
	}
}

// TestConfigurationAnswerCoversEveryOutcome 证退出码翻译对应用结果的封闭八格逐格
// 成立，未知格折未决。
func TestConfigurationAnswerCoversEveryOutcome(t *testing.T) {
	cases := []struct {
		outcome application.CaseConfigurationOutcome
		code    int
	}{
		{application.ConfigurationRegistered, exitRegistered},
		{application.ConfigurationExisting, exitRegistered},
		{application.ConfigurationRevoked, exitRegistered},
		{application.ConfigurationAlreadyRevoked, exitRegistered},
		{application.ConfigurationNotAccepted, exitUsage},
		{application.ConfigurationNotRegistered, exitUsage},
		{application.ConfigurationContentConflict, exitConflict},
		{application.ConfigurationUndecided, exitUndecided},
		{application.CaseConfigurationOutcomeInvalid, exitUndecided},
	}
	for _, spec := range cases {
		if _, code := configurationAnswer(commandReadinessRegister, spec.outcome); code != spec.code {
			t.Fatalf("%s → %d，要 %d", spec.outcome, code, spec.code)
		}
	}
}

// TestRunRejectsUsageErrors 证进程口的用法边界：缺命令、集合外命令、缺输入文件、
// 缺 DSN 各自以用法错误退出，不碰数据库。
func TestRunRejectsUsageErrors(t *testing.T) {
	ctx := context.Background()
	noEnv := func(string) string { return "" }

	if code := run(ctx, nil, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺命令退出码 = %d", code)
	}
	if code := run(ctx, []string{"case-requirement"}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("集合外命令退出码 = %d（第六本册子另票）", code)
	}
	if code := run(ctx, []string{commandReadinessRegister}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺 -input 退出码 = %d", code)
	}

	input := filepath.Join(t.TempDir(), "readiness.json")
	if err := os.WriteFile(input, readinessInput("SYN-BASIS-1"), 0o600); err != nil {
		t.Fatalf("写输入文件：%v", err)
	}
	if code := run(ctx, []string{commandReadinessRegister, "-input", input}, noEnv, io.Discard, io.Discard); code != exitUsage {
		t.Fatalf("缺 DSN 退出码 = %d（登记口不猜连接串）", code)
	}
}
