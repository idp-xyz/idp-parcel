package application_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// UC-CC-009 步 4–7 的行为面（票 mechanism-executor-triage/07 CC-c）：协作事项两格分立且「缺少
// 税费结果」保持未决；外部资金事实按引用入向登记；核对须先有协作事项与已接收的资金事实、
// 且必带关联依据——无依据即保持待关联，不按金额相等猜。替身照真库代数：同键只答`已登记`。
// 步 8 的结算交接（票 sa-cc/05）也在这里钉：核对形成那一格交一封、其余格不交、交接失败不翻结果。

var dutyBaseAt = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

// dutyStoreDouble 同一本替身充当三口登记册、付款人规则读口加交接口。交接半边记下每一份意图与调用次数——
// 「`已存在`不重发」要能从调用次数上读出来，不能只靠认领键吞重去证。
type dutyStoreDouble struct {
	collaborations map[string]domain.DutyPaymentCollaboration
	// funds 按（租户 | 引用 | 版本）一版本一行，fundsOrder 记接收先后——真库上 received_at 说的那件事，替身用顺序说。
	funds            map[string]ports.ExternalFundsFactRegistration
	fundsOrder       []string
	verifications    map[string]ports.DutyVerificationRecord
	payerRules       map[string]domain.PayerRequirement
	handoffs         []ports.DutyPaymentVerificationHandoffIntent
	collaborationErr error
	fundsErr         error
	verificationErr  error
	payerRuleErr     error
	handoffErr       error
}

// newDutyStore 交回的替身册上已登默认监管程序「要求付款人」那一条规则：核对族其余用例证的是三轴、依据、
// 版本与交接，它们的事实都带付款人，规则那一维对它们只该是「要求且提供 → 放行」的背景；三停格各自的
// 用例按格改写或清掉它。真库上没有这一行——那是实例半边，登记方一条条登进来。
func newDutyStore() *dutyStoreDouble {
	return &dutyStoreDouble{
		collaborations: map[string]domain.DutyPaymentCollaboration{},
		funds:          map[string]ports.ExternalFundsFactRegistration{},
		verifications:  map[string]ports.DutyVerificationRecord{},
		payerRules:     map[string]domain.PayerRequirement{"tenant-a|SYN-PROC-01": domain.PayerRequired},
	}
}

func payerRuleKey(tenant domain.TenantID, procedure domain.CustomsProcedureReference) string {
	return tenant.String() + "|" + procedure.String()
}

func (double *dutyStoreDouble) LoadPayerRequirement(
	_ context.Context,
	tenant domain.TenantID,
	procedure domain.CustomsProcedureReference,
) (domain.PayerRequirement, bool, error) {
	if double.payerRuleErr != nil {
		return domain.PayerRequirementInvalid, false, double.payerRuleErr
	}
	requirement, ok := double.payerRules[payerRuleKey(tenant, procedure)]
	return requirement, ok, nil
}

func collaborationKey(tenant domain.TenantID, scope domain.DecisionScopeReference, duty domain.AssessedDutyReference) string {
	return tenant.String() + "|" + scope.String() + "|" + duty.String()
}

func (double *dutyStoreDouble) FindCollaboration(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	duty domain.AssessedDutyReference,
) (domain.DutyPaymentCollaboration, bool, error) {
	if double.collaborationErr != nil {
		return domain.DutyPaymentCollaboration{}, false, double.collaborationErr
	}
	found, ok := double.collaborations[collaborationKey(tenant, scope, duty)]
	return found, ok, nil
}

func (double *dutyStoreDouble) SaveCollaboration(
	_ context.Context,
	tenant domain.TenantID,
	collaboration domain.DutyPaymentCollaboration,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.collaborationErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.collaborationErr
	}
	duty, _ := collaboration.Duty()
	key := collaborationKey(tenant, collaboration.Scope(), duty)
	if _, exists := double.collaborations[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.collaborations[key] = collaboration
	return ports.CaseConfigurationRegistered, nil
}

func fundsVersionKey(tenant domain.TenantID, fact domain.ExternalFundsFactReference, version domain.FundsFactVersion) string {
	return tenant.String() + "|" + fact.String() + "|" + version.String()
}

func (double *dutyStoreDouble) RegisterFundsFact(
	_ context.Context,
	tenant domain.TenantID,
	registration ports.ExternalFundsFactRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.fundsErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.fundsErr
	}
	key := fundsVersionKey(tenant, registration.Fact, registration.Version)
	if _, exists := double.funds[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.funds[key] = registration
	double.fundsOrder = append(double.fundsOrder, key)
	return ports.CaseConfigurationRegistered, nil
}

func (double *dutyStoreDouble) ListFundsFactVersions(
	_ context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
) ([]ports.ExternalFundsFactRegistration, error) {
	if double.fundsErr != nil {
		return nil, double.fundsErr
	}
	prefix := tenant.String() + "|" + fact.String() + "|"
	var versions []ports.ExternalFundsFactRegistration
	for _, key := range double.fundsOrder {
		if strings.HasPrefix(key, prefix) {
			versions = append(versions, double.funds[key])
		}
	}
	return versions, nil
}

// LoadFundsFactVersion 按（租户、事实、版本）点读一版——真库口径。
func (double *dutyStoreDouble) LoadFundsFactVersion(
	_ context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
	version domain.FundsFactVersion,
) (ports.ExternalFundsFactRegistration, bool, error) {
	if double.fundsErr != nil {
		return ports.ExternalFundsFactRegistration{}, false, double.fundsErr
	}
	registration, ok := double.funds[fundsVersionKey(tenant, fact, version)]
	return registration, ok, nil
}

func verificationKey(key ports.DutyVerificationKey) string {
	return key.TenantID.String() + "|" + key.Duty.String() + "|" + key.Funds.String() + "|" + key.Scope.String() + "|" + key.Digest
}

