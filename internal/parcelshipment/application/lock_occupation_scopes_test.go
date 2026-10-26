package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证撤回、主动拒绝、授权处置与接受链形成决定的拒绝、停进等待受控补充两支，都在保存委托之前先取这份委托各版占用所在
// 作用域的锁（票 docs-consistency/code/99；锁序全文见 ports.PreAcceptanceControlScopeLock 头注）。保存委托之前取锁，是因为
// 接受链先取作用域锁、再保存委托；这几条路径若先保存、再在释放里取，同一份委托并发时两边互等。真库上的互等与等在锁上的
// 证据在 cmd/parcel-api 与 cmd/parcel-dispatch 的 control_scope_lock_order_test.go；这里证编排自己的顺序、去重与各格。
//
// 每一次取锁在锁替身里留下「作用域账户段 + 取锁那一刻委托是否已经保存」：账户段带结果标识，两版的作用域因此不同，次序可验。

// lockObserver 把取锁登记成「账户 先于保存 / 后于保存」，saved 回答取锁那一刻委托是否已经落过一次保存。
func lockObserver(double *scopeLockDouble, saved func() bool) *[]string {
	observed := []string{}
	double.onLock = func(scope domain.OccupationScope) {
		when := "before-save"
		if saved() {
			when = "after-save"
		}
		observed = append(observed, scope.Account()+" "+when)
	}
	return &observed
}

// assertEveryReleaseWasLockedFirst 钉「释放去认领的每一份存下的作用域，都已在保存之前锁过」：否则释放里的取锁还会落在保存之后。
func assertEveryReleaseWasLockedFirst(t *testing.T, locked []domain.OccupationScope, released []domain.OccupationScope) {
	t.Helper()
	for _, scope := range released {
		if !scope.Recorded() {
			continue
		}
		if !slices.ContainsFunc(locked, func(candidate domain.OccupationScope) bool { return candidate == scope }) {
			t.Fatalf("释放认领作用域 %s，保存之前却没锁它；锁过 %v", scope.Account(), locked)
		}
	}
}

var (
	// 两版各占一笔时的锁序：被取代的 version-1 在前、当前的 version-2 在后，与接受链开头放被取代各版再施加当前版本一致。
	bothLockedBeforeSave = []string{
		"SYN-ACCOUNT-SAC-version-1 before-save",
		"SYN-ACCOUNT-SAC-version-2 before-save",
	}
	errScopeLockUnavailable = errors.New("control scope lock is unavailable")
)

// Covers: PS ports.PreAcceptanceControlScopeLock 头注的锁序——撤回先 S 后 R：两版的作用域按版本形成顺序在保存委托之前取齐，
// 释放认领的作用域都在其中。
func TestAWithdrawalLocksEachVersionsScopeBeforeSavingTheRequest(t *testing.T) {
	fixture, command := supersededWithdrawal(t)
	observed := lockObserver(fixture.scopeLocks, func() bool { return fixture.requests.saved != nil })

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if !slices.Equal(*observed, bothLockedBeforeSave) {
		t.Fatalf("取锁 = %v, want %v", *observed, bothLockedBeforeSave)
	}
	assertEveryReleaseWasLockedFirst(t, fixture.scopeLocks.scopes, fixture.release.scopes)
	if fixture.scopeLocks.tenants[0].String() != "tenant-1" {
		t.Fatalf("取锁的租户 = %q, want tenant-1", fixture.scopeLocks.tenants[0])
	}
}

// Covers: 主动拒绝同样在保存委托之前按版本形成顺序取齐各版作用域的锁。
func TestAnActiveRejectionLocksEachVersionsScopeBeforeSavingTheRequest(t *testing.T) {
	fixture := newRejectionFixture(t)
	superseded := supersededRequest(t)
	fixture.requests.stored = &superseded
	fixture.judgments.controlByVersion = bothVersionsOccupied()
	observed := lockObserver(fixture.scopeLocks, func() bool { return fixture.requests.saved != nil })
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ActiveRejectionFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if !slices.Equal(*observed, bothLockedBeforeSave) {
		t.Fatalf("取锁 = %v, want %v", *observed, bothLockedBeforeSave)
	}
	assertEveryReleaseWasLockedFirst(t, fixture.scopeLocks.scopes, fixture.release.scopes)
}

