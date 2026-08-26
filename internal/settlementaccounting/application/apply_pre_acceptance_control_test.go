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

var (
	asOfAt      = time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	controlAt   = time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC)
	freezeScope = struct{ legalEntity, account, currency string }{"legal-1", "account-1", "CNY"}
)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// unconfigured 取反向默认：既有用例给的都是已登记的策略，零值应当继续表示「登记了」。
// 正向的 configured 会让每个既有构造点都要补一句 true，而漏补的那个会静默变成未配置。
type policyDouble struct {
	policy       domain.PreAcceptanceControlPolicy
	unconfigured bool
	err          error
	asked        int
	// askedWith 记下最后一次被问到的商业解析回指，供「编排把回指原样透传」那条用例取证。
	askedWith domain.CommercialResolutionReference
}

func (double *policyDouble) LoadControlPolicy(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SettlementScope,
	resolution domain.CommercialResolutionReference,
) (domain.PreAcceptanceControlPolicy, bool, error) {
	double.asked++
	double.askedWith = resolution
	if double.err != nil {
		return domain.PreAcceptanceControlPolicy{}, false, double.err
	}
	if double.unconfigured {
		return domain.PreAcceptanceControlPolicy{}, false, nil
	}
	return double.policy, true, nil
}

// Covers: ADR-0054 — 控制策略未登记不得落成`无控制`。两者的恢复动作相反：未配置等
// 商业侧登记 PAR-COM-15，`无控制`是合同已经说过的终局答案，可以据以放行接受判断。
// 混成一格，一个没人登记过的范围会拿到一份「合同说不用控制」的结论，而那正是 CONTEXT
// 禁止的默认信用通过。
func TestAnUnconfiguredControlPolicyIsNotFormedNotNoControl(t *testing.T) {
	balance := &balanceDouble{}
	handler := newHandler(&policyDouble{unconfigured: true}, balance, &ledgerDouble{})

	result, err := handler.Handle(context.Background(), command(t, 1_000))
	if err != nil {
		t.Fatalf("控制：%v", err)
	}
	if result.Outcome() != application.ControlNotFormed ||
		result.NotFormedReason() != application.ControlPolicyNotConfigured {
		t.Fatalf("result = %q/%q, want NOT_FORMED/CONTROL_POLICY_NOT_CONFIGURED",
			result.Outcome(), result.NotFormedReason())
	}
	if result.ControlBasis().String() != "" {
		t.Fatal("未配置却带出了一份商业不适用依据")
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("未配置没有留下可续办引用——它要等的是登记，不是重试到死")
	}
	if balance.loaded != 0 {
		t.Fatal("策略未配置却已经去读了这个客户的余额")
	}
}

// Covers: 未配置与调不通分成两格。恢复动作不同：一个去催登记，一个去重试依赖。
func TestAnUnconfiguredPolicyAndAnUnavailableOneDoNotShareAReason(t *testing.T) {
	if application.ControlPolicyNotConfigured == application.ControlPolicyUnavailable {
		t.Fatal("未配置与调不通共用一个原因，运维读不出该催登记还是该重试")
	}
}

type balanceDouble struct {
	balance domain.OperationalBalance
	err     error
	loaded  int
}

func (double *balanceDouble) LoadBalance(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SettlementScope,
) (domain.OperationalBalance, error) {
	double.loaded++
	if double.err != nil {
		return domain.OperationalBalance{}, double.err
	}
	return double.balance, nil
}

type ledgerDouble struct {
	ledger    *domain.FreezeLedger
	loadErr   error
	saveErr   error
	loaded    int
	saved     int
	askTenant domain.TenantID
}

func (double *ledgerDouble) LoadForScope(
	_ context.Context,
	tenant domain.TenantID,
	_ domain.SettlementScope,
) (*domain.FreezeLedger, error) {
	double.loaded++
	double.askTenant = tenant
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	return double.ledger, nil
}

func (double *ledgerDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SettlementScope,
	_ *domain.FreezeLedger,
) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.saved++
	return nil
}

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func settlementScope(t *testing.T) domain.SettlementScope {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		value(t, domain.NewLegalEntityReference, freezeScope.legalEntity),
		value(t, domain.NewSettlementAccountID, freezeScope.account),
		value(t, domain.NewCurrencyCode, freezeScope.currency),
	)
	if err != nil {
		t.Fatalf("new settlement scope: %v", err)
	}
	return scope
}

