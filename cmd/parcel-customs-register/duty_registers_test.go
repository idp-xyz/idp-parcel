package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 本文件证凭证 / 税费付款协作事项 / 税费付款核对三册子命令（票 sa-cc/07 步一）接对了
// 各自的编排：译装出的领域值原样到册，用例的答案代数原样到退出码——协作与核对那族的
// 格与案件配置族的格不是一张表，退出码只按恢复动作归格，不把一族的格翻成另一族的。

// fixedClock 让协作事项与核对的形成时间可断言：那两个时刻不是输入，取时钟。
type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

var registerClockNow = time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC)

// fakeCredentialBook 同一本替身册充当凭证写口与读口两半。
type fakeCredentialBook struct {
	byKey       map[string]domain.RegulatoryCredential
	registerErr error
}

func credentialKey(tenant domain.TenantID, id domain.CredentialID) string {
	return tenant.String() + "/" + id.String()
}

func (book *fakeCredentialBook) RegisterCredential(
	_ context.Context,
	tenant domain.TenantID,
	credential domain.RegulatoryCredential,
) (ports.CaseConfigurationSaveOutcome, error) {
	if book.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, book.registerErr
	}
	key := credentialKey(tenant, credential.ID())
	if _, exists := book.byKey[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.byKey[key] = credential
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeCredentialBook) LoadCredential(
	_ context.Context,
	tenant domain.TenantID,
	id domain.CredentialID,
) (domain.RegulatoryCredential, bool, error) {
	credential, found := book.byKey[credentialKey(tenant, id)]
	return credential, found, nil
}

// fakeDutyBook 一本替身充当协作事项、资金事实引用、付款核对三口与付款人规则读口——与真库适配器
// DutyPaymentReconciliation 几口一体同形；各张表的键互不相干。核对形成那一格向 SA 交
// 信封的交接口（票 sa-cc/05）也挂在这本上，记下每一份意图，本口证的是子命令把它接通了。
type fakeDutyBook struct {
	collaborations   map[string]domain.DutyPaymentCollaboration
	funds            map[string]ports.ExternalFundsFactRegistration
	verifications    map[string]ports.DutyVerificationRecord
	payerRules       map[string]domain.PayerRequirement
	handoffs         []ports.DutyPaymentVerificationHandoffIntent
	collaborationErr error
	verificationErr  error
}

func newFakeDutyBook() *fakeDutyBook {
	return &fakeDutyBook{
		collaborations: map[string]domain.DutyPaymentCollaboration{},
		funds:          map[string]ports.ExternalFundsFactRegistration{},
		verifications:  map[string]ports.DutyVerificationRecord{},
		payerRules:     map[string]domain.PayerRequirement{},
	}
}

func (book *fakeDutyBook) LoadPayerRequirement(
	_ context.Context,
	tenant domain.TenantID,
	procedure domain.CustomsProcedureReference,
) (domain.PayerRequirement, bool, error) {
	requirement, found := book.payerRules[tenant.String()+"/"+procedure.String()]
	return requirement, found, nil
}

func collaborationKey(tenant domain.TenantID, scope domain.DecisionScopeReference, duty domain.AssessedDutyReference) string {
	return tenant.String() + "/" + scope.String() + "/" + duty.String()
}

func (book *fakeDutyBook) FindCollaboration(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	duty domain.AssessedDutyReference,
) (domain.DutyPaymentCollaboration, bool, error) {
	collaboration, found := book.collaborations[collaborationKey(tenant, scope, duty)]
	return collaboration, found, nil
}

func (book *fakeDutyBook) SaveCollaboration(
	_ context.Context,
	tenant domain.TenantID,
	collaboration domain.DutyPaymentCollaboration,
) (ports.CaseConfigurationSaveOutcome, error) {
	if book.collaborationErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, book.collaborationErr
	}
	duty, _ := collaboration.Duty()
	key := collaborationKey(tenant, collaboration.Scope(), duty)
	if _, exists := book.collaborations[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.collaborations[key] = collaboration
	return ports.CaseConfigurationRegistered, nil
}

