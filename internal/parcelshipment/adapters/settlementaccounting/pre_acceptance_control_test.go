package settlementaccounting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	controlAsOfAt = time.Date(2026, 6, 2, 10, 15, 0, 0, time.UTC)
	controlledAt  = time.Date(2026, 6, 2, 11, 0, 0, 0, time.UTC)
)

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// unconfigured 取反向默认，理由同 SA 应用层那个替身：既有用例给的都是已登记的策略。
type policyDouble struct {
	policy       sadomain.PreAcceptanceControlPolicy
	unconfigured bool
	err          error
	asked        int
}

func (double *policyDouble) LoadControlPolicy(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	double.asked++
	if double.err != nil {
		return sadomain.PreAcceptanceControlPolicy{}, false, double.err
	}
	if double.unconfigured {
		return sadomain.PreAcceptanceControlPolicy{}, false, nil
	}
	return double.policy, true, nil
}

type balanceDouble struct {
	balance sadomain.OperationalBalance
	err     error
}

func (double *balanceDouble) LoadBalance(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
) (sadomain.OperationalBalance, error) {
	if double.err != nil {
		return sadomain.OperationalBalance{}, double.err
	}
	return double.balance, nil
}

type ledgerDouble struct {
	ledger  *sadomain.FreezeLedger
	loadErr error
	saveErr error
}

func (double *ledgerDouble) LoadForScope(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
) (*sadomain.FreezeLedger, error) {
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	return double.ledger, nil
}

func (double *ledgerDouble) Save(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
	_ *sadomain.FreezeLedger,
) error {
	return double.saveErr
}

type creditDouble struct {
	standing sadomain.CreditStanding
	err      error
}

func (double *creditDouble) LoadCreditStanding(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
) (sadomain.CreditStanding, error) {
	if double.err != nil {
		return sadomain.CreditStanding{}, double.err
	}
	return double.standing, nil
}

type exposureLedgerDouble struct {
	ledger  *sadomain.CreditExposureLedger
	loadErr error
	saveErr error
}

func (double *exposureLedgerDouble) LoadForScope(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
) (*sadomain.CreditExposureLedger, error) {
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	return double.ledger, nil
}