func (double *dutyStoreDouble) FindVerification(
	_ context.Context,
	key ports.DutyVerificationKey,
) (ports.DutyVerificationRecord, bool, error) {
	if double.verificationErr != nil {
		return ports.DutyVerificationRecord{}, false, double.verificationErr
	}
	found, ok := double.verifications[verificationKey(key)]
	return found, ok, nil
}

// ListVerificationsByFundsFact 照真库口径按核对时刻升序、同一时刻按指纹字典序列一条事实的全部核对版本。
func (double *dutyStoreDouble) ListVerificationsByFundsFact(
	_ context.Context,
	tenant domain.TenantID,
	funds domain.ExternalFundsFactReference,
) ([]ports.DutyVerificationRecord, error) {
	if double.verificationErr != nil {
		return nil, double.verificationErr
	}
	var records []ports.DutyVerificationRecord
	for _, record := range double.verifications {
		if record.Key.TenantID == tenant && record.Key.Funds == funds {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i].Verification.VerifiedAt(), records[j].Verification.VerifiedAt()
		if !left.Equal(right) {
			return left.Before(right)
		}
		return records[i].Key.Digest < records[j].Key.Digest
	})
	return records, nil
}

func (double *dutyStoreDouble) SaveVerification(
	_ context.Context,
	record ports.DutyVerificationRecord,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.verificationErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.verificationErr
	}
	key := verificationKey(record.Key)
	if _, exists := double.verifications[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.verifications[key] = record
	return ports.CaseConfigurationRegistered, nil
}

func (double *dutyStoreDouble) HandOffDutyPaymentVerification(
	_ context.Context,
	intent ports.DutyPaymentVerificationHandoffIntent,
) error {
	if double.handoffErr != nil {
		return double.handoffErr
	}
	double.handoffs = append(double.handoffs, intent)
	return nil
}

type dutyClock struct{ at time.Time }

func (clock dutyClock) Now() time.Time { return clock.at }

func newDutyHandler(t *testing.T, store *dutyStoreDouble) *application.DutyPaymentReconciliationHandler {
	t.Helper()
	handler, err := application.NewDutyPaymentReconciliationHandler(fullDutyDeps(store))
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	return handler
}

func fullDutyDeps(store *dutyStoreDouble) application.DutyPaymentReconciliationDeps {
	return application.DutyPaymentReconciliationDeps{
		Collaborations: store,
		Funds:          store,
		Verifications:  store,
		PayerRules:     store,
		Handoff:        store,
		Clock:          dutyClock{at: dutyBaseAt},
	}
}

// 构造门：DutyPaymentReconciliationDeps 每一口各缺一次，构造期就以具名错误停下，不等到
// FormCollaboration / ReceiveFundsFact / VerifyPayment 解引用时才 panic（票 sa-cc/14；形照 SA
// NewApplyPreAcceptanceControlHandler）。表里的口要与 Deps 的字段一一对上——加口不加表，这里不会红。
func TestTheReconciliationHandlerNamesWhichDependencyIsMissing(t *testing.T) {
	store := newDutyStore()
	cases := []struct {
		name   string
		mutate func(*application.DutyPaymentReconciliationDeps)
	}{
		{"duty collaboration store", func(deps *application.DutyPaymentReconciliationDeps) { deps.Collaborations = nil }},
		{"external funds fact register", func(deps *application.DutyPaymentReconciliationDeps) { deps.Funds = nil }},
		{"duty verification store", func(deps *application.DutyPaymentReconciliationDeps) { deps.Verifications = nil }},
		{"payer requirement rule view", func(deps *application.DutyPaymentReconciliationDeps) { deps.PayerRules = nil }},
		{"duty payment verification handoff", func(deps *application.DutyPaymentReconciliationDeps) { deps.Handoff = nil }},
		{"clock", func(deps *application.DutyPaymentReconciliationDeps) { deps.Clock = nil }},
	}
	for _, testCase := range cases {
		deps := fullDutyDeps(store)
		testCase.mutate(&deps)
		handler, err := application.NewDutyPaymentReconciliationHandler(deps)
		if !errors.Is(err, application.ErrNilDependency) {
			t.Fatalf("缺 %s：err = %v, want ErrNilDependency", testCase.name, err)
		}
		if !strings.Contains(err.Error(), testCase.name) {
			t.Fatalf("缺 %s：错误没点名那一口：%v", testCase.name, err)
		}
		if handler != nil {
			t.Fatalf("缺 %s：拒了还交出编排", testCase.name)
		}
	}
	if _, err := application.NewDutyPaymentReconciliationHandler(fullDutyDeps(store)); err != nil {
		t.Fatalf("口齐全却被拒：%v", err)
	}
}

func assessedCollaborationCommand(t *testing.T) application.FormDutyCollaborationCommand {
	t.Helper()
	return application.FormDutyCollaborationCommand{
		TenantID:    configValue(t, domain.NewTenantID, "tenant-a"),
		Kind:        domain.ObligationFromAssessedDuty,
		Duty:        configValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
		Scope:       configValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		Obligor:     configValue(t, domain.NewLegalObligorReference, "SYN-OBLIGOR-01"),
		Requirement: configValue(t, domain.NewPaymentRequirementSource, "SYN-ASSESSMENT-01"),
		Target:      configValue(t, domain.NewResponsibilityTargetReference, "SYN-DUTY-DESK"),
	}
}

func notRequiredCollaborationCommand(t *testing.T) application.FormDutyCollaborationCommand {
	t.Helper()
	command := assessedCollaborationCommand(t)
	command.Kind = domain.ObligationExplicitlyNotRequired
	command.Duty = domain.AssessedDutyReference{}
	command.NoPayBasis = "SYN-PROGRAM-01: no duty on this scope"
	return command
}

