package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证收寄采用在冲突那一格对解开决定的落点（ADR-0295 决定二、四）：冲突入册时带上被挡住的来源作回放原料；这一代边界事实已被认定有误、交来源方
// 更正的，不采用并指名那份决定，不停在冲突上也不写第二行冲突；决定读不动按依赖未决回滚重投；读到被认定不成立的取消是取消读口没守住排除，响亮报错。

type decisionViewDouble struct {
	byKey map[ports.CancellationOrderConflictKey]ports.CancellationOrderConflictDecision
	err   error
	asked int
}

func newDecisionView() *decisionViewDouble {
	return &decisionViewDouble{byKey: map[ports.CancellationOrderConflictKey]ports.CancellationOrderConflictDecision{}}
}

func (double *decisionViewDouble) FindConflictDecision(
	_ context.Context,
	key ports.CancellationOrderConflictKey,
) (ports.CancellationOrderConflictDecision, bool, error) {
	double.asked++
	if double.err != nil {
		return ports.CancellationOrderConflictDecision{}, false, double.err
	}
	decision, found := double.byKey[key]
	return decision, found, nil
}

func (double *decisionViewDouble) decide(
	t *testing.T,
	key ports.CancellationOrderConflictKey,
	kind domain.CancellationOrderConflictDecisionKind,
) {
	t.Helper()
	double.byKey[key] = ports.CancellationOrderConflictDecision{
		Key: key, Kind: kind,
		Decider:   mustValue(t, domain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#alice"),
		Basis:     mustValue(t, domain.NewCancellationOrderConflictDecisionBasis, "SYN-BASIS"),
		DecidedAt: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC),
	}
}

// conflictedIntake 摆一份已入册的冲突：取消的业务时间晚于收寄发生，第一次处理把冲突记进册、停在等决定；交回装好决定视图的处理方与那份冲突的身份。
func conflictedIntake(t *testing.T) (*intakeFixture, *decisionViewDouble, ports.CancellationOrderConflictKey) {
	t.Helper()
	fixture := newIntakeFixture(t)
	fixture.cancellations.cancellation = cancelledAt(t, intakeHappenedAt.Add(time.Hour))
	fixture.cancellations.cancelled = true
	if result, err := fixture.handler.Handle(context.Background(), adoptCommand(t)); err != nil ||
		result.UndecidedReason() != application.IntakeCancellationOrderConflict {
		t.Fatalf("第一次处理 = %q/%q err=%v, want 停在冲突上", result.Outcome(), result.UndecidedReason(), err)
	}
	if len(fixture.conflicts.records) != 1 {
		t.Fatalf("冲突册行数 = %d, want 1", len(fixture.conflicts.records))
	}
	record := fixture.conflicts.records[0]
	decisions := newDecisionView()
	deps := fixture.deps()
	deps.Decisions = decisions
	fixture.handler = application.NewAdoptNetworkIntakeHandler(deps)
	return fixture, decisions, ports.KeyOfConflict(record.TenantID, record.Conflict)
}

// Covers: AT-PS-173；ADR-0295 决定四——冲突入册时带上被挡住的来源规格与取消标识作回放原料，续办凭它把本来源重新交回采用编排。
func TestAnIntakeConflictCarriesItsSourceAsReplayMaterial(t *testing.T) {
	fixture, _, _ := conflictedIntake(t)

	replay := fixture.conflicts.records[0].Replay
	if replay == nil || replay.Intake == nil || replay.Final != nil {
		t.Fatalf("回放原料 = %+v, want 带收寄来源规格", replay)
	}
	if replay.Cancellation.String() != "cancel-1" || *replay.Intake != adoptCommand(t).Source {
		t.Fatalf("回放原料 = %+v, want 指名 cancel-1 与命令里那份来源规格原样", replay)
	}
}

