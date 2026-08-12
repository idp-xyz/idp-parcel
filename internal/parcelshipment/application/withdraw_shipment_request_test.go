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
//
// Covers: `AT-PS-075`「其他客户账户请求撤回 → 拒绝越权操作」的拒绝半边；不泄露半边由
// TestAWithdrawalForAnUnknownRequestIsUniformlyInvisible 承重（同一租户内的越权按完整来源
// 身份定位，落的也是那条统一不可见出口）。
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

// Covers: UC-PS-005 `AT-PS-072`「拒绝已经先行提交 → 返回拒绝结果，不追加撤回决定」——
// 后到者只能读既有结果。夹具里的先行决定是一次真实的 RejectByAuthority；接受先行的字面
// 场景由下一条单独演练，两条不共用一个方向（此前本注释把两条 AT 挂在同一个拒绝夹具上，
// 失败消息还写着 acceptance——声称与夹具不符，实测于 `d551872` 订正）。
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
		t.Fatal("a late withdrawal re-ran the compensation of a rejection it did not form")
	}
}

// Covers: UC-PS-005 `AT-PS-071`「接受先合法提交，决定前撤回请求随后到达 → 返回接受结果，
// 不释放合法冻结」——冻结在接受成立后是合法占用，后到的撤回既不得追加决定，更不得把它
// 放掉。「转接受后路径」半句是回执语义，UC-PS-006 未实现，此处不冒领。
func TestALateWithdrawalAfterAnAcceptanceKeepsItsLawfulFreeze(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.requests.acceptedFirst = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.WithdrawalDecisionAlreadyFormed {
		t.Fatalf("outcome = %q, want DECISION_ALREADY_FORMED", result.Outcome())
	}
	decision, present := result.AcceptanceDecision()
	if !present || !decision.Accepted() {
		t.Fatalf("decision = %#v; 后到的撤回读回的必须是那一次已成立的接受", decision)
	}
	if _, withdrawn := result.Withdrawal(); withdrawn {
		t.Fatal("a late withdrawal recorded itself against an accepted version")
	}
	if fixture.requests.saved != nil {
		t.Fatal("a late withdrawal saved over an accepted version")
	}
	if fixture.release.calls != 0 {
		t.Fatal("a late withdrawal released a freeze that a formed acceptance still lawfully holds")
	}
}

// Covers: `AT-PS-075` 的不泄露半边「其他客户账户请求撤回 → 不泄露对象」——查不到与越权在
// 端口上是同一个否定答案（FindBySourceIdentity 以完整来源身份为键），这条出口上抛统一的
// ErrInvalidShipmentRequest，接 HTTP 时映射`统一不可见结果`（ADR-0022）。授权在定位之后，
// 一个查不到的对象连授权都不该问——问了，授权服务的答复本身就泄露了对象存在与否。
func TestAWithdrawalForAnUnknownRequestIsUniformlyInvisible(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.requests.missing = true

	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))

	if !errors.Is(err, domain.ErrInvalidShipmentRequest) {
		t.Fatalf("err = %v, want ErrInvalidShipmentRequest——统一不可见出口没有生效", err)
	}
	if fixture.authorizer.calls != 0 {
		t.Fatal("对一份查不到的委托问了授权——答复本身会泄露对象存在与否")
	}
	if fixture.release.calls != 0 {
		t.Fatal("对一份查不到的委托发了释放")
	}
}

// Covers: UC-PS-005 步骤 6「按原业务关联幂等形成适用冻结释放」与 CONTEXT「已经形成的接受前
// 资金冻结必须通过原业务关联请求显式释放」— 撤回同样是接受确定未成立。
//
// Covers: `AT-PS-070`「撤回与自动接受并发，撤回先合法提交 → 适用冻结进入释放补偿」的释放
// 半边；「接受不得再成立」半边由领域侧 TestADecisionCannotFormAfterAWithdrawalWon 承重。
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
//
// 第三条路径由 reject_shipment_request_test.go 的「两条拒绝路径同引用」接上：那里比的是主动
// 拒绝与规则拒绝，与本测试共用主动拒绝这一端，三条因而首尾相连。
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