func fundsFactCommand(t *testing.T) application.ReceiveExternalFundsFactCommand {
	t.Helper()
	return application.ReceiveExternalFundsFactCommand{
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		Registration: ports.ExternalFundsFactRegistration{
			Fact:        configValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
			Version:     configValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1"),
			Source:      "SYN-BANK-01",
			Payer:       configValue(t, domain.ProvidedFundsPayer, "SYN-PAYER-01"),
			Currency:    "XTS",
			AmountMinor: 12500,
			OccurredAt:  dutyBaseAt.Add(-time.Hour),
		},
	}
}

func verifyDutyCommand(t *testing.T) application.VerifyDutyPaymentCommand {
	t.Helper()
	return application.VerifyDutyPaymentCommand{
		TenantID:     configValue(t, domain.NewTenantID, "tenant-a"),
		Duty:         configValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
		Funds:        configValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
		FundsVersion: configValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v1"),
		Scope:        configValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		Procedure:    configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-01"),
		Coverage:     domain.CoverageFull,
		Delta:        domain.DeltaNone,
		Validity:     domain.FundsFactValid,
		Basis:        "SYN-RULE-01: assessment reference quoted on the remittance",
	}
}

// unprovidedPayerFactCommand 是一条来源显式未提供付款人的资金事实——付款人三停格里唯一会让答案分岔的形。
func unprovidedPayerFactCommand(t *testing.T) application.ReceiveExternalFundsFactCommand {
	t.Helper()
	command := fundsFactCommand(t)
	command.Registration.Payer = domain.FundsPayerNotProvided()
	return command
}

// formedOverUnprovidedPayer 铺好核对的两道前置：协作事项已形成、一条未提供付款人的事实已接收。
func formedOverUnprovidedPayer(t *testing.T, store *dutyStoreDouble) *application.DutyPaymentReconciliationHandler {
	t.Helper()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if result, err := handler.ReceiveFundsFact(t.Context(), unprovidedPayerFactCommand(t)); err != nil ||
		result.Outcome() != application.FundsFactReceived {
		t.Fatalf("未提供付款人的事实该`已接收`：err=%v outcome=%v", err, result.Outcome())
	}
	return handler
}

// 步 4–5：核定税费格与明确无需付款格各成一份协作事项，两格都不创建支付交易——形成的只是
// 范围化核对入口。
func TestBothCollaborationKindsAreFormed(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)

	result, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t))
	if err != nil || result.Outcome() != application.CollaborationFormed {
		t.Fatalf("核定税费格：err=%v outcome=%v", err, result.Outcome())
	}
	result, err = handler.FormCollaboration(t.Context(), notRequiredCollaborationCommand(t))
	if err != nil || result.Outcome() != application.CollaborationFormed {
		t.Fatalf("无需付款格：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.collaborations) != 2 {
		t.Fatalf("协作事项行数走样：%d", len(store.collaborations))
	}
	for _, collaboration := range store.collaborations {
		if !collaboration.FormedAt().Equal(dutyBaseAt) {
			t.Fatalf("形成时间没取时钟：%v", collaboration.FormedAt())
		}
	}
}

// 「缺少税费结果」走不进任何一格：既不是核定税费也没有明确无需付款依据时，编排保持未决——
// 不形成支付指令，也不把「没有税费消息」解释为无需付款（UC-CC-009 启动条件那句）。
func TestAMissingDutyResultKeepsTheCollaborationUndecided(t *testing.T) {
	store := newDutyStore()
	command := assessedCollaborationCommand(t)
	command.Kind = domain.DutyObligationKindInvalid
	command.Duty = domain.AssessedDutyReference{}

	result, err := newDutyHandler(t, store).FormCollaboration(t.Context(), command)
	if err != nil || result.Outcome() != application.DutyReconciliationUndecided ||
		result.UndecidedReason() != application.DutyObligationBasisAbsent {
		t.Fatalf("缺税费结果该未决且指名依据缺席：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	if len(store.collaborations) != 0 {
		t.Fatal("未决却落了协作事项")
	}
}

// 受理门与领域形状：租户缺席不受理；核定税费格带无需付款依据、无需付款格带税费引用都是
// 领域拒的矛盾形状，翻成`未受理`且不落。
func TestCollaborationRefusesContradictoryShapes(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)

	blankTenant := assessedCollaborationCommand(t)
	blankTenant.TenantID = domain.TenantID{}
	assessedWithBasis := assessedCollaborationCommand(t)
	assessedWithBasis.NoPayBasis = "should not be here"
	notRequiredWithDuty := notRequiredCollaborationCommand(t)
	notRequiredWithDuty.Duty = configValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1")

	for name, command := range map[string]application.FormDutyCollaborationCommand{
		"租户缺席":       blankTenant,
		"核定格带无需付款依据": assessedWithBasis,
		"无需付款格带税费引用": notRequiredWithDuty,
	} {
		result, err := handler.FormCollaboration(t.Context(), command)
		if err != nil || result.Outcome() != application.DutyReconciliationNotAccepted {
			t.Fatalf("%s该不受理：err=%v outcome=%v", name, err, result.Outcome())
		}
	}
	if len(store.collaborations) != 0 {
		t.Fatal("被拒的协作事项落了册")
	}
}

// 同（范围，税费引用）重放是`已存在`；换责任交接目标是`内容冲突`——在册那份纹丝不动。
func TestReFormingACollaborationSplitsReplayFromConflict(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)

	if result, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil ||
		result.Outcome() != application.CollaborationFormed {
		t.Fatalf("首次：err=%v outcome=%v", err, result.Outcome())
	}
	if result, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil ||
		result.Outcome() != application.CollaborationExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, result.Outcome())
	}
	changed := assessedCollaborationCommand(t)
	changed.Target = configValue(t, domain.NewResponsibilityTargetReference, "SYN-OTHER-DESK")
	if result, err := handler.FormCollaboration(t.Context(), changed); err != nil ||
		result.Outcome() != application.CollaborationContentConflict {
		t.Fatalf("换目标该是`内容冲突`：err=%v outcome=%v", err, result.Outcome())
	}
}