func (double *exposureLedgerDouble) Save(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
	_ *sadomain.CreditExposureLedger,
) error {
	return double.saveErr
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

var (
	_ saports.PreAcceptanceControlPolicyView = (*policyDouble)(nil)
	_ saports.OperationalBalanceView         = (*balanceDouble)(nil)
	_ saports.FreezeLedgerRepository         = (*ledgerDouble)(nil)
	_ saports.CreditStandingView             = (*creditDouble)(nil)
	_ saports.CreditExposureLedgerRepository = (*exposureLedgerDouble)(nil)
)

type scopeSourceDouble struct {
	scope  sadomain.SettlementScope
	formed bool
	err    error
}

func (double *scopeSourceDouble) FormControlScope(
	_ context.Context,
	_ psdomain.SourceIdentity,
	_ psdomain.ShipmentRequestID,
	_ psdomain.SubmissionVersionID,
) (sadomain.SettlementScope, bool, error) {
	return double.scope, double.formed, double.err
}

type amountSourceDouble struct {
	amount int64
	formed bool
	err    error
}

func (double *amountSourceDouble) FormControlAmount(
	_ context.Context,
	_ psports.FinancialControlRequest,
) (int64, bool, error) {
	return double.amount, double.formed, double.err
}

type controlFixture struct {
	adapter *adapter.PreAcceptanceControlAdapter
	policy  *policyDouble
	balance *balanceDouble
	ledger  *ledgerDouble
	scopes  *scopeSourceDouble
	amounts *amountSourceDouble
}

func settlementScope(t *testing.T) sadomain.SettlementScope {
	t.Helper()
	scope, err := sadomain.NewSettlementScope(
		value(t, sadomain.NewLegalEntityReference, "legal-1"),
		value(t, sadomain.NewSettlementAccountID, "account-1"),
		value(t, sadomain.NewCurrencyCode, "CNY"),
	)
	if err != nil {
		t.Fatalf("new settlement scope: %v", err)
	}
	return scope
}

func newControlFixture(t *testing.T) *controlFixture {
	t.Helper()

	scope := settlementScope(t)
	policy, err := sadomain.NewRequiredControlPolicy(
		sadomain.PrepaidSettlement,
		value(t, sadomain.NewAdoptedPolicyReference, "PC-SETTLEMENT-POLICY-V3"),
	)
	if err != nil {
		t.Fatalf("new control policy: %v", err)
	}
	balance, err := sadomain.NewOperationalBalance(scope, 10_000, 0, 0, 0)
	if err != nil {
		t.Fatalf("new operational balance: %v", err)
	}

	fixture := &controlFixture{
		policy:  &policyDouble{policy: policy},
		balance: &balanceDouble{balance: balance},
		ledger:  &ledgerDouble{ledger: sadomain.NewFreezeLedger()},
		scopes:  &scopeSourceDouble{scope: scope, formed: true},
		amounts: &amountSourceDouble{amount: 4_000, formed: true},
	}
	exposures := &exposureLedgerDouble{ledger: sadomain.NewCreditExposureLedger()}
	fixture.adapter = adapter.NewPreAcceptanceControlAdapter(adapter.PreAcceptanceControlAdapterDeps{
		Apply: saapplication.NewApplyPreAcceptanceControlHandler(saapplication.ApplyPreAcceptanceControlDeps{
			Policy:    fixture.policy,
			Balance:   fixture.balance,
			Freezes:   fixture.ledger,
			Credit:    &creditDouble{},
			Exposures: exposures,
			Clock:     fixedClock{at: controlledAt},
		}),
		Release: saapplication.NewReleasePreAcceptanceControlHandler(
			fixture.ledger, exposures, fixedClock{at: controlledAt.Add(time.Hour)}),
		Scopes:  fixture.scopes,
		Amounts: fixture.amounts,
	})
	return fixture
}

func (fixture *controlFixture) request(t *testing.T) psports.FinancialControlRequest {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	echoed, err := psdomain.NewEchoedAsOfPolicy(
		psdomain.FinancialControlJudgmentKind,
		value(t, psdomain.NewAsOfSemanticsReference, "CONTROL_EVALUATION_AT"),
		value(t, psdomain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new echoed policy: %v", err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(controlAsOfAt, echoed)
	if err != nil {
		t.Fatalf("new judgment as-of: %v", err)
	}
	return psports.FinancialControlRequest{
		Identity:          identity,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		AsOf:              asOf,
	}
}

// expectedAsOf 是按提供方回显重建的时点——回显取自答复而非转手请求，两者在夹具里数值
// 相同（提供方回显的就是收到的），断言等值即钉住整条译回链。
func (fixture *controlFixture) expectedAsOf(t *testing.T) psdomain.JudgmentAsOf {
	t.Helper()
	echoed, err := psdomain.NewEchoedAsOfPolicy(
		psdomain.FinancialControlJudgmentKind,
		value(t, psdomain.NewAsOfSemanticsReference, "CONTROL_EVALUATION_AT"),
		value(t, psdomain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new echoed policy: %v", err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(controlAsOfAt, echoed)
	if err != nil {
		t.Fatalf("new judgment as-of: %v", err)
	}
	return asOf
}

// Covers: UC-SA-002 `AT-SA-044` 冻结半边经消费侧适配器成为 UC-PS-001 步骤 7 的控制结果——
// `已冻结`带结果标识；标识取控制请求身份（冻结标识是提供方账本的内部编号，不出上下文），
// 时点按提供方回显重建。
func TestAHeldFreezeBecomesAFormedHeldResult(t *testing.T) {
	fixture := newControlFixture(t)

	assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("apply pre-acceptance financial control: %v", err)
	}

	if assessment.Outcome != psports.PreAcceptanceControlFormed {
		t.Fatalf("outcome = %q reason = %q, want FORMED", assessment.Outcome, assessment.Reason)
	}
	if assessment.Result.Outcome() != psdomain.FinancialControlHeld {
		t.Fatalf("control outcome = %q, want HELD", assessment.Result.Outcome())
	}
	if assessment.Result.ResultID().String() != "request-1/version-1" {
		t.Fatalf("result ID = %q, want the control request identity", assessment.Result.ResultID())
	}
	if assessment.Result.AsOf() != fixture.expectedAsOf(t) {
		t.Fatalf("as-of = %#v, want the provider echo", assessment.Result.AsOf())
	}
}

// Covers: `AT-PS-035` 依赖的「业务限制」一格——余额不足是已执行的控制，带限制依据与结果
// 标识译回。`业务限制`在提供方没有冻结标识，请求身份作标识让这一格不再缺标识。
func TestARestrictedFreezeCarriesItsLimitBasisAndAnIdentity(t *testing.T) {
	fixture := newControlFixture(t)
	fixture.amounts.amount = 50_000

	assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("apply pre-acceptance financial control: %v", err)
	}

	if assessment.Outcome != psports.PreAcceptanceControlFormed {
		t.Fatalf("outcome = %q, want FORMED——业务限制同样是一次已执行的控制", assessment.Outcome)
	}
	if assessment.Result.Outcome() != psdomain.FinancialControlRestricted {
		t.Fatalf("control outcome = %q, want RESTRICTED", assessment.Result.Outcome())
	}
	if assessment.Result.Basis().String() != "AVAILABLE_BALANCE_INSUFFICIENT" {
		t.Fatalf("basis = %q, want the provider's restriction reason", assessment.Result.Basis())
	}
	if assessment.Result.ResultID().String() == "" {
		t.Fatal("业务限制没带结果标识——释放与追溯都无从按它认领")
	}
}

// Covers: UC-SA-002 `AT-SA-049`「合同明确无控制 → 保存不适用依据，不造零额冻结」经适配器
// 译回 `明确无控制`——依据随行、不带结果标识（那一支下提供方不形成冻结）。
func TestAnExplicitNoControlCarriesItsCommercialBasis(t *testing.T) {
	fixture := newControlFixture(t)
	policy, err := sadomain.NewNoControlPolicy(
		value(t, sadomain.NewControlBasisReference, "PC-NO-CONTROL-BASIS-1"),
	)
	if err != nil {
		t.Fatalf("new control policy: %v", err)
	}
	fixture.policy.policy = policy

	assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("apply pre-acceptance financial control: %v", err)
	}

	if assessment.Outcome != psports.PreAcceptanceControlFormed {
		t.Fatalf("outcome = %q, want FORMED", assessment.Outcome)
	}
	if assessment.Result.Outcome() != psdomain.FinancialControlNotApplicable {
		t.Fatalf("control outcome = %q, want NOT_APPLICABLE", assessment.Result.Outcome())
	}
	if assessment.Result.Basis().String() != "PC-NO-CONTROL-BASIS-1" {
		t.Fatalf("basis = %q, want the commercial inapplicability basis", assessment.Result.Basis())
	}
}

// termsFixture 把夹具切到账期分支：TERMS 政策 + 信用状况，预付那本账留空。
func termsFixture(t *testing.T, standing sadomain.CreditStanding) *controlFixture {
	t.Helper()
	fixture := newControlFixture(t)
	policy, err := sadomain.NewRequiredControlPolicy(
		sadomain.TermsSettlement,
		value(t, sadomain.NewAdoptedPolicyReference, "PC-SETTLEMENT-POLICY-V3"),
	)
	if err != nil {
		t.Fatalf("new terms policy: %v", err)
	}
	fixture.policy.policy = policy
	exposures := &exposureLedgerDouble{ledger: sadomain.NewCreditExposureLedger()}
	fixture.adapter = adapter.NewPreAcceptanceControlAdapter(adapter.PreAcceptanceControlAdapterDeps{
		Apply: saapplication.NewApplyPreAcceptanceControlHandler(saapplication.ApplyPreAcceptanceControlDeps{
			Policy:    fixture.policy,
			Balance:   fixture.balance,
			Freezes:   fixture.ledger,
			Credit:    &creditDouble{standing: standing},
			Exposures: exposures,
			Clock:     fixedClock{at: controlledAt},
		}),
		Release: saapplication.NewReleasePreAcceptanceControlHandler(
			fixture.ledger, exposures, fixedClock{at: controlledAt.Add(time.Hour)}),
		Scopes:  fixture.scopes,
		Amounts: fixture.amounts,
	})
	return fixture
}

// Covers: ADR-0047 决策三经适配器落地——账期控制通过译成 `CREDIT_EXPOSED` 而不冒用
// `HELD`（没有资金被冻结），结果标识同一来源可供释放认领；超额译成带原因的 `RESTRICTED`，
// 与预付分支同格。
func TestATermsControlTranslatesToCreditExposedNotHeld(t *testing.T) {
	scope := settlementScope(t)
	standing, err := sadomain.NewCreditStanding(scope, 10_000, 0, false)
	if err != nil {
		t.Fatalf("new credit standing: %v", err)
	}
	fixture := termsFixture(t, standing)

	assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("apply pre-acceptance financial control: %v", err)
	}

	if assessment.Outcome != psports.PreAcceptanceControlFormed {
		t.Fatalf("outcome = %q, want FORMED", assessment.Outcome)
	}
	if assessment.Result.Outcome() != psdomain.FinancialControlCreditExposed {
		t.Fatalf("result = %q, want CREDIT_EXPOSED——账期通过不是一笔冻结", assessment.Result.Outcome())
	}
	if assessment.Result.ResultID().String() != "request-1/version-1" {
		t.Fatalf("result ID = %q; 结果标识必须与释放认领同一来源", assessment.Result.ResultID())
	}

	restrictedStanding, err := sadomain.NewCreditStanding(scope, 1_000, 0, false)
	if err != nil {
		t.Fatalf("new restricted standing: %v", err)
	}
	restricted := termsFixture(t, restrictedStanding)
	answer, err := restricted.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), restricted.request(t))
	if err != nil {
		t.Fatalf("apply restricted control: %v", err)
	}
	if answer.Result.Outcome() != psdomain.FinancialControlRestricted ||
		answer.Result.Basis().String() != "AVAILABLE_CREDIT_INSUFFICIENT" {
		t.Fatalf("restricted = %q/%q; 超额是带原因的业务限制", answer.Result.Outcome(), answer.Result.Basis())
	}
}

