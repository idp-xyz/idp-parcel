package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证形成决定这一步读到一份已撤回或已拒绝（非本轮形成）的委托时的释放（ADR-0290）：接受链在控制作用域锁上排队、或
// 在撤回提交之前已写下冻结，到这一步时委托已经终结，这一轮占下的资金没有别的路径去放。已接受的委托则绝不放。

// terminalKind 把三种已有决定的委托摆成同一张表：释放只对已撤回、已拒绝成立。
type terminalKind struct {
	name         string
	state        domain.ShipmentRequestState
	releases     bool
	prepare      func(t *testing.T, fixture *decisionFixture)
	hasDecision  bool
	decisionSent bool
}

func terminalKinds() []terminalKind {
	return []terminalKind{
		{
			name: "withdrawn", state: domain.ShipmentRequestWithdrawn, releases: true,
			prepare: func(t *testing.T, fixture *decisionFixture) {
				withdrawn := withdrawnRequest(t)
				fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
			},
		},
		{
			name: "rejected", state: domain.ShipmentRequestRejected, releases: true, hasDecision: true, decisionSent: true,
			prepare: func(t *testing.T, fixture *decisionFixture) {
				formedRejection(t, fixture)
			},
		},
		{
			name: "accepted", state: domain.ShipmentRequestAccepted, releases: false, hasDecision: true, decisionSent: true,
			prepare: func(t *testing.T, fixture *decisionFixture) {
				formedAcceptance(t, fixture)
			},
		},
	}
}

// formedRejection 让夹具里的委托经本编排自己形成一次拒绝（可达性不通过），并把释放计数清零：之后的断言只看重放。
func formedRejection(t *testing.T, fixture *decisionFixture) {
	t.Helper()
	fixture.requests.sticky = true
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil || first.State() != domain.ShipmentRequestRejected {
		t.Fatalf("先形成一次拒绝：state = %q err = %v", first.State(), err)
	}
	resetObservations(fixture)
}

func formedAcceptance(t *testing.T, fixture *decisionFixture) {
	t.Helper()
	fixture.requests.sticky = true
	first, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil || first.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("先形成一次接受：state = %q err = %v", first.State(), err)
	}
	resetObservations(fixture)
}

func resetObservations(fixture *decisionFixture) {
	fixture.release.calls, fixture.release.versions, fixture.release.scopes = 0, nil, nil
	fixture.scopeLocks.tenants, fixture.scopeLocks.scopes = nil, nil
	fixture.downstream.intents = nil
	fixture.identities.issued = 0
}

// Covers: AT-PS-153 与 UC-PS-005 步骤 6——接受链读到的委托已是已撤回或已拒绝时，这一轮占下的资金要放：先取这份委托
// 各版占用所在作用域的锁，再按原关联逐版释放；已接受的委托绝不放。
func TestFormationOnAnAlreadyDecidedRequestReleasesOnlyWhenItWasWithdrawnOrRejected(t *testing.T) {
	for _, kind := range terminalKinds() {
		t.Run(kind.name, func(t *testing.T) {
			fixture := newDecisionFixture(t)
			kind.prepare(t, fixture)

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AcceptanceDecided || result.State() != kind.state {
				t.Fatalf("outcome = %q state = %q, want DECIDED / %q", result.Outcome(), result.State(), kind.state)
			}
			if _, present := result.AcceptanceDecision(); present != kind.hasDecision {
				t.Fatalf("decision present = %v, want %v", present, kind.hasDecision)
			}
			wantCalls := 0
			if kind.releases {
				wantCalls = 1
			}
			if fixture.release.calls != wantCalls {
				t.Fatalf("release calls = %d, want %d——%s 的委托上放（或不放）错了", fixture.release.calls, wantCalls, kind.name)
			}
			if len(fixture.scopeLocks.scopes) != wantCalls {
				t.Fatalf("scope locks = %d, want %d——释放之前先取作用域锁", len(fixture.scopeLocks.scopes), wantCalls)
			}
			if result.ControlReleaseNotAccepted() || result.CompensationReference().String() != "" {
				t.Fatal("放掉了却带着未受理或补偿引用")
			}
			if fixture.identities.issued != 0 {
				t.Fatalf("issued %d decision IDs, want 0——已终结的委托不再形成决定", fixture.identities.issued)
			}
			if (len(fixture.downstream.intents) == 1) != kind.decisionSent {
				t.Fatalf("handed off %d intents, want 决定在场时重发一次、不在场时不发", len(fixture.downstream.intents))
			}
		})
	}
}