func balanceWith(t *testing.T, availableMinor int64) domain.OperationalBalance {
	t.Helper()
	balance, err := domain.NewOperationalBalance(settlementScope(t), availableMinor, 0, 0, 0)
	if err != nil {
		t.Fatalf("new operational balance: %v", err)
	}
	return balance
}

func requiredPolicy(t *testing.T) *policyDouble {
	t.Helper()
	return &policyDouble{policy: methodPolicy(t, domain.PrepaidSettlement)}
}

func methodPolicy(t *testing.T, method domain.SettlementMethod) domain.PreAcceptanceControlPolicy {
	t.Helper()
	policy, err := domain.NewRequiredControlPolicy(
		method,
		value(t, domain.NewAdoptedPolicyReference, "PC-SETTLEMENT-POLICY-V3"),
	)
	if err != nil {
		t.Fatalf("new required control policy: %v", err)
	}
	return policy
}

type creditDouble struct {
	standing domain.CreditStanding
	err      error
	loaded   int
}

func (double *creditDouble) LoadCreditStanding(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SettlementScope,
) (domain.CreditStanding, error) {
	double.loaded++
	if double.err != nil {
		return domain.CreditStanding{}, double.err
	}
	return double.standing, nil
}

type exposureLedgerDouble struct {
	ledger  *domain.CreditExposureLedger
	loadErr error
	saveErr error
	loaded  int
	saved   int
}

func (double *exposureLedgerDouble) LoadForScope(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SettlementScope,
) (*domain.CreditExposureLedger, error) {
	double.loaded++
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	return double.ledger, nil
}

func (double *exposureLedgerDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SettlementScope,
	_ *domain.CreditExposureLedger,
) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.saved++
	return nil
}

func standingWith(t *testing.T, limitMinor, exposedMinor int64, overdue bool) domain.CreditStanding {
	t.Helper()
	standing, err := domain.NewCreditStanding(settlementScope(t), limitMinor, exposedMinor, overdue)
	if err != nil {
		t.Fatalf("new credit standing: %v", err)
	}
	return standing
}

func command(t *testing.T, amountMinor int64) application.ApplyPreAcceptanceControlCommand {
	t.Helper()
	asOf, err := domain.NewControlAsOf(
		value(t, domain.NewAsOfSemantic, "CONTROL_EVALUATION_AT"),
		asOfAt,
		value(t, domain.NewAsOfStrategyVersion, "control-strategy-v1"),
	)
	if err != nil {
		t.Fatalf("new control asOf: %v", err)
	}
	return application.ApplyPreAcceptanceControlCommand{
		TenantID:    value(t, domain.NewTenantID, "tenant-1"),
		RequestID:   value(t, domain.NewControlRequestID, "control-request-1"),
		Scope:       settlementScope(t),
		AmountMinor: amountMinor,
		Association: value(t, domain.NewBusinessAssociationReference, "submission-1"),
		AsOf:        asOf,
		Resolution:  value(t, domain.NewCommercialResolutionReference, "RES-1"),
	}
}

func newHandler(
	policy ports.PreAcceptanceControlPolicyView,
	balance ports.OperationalBalanceView,
	ledger ports.FreezeLedgerRepository,
) *application.ApplyPreAcceptanceControlHandler {
	return application.NewApplyPreAcceptanceControlHandler(application.ApplyPreAcceptanceControlDeps{
		Policy:  policy,
		Balance: balance,
		Freezes: ledger,
		// 预付路径不读信用；零值替身只为满足装配，被读到即是测试该失败的信号。
		Credit:    &creditDouble{},
		Exposures: &exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()},
		Clock:     fixedClock{at: controlAt},
	})
}

func newTermsHandler(
	policy ports.PreAcceptanceControlPolicyView,
	credit ports.CreditStandingView,
	exposures ports.CreditExposureLedgerRepository,
) *application.ApplyPreAcceptanceControlHandler {
	return application.NewApplyPreAcceptanceControlHandler(application.ApplyPreAcceptanceControlDeps{
		Policy:    policy,
		Balance:   &balanceDouble{},
		Freezes:   &ledgerDouble{ledger: domain.NewFreezeLedger()},
		Credit:    credit,
		Exposures: exposures,
		Clock:     fixedClock{at: controlAt},
	})
}