// Covers: 授权处置的两个去向都释放占用，都先 S 后 R。
func TestAnAuthorizedDispositionLocksEachVersionsScopeBeforeSavingTheRequest(t *testing.T) {
	for name, choice := range map[string]domain.AuthorizedDispositionChoice{
		"拒绝":    domain.DisposeByRejection,
		"交客户补充": domain.DisposeByCustomerSupplement,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDispositionFixture(t)
			fixture.requests.request = awaitingDispositionRequestOn(t, supersededRequest(t))
			// version-2 不列：落回夹具的「第一项冻结成立、第二项受限」那份（SAC-1）。
			fixture.judgments.controlByVersion = map[string]domain.FinancialControlOutcome{
				"version-1": domain.FinancialControlHeld,
			}
			observed := lockObserver(fixture.scopeLocks, func() bool { return fixture.requests.saved != nil })
			command := fixture.command(t, choice)
			command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.AuthorizedDispositionRecorded {
				t.Fatalf("outcome = %q, want RECORDED", result.Outcome())
			}
			want := []string{"SYN-ACCOUNT-SAC-version-1 before-save", "SYN-ACCOUNT-SAC-1 before-save"}
			if !slices.Equal(*observed, want) {
				t.Fatalf("取锁 = %v, want %v", *observed, want)
			}
			assertEveryReleaseWasLockedFirst(t, fixture.scopeLocks.scopes, fixture.release.scopes)
		})
	}
}

// Covers: 接受链形成决定的自动拒绝：保存拒绝之前先取这份委托各版的作用域锁（接受链里这些锁施加与链开头放被取代各版时已取过，
// 这里是重入；单驱这一步时也照样先 S 后 R）。
func TestAnAutomaticRejectionLocksEachVersionsScopeBeforeSavingTheRequest(t *testing.T) {
	fixture := newDecisionFixture(t)
	superseded := supersededRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &superseded, true
	fixture.judgments.controlByVersion = bothVersionsOccupied()
	fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
	// 夹具的 saved 起手就放着被取代的那份供 Find 交回，所以「已保存」认的是它被换掉之后。
	original := fixture.requests.saved
	observed := lockObserver(fixture.scopeLocks, func() bool { return fixture.requests.saved != original })
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-2")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", result.State())
	}
	if !slices.Equal(*observed, bothLockedBeforeSave) {
		t.Fatalf("取锁 = %v, want %v", *observed, bothLockedBeforeSave)
	}
	assertEveryReleaseWasLockedFirst(t, fixture.scopeLocks.scopes, fixture.release.scopes)
}

// Covers: 接受链形成决定停进`等待受控补充`：暂停落库之后要放本版的占用，保存之前先取它的作用域锁；停进别的等待态
// （人工复核）与接受都不释放，不取。
func TestAPauseForTheCustomerLocksTheVersionsScopeBeforeSavingButOtherOutcomesLockNothing(t *testing.T) {
	pause := newDecisionFixture(t)
	pause.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
	pauseObserved := lockObserver(pause.scopeLocks, func() bool { return pause.requests.saved != nil })
	if _, err := pause.handler.Handle(context.Background(), pause.command(t)); err != nil {
		t.Fatalf("handle pause: %v", err)
	}
	if want := []string{"SYN-ACCOUNT-SAC-1 before-save"}; !slices.Equal(*pauseObserved, want) {
		t.Fatalf("停进等待受控补充：取锁 = %v, want %v", *pauseObserved, want)
	}
	assertEveryReleaseWasLockedFirst(t, pause.scopeLocks.scopes, pause.release.scopes)

	accepted := newDecisionFixture(t)
	if result, err := accepted.handler.Handle(context.Background(), accepted.command(t)); err != nil ||
		result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("handle acceptance: state = %q err = %v", result.State(), err)
	}
	if len(accepted.scopeLocks.scopes) != 0 {
		t.Fatalf("接受不释放，却取了锁 %v", accepted.scopeLocks.scopes)
	}
}