// Covers: AT-PS-153——取锁先于释放，且一把锁覆盖释放认领的作用域；锁序与撤回类路径一致（被取代各版在前、当前版本在后）。
func TestTheTerminalReleaseTakesTheScopeLocksBeforeAnyRelease(t *testing.T) {
	fixture := newDecisionFixture(t)
	withdrawn := withdrawnSupersededRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
	fixture.judgments.controlByVersion = bothVersionsOccupied()
	var steps []string
	fixture.scopeLocks.onLock = func(domain.OccupationScope) { steps = append(steps, "lock") }
	fixture.release.record = func(step string) { steps = append(steps, step) }
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")

	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !slices.Equal(steps, []string{"lock", "lock", "release-control", "release-control"}) {
		t.Fatalf("调用序 = %v, want 两把锁都在两次释放之前", steps)
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
		t.Fatalf("释放指名的版本 = %v, want [version-2 version-1]——被取代那一版那一笔成了孤儿", got)
	}
	locked := make([]string, 0, len(fixture.scopeLocks.scopes))
	for _, scope := range fixture.scopeLocks.scopes {
		locked = append(locked, scope.Account())
	}
	if !slices.Equal(locked, []string{"SYN-ACCOUNT-SAC-version-1", "SYN-ACCOUNT-SAC-version-2"}) {
		t.Fatalf("取锁次序 = %v, want 被取代的 version-1 在前、当前版本 version-2 在后", locked)
	}
}

// Covers: AT-PS-153——没占下什么的委托不发释放，也不取锁。
func TestATerminalRequestThatOccupiedNothingSendsNoReleaseAndTakesNoLock(t *testing.T) {
	fixture := newDecisionFixture(t)
	withdrawn := withdrawnRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
	fixture.judgments.controlOutcome = domain.FinancialControlOutcomeInvalid

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil || result.Outcome() != application.AcceptanceDecided {
		t.Fatalf("outcome = %q err = %v, want DECIDED", result.Outcome(), err)
	}
	if fixture.release.calls != 0 || len(fixture.scopeLocks.scopes) != 0 {
		t.Fatalf("release calls = %d locks = %d, want 0 / 0", fixture.release.calls, len(fixture.scopeLocks.scopes))
	}
}

// Covers: AT-PS-154——放不收口（可重试未形成）不往下交「已决定」：这一轮回滚，刚占下的冻结随之撤回。回滚靠未决原因的恢复
// 动作是内部重试，消费门据此回滚重投。
func TestARetryableReleaseOnATerminalRequestRollsTheRoundBack(t *testing.T) {
	for _, kind := range terminalKinds()[:2] {
		t.Run(kind.name, func(t *testing.T) {
			fixture := newDecisionFixture(t)
			kind.prepare(t, fixture)
			fixture.release.outcome = ports.ControlReleaseNotFormed

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AcceptanceUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED——放不掉还交回已决定，刚占下的冻结就随这一轮提交成了孤儿", result.Outcome())
			}
			if result.PendingReason() != application.ControlReleasePending {
				t.Fatalf("reason = %q, want CONTROL_RELEASE_PENDING", result.PendingReason())
			}
			if result.State() != kind.state {
				t.Fatalf("state = %q, want %q", result.State(), kind.state)
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("没有续办引用")
			}
			if len(fixture.refusals.recorded) != 0 {
				t.Fatal("可重试未形成不进释放未受理册")
			}
			if len(fixture.downstream.intents) != 0 {
				t.Fatal("回滚的一轮不该发出决定的发布意图")
			}
		})
	}
}

// Covers: AT-PS-154——一版放不掉不挡住其余各版去放，但整轮仍然回滚（多放的几笔随之撤回，不改变结论）。
func TestOneUnreleasableVersionOnATerminalRequestStillTriesTheOthersAndRollsBack(t *testing.T) {
	fixture := newDecisionFixture(t)
	withdrawn := withdrawnSupersededRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
	fixture.judgments.controlByVersion = bothVersionsOccupied()
	fixture.release.outcomeFor = map[string]ports.ControlReleaseOutcome{"version-2": ports.ControlReleaseNotFormed}
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceUndecided || result.PendingReason() != application.ControlReleasePending {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED / CONTROL_RELEASE_PENDING", result.Outcome(), result.PendingReason())
	}
	if got := releasedVersions(fixture.release); !slices.Equal(got, []string{"version-2", "version-1"}) {
		t.Fatalf("释放指名的版本 = %v, want 两版都问过", got)
	}
}

// Covers: AT-PS-155、ADR-0282 决定二——提供方确定不受理：那一笔记进释放未受理册待人处理，不回滚、不留补偿引用；终态委托上
// 重投换不来另一个答案，所以也不无限重试。重放同一笔答已有记录，册上仍是一行。
func TestADefinitelyRefusedReleaseOnATerminalRequestIsRegisteredAndNotRolledBack(t *testing.T) {
	for _, kind := range terminalKinds()[:2] {
		t.Run(kind.name, func(t *testing.T) {
			fixture := newDecisionFixture(t)
			kind.prepare(t, fixture)
			fixture.release.outcome = ports.ControlReleaseNotAccepted

			for round := 1; round <= 2; round++ {
				result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
				if err != nil {
					t.Fatalf("round %d handle: %v", round, err)
				}
				if result.Outcome() != application.AcceptanceDecided || result.State() != kind.state {
					t.Fatalf("round %d outcome = %q state = %q, want DECIDED / %q", round, result.Outcome(), result.State(), kind.state)
				}
				if !result.ControlReleaseNotAccepted() {
					t.Fatalf("round %d 没带出「释放未受理、待人处理」这一格", round)
				}
				if result.CompensationReference().String() != "" {
					t.Fatalf("round %d 确定未受理不留补偿续办引用", round)
				}
			}
			if len(fixture.refusals.recorded) != 1 {
				t.Fatalf("册上 %d 行, want 1——同一笔占用重放答已有记录", len(fixture.refusals.recorded))
			}
			if fixture.refusals.recorded[0].ControlResultID.String() != "SAC-1" {
				t.Fatalf("册上记的控制结果 = %q, want SAC-1", fixture.refusals.recorded[0].ControlResultID)
			}
		})
	}
}

