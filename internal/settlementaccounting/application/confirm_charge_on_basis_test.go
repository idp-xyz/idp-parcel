package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0336 决定一、二：确认依据到了就记，这一户登了确认时点才调确认；预估费用形成之后同笔登七项确认事实，已终局的
// 补记依据。确认编排本体照 confirm_charge_test.go 的夹具接，依据册、时点册、账户册、终局读口是本文件的替身。夹具全是
// 合成串。

type basisRecorderDouble struct {
	recorded []string
	err      error
}

func (double *basisRecorderDouble) RecordConfirmationBasis(
	_ context.Context,
	_ domain.TenantID,
	charge domain.CustomerChargeID,
	feeItem domain.FeeItemReference,
	kind domain.ConfirmationBasisKind,
	basis domain.ConfirmationBasisReference,
	_ time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	if double.err != nil {
		return 0, double.err
	}
	double.recorded = append(double.recorded, charge.String()+"|"+feeItem.String()+"|"+kind.String()+"|"+basis.String())
	return ports.CatalogueRegistered, nil
}

type parcelRequestDouble struct {
	record ports.SellEvaluationRequestRecord
	found  bool
	err    error
}

func (double parcelRequestDouble) FindByID(
	context.Context, ports.SellEvaluationRequestKey,
) (ports.SellEvaluationRequestRecord, bool, error) {
	return ports.SellEvaluationRequestRecord{}, false, errors.New("not used by confirm on basis")
}

func (double parcelRequestDouble) FindCurrentByParcel(
	context.Context, domain.TenantID, domain.ParcelReference,
) (ports.SellEvaluationRequestRecord, bool, error) {
	return double.record, double.found, double.err
}

type confirmOnBasisFixture struct {
	confirm  *confirmFixture
	bases    *basisRecorderDouble
	requests *parcelRequestDouble
	moment   *settlementMomentViewDouble
	handler  *application.ConfirmChargeOnBasisHandler
}