// Covers: 实例半边纪律——作用域与金额未配置停在`未形成`且不问提供方：作用域该来自结算
// 政策（ADR-0044）、金额该来自估价，代拟任何一个都是拿别人的钱做实验。
func TestUnconfiguredSourcesStopAtNotFormedWithoutAskingTheProvider(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*controlFixture) *adapter.PreAcceptanceControlAdapter
		wantReason string
	}{
		"no scope source": {
			arrange: func(fixture *controlFixture) *adapter.PreAcceptanceControlAdapter {
				return adapter.NewPreAcceptanceControlAdapter(adapter.PreAcceptanceControlAdapterDeps{
					Apply: saapplication.NewApplyPreAcceptanceControlHandler(saapplication.ApplyPreAcceptanceControlDeps{
						Policy:    fixture.policy,
						Balance:   fixture.balance,
						Freezes:   fixture.ledger,
						Credit:    &creditDouble{},
						Exposures: &exposureLedgerDouble{ledger: sadomain.NewCreditExposureLedger()},
						Clock:     fixedClock{at: controlledAt},
					}),
					Amounts: fixture.amounts,
				})
			},
			wantReason: "CONTROL_SCOPE_NOT_CONFIGURED",
		},
		"scope source cannot form": {
			arrange: func(fixture *controlFixture) *adapter.PreAcceptanceControlAdapter {
				fixture.scopes.formed = false
				return fixture.adapter
			},
			wantReason: "CONTROL_SCOPE_NOT_CONFIGURED",
		},
		"amount source cannot form": {
			arrange: func(fixture *controlFixture) *adapter.PreAcceptanceControlAdapter {
				fixture.amounts.formed = false
				return fixture.adapter
			},
			wantReason: "CONTROL_AMOUNT_NOT_CONFIGURED",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newControlFixture(t)
			subject := testCase.arrange(fixture)

			assessment, err := subject.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
			if err != nil {
				t.Fatalf("apply pre-acceptance financial control: %v", err)
			}

			if assessment.Outcome != psports.PreAcceptanceControlNotFormed {
				t.Fatalf("outcome = %q, want NOT_FORMED", assessment.Outcome)
			}
			if assessment.Reason.String() != testCase.wantReason {
				t.Fatalf("reason = %q, want %q", assessment.Reason, testCase.wantReason)
			}
			if fixture.policy.asked != 0 {
				t.Fatal("实例半边未配置仍去问了提供方")
			}
		})
	}
}

