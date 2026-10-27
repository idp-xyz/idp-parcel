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

// 本文件证取消先后冲突的决定编排（ADR-0295 决定一至四）：先取包裹取消边界锁再读册；认定取消依据不成立时作废依据这份取消的终局不采用行并交续办信封，
// 交来源方更正只留痕；已决定的与并发里输掉的读回先到的那一份，不作废、不交第二封信封；册上没有的、别的租户的与几段对不上的一律「统一不可见」；
// 少接一个件、去向不在封闭集合内、取不到锁都响亮报错而不是悄悄跳过。

type decisionStoreDouble struct {
	steps    *[]string
	byKey    map[ports.CancellationOrderConflictKey]ports.CancellationOrderConflictDecision
	saveWith ports.CancellationOrderConflictDecisionSaveOutcome
	winner   ports.CancellationOrderConflictDecision
	err      error
}

func (double *decisionStoreDouble) FindConflictDecision(
	_ context.Context,
	key ports.CancellationOrderConflictKey,
) (ports.CancellationOrderConflictDecision, bool, error) {
	*double.steps = append(*double.steps, "find-decision")
	decision, found := double.byKey[key]
	return decision, found, double.err
}

func (double *decisionStoreDouble) DecideCancellationOrderConflict(
	_ context.Context,
	decision ports.CancellationOrderConflictDecision,
) (ports.CancellationOrderConflictDecisionSaveOutcome, ports.CancellationOrderConflictDecision, error) {
	*double.steps = append(*double.steps, "save-decision")
	if double.saveWith != ports.CancellationOrderConflictDecisionSaveOutcomeInvalid {
		return double.saveWith, double.winner, nil
	}
	double.byKey[decision.Key] = decision
	return ports.CancellationOrderConflictDecisionRecorded, decision, nil
}

type voiderDouble struct {
	steps  *[]string
	calls  []string
	voided int
	err    error
}

func (double *voiderDouble) VoidCancellationRefusedFinals(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
	cancellation domain.ParcelCancellationID,
) (int, error) {
	*double.steps = append(*double.steps, "void")
	double.calls = append(double.calls, tenant.String()+"/"+parcel.String()+"/"+cancellation.String())
	return double.voided, double.err
}

type notUpheldHandoffDouble struct {
	steps   *[]string
	intents []ports.CancellationNotUpheldHandoffIntent
	err     error
}

func (double *notUpheldHandoffDouble) HandOffCancellationNotUpheld(
	_ context.Context,
	intent ports.CancellationNotUpheldHandoffIntent,
) error {
	*double.steps = append(*double.steps, "handoff")
	double.intents = append(double.intents, intent)
	return double.err
}

type decideFixture struct {
	steps     []string
	lock      *boundaryLockDouble
	conflicts *conflictRegisterDouble
	decisions *decisionStoreDouble
	voider    *voiderDouble
	handoff   *notUpheldHandoffDouble
	deps      application.DecideCancellationOrderConflictDeps
	command   application.DecideCancellationOrderConflictCommand
}

var decideAt = time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