// 步 6 的 CC 半边：外部资金事实按（引用 + 版本）入向登记；同键重放`已存在`，同键换金额是`内容冲突`
// ——资金事实的更正在来源那头是同一事实的新版本回指前版（见下一条用例），不是同一版本改数。
func TestExternalFundsFactsAreReceivedByReference(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)

	if result, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil ||
		result.Outcome() != application.FundsFactReceived {
		t.Fatalf("首次：err=%v outcome=%v", err, result.Outcome())
	}
	if result, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil ||
		result.Outcome() != application.FundsFactExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, result.Outcome())
	}
	changed := fundsFactCommand(t)
	changed.Registration.AmountMinor = 99
	if result, err := handler.ReceiveFundsFact(t.Context(), changed); err != nil ||
		result.Outcome() != application.FundsFactContentConflict {
		t.Fatalf("同引用换金额该是`内容冲突`：err=%v outcome=%v", err, result.Outcome())
	}

	blank := fundsFactCommand(t)
	blank.Registration.Currency = ""
	if result, err := handler.ReceiveFundsFact(t.Context(), blank); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("币种缺席该不受理：err=%v outcome=%v", err, result.Outcome())
	}
}

// correctionOf 造同一事实的更正版本：新版本字面、回指被更正的那一版、金额改了。
func correctionOf(t *testing.T, previous application.ReceiveExternalFundsFactCommand, version string, amountMinor int64) application.ReceiveExternalFundsFactCommand {
	t.Helper()
	command := previous
	command.Registration.Version = configValue(t, domain.NewFundsFactVersion, version)
	command.Registration.Corrects = previous.Registration.Version
	command.Registration.AmountMinor = amountMinor
	return command
}

// Covers: 票 sa-cc/13 完成判据 1——v1 已登记，v2 同引用、回指 v1、金额变 → `已接收`落新一行；按引用列出两版且
// v2 回指 v1，v1 一字不动（UC-CC-009「不删除原付款、不按最后到达覆盖」）；同版本重投 → `已存在`；同版本换内容
// （金额或回指）→ `内容冲突`——那才是真冲突：同一版本两个来源各说一套（裁决 1）。两版各自按版本点读得回
// （票 sa-cc/19 起「最近接收」读口退役）。
func TestACorrectionVersionIsReceivedAsANewRowThatPointsBackToTheVersionItCorrects(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	first := fundsFactCommand(t)
	if _, err := handler.ReceiveFundsFact(t.Context(), first); err != nil {
		t.Fatalf("首版：%v", err)
	}

	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if result, err := handler.ReceiveFundsFact(t.Context(), second); err != nil ||
		result.Outcome() != application.FundsFactReceived {
		t.Fatalf("更正版本该`已接收`成新一行：err=%v outcome=%v", err, result.Outcome())
	}
	versions, err := store.ListFundsFactVersions(t.Context(), first.TenantID, first.Registration.Fact)
	if err != nil || len(versions) != 2 {
		t.Fatalf("按引用该列出两版：err=%v n=%d", err, len(versions))
	}
	if versions[0] != first.Registration {
		t.Fatalf("原版本被动过：%+v", versions[0])
	}
	if versions[1].Version != second.Registration.Version || versions[1].Corrects != first.Registration.Version ||
		versions[1].AmountMinor != 9000 {
		t.Fatalf("新版本该回指 v1 且带自己的内容：%+v", versions[1])
	}
	for _, want := range []application.ReceiveExternalFundsFactCommand{first, second} {
		if got, found, _ := store.LoadFundsFactVersion(t.Context(), want.TenantID, want.Registration.Fact, want.Registration.Version); !found ||
			got != want.Registration {
			t.Fatalf("按版本点读 %s 该原样交回：found=%v got=%+v", want.Registration.Version, found, got)
		}
	}

	if result, err := handler.ReceiveFundsFact(t.Context(), second); err != nil ||
		result.Outcome() != application.FundsFactExisting {
		t.Fatalf("同版本重投该`已存在`：err=%v outcome=%v", err, result.Outcome())
	}
	changedAmount := second
	changedAmount.Registration.AmountMinor = 9001
	if result, err := handler.ReceiveFundsFact(t.Context(), changedAmount); err != nil ||
		result.Outcome() != application.FundsFactContentConflict {
		t.Fatalf("同版本换金额该`内容冲突`：err=%v outcome=%v", err, result.Outcome())
	}
	changedCorrects := second
	changedCorrects.Registration.Corrects = domain.FundsFactVersion{}
	if result, err := handler.ReceiveFundsFact(t.Context(), changedCorrects); err != nil ||
		result.Outcome() != application.FundsFactContentConflict {
		t.Fatalf("同版本换回指该`内容冲突`：err=%v outcome=%v", err, result.Outcome())
	}
	if versions, _ := store.ListFundsFactVersions(t.Context(), first.TenantID, first.Registration.Fact); len(versions) != 2 ||
		versions[1].AmountMinor != 9000 {
		t.Fatalf("冲突不得顶替已登记的版本：%+v", versions)
	}
}