// Covers: judgmentContinuation 的派生约定「停在不同阶段的两次未决给出不同引用，用例要求二者
// 分别统计、各走各的续办路径」——同一份委托因两种不同原因停下，绝不能收敛成同一个引用。
//
// 拒绝那条路上同一条性质已由 `TestBothRejectionPathsDeriveTheSameCompensationReference` 挡住，
// 但撤回有自己的 compensationReference 与自己的两个调用点：那边全绿也拦不住这边把原因写岔。
// `RecordedJudgmentsUnavailable`（判断读不回来，压根不知道该释放哪一笔）与 `ControlReleasePending`
// （关联清楚，释放没确认完成）要补的东西完全不同，而只断言引用非空的测试对写岔是全盲的。
func TestTwoDifferentStoppingCausesNeverShareOneCompensationReference(t *testing.T) {
	unreadable := newWithdrawalFixture(t)
	unreadable.judgments.err = errors.New("recorded judgement store unavailable")

	byUnreadableJudgments, err := unreadable.handler.Handle(context.Background(), unreadable.command(t))
	if err != nil {
		t.Fatalf("handle the withdrawal whose judgements were unreadable: %v", err)
	}

	unreleased := newWithdrawalFixture(t)
	unreleased.release.err = errors.New("settlement authority unavailable")

	byPendingRelease, err := unreleased.handler.Handle(context.Background(), unreleased.command(t))
	if err != nil {
		t.Fatalf("handle the withdrawal whose release failed: %v", err)
	}

	if byUnreadableJudgments.CompensationReference().String() == "" {
		t.Fatal("unreadable judgements left no continuation to resume the compensation")
	}
	if byPendingRelease.CompensationReference().String() == "" {
		t.Fatal("a failed release left no continuation to resume the compensation")
	}
	if byUnreadableJudgments.CompensationReference().String() == byPendingRelease.CompensationReference().String() {
		t.Fatalf(
			"both stops derived %q; a compensation that never knew which freeze to release must not be filed as one that failed to release a known freeze",
			byPendingRelease.CompensationReference(),
		)
	}

	// 上面那条不相等挡不住原因写岔：释放失败那一轮比判断读不回来那一轮多带一个控制关联，光凭
	// 范围就已经不同，两个调用点即便共用同一个原因也照样不相等。真正把原因钉住的是跨路径相等
	// ——主动拒绝在同一处停下时给的是同一个范围同一个原因，两边必须逐字一致。
	byRejection := newRejectionFixture(t)
	byRejection.judgments.err = errors.New("recorded judgement store unavailable")

	rejected, err := byRejection.handler.Handle(context.Background(), byRejection.command(t))
	if err != nil {
		t.Fatalf("handle the rejection whose judgements were unreadable: %v", err)
	}
	if byUnreadableJudgments.CompensationReference().String() != rejected.CompensationReference().String() {
		t.Fatalf(
			"withdrawal %q, rejection %q; one unresolved association must carry one continuation whichever path ended the acceptance",
			byUnreadableJudgments.CompensationReference(), rejected.CompensationReference(),
		)
	}
}

