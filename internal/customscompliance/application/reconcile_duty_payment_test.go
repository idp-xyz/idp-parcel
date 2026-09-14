package application_test

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

// UC-CC-009 步 4–7 的行为面（票 mechanism-executor-triage/07 CC-c）：协作事项两格分立且「缺少
// 税费结果」保持未决；外部资金事实按引用入向登记；核对须先有协作事项与已接收的资金事实、
// 且必带关联依据——无依据即保持待关联，不按金额相等猜。替身照真库代数：同键只答`已登记`。
// 步 8 的结算交接（票 sa-cc/05）也在这里钉：核对形成那一格交一封、其余格不交、交接失败不翻结果。

var dutyBaseAt = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

// dutyStoreDouble 同一本替身充当三口登记册加交接口。交接半边记下每一份意图与调用次数——
// 「`已存在`不重发」要能从调用次数上读出来，不能只靠认领键吞重去证。
type dutyStoreDouble struct {
	collaborations   map[string]domain.DutyPaymentCollaboration
	funds            map[string]ports.ExternalFundsFactRegistration
	verifications    map[string]ports.DutyVerificationRecord
	handoffs         []ports.DutyPaymentVerificationHandoffIntent
	collaborationErr error
	fundsErr         error
	verificationErr  error
	handoffErr       error
}

func newDutyStore() *dutyStoreDouble {
	return &dutyStoreDouble{
		collaborations: map[string]domain.DutyPaymentCollaboration{},
		funds:          map[string]ports.ExternalFundsFactRegistration{},
		verifications:  map[string]ports.DutyVerificationRecord{},
	}
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

func (double *dutyStoreDouble) RegisterFundsFact(
	_ context.Context,
	tenant domain.TenantID,
	registration ports.ExternalFundsFactRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.fundsErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.fundsErr
	}
	key := tenant.String() + "|" + registration.Fact.String()
	if _, exists := double.funds[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.funds[key] = registration
	return ports.CaseConfigurationRegistered, nil
}

func (double *dutyStoreDouble) LoadFundsFact(
	_ context.Context,
	tenant domain.TenantID,
	fact domain.ExternalFundsFactReference,
) (ports.ExternalFundsFactRegistration, bool, error) {
	if double.fundsErr != nil {
		return ports.ExternalFundsFactRegistration{}, false, double.fundsErr
	}
	found, ok := double.funds[tenant.String()+"|"+fact.String()]
	return found, ok, nil
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
		TenantID: configValue(t, domain.NewTenantID, "tenant-a"),
		Duty:     configValue(t, domain.NewAssessedDutyReference, "SYN-DUTY-01/v1"),
		Funds:    configValue(t, domain.NewExternalFundsFactReference, "SYN-FUNDS-01"),
		Scope:    configValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		Coverage: domain.CoverageFull,
		Delta:    domain.DeltaNone,
		Validity: domain.FundsFactValid,
		Basis:    "SYN-RULE-01: assessment reference quoted on the remittance",
	}
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

// 步 6 的 CC 半边：外部资金事实按引用入向登记；同引用重放`已存在`，同引用换金额是`内容冲突`
// ——资金事实的更正在来源那头是新事实回指原事实，不是同一引用改数。
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
	registered, found, err := store.LoadFundsFact(t.Context(), unprovided.TenantID, unprovided.Registration.Fact)
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
	if _, found, _ := store.LoadFundsFact(t.Context(), zero.TenantID, zero.Registration.Fact); found {
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
