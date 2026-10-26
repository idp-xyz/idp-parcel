package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证释放答复的三格在各释放点上逐格落点（ADR-0282 决定一至三）：`已收口`不留任何东西；`可重试未形成`照旧留补偿
// 续办（各路的老用例守着）；`确定未受理`在终局各路与暂停那一路记进释放未受理册、业务事实照常成立、结果带出这一格、不留
// 补偿引用，在接受链开头那一步响亮上抛、不折成内部重试。端口形不成三格之一（答出集合之外）与`确定未受理`同落。

// releasePathRun 是一条释放路走一轮之后要比的几样：业务事实成没成立、结果带没带出这一格、补偿引用、册上记了什么。
type releasePathRun struct {
	established  bool
	notAccepted  bool
	compensation domain.OwnershipContinuationReference
	refusals     *controlReleaseRefusalDouble
	release      *controlReleaseDouble
}

// releasePaths 是记册的那几路。每一路自己摆出「本版已占下一笔」的委托，把释放替身与册替身交给 arrange 去拨，再走一轮。
func releasePaths() map[string]func(*testing.T, func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun {
	return map[string]func(*testing.T, func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun{
		"撤回": func(t *testing.T, arrange func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun {
			fixture := newWithdrawalFixture(t)
			arrange(fixture.release, fixture.refusals)
			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			return releasePathRun{
				established:  result.Outcome() == application.WithdrawalFormed,
				notAccepted:  result.ControlReleaseNotAccepted(),
				compensation: result.CompensationReference(),
				refusals:     fixture.refusals,
				release:      fixture.release,
			}
		},
		"主动拒绝": func(t *testing.T, arrange func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun {
			fixture := newRejectionFixture(t)
			arrange(fixture.release, fixture.refusals)
			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			return releasePathRun{
				established:  result.Outcome() == application.ActiveRejectionFormed,
				notAccepted:  result.ControlReleaseNotAccepted(),
				compensation: result.CompensationReference(),
				refusals:     fixture.refusals,
				release:      fixture.release,
			}
		},
		"授权处置·拒绝": func(t *testing.T, arrange func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun {
			return disposeOnce(t, domain.DisposeByRejection, arrange)
		},
		"授权处置·交客户补充": func(t *testing.T, arrange func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun {
			return disposeOnce(t, domain.DisposeByCustomerSupplement, arrange)
		},
		"自动拒绝": func(t *testing.T, arrange func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun {
			fixture := newDecisionFixture(t)
			fixture.judgments.reachability["parcel-2"] = domain.ReachabilityUnreachable
			arrange(fixture.release, fixture.refusals)
			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			return releasePathRun{
				established:  result.Outcome() == application.AcceptanceDecided && result.State() == domain.ShipmentRequestRejected,
				notAccepted:  result.ControlReleaseNotAccepted(),
				compensation: result.CompensationReference(),
				refusals:     fixture.refusals,
				release:      fixture.release,
			}
		},
		"停进等待受控补充": func(t *testing.T, arrange func(*controlReleaseDouble, *controlReleaseRefusalDouble)) releasePathRun {
			fixture := newDecisionFixture(t)
			fixture.judgments.reachability["parcel-2"] = domain.ReachabilityInsufficientEvidence
			arrange(fixture.release, fixture.refusals)
			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			return releasePathRun{
				established:  result.PendingReason() == application.CustomerSupplementPending && fixture.requests.saved != nil,
				notAccepted:  result.ControlReleaseNotAccepted(),
				compensation: result.CompensationReference(),
				refusals:     fixture.refusals,
				release:      fixture.release,
			}
		},
	}
}

func disposeOnce(
	t *testing.T,
	choice domain.AuthorizedDispositionChoice,
	arrange func(*controlReleaseDouble, *controlReleaseRefusalDouble),
) releasePathRun {
	t.Helper()
	fixture := newDispositionFixture(t)
	arrange(fixture.release, fixture.refusals)
	result, err := fixture.handler.Handle(context.Background(), fixture.command(t, choice))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	return releasePathRun{
		established:  result.Outcome() == application.AuthorizedDispositionRecorded,
		notAccepted:  result.ControlReleaseNotAccepted(),
		compensation: result.CompensationReference(),
		refusals:     fixture.refusals,
		release:      fixture.release,
	}
}

// Covers: AT-PS-115、AT-PS-035 释放被确定不受理那一格；ADR-0282 决定二；UC-PS-005「也不得把释放失败改写为撤回失败」——
// 提供方确定不受理的释放，各路同一判法：业务事实
// 照常成立、不回滚；那一笔记进释放未受理册（委托、版本、控制关联、存下的作用域与提供方的原因），结果带出这一格；不留补偿
// 续办引用——重放一万次也是同一个答复，留引用只会让调用方以为系统会把它补上。
func TestARefusedReleaseIsRecordedForHandlingOnEveryPathWithoutACompensation(t *testing.T) {
	for name, run := range releasePaths() {
		t.Run(name, func(t *testing.T) {
			got := run(t, func(release *controlReleaseDouble, _ *controlReleaseRefusalDouble) {
				release.outcome = ports.ControlReleaseNotAccepted
			})
			if !got.established {
				t.Fatal("释放被拒收，业务事实却没成立——一次释放失败改写了业务结果")
			}
			if !got.notAccepted {
				t.Fatal("结果没带出「释放确定未受理、待人处理」这一格")
			}
			if got.compensation.String() != "" {
				t.Fatalf("拒收已成定局却留了补偿引用 %q——那一笔没有东西推得动", got.compensation)
			}
			if len(got.refusals.recorded) == 0 {
				t.Fatal("拒收的那一笔没记进释放未受理册，没有人知道它还占着")
			}
			refusal := got.refusals.recorded[0]
			if refusal.Reason.String() != "SA-RELEASE_REQUEST_NOT_ACCEPTED" {
				t.Fatalf("册上原因 = %q, want 提供方给的 SA-RELEASE_REQUEST_NOT_ACCEPTED", refusal.Reason)
			}
			if refusal.TenantID.String() != "tenant-1" || refusal.CustomerAccountID.String() != "customer-1" ||
				refusal.ShipmentRequestID.String() != "request-1" {
				t.Fatalf("册上的范围 = %s/%s/%s, want tenant-1/customer-1/request-1",
					refusal.TenantID, refusal.CustomerAccountID, refusal.ShipmentRequestID)
			}
			if refusal.ControlResultID.String() != got.release.controlResultID && len(got.refusals.recorded) == 1 {
				t.Fatalf("册上的控制关联 = %q, 释放请求的是 %q", refusal.ControlResultID, got.release.controlResultID)
			}
			if !refusal.Scope.Recorded() || refusal.Scope != got.release.scopes[0] {
				t.Fatalf("册上的作用域 = %+v, want 释放交出的那一份 %+v", refusal.Scope, got.release.scopes[0])
			}
			if !refusal.RecordedAt.Equal(handlerClockAt) {
				t.Fatalf("登记时间 = %v, want 编排时钟 %v", refusal.RecordedAt, handlerClockAt)
			}
		})
	}
}

// Covers: ADR-0282 决定二末句——端口形不成三格之一（答出集合之外）时这几路同样落待人处理格，原因是本上下文自己的
// CONTROL_RELEASE_ANSWER_UNTRANSLATABLE；上抛会让已成立的业务事实随整笔回滚。
func TestAnUntranslatableReleaseAnswerIsRecordedForHandlingOnEveryPath(t *testing.T) {
	for name, run := range releasePaths() {
		t.Run(name, func(t *testing.T) {
			got := run(t, func(release *controlReleaseDouble, _ *controlReleaseRefusalDouble) {
				release.err = errors.Join(ports.ErrUntranslatableProviderAnswer, errors.New("release outcome 9"))
			})
			if !got.established || !got.notAccepted || got.compensation.String() != "" {
				t.Fatalf("established = %v notAccepted = %v compensation = %q, want 成立 / 带出这一格 / 无补偿引用",
					got.established, got.notAccepted, got.compensation)
			}
			if len(got.refusals.recorded) == 0 ||
				got.refusals.recorded[0].Reason.String() != "CONTROL_RELEASE_ANSWER_UNTRANSLATABLE" {
				t.Fatalf("册上 = %+v, want 一笔 CONTROL_RELEASE_ANSWER_UNTRANSLATABLE", got.refusals.recorded)
			}
		})
	}
}

// Covers: ADR-0282 决定二——记不下册（写不进或没接上）时那一笔改留补偿续办，原因 CONTROL_RELEASE_REFUSAL_NOT_RECORDED，与
// `释放未收口`各派各的引用；结果不带出这一格，业务事实照样成立。
func TestARefusalThatCannotBeRecordedLeavesItsOwnCompensation(t *testing.T) {
	for name, run := range releasePaths() {
		t.Run(name, func(t *testing.T) {
			unrecorded := run(t, func(release *controlReleaseDouble, refusals *controlReleaseRefusalDouble) {
				release.outcome = ports.ControlReleaseNotAccepted
				refusals.err = errors.New("register unavailable")
			})
			if !unrecorded.established {
				t.Fatal("册写不进，业务事实却没成立")
			}
			if unrecorded.notAccepted {
				t.Fatal("册上没记下却答「已记进释放未受理册」")
			}
			if unrecorded.compensation.String() == "" {
				t.Fatal("拒收了又没记下，却连补偿引用也没留——这一笔从此没人知道")
			}
			pending := run(t, func(release *controlReleaseDouble, _ *controlReleaseRefusalDouble) {
				release.outcome = ports.ControlReleaseNotFormed
			})
			if pending.compensation.String() == "" || pending.notAccepted {
				t.Fatalf("可重试未形成 compensation = %q notAccepted = %v, want 留补偿引用、不带出拒收这一格",
					pending.compensation, pending.notAccepted)
			}
			if len(pending.refusals.recorded) != 0 {
				t.Fatalf("可重试未形成却记进了释放未受理册：%+v", pending.refusals.recorded)
			}
			if unrecorded.compensation.String() == pending.compensation.String() {
				t.Fatalf("记不下册与放不掉共用一个引用 %q——续办方按引用查回来的会是另一种缺口", pending.compensation)
			}
		})
	}
}

// Covers: ADR-0282 决定四——释放交出的作用域是控制结果上存下的那一份，不在释放时重新推导。
func TestAReleaseCarriesTheScopeRecordedWithTheControlResult(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(fixture.release.scopes) != 1 || fixture.release.scopes[0] != occupationScopeOf(t, "SAC-1") {
		t.Fatalf("释放交出的作用域 = %+v, want 控制结果上存下的那一份 %+v",
			fixture.release.scopes, occupationScopeOf(t, "SAC-1"))
	}
}

// Covers: ADR-0282 决定二——一份委托两版各占一笔，一版拒收、一版放不掉：拒收那一版记册，放不掉那一版留补偿，结果两样都带。
func TestARefusedAndAPendingVersionAreSettledEachInTheirOwnCell(t *testing.T) {
	fixture, command := supersededWithdrawal(t)
	fixture.release.outcomeFor = map[string]ports.ControlReleaseOutcome{
		"version-2": ports.ControlReleaseNotFormed,
		"version-1": ports.ControlReleaseNotAccepted,
	}
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalFormed {
		t.Fatalf("outcome = %q, want FORMED", result.Outcome())
	}
	if !result.ControlReleaseNotAccepted() || result.CompensationReference().String() == "" {
		t.Fatalf("notAccepted = %v compensation = %q, want 两样都带", result.ControlReleaseNotAccepted(), result.CompensationReference())
	}
	if len(fixture.refusals.recorded) != 1 || fixture.refusals.recorded[0].SubmissionVersion.String() != "version-1" {
		t.Fatalf("册上 = %+v, want 只记被拒收的 version-1", fixture.refusals.recorded)
	}
}

// Covers: AT-PS-115 与 AT-PS-068「重复撤回」；ADR-0282 决定二——重放撤回再遇到同一笔拒收，册上仍是那一笔（已有记录），结果照旧带出这一格。
func TestARepeatedWithdrawalFindsTheSameRefusalOnTheRegister(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.release.outcome = ports.ControlReleaseNotAccepted
	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("first: %v", err)
	}
	// 委托替身不把撤回写回去，重放那一轮读到的是已撤回的那一份，照 AT-PS-068 那条老用例拨。
	fixture.requests.withdrawn = true
	repeat, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if repeat.Outcome() != application.WithdrawalDecisionAlreadyFormed || !repeat.ControlReleaseNotAccepted() {
		t.Fatalf("repeat outcome = %q notAccepted = %v, want DECISION_ALREADY_FORMED 并带出这一格",
			repeat.Outcome(), repeat.ControlReleaseNotAccepted())
	}
	if len(fixture.refusals.recorded) != 1 {
		t.Fatalf("册上 %d 笔, want 1——同一笔占用不是第二份", len(fixture.refusals.recorded))
	}
}

// Covers: AT-PS-116；ADR-0282 决定三；ADR-0081 决定三——接受链判新版本之前放被取代那一版，提供方确定不受理时响亮上抛（哨兵
// ErrControlReleaseNotAccepted），不停在本步内部重试，新版本一步不判，也不记册（这一轮整笔回滚）。
func TestARefusedSupersededReleaseFailsTheChainLoudly(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
	fixture.release.outcome = ports.ControlReleaseNotAccepted

	result, err := fixture.handler.Handle(context.Background(), command)
	if !errors.Is(err, application.ErrControlReleaseNotAccepted) {
		t.Fatalf("err = %v（outcome = %q reason = %q）, want ErrControlReleaseNotAccepted——拒收折成内部重试会一直挡着新版本",
			err, result.Outcome(), result.PendingReason())
	}
	if fixture.reachability.calls != 0 || fixture.controller.calls != 0 {
		t.Fatalf("旧占用拒收了，链却判了 %d 个成员的可达性、发起了 %d 次资金占用",
			fixture.reachability.calls, fixture.controller.calls)
	}
}

// Covers: 同上——释放口形不成三格之一时，接受链原样上抛那个错误（包着端口哨兵），不折成内部重试。
func TestAnUntranslatableSupersededReleaseFailsTheChainLoudly(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
	fixture.release.err = errors.Join(ports.ErrUntranslatableProviderAnswer, errors.New("release outcome 9"))

	_, err := fixture.handler.Handle(context.Background(), command)
	if !errors.Is(err, ports.ErrUntranslatableProviderAnswer) {
		t.Fatalf("err = %v, want 包着 ErrUntranslatableProviderAnswer 的错误", err)
	}
}

// Covers: 票 code/71「控制端口同此」；ADR-0081 决定三——控制端口答出译不动的东西时，财务控制那一步原样上抛，不再折成
// FINANCIAL_CONTROL_UNAVAILABLE 内部重试；依赖调不通照旧落那一格。
func TestAnUntranslatableControlAnswerFailsTheChainLoudly(t *testing.T) {
	fixture := newChainFixture(t)
	fixture.controller.err = errors.Join(ports.ErrUntranslatableProviderAnswer, errors.New("applied control came back released"))

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, ports.ErrUntranslatableProviderAnswer) {
		t.Fatalf("err = %v（reason = %q）, want 包着 ErrUntranslatableProviderAnswer 的错误", err, result.PendingReason())
	}
}