// Covers: UC-SA-002 接受前财务控制「`可用余额 → 已冻结` 必须关联明确账户、金额、提交版本和
// 控制策略」，以及 CONTEXT 要求控制结果保存判断时间与实际采用的 `asOf`——两者是两回事。
func TestSufficientBalanceHoldsFundsAndRecordsBothAsOfAndControlTime(t *testing.T) {
	ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := newHandler(requiredPolicy(t), &balanceDouble{balance: balanceWith(t, 10_000)}, ledger)

	result, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ControlApplied {
		t.Fatalf("outcome = %q, want CONTROL_APPLIED", result.Outcome())
	}
	freeze, present := result.Freeze()
	if !present || freeze.Status() != domain.FreezeHeld {
		t.Fatalf("freeze = %#v, want a HELD freeze", freeze)
	}
	if freeze.AmountMinor() != 4_000 {
		t.Fatalf("frozen %d, want 4000", freeze.AmountMinor())
	}
	if !result.ControlledAt().Equal(controlAt) {
		t.Fatalf("controlled at = %s, want the clock reading %s", result.ControlledAt(), controlAt)
	}
	if result.ControlledAt().Equal(asOfAt) {
		t.Fatal("控制时间取成了 asOf：asOf 决定按哪一版策略判断，控制时间说明资金何时被占用")
	}
	if result.AsOf().At() != asOfAt {
		t.Fatalf("asOf = %s, want the adopted %s", result.AsOf().At(), asOfAt)
	}
	if ledger.saved != 1 {
		t.Fatalf("saved ledger %d times, want exactly one", ledger.saved)
	}
	if ledger.askTenant.String() != "tenant-1" {
		t.Fatalf("asked tenant = %q, want tenant-1", ledger.askTenant)
	}
}

// Covers: UC-SA-002「余额不足」——这是本上下文报告的业务限制，不是错误，也不是接受判决。
func TestInsufficientBalanceIsARestrictedControlResultNotAnError(t *testing.T) {
	ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := newHandler(requiredPolicy(t), &balanceDouble{balance: balanceWith(t, 1_000)}, ledger)

	result, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("余额不足被当成技术错误抛出，而它是调用方必须能据以行动的业务答案: %v", err)
	}

	if result.Outcome() != application.ControlApplied {
		t.Fatalf("outcome = %q, want CONTROL_APPLIED——限制也是一次已执行的控制", result.Outcome())
	}
	freeze, present := result.Freeze()
	if !present || freeze.Status() != domain.FreezeRestricted {
		t.Fatalf("freeze = %#v, want a RESTRICTED result", freeze)
	}
	if freeze.Reason().String() == "" {
		t.Fatal("业务限制没有说明原因")
	}
}

// Covers: AT-SA-049「合同明确接受前无财务控制 → 保存商业不适用依据，不制造零金额冻结或
// 信用通过」。
func TestContractWithoutPreAcceptanceControlIsNotApplicable(t *testing.T) {
	basis := value(t, domain.NewControlBasisReference, "CONTRACT_DECLARES_NO_PRE_ACCEPTANCE_CONTROL")
	notRequired, err := domain.NewNoControlPolicy(basis)
	if err != nil {
		t.Fatalf("new control policy: %v", err)
	}
	balance := &balanceDouble{balance: balanceWith(t, 10_000)}
	ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := newHandler(&policyDouble{policy: notRequired}, balance, ledger)

	result, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ControlNotApplicable {
		t.Fatalf("outcome = %q, want CONTROL_NOT_APPLICABLE", result.Outcome())
	}
	if _, present := result.Freeze(); present {
		t.Fatal("无控制却产生了冻结——这正是用零金额冻结冒充无控制的做法")
	}
	if result.ControlBasis() != basis {
		t.Fatalf("basis = %q, want the explicit %q", result.ControlBasis(), basis)
	}
	if balance.loaded != 0 {
		t.Fatal("合同不要求控制却仍去读余额")
	}
	if ledger.saved != 0 {
		t.Fatal("无控制写入了冻结登记册")
	}
}