// 版本是键维，缺了就登不成键：版本空白`未受理`；回指自己也是形状矛盾，`未受理`；两格都不落。迟到的前版按
// 自己的版本进（先到 v2 再到 v1 各占一行），不因为「更旧」被拒、也不覆盖谁。
func TestFundsFactVersionsKeepTheirOwnRowsRegardlessOfArrivalOrder(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	first := fundsFactCommand(t)

	blankVersion := first
	blankVersion.Registration.Version = domain.FundsFactVersion{}
	if result, err := handler.ReceiveFundsFact(t.Context(), blankVersion); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("版本空白该`未受理`：err=%v outcome=%v", err, result.Outcome())
	}
	selfCorrecting := first
	selfCorrecting.Registration.Corrects = first.Registration.Version
	if result, err := handler.ReceiveFundsFact(t.Context(), selfCorrecting); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("回指自己该`未受理`：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.funds) != 0 {
		t.Fatalf("被拒的登记落了册：%d", len(store.funds))
	}

	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if result, err := handler.ReceiveFundsFact(t.Context(), second); err != nil || result.Outcome() != application.FundsFactReceived {
		t.Fatalf("先到的 v2：err=%v outcome=%v", err, result.Outcome())
	}
	if result, err := handler.ReceiveFundsFact(t.Context(), first); err != nil || result.Outcome() != application.FundsFactReceived {
		t.Fatalf("迟到的 v1 该按自己的版本进：err=%v outcome=%v", err, result.Outcome())
	}
	versions, _ := store.ListFundsFactVersions(t.Context(), first.TenantID, first.Registration.Fact)
	if len(versions) != 2 || versions[0].Version != second.Registration.Version || versions[1].Version != first.Registration.Version {
		t.Fatalf("两版该各占一行、按接收先后列：%+v", versions)
	}
}

// Covers: 票 sa-cc/12 完成判据 1 的登记半边——来源未提供付款人的事实`已接收`，登记里付款人显式「未提供」
// （CONTEXT「未提供或不适用必须明确记录」）；同引用重放`已存在`；同引用换成「提供了」是`内容冲突`（付款人
// 是登记内容的一维，不是可补的空位）；两格都不是的零值付款人是矛盾输入，`未受理`且不落。
func TestAFundsFactWithoutAPayerIsReceivedWithThePayerRecordedAsNotProvided(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)

	unprovided := fundsFactCommand(t)
	unprovided.Registration.Payer = domain.FundsPayerNotProvided()
	if result, err := handler.ReceiveFundsFact(t.Context(), unprovided); err != nil ||
		result.Outcome() != application.FundsFactReceived {
		t.Fatalf("来源未提供付款人该`已接收`：err=%v outcome=%v", err, result.Outcome())
	}
	registered, found, err := store.LoadFundsFactVersion(t.Context(), unprovided.TenantID, unprovided.Registration.Fact, unprovided.Registration.Version)
	if err != nil || !found || registered.Payer.Provided() || !registered.Payer.Valid() {
		t.Fatalf("登记里付款人该显式为「未提供」：found=%v err=%v payer=%#v", found, err, registered.Payer)
	}

	if result, err := handler.ReceiveFundsFact(t.Context(), unprovided); err != nil ||
		result.Outcome() != application.FundsFactExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, result.Outcome())
	}

	nowProvided := fundsFactCommand(t)
	if result, err := handler.ReceiveFundsFact(t.Context(), nowProvided); err != nil ||
		result.Outcome() != application.FundsFactContentConflict {
		t.Fatalf("同引用从「未提供」换成「提供了」该是`内容冲突`：err=%v outcome=%v", err, result.Outcome())
	}

	zero := fundsFactCommand(t)
	zero.Registration.Fact = configValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-02")
	zero.Registration.Payer = domain.FundsPayer{}
	if result, err := handler.ReceiveFundsFact(t.Context(), zero); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("零值付款人该`未受理`：err=%v outcome=%v", err, result.Outcome())
	}
	if _, found, _ := store.LoadFundsFactVersion(t.Context(), zero.TenantID, zero.Registration.Fact, zero.Registration.Version); found {
		t.Fatal("零值付款人的事实落了册")
	}
}

// 步 7 的正路：协作事项在、资金事实已接收、关联依据在——形成三轴分立的核对，验证时间取时钟，
// 依据随记录留下。
func TestAVerificationIsFormedOnAReceivedFactAgainstAFormedCollaboration(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}

	result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
	if err != nil || result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("核对：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.verifications) != 1 {
		t.Fatalf("核对行数走样：%d", len(store.verifications))
	}
	for _, record := range store.verifications {
		if record.Verification.Coverage() != domain.CoverageFull || record.Verification.Delta() != domain.DeltaNone ||
			record.Verification.Validity() != domain.FundsFactValid || !record.Verification.VerifiedAt().Equal(dutyBaseAt) ||
			record.Basis == "" {
			t.Fatalf("核对走样：%+v", record)
		}
	}
	replay, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
	if err != nil || replay.Outcome() != application.DutyVerificationExisting {
		t.Fatalf("同内容重核该是`已存在`：err=%v outcome=%v", err, replay.Outcome())
	}
}

// 迟到事实改判：同三维、三轴不同是新版本追加（不覆盖前版），两版都在。
func TestAChangedVerificationAppendsANewVersion(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	if _, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil {
		t.Fatalf("首版：%v", err)
	}

	invalidated := verifyDutyCommand(t)
	invalidated.Validity = domain.FundsFactInvalidated
	invalidated.Coverage = domain.CoverageNone
	invalidated.Basis = "SYN-BANK-01: remittance reversed"
	result, err := handler.VerifyPayment(t.Context(), invalidated)
	if err != nil || result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("改判该是新版本：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.verifications) != 2 {
		t.Fatalf("改判覆盖了前版：%d", len(store.verifications))
	}
}