// Covers: 锁序按版本形成顺序——被取代各版依次在前、当前版本在后。只有一个被取代版本的委托分不出「被取代各版内部按形成顺序」与
// 「被取代各版内部反序」（被取代的只有一项，怎么排都是它自己在前），所以这里摆两个被取代版本：与接受链开头依次放被取代各版
// 的次序不一致时，同一份委托上两边的锁之间会成环。
func TestTheScopeLocksOfSeveralSupersededVersionsAreTakenInFormationOrderWithTheCurrentVersionLast(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	request := twiceSupersededRequest(t)
	fixture.requests.stored = &request
	fixture.judgments.controlByVersion = map[string]domain.FinancialControlOutcome{
		"version-1": domain.FinancialControlHeld,
		"version-2": domain.FinancialControlHeld,
		"version-3": domain.FinancialControlHeld,
	}
	observed := lockObserver(fixture.scopeLocks, func() bool { return fixture.requests.saved != nil })
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-3")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	want := []string{
		"SYN-ACCOUNT-SAC-version-1 before-save",
		"SYN-ACCOUNT-SAC-version-2 before-save",
		"SYN-ACCOUNT-SAC-version-3 before-save",
	}
	if !slices.Equal(*observed, want) {
		t.Fatalf("取锁 = %v, want %v——被取代各版按形成顺序、当前版本在后", *observed, want)
	}
	assertEveryReleaseWasLockedFirst(t, fixture.scopeLocks.scopes, fixture.release.scopes)
}

// Covers: 同一作用域只取一次；没有成立占用的版本、没存作用域的存量结果都不取（释放那一侧本来就不会为它们碰账本）。
func TestTheScopeLockIsTakenOncePerScopeAndOnlyForAnOccupationWithAStoredScope(t *testing.T) {
	t.Run("两版同一作用域只取一次", func(t *testing.T) {
		fixture, command := supersededWithdrawal(t)
		// 不按版本给：每一版都读回同一份控制结果（SAC-1），作用域相同。
		fixture.judgments.controlByVersion = nil
		if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.scopeLocks.scopes) != 1 {
			t.Fatalf("取锁 %d 次, want 1——同一作用域取两次在库里是重入，但编排不该靠它", len(fixture.scopeLocks.scopes))
		}
	})
	t.Run("被取代那一版没占下什么", func(t *testing.T) {
		fixture, command := supersededWithdrawal(t)
		fixture.judgments.controlByVersion["version-1"] = domain.FinancialControlOutcomeInvalid
		if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if len(fixture.scopeLocks.scopes) != 1 || fixture.scopeLocks.scopes[0].Account() != "SYN-ACCOUNT-SAC-version-2" {
			t.Fatalf("取锁 = %v, want 只有 version-2 的作用域", fixture.scopeLocks.scopes)
		}
	})
	t.Run("存量结果没存作用域", func(t *testing.T) {
		fixture := newWithdrawalFixture(t)
		unscoped := unscopedHeldControl(t)
		fixture.judgments.control = &unscoped
		result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.WithdrawalFormed {
			t.Fatalf("outcome = %q, want FORMED——没存作用域的占用不挡撤回", result.Outcome())
		}
		if len(fixture.scopeLocks.scopes) != 0 {
			t.Fatalf("没存作用域却取了锁 %v", fixture.scopeLocks.scopes)
		}
		if fixture.release.calls != 1 {
			t.Fatalf("释放 %d 次, want 1——取不取锁不改释放那一侧的落法（交提供方短路作答）", fixture.release.calls)
		}
	})
}

// unscopedHeldControl 是施加时还不存作用域的存量结果：占用成立，作用域零值。
func unscopedHeldControl(t *testing.T) domain.FinancialControlResult {
	t.Helper()
	item, err := domain.NewControlItemResult(
		domain.PrepaidFreezeControlItem, 1, domain.ControlItemSatisfied, domain.ControlBasisReference{})
	if err != nil {
		t.Fatalf("new control item result: %v", err)
	}
	result, err := domain.NewExecutedFinancialControlResult(domain.ExecutedFinancialControlSpec{
		ResultID:  mustValue(t, domain.NewFinancialControlResultID, "SAC-1"),
		Items:     []domain.ControlItemResult{item},
		JointPass: domain.AllControlsPass,
		AsOf:      formedAsOfFor(t, domain.FinancialControlJudgmentKind, controlPolicyFormedAsOf),
	})
	if err != nil {
		t.Fatalf("new executed financial control result: %v", err)
	}
	return result
}