// 资金事实在本口没有子命令，替身册按引用存一版就够核对读前置；版本维（票 sa-cc/13）在这里只需满足端口。
func (book *fakeDutyBook) RegisterFundsFact(
	_ context.Context,
	tenant domain.TenantID,
	registration ports.ExternalFundsFactRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	key := tenant.String() + "/" + registration.Fact.String()
	if _, exists := book.funds[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.funds[key] = registration
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeDutyBook) LoadFundsFact(
	_ context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
) (ports.ExternalFundsFactRegistration, bool, error) {
	registration, found := book.funds[tenant.String()+"/"+fact.String()]
	return registration, found, nil
}

func (book *fakeDutyBook) ListFundsFactVersions(
	_ context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
) ([]ports.ExternalFundsFactRegistration, error) {
	registration, found := book.funds[tenant.String()+"/"+fact.String()]
	if !found {
		return nil, nil
	}
	return []ports.ExternalFundsFactRegistration{registration}, nil
}

func verificationKey(key ports.DutyVerificationKey) string {
	return strings.Join([]string{
		key.TenantID.String(), key.Duty.String(), key.Funds.String(), key.Scope.String(), key.Digest,
	}, "/")
}

func (book *fakeDutyBook) FindVerification(
	_ context.Context,
	key ports.DutyVerificationKey,
) (ports.DutyVerificationRecord, bool, error) {
	record, found := book.verifications[verificationKey(key)]
	return record, found, nil
}

func (book *fakeDutyBook) SaveVerification(
	_ context.Context,
	record ports.DutyVerificationRecord,
) (ports.CaseConfigurationSaveOutcome, error) {
	if book.verificationErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, book.verificationErr
	}
	key := verificationKey(record.Key)
	if _, exists := book.verifications[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	book.verifications[key] = record
	return ports.CaseConfigurationRegistered, nil
}

func (book *fakeDutyBook) HandOffDutyPaymentVerification(
	_ context.Context,
	intent ports.DutyPaymentVerificationHandoffIntent,
) error {
	book.handoffs = append(book.handoffs, intent)
	return nil
}

type dutyFixture struct {
	registrar   registrar
	credentials *fakeCredentialBook
	duties      *fakeDutyBook
}

func newDutyFixture(t *testing.T) *dutyFixture {
	t.Helper()
	credentials := &fakeCredentialBook{byKey: map[string]domain.RegulatoryCredential{}}
	duties := newFakeDutyBook()
	base := newExecuteFixture().registrar
	base.credentials = application.NewRegisterCredentialHandler(
		application.RegisterCredentialDeps{Registry: credentials, View: credentials})
	reconciliation, err := application.NewDutyPaymentReconciliationHandler(
		application.DutyPaymentReconciliationDeps{
			Collaborations: duties,
			Funds:          duties,
			Verifications:  duties,
			PayerRules:     duties,
			Handoff:        duties,
			Clock:          fixedClock{now: registerClockNow},
		})
	if err != nil {
		t.Fatalf("装配税费付款协作与核对编排替身：%v", err)
	}
	base.dutyReconciliation = reconciliation
	return &dutyFixture{registrar: base, credentials: credentials, duties: duties}
}

// seedFundsFact 直接把一条资金事实引用放进替身册：本 CLI 没有它的入口（ADR-0137 决定四，
// 资金事实进 CC 只经 settlement-accounting 的采用信封），核对的前置只能这样铺。
func seedFundsFact(t *testing.T, book *fakeDutyBook, tenant, fact string) {
	t.Helper()
	reference, err := domain.NewExternalFundsFactReference(fact)
	if err != nil {
		t.Fatalf("构造资金事实引用：%v", err)
	}
	payer, err := domain.ProvidedFundsPayer("SYN-PAYER-01")
	if err != nil {
		t.Fatalf("构造付款人：%v", err)
	}
	version, err := domain.NewFundsFactVersion(fact + "/v1")
	if err != nil {
		t.Fatalf("构造版本：%v", err)
	}
	book.funds[tenant+"/"+fact] = ports.ExternalFundsFactRegistration{
		Fact: reference, Version: version, Source: "SYN-BANK-01", Payer: payer, Currency: "XTS",
		AmountMinor: 12500, OccurredAt: registerClockNow.Add(-time.Hour),
	}
}

// seedFundsFactWithoutPayer 铺一条来源显式未提供付款人的事实——付款人三停格（票 sa-cc/12 裁决 2）唯一会让
// 答案分岔的形。
func seedFundsFactWithoutPayer(t *testing.T, book *fakeDutyBook, tenant, fact string) {
	t.Helper()
	seedFundsFact(t, book, tenant, fact)
	registration := book.funds[tenant+"/"+fact]
	registration.Payer = domain.FundsPayerNotProvided()
	book.funds[tenant+"/"+fact] = registration
}

// seedPayerRule 直接把「这个程序要不要付款人」放进替身册：本 CLI 今天没有登这一格的子命令（登记面在
// RegisterCaseConfigurationHandler，同 0019 那一格），核对读的这一维只能这样铺。
func seedPayerRule(book *fakeDutyBook, tenant, procedure string, requirement domain.PayerRequirement) {
	book.payerRules[tenant+"/"+procedure] = requirement
}

func credentialInput(validTo string, uses string) []byte {
	usesField := ""
	if uses != "" {
		usesField = `, "uses": ` + uses
	}
	return []byte(`{
		"tenantId": "SYN-T1",
		"credentialId": "SYN-CRED-01",
		"issuerRef": "SYN-AUTHORITY-01",
		"holderRef": "SYN-HOLDER-01",
		"procedureRef": "SYN-PROC-01",
		"validFrom": "2026-09-01T00:00:00Z",
		"validTo": "` + validTo + `"` + usesField + `
	}`)
}

// TestExecuteRegulatoryCredentialLandsReplaysAndConflicts 证凭证子命令三格：登记 0 且
// 七件原样到册（uses 缺席即「来源未提供」，读口第二个返回值为 false）、同一份重放 0
// 且含 EXISTING、换有效期 2 且册面纹丝不动——凭证是不可变版本，换期限是另一张凭证。
func TestExecuteRegulatoryCredentialLandsReplaysAndConflicts(t *testing.T) {
	fixture := newDutyFixture(t)
	ctx := context.Background()

	message, code := execute(ctx, commandRegulatoryCredential, credentialInput("2027-09-01T00:00:00Z", ""), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "REGISTERED") {
		t.Fatalf("首登 = %d（%s），要 %d 且含 REGISTERED", code, message, exitRegistered)
	}
	credential, found := fixture.credentials.byKey["SYN-T1/SYN-CRED-01"]
	if !found {
		t.Fatalf("凭证没落册")
	}
	if credential.Issuer().String() != "SYN-AUTHORITY-01" || credential.Holder().String() != "SYN-HOLDER-01" ||
		credential.Procedure().String() != "SYN-PROC-01" {
		t.Fatalf("册上机构 / 持有人 / 程序不是输入原值：%s / %s / %s",
			credential.Issuer(), credential.Holder(), credential.Procedure())
	}
	if !credential.ValidFrom().Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) ||
		!credential.ValidTo().Equal(time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("册上有效期不是输入原值：%s – %s", credential.ValidFrom(), credential.ValidTo())
	}
	if _, provided := credential.Uses(); provided {
		t.Fatalf("uses 缺席要落成「来源未提供」，不得读成任何额度")
	}

	message, code = execute(ctx, commandRegulatoryCredential, credentialInput("2027-09-01T00:00:00Z", ""), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "EXISTING") {
		t.Fatalf("重放 = %d（%s），要 0 且含 EXISTING", code, message)
	}
	message, code = execute(ctx, commandRegulatoryCredential, credentialInput("2028-09-01T00:00:00Z", ""), fixture.registrar)
	if code != exitConflict || !strings.Contains(message, "CONTENT_CONFLICT") {
		t.Fatalf("换有效期 = %d（%s），要 %d 且含 CONTENT_CONFLICT", code, message, exitConflict)
	}
	if !fixture.credentials.byKey["SYN-T1/SYN-CRED-01"].ValidTo().Equal(time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("冲突把册面顶掉了——绝不覆盖被破")
	}
}