// Covers: CONTEXT「不得用默认信用通过冒充无控制」——商业侧调不通不得被读成「不要求控制」。
func TestUnavailableControlPolicyIsNotFormedRatherThanNotApplicable(t *testing.T) {
	balance := &balanceDouble{balance: balanceWith(t, 10_000)}
	ledger := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := newHandler(&policyDouble{err: errors.New("control policy view unavailable")}, balance, ledger)

	result, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("商业依赖失败被当成技术错误抛出，而它应形成待判断: %v", err)
	}

	if result.Outcome() != application.ControlNotFormed {
		t.Fatalf("outcome = %q, want CONTROL_NOT_FORMED", result.Outcome())
	}
	if result.NotFormedReason() != application.ControlPolicyUnavailable {
		t.Fatalf("reason = %q, want CONTROL_POLICY_UNAVAILABLE", result.NotFormedReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("待判断无法安全续办")
	}
	if balance.loaded != 0 {
		t.Fatal("控制策略未确定却已经读余额")
	}
	if ledger.saved != 0 {
		t.Fatal("待判断写入了冻结登记册")
	}
}

// Covers: AT-SA-051「同一请求身份携带不同金额 → 形成请求冲突，原结果不被覆盖」。
func TestSameRequestIdentityWithADifferentAmountIsAConflict(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	repository := &ledgerDouble{ledger: ledger}
	handler := newHandler(requiredPolicy(t), &balanceDouble{balance: balanceWith(t, 10_000)}, repository)

	first, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	original, _ := first.Freeze()

	conflicting, err := handler.Handle(context.Background(), command(t, 7_000))
	if err != nil {
		t.Fatalf("请求冲突被当成技术错误抛出: %v", err)
	}

	if conflicting.Outcome() != application.ControlRequestConflict {
		t.Fatalf("outcome = %q, want CONTROL_REQUEST_CONFLICT", conflicting.Outcome())
	}
	if _, present := conflicting.Freeze(); present {
		t.Fatal("冲突结果携带了冻结")
	}
	held, found := ledger.Lookup(original.FreezeID())
	if !found || held.AmountMinor() != 4_000 {
		t.Fatalf("原冻结被覆盖为 %#v，应保持 4000", held)
	}
}

// Covers: UC-SA-002 一致性「相同请求身份、相同范围、来源版本和规则版本返回既有结果」。
func TestSameRequestReplayReturnsTheOriginalFreeze(t *testing.T) {
	ledger := domain.NewFreezeLedger()
	repository := &ledgerDouble{ledger: ledger}
	handler := newHandler(requiredPolicy(t), &balanceDouble{balance: balanceWith(t, 10_000)}, repository)

	first, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	replay, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle replay: %v", err)
	}

	original, _ := first.Freeze()
	repeated, present := replay.Freeze()
	if !present || repeated.FreezeID() != original.FreezeID() {
		t.Fatalf("replay freeze = %#v, want the original %#v", repeated, original)
	}
	if ledger.HeldCount() != 1 {
		t.Fatalf("held %d freezes, want exactly one——重放不得二次占用资金", ledger.HeldCount())
	}
}

// Covers: UC-SA-002 步骤 2 与隔离要求——最小控制身份不成立时不得读任何权威。
func TestIncompleteRequestIsRefusedWithoutReadingAnyAuthority(t *testing.T) {
	policy := requiredPolicy(t)
	balance := &balanceDouble{balance: balanceWith(t, 10_000)}
	repository := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := newHandler(policy, balance, repository)

	incomplete := command(t, 4_000)
	incomplete.TenantID = domain.TenantID{}

	result, err := handler.Handle(context.Background(), incomplete)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ControlRequestNotAccepted {
		t.Fatalf("outcome = %q, want CONTROL_REQUEST_NOT_ACCEPTED", result.Outcome())
	}
	if policy.asked != 0 || balance.loaded != 0 || repository.loaded != 0 {
		t.Fatal("身份不成立却已经读了权威")
	}
}