func newDecideFixture(t *testing.T) *decideFixture {
	t.Helper()
	fixture := &decideFixture{
		lock:      &boundaryLockDouble{},
		conflicts: &conflictRegisterDouble{},
	}
	fixture.lock.onLock = func() { fixture.steps = append(fixture.steps, "lock") }
	fixture.decisions = &decisionStoreDouble{steps: &fixture.steps, byKey: map[ports.CancellationOrderConflictKey]ports.CancellationOrderConflictDecision{}}
	fixture.voider = &voiderDouble{steps: &fixture.steps, voided: 2}
	fixture.handoff = &notUpheldHandoffDouble{steps: &fixture.steps}

	conflict := judgedNetworkConflict(t, "PARCEL-1", "PCXL-1")
	fixture.conflicts.records = append(fixture.conflicts.records, ports.CancellationOrderConflictRecord{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"), Conflict: conflict, DetectedAt: decideAt.Add(-time.Hour),
	})
	fixture.command = application.DecideCancellationOrderConflictCommand{
		Tenant:       mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:       conflict.Parcel(),
		BoundaryKind: conflict.Boundary().Kind,
		BoundaryRef:  conflict.Boundary().Reference,
		Cancellation: conflict.Cancellation(),
		Kind:         domain.CancellationNotUpheld,
		Decider:      mustValue(t, domain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#alice"),
		Basis:        mustValue(t, domain.NewCancellationOrderConflictDecisionBasis, "SYN-NODE-SCAN-AT-0800"),
	}
	fixture.deps = application.DecideCancellationOrderConflictDeps{
		Conflicts: fixture.conflicts, Decisions: fixture.decisions, Voider: fixture.voider, Handoff: fixture.handoff,
		BoundaryLock: fixture.lock, Clock: fixedClock{at: decideAt},
	}
	return fixture
}

func (fixture *decideFixture) handle(t *testing.T) (application.DecideCancellationOrderConflictResult, error) {
	t.Helper()
	return application.NewDecideCancellationOrderConflictHandler(fixture.deps).Handle(t.Context(), fixture.command)
}

// judgedNetworkConflict 判出一份网络服务收寄与取消的冲突：取消的业务时间等于收寄的发生时间（相等按冲突）。
func judgedNetworkConflict(t *testing.T, parcel, cancellationID string) domain.CancellationOrderConflict {
	t.Helper()
	at := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustValue(t, domain.NewParcelCancellationID, cancellationID),
		Parcel:      mustValue(t, domain.NewDeclaredParcelID, parcel),
		Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-a"),
		Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/1"),
		Reason:      mustValue(t, domain.NewCancellationReasonReference, "CHANGED_MIND"),
		RequestedAt: at,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide cancellation: %v", err)
	}
	conflict, conflicted, err := domain.JudgeCancellationOrder(cancellation, domain.CancellationBoundaryFact{
		Kind:       domain.NetworkIntakeBoundary,
		Reference:  mustValue(t, domain.NewCancellationBoundaryReference, "NODE_INTAKE/v1"),
		OccurredAt: at,
	})
	if err != nil || !conflicted {
		t.Fatalf("judge order: conflicted=%v err=%v", conflicted, err)
	}
	return conflict
}

// Covers: AT-PS-166, AT-PS-169；ADR-0295 决定一至四——认定取消依据不成立：先取锁，再读册，再留痕，再作废，最后交信封；结果带出本次作废的行数。
func TestNotUpheldDecisionLocksFirstThenRecordsVoidsAndHandsOffTheResume(t *testing.T) {
	fixture := newDecideFixture(t)

	result, err := fixture.handle(t)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Outcome() != application.CancellationOrderConflictDecided || result.VoidedRefusals() != 2 {
		t.Fatalf("outcome = %s voided = %d, want DECIDED 与作废 2 行", result.Outcome(), result.VoidedRefusals())
	}
	if got, want := strings.Join(fixture.steps, ","), "lock,find-decision,save-decision,void,handoff"; got != want {
		t.Fatalf("步骤 = %s, want %s——锁必须先于册上的读，信封最后", got, want)
	}
	if len(fixture.lock.locked) != 1 || fixture.lock.locked[0] != "tenant-1/PARCEL-1" {
		t.Fatalf("取锁 = %v, want 本包裹一次", fixture.lock.locked)
	}
	if len(fixture.voider.calls) != 1 || fixture.voider.calls[0] != "tenant-1/PARCEL-1/PCXL-1" {
		t.Fatalf("作废 = %v, want 依据这份取消", fixture.voider.calls)
	}
	if len(fixture.handoff.intents) != 1 ||
		fixture.handoff.intents[0].Decision.Kind != domain.CancellationNotUpheld ||
		fixture.handoff.intents[0].Decision.DecidedAt != decideAt {
		t.Fatalf("信封意图 = %+v, want 一封，带本次决定与决定时刻", fixture.handoff.intents)
	}
	if result.Decision().Decider.String() != "https://id.syn.example#alice" {
		t.Fatalf("决定人 = %q", result.Decision().Decider)
	}
}

// Covers: AT-PS-166, AT-PS-171；ADR-0295 决定二——交来源方更正只留痕：取消保持有效、没有被挡住的来源要重驱，所以不作废、不交信封。
func TestReferredDecisionOnlyRecordsAndNeitherVoidsNorHandsOff(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.command.Kind = domain.BoundaryFactReferred
	fixture.deps.Voider, fixture.deps.Handoff = nil, nil

	result, err := fixture.handle(t)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Outcome() != application.CancellationOrderConflictDecided || result.Decision().Kind != domain.BoundaryFactReferred {
		t.Fatalf("outcome = %s kind = %s", result.Outcome(), result.Decision().Kind)
	}
	if got, want := strings.Join(fixture.steps, ","), "lock,find-decision,save-decision"; got != want {
		t.Fatalf("步骤 = %s, want %s", got, want)
	}
}