// TestExecuteRegulatoryCredentialUsesLandsVerbatim 证给了次数额度就原样到册且读口报
// 「来源提供了」——0 与缺席都是未提供，正数才是额度。
func TestExecuteRegulatoryCredentialUsesLandsVerbatim(t *testing.T) {
	fixture := newDutyFixture(t)

	if _, code := execute(context.Background(), commandRegulatoryCredential,
		credentialInput("2027-09-01T00:00:00Z", "3"), fixture.registrar); code != exitRegistered {
		t.Fatalf("带额度首登退出码 = %d", code)
	}
	uses, provided := fixture.credentials.byKey["SYN-T1/SYN-CRED-01"].Uses()
	if !provided || uses != 3 {
		t.Fatalf("册上额度 = (%d, %t)，要 (3, true)", uses, provided)
	}
}

// TestExecuteRegulatoryCredentialAcceptanceGateIsUsage 证受理门拒绝答 1：有效期倒置
// 由领域构造拒、用例折成 NOT_ACCEPTED，本口不在译装处替领域重判期限——改请求，不是重试。
func TestExecuteRegulatoryCredentialAcceptanceGateIsUsage(t *testing.T) {
	fixture := newDutyFixture(t)

	message, code := execute(context.Background(), commandRegulatoryCredential,
		credentialInput("2026-08-01T00:00:00Z", ""), fixture.registrar)
	if code != exitUsage || !strings.Contains(message, "NOT_ACCEPTED") {
		t.Fatalf("有效期倒置 = %d（%s），要 %d 且含 NOT_ACCEPTED", code, message, exitUsage)
	}
	if len(fixture.credentials.byKey) != 0 {
		t.Fatalf("被拒的输入不得落册")
	}
}