// Covers: 撤回重放「按委托状态判、已决态不问授权」不因取锁改变：上一轮停在授权之后，委托在两轮之间被接受或撤回——重放不保存委托，
// 不取锁，不问授权；委托仍是已提交的重放（接着办）则照首次请求取齐锁再保存。
func TestAReplayOverADecidedRequestTakesNoScopeLockWhileOneOverASubmittedRequestDoes(t *testing.T) {
	stop := func(t *testing.T) (*withdrawalFixture, *withdrawalReplayStore, application.WithdrawShipmentRequestCommand) {
		t.Helper()
		fixture, store, _ := newWithdrawalReplayFixture(t)
		command := fixture.command(t)
		fixture.authorizer.err = errors.New("authority unreachable")
		if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
			t.Fatalf("handle the round that stops: %v", err)
		}
		fixture.authorizer.err = nil
		return fixture, store, command
	}

	for name, decided := range map[string]func(*testing.T) domain.ShipmentRequest{
		"已接受": acceptedRequest,
		"已撤回": withdrawnRequest,
	} {
		t.Run(name, func(t *testing.T) {
			fixture, store, command := stop(t)
			store.current = decided(t)
			authorizations := fixture.authorizer.calls

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("replay: %v", err)
			}
			if result.Outcome() != application.WithdrawalDecisionAlreadyFormed {
				t.Fatalf("outcome = %q, want DECISION_ALREADY_FORMED", result.Outcome())
			}
			if len(fixture.scopeLocks.scopes) != 0 || store.saves != 0 {
				t.Fatalf("已决态重放取了锁 %v 或保存了委托（%d 次）", fixture.scopeLocks.scopes, store.saves)
			}
			if fixture.authorizer.calls != authorizations {
				t.Fatal("已决态重放又问了授权")
			}
		})
	}

	t.Run("仍是已提交", func(t *testing.T) {
		fixture, store, command := stop(t)
		observed := lockObserver(fixture.scopeLocks, func() bool { return store.saves > 0 })

		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if result.Outcome() != application.WithdrawalFormed {
			t.Fatalf("outcome = %q, want FORMED", result.Outcome())
		}
		if want := []string{"SYN-ACCOUNT-SAC-1 before-save"}; !slices.Equal(*observed, want) {
			t.Fatalf("取锁 = %v, want %v", *observed, want)
		}
	})
}

// 以下三条证「已决态首次到达不取锁」：新来源身份（主动拒绝与授权处置没有来源保全，就是头一次读到）头一次到达一份已经决定的委托，
// 走的是撤回的 decide、主动拒绝的 ActiveRejectionAdmissible、授权处置的 AuthorizedDispositionAdmissible 自己的已决态判断，
// 而不是 resolvePreserved 重放那条在进 decide 之前就短路的路。取锁挪到这些判断之前时，
// TestAReplayOverADecidedRequestTakesNoScopeLockWhileOneOverASubmittedRequestDoes 仍绿（重放根本到不了取锁），只有这三条会红。
// 已决态不保存委托，没有行锁可倒挂，也就不为它白取锁。
//
// 三条路径里授权都先于已决态判断（授权先于一切写动作），所以这里不断言「不问授权」；断言的是取锁口零调用、不保存委托、
// 不领决定标识。

// decidedRequests 是首次到达时已经越过决定边界的两种委托：接受与客户撤回。夹具的判断读回都带一笔成立的占用，取锁若被触发就一定
// 留下痕迹。
var decidedRequests = map[string]func(*testing.T) domain.ShipmentRequest{
	"已接受": acceptedRequest,
	"已撤回": withdrawnRequest,
}