// Covers: 票 sa-cc/22 完成判据 (1)——程序是核对记录的依据维并折进版本指纹（裁决 1 / 2）：同三轴同依据、按不同
// 程序的规则判付款人维，是两份不同的判断 → 两行并存、两封信封、指纹不同；同程序第二次是重放`已存在`；落册的
// 核对对象自己带着它按哪个程序判。程序空白仍`未受理`（既有形，钉在
// TestThePayerRuleIsReadAfterBothPrerequisitesAndNamesItsOwnFailure）。键上三维身份不变——身份不带程序，
// 是指纹带（裁决 2「折进指纹、不加主键列」）。
func TestVerificationsUnderDifferentProceduresAreDifferentVersions(t *testing.T) {
	store := newDutyStore()
	store.payerRules["tenant-a|SYN-PROC-02"] = domain.PayerRequired
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}

	underFirst := verifyDutyCommand(t)
	if result, err := handler.VerifyPayment(t.Context(), underFirst); err != nil ||
		result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("按 SYN-PROC-01 判：err=%v outcome=%v", err, result.Outcome())
	}
	underSecond := verifyDutyCommand(t)
	underSecond.Procedure = configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-02")
	if result, err := handler.VerifyPayment(t.Context(), underSecond); err != nil ||
		result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("同三轴同依据、按 SYN-PROC-02 判该是另一版：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.verifications) != 2 || len(store.handoffs) != 2 {
		t.Fatalf("两个程序该各成一版、各交一封：%d 行 %d 封", len(store.verifications), len(store.handoffs))
	}
	if store.handoffs[0].Key.Digest == store.handoffs[1].Key.Digest {
		t.Fatal("换程序没换指纹——程序没折进版本指纹")
	}
	procedures := map[domain.CustomsProcedureReference]bool{}
	for _, record := range store.verifications {
		if record.Key.Duty != underFirst.Duty || record.Key.Funds != underFirst.Funds || record.Key.Scope != underFirst.Scope {
			t.Fatalf("键上三维身份该原样：%+v", record.Key)
		}
		procedures[record.Verification.Procedure()] = true
	}
	if !procedures[underFirst.Procedure] || !procedures[underSecond.Procedure] {
		t.Fatalf("落册的核对该各自带着按哪个程序判：%v", procedures)
	}

	if result, err := handler.VerifyPayment(t.Context(), underSecond); err != nil ||
		result.Outcome() != application.DutyVerificationExisting {
		t.Fatalf("同程序重核该是`已存在`：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.verifications) != 2 || len(store.handoffs) != 2 {
		t.Fatalf("重放不得再落行、再交封：%d 行 %d 封", len(store.verifications), len(store.handoffs))
	}
}

// Covers: 票 sa-cc/19 做法 3——核对按版本读：`FundsVersion` 必填（空白`未受理`）；前置按命令所指那一版读——v1 在册、
// 命令指 v9 → `资金事实未接收`，别的版本在册不顶替；v2 到册后以同三轴同依据同程序、只换资金版本再核 → 另一版
// （资金版本折进指纹，裁决 3 (3)），落册对象各带自己比的那一版；同版本重核`已存在`。
func TestAVerificationReadsThePrerequisiteByTheVersionItNames(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	first := fundsFactCommand(t)
	if _, err := handler.ReceiveFundsFact(t.Context(), first); err != nil {
		t.Fatalf("资金事实 v1：%v", err)
	}

	blankVersion := verifyDutyCommand(t)
	blankVersion.FundsVersion = domain.FundsFactVersion{}
	if result, err := handler.VerifyPayment(t.Context(), blankVersion); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("不说比的是哪一版该`未受理`：err=%v outcome=%v", err, result.Outcome())
	}
	unreceived := verifyDutyCommand(t)
	unreceived.FundsVersion = configValue(t, domain.NewFundsFactVersion, "SYN-FUNDS-01/v9")
	if result, err := handler.VerifyPayment(t.Context(), unreceived); err != nil ||
		result.Outcome() != application.FundsFactNotReceived {
		t.Fatalf("命令所指那一版没接收，别的版本在册不顶替：err=%v outcome=%v", err, result.Outcome())
	}

	onFirst := verifyDutyCommand(t)
	if result, err := handler.VerifyPayment(t.Context(), onFirst); err != nil ||
		result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("按 v1 核对：err=%v outcome=%v", err, result.Outcome())
	}
	second := correctionOf(t, first, "SYN-FUNDS-01/v2", 9000)
	if _, err := handler.ReceiveFundsFact(t.Context(), second); err != nil {
		t.Fatalf("资金事实 v2：%v", err)
	}
	onSecond := verifyDutyCommand(t)
	onSecond.FundsVersion = second.Registration.Version
	if result, err := handler.VerifyPayment(t.Context(), onSecond); err != nil ||
		result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("同三轴同依据同程序、只换资金版本该是另一版：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.verifications) != 2 || len(store.handoffs) != 2 || store.handoffs[0].Key.Digest == store.handoffs[1].Key.Digest {
		t.Fatalf("两版该各成一行、各交一封、指纹不同：%d 行 %d 封", len(store.verifications), len(store.handoffs))
	}
	versions := map[domain.FundsFactVersion]bool{}
	for _, record := range store.verifications {
		versions[record.Verification.FundsVersion()] = true
	}
	if !versions[first.Registration.Version] || !versions[second.Registration.Version] {
		t.Fatalf("落册的核对该各自带着比的是哪一版：%v", versions)
	}
	if result, err := handler.VerifyPayment(t.Context(), onSecond); err != nil ||
		result.Outcome() != application.DutyVerificationExisting {
		t.Fatalf("同版本重核该`已存在`：err=%v outcome=%v", err, result.Outcome())
	}
}