// TestExecuteRegulatoryCredentialWriterFailureIsUndecided 证依赖故障答 3。
func TestExecuteRegulatoryCredentialWriterFailureIsUndecided(t *testing.T) {
	fixture := newDutyFixture(t)
	fixture.credentials.registerErr = errors.New("credential store unavailable")

	message, code := execute(context.Background(), commandRegulatoryCredential,
		credentialInput("2027-09-01T00:00:00Z", ""), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "UNDECIDED") {
		t.Fatalf("写口故障 = %d（%s），要 %d 且含 UNDECIDED", code, message, exitUndecided)
	}
}

func collaborationInput(kind, dutyRef, noPayBasis, target string) []byte {
	return []byte(`{
		"tenantId": "SYN-T1",
		"kind": "` + kind + `",
		"dutyRef": "` + dutyRef + `",
		"noPayBasis": "` + noPayBasis + `",
		"scopeRef": "SYN-UNIT-01",
		"obligorRef": "SYN-OBLIGOR-01",
		"requirementRef": "SYN-ASSESSMENT-01",
		"targetRef": "` + target + `"
	}`)
}

// TestExecuteDutyCollaborationLandsReplaysAndConflicts 证协作事项子命令三格：核定税费格
// 形成 0 且七件原样到册（形成时间取时钟，不是输入）、重放 0 含 EXISTING、换责任交接目标
// 2 且册面不动。
func TestExecuteDutyCollaborationLandsReplaysAndConflicts(t *testing.T) {
	fixture := newDutyFixture(t)
	ctx := context.Background()

	message, code := execute(ctx, commandDutyCollaboration,
		collaborationInput("ASSESSED_DUTY", "SYN-DUTY-01/v1", "", "SYN-DUTY-DESK"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "COLLABORATION_FORMED") {
		t.Fatalf("形成 = %d（%s），要 %d 且含 COLLABORATION_FORMED", code, message, exitRegistered)
	}
	collaboration, found := fixture.duties.collaborations["SYN-T1/SYN-UNIT-01/SYN-DUTY-01/v1"]
	if !found {
		t.Fatalf("协作事项没落册")
	}
	duty, assessed := collaboration.Duty()
	if collaboration.Kind() != domain.ObligationFromAssessedDuty || !assessed || duty.String() != "SYN-DUTY-01/v1" ||
		collaboration.Obligor().String() != "SYN-OBLIGOR-01" || collaboration.Requirement().String() != "SYN-ASSESSMENT-01" ||
		collaboration.Target().String() != "SYN-DUTY-DESK" {
		t.Fatalf("册上协作事项不是输入原值：%+v", collaboration)
	}
	if !collaboration.FormedAt().Equal(registerClockNow) {
		t.Fatalf("形成时间 = %s，要取时钟 %s", collaboration.FormedAt(), registerClockNow)
	}

	message, code = execute(ctx, commandDutyCollaboration,
		collaborationInput("ASSESSED_DUTY", "SYN-DUTY-01/v1", "", "SYN-DUTY-DESK"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "EXISTING_COLLABORATION") {
		t.Fatalf("重放 = %d（%s），要 0 且含 EXISTING_COLLABORATION", code, message)
	}
	message, code = execute(ctx, commandDutyCollaboration,
		collaborationInput("ASSESSED_DUTY", "SYN-DUTY-01/v1", "", "SYN-OTHER-DESK"), fixture.registrar)
	if code != exitConflict || !strings.Contains(message, "COLLABORATION_CONTENT_CONFLICT") {
		t.Fatalf("换目标 = %d（%s），要 %d 且含 COLLABORATION_CONTENT_CONFLICT", code, message, exitConflict)
	}
	if fixture.duties.collaborations["SYN-T1/SYN-UNIT-01/SYN-DUTY-01/v1"].Target().String() != "SYN-DUTY-DESK" {
		t.Fatalf("冲突把册面顶掉了——绝不覆盖被破")
	}
}

// TestExecuteDutyCollaborationNotRequiredLandsWithBlankDutyKey 证明确无需付款格：税费引用
// 为空、依据落册；键上的税费引用是空串（0016 自注），与核定格是两行。
func TestExecuteDutyCollaborationNotRequiredLandsWithBlankDutyKey(t *testing.T) {
	fixture := newDutyFixture(t)

	message, code := execute(context.Background(), commandDutyCollaboration,
		collaborationInput("EXPLICITLY_NOT_REQUIRED", "", "SYN-PROGRAM-01: no duty on this scope", "SYN-DUTY-DESK"),
		fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "COLLABORATION_FORMED") {
		t.Fatalf("无需付款格形成 = %d（%s），要 0 且含 COLLABORATION_FORMED", code, message)
	}
	collaboration, found := fixture.duties.collaborations["SYN-T1/SYN-UNIT-01/"]
	if !found {
		t.Fatalf("无需付款格没按空税费引用落键")
	}
	basis, notRequired := collaboration.NoPayBasis()
	if !notRequired || basis != "SYN-PROGRAM-01: no duty on this scope" {
		t.Fatalf("册上无需付款依据 = (%q, %t)，要输入原值", basis, notRequired)
	}
}

// TestExecuteDutyCollaborationAcceptanceAndBasisAbsent 证两格各归各的退出码：核定格夹带
// 无需付款依据是矛盾输入，领域拒、用例折 NOT_ACCEPTED → 1；义务依据两格都不给是「税费结果
// 还没到」，用例答业务未决 DUTY_OBLIGATION_BASIS_ABSENT → 3——本口不在译装处把它改判成用法
// 错误，那一格的续办是等 UC-CC-006 的核定税费或真实程序的无需付款依据。
func TestExecuteDutyCollaborationAcceptanceAndBasisAbsent(t *testing.T) {
	fixture := newDutyFixture(t)
	ctx := context.Background()

	message, code := execute(ctx, commandDutyCollaboration,
		collaborationInput("ASSESSED_DUTY", "SYN-DUTY-01/v1", "SYN-PROGRAM-01: contradiction", "SYN-DUTY-DESK"),
		fixture.registrar)
	if code != exitUsage || !strings.Contains(message, "NOT_ACCEPTED") {
		t.Fatalf("矛盾输入 = %d（%s），要 %d 且含 NOT_ACCEPTED", code, message, exitUsage)
	}

	message, code = execute(ctx, commandDutyCollaboration,
		collaborationInput("", "", "", "SYN-DUTY-DESK"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "UNDECIDED") ||
		!strings.Contains(message, "DUTY_OBLIGATION_BASIS_ABSENT") {
		t.Fatalf("义务依据缺席 = %d（%s），要 %d 且含 UNDECIDED 与 DUTY_OBLIGATION_BASIS_ABSENT", code, message, exitUndecided)
	}
	if len(fixture.duties.collaborations) != 0 {
		t.Fatalf("两格都不得落册")
	}
}

// TestExecuteDutyCollaborationStoreFailureIsUndecided 证依赖故障答 3 且指名等谁。
func TestExecuteDutyCollaborationStoreFailureIsUndecided(t *testing.T) {
	fixture := newDutyFixture(t)
	fixture.duties.collaborationErr = errors.New("collaboration store unavailable")

	message, code := execute(context.Background(), commandDutyCollaboration,
		collaborationInput("ASSESSED_DUTY", "SYN-DUTY-01/v1", "", "SYN-DUTY-DESK"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "UNDECIDED") ||
		!strings.Contains(message, "COLLABORATION_STORE_UNAVAILABLE") {
		t.Fatalf("写口故障 = %d（%s），要 %d 且含 UNDECIDED 与 COLLABORATION_STORE_UNAVAILABLE", code, message, exitUndecided)
	}
}

func verificationInput(coverage, basis string) []byte {
	return []byte(`{
		"tenantId": "SYN-T1",
		"dutyRef": "SYN-DUTY-01/v1",
		"fundsRef": "SYN-FUNDS-01",
		"scopeRef": "SYN-UNIT-01",
		"procedureRef": "SYN-PROC-01",
		"coverage": "` + coverage + `",
		"delta": "SHORT",
		"validity": "PENDING",
		"basis": "` + basis + `"
	}`)
}

// seedCollaboration 经本口自己的子命令铺协作事项前置——核对的两道前置里这一道有入口。
func seedCollaboration(t *testing.T, fixture *dutyFixture) {
	t.Helper()
	if _, code := execute(context.Background(), commandDutyCollaboration,
		collaborationInput("ASSESSED_DUTY", "SYN-DUTY-01/v1", "", "SYN-DUTY-DESK"), fixture.registrar); code != exitRegistered {
		t.Fatalf("铺协作事项退出码 = %d", code)
	}
}

// TestExecuteDutyPaymentVerificationLandsReplaysAndAppendsVersions 证核对子命令：两道前置
// 齐备时形成 0 且三轴、依据、三维键原样到册（核对时间取时钟）；同一份重放 0 含 EXISTING；
// 换一轴不是冲突而是新版本追加——迟到事实按新版本进、不按到达顺序覆盖（UC-CC-009），两版
// 并存。这族没有「内容冲突」格，本口也不替它造一个。结算交接随形成走（票 sa-cc/05）：每形成
// 一版交一封、重放不交，本口只证接通了，信封内容归适配器自己的用例。
func TestExecuteDutyPaymentVerificationLandsReplaysAndAppendsVersions(t *testing.T) {
	fixture := newDutyFixture(t)
	ctx := context.Background()
	seedFundsFact(t, fixture.duties, "SYN-T1", "SYN-FUNDS-01")
	seedCollaboration(t, fixture)
	seedPayerRule(fixture.duties, "SYN-T1", "SYN-PROC-01", domain.PayerRequired)

	message, code := execute(ctx, commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01: remittance quotes assessment"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "DUTY_VERIFICATION_FORMED") {
		t.Fatalf("形成 = %d（%s），要 %d 且含 DUTY_VERIFICATION_FORMED", code, message, exitRegistered)
	}
	if len(fixture.duties.verifications) != 1 {
		t.Fatalf("核对册行数 = %d，要 1", len(fixture.duties.verifications))
	}
	for _, record := range fixture.duties.verifications {
		if record.Key.TenantID.String() != "SYN-T1" || record.Key.Duty.String() != "SYN-DUTY-01/v1" ||
			record.Key.Funds.String() != "SYN-FUNDS-01" || record.Key.Scope.String() != "SYN-UNIT-01" {
			t.Fatalf("三维键不是输入原值：%+v", record.Key)
		}
		if record.Verification.Coverage() != domain.CoveragePartial || record.Verification.Delta() != domain.DeltaShort ||
			record.Verification.Validity() != domain.FundsFactPending {
			t.Fatalf("三轴不是输入原值：%s / %s / %s",
				record.Verification.Coverage(), record.Verification.Delta(), record.Verification.Validity())
		}
		if record.Basis != "SYN-RULE-01: remittance quotes assessment" {
			t.Fatalf("关联依据 = %q，要输入原值", record.Basis)
		}
		if !record.Verification.VerifiedAt().Equal(registerClockNow) {
			t.Fatalf("核对时间 = %s，要取时钟 %s", record.Verification.VerifiedAt(), registerClockNow)
		}
		if len(fixture.duties.handoffs) != 1 || fixture.duties.handoffs[0].Key != record.Key {
			t.Fatalf("形成后该向 SA 交出恰好一封、认领刚落册那一版：%+v", fixture.duties.handoffs)
		}
	}

	message, code = execute(ctx, commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01: remittance quotes assessment"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "EXISTING_DUTY_VERIFICATION") {
		t.Fatalf("重放 = %d（%s），要 0 且含 EXISTING_DUTY_VERIFICATION", code, message)
	}
	if len(fixture.duties.handoffs) != 1 {
		t.Fatalf("重放后意图数 = %d，要仍是 1——`已存在`不重发", len(fixture.duties.handoffs))
	}
	message, code = execute(ctx, commandDutyPaymentVerification,
		verificationInput("COVERED", "SYN-RULE-01: remittance quotes assessment"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "DUTY_VERIFICATION_FORMED") {
		t.Fatalf("换一轴 = %d（%s），要 0 且含 DUTY_VERIFICATION_FORMED（新版本追加）", code, message)
	}
	if len(fixture.duties.verifications) != 2 {
		t.Fatalf("核对册行数 = %d，要两版并存", len(fixture.duties.verifications))
	}
	if len(fixture.duties.handoffs) != 2 {
		t.Fatalf("新版本后意图数 = %d，要每版各一封", len(fixture.duties.handoffs))
	}
}

// TestExecuteDutyPaymentVerificationPreconditionsKeepTheirGrids 证核对的三个「没形成」格各
// 归各的退出码：无关联依据 → 待关联 1（补依据再登，重跑同一份没有意义——金额相等不能单独作为
// 关联）；资金事实未接收 / 协作事项未形成 → 3（前置未齐，等它落册后重跑同一命令续办；两格
// 的续办动作与依赖故障同款，答案原词各自可见）。三格都不落册。
func TestExecuteDutyPaymentVerificationPreconditionsKeepTheirGrids(t *testing.T) {
	fixture := newDutyFixture(t)
	ctx := context.Background()

	message, code := execute(ctx, commandDutyPaymentVerification, verificationInput("PARTIAL", ""), fixture.registrar)
	if code != exitUsage || !strings.Contains(message, "FUNDS_FACT_PENDING_ASSOCIATION") {
		t.Fatalf("无依据 = %d（%s），要 %d 且含 FUNDS_FACT_PENDING_ASSOCIATION", code, message, exitUsage)
	}

	message, code = execute(ctx, commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "FUNDS_FACT_NOT_RECEIVED") {
		t.Fatalf("资金事实未接收 = %d（%s），要 %d 且含 FUNDS_FACT_NOT_RECEIVED", code, message, exitUndecided)
	}

	seedFundsFact(t, fixture.duties, "SYN-T1", "SYN-FUNDS-01")
	message, code = execute(ctx, commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "COLLABORATION_NOT_FORMED") {
		t.Fatalf("协作事项未形成 = %d（%s），要 %d 且含 COLLABORATION_NOT_FORMED", code, message, exitUndecided)
	}
	if len(fixture.duties.verifications) != 0 {
		t.Fatalf("三格都不得落册")
	}
}

// TestExecuteDutyPaymentVerificationStoreFailureIsUndecided 证依赖故障答 3 且指名等谁。
func TestExecuteDutyPaymentVerificationStoreFailureIsUndecided(t *testing.T) {
	fixture := newDutyFixture(t)
	seedFundsFact(t, fixture.duties, "SYN-T1", "SYN-FUNDS-01")
	seedCollaboration(t, fixture)
	seedPayerRule(fixture.duties, "SYN-T1", "SYN-PROC-01", domain.PayerRequired)
	fixture.duties.verificationErr = errors.New("verification store unavailable")

	message, code := execute(context.Background(), commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "UNDECIDED") ||
		!strings.Contains(message, "DUTY_VERIFICATION_STORE_UNAVAILABLE") {
		t.Fatalf("写口故障 = %d（%s），要 %d 且含 UNDECIDED 与 DUTY_VERIFICATION_STORE_UNAVAILABLE", code, message, exitUndecided)
	}
}

// TestExecuteDutyPaymentVerificationPayerGridsKeepTheirExitCodes 证付款人三停格（票 sa-cc/12 裁决 2）
// 经子命令各归各格：程序没登要不要 → 3 且点名 PAYER_REQUIREMENT_NOT_CONFIGURED（等登记方补规则）；
// 程序要求而来源未提供 → 3 且点名 PAYER_REQUIRED_NOT_PROVIDED（等来源补事实）；程序不要求 → 0 照常形成，
// 「未提供」原样带着。两格未决同一退出码、原词分得开在等谁——与义务依据缺席那格同款。
func TestExecuteDutyPaymentVerificationPayerGridsKeepTheirExitCodes(t *testing.T) {
	fixture := newDutyFixture(t)
	ctx := context.Background()
	seedFundsFactWithoutPayer(t, fixture.duties, "SYN-T1", "SYN-FUNDS-01")
	seedCollaboration(t, fixture)

	message, code := execute(ctx, commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "UNDECIDED") ||
		!strings.Contains(message, "PAYER_REQUIREMENT_NOT_CONFIGURED") {
		t.Fatalf("规则未配置 = %d（%s），要 %d 且含 UNDECIDED 与 PAYER_REQUIREMENT_NOT_CONFIGURED", code, message, exitUndecided)
	}

	seedPayerRule(fixture.duties, "SYN-T1", "SYN-PROC-01", domain.PayerRequired)
	message, code = execute(ctx, commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01"), fixture.registrar)
	if code != exitUndecided || !strings.Contains(message, "UNDECIDED") ||
		!strings.Contains(message, "PAYER_REQUIRED_NOT_PROVIDED") {
		t.Fatalf("要求而未提供 = %d（%s），要 %d 且含 UNDECIDED 与 PAYER_REQUIRED_NOT_PROVIDED", code, message, exitUndecided)
	}
	if len(fixture.duties.verifications) != 0 || len(fixture.duties.handoffs) != 0 {
		t.Fatalf("两格未决都不得落册、不得交信封：%d 行 %d 封", len(fixture.duties.verifications), len(fixture.duties.handoffs))
	}

	seedPayerRule(fixture.duties, "SYN-T1", "SYN-PROC-01", domain.PayerNotRequired)
	message, code = execute(ctx, commandDutyPaymentVerification,
		verificationInput("PARTIAL", "SYN-RULE-01"), fixture.registrar)
	if code != exitRegistered || !strings.Contains(message, "DUTY_VERIFICATION_FORMED") {
		t.Fatalf("不要求付款人 = %d（%s），要 %d 且含 DUTY_VERIFICATION_FORMED", code, message, exitRegistered)
	}
	if len(fixture.duties.verifications) != 1 || len(fixture.duties.handoffs) != 1 {
		t.Fatalf("形成该落一行、交一封：%d 行 %d 封", len(fixture.duties.verifications), len(fixture.duties.handoffs))
	}
	if registration := fixture.duties.funds["SYN-T1/SYN-FUNDS-01"]; registration.Payer.Provided() || !registration.Payer.Valid() {
		t.Fatalf("核对不得替事实补付款人：%#v", registration.Payer)
	}
}

// TestDutyReconciliationAnswerCoversEveryOutcome 证退出码翻译逐格对着 `application` 的
// 协作 / 核对族枚举表成立、未知格折未决。资金事实那族格本口没有命令能交回，仍在表上：一族一张表，同一格
// 不因来自哪条命令而换退出码。
func TestDutyReconciliationAnswerCoversEveryOutcome(t *testing.T) {
	cases := []struct {
		outcome application.DutyReconciliationOutcome
		code    int
	}{
		{application.CollaborationFormed, exitRegistered},
		{application.CollaborationExisting, exitRegistered},
		{application.CollaborationContentConflict, exitConflict},
		{application.FundsFactReceived, exitRegistered},
		{application.FundsFactExisting, exitRegistered},
		{application.FundsFactContentConflict, exitConflict},
		{application.FundsFactPendingAssociation, exitUsage},
		{application.FundsFactNotReceived, exitUndecided},
		{application.CollaborationNotFormed, exitUndecided},
		{application.DutyVerificationFormed, exitRegistered},
		{application.DutyVerificationExisting, exitRegistered},
		{application.DutyReconciliationNotAccepted, exitUsage},
		{application.DutyReconciliationUndecided, exitUndecided},
		{application.DutyReconciliationOutcomeInvalid, exitUndecided},
	}
	for _, spec := range cases {
		_, code := dutyReconciliationAnswer(commandDutyCollaboration, spec.outcome, application.DutyReconciliationReasonNone)
		if code != spec.code {
			t.Fatalf("%s → %d，要 %d", spec.outcome, code, spec.code)
		}
	}
}

// TestDutyReconciliationAnswerNamesTheUndecidedReason 证未决答复带上编排指名的「等谁」：
// 业务未决各格与各依赖故障的续办动作互不相同，折成一个 UNDECIDED 就得让操作员去猜。
func TestDutyReconciliationAnswerNamesTheUndecidedReason(t *testing.T) {
	message, code := dutyReconciliationAnswer(commandDutyCollaboration,
		application.DutyReconciliationUndecided, application.DutyObligationBasisAbsent)
	if code != exitUndecided || !strings.Contains(message, "DUTY_OBLIGATION_BASIS_ABSENT") {
		t.Fatalf("未决答复 = %d（%s），要 %d 且指名 DUTY_OBLIGATION_BASIS_ABSENT", code, message, exitUndecided)
	}
}
