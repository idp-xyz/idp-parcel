package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-005 `AT-PS-067`「已提交且尚无决定，客户授权有效 → 形成撤回和任务停止，不形成
// 拒绝」，以及 CONTEXT「委托撤回不能伪装成运营企业拒绝」。
func TestAnAuthorizedCustomerWithdrawsAPendingRequest(t *testing.T) {
	fixture := newWithdrawalFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("state = %q, want WITHDRAWN", result.State())
	}
	if _, present := result.AcceptanceDecision(); present {
		t.Fatal("a customer withdrawal produced an acceptance decision; it must not masquerade as an operator rejection")
	}
	record, present := result.Withdrawal()
	if !present || record.Authority().String() != "PC-WITHDRAW-ROLE-1" {
		t.Fatalf("withdrawal = %#v; the adopted authorization must be the one party-commercial issued", record)
	}
	if fixture.requests.saved == nil {
		t.Fatal("a withdrawal was formed but never saved")
	}
}

// Covers: UC-PS-005「参数未确认时…不得默认任何角色有撤回权」与输入语义「客户备注或连接中断
// 不构成撤回」— 未获授权是确定的业务答案，不是未决，续办也补不出授权来。
func TestAnUnauthorizedWithdrawalFormsNothing(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.authorizer.granted = false

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalNotAuthorized {
		t.Fatalf("outcome = %q, want NOT_AUTHORIZED", result.Outcome())
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d decision IDs; an unauthorized attempt consumed a scarce identity", fixture.identities.issued)
	}
	if fixture.requests.saved != nil {
		t.Fatal("an unauthorized attempt saved the request")
	}
	if fixture.release.calls != 0 {
		t.Fatal("an unauthorized attempt released the freeze")
	}
}

// Covers: UC-PS-005 `AT-PS-071`「接受先合法提交，决定前撤回请求随后到达 → 返回接受结果，不释放
// 合法冻结」与 `AT-PS-072`（拒绝先行）——后到者只能读既有结果。
func TestALateWithdrawalReadsTheDecisionThatAlreadyWon(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.requests.decided = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalDecisionAlreadyFormed {
		t.Fatalf("outcome = %q, want DECISION_ALREADY_FORMED", result.Outcome())
	}
	decision, present := result.AcceptanceDecision()
	if !present || decision.DecisionID().String() != "decision-0" {
		t.Fatalf("decision = %#v; the later withdrawal must read the earlier decision, not form its own", decision)
	}
	if _, withdrawn := result.Withdrawal(); withdrawn {
		t.Fatal("a late withdrawal recorded itself against a version that was already decided")
	}
	if fixture.requests.saved != nil {
		t.Fatal("a late withdrawal saved over an already decided version")
	}
	if fixture.release.calls != 0 {
		t.Fatal("a late withdrawal released a freeze that a formed acceptance still lawfully holds")
	}
}

// Covers: UC-PS-005 步骤 6「按原业务关联幂等形成适用冻结释放」与 CONTEXT「已经形成的接受前
// 资金冻结必须通过原业务关联请求显式释放」— 撤回同样是接受确定未成立。
func TestAWithdrawalReleasesTheFreezeByItsOriginalAssociation(t *testing.T) {
	fixture := newWithdrawalFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if fixture.release.calls != 1 {
		t.Fatalf("release calls = %d, want exactly 1", fixture.release.calls)
	}
	if fixture.release.controlResultID != "SAC-1" {
		t.Fatalf("released %q, want the original control association SAC-1", fixture.release.controlResultID)
	}
}

// Covers: UC-PS-005 `AT-PS-073`「撤回成立但冻结释放暂时失败 → 撤回保持有效，只续办原释放请求」
// 与「不得为了保持表面原子性…把委托改回`已提交`」。
func TestAFailedReleaseKeepsTheWithdrawalAndLeavesCompensationPending(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.release.err = errors.New("settlement authority unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalFormed || result.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("outcome = %q state = %q; a failed release rolled the withdrawal back", result.Outcome(), result.State())
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("a failed release left no continuation to resume the compensation")
	}
}

