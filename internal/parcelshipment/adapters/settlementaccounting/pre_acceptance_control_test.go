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
	// askedWith 记下最后一次被问到的商业解析回指：本适配器铸的命令带没带对回指，
	// 只有到这一层才看得见。
	askedWith sadomain.CommercialResolutionReference
}

func (double *policyDouble) LoadControlPolicy(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
	resolution sadomain.CommercialResolutionReference,
) (sadomain.PreAcceptanceControlPolicy, bool, error) {
	double.asked++
	double.askedWith = resolution
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

// creditBasisDouble 替商业侧的授信依据口（ADR-0127 决定四），照 policyDouble 的写法三格可配：
// 零值答「闭包采用了信用政策、带 basis」，unconfigured 答未配置，err 答调不通。它进本夹具是为 SA
// contract 段铺路——那一段要把 `Deps.CreditBasis` 收成 mandatory，而本文件是全仓唯一一处在 SA 之外
// 直接构造那份 Deps 的地方。
type creditBasisDouble struct {
	basis        sadomain.CreditBasis
	unconfigured bool
	err          error
}

func (double *creditBasisDouble) LoadCreditBasis(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
	_ sadomain.CommercialResolutionReference,
) (sadomain.CreditBasis, bool, error) {
	if double.err != nil {
		return sadomain.CreditBasis{}, false, double.err
	}
	if double.unconfigured {
		return sadomain.CreditBasis{}, false, nil
	}
	return double.basis, true, nil
}

// authorizedBasis 造一份金额额度的授信依据：额度出自哪一版信用政策由字面量固定，金额由用例给。
func authorizedBasis(t *testing.T, amountMinor int64) *creditBasisDouble {
	t.Helper()
	basis, err := sadomain.NewCreditAmountBasis(
		value(t, sadomain.NewCreditPolicyReference, "PC-CREDIT-POLICY/v1"), amountMinor)
	if err != nil {
		t.Fatalf("new credit amount basis: %v", err)
	}
	return &creditBasisDouble{basis: basis}
}

// ratioBaseDouble 替 SA 基数取值那一口（ADR-0129 决定三）。本夹具只给金额额度，这一口不会被问到；它在这里
// 只为装配齐全——`Deps.RatioBases` 与其余依赖同一道构造门，缺了门就拒。
type ratioBaseDouble struct{}

func (ratioBaseDouble) LoadCreditRatioBase(
	_ context.Context,
	_ sadomain.TenantID,
	_ sadomain.SettlementScope,
	_ sadomain.CreditRatioBase,
) (int64, bool, error) {
	return 0, false, nil
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
	_ saports.CreditBasisView                = (*creditBasisDouble)(nil)
	_ saports.CreditExposureLedgerRepository = (*exposureLedgerDouble)(nil)
)

type scopeSourceDouble struct {
	scope  adapter.ControlScope
	formed bool
	err    error
}

func (double *scopeSourceDouble) FormControlScope(
	_ context.Context,
	_ psdomain.SourceIdentity,
	_ psdomain.ShipmentRequestID,
	_ psdomain.SubmissionVersionID,
) (adapter.ControlScope, bool, error) {
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

// requiredPolicy 造一份`要求`控制的策略答复（ADR-0122 形状）：控制项按给定种类各一项、按给定次序
// 排，共同通过条件取首发唯一值；结算方式与两份采用依据按夹具固定。
func requiredPolicy(t *testing.T, method sadomain.SettlementMethod, kinds ...sadomain.ControlKind) sadomain.PreAcceptanceControlPolicy {
	t.Helper()
	items := make([]sadomain.ControlItem, 0, len(kinds))
	for index, kind := range kinds {
		item, err := sadomain.NewControlItem(kind, uint32(index+1))
		if err != nil {
			t.Fatalf("new control item: %v", err)
		}
		items = append(items, item)
	}
	policy, err := sadomain.NewRequiredControlPolicy(
		items,
		sadomain.AllControlsPass,
		value(t, sadomain.NewControlPolicyReference, "PC-CONTROL-POLICY/v1"),
		method,
		value(t, sadomain.NewAdoptedPolicyReference, "PC-SETTLEMENT-POLICY-V3"),
	)
	if err != nil {
		t.Fatalf("new required control policy: %v", err)
	}
	return policy
}

// controlScope 把资金作用域与商业解析回指装成 ControlScopeSource 交回的那一件。回指用
// 与 scope_source_test 同一个字面量，两处夹具因此说的是同一次解析。
func controlScope(t *testing.T, settlement sadomain.SettlementScope) adapter.ControlScope {
	t.Helper()
	return adapter.ControlScope{
		Settlement: settlement,
		Resolution: value(t, sadomain.NewCommercialResolutionReference, "RES-1"),
	}
}

func newControlFixture(t *testing.T) *controlFixture {
	t.Helper()

	scope := settlementScope(t)
	policy := requiredPolicy(t, sadomain.PrepaidSettlement, sadomain.PrepaidFreezeControl)
	balance, err := sadomain.NewOperationalBalance(scope, 10_000, 0, 0, 0)
	if err != nil {
		t.Fatalf("new operational balance: %v", err)
	}

	fixture := &controlFixture{
		policy:  &policyDouble{policy: policy},
		balance: &balanceDouble{balance: balance},
		ledger:  &ledgerDouble{ledger: sadomain.NewFreezeLedger()},
		scopes:  &scopeSourceDouble{scope: controlScope(t, scope), formed: true},
		amounts: &amountSourceDouble{amount: 4_000, formed: true},
	}
	exposures := &exposureLedgerDouble{ledger: sadomain.NewCreditExposureLedger()}
	fixture.adapter = adapter.NewPreAcceptanceControlAdapter(adapter.PreAcceptanceControlAdapterDeps{
		Apply: mustApplyHandler(t, saapplication.ApplyPreAcceptanceControlDeps{
			Policy:  fixture.policy,
			Balance: fixture.balance,
			Freezes: fixture.ledger,
			// 预付路不读信用；两份信用依赖给零值 / 任意额度只为装配齐全，不是本夹具要证的东西。
			Credit:      &creditDouble{},
			CreditBasis: authorizedBasis(t, 10_000),
			RatioBases:  ratioBaseDouble{},
			Exposures:   exposures,
			Clock:       fixedClock{at: controlledAt},
		}),
		Release: saapplication.NewReleasePreAcceptanceControlHandler(
			fixture.ledger, exposures, fixedClock{at: controlledAt.Add(time.Hour)}),
		Scopes:  fixture.scopes,
		Amounts: fixture.amounts,
	})
	return fixture
}

// mustApplyHandler 走 SA 的构造门。本夹具证的是适配器的译法，不是那道门；门拒了就是夹具没配齐。
func mustApplyHandler(t *testing.T, deps saapplication.ApplyPreAcceptanceControlDeps) *saapplication.ApplyPreAcceptanceControlHandler {
	t.Helper()
	handler, err := saapplication.NewApplyPreAcceptanceControlHandler(deps)
	if err != nil {
		t.Fatalf("new apply pre-acceptance control handler: %v", err)
	}
	return handler
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

// Covers: sa-preacceptance-policy-view/01 —— 本适配器铸的命令带上作用域源交回的那个商业
// 解析回指，一路到达提供方的控制策略视图。
//
// 断言落在视图被问到的值上而不是命令字段上：命令是本包内部的中间物，钉住它只证明字段被
// 赋过值；钉住视图收到什么，才证明这条回指真的走完了「作用域源 → 命令 → 编排 → 视图」
// 全程。中途任何一段丢了它，提供方就会拿不到键，而那一格已由 SA 编排答`未受理`。
func TestTheAdapterCarriesTheResolutionEchoThroughToThePolicyView(t *testing.T) {
	fixture := newControlFixture(t)
	fixture.scopes.scope.Resolution = value(t, sadomain.NewCommercialResolutionReference, "RES-ECHOED-2")

	if _, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(
		context.Background(), fixture.request(t)); err != nil {
		t.Fatalf("apply pre-acceptance financial control: %v", err)
	}

	if fixture.policy.askedWith.String() != "RES-ECHOED-2" {
		t.Fatalf("视图被问到的回指 = %q, want 作用域源交回的 RES-ECHOED-2", fixture.policy.askedWith)
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

// termsFixture 把夹具切到账期分支：TERMS 政策 + 信用状况，预付那本账留空。授信依据的额度取状况里
// 登记的那一格同值：额度自 ADR-0127 起出自政策而不出自登记状况，两处给同一个数，用例读起来仍是
// 「额度 10_000 / 1_000」，钉的是译回，不是额度来源——那是 SA 应用层测试的事。
func termsFixture(t *testing.T, standing sadomain.CreditStanding) *controlFixture {
	t.Helper()
	fixture := newControlFixture(t)
	fixture.policy.policy = requiredPolicy(t, sadomain.TermsSettlement, sadomain.CreditCheckControl)
	exposures := &exposureLedgerDouble{ledger: sadomain.NewCreditExposureLedger()}
	fixture.adapter = adapter.NewPreAcceptanceControlAdapter(adapter.PreAcceptanceControlAdapterDeps{
		Apply: mustApplyHandler(t, saapplication.ApplyPreAcceptanceControlDeps{
			Policy:      fixture.policy,
			Balance:     fixture.balance,
			Freezes:     fixture.ledger,
			Credit:      &creditDouble{standing: standing},
			CreditBasis: authorizedBasis(t, standing.LimitMinor()),
			RatioBases:  ratioBaseDouble{},
			Exposures:   exposures,
			Clock:       fixedClock{at: controlledAt},
		}),
		Release: saapplication.NewReleasePreAcceptanceControlHandler(
			fixture.ledger, exposures, fixedClock{at: controlledAt.Add(time.Hour)}),
		Scopes:  fixture.scopes,
		Amounts: fixture.amounts,
	})
	return fixture
}

// combinedFixture 把夹具切到组合策略：同一份策略先预付冻结、后信用校验（ADR-0115 允许、ADR-0122
// 执行的组合），两本账都真在。
func combinedFixture(t *testing.T, standing sadomain.CreditStanding) *controlFixture {
	t.Helper()
	fixture := termsFixture(t, standing)
	fixture.policy.policy = requiredPolicy(t, sadomain.TermsSettlement,
		sadomain.PrepaidFreezeControl, sadomain.CreditCheckControl)
	return fixture
}

// Covers: ADR-0125 决定三——适配器把 SA 的 `ExecutedControls` 与 `JointPassCondition` 全函数译成本上下文
// 的逐项结果与条件，结论由领域按条件推出：组合策略两项都成立时 `HELD`（暴露留在提供方账本由同一身份
// 释放），逐项两项都在、按判断顺序排；排在后面的一项形成`业务限制`时 `RESTRICTED`、依据是那一项自己
// 的原因，而第一项占下的资金仍算形成了占用（`OccupationFormed`），不因结论受限而消失。
func TestACombinedControlCarriesEveryItemAndTheJointPassCondition(t *testing.T) {
	scope := settlementScope(t)
	roomy, err := sadomain.NewCreditStanding(scope, 10_000, 0, false)
	if err != nil {
		t.Fatalf("new credit standing: %v", err)
	}
	fixture := combinedFixture(t, roomy)
	assessment, err := fixture.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("apply pre-acceptance financial control: %v", err)
	}
	if assessment.Outcome != psports.PreAcceptanceControlFormed ||
		assessment.Result.Outcome() != psdomain.FinancialControlHeld {
		t.Fatalf("outcome = %q / %q, want FORMED / HELD", assessment.Outcome, assessment.Result.Outcome())
	}
	if assessment.Result.JointPassCondition() != psdomain.AllControlsPass {
		t.Fatalf("joint pass condition = %q, want ALL_CONTROLS_PASS", assessment.Result.JointPassCondition())
	}
	items := assessment.Result.Items()
	if len(items) != 2 ||
		items[0].Kind() != psdomain.PrepaidFreezeControlItem || items[0].Order() != 1 || !items[0].Satisfied() ||
		items[1].Kind() != psdomain.CreditCheckControlItem || items[1].Order() != 2 || !items[1].Satisfied() {
		t.Fatalf("items = %+v; 两项都成立时逐项两项都要在、按判断顺序排", items)
	}

	tight, err := sadomain.NewCreditStanding(scope, 1_000, 0, false)
	if err != nil {
		t.Fatalf("new credit standing: %v", err)
	}
	restricted := combinedFixture(t, tight)
	assessment, err = restricted.adapter.ApplyPreAcceptanceFinancialControl(context.Background(), restricted.request(t))
	if err != nil {
		t.Fatalf("apply pre-acceptance financial control: %v", err)
	}
	if assessment.Result.Outcome() != psdomain.FinancialControlRestricted {
		t.Fatalf("control outcome = %q, want RESTRICTED——第一项冻结成立不能放行第二项的超额", assessment.Result.Outcome())
	}
	if assessment.Result.Basis().String() != "AVAILABLE_CREDIT_INSUFFICIENT" {
		t.Fatalf("reason = %q, want the restricting item's own reason", assessment.Result.Basis())
	}
	items = assessment.Result.Items()
	if len(items) != 2 || !items[0].Satisfied() || items[1].Satisfied() ||
		items[1].Basis().String() != "AVAILABLE_CREDIT_INSUFFICIENT" {
		t.Fatalf("items = %+v; 第一项成立、第二项带原因受限，两项都要原样在场", items)
	}
	if !assessment.Result.OccupationFormed() {
		t.Fatal("结论受限就报没有占用——第一项占下的 4000 会成孤儿")
	}
	if restricted.ledger.ledger.HeldMinor() != 4_000 {
		t.Fatalf("held = %d; 第一项占下的资金留在账本上等释放，不因第二项受限而消失", restricted.ledger.ledger.HeldMinor())
	}
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
					Apply: mustApplyHandler(t, saapplication.ApplyPreAcceptanceControlDeps{
						Policy:      fixture.policy,
						Balance:     fixture.balance,
						Freezes:     fixture.ledger,
						Credit:      &creditDouble{},
						CreditBasis: authorizedBasis(t, 10_000),
						RatioBases:  ratioBaseDouble{},
						Exposures:   &exposureLedgerDouble{ledger: sadomain.NewCreditExposureLedger()},
						Clock:       fixedClock{at: controlledAt},
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