// Covers: sa-preacceptance-policy-view/01 键形裁决——缺商业解析回指的命令停在`未受理`，
// 且一次也不去问控制策略。
//
// 这一格必须与`未配置`分开：放它过去，视图拿不到键只能答 found=false，编排就会落成
// CONTROL_POLICY_NOT_CONFIGURED——那句话是「商业侧没登记过这份合同」，会把租户支去补一份
// 其实早就存在的声明，而真正缺的是调用方少给了一个键。两种缺口的补法完全相反。
func TestAControlRequestWithoutACommercialResolutionIsRefusedBeforeAsking(t *testing.T) {
	policy := requiredPolicy(t)
	balance := &balanceDouble{balance: balanceWith(t, 10_000)}
	repository := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := newHandler(policy, balance, repository)

	unreferenced := command(t, 4_000)
	unreferenced.Resolution = domain.CommercialResolutionReference{}

	result, err := handler.Handle(context.Background(), unreferenced)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ControlRequestNotAccepted {
		t.Fatalf("outcome = %q, want CONTROL_REQUEST_NOT_ACCEPTED", result.Outcome())
	}
	if result.NotFormedReason() == application.ControlPolicyNotConfigured {
		t.Fatal("少给键被答成了「商业侧未登记」——租户会去补一份已经存在的声明")
	}
	if policy.asked != 0 {
		t.Fatal("没有回指还是去问了控制策略——那一问问的是哪份合同，谁也说不出")
	}
	if balance.loaded != 0 || repository.loaded != 0 {
		t.Fatal("回指缺席却已经读了这个客户的资金")
	}
}

// Covers: sa-preacceptance-policy-view/01 —— 编排把命令上的回指原样交给控制策略视图。
// 编排不得在这里挑、改或补一个回指：它自己不认识商业侧的键，能做的只有转交。
func TestTheCommercialResolutionReachesThePolicyViewVerbatim(t *testing.T) {
	policy := requiredPolicy(t)
	handler := newHandler(policy, &balanceDouble{balance: balanceWith(t, 10_000)},
		&ledgerDouble{ledger: domain.NewFreezeLedger()})

	referenced := command(t, 4_000)
	referenced.Resolution = value(t, domain.NewCommercialResolutionReference, "RES-OTHER-9")

	if _, err := handler.Handle(context.Background(), referenced); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if policy.askedWith.String() != "RES-OTHER-9" {
		t.Fatalf("视图被问到的回指 = %q, want RES-OTHER-9", policy.askedWith)
	}
}

// Covers: ADR-0047 账期分支与 SET-03「不与同一客户账期范围共用余额、额度」——TERMS 方式
// 的控制在暴露账本上占额度：不读运营余额、不进冻结账本，结果携带实际采用的方式与政策
// 引用（CONTEXT 对控制结果保存政策的硬句）。
func TestATermsContractRecordsACreditExposureWithoutTouchingTheBalance(t *testing.T) {
	policy := &policyDouble{policy: methodPolicy(t, domain.TermsSettlement)}
	credit := &creditDouble{standing: standingWith(t, 10_000, 2_000, false)}
	exposures := &exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()}
	balance := &balanceDouble{}
	freezes := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := application.NewApplyPreAcceptanceControlHandler(application.ApplyPreAcceptanceControlDeps{
		Policy: policy, Balance: balance, Freezes: freezes,
		Credit: credit, Exposures: exposures, Clock: fixedClock{at: controlAt},
	})

	result, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ControlApplied {
		t.Fatalf("outcome = %q, want CONTROL_APPLIED", result.Outcome())
	}
	exposure, present := result.Exposure()
	if !present || exposure.Status() != domain.ExposureRecorded || exposure.AmountMinor() != 4_000 {
		t.Fatalf("exposure = %#v, want a RECORDED 4000", exposure)
	}
	if _, present := result.Freeze(); present {
		t.Fatal("账期控制交回了冻结——它占的是额度不是资金")
	}
	if result.Method() != domain.TermsSettlement || result.AdoptedPolicy().String() != "PC-SETTLEMENT-POLICY-V3" {
		t.Fatalf("method/policy = %v/%q; 控制结果必须保存实际采用的方式与结算政策", result.Method(), result.AdoptedPolicy())
	}
	if balance.loaded != 0 || freezes.loaded != 0 {
		t.Fatal("账期分支碰了预付那本账——两本账互不借用")
	}
	if exposures.saved != 1 {
		t.Fatalf("saved exposure ledger %d times, want exactly one", exposures.saved)
	}
}