// Covers: UC-PS-005 `AT-PS-074`「未形成过资金冻结 → 撤回成立并保存财务补偿不适用依据」——
// 没有冻结就不发释放，也不凭空留一个待续补偿让对账去追一笔不存在的释放。
func TestAWithdrawalWithoutAnyFreezeSendsNoReleaseAndLeavesNoCompensation(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.judgments.controlOutcome = domain.FinancialControlOutcomeInvalid

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("state = %q, want WITHDRAWN", result.State())
	}
	if fixture.release.calls != 0 {
		t.Fatal("a release was sent for a freeze that was never formed")
	}
	if result.CompensationReference().String() != "" {
		t.Fatal("an inapplicable compensation still left a continuation to chase")
	}
}

// Covers: UC-PS-001 结果语义`尚未决定` — 授权服务答不出是依赖故障，与「不授权」用不同结果和
// 不同原因：前者要重试，后者要去补授权。
func TestAnUnavailableWithdrawalAuthorizerIsUndecidedRatherThanUnauthorized(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.authorizer.err = errors.New("authorization service unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.WithdrawalAuthorityUnavailable {
		t.Fatalf("pending reason = %q, want WITHDRAWAL_AUTHORITY_UNAVAILABLE", result.PendingReason())
	}
	if result.ContinuationReference().String() == "" {
		t.Fatal("an undecided round left no continuation")
	}
}

// Covers: judgmentContinuation 的派生约定「同一范围因同一原因停滞时拿到的引用始终相同」——
// 同一笔冻结释放失败，撤回与主动拒绝必须给出同一个续办引用；三条路径共用一个补偿身份。
func TestWithdrawalAndRejectionDeriveTheSameCompensationReference(t *testing.T) {
	withdrawal := newWithdrawalFixture(t)
	withdrawal.release.err = errors.New("settlement authority unavailable")

	byWithdrawal, err := withdrawal.handler.Handle(context.Background(), withdrawal.command(t))
	if err != nil {
		t.Fatalf("handle the withdrawal: %v", err)
	}

	rejection := newRejectionFixture(t)
	rejection.release.err = errors.New("settlement authority unavailable")

	byRejection, err := rejection.handler.Handle(context.Background(), rejection.command(t))
	if err != nil {
		t.Fatalf("handle the active rejection: %v", err)
	}

	if byWithdrawal.CompensationReference().String() != byRejection.CompensationReference().String() {
		t.Fatalf(
			"withdrawal %q, rejection %q; one pending compensation must carry one continuation whichever path ended the acceptance",
			byWithdrawal.CompensationReference(), byRejection.CompensationReference(),
		)
	}
}

// Covers: UC-PS-005 `AT-PS-068`「同一撤回请求重复到达 → 返回原撤回及原补偿关联」与步骤 6
// 「按原业务关联幂等形成适用冻结释放」— 重复请求读回原撤回，释放按幂等重跑而不是再撤一次。
func TestARepeatedWithdrawalReturnsTheOriginalWithdrawalAndItsCompensation(t *testing.T) {
	first := newWithdrawalFixture(t)
	first.release.err = errors.New("settlement authority unavailable")

	original, err := first.handler.Handle(context.Background(), first.command(t))
	if err != nil {
		t.Fatalf("handle the first withdrawal: %v", err)
	}

	repeat := newWithdrawalFixture(t)
	repeat.requests.withdrawn = true
	repeat.release.err = errors.New("settlement authority unavailable")

	again, err := repeat.handler.Handle(context.Background(), repeat.command(t))
	if err != nil {
		t.Fatalf("handle the repeated withdrawal: %v", err)
	}

	if again.Outcome() != application.WithdrawalDecisionAlreadyFormed {
		t.Fatalf("outcome = %q, want DECISION_ALREADY_FORMED", again.Outcome())
	}
	record, present := again.Withdrawal()
	if !present || record.DecisionID().String() != "decision-0" {
		t.Fatalf("withdrawal = %#v; the repeat must read the earlier withdrawal, not form a second one", record)
	}
	if repeat.requests.saved != nil {
		t.Fatal("a repeated withdrawal saved a second decision over the first")
	}
	if again.CompensationReference().String() != original.CompensationReference().String() {
		t.Fatalf(
			"repeat %q, original %q; a repeated request must resume the original compensation association",
			again.CompensationReference(), original.CompensationReference(),
		)
	}
}

