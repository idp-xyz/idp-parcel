package labelfinal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/labelfinal"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证三路共用的处理方核与五值翻译（ADR-0134 决定二，lc/25 做法 3 那张表）：按（租户 + 包裹）反查
// 目标委托、折命令、交真判断编排，五值逐格译成消费结论；反查不中 / 歧义 / 译不出各自可识别。判断编排
// 是真的（psapplication.JudgeLabelServiceFinalHandler），四个读口与采用路径用替身——五值要由真编排从
// 夹具里判出来，替身直接吐结果就成了对着自己写的翻译表打勾。

var judgedAt = time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func identity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	built, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return built
}

func acceptedTarget(t *testing.T) psdomain.CurrentAcceptedParcelTarget {
	t.Helper()
	target, err := psdomain.NewCurrentAcceptedParcelTarget(
		identity(t),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		value(t, psdomain.NewSubmissionVersionID, "version-1"),
	)
	if err != nil {
		t.Fatalf("new target: %v", err)
	}
	return target
}

type targetViewDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	tenant string
	parcel string
}

func (double *targetViewDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.tenant = tenant.String()
	double.parcel = parcel.String()
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

type transactionsViewDouble struct {
	transactions []psdomain.LabelTransaction
	err          error
}

func (double *transactionsViewDouble) ListByCoveredParcel(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) ([]psdomain.LabelTransaction, error) {
	return double.transactions, double.err
}

type registerViewDouble struct {
	register psdomain.ContinuedAttemptRegister
	found    bool
}

func (double *registerViewDouble) FindByParcel(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) (psdomain.ContinuedAttemptRegister, bool, error) {
	return double.register, double.found, nil
}

type cancellationViewDouble struct {
	cancelled bool
}

func (double *cancellationViewDouble) FindCancellation(
	context.Context, psdomain.TenantID, psdomain.DeclaredParcelID,
) (psdomain.ParcelCancellation, bool, error) {
	return psdomain.ParcelCancellation{}, double.cancelled, nil
}

// adopterDouble 记下判断交给采用路径的命令。它交回零值结果：采用各格怎么译归 finalconsume 自己的
// 用例，这里只要证「判出终局的那一格确实交到了采用路径并经 finalconsume 收口」。
type adopterDouble struct {
	commands []psapplication.FormParcelFinalCommand
}

func (double *adopterDouble) Handle(
	_ context.Context, command psapplication.FormParcelFinalCommand,
) (psapplication.FormParcelFinalResult, error) {
	double.commands = append(double.commands, command)
	return psapplication.FormParcelFinalResult{}, nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// recordingJudge 包住真判断编排，记下它收到的命令。
type recordingJudge struct {
	inner    *psapplication.JudgeLabelServiceFinalHandler
	commands []psapplication.JudgeLabelServiceFinalCommand
}

func (judge *recordingJudge) Handle(
	ctx context.Context, command psapplication.JudgeLabelServiceFinalCommand,
) (psapplication.LabelServiceFinalResult, error) {
	judge.commands = append(judge.commands, command)
	return judge.inner.Handle(ctx, command)
}

type fixture struct {
	core         *adapter.ParcelJudgmentCore
	targets      *targetViewDouble
	transactions *transactionsViewDouble
	registers    *registerViewDouble
	cancels      *cancellationViewDouble
	adopter      *adopterDouble
	judge        *recordingJudge
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	targets := &targetViewDouble{target: acceptedTarget(t), found: true}
	transactions := &transactionsViewDouble{}
	registers := &registerViewDouble{}
	cancels := &cancellationViewDouble{}
	adopter := &adopterDouble{}
	judge := &recordingJudge{inner: psapplication.NewJudgeLabelServiceFinalHandler(psapplication.JudgeLabelServiceFinalDeps{
		Transactions:  transactions,
		Registers:     registers,
		Cancellations: cancels,
		Adoption:      adopter,
		Clock:         fixedClock{at: judgedAt},
	})}
	core, err := adapter.NewParcelJudgmentCore(targets, judge)
	if err != nil {
		t.Fatalf("new core: %v", err)
	}
	return &fixture{core: core, targets: targets, transactions: transactions, registers: registers,
		cancels: cancels, adopter: adopter, judge: judge}
}

func (f *fixture) judgeParcel(t *testing.T) error {
	t.Helper()
	return f.core.JudgeParcel(context.Background(), "tenant-1", "parcel-1", psdomain.CarrierFirstEffectivePickupSpec{})
}

// failedTransaction 造一笔覆盖 parcel-1、交易级明确失败、包裹级未受理的已定案交易。
func failedTransaction(t *testing.T) psdomain.LabelTransaction {
	t.Helper()
	parcel := value(t, psdomain.NewDeclaredParcelID, "parcel-1")
	established, err := psdomain.EstablishLabelTransaction(psdomain.EstablishLabelTransactionSpec{
		Tenant:                 value(t, psdomain.NewTenantID, "tenant-1"),
		ID:                     value(t, psdomain.NewLabelTransactionID, "LT-1"),
		CoveredParcels:         []psdomain.DeclaredParcelID{parcel},
		ChannelAccount:         value(t, psdomain.NewChannelAccountReference, "ACCT-1"),
		AccountHolder:          value(t, psdomain.NewChannelAccountHolderReference, "HOLDER-1"),
		ServiceProvider:        value(t, psdomain.NewChannelServiceProviderReference, "PROVIDER-1"),
		SettlementCounterparty: value(t, psdomain.NewSettlementCounterpartyReference, "COUNTERPARTY-1"),
		Contract:               value(t, psdomain.NewChannelContractReference, "CONTRACT-1"),
		Rate:                   value(t, psdomain.NewChannelRateReference, "RATE-1"),
		ResponsibilityBasis:    value(t, psdomain.NewResponsibilityBasisSnapshotReference, "BASIS-1"),
		EstablishedAt:          judgedAt.Add(-3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	submitted, err := established.SubmitToChannel(judgedAt.Add(-2 * time.Hour))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	failed, err := submitted.RecordChannelResult(psdomain.RecordChannelResultSpec{
		Outcome: psdomain.LabelTransactionFailed,
		ParcelResults: []psdomain.LabelTransactionParcelResultSpec{{
			Parcel: parcel,
			Reason: value(t, psdomain.NewChannelResultReasonReference, "CHANNEL_REJECTED"),
		}},
		ObservedAt: judgedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("record result: %v", err)
	}
	return failed
}

// closedRegister 造一册带一份生效受控关闭的登记册。
func closedRegister(t *testing.T) psdomain.ContinuedAttemptRegister {
	t.Helper()
	register, err := psdomain.OpenContinuedAttemptRegister(
		value(t, psdomain.NewTenantID, "tenant-1"), value(t, psdomain.NewDeclaredParcelID, "parcel-1"))
	if err != nil {
		t.Fatalf("open register: %v", err)
	}
	closed, err := register.Append(psdomain.ContinuedAttemptDecisionSpec{
		ID:                          value(t, psdomain.NewContinuedAttemptDecisionID, "CAD-1"),
		Kind:                        psdomain.ControlledClosureDecision,
		Decider:                     value(t, psdomain.NewDeciderReference, "OPERATOR-1"),
		AuthorityRole:               value(t, psdomain.NewContinuedAttemptAuthorityRoleReference, "ROLE-1"),
		AuthoritySnapshot:           value(t, psdomain.NewContinuedAttemptAuthoritySnapshot, "GRANT-1"),
		Reason:                      value(t, psdomain.NewContinuedAttemptReasonReference, "NO_MORE_ATTEMPTS"),
		EffectiveAt:                 judgedAt.Add(-30 * time.Minute),
		CutoffBoundary:              value(t, psdomain.NewAuthoritativeCutoffBoundary, "CAD-1"),
		ClosureResponsibilitySource: value(t, psdomain.NewClosureResponsibilitySourceReference, "OPERATIONS"),
	}, false)
	if err != nil {
		t.Fatalf("append closure: %v", err)
	}
	return closed
}

// Covers: 命令由反查结果折成——委托来源身份与委托标识取自当前已接受目标，包裹取自信封，不带收寄事实；
// 租户串从信封带到反查。关闭路径不成立时（无关闭、无交易）判为 NOT_FINAL → 入账。
func TestTheCoreFoldsTheCommandFromTheAcceptedTargetAndConsumesNotFinal(t *testing.T) {
	f := newFixture(t)

	if err := f.judgeParcel(t); err != nil {
		t.Fatalf("NOT_FINAL 应入账，实得：%v", err)
	}
	if f.targets.tenant != "tenant-1" || f.targets.parcel != "parcel-1" {
		t.Fatalf("反查用的键 = %s/%s", f.targets.tenant, f.targets.parcel)
	}
	if len(f.judge.commands) != 1 {
		t.Fatalf("判断次数 = %d, want 1", len(f.judge.commands))
	}
	command := f.judge.commands[0]
	if command.Identity != identity(t) || command.ShipmentRequestID.String() != "request-1" ||
		command.Parcel.String() != "parcel-1" {
		t.Fatalf("命令折错了：%#v", command)
	}
	if command.FirstEffectivePickup != (psdomain.CarrierFirstEffectivePickupSpec{}) {
		t.Fatal("交易那一路的命令带了收寄事实——它该走关闭路径")
	}
	if len(f.adopter.commands) != 0 {
		t.Fatal("不形成终局却交了采用路径")
	}
}

// Covers: CANCELLATION_STANDS → 入账，不交采用路径。
func TestAStandingCancellationIsConsumedWithoutAdoption(t *testing.T) {
	f := newFixture(t)
	f.cancels.cancelled = true

	if err := f.judgeParcel(t); err != nil {
		t.Fatalf("CANCELLATION_STANDS 应入账，实得：%v", err)
	}
	if len(f.adopter.commands) != 0 {
		t.Fatal("取消在先却交了采用路径")
	}
}

// Covers: 生效关闭 + 全部交易明确失败 → 判出终局失败 → 交采用路径，结论由 finalconsume 收口
// （替身交回集合外的零值结果，finalconsume 响亮报错——这正证明结果走到了它手上）。
func TestAFormedFinalIsHandedToTheAdoptionPathAndSettledByFinalconsume(t *testing.T) {
	f := newFixture(t)
	f.transactions.transactions = []psdomain.LabelTransaction{failedTransaction(t)}
	f.registers.register, f.registers.found = closedRegister(t), true

	err := f.judgeParcel(t)
	if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("采用结果应经 finalconsume 收口，实得：%v", err)
	}
	if len(f.adopter.commands) != 1 {
		t.Fatalf("采用路径调用次数 = %d, want 1", len(f.adopter.commands))
	}
	command := f.adopter.commands[0]
	if command.Identity != identity(t) || command.ShipmentRequestID.String() != "request-1" ||
		command.Outcome.Parcel.String() != "parcel-1" {
		t.Fatalf("交给采用路径的命令指错了对象：%#v", command)
	}
}

// Covers: JUDGMENT_UNDECIDED（读口答不出）→ 未决哨兵，重投会改变结果。
func TestAnUndecidedJudgmentIsASentinelNotAPoison(t *testing.T) {
	f := newFixture(t)
	f.transactions.err = errors.New("label transactions unavailable")

	err := f.judgeParcel(t)
	if !errors.Is(err, adapter.ErrJudgmentUndecided) {
		t.Fatalf("err = %v, want ErrJudgmentUndecided", err)
	}
}

// Covers: REQUEST_NOT_ACCEPTED（反查交回一份立不起的目标）→ 适配器缺陷，响亮报错、不进未决。
func TestARejectedCommandIsLoud(t *testing.T) {
	f := newFixture(t)
	f.targets.target = psdomain.CurrentAcceptedParcelTarget{}

	err := f.judgeParcel(t)
	if !errors.Is(err, adapter.ErrJudgmentNotAccepted) {
		t.Fatalf("err = %v, want ErrJudgmentNotAccepted", err)
	}
	if errors.Is(err, adapter.ErrJudgmentUndecided) {
		t.Fatal("命令立不起被登成了未决")
	}
}

// Covers: 反查不中 → ErrParcelTargetNotFound（不猜委托）；歧义 → 原样上抛；译不出 → ErrUntranslatableEnvelope。
func TestTargetLookupFailuresStayDistinguishable(t *testing.T) {
	f := newFixture(t)
	f.targets.found = false
	if err := f.judgeParcel(t); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("反查不中 err = %v, want ErrParcelTargetNotFound", err)
	}
	if len(f.judge.commands) != 0 {
		t.Fatal("反查不中仍去判了")
	}

	f = newFixture(t)
	f.targets.err = psdomain.ErrAmbiguousParcelTarget
	if err := f.judgeParcel(t); !errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatalf("歧义 err = %v, want ErrAmbiguousParcelTarget 原样上抛", err)
	}

	f = newFixture(t)
	err := f.core.JudgeParcel(context.Background(), "", "parcel-1", psdomain.CarrierFirstEffectivePickupSpec{})
	if !errors.Is(err, adapter.ErrUntranslatableEnvelope) {
		t.Fatalf("空租户 err = %v, want ErrUntranslatableEnvelope", err)
	}
}

// Covers: 面单交易那一路的处理方只把（租户 + 包裹）交给核，不带收寄事实、不读回交易。
func TestTheLabelTransactionAdapterDelegatesToTheCoreWithoutAPickup(t *testing.T) {
	f := newFixture(t)
	handler, err := adapter.NewLabelTransactionJudgmentAdapter(f.core)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	if err := handler.HandleLabelTransactionJudgmentDue(context.Background(), psinbox.LabelTransactionJudgmentDue{
		TenantID: "tenant-1", Transaction: "LT-1", Parcel: "parcel-1",
	}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(f.judge.commands) != 1 || f.judge.commands[0].FirstEffectivePickup != (psdomain.CarrierFirstEffectivePickupSpec{}) {
		t.Fatalf("命令 = %#v，want 一次、不带收寄事实", f.judge.commands)
	}
	if f.targets.parcel != "parcel-1" {
		t.Fatalf("反查包裹 = %q", f.targets.parcel)
	}
}

func TestTheCoreRefusesNilDependencies(t *testing.T) {
	if _, err := adapter.NewParcelJudgmentCore(nil, &recordingJudge{}); err == nil {
		t.Fatal("nil 反查口被接受了")
	}
	if _, err := adapter.NewParcelJudgmentCore(&targetViewDouble{}, nil); err == nil {
		t.Fatal("nil 判断口被接受了")
	}
	if _, err := adapter.NewLabelTransactionJudgmentAdapter(nil); err == nil {
		t.Fatal("nil 核被接受了")
	}
}