// 步 8 的结算交接（票 sa-cc/05 完成判据 1）：核对形成那一格交一封，意图由核对幂等键认领、
// 携带的就是刚落册那一版的三维键与指纹；同内容重核是`已存在`，**不再调交接口**——信封随形成
// 那一版同事务入队，重放没有可补的那一格；改判是新版本，再交一封、键上指纹不同。
func TestAFormedVerificationHandsOffOneEnvelopeAndReplayDoesNotResend(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}

	formed, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
	if err != nil || formed.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("核对：err=%v outcome=%v", err, formed.Outcome())
	}
	if formed.HandoffReference() != "" {
		t.Fatalf("交接成功不该留续办引用，实得 %q", formed.HandoffReference())
	}
	if len(store.handoffs) != 1 {
		t.Fatalf("形成后意图数 = %d，want 1", len(store.handoffs))
	}
	for key, record := range store.verifications {
		intent := store.handoffs[0]
		if verificationKey(intent.Key) != key {
			t.Fatalf("意图认领的键 = %+v，不是刚落册那一版 %s", intent.Key, key)
		}
		if intent.Key.Digest == "" || intent.Verification.Duty() != record.Verification.Duty() ||
			intent.Verification.Funds() != record.Verification.Funds() ||
			intent.Verification.Scope() != record.Verification.Scope() ||
			!intent.Verification.VerifiedAt().Equal(record.Verification.VerifiedAt()) {
			t.Fatalf("意图携带的核对与落册那份不是同一件：%+v", intent)
		}
	}

	replay, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
	if err != nil || replay.Outcome() != application.DutyVerificationExisting {
		t.Fatalf("重核：err=%v outcome=%v", err, replay.Outcome())
	}
	if len(store.handoffs) != 1 {
		t.Fatalf("`已存在`后意图数 = %d，want 1——重放不重发", len(store.handoffs))
	}

	invalidated := verifyDutyCommand(t)
	invalidated.Validity = domain.FundsFactInvalidated
	invalidated.Coverage = domain.CoverageNone
	invalidated.Basis = "SYN-BANK-01: remittance reversed"
	if result, err := handler.VerifyPayment(t.Context(), invalidated); err != nil ||
		result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("改判：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.handoffs) != 2 || store.handoffs[0].Key.Digest == store.handoffs[1].Key.Digest {
		t.Fatalf("改判该另交一封且指纹不同：%d 封", len(store.handoffs))
	}
}

// 没形成核对的每一格都不交：无依据的待关联、资金事实未接收、协作事项未形成、三轴集外、
// 核对库故障——信封说的是「这一版核对已形成」，没形成就无物可交。
func TestNoEnvelopeLeavesWhenNoVerificationIsFormed(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)

	noBasis := verifyDutyCommand(t)
	noBasis.Basis = ""
	if result, err := handler.VerifyPayment(t.Context(), noBasis); err != nil ||
		result.Outcome() != application.FundsFactPendingAssociation {
		t.Fatalf("待关联：err=%v outcome=%v", err, result.Outcome())
	}
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.FundsFactNotReceived {
		t.Fatalf("资金事实未接收：err=%v outcome=%v", err, result.Outcome())
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.CollaborationNotFormed {
		t.Fatalf("协作事项未形成：err=%v outcome=%v", err, result.Outcome())
	}
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	offAxis := verifyDutyCommand(t)
	offAxis.Delta = domain.DutyDeltaInvalid
	if result, err := handler.VerifyPayment(t.Context(), offAxis); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("三轴集外：err=%v outcome=%v", err, result.Outcome())
	}
	store.verificationErr = errors.New("verification store down")
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.DutyReconciliationUndecided {
		t.Fatalf("核对库故障：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.handoffs) != 0 {
		t.Fatalf("没形成核对却交了 %d 封", len(store.handoffs))
	}
}

// 交接失败按仓内既有形（close_customs_case 的 handOffClosure）：核对已落册不翻成未决，续办引用
// 非空指名哪一版的信封没交出去。这里不再有「重放补交」那半——`已存在`不重发是本口有意的选择：
// 真库上信封与核对同一事务，库侧入队失败会把整笔事务连核对一起中止，重跑仍走`形成`那一格。
func TestAFailedHandoffLeavesTheVerificationFormedWithAContinuationReference(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	store.handoffErr = errors.New("outbox unavailable")

	result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
	if err != nil || result.Outcome() != application.DutyVerificationFormed {
		t.Fatalf("核对已落册，交接失败不翻它：err=%v outcome=%v", err, result.Outcome())
	}
	if result.HandoffReference() == "" {
		t.Fatal("交接失败必须留续办引用")
	}
	if len(store.verifications) != 1 {
		t.Fatalf("核对行数 = %d，want 1", len(store.verifications))
	}
	if len(store.handoffs) != 0 {
		t.Fatalf("失败的交接不该留下意图：%d", len(store.handoffs))
	}
}