// Covers: UC-PS-005 `AT-PS-076`「已撤回后客户要求恢复原委托 → 原委托不恢复，建立关联新委托」
// 与 CONTEXT「已撤回委托不得原地恢复」— 后到的请求读回原撤回，委托绝不回到`已提交`。
func TestAWithdrawnRequestIsNeverRestoredInPlace(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.requests.withdrawn = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("state = %q; a withdrawn request was restored in place", result.State())
	}
	if fixture.requests.saved != nil {
		t.Fatal("a request that was already withdrawn got written again")
	}
	// 用例把读取既有决定排在提交撤回之前，因此一份已决委托不该再消耗一个决定标识。
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d decision IDs; a request that was already decided consumed a scarce identity", fixture.identities.issued)
	}
}

// Covers: `UC-PS-005` 结果语义「撤回未决：已知事实、缺口、判断版本和续办入口」— 未决交回的是
// 已知事实，而委托当前是什么状态正是本轮已经查到的事实之一。授权服务答不出时委托可能早已撤回
// 或接受，这时报`已提交`是编出来的：调用方会据此以为这单还等着自己去撤。
func TestAnUndecidedRoundReportsTheStateItActuallyRead(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.requests.withdrawn = true
	fixture.authorizer.err = errors.New("party-commercial authority service unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("state = %q, want WITHDRAWN; the round had the request in hand", result.State())
	}
}

// Covers: 同一条结果语义的另一半 — 委托根本没取回来时，状态是未知而不是`已提交`。给一个查都
// 没查到的委托安上生命周期状态，与上一个测试挡的是同一类编造。
func TestAnUndecidedRoundReportsNoStateWhenItNeverReadTheRequest(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.requests.err = errors.New("shipment request store unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.State() != domain.ShipmentRequestStateInvalid {
		t.Fatalf("state = %q, want it reported as unknown", result.State())
	}
}

// Covers: UC-PS-005 步骤 1「保全撤回来源、请求身份、对象、请求方和业务时间」— 来源先于一切
// 业务判断保全，否则一次没能形成撤回的请求会连它到达过都无从追溯。
func TestAWithdrawalRequestPreservesItsSourceBeforeAnythingElse(t *testing.T) {
	fixture := newWithdrawalFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}

	if _, found := fixture.sources.stored(t, withdrawalSourceIdentity(t)); !found {
		t.Fatal("a withdrawal request was handled without preserving its own source")
	}
	if fixture.steps[0] != "preserve-source" {
		t.Fatalf("first step = %q; the source must be preserved before any business judgement", fixture.steps[0])
	}
}

// Covers: UC-PS-005 `AT-PS-069`「同一请求身份携带不同内容 → 形成来源冲突，不覆盖原请求」与
// 步骤 1「冲突不覆盖」— 同一撤回请求身份带着不同内容到达，是冲突而不是第二次撤回。
func TestAWithdrawalSourceConflictNeitherOverwritesNorWithdrawsAgain(t *testing.T) {
	fixture := newWithdrawalFixture(t)

	first := fixture.command(t)
	if _, err := fixture.handler.Handle(context.Background(), first); err != nil {
		t.Fatalf("handle the first request: %v", err)
	}

	conflicting := fixture.command(t)
	conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "withdrawal-digest-2")

	result, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("handle the conflicting request: %v", err)
	}

	if result.Outcome() != application.WithdrawalSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
	}
	preserved, _ := fixture.sources.stored(t, withdrawalSourceIdentity(t))
	if preserved.Digest().String() != "withdrawal-digest-1" {
		t.Fatalf("preserved digest = %q; a conflicting request overwrote the original source", preserved.Digest())
	}
	if fixture.sources.preserveCount != 1 {
		t.Fatalf("preserved %d times; a conflict must not preserve a second source fact", fixture.sources.preserveCount)
	}
}

// Covers: UC-PS-005 `AT-PS-068`「同一撤回请求重复到达 → 返回原撤回及原补偿关联」与步骤 1
// 「重复返回原处理」— 重复到达在来源层就短路，不再走一遍授权与决定边界。
func TestARepeatedWithdrawalSourceReturnsTheOriginalHandling(t *testing.T) {
	fixture := newWithdrawalFixture(t)

	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle the first request: %v", err)
	}
	fixture.requests.withdrawn = true
	authorizationsBefore := fixture.authorizer.calls

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle the repeated request: %v", err)
	}

	if result.Outcome() != application.WithdrawalDecisionAlreadyFormed {
		t.Fatalf("outcome = %q, want DECISION_ALREADY_FORMED", result.Outcome())
	}
	if _, present := result.Withdrawal(); !present {
		t.Fatal("a repeated request did not return the original withdrawal")
	}
	if fixture.authorizer.calls != authorizationsBefore {
		t.Fatal("a replayed source went on to ask the authorizer again instead of returning the original handling")
	}
	if len(fixture.sources.observations) != 1 {
		t.Fatalf("observations = %d; a replay must be appended beside the preserved fact", len(fixture.sources.observations))
	}
}

