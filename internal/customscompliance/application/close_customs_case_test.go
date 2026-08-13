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

var closureAt = time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

type inventoryViewDouble struct {
	items      []domain.ClosureObligationItem
	configured bool
	err        error
}

func (double *inventoryViewDouble) LoadObligationItems(
	_ context.Context,
	_ domain.TenantID,
	_ string,
	_ time.Time,
) ([]domain.ClosureObligationItem, bool, error) {
	if double.err != nil {
		return nil, false, double.err
	}
	return double.items, double.configured, nil
}

type caseClosureStoreDouble struct {
	byCase map[string]*domain.CustomsCaseClosure
	saved  int
}

func (double *caseClosureStoreDouble) FindByCase(
	_ context.Context,
	_ domain.TenantID,
	caseRef string,
) (*domain.CustomsCaseClosure, bool, error) {
	closure, found := double.byCase[caseRef]
	return closure, found, nil
}

func (double *caseClosureStoreDouble) Save(
	_ context.Context,
	_ domain.TenantID,
	closure *domain.CustomsCaseClosure,
) (ports.CaseClosureSaveOutcome, error) {
	if _, exists := double.byCase[closure.CaseRef()]; exists {
		return ports.CaseClosureAlreadyRecorded, nil
	}
	double.byCase[closure.CaseRef()] = closure
	double.saved++
	return ports.CaseClosureSaved, nil
}

type closureDownstreamDouble struct {
	intents []ports.CaseClosureHandoffIntent
	err     error
}