// Covers: 撤回先 S 后 R 只在会保存委托的路径上取锁——撤回：新来源身份头一次到达一份已接受或已撤回的委托，答已有决定，不取锁、不保存、不领决定标识。
func TestAFirstArrivalWithdrawalOverADecidedRequestTakesNoScopeLock(t *testing.T) {
	for name, decided := range decidedRequests {
		t.Run(name, func(t *testing.T) {
			fixture := newWithdrawalFixture(t)
			request := decided(t)
			fixture.requests.stored = &request
			command := fixture.command(t)
			if len(fixture.sources.records) != 0 {
				t.Fatal("夹具里已有被保全的来源，这就不是首次到达")
			}

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.WithdrawalDecisionAlreadyFormed {
				t.Fatalf("outcome = %q, want DECISION_ALREADY_FORMED", result.Outcome())
			}
			if _, preserved, _ := fixture.sources.FindPreserved(context.Background(), command.WithdrawalIdentity); !preserved {
				t.Fatal("这次撤回的来源身份没有被保全，走的不是首次到达的路")
			}
			if len(fixture.scopeLocks.scopes) != 0 {
				t.Fatalf("已决态首次到达却取了锁 %v", fixture.scopeLocks.scopes)
			}
			if fixture.requests.saved != nil || fixture.identities.issued != 0 {
				t.Fatalf("已决态首次到达保存了委托或领了决定标识（%d 个）", fixture.identities.issued)
			}
		})
	}
}

// Covers: 同一条规则的主动拒绝：一份已接受或已撤回的委托，答任务已完结并交回既有决定，不取锁、不保存、不领决定标识、不发释放。
func TestAnActiveRejectionOverADecidedRequestTakesNoScopeLock(t *testing.T) {
	for name, decided := range decidedRequests {
		t.Run(name, func(t *testing.T) {
			fixture := newRejectionFixture(t)
			request := decided(t)
			fixture.requests.stored = &request

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.ActiveRejectionTaskAlreadyClosed {
				t.Fatalf("outcome = %q, want TASK_ALREADY_CLOSED", result.Outcome())
			}
			if len(fixture.scopeLocks.scopes) != 0 {
				t.Fatalf("已决态首次到达却取了锁 %v", fixture.scopeLocks.scopes)
			}
			if fixture.requests.saved != nil || fixture.identities.issued != 0 || fixture.release.calls != 0 {
				t.Fatal("已决态首次到达保存了委托、领了决定标识或发了释放")
			}
		})
	}
}

// Covers: 同一条规则的授权处置：一份已接受或已撤回的委托，两个去向都答任务已完结，不取锁、不保存、不领决定标识、不发释放。
func TestAnAuthorizedDispositionOverADecidedRequestTakesNoScopeLock(t *testing.T) {
	for name, decided := range decidedRequests {
		for choiceName, choice := range map[string]domain.AuthorizedDispositionChoice{
			"拒绝":    domain.DisposeByRejection,
			"交客户补充": domain.DisposeByCustomerSupplement,
		} {
			t.Run(name+"/"+choiceName, func(t *testing.T) {
				fixture := newDispositionFixture(t)
				fixture.requests.request = decided(t)

				result, err := fixture.handler.Handle(context.Background(), fixture.command(t, choice))
				if err != nil {
					t.Fatalf("handle: %v", err)
				}
				if result.Outcome() != application.AuthorizedDispositionTaskAlreadyClosed {
					t.Fatalf("outcome = %q, want TASK_ALREADY_CLOSED", result.Outcome())
				}
				if len(fixture.scopeLocks.scopes) != 0 {
					t.Fatalf("已决态首次到达却取了锁 %v", fixture.scopeLocks.scopes)
				}
				fixture.assertNothingWritten(t)
			})
		}
	}
}