// Covers: AT-PS-171, AT-PS-173；ADR-0295 决定二——这一代边界事实已被认定有误、交来源方更正：不采用并指名那份决定，不形成责任起点，不写第二行冲突；
// 重放按已有结果作答。
func TestAnIntakeWhoseConflictWasReferredToTheSourceIsNotAdopted(t *testing.T) {
	fixture, decisions, key := conflictedIntake(t)
	decisions.decide(t, key, domain.BoundaryFactReferred)

	result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.IntakeSourceNotAdopted || result.Basis().String() != "BOUNDARY_FACT_REFERRED/cancel-1" {
		t.Fatalf("outcome = %q basis = %q, want SOURCE_NOT_ADOPTED 指名认定有误的决定与取消", result.Outcome(), result.Basis())
	}
	record, present := result.Record()
	if !present || record.Adopted {
		t.Fatalf("record = %#v present = %v; 不采用记录保留物理来源引用", record, present)
	}
	if len(fixture.conflicts.records) != 1 {
		t.Fatalf("冲突册行数 = %d, want 仍是 1——已决定的冲突不再写第二行", len(fixture.conflicts.records))
	}
	if len(fixture.downstream.intents) != 1 || fixture.downstream.intents[0].Record.Adopted {
		t.Fatalf("发布意图 = %d, want 一份不采用意图（下游对不采用也有消费）", len(fixture.downstream.intents))
	}

	replay, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil || replay.Outcome() != application.IntakeExistingResult {
		t.Fatalf("重放 = %q err=%v, want 按已有结果作答", replay.Outcome(), err)
	}
	if fixture.adoptions.saved != 1 {
		t.Fatalf("采用记录 %d 份, want 1", fixture.adoptions.saved)
	}
}

// Covers: AT-PS-173；没有决定的冲突停在等决定上：决定视图接入但这份冲突没被决定，不采用、不形成责任起点，冲突不翻倍。
func TestAnUndecidedConflictStillWaitsWhenTheDecisionViewIsWired(t *testing.T) {
	fixture, decisions, _ := conflictedIntake(t)

	result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.UndecidedReason() != application.IntakeCancellationOrderConflict || !result.UndecidedReason().AwaitsDecision() {
		t.Fatalf("reason = %q, want 仍等业务决定", result.UndecidedReason())
	}
	if decisions.asked == 0 || fixture.adoptions.saved != 0 || len(fixture.conflicts.records) != 1 {
		t.Fatalf("asked=%d saved=%d conflicts=%d", decisions.asked, fixture.adoptions.saved, len(fixture.conflicts.records))
	}
}

// Covers: AT-PS-173；决定读不动是依赖缺口（修依赖、重投就会读到），不是等业务决定：不入账、不采用、不写。
func TestAnUnreadableDecisionIsADependencyGapNotAWaitingConflict(t *testing.T) {
	fixture, decisions, _ := conflictedIntake(t)
	decisions.err = errors.New("decision store down")

	result, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.UndecidedReason() != application.IntakeConflictRegisterUnavailable || result.UndecidedReason().AwaitsDecision() {
		t.Fatalf("reason = %q, want CONFLICT_REGISTER_UNAVAILABLE 且不等决定", result.UndecidedReason())
	}
	if fixture.adoptions.saved != 0 {
		t.Fatal("读不动决定仍落了采用记录")
	}
}

// Covers: AT-PS-173；取消读口交回了一份已被认定不成立的取消，是读口没守住排除：响亮报错，不当冲突再入账一次，也不悄悄采用。
func TestACancellationThatWasSetAsideButIsStillReadIsLoud(t *testing.T) {
	fixture, decisions, key := conflictedIntake(t)
	decisions.decide(t, key, domain.CancellationNotUpheld)

	_, err := fixture.handler.Handle(context.Background(), adoptCommand(t))
	if err == nil || !strings.Contains(err.Error(), "set aside") {
		t.Fatalf("err = %v, want 响亮报错", err)
	}
	if fixture.adoptions.saved != 0 || len(fixture.conflicts.records) != 1 {
		t.Fatalf("saved=%d conflicts=%d", fixture.adoptions.saved, len(fixture.conflicts.records))
	}
}