// Covers: AT-PS-155、ADR-0282 决定二末条——释放口形不成三格之一同落此格，原因是本上下文自有的
// CONTROL_RELEASE_ANSWER_UNTRANSLATABLE；上抛会让这一轮在重投里无限回滚。
func TestAnUntranslatableReleaseOnATerminalRequestIsRegisteredLikeARefusal(t *testing.T) {
	fixture := newDecisionFixture(t)
	withdrawn := withdrawnRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
	fixture.release.err = errors.Join(ports.ErrUntranslatableProviderAnswer, errors.New("provider answered outside the closed set"))

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceDecided || !result.ControlReleaseNotAccepted() {
		t.Fatalf("outcome = %q notAccepted = %v, want DECIDED / true", result.Outcome(), result.ControlReleaseNotAccepted())
	}
	if len(fixture.refusals.recorded) != 1 || fixture.refusals.recorded[0].Reason.String() != "CONTROL_RELEASE_ANSWER_UNTRANSLATABLE" {
		t.Fatalf("册上 = %+v, want 一行 CONTROL_RELEASE_ANSWER_UNTRANSLATABLE", fixture.refusals.recorded)
	}
}

// Covers: AT-PS-154——确定未受理却记不下册：那一笔没放掉、也没交到人手里，提交这一轮就是留一笔谁也不知道的孤儿，所以回滚。
func TestARefusalThatCannotBeRegisteredOnATerminalRequestRollsTheRoundBack(t *testing.T) {
	fixture := newDecisionFixture(t)
	withdrawn := withdrawnRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
	fixture.release.outcome = ports.ControlReleaseNotAccepted
	fixture.refusals.err = errors.New("register unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceUndecided || result.PendingReason() != application.ControlReleaseRefusalNotRecorded {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED / CONTROL_RELEASE_REFUSAL_NOT_RECORDED",
			result.Outcome(), result.PendingReason())
	}
}

// Covers: AT-PS-154——取不到作用域锁就不放：锁序要求先锁后放，取不到锁不能退成无锁的释放。放不了与放不掉同格，整轮回滚。
func TestAnUnobtainableScopeLockOnATerminalRequestReleasesNothingAndRollsBack(t *testing.T) {
	for _, failure := range []error{errors.New("lock unavailable"), application.ErrControlScopeLockNotAssembled} {
		fixture := newDecisionFixture(t)
		withdrawn := withdrawnRequest(t)
		fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
		fixture.scopeLocks.err = failure

		result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.AcceptanceUndecided || result.PendingReason() != application.ControlReleasePending {
			t.Fatalf("outcome = %q reason = %q, want UNDECIDED / CONTROL_RELEASE_PENDING", result.Outcome(), result.PendingReason())
		}
		if fixture.release.calls != 0 {
			t.Fatal("没取到锁却发了释放")
		}
	}
}

// Covers: AT-PS-154——读不回某一版的判断：不知道它形成过什么就不能说它没占，不往下交「已决定」。
func TestAnUnreadableJudgmentOnATerminalRequestRollsTheRoundBack(t *testing.T) {
	fixture := newDecisionFixture(t)
	withdrawn := withdrawnRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &withdrawn, true
	fixture.judgments.err = errors.New("judgments unavailable")

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceUndecided || result.PendingReason() != application.RecordedJudgmentsUnavailable {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED / RECORDED_JUDGMENTS_UNAVAILABLE",
			result.Outcome(), result.PendingReason())
	}
}

func withdrawnSupersededRequest(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	withdrawn, err := supersededRequest(t).WithdrawByCustomer(domain.WithdrawalSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "withdrawal-1"),
		Authority:  mustValue(t, domain.NewWithdrawalAuthorityReference, "WITHDRAW-ROLE-1"),
		Requester:  mustValue(t, domain.NewWithdrawalRequesterReference, "CUSTOMER-CONTACT-1"),
		Reason:     mustValue(t, domain.NewWithdrawalReasonReference, "CUSTOMER_NO_LONGER_REQUIRES_SERVICE"),
		DecidedAt:  time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("withdraw superseded request: %v", err)
	}
	return withdrawn
}