// 无权威关联依据时保持外部资金事实待关联——金额相等、同一范围都不单独构成关联；不形成核对。
func TestAVerificationWithoutABasisKeepsTheFactPendingAssociation(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	command := verifyDutyCommand(t)
	command.Basis = "   "

	result, err := handler.VerifyPayment(t.Context(), command)
	if err != nil || result.Outcome() != application.FundsFactPendingAssociation {
		t.Fatalf("无依据该保持待关联：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.verifications) != 0 {
		t.Fatal("无依据却形成了核对")
	}
}

// 步 7 的两道前置各有自己的格：资金事实没经步 6 接收是`资金事实未接收`；协作事项没经步 4–5
// 形成是`协作事项未形成`——两者都不是核对结论，续办动作不同。
func TestAVerificationNamesWhichPrerequisiteIsMissing(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)

	noFact, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
	if err != nil || noFact.Outcome() != application.FundsFactNotReceived {
		t.Fatalf("事实未接收：err=%v outcome=%v", err, noFact.Outcome())
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	noCollaboration, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
	if err != nil || noCollaboration.Outcome() != application.CollaborationNotFormed {
		t.Fatalf("协作事项未形成：err=%v outcome=%v", err, noCollaboration.Outcome())
	}
}

// 三轴集外与租户缺席不受理；依赖故障各折未决并指名哪一口。
func TestDutyReconciliationRefusalsAndDependencyFailures(t *testing.T) {
	store := newDutyStore()
	handler := newDutyHandler(t, store)
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), fundsFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	offAxis := verifyDutyCommand(t)
	offAxis.Coverage = domain.DutyCoverageInvalid
	if result, err := handler.VerifyPayment(t.Context(), offAxis); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("覆盖轴集外该不受理：err=%v outcome=%v", err, result.Outcome())
	}

	store.verificationErr = errors.New("verification store down")
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.DutyReconciliationUndecided ||
		result.UndecidedReason() != application.DutyVerificationStoreUnavailable {
		t.Fatalf("核对库故障该未决：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	store.verificationErr = nil
	store.fundsErr = errors.New("funds register down")
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.DutyReconciliationUndecided ||
		result.UndecidedReason() != application.FundsFactRegisterUnavailable {
		t.Fatalf("资金登记册故障该未决：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	store.fundsErr = nil
	store.collaborationErr = errors.New("collaboration store down")
	if result, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil ||
		result.Outcome() != application.DutyReconciliationUndecided ||
		result.UndecidedReason() != application.CollaborationStoreUnavailable {
		t.Fatalf("协作库故障该未决：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
}

// Covers: 票 sa-cc/12 完成判据 1 的核对半边——裁决 2 的三停格（CC CONTEXT「未提供或不适用必须明确记录，
// 规则要求但缺失时保持未决」）。付款人那一维按命令所指监管程序的登记规则判：
//   - 程序要求而来源未提供 → 未决 PayerRequiredNotProvided，等的是来源补事实；
//   - 程序不要求而来源未提供 → 照常形成，「未提供」原样带着——不是「不适用」，也不替它补任何值；
//   - 程序没登要不要 → 未决 PayerRequirementNotConfigured，等的是登记方补规则，不取任何默认。
//
// 前一格与后一格恢复动作不同，所以是两个词（ADR-0029）；两格都不落核对、不交信封。
func TestThePayerDimensionIsJudgedByTheProcedureRule(t *testing.T) {
	t.Run("程序要求而来源未提供", func(t *testing.T) {
		store := newDutyStore()
		handler := formedOverUnprovidedPayer(t, store)

		result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
		if err != nil || result.Outcome() != application.DutyReconciliationUndecided ||
			result.UndecidedReason() != application.PayerRequiredNotProvided {
			t.Fatalf("该未决且点名缺付款人：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
		}
		if len(store.verifications) != 0 || len(store.handoffs) != 0 {
			t.Fatalf("未决却落了核对 %d 行、交了 %d 封", len(store.verifications), len(store.handoffs))
		}
	})

	t.Run("程序不要求而来源未提供", func(t *testing.T) {
		store := newDutyStore()
		store.payerRules["tenant-a|SYN-PROC-01"] = domain.PayerNotRequired
		handler := formedOverUnprovidedPayer(t, store)

		result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
		if err != nil || result.Outcome() != application.DutyVerificationFormed {
			t.Fatalf("不要求付款人该照常形成：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
		}
		if len(store.verifications) != 1 || len(store.handoffs) != 1 {
			t.Fatalf("形成该落一行、交一封：%d 行 %d 封", len(store.verifications), len(store.handoffs))
		}
		command := verifyDutyCommand(t)
		registered, _, _ := store.LoadFundsFactVersion(t.Context(), command.TenantID, command.Funds, command.FundsVersion)
		if registered.Payer.Provided() || !registered.Payer.Valid() {
			t.Fatalf("核对不得替事实补付款人：%#v", registered.Payer)
		}
	})

	t.Run("程序没登要不要", func(t *testing.T) {
		store := newDutyStore()
		delete(store.payerRules, "tenant-a|SYN-PROC-01")
		handler := formedOverUnprovidedPayer(t, store)

		result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t))
		if err != nil || result.Outcome() != application.DutyReconciliationUndecided ||
			result.UndecidedReason() != application.PayerRequirementNotConfigured {
			t.Fatalf("该未决且点名缺规则：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
		}
		if len(store.verifications) != 0 || len(store.handoffs) != 0 {
			t.Fatalf("未决却落了核对 %d 行、交了 %d 封", len(store.verifications), len(store.handoffs))
		}
	})
}

// 规则那一维的边：读口故障是依赖故障（重投会变），与「规则未配置」（重投不会变）分格指名；命令不带监管程序
// 是形状缺格，`未受理`，不是任何一格业务答案；规则在两道前置之后才读——事实未接收时即便规则没登也答前置未齐，
// 缺规则不该盖住缺事实，操作员先补哪一样得从原词读得出来。
func TestThePayerRuleIsReadAfterBothPrerequisitesAndNamesItsOwnFailure(t *testing.T) {
	store := newDutyStore()
	delete(store.payerRules, "tenant-a|SYN-PROC-01")
	handler := newDutyHandler(t, store)

	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.FundsFactNotReceived {
		t.Fatalf("事实未接收该先于规则未配置：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	if _, err := handler.ReceiveFundsFact(t.Context(), unprovidedPayerFactCommand(t)); err != nil {
		t.Fatalf("资金事实：%v", err)
	}
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.CollaborationNotFormed {
		t.Fatalf("协作事项未形成该先于规则未配置：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	if _, err := handler.FormCollaboration(t.Context(), assessedCollaborationCommand(t)); err != nil {
		t.Fatalf("协作事项：%v", err)
	}

	store.payerRuleErr = errors.New("payer rule view down")
	if result, err := handler.VerifyPayment(t.Context(), verifyDutyCommand(t)); err != nil ||
		result.Outcome() != application.DutyReconciliationUndecided ||
		result.UndecidedReason() != application.PayerRequirementViewUnavailable {
		t.Fatalf("读口故障该未决且指名读口：err=%v outcome=%v reason=%v", err, result.Outcome(), result.UndecidedReason())
	}
	store.payerRuleErr = nil

	blankProcedure := verifyDutyCommand(t)
	blankProcedure.Procedure = domain.CustomsProcedureReference{}
	if result, err := handler.VerifyPayment(t.Context(), blankProcedure); err != nil ||
		result.Outcome() != application.DutyReconciliationNotAccepted {
		t.Fatalf("不带监管程序该`未受理`：err=%v outcome=%v", err, result.Outcome())
	}
	if len(store.verifications) != 0 {
		t.Fatal("没有一格该落核对")
	}
}