// Covers: ADR-0025「翻译必须是全函数」在控制一侧——冲突/未受理/未形成各落各格，未形成的
// 封闭原因冠 SA- 前缀原样带回。
func TestEveryProviderAnswerLandsOnItsOwnConsumerValue(t *testing.T) {
	t.Run("conflict", func(t *testing.T) {
		fixture := newControlFixture(t)
		if _, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t)); err != nil {
			t.Fatalf("first apply: %v", err)
		}
		fixture.amounts.amount = 5_000

		assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
		if err != nil {
			t.Fatalf("second apply: %v", err)
		}
		if assessment.Outcome != psports.PreAcceptanceControlRequestConflict {
			t.Fatalf("outcome = %q, want REQUEST_CONFLICT", assessment.Outcome)
		}
	})

	t.Run("not accepted", func(t *testing.T) {
		fixture := newControlFixture(t)
		fixture.amounts.amount = 0

		assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if assessment.Outcome != psports.PreAcceptanceControlRequestNotAccepted {
			t.Fatalf("outcome = %q, want REQUEST_NOT_ACCEPTED", assessment.Outcome)
		}
	})

	t.Run("not formed carries the provider reason", func(t *testing.T) {
		fixture := newControlFixture(t)
		fixture.policy.err = errors.New("policy view unavailable")

		assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		if assessment.Outcome != psports.PreAcceptanceControlNotFormed {
			t.Fatalf("outcome = %q, want NOT_FORMED", assessment.Outcome)
		}
		if assessment.Reason.String() != "SA-CONTROL_POLICY_UNAVAILABLE" {
			t.Fatalf("reason = %q, want SA-CONTROL_POLICY_UNAVAILABLE", assessment.Reason)
		}
	})
}

