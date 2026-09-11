package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var verificationAt = time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type executionFactViewDouble struct {
	facts []domain.ExecutionFact
	err   error
}

func (double *executionFactViewDouble) LoadExecutionFacts(
	_ context.Context,
	_ domain.TenantID,
	_ domain.RegulatoryDecisionID,
) ([]domain.ExecutionFact, error) {
	if double.err != nil {
		return nil, double.err
	}
	return double.facts, nil
}

type verificationStoreDouble struct {
	byKey map[ports.VerificationKey]domain.DispositionVerification
	saved int
}

func (double *verificationStoreDouble) FindByKey(
	_ context.Context,
	key ports.VerificationKey,
) (domain.DispositionVerification, bool, error) {
	verification, found := double.byKey[key]
	return verification, found, nil
}

func (double *verificationStoreDouble) Save(
	_ context.Context,
	key ports.VerificationKey,
	verification domain.DispositionVerification,
) (ports.VerificationSaveOutcome, error) {
	if _, exists := double.byKey[key]; exists {
		return ports.VerificationAlreadyRecorded, nil
	}
	double.byKey[key] = verification
	double.saved++
	return ports.VerificationSaved, nil
}

type verificationDownstreamDouble struct {
	intents []ports.VerificationHandoffIntent
	err     error
}

func (double *verificationDownstreamDouble) HandOffVerification(
	_ context.Context,
	intent ports.VerificationHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type verifyFixture struct {
	handler    *application.VerifyDispositionHandler
	facts      *executionFactViewDouble
	store      *verificationStoreDouble
	downstream *verificationDownstreamDouble
}

func newVerifyFixture(t *testing.T) *verifyFixture {
	t.Helper()
	fixture := &verifyFixture{
		facts:      &executionFactViewDouble{},
		store:      &verificationStoreDouble{byKey: map[ports.VerificationKey]domain.DispositionVerification{}},
		downstream: &verificationDownstreamDouble{},
	}
	fixture.handler = application.NewVerifyDispositionHandler(application.VerifyDispositionDeps{
		Facts:      fixture.facts,
		Store:      fixture.store,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: verificationAt},
	})
	return fixture
}

func destructionDecision(t *testing.T) domain.RegulatoryDecision {
	t.Helper()
	decision, err := domain.NewRegulatoryDecision(domain.RegulatoryDecisionSpec{
		ID:         mustValue(t, domain.NewRegulatoryDecisionID, "decision-1"),
		Authority:  mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		Action:     mustValue(t, domain.NewLegalActionReference, "DESTRUCTION"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		Quantity:   domain.RequiredQuantity{Provided: true, Units: 2},
		ReceivedAt: verificationAt.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("new regulatory decision: %v", err)
	}
	return decision
}

func executionFact(t *testing.T, reference string, units int) domain.ExecutionFact {
	t.Helper()
	fact, err := domain.NewExecutionFact(domain.ExecutionFactSpec{
		Executor:   mustValue(t, domain.NewExecutorReference, "node-origin"),
		Fact:       mustValue(t, domain.NewExecutionFactReference, reference),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "parcel-1"),
		Units:      units,
		OccurredAt: verificationAt.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("new execution fact: %v", err)
	}
	return fact
}

func verifyCommand(t *testing.T) application.VerifyDispositionCommand {
	t.Helper()
	return application.VerifyDispositionCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Decision: destructionDecision(t),
	}
}

// Covers: CC CONTEXT「处置执行核对」经编排端到端——无事实即证据不足如实入册（空清单
// 是领域答案不是依赖故障，`AT-CC-233` 的证据不足半边「事实不足时形成核对证据不足」）；
// 同决定同事实集指纹只出一版（重放返原核对不重出版本），新事实到达换指纹换版且两版
// 并存（部分执行→再次执行的版本化比较，CONTEXT「数量差异和再次执行事实」；`AT-CC-237`「执行事实迟到……按事实时间、有效性和范围更新新核对版本；历史结果和已完成范围保留」）。
func TestVerificationIsIdempotentPerFactSetAndVersionsAccrue(t *testing.T) {
	fixture := newVerifyFixture(t)

	insufficient, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	if err != nil {
		t.Fatalf("insufficient handle: %v", err)
	}
	verification, _ := insufficient.Verification()
	if verification.Conclusion() != domain.ExecutionEvidenceInsufficient {
		t.Fatalf("conclusion = %q; 决定推导不出执行", verification.Conclusion())
	}

	replay, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.VerificationExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d; 同事实集重复出了版本", fixture.store.saved)
	}

	fixture.facts.facts = []domain.ExecutionFact{executionFact(t, "DESTRUCTION-EXEC/1", 1)}
	partial, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	if err != nil {
		t.Fatalf("partial handle: %v", err)
	}
	partialVerification, _ := partial.Verification()
	if partial.Outcome() != application.VerificationRecorded ||
		partialVerification.Conclusion() != domain.ExecutionPartiallyCovered {
		t.Fatalf("outcome = %q conclusion = %q", partial.Outcome(), partialVerification.Conclusion())
	}
	if fixture.store.saved != 2 {
		t.Fatalf("saved = %d; 新事实到达该换版且两版并存", fixture.store.saved)
	}

	fixture.facts.facts = append(fixture.facts.facts, executionFact(t, "DESTRUCTION-EXEC/2", 1))
	covered, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	if err != nil {
		t.Fatalf("covered handle: %v", err)
	}
	coveredVerification, _ := covered.Verification()
	if coveredVerification.Conclusion() != domain.ExecutionCovered {
		t.Fatalf("conclusion = %q", coveredVerification.Conclusion())
	}
}

// Covers: 编排纪律——事实读不回未决（与证据不足分开：一个是依赖故障一个是领域答案）；
// 意图投递失败核对不翻留续办、重放重发同一份（`AT-CC-250`「协作事项或核对已提交，但
// 事件首次投递失败→保持原事项/核对结果有效，只重试同一发布意图」）。
func TestFactFailuresStallAndIntentsRetry(t *testing.T) {
	fixture := newVerifyFixture(t)
	fixture.facts.err = errors.New("facts unreachable")

	stalled, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	if err != nil {
		t.Fatalf("stalled handle: %v", err)
	}
	if stalled.Outcome() != application.VerificationUndecided ||
		stalled.UndecidedReason() != application.ExecutionFactsUnavailable {
		t.Fatalf("outcome = %q/%q", stalled.Outcome(), stalled.UndecidedReason())
	}

	fixture.facts.err = nil
	fixture.downstream.err = errors.New("downstream unreachable")
	first, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.VerificationRecorded {
		t.Fatalf("outcome = %q; 投递失败不得翻核对", first.Outcome())
	}
	if first.HandoffReference() == "" {
		t.Fatal("首投失败没有留续办引用")
	}

	fixture.downstream.err = nil
	replay, err := fixture.handler.Handle(context.Background(), verifyCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.VerificationExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if len(fixture.downstream.intents) != 1 || fixture.store.saved != 1 {
		t.Fatalf("intents = %d saved = %d; 重发的必须是原核对那一份", len(fixture.downstream.intents), fixture.store.saved)
	}
}