// Covers: judgmentContinuation「由未决原因与判断范围共同派生」——原因必须真的参与派生。
//
// 前一个测试只能证明两个引用不同，而不同也可能全部来自范围：释放失败那一条比判断读不回来那
// 一条多带一个控制关联。这里两轮的范围完全一致，剩下的变量只有原因，所以引用一旦相同就说明
// 原因根本没进派生——那样一来所有按原因区分的断言都是假的，包括上一个测试。
func TestTheReasonItselfParticipatesInTheContinuationDerivation(t *testing.T) {
	undecided := newWithdrawalFixture(t)
	undecided.authorizer.err = errors.New("party-commercial authority service unavailable")

	byUnavailableAuthority, err := undecided.handler.Handle(context.Background(), undecided.command(t))
	if err != nil {
		t.Fatalf("handle the withdrawal whose authorizer was unavailable: %v", err)
	}
	if byUnavailableAuthority.PendingReason() != application.WithdrawalAuthorityUnavailable {
		t.Fatalf("pending reason = %q, want WITHDRAWAL_AUTHORITY_UNAVAILABLE", byUnavailableAuthority.PendingReason())
	}

	unreadable := newWithdrawalFixture(t)
	unreadable.judgments.err = errors.New("recorded judgement store unavailable")

	byUnreadableJudgments, err := unreadable.handler.Handle(context.Background(), unreadable.command(t))
	if err != nil {
		t.Fatalf("handle the withdrawal whose judgements were unreadable: %v", err)
	}

	// 两条路径的范围同为租户、客户、委托与判断版本四项，没有附加范围。
	if byUnavailableAuthority.ContinuationReference().String() == byUnreadableJudgments.CompensationReference().String() {
		t.Fatalf(
			"both derived %q at one identical scope; the reason is not participating in the derivation, so no reference can tell two stopping causes apart",
			byUnavailableAuthority.ContinuationReference(),
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

// Covers: UC-PS-005「参数未确认时不得默认任何角色有撤回权」与 AGENTS.md 红线「实例半边留空
// 并拒绝默认值」——授权规则一条都没登记时，客户得到的不能是「你无权撤回」。
//
// 这一格今天必然发生而不是偶发：撤回授权角色是 `PAR-COM-14` 待提供的实例参数，没有租户就没有
// 任何规则，于是首发期每一次撤回都走这一支。把它答成业务拒绝，等于把一个尚未配置的产品说成
// 对客户的判定。它与`未获授权`的恢复动作相反——一个等租户登记，一个再登记也不会变。
//
// `ReachabilityAsOfNotConfigured` 早为同一个 `PAR-COM-14` 写过同一条理由，只是当时只做在时点
// 那一维。本条把它补到授权这一维。
func TestUnconfiguredWithdrawalRulesStallRatherThanRefuseTheCustomer(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.authorizer.rulesNotConfigured = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() == application.WithdrawalNotAuthorized {
		t.Fatal("一条授权规则都没登记，客户却被告知无权撤回——那是把未配置说成了业务拒绝")
	}
	if result.Outcome() != application.WithdrawalUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.WithdrawalAuthorityRulesNotConfigured {
		t.Fatalf("pending reason = %q, want WITHDRAWAL_AUTHORITY_RULES_NOT_CONFIGURED", result.PendingReason())
	}
	// 与「授权服务答不出」必须分得开：续办引用由原因派生，共用会让运维拿一条引用查回来另一种
	// 缺口——一个要去催租户登记规则，一个要去看服务为什么调不通。
	if application.WithdrawalAuthorityRulesNotConfigured == application.WithdrawalAuthorityUnavailable {
		t.Fatal("未配置与答不出共用同一个原因，两者的恢复动作因此分不开")
	}
	if fixture.identities.issued != 0 {
		t.Fatalf("issued %d decision IDs; 一次停下来的撤回消耗了稀缺身份", fixture.identities.issued)
	}
	if fixture.requests.saved != nil {
		t.Fatal("一次停下来的撤回把委托写了回去")
	}
}

// withdrawalAuthorizerDouble 按端口约定用取值表示答案，错误只留给「授权服务答不出」。
// 两个布尔而不直接收枚举，理由同 rejectionAuthorizerDouble。
type withdrawalAuthorizerDouble struct {
	t                  *testing.T
	granted            bool
	rulesNotConfigured bool
	err                error
	calls              int
	record             func(string)
}

func (double *withdrawalAuthorizerDouble) AuthorizeWithdrawal(
	_ context.Context,
	_ ports.WithdrawalAuthorizationQuery,
) (ports.WithdrawalAuthorization, error) {
	double.t.Helper()
	double.calls++
	if double.record != nil {
		double.record("authorize-withdrawal")
	}
	if double.err != nil {
		return ports.WithdrawalAuthorization{}, double.err
	}
	if double.rulesNotConfigured {
		return ports.WithdrawalAuthorization{Outcome: ports.AuthorizationRulesNotConfigured}, nil
	}
	if !double.granted {
		return ports.WithdrawalAuthorization{Outcome: ports.AuthorizationRefused}, nil
	}
	return ports.WithdrawalAuthorization{
		Outcome:   ports.AuthorizationGranted,
		Authority: mustValue(double.t, domain.NewWithdrawalAuthorityReference, "PC-WITHDRAW-ROLE-1"),
	}, nil
}

var _ ports.WithdrawalAuthorizer = (*withdrawalAuthorizerDouble)(nil)