func (double *closureDownstreamDouble) HandOffClosure(
	_ context.Context,
	intent ports.CaseClosureHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type closeCaseFixture struct {
	handler    *application.CloseCustomsCaseHandler
	inventory  *inventoryViewDouble
	store      *caseClosureStoreDouble
	downstream *closureDownstreamDouble
}

func newCloseCaseFixture(t *testing.T) *closeCaseFixture {
	t.Helper()
	fixture := &closeCaseFixture{
		inventory:  &inventoryViewDouble{configured: true},
		store:      &caseClosureStoreDouble{byCase: map[string]*domain.CustomsCaseClosure{}},
		downstream: &closureDownstreamDouble{},
	}
	fixture.handler = application.NewCloseCustomsCaseHandler(application.CloseCustomsCaseDeps{
		Inventory:  fixture.inventory,
		Store:      fixture.store,
		Downstream: fixture.downstream,
		Clock:      fixedClock{at: closureAt},
	})
	return fixture
}

func closeCommand(t *testing.T) application.CloseCustomsCaseCommand {
	t.Helper()
	return application.CloseCustomsCaseCommand{
		TenantID:  mustValue(t, domain.NewTenantID, "tenant-1"),
		CaseRef:   "customs-case-1",
		CutoffAt:  closureAt.Add(-time.Hour),
		DecidedBy: "customs-case-owner",
	}
}

func concludedItem(obligation string) domain.ClosureObligationItem {
	return domain.ClosureObligationItem{
		Obligation: obligation,
		Scope:      "declaration-unit-1",
		State:      domain.ObligationConcluded,
		Basis:      "RELEASE/final",
	}
}

// Covers: CC CONTEXT 硬句 218「任一义务未终结或未有效承接都阻止案件关闭；单个案件
// 不存在部分关闭」的编排面——未解决项阻止关闭带清单（业务负向，恢复动作是逐项处置
// 不是重试）；全部终结或有效承接后整案一次关闭；已关案件重放返原关闭不出第二份。
// 点名 `AT-CC-307`「所有依据项均已终结或有效移交……→形成不可覆盖关闭决定」与
// `AT-CC-308`「相同关闭请求和相同内容再次到达→返回已有关闭结果，不形成第二决定」。
func TestClosureIsBlockedItemByItemAndClosesOnceWhole(t *testing.T) {
	fixture := newCloseCaseFixture(t)
	fixture.inventory.items = []domain.ClosureObligationItem{
		concludedItem("DUTY_SETTLEMENT"),
		{
			Obligation: "DISPOSITION_EXECUTION",
			Scope:      "declaration-unit-1",
			State:      domain.ObligationUnresolved,
			Basis:      "verification/pending",
		},
	}

	blocked, err := fixture.handler.Handle(context.Background(), closeCommand(t))
	if err != nil {
		t.Fatalf("blocked handle: %v", err)
	}
	if blocked.Outcome() != application.ClosureBlocked {
		t.Fatalf("outcome = %q", blocked.Outcome())
	}
	if len(blocked.Unresolved()) != 1 || blocked.Unresolved()[0].Obligation != "DISPOSITION_EXECUTION" {
		t.Fatalf("unresolved = %#v; 被谁挡着必须一目了然", blocked.Unresolved())
	}
	if fixture.store.saved != 0 {
		t.Fatal("被阻止的关闭落了库")
	}

	fixture.inventory.items = []domain.ClosureObligationItem{
		concludedItem("DUTY_SETTLEMENT"),
		{
			Obligation: "DISPOSITION_EXECUTION",
			Scope:      "declaration-unit-1",
			State:      domain.ObligationHandedOver,
			Basis:      "handover/accepted",
			HandedTo:   "regulatory-broker-1",
		},
	}
	closed, err := fixture.handler.Handle(context.Background(), closeCommand(t))
	if err != nil {
		t.Fatalf("closed handle: %v", err)
	}
	if closed.Outcome() != application.CaseClosed {
		t.Fatalf("outcome = %q", closed.Outcome())
	}

	replay, err := fixture.handler.Handle(context.Background(), closeCommand(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.CaseAlreadyClosed {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d; 一案出了第二份关闭", fixture.store.saved)
	}
}

// Covers: 编排纪律——义务目录未登记未决（不是「没有义务所以可关」）；目录读不回未决
// 分格；意图投递失败关闭不翻留续办、重放重发同一份。
func TestInventoryGapsStallAndClosureIntentsRetry(t *testing.T) {
	fixture := newCloseCaseFixture(t)
	fixture.inventory.configured = false

	unconfigured, err := fixture.handler.Handle(context.Background(), closeCommand(t))
	if err != nil {
		t.Fatalf("unconfigured handle: %v", err)
	}
	if unconfigured.Outcome() != application.CloseCaseUndecided ||
		unconfigured.UndecidedReason() != application.ObligationInventoryUnconfigured {
		t.Fatalf("outcome = %q/%q", unconfigured.Outcome(), unconfigured.UndecidedReason())
	}

	fixture.inventory.configured = true
	fixture.inventory.err = errors.New("inventory unreachable")
	stalled, err := fixture.handler.Handle(context.Background(), closeCommand(t))
	if err != nil {
		t.Fatalf("stalled handle: %v", err)
	}
	if stalled.UndecidedReason() != application.ObligationInventoryUnavailable {
		t.Fatalf("reason = %q", stalled.UndecidedReason())
	}

	fixture.inventory.err = nil
	fixture.inventory.items = []domain.ClosureObligationItem{concludedItem("DUTY_SETTLEMENT")}
	fixture.downstream.err = errors.New("downstream unreachable")
	held, err := fixture.handler.Handle(context.Background(), closeCommand(t))
	if err != nil {
		t.Fatalf("held handle: %v", err)
	}
	if held.Outcome() != application.CaseClosed || held.HandoffReference() == "" {
		t.Fatalf("outcome = %q ref = %q", held.Outcome(), held.HandoffReference())
	}

	fixture.downstream.err = nil
	resent, err := fixture.handler.Handle(context.Background(), closeCommand(t))
	if err != nil {
		t.Fatalf("resend handle: %v", err)
	}
	if resent.Outcome() != application.CaseAlreadyClosed || resent.HandoffReference() != "" {
		t.Fatalf("outcome = %q ref = %q", resent.Outcome(), resent.HandoffReference())
	}
	if len(fixture.downstream.intents) != 1 || fixture.store.saved != 1 {
		t.Fatalf("intents = %d saved = %d; 重发的必须是原关闭那一份", len(fixture.downstream.intents), fixture.store.saved)
	}
}