func newConfirmOnBasisFixture(t *testing.T, momentRegistered bool) *confirmOnBasisFixture {
	t.Helper()
	fixture := &confirmOnBasisFixture{
		confirm:  newConfirmFixture(t),
		bases:    &basisRecorderDouble{},
		requests: &parcelRequestDouble{},
		moment:   &settlementMomentViewDouble{found: momentRegistered, moment: domain.SettlementMomentConfirm},
	}
	handler, err := application.NewConfirmChargeOnBasisHandler(application.ConfirmChargeOnBasisDeps{
		Requests: fixture.requests,
		Charges:  fixture.confirm.store,
		Bases:    fixture.bases,
		Moments:  newMomentHandler(t, momentViewRef{fixture.moment}),
		Confirm:  fixture.confirm.handler,
		Clock:    confirmClock{at: chargeConfirmedAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.handler = handler
	return fixture
}

// momentViewRef 让用例在构造之后还能改时点册的答案。
type momentViewRef struct{ view *settlementMomentViewDouble }

func (ref momentViewRef) LoadSettlementMoment(ctx context.Context, tenant domain.TenantID, moment domain.SettlementMoment) (bool, error) {
	return ref.view.LoadSettlementMoment(ctx, tenant, moment)
}

func finalBasis(t *testing.T) application.ConfirmationBasis {
	t.Helper()
	return application.ConfirmationBasis{
		Kind:      domain.EffectiveDeliveryFinalizedBasis(),
		Reference: billValue(t, domain.NewConfirmationBasisReference, "final-outcome/SYN-PARCEL-1/EFFECTIVE_DELIVERY/DRV-1"),
	}
}

// Covers: AT-SA-218（ADR-0336 决定二）——依据记在（费用、费用项目、内置种类）上；这一户登了确认时点就当场确认，确认走的是同一只确认
// 编排，答已确认、费用行转确认。
func TestABasisOnAConfirmingTenantConfirmsTheCharge(t *testing.T) {
	fixture := newConfirmOnBasisFixture(t, true)
	tenant := confirmCommand(t).TenantID

	result, err := fixture.handler.OnCharge(t.Context(), tenant, billValue(t, domain.NewCustomerChargeID, "charge-1"), finalBasis(t))
	if err != nil || result.Outcome() != application.ConfirmOnBasisConfirmAttempted {
		t.Fatalf("结果 = %s err = %v，want CONFIRM_ATTEMPTED", result.Outcome(), err)
	}
	if result.Confirmation().Outcome() != application.ChargeConfirmedOutcome || result.Retryable() {
		t.Fatalf("确认 = %s 重投 = %v，want CHARGE_CONFIRMED 不重投", result.Confirmation().Outcome(), result.Retryable())
	}
	want := "charge-1|fee-freight|EFFECTIVE_DELIVERY_FINALIZED|final-outcome/SYN-PARCEL-1/EFFECTIVE_DELIVERY/DRV-1"
	if len(fixture.bases.recorded) != 1 || fixture.bases.recorded[0] != want {
		t.Fatalf("依据册 = %v，want [%s]", fixture.bases.recorded, want)
	}
	if fixture.confirm.store.charges["charge-1"].Stage() != domain.ChargeConfirmed {
		t.Fatalf("费用阶段 = %s，want 已确认", fixture.confirm.store.charges["charge-1"].Stage())
	}
}

// Covers: AT-SA-218（ADR-0336 决定二与 ADR-0170 决定二）——没登确认时点，依据照记，确认不调；这是答案，不重投。
func TestABasisWithoutTheConfirmMomentIsOnlyRecorded(t *testing.T) {
	fixture := newConfirmOnBasisFixture(t, false)
	tenant := confirmCommand(t).TenantID

	result, err := fixture.handler.OnCharge(t.Context(), tenant, billValue(t, domain.NewCustomerChargeID, "charge-1"), finalBasis(t))
	if err != nil || result.Outcome() != application.ConfirmOnBasisRecorded || result.Retryable() {
		t.Fatalf("结果 = %s 重投 = %v err = %v，want BASIS_RECORDED 不重投", result.Outcome(), result.Retryable(), err)
	}
	if result.Moment() != application.SettlementMomentUnconfigured {
		t.Fatalf("时点 = %s，want 未配置", result.Moment())
	}
	if len(fixture.bases.recorded) != 1 || fixture.confirm.store.saves != 0 {
		t.Fatalf("依据 %v 确认写入 %d 次，want 记一份、不确认", fixture.bases.recorded, fixture.confirm.store.saves)
	}
}

// Covers: AT-SA-218（ADR-0336 决定二）——按包裹找当前那份 SELL 请求形成的费用：没有请求、请求没形成费用、费用不在册上都答「没有已
// 形成的费用」，什么都不记。
func TestNoFormedChargeRecordsNothing(t *testing.T) {
	fixture := newConfirmOnBasisFixture(t, true)
	tenant := confirmCommand(t).TenantID
	parcel := billValue(t, domain.NewParcelReference, "SYN-PARCEL-1")

	result, err := fixture.handler.OnParcel(t.Context(), tenant, parcel, finalBasis(t))
	if err != nil || result.Outcome() != application.ConfirmOnBasisNoFormedCharge {
		t.Fatalf("没有请求 = %s err = %v", result.Outcome(), err)
	}
	fixture.requests.found = true
	if result, _ := fixture.handler.OnParcel(t.Context(), tenant, parcel, finalBasis(t)); result.Outcome() != application.ConfirmOnBasisNoFormedCharge {
		t.Fatalf("请求没形成费用 = %s", result.Outcome())
	}
	fixture.requests.record.FormedCharge = billValue(t, domain.NewCustomerChargeID, "charge-absent")
	if result, _ := fixture.handler.OnParcel(t.Context(), tenant, parcel, finalBasis(t)); result.Outcome() != application.ConfirmOnBasisNoFormedCharge {
		t.Fatalf("费用不在册 = %s", result.Outcome())
	}
	if len(fixture.bases.recorded) != 0 {
		t.Fatalf("依据册 = %v，want 空", fixture.bases.recorded)
	}

	fixture.requests.record.FormedCharge = billValue(t, domain.NewCustomerChargeID, "charge-1")
	if result, _ := fixture.handler.OnParcel(t.Context(), tenant, parcel, finalBasis(t)); result.Outcome() != application.ConfirmOnBasisConfirmAttempted {
		t.Fatalf("请求形成了在册的费用 = %s，want CONFIRM_ATTEMPTED", result.Outcome())
	}
}

// Covers: AT-SA-218（ADR-0336 决定二与 ADR-0029）——自己的册读写不动、确认停在依赖上要重投；确认条件或确认事实没登是租户取值缺席，
// 不重投。
func TestOnlyDependencyFailuresAreRetryable(t *testing.T) {
	tenant := confirmCommand(t).TenantID
	charge := billValue(t, domain.NewCustomerChargeID, "charge-1")

	down := newConfirmOnBasisFixture(t, true)
	down.bases.err = errors.New("synthetic basis register down")
	if result, err := down.handler.OnCharge(t.Context(), tenant, charge, finalBasis(t)); err != nil ||
		result.Outcome() != application.ConfirmOnBasisUndecided || !result.Retryable() || result.UndecidedCause() == nil {
		t.Fatalf("依据册读写不动 = %s 重投 %v err %v", result.Outcome(), result.Retryable(), err)
	}

	momentDown := newConfirmOnBasisFixture(t, true)
	momentDown.moment.err = errors.New("synthetic moment register down")
	if result, _ := momentDown.handler.OnCharge(t.Context(), tenant, charge, finalBasis(t)); result.Outcome() != application.ConfirmOnBasisUndecided {
		t.Fatalf("时点册读不动 = %s，want 未决", result.Outcome())
	}

	unconfigured := newConfirmOnBasisFixture(t, true)
	unconfigured.confirm.conditions.configured = false
	result, _ := unconfigured.handler.OnCharge(t.Context(), tenant, charge, finalBasis(t))
	if result.Confirmation().UndecidedReason() != application.ConditionUnconfigured || result.Retryable() {
		t.Fatalf("条件未配置 = %s 重投 %v，want CONDITION_UNCONFIGURED 不重投", result.Confirmation().UndecidedReason(), result.Retryable())
	}

	conditionDown := newConfirmOnBasisFixture(t, true)
	conditionDown.confirm.conditions.err = errors.New("synthetic condition view down")
	if result, _ := conditionDown.handler.OnCharge(t.Context(), tenant, charge, finalBasis(t)); !result.Retryable() {
		t.Fatalf("条件目录读不动 = %s，want 重投", result.Confirmation().UndecidedReason())
	}
}

type accountByIDDouble struct {
	account domain.SettlementAccount
	found   bool
	err     error
}

func (double accountByIDDouble) FindByID(
	context.Context, domain.TenantID, domain.SettlementAccountID,
) (domain.SettlementAccount, bool, error) {
	return double.account, double.found, double.err
}

type factRegisterDouble struct {
	saved []domain.ChargeConfirmationFactRegistration
	err   error
}

func (double *factRegisterDouble) SaveChargeConfirmationFacts(
	_ context.Context, _ domain.TenantID, registration domain.ChargeConfirmationFactRegistration, _ time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	if double.err != nil {
		return 0, double.err
	}
	double.saved = append(double.saved, registration)
	return ports.CatalogueRegistered, nil
}

type effectiveFinalDouble struct {
	basis domain.ConfirmationBasisReference
	found bool
	err   error
}

func (double effectiveFinalDouble) FindEffectiveDeliveryFinal(
	context.Context, domain.TenantID, domain.ParcelReference,
) (domain.ConfirmationBasisReference, bool, error) {
	return double.basis, double.found, double.err
}

func followUpAccount(t *testing.T) domain.SettlementAccount {
	t.Helper()
	key, err := domain.NewSettlementAccountKey(
		billValue(t, domain.NewLegalEntityReference, "SYN-LE-1"),
		billValue(t, domain.NewSettlementCounterpartyReference, "SYN-SHIPPER-1"),
		domain.ChargeReceivable,
		billValue(t, domain.NewCurrencyCode, "USD"),
		billValue(t, domain.NewSettlementPolicyReference, "SYN-POLICY-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	account, err := domain.NewSettlementAccount(billValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1"), key,
		domain.SettlementCounterpartyReference{}, false, billValue(t, domain.NewResponsibilityBasis, "SYN-CONTRACT-1"))
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func followUpRequest(t *testing.T) domain.SellEvaluationRequest {
	t.Helper()
	request, err := domain.SubmitSellEvaluationRequest(domain.SellEvaluationRequestSpec{
		ID:                 billValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-1"),
		Parcel:             billValue(t, domain.NewParcelReference, "SYN-PARCEL-1"),
		Account:            billValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
		SettlementCurrency: billValue(t, domain.NewCurrencyCode, "USD"),
		PricingScope:       billValue(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"),
		Moment:             domain.BillingMomentNetworkIntakeAdopted,
		Source:             billValue(t, domain.NewSourceFactReference, "SYN-INTAKE-1"),
		PricingBasisAt:     chargeFormedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

type followUpFixture struct {
	basis    *confirmOnBasisFixture
	accounts *accountByIDDouble
	facts    *factRegisterDouble
	finals   *effectiveFinalDouble
	followUp *application.FormedChargeConfirmationFollowUp
}

func newFollowUpFixture(t *testing.T) *followUpFixture {
	t.Helper()
	fixture := &followUpFixture{
		basis:    newConfirmOnBasisFixture(t, true),
		accounts: &accountByIDDouble{account: followUpAccount(t), found: true},
		facts:    &factRegisterDouble{},
		finals:   &effectiveFinalDouble{},
	}
	followUp, err := application.NewFormedChargeConfirmationFollowUp(application.FormedChargeConfirmationDeps{
		Accounts:     accountsRef{fixture.accounts},
		Facts:        fixture.facts,
		Finals:       finalsRef{fixture.finals},
		Confirmation: fixture.basis.handler,
		Clock:        confirmClock{at: chargeConfirmedAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.followUp = followUp
	return fixture
}

type accountsRef struct{ double *accountByIDDouble }

func (ref accountsRef) FindByID(ctx context.Context, tenant domain.TenantID, id domain.SettlementAccountID) (domain.SettlementAccount, bool, error) {
	return ref.double.FindByID(ctx, tenant, id)
}

type finalsRef struct{ double *effectiveFinalDouble }

func (ref finalsRef) FindEffectiveDeliveryFinal(ctx context.Context, tenant domain.TenantID, parcel domain.ParcelReference) (domain.ConfirmationBasisReference, bool, error) {
	return ref.double.FindEffectiveDeliveryFinal(ctx, tenant, parcel)
}

// Covers: AT-SA-217（ADR-0336 决定一）——七项事实逐格出自请求与它指名的那一户：法人、相对方、方向取账户的固定属性，合同依据取账户
// 的成立依据，账户、计价范围、来源事实取请求。没有终局就到此为止，不记依据、不确认。
func TestTheFollowUpRegistersTheSevenFactsFromTheRequestAndItsAccount(t *testing.T) {
	fixture := newFollowUpFixture(t)
	tenant := confirmCommand(t).TenantID

	if err := fixture.followUp.AfterEstimatedChargeFormed(t.Context(), tenant, followUpRequest(t),
		billValue(t, domain.NewCustomerChargeID, "charge-1")); err != nil {
		t.Fatal(err)
	}
	if len(fixture.facts.saved) != 1 {
		t.Fatalf("确认事实册 = %d 行，want 1", len(fixture.facts.saved))
	}
	saved := fixture.facts.saved[0]
	facts := saved.Facts()
	got := []string{saved.Charge().String(), facts.ResponsibleEntity.String(), facts.Counterparty.String(),
		facts.Direction.String(), facts.SettlementAccount.String(), facts.ContractBasis.String(),
		facts.PrimaryChargingScope.String(), facts.SourceFact.String()}
	want := []string{"charge-1", "SYN-LE-1", "SYN-SHIPPER-1", domain.ChargeReceivable.String(), "SYN-ACCT-1",
		"SYN-CONTRACT-1", "SYN-SCOPE-SELL-1", "SYN-INTAKE-1"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("确认事实 = %v，want %v", got, want)
		}
	}
	if len(fixture.basis.bases.recorded) != 0 || fixture.basis.confirm.store.saves != 0 {
		t.Fatalf("没有终局也记了依据 %v 或确认了", fixture.basis.bases.recorded)
	}
}

// Covers: AT-SA-219（ADR-0336 决定二）——费用在终局之后才形成：形成那一笔回读到已终局的有效交付，补记依据并按时点确认。
func TestAChargeFormedAfterTheFinalOutcomeIsConfirmedOnFormation(t *testing.T) {
	fixture := newFollowUpFixture(t)
	fixture.finals.found = true
	fixture.finals.basis = finalBasis(t).Reference

	if err := fixture.followUp.AfterEstimatedChargeFormed(t.Context(), confirmCommand(t).TenantID, followUpRequest(t),
		billValue(t, domain.NewCustomerChargeID, "charge-1")); err != nil {
		t.Fatal(err)
	}
	if len(fixture.basis.bases.recorded) != 1 || fixture.basis.confirm.store.charges["charge-1"].Stage() != domain.ChargeConfirmed {
		t.Fatalf("依据 %v 阶段 %s，want 记一份并已确认", fixture.basis.bases.recorded,
			fixture.basis.confirm.store.charges["charge-1"].Stage())
	}
}

// Covers: AT-SA-217、AT-SA-219（ADR-0336 决定一、二）——请求指名的账户不在册上是矛盾；账户册、事实册、终局读口读写不动与确认停在依赖上都交回
// 错误，让外层整笔回滚重投。
func TestFollowUpFailuresAreReturned(t *testing.T) {
	tenant := confirmCommand(t).TenantID
	charge := billValue(t, domain.NewCustomerChargeID, "charge-1")

	missing := newFollowUpFixture(t)
	missing.accounts.found = false
	if err := missing.followUp.AfterEstimatedChargeFormed(t.Context(), tenant, followUpRequest(t), charge); !errors.Is(err, application.ErrSellRequestAccountMissing) {
		t.Fatalf("账户不在册 err = %v", err)
	}

	for name, arrange := range map[string]func(*followUpFixture){
		"账户册":  func(f *followUpFixture) { f.accounts.err = errors.New("synthetic down") },
		"事实册":  func(f *followUpFixture) { f.facts.err = errors.New("synthetic down") },
		"终局读口": func(f *followUpFixture) { f.finals.err = errors.New("synthetic down") },
		"确认依赖": func(f *followUpFixture) {
			f.finals.found, f.finals.basis = true, finalBasis(t).Reference
			f.basis.bases.err = errors.New("synthetic down")
		},
	} {
		fixture := newFollowUpFixture(t)
		arrange(fixture)
		if err := fixture.followUp.AfterEstimatedChargeFormed(t.Context(), tenant, followUpRequest(t), charge); err == nil {
			t.Fatalf("%s读写不动 err = nil", name)
		}
	}
}