type withdrawalFixture struct {
	handler    *application.WithdrawShipmentRequestHandler
	sources    *sourceRepositoryDouble
	requests   *rejectableRequestStore
	authorizer *withdrawalAuthorizerDouble
	judgments  *recordedJudgmentsDouble
	release    *controlReleaseDouble
	identities *countingIdentityFactory
	steps      []string
}

func newWithdrawalFixture(t *testing.T) *withdrawalFixture {
	t.Helper()
	value := &withdrawalFixture{
		requests:   &rejectableRequestStore{t: t},
		authorizer: &withdrawalAuthorizerDouble{t: t, granted: true},
		judgments:  &recordedJudgmentsDouble{t: t, controlOutcome: domain.FinancialControlHeld},
		release:    &controlReleaseDouble{},
		identities: &countingIdentityFactory{t: t},
	}
	value.sources = &sourceRepositoryDouble{
		records: map[domain.SourceIdentity]domain.SourceSubmissionFingerprint{},
		record:  func(step string) { value.steps = append(value.steps, step) },
	}
	value.authorizer.record = func(step string) { value.steps = append(value.steps, step) }
	value.handler = application.NewWithdrawShipmentRequestHandler(application.WithdrawShipmentRequestDeps{
		Sources:    value.sources,
		Requests:   value.requests,
		Authorizer: value.authorizer,
		Judgments:  value.judgments,
		Recorder:   &judgmentRequestStore{},
		Release:    value.release,
		Identities: value.identities,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return value
}

// withdrawalSourceIdentity 是撤回请求自己的来源身份，与产生委托的那一次提交分开：同一个客户
// 就同一份委托先提交后撤回，是两次来源请求，各自有各自的请求标识与内容摘要。
func withdrawalSourceIdentity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	return sourceIdentity(t, "tenant-1", "customer-1", "source-a", "withdrawal-key-1")
}

func (value *withdrawalFixture) command(t *testing.T) application.WithdrawShipmentRequestCommand {
	t.Helper()
	return application.WithdrawShipmentRequestCommand{
		Identity:           sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		WithdrawalIdentity: withdrawalSourceIdentity(t),
		PayloadDigest:      mustValue(t, domain.NewPayloadDigest, "withdrawal-digest-1"),
		OccurredAt:         handlerClockAt,
		ReceivedAt:         handlerClockAt,
		ShipmentRequestID:  mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion:  mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		Requester:          mustValue(t, domain.NewWithdrawalRequesterReference, "CUSTOMER-CONTACT-1"),
		Reason:             mustValue(t, domain.NewWithdrawalReasonReference, "CUSTOMER_NO_LONGER_REQUIRES_SERVICE"),
	}
}

// withdrawalAuthorizerDouble 用零值引用表示未授权，与端口约定一致：未授权是业务答案，不是错误。
type withdrawalAuthorizerDouble struct {
	t       *testing.T
	granted bool
	err     error
	calls   int
	record  func(string)
}

func (double *withdrawalAuthorizerDouble) AuthorizeWithdrawal(
	_ context.Context,
	_ ports.WithdrawalAuthorizationQuery,
) (domain.WithdrawalAuthorityReference, error) {
	double.t.Helper()
	double.calls++
	if double.record != nil {
		double.record("authorize-withdrawal")
	}
	if double.err != nil {
		return domain.WithdrawalAuthorityReference{}, double.err
	}
	if !double.granted {
		return domain.WithdrawalAuthorityReference{}, nil
	}
	return mustValue(double.t, domain.NewWithdrawalAuthorityReference, "PC-WITHDRAW-ROLE-1"), nil
}

var _ ports.WithdrawalAuthorizer = (*withdrawalAuthorizerDouble)(nil)