// Covers: AT-PS-166；ADR-0295 决定一——已决定的冲突读回先到的那一份：不覆盖、不作废、不交第二封信封，即便这一次说的是另一种去向。
func TestAnAlreadyDecidedConflictReadsTheEarlierDecisionAndDoesNothingElse(t *testing.T) {
	fixture := newDecideFixture(t)
	earlier := ports.CancellationOrderConflictDecision{
		Key: ports.KeyOfConflictEntry(entryOf(fixture.conflicts.records[0])), Kind: domain.BoundaryFactReferred,
		Decider: mustValue(t, domain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#bob"),
		Basis:   mustValue(t, domain.NewCancellationOrderConflictDecisionBasis, "EARLIER"), DecidedAt: decideAt.Add(-time.Minute),
	}
	fixture.decisions.byKey[earlier.Key] = earlier

	result, err := fixture.handle(t)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Outcome() != application.CancellationOrderConflictAlreadyDecidedOutcome || result.Decision() != earlier {
		t.Fatalf("outcome = %s decision = %+v, want 已被决定且是先到的那一份", result.Outcome(), result.Decision())
	}
	if len(fixture.voider.calls) != 0 || len(fixture.handoff.intents) != 0 || contains(fixture.steps, "save-decision") {
		t.Fatalf("已决定仍在写：步骤 %v 作废 %v 信封 %v", fixture.steps, fixture.voider.calls, fixture.handoff.intents)
	}
}

// Covers: AT-PS-166；并发里输掉的一方：保存时才发现先被别人决定，读回赢家，不作废、不交信封。
func TestALostRaceReadsTheWinnerAndDoesNotVoidOrHandOff(t *testing.T) {
	fixture := newDecideFixture(t)
	winner := ports.CancellationOrderConflictDecision{
		Key: ports.KeyOfConflictEntry(entryOf(fixture.conflicts.records[0])), Kind: domain.CancellationNotUpheld,
		Decider: mustValue(t, domain.NewCancellationOrderConflictDeciderReference, "https://id.syn.example#bob"),
		Basis:   mustValue(t, domain.NewCancellationOrderConflictDecisionBasis, "WINNER"), DecidedAt: decideAt.Add(-time.Second),
	}
	fixture.decisions.saveWith, fixture.decisions.winner = ports.CancellationOrderConflictAlreadyDecided, winner

	result, err := fixture.handle(t)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Outcome() != application.CancellationOrderConflictAlreadyDecidedOutcome || result.Decision() != winner {
		t.Fatalf("outcome = %s decision = %+v", result.Outcome(), result.Decision())
	}
	if len(fixture.voider.calls) != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("输家仍在作废或交信封：作废 %v 信封 %v", fixture.voider.calls, fixture.handoff.intents)
	}
}

// Covers: AT-PS-166；ADR-0295 决定一——统一不可见：册上没有这份冲突、别的租户、几段对不上，同一个答案，什么也不写。
func TestAnUnknownOrForeignConflictIsNotVisibleAndNothingIsWritten(t *testing.T) {
	cases := map[string]func(*decideFixture, *testing.T){
		"别的租户": func(fixture *decideFixture, t *testing.T) {
			fixture.command.Tenant = mustValue(t, domain.NewTenantID, "tenant-2")
		},
		"别的取消": func(fixture *decideFixture, t *testing.T) {
			fixture.command.Cancellation = mustValue(t, domain.NewParcelCancellationID, "PCXL-9")
		},
		"别的边界那一代": func(fixture *decideFixture, t *testing.T) {
			fixture.command.BoundaryRef = mustValue(t, domain.NewCancellationBoundaryReference, "NODE_INTAKE/v9")
		},
		"别的边界种类": func(fixture *decideFixture, t *testing.T) {
			fixture.command.BoundaryKind = domain.CarrierFirstEffectivePickupBoundary
		},
		"别的包裹": func(fixture *decideFixture, t *testing.T) {
			fixture.command.Parcel = mustValue(t, domain.NewDeclaredParcelID, "PARCEL-9")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newDecideFixture(t)
			mutate(fixture, t)
			_, err := fixture.handle(t)
			if !errors.Is(err, application.ErrCancellationOrderConflictNotVisible) {
				t.Fatalf("err = %v, want 统一不可见", err)
			}
			if contains(fixture.steps, "save-decision") || len(fixture.voider.calls) != 0 || len(fixture.handoff.intents) != 0 {
				t.Fatalf("不可见仍在写：步骤 %v", fixture.steps)
			}
		})
	}
}