// Covers: CONTEXT「余额不足或逾期只向订单接受等责任上下文提供信用暴露和业务限制依据」——
// 超额与逾期都是带原因的业务限制，不是错误也不是接受判决；逾期先于额度判（账户状态与
// 本笔金额无关）。
func TestCreditShortfallAndOverdueEachFormARestriction(t *testing.T) {
	cases := map[string]struct {
		standing domain.CreditStanding
		reason   string
	}{
		"headroom insufficient": {standing: standingWith(t, 5_000, 2_000, false), reason: "AVAILABLE_CREDIT_INSUFFICIENT"},
		"account overdue":       {standing: standingWith(t, 100_000, 0, true), reason: "ACCOUNT_OVERDUE"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			policy := &policyDouble{policy: methodPolicy(t, domain.TermsSettlement)}
			handler := newTermsHandler(policy,
				&creditDouble{standing: testCase.standing},
				&exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()})

			result, err := handler.Handle(context.Background(), command(t, 4_000))
			if err != nil {
				t.Fatalf("业务限制被当成技术错误抛出: %v", err)
			}
			if result.Outcome() != application.ControlApplied {
				t.Fatalf("outcome = %q, want CONTROL_APPLIED——限制也是一次已执行的控制", result.Outcome())
			}
			exposure, present := result.Exposure()
			if !present || exposure.Status() != domain.ExposureRestricted {
				t.Fatalf("exposure = %#v, want RESTRICTED", exposure)
			}
			if exposure.Reason().String() != testCase.reason {
				t.Fatalf("reason = %q, want %q", exposure.Reason(), testCase.reason)
			}
		})
	}
}

// Covers: 账期分支的幂等与冲突走同一本暴露账（AT-SA-045/051 的账期同款）：重放返回原暴露
// 不二次占额度，同身份异金额是冲突且原暴露不被覆盖。信用侧调不通形成待判断，不读成
// 「有额度」。
func TestTermsExposureReplayConflictAndUnavailableStanding(t *testing.T) {
	policy := &policyDouble{policy: methodPolicy(t, domain.TermsSettlement)}
	ledger := domain.NewCreditExposureLedger()
	handler := newTermsHandler(policy,
		&creditDouble{standing: standingWith(t, 10_000, 0, false)},
		&exposureLedgerDouble{ledger: ledger})

	first, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle first: %v", err)
	}
	original, _ := first.Exposure()

	replay, err := handler.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("handle replay: %v", err)
	}
	repeated, present := replay.Exposure()
	if !present || repeated.ExposureID() != original.ExposureID() {
		t.Fatalf("replay exposure = %#v, want the original %#v", repeated, original)
	}

	conflicting, err := handler.Handle(context.Background(), command(t, 7_000))
	if err != nil {
		t.Fatalf("请求冲突被当成技术错误抛出: %v", err)
	}
	if conflicting.Outcome() != application.ControlRequestConflict {
		t.Fatalf("outcome = %q, want CONTROL_REQUEST_CONFLICT", conflicting.Outcome())
	}
	if kept, _ := ledger.FindByRequest(original.RequestID()); kept.AmountMinor() != 4_000 {
		t.Fatalf("原暴露被覆盖为 %#v，应保持 4000", kept)
	}

	unavailable := newTermsHandler(policy,
		&creditDouble{err: errors.New("credit view down")},
		&exposureLedgerDouble{ledger: domain.NewCreditExposureLedger()})
	result, err := unavailable.Handle(context.Background(), command(t, 4_000))
	if err != nil {
		t.Fatalf("信用侧失败被当成技术错误抛出，而它应形成待判断: %v", err)
	}
	if result.Outcome() != application.ControlNotFormed ||
		result.NotFormedReason() != application.CreditStandingUnavailable {
		t.Fatalf("outcome/reason = %q/%q, want CONTROL_NOT_FORMED/CREDIT_STANDING_UNAVAILABLE",
			result.Outcome(), result.NotFormedReason())
	}
}

// Covers: UC-SA-002「非正数金额」——零金额冻结是一次什么也没占用、看上去却执行过的控制，
// 用例禁止用它冒充明确无控制。它在受理阶段就该被挡住，不该走到读余额。
func TestNonPositiveAmountIsRefusedBeforeReadingTheBalance(t *testing.T) {
	policy := requiredPolicy(t)
	balance := &balanceDouble{balance: balanceWith(t, 10_000)}
	repository := &ledgerDouble{ledger: domain.NewFreezeLedger()}
	handler := newHandler(policy, balance, repository)

	result, err := handler.Handle(context.Background(), command(t, 0))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ControlRequestNotAccepted {
		t.Fatalf("outcome = %q, want CONTROL_REQUEST_NOT_ACCEPTED", result.Outcome())
	}
	if balance.loaded != 0 {
		t.Fatal("零金额请求仍去读余额")
	}
}