// Covers: UC-PS-005/UC-PS-001 的释放补偿经适配器按原关联认领——本上下文记下的结果标识
// 译回提供方的控制请求身份，原冻结被释放；`无可释放`不是失败，补偿据此收口（AT-PS-074
// 的 SA 半边）；读不回是可重试错误，补偿续办留在决定那一侧。
func TestAReleaseByTheRecordedResultIdentityFreesTheOriginalFreeze(t *testing.T) {
	fixture := newControlFixture(t)
	assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	request := fixture.request(t)
	release := psports.ControlReleaseRequest{
		Identity:          request.Identity,
		ShipmentRequestID: request.ShipmentRequestID,
		SubmissionVersion: request.SubmissionVersion,
		ControlResultID:   assessment.Result.ResultID(),
	}
	if err := fixture.adapter.ReleasePreAcceptanceControl(context.Background(), release); err != nil {
		t.Fatalf("release pre-acceptance control: %v", err)
	}

	requestID := value(t, sadomain.NewControlRequestID, "request-1/version-1")
	freeze, found := fixture.ledger.ledger.FindByRequest(requestID)
	if !found || freeze.Status() != sadomain.FreezeReleased {
		t.Fatalf("freeze = %#v found = %v, want the original freeze RELEASED", freeze, found)
	}

	t.Run("nothing to release is not an error", func(t *testing.T) {
		fresh := newControlFixture(t)
		if err := fresh.adapter.ReleasePreAcceptanceControl(context.Background(), release); err != nil {
			t.Fatalf("release with nothing frozen: %v——对着一笔不存在的占用报错，补偿会无休止重试", err)
		}
	})

	t.Run("an unreachable ledger keeps the release retryable", func(t *testing.T) {
		broken := newControlFixture(t)
		broken.ledger.loadErr = errors.New("ledger unavailable")
		if err := broken.adapter.ReleasePreAcceptanceControl(context.Background(), release); err == nil {
			t.Fatal("账本读不回却答成功，补偿会按一笔其实还占着的资金收口")
		}
	})
}