// Covers: 取不到锁与保存不了同一格——业务事实尚未形成：撤回与主动拒绝答`决定没落库`、不保存委托、不领决定标识、不发释放；
// 授权处置如其保存失败上抛；接受链自动拒绝与停进等待受控补充答`决定没落库`、不保存。
func TestAScopeLockThatCannotBeTakenLeavesTheDecisionUnformed(t *testing.T) {
	t.Run("撤回", func(t *testing.T) {
		fixture, command := supersededWithdrawal(t)
		fixture.scopeLocks.err = errScopeLockUnavailable
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.WithdrawalUndecided || result.PendingReason() != application.DecisionNotRecorded {
			t.Fatalf("outcome = %q reason = %q, want UNDECIDED / DECISION_NOT_RECORDED", result.Outcome(), result.PendingReason())
		}
		if fixture.requests.saved != nil || fixture.identities.issued != 0 || fixture.release.calls != 0 {
			t.Fatal("取锁失败却保存了委托、领了决定标识或发了释放")
		}
	})
	t.Run("主动拒绝", func(t *testing.T) {
		fixture := newRejectionFixture(t)
		fixture.scopeLocks.err = errScopeLockUnavailable
		result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ActiveRejectionUndecided || result.PendingReason() != application.DecisionNotRecorded {
			t.Fatalf("outcome = %q reason = %q, want UNDECIDED / DECISION_NOT_RECORDED", result.Outcome(), result.PendingReason())
		}
		if fixture.requests.saved != nil || fixture.identities.issued != 0 || fixture.release.calls != 0 {
			t.Fatal("取锁失败却保存了委托、领了决定标识或发了释放")
		}
	})
	t.Run("授权处置", func(t *testing.T) {
		fixture := newDispositionFixture(t)
		fixture.scopeLocks.err = errScopeLockUnavailable
		_, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
		if !errors.Is(err, errScopeLockUnavailable) {
			t.Fatalf("err = %v, want 取锁失败上抛", err)
		}
		if fixture.requests.saved != nil || fixture.release.calls != 0 {
			t.Fatal("取锁失败却保存了委托或发了释放")
		}
	})
	t.Run("自动拒绝", func(t *testing.T) {
		fixture := newDecisionFixture(t)
		fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
		fixture.scopeLocks.err = errScopeLockUnavailable
		result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.PendingReason() != application.DecisionNotRecorded || fixture.requests.saved != nil || fixture.release.calls != 0 {
			t.Fatalf("reason = %q saved = %v release = %d, want 决定没落库、不保存、不释放",
				result.PendingReason(), fixture.requests.saved != nil, fixture.release.calls)
		}
	})
	t.Run("停进等待受控补充", func(t *testing.T) {
		fixture := newDecisionFixture(t)
		fixture.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
		fixture.scopeLocks.err = errScopeLockUnavailable
		result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.PendingReason() != application.DecisionNotRecorded || fixture.requests.saved != nil {
			t.Fatalf("reason = %q saved = %v, want 决定没落库——暂停没落库就不能交回等待受控补充",
				result.PendingReason(), fixture.requests.saved != nil)
		}
	})
}

// Covers: 有要排队的作用域而取锁口没接上是装配缺口，与取锁失败落同一格，不静默放过。
func TestAnUnassembledScopeLockStopsAWithdrawalThatHasAnOccupation(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	handler := application.NewWithdrawShipmentRequestHandler(application.WithdrawShipmentRequestDeps{
		Sources:    fixture.sources,
		Requests:   fixture.requests,
		Authorizer: fixture.authorizer,
		Judgments:  fixture.judgments,
		Recorder:   &judgmentRequestStore{},
		Release:    fixture.release,
		Refusals:   fixture.refusals,
		Identities: fixture.identities,
		Clock:      fixedClock{at: handlerClockAt},
	})
	result, err := handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalUndecided || result.PendingReason() != application.DecisionNotRecorded {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED / DECISION_NOT_RECORDED", result.Outcome(), result.PendingReason())
	}
	if fixture.requests.saved != nil {
		t.Fatal("没接取锁口却保存了委托")
	}
}

// Covers: 读不回判断的一轮不取锁也不停下：撤回不等财务释放才生效（UC-PS-005），保存之后的释放读同一份判断、同样读不回，
// 会把那一版留成补偿续办。
func TestAnUnreadableJudgmentDoesNotStopAWithdrawalBeforeItsScopeLock(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.judgments.err = errors.New("judgments unavailable")
	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if len(fixture.scopeLocks.scopes) != 0 {
		t.Fatalf("读不回判断却取了锁 %v", fixture.scopeLocks.scopes)
	}
	if result.CompensationReference().String() == "" {
		t.Fatal("读不回判断的一版没有留补偿续办")
	}
}
