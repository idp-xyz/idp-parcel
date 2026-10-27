package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证面单渠道终局判断在冲突那一格对解开决定的落点（ADR-0295 决定二）：这一代收寄已被认定有误、交来源方更正的，取消照旧在先（`CANCELLATION_STANDS`），
// 不再答`不形成`/冲突，不写第二行冲突；册上另有没被决定的冲突，不带收寄的两路仍答冲突；决定读不动按依赖未决；读到被认定不成立的取消是取消读口没守住排除，
// 响亮报错。

// conflictedPickup 摆一份已入册的冲突：有效取消的业务时间晚于收寄，收寄那一路把冲突记进册、答`不形成`；交回装好决定视图的处理方与那份冲突的身份。
func conflictedPickup(t *testing.T) (*labelServiceFinalFixture, *decisionViewDouble, ports.CancellationOrderConflictKey) {
	t.Helper()
	fixture := newLabelServiceFinalFixture(t)
	fixture.final.cancellations.cancelled = true
	fixture.final.cancellations.cancellation = cancelledAt(t, labelServicePickupAt.Add(time.Hour))
	command := labelServiceCommand(t)
	command.FirstEffectivePickup = labelServicePickup(t)
	if result, err := fixture.handler.Handle(context.Background(), command); err != nil ||
		result.Outcome() != application.LabelServiceNotFinalOutcome || len(fixture.conflicts.records) != 1 {
		t.Fatalf("收寄那一路 = %q err=%v 冲突 %d 行, want 不形成并记下冲突", result.Outcome(), err, len(fixture.conflicts.records))
	}
	record := fixture.conflicts.records[0]
	decisions := newDecisionView()
	fixture.handler = application.NewJudgeLabelServiceFinalHandler(application.JudgeLabelServiceFinalDeps{
		Transactions:  fixture.transactions,
		Registers:     fixture.registers,
		Cancellations: fixture.final.cancellations,
		Validity:      fixture.validity,
		Conflicts:     fixture.conflicts,
		Decisions:     decisions,
		Adoption:      fixture.final.handler,
		Clock:         fixedClock{at: labelServiceClosureAt.Add(time.Hour)},
		BoundaryLock:  fixture.lock,
	})
	return fixture, decisions, ports.KeyOfConflict(record.TenantID, record.Conflict)
}

// Covers: AT-PS-171, AT-PS-172；ADR-0295 决定二——收寄这一代已被认定有误、交来源方更正：取消照旧在先，答`取消在先`而不是`不形成`/冲突，不写第二行冲突，不走采用路径。
func TestAPickupReferredToTheSourceLeavesTheCancellationStanding(t *testing.T) {
	fixture, decisions, key := conflictedPickup(t)
	decisions.decide(t, key, domain.BoundaryFactReferred)
	command := labelServiceCommand(t)
	command.FirstEffectivePickup = labelServicePickup(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.LabelServiceCancellationStandsOutcome {
		t.Fatalf("outcome = %q, want CANCELLATION_STANDS", result.Outcome())
	}
	if _, delegated := result.Adoption(); delegated || fixture.final.finals.saved != 0 || len(fixture.conflicts.records) != 1 {
		t.Fatalf("delegated=%v finals=%d conflicts=%d", delegated, fixture.final.finals.saved, len(fixture.conflicts.records))
	}
}

// Covers: AT-PS-172；没有被决定的冲突照旧答冲突（决定视图接入但这份冲突没被决定）。
func TestAnUndecidedPickupConflictStillAnswersTheConflict(t *testing.T) {
	fixture, _, _ := conflictedPickup(t)
	command := labelServiceCommand(t)
	command.FirstEffectivePickup = labelServicePickup(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.LabelServiceNotFinalOutcome ||
		result.Verdict().Reason() != domain.LabelServiceCancellationOrderConflict {
		t.Fatalf("outcome = %q reason = %q, want 不形成/冲突", result.Outcome(), result.Verdict().Reason())
	}
}

// Covers: AT-PS-172；不带收寄的两路（交易与关闭）读册作答：册上指向这份取消的冲突都被决定为交来源方更正，取消在先；还有一行没被决定，就仍答冲突。
func TestTheClosureRouteReadsTheDecisionsOfTheRecordedConflicts(t *testing.T) {
	fixture, decisions, key := conflictedPickup(t)
	decisions.decide(t, key, domain.BoundaryFactReferred)

	result, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t))
	if err != nil || result.Outcome() != application.LabelServiceCancellationStandsOutcome {
		t.Fatalf("全部冲突都已交来源方更正：outcome = %q err=%v, want CANCELLATION_STANDS", result.Outcome(), err)
	}

	later := labelServicePickup(t)
	later.Version = mustValue(t, domain.NewCarrierFirstEffectivePickupFactVersion, "TF-TRACK-FACT-7/v2")
	command := labelServiceCommand(t)
	command.FirstEffectivePickup = later
	if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("更正出的新一代：%v", err)
	}
	if len(fixture.conflicts.records) != 2 {
		t.Fatalf("冲突册 %d 行, want 2——新一代按决定二重判，又是冲突就另起一行", len(fixture.conflicts.records))
	}
	result, err = fixture.handler.Handle(context.Background(), labelServiceCommand(t))
	if err != nil || result.Outcome() != application.LabelServiceNotFinalOutcome {
		t.Fatalf("还有一行没被决定：outcome = %q err=%v, want 不形成/冲突", result.Outcome(), err)
	}
}

// Covers: AT-PS-172；决定读不动是依赖缺口：收寄那一路与关闭那一路都按依赖未决交回，不答取消在先也不答冲突。
func TestAnUnreadableDecisionStopsTheLabelJudgmentAsADependencyGap(t *testing.T) {
	fixture, decisions, _ := conflictedPickup(t)
	decisions.err = errors.New("decision store down")
	withPickup := labelServiceCommand(t)
	withPickup.FirstEffectivePickup = labelServicePickup(t)

	for name, command := range map[string]application.JudgeLabelServiceFinalCommand{
		"带收寄": withPickup, "不带收寄": labelServiceCommand(t),
	} {
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.Outcome() != application.LabelServiceJudgmentUndecided ||
			result.UndecidedReason() != application.LabelConflictRegisterUnavailable {
			t.Fatalf("%s: outcome = %q reason = %q, want JUDGMENT_UNDECIDED/CONFLICT_REGISTER_UNAVAILABLE", name, result.Outcome(), result.UndecidedReason())
		}
	}
}

// Covers: AT-PS-172；取消读口交回了一份已被认定不成立的取消，是读口没守住排除：响亮报错，不折成依赖未决重投。
func TestACancellationSetAsideButStillReadIsLoudInTheLabelJudgment(t *testing.T) {
	fixture, decisions, key := conflictedPickup(t)
	decisions.decide(t, key, domain.CancellationNotUpheld)

	if _, err := fixture.handler.Handle(context.Background(), labelServiceCommand(t)); err == nil {
		t.Fatal("读到被认定不成立的取消，没有响亮报错")
	}
}