// Covers: AT-PS-166；保存时册上已无此份（读与写之间被移走）同答统一不可见。
func TestAConflictThatVanishesBetweenReadAndSaveIsNotVisible(t *testing.T) {
	fixture := newDecideFixture(t)
	fixture.decisions.saveWith = ports.CancellationOrderConflictNotRegistered
	if _, err := fixture.handle(t); !errors.Is(err, application.ErrCancellationOrderConflictNotVisible) {
		t.Fatalf("err = %v, want 统一不可见", err)
	}
	if len(fixture.voider.calls) != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("不可见仍在作废或交信封")
	}
}

// Covers: AT-PS-166；响亮报错：取不到锁、去向不在封闭集合内、读册读决定写决定作废交信封任一步出错——整笔失败，不在锁外读写，也不吞掉错误。
func TestFailuresAreLoudAndNothingRunsOutsideTheLock(t *testing.T) {
	boom := errors.New("boom")
	t.Run("取不到锁", func(t *testing.T) {
		fixture := newDecideFixture(t)
		fixture.lock.err = boom
		if _, err := fixture.handle(t); !errors.Is(err, application.ErrCancellationBoundaryNotLocked) {
			t.Fatalf("err = %v", err)
		}
		if contains(fixture.steps, "find-decision") || contains(fixture.steps, "save-decision") {
			t.Fatalf("取不到锁仍在读写：%v", fixture.steps)
		}
	})
	t.Run("去向不在封闭集合内", func(t *testing.T) {
		fixture := newDecideFixture(t)
		fixture.command.Kind = domain.CancellationOrderConflictDecisionKindInvalid
		if _, err := fixture.handle(t); !errors.Is(err, application.ErrCancellationOrderConflictDecisionKindUnknown) {
			t.Fatalf("err = %v", err)
		}
		if len(fixture.lock.locked) != 0 {
			t.Fatalf("坏命令不该取锁")
		}
	})
	for name, arrange := range map[string]func(*decideFixture){
		"读册":  func(fixture *decideFixture) { fixture.conflicts.err = boom },
		"读决定": func(fixture *decideFixture) { fixture.decisions.err = boom },
		"作废":  func(fixture *decideFixture) { fixture.voider.err = boom },
		"交信封": func(fixture *decideFixture) { fixture.handoff.err = boom },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDecideFixture(t)
			arrange(fixture)
			if _, err := fixture.handle(t); !errors.Is(err, boom) {
				t.Fatalf("err = %v, want 原样上抛", err)
			}
		})
	}
}

// Covers: 装配缺陷响亮：少接任一必需件答`未装配`；交来源方更正不需要作废与信封两个件，认定取消不成立需要。
func TestAnUnassembledDecisionHandlerIsLoud(t *testing.T) {
	for name, remove := range map[string]func(*application.DecideCancellationOrderConflictDeps){
		"册":    func(deps *application.DecideCancellationOrderConflictDeps) { deps.Conflicts = nil },
		"决定留痕": func(deps *application.DecideCancellationOrderConflictDeps) { deps.Decisions = nil },
		"锁":    func(deps *application.DecideCancellationOrderConflictDeps) { deps.BoundaryLock = nil },
		"时钟":   func(deps *application.DecideCancellationOrderConflictDeps) { deps.Clock = nil },
		"作废":   func(deps *application.DecideCancellationOrderConflictDeps) { deps.Voider = nil },
		"信封":   func(deps *application.DecideCancellationOrderConflictDeps) { deps.Handoff = nil },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDecideFixture(t)
			remove(&fixture.deps)
			if _, err := fixture.handle(t); !errors.Is(err, application.ErrCancellationOrderConflictDecisionNotAssembled) {
				t.Fatalf("err = %v, want 未装配", err)
			}
			if len(fixture.steps) != 0 {
				t.Fatalf("未装配仍在跑：%v", fixture.steps)
			}
		})
	}
}

func entryOf(record ports.CancellationOrderConflictRecord) ports.CancellationOrderConflictEntry {
	return ports.CancellationOrderConflictEntry{
		TenantID: record.TenantID, Parcel: record.Conflict.Parcel(), Cancellation: record.Conflict.Cancellation(),
		Boundary: record.Conflict.Boundary(),
	}
}

func contains(steps []string, step string) bool {
	for _, got := range steps {
		if got == step {
			return true
		}
	}
	return false
}
