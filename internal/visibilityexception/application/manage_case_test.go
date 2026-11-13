package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	caseEstablishedAt = time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)
	caseDecidedAt     = time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC)
)

// caseStoreDouble 按（租户，案件标识）存案件，推进带「册上仍是 From 那一格」的条件——与真库同一个语义。
type caseStoreDouble struct {
	byKey     map[string]*domain.ExceptionCase
	operators map[string][]string
	err       error
	// staleOnce 非空时，下一次 FindCase 交回这份旧案件（只一次）：模拟「读到之后、落库之前另一笔先落了」。
	staleOnce *domain.ExceptionCase
	// calls 记下各口被调的先后，证归并先锁后读。
	calls []string
}

func (double *caseStoreDouble) LockCases(_ context.Context, _ domain.TenantID, ids ...domain.CaseID) error {
	if double.err != nil {
		return double.err
	}
	call := "lock"
	for _, id := range ids {
		call += " " + id.String()
	}
	double.calls = append(double.calls, call)
	return nil
}

func caseKey(tenant domain.TenantID, id domain.CaseID) string {
	return tenant.String() + "|" + id.String()
}

func (double *caseStoreDouble) FindCase(_ context.Context, tenant domain.TenantID, id domain.CaseID) (*domain.ExceptionCase, bool, error) {
	if double.err != nil {
		return nil, false, double.err
	}
	double.calls = append(double.calls, "find "+id.String())
	if stale := double.staleOnce; stale != nil && stale.ID() == id {
		double.staleOnce = nil
		return stale, true, nil
	}
	stored, found := double.byKey[caseKey(tenant, id)]
	if !found {
		return nil, false, nil
	}
	copied, err := domain.RehydrateCase(stored.Snapshot())
	return copied, err == nil, err
}

func (double *caseStoreDouble) AdvanceCase(_ context.Context, advance ports.CaseAdvance) (ports.CaseAdvanceOutcome, error) {
	if double.err != nil {
		return ports.CaseAdvanceOutcomeInvalid, double.err
	}
	key := caseKey(advance.Tenant, advance.Case.ID())
	stored, found := double.byKey[key]
	if !found || stored.Phase() != advance.From {
		return ports.CasePhaseMoved, nil
	}
	double.byKey[key] = advance.Case
	double.operators[key] = append(double.operators[key], advance.Operator)
	return ports.CaseAdvanced, nil
}

type caseFixture struct {
	handler *application.ManageCaseHandler
	store   *caseStoreDouble
	tenant  domain.TenantID
}

func newCaseFixture(t *testing.T) *caseFixture {
	t.Helper()
	store := &caseStoreDouble{byKey: map[string]*domain.ExceptionCase{}, operators: map[string][]string{}}
	handler, err := application.NewManageCaseHandler(application.ManageCaseDeps{Cases: store, Clock: fixedClock{at: caseDecidedAt}})
	if err != nil {
		t.Fatal(err)
	}
	return &caseFixture{handler: handler, store: store, tenant: mustValue(t, domain.NewTenantID, "tenant-1")}
}

// seed 在册上放一份待响应的案件。
func (fixture *caseFixture) seed(t *testing.T, tenant domain.TenantID, id string) domain.CaseID {
	t.Helper()
	caseID := mustValue(t, domain.NewCaseID, id)
	number, err := domain.NewDocumentNumber("EXC-" + id)
	if err != nil {
		t.Fatal(err)
	}
	established, err := domain.EstablishCase(caseID, number,
		mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		mustValue(t, domain.NewImpactScopeReference, "scope-1"),
		mustValue(t, domain.NewResponsibleTeamReference, "team-1"),
		caseEstablishedAt)
	if err != nil {
		t.Fatalf("establish %s: %v", id, err)
	}
	fixture.store.byKey[caseKey(tenant, caseID)] = established
	return caseID
}

const operatorAlice, operatorBob = "https://issuer#alice", "https://issuer#bob"

// Covers: UC-VE-005「案件主生命周期只使用 待响应 → 处理中 → 已关闭」与「首次响应……分别形成」——接单把待响应的案件推到
// 处理中，首次响应时间取这一刻、接单人是认证出来的操作者；再接一次答已在处理中，首次响应时间与接单人都不变；随后以
// 明确结论关闭；已关闭的案件再接单、再关闭都答已关闭，结论不被改写。
func TestACaseIsTakenUpOnceThenClosedWithAConclusion(t *testing.T) {
	fixture := newCaseFixture(t)
	id := fixture.seed(t, fixture.tenant, "case-1")

	taken, err := fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorAlice})
	if err != nil || taken.Outcome() != application.CaseTakenUp {
		t.Fatalf("take up: outcome = %q err = %v", taken.Outcome(), err)
	}
	if current, _ := taken.Case(); current.Phase() != domain.CaseInProgress || !current.FirstResponseAt().Equal(caseDecidedAt) {
		t.Fatalf("接单后的案件：phase %q firstResponse %v", current.Phase(), current.FirstResponseAt())
	}

	again, err := fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorBob})
	if err != nil || again.Outcome() != application.CaseAlreadyInProgress {
		t.Fatalf("再接单：outcome = %q err = %v, want ALREADY_IN_PROGRESS", again.Outcome(), err)
	}
	if operators := fixture.store.operators[caseKey(fixture.tenant, id)]; len(operators) != 1 || operators[0] != operatorAlice {
		t.Fatalf("接单人 = %v——第二次接单不该落任何东西", operators)
	}

	closed, err := fixture.handler.Close(context.Background(), application.CloseCaseCommand{
		TenantID: fixture.tenant, Case: id, Conclusion: "  已恢复：改派后妥投  ", Operator: operatorBob,
	})
	if err != nil || closed.Outcome() != application.CaseClosedWithConclusion {
		t.Fatalf("close: outcome = %q err = %v", closed.Outcome(), err)
	}
	if snapshot := fixture.store.byKey[caseKey(fixture.tenant, id)].Snapshot(); snapshot.Conclusion != "已恢复：改派后妥投" || !snapshot.ClosedAt.Equal(caseDecidedAt) {
		t.Fatalf("关闭后的案件：%+v", snapshot)
	}

	for name, attempt := range map[string]func() (application.ManageCaseResult, error){
		"再接单": func() (application.ManageCaseResult, error) {
			return fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorAlice})
		},
		"再关闭": func() (application.ManageCaseResult, error) {
			return fixture.handler.Close(context.Background(), application.CloseCaseCommand{TenantID: fixture.tenant, Case: id, Conclusion: "另一个结论", Operator: operatorAlice})
		},
	} {
		result, err := attempt()
		if err != nil || result.Outcome() != application.CaseAlreadyClosed {
			t.Fatalf("%s：outcome = %q err = %v, want ALREADY_CLOSED", name, result.Outcome(), err)
		}
	}
	if conclusion := fixture.store.byKey[caseKey(fixture.tenant, id)].Snapshot().Conclusion; conclusion != "已恢复：改派后妥投" {
		t.Fatalf("结论被后来的关闭改写成了 %q", conclusion)
	}
}

// Covers: 关闭与归并的边界——待响应的案件可以不经接单直接关闭（判定不成立）；没有结论不关；单写「已归并」这个词不关，
// 那个结论只由归并形成；不在册的案件、另一个租户的同名案件都答不在册；没有操作者的请求不受理。
func TestClosureNeedsAConclusionAndACaseOfThisTenant(t *testing.T) {
	fixture := newCaseFixture(t)
	id := fixture.seed(t, fixture.tenant, "case-1")
	other := mustValue(t, domain.NewTenantID, "tenant-2")

	for name, test := range map[string]struct {
		command application.CloseCaseCommand
		want    application.ManageCaseOutcome
	}{
		"没有结论":     {application.CloseCaseCommand{TenantID: fixture.tenant, Case: id, Conclusion: "   ", Operator: operatorAlice}, application.CaseNotAccepted},
		"单写已归并":    {application.CloseCaseCommand{TenantID: fixture.tenant, Case: id, Conclusion: "MERGED", Operator: operatorAlice}, application.CaseNotAccepted},
		"没有操作者":    {application.CloseCaseCommand{TenantID: fixture.tenant, Case: id, Conclusion: "判定不成立"}, application.CaseNotAccepted},
		"不在册":      {application.CloseCaseCommand{TenantID: fixture.tenant, Case: mustValue(t, domain.NewCaseID, "case-9"), Conclusion: "判定不成立", Operator: operatorAlice}, application.CaseNotFound},
		"另一个租户的同名": {application.CloseCaseCommand{TenantID: other, Case: id, Conclusion: "判定不成立", Operator: operatorAlice}, application.CaseNotFound},
	} {
		result, err := fixture.handler.Close(context.Background(), test.command)
		if err != nil || result.Outcome() != test.want {
			t.Fatalf("%s：outcome = %q err = %v, want %q", name, result.Outcome(), err, test.want)
		}
	}
	if fixture.store.byKey[caseKey(fixture.tenant, id)].Phase() != domain.CaseAwaitingResponse {
		t.Fatal("被拒的关闭动了案件")
	}

	direct, err := fixture.handler.Close(context.Background(), application.CloseCaseCommand{TenantID: fixture.tenant, Case: id, Conclusion: "判定不成立", Operator: operatorAlice})
	if err != nil || direct.Outcome() != application.CaseClosedWithConclusion {
		t.Fatalf("待响应直接关闭：outcome = %q err = %v", direct.Outcome(), err)
	}
	if current, _ := direct.Case(); !current.FirstResponseAt().IsZero() {
		t.Fatal("没接过单的案件关闭后凭空有了首次响应时间")
	}
}

// Covers: UC-VE-005「发现重复案件时指定主案……将重复案以已归并结论关闭」与 CONTEXT「原编号与历史不删不重置」——重复案件
// 并进一份仍在处置的主案件：它以已归并关闭并指向主案件，主案件不动；主案件不在册、已关闭、或就是它自己，都答主案件
// 不合格，重复案件不动。
func TestADuplicateCaseMergesOnlyIntoALiveMainCase(t *testing.T) {
	fixture := newCaseFixture(t)
	duplicate := fixture.seed(t, fixture.tenant, "case-dup")
	main := fixture.seed(t, fixture.tenant, "case-main")
	closedMain := fixture.seed(t, fixture.tenant, "case-closed")
	if _, err := fixture.handler.Close(context.Background(), application.CloseCaseCommand{TenantID: fixture.tenant, Case: closedMain, Conclusion: "已恢复", Operator: operatorAlice}); err != nil {
		t.Fatal(err)
	}

	for name, target := range map[string]domain.CaseID{
		"主案件不在册": mustValue(t, domain.NewCaseID, "case-9"),
		"主案件已关闭": closedMain,
		"并进自己":   duplicate,
	} {
		refused, err := fixture.handler.Merge(context.Background(), application.MergeCaseCommand{TenantID: fixture.tenant, Case: duplicate, Main: target, Operator: operatorAlice})
		if err != nil || refused.Outcome() != application.CaseMainNotEligible {
			t.Fatalf("%s：outcome = %q err = %v, want MAIN_CASE_NOT_ELIGIBLE", name, refused.Outcome(), err)
		}
	}
	if fixture.store.byKey[caseKey(fixture.tenant, duplicate)].Phase() != domain.CaseAwaitingResponse {
		t.Fatal("被拒的归并动了重复案件")
	}

	merged, err := fixture.handler.Merge(context.Background(), application.MergeCaseCommand{TenantID: fixture.tenant, Case: duplicate, Main: main, Operator: operatorAlice})
	if err != nil || merged.Outcome() != application.CaseMerged {
		t.Fatalf("merge: outcome = %q err = %v", merged.Outcome(), err)
	}
	snapshot := fixture.store.byKey[caseKey(fixture.tenant, duplicate)].Snapshot()
	if snapshot.Phase != domain.CaseClosed || snapshot.Conclusion != "MERGED" || snapshot.MergedInto != main {
		t.Fatalf("归并后的重复案件：%+v", snapshot)
	}
	if number, _ := snapshot.DocumentNumber.Number(); number != "EXC-case-dup" {
		t.Fatalf("归并改了重复案件的单据号：%q", number)
	}
	if fixture.store.byKey[caseKey(fixture.tenant, main)].Phase() != domain.CaseAwaitingResponse {
		t.Fatal("归并动了主案件")
	}

	// 先锁两份、再读：锁在任何一次读之前，否则读到的「主案件仍在处置」在落库时可能已经不成立。
	fixture.store.calls = nil
	third := fixture.seed(t, fixture.tenant, "case-third")
	if _, err := fixture.handler.Merge(context.Background(), application.MergeCaseCommand{TenantID: fixture.tenant, Case: third, Main: main, Operator: operatorAlice}); err != nil {
		t.Fatal(err)
	}
	if calls := fixture.store.calls; len(calls) < 3 || calls[0] != "lock case-third case-main" || calls[1] != "find case-third" || calls[2] != "find case-main" {
		t.Fatalf("调用次序 = %v——归并要先把两份案件一起锁住，再读", calls)
	}
}

// Covers: 两个人同时处置同一份案件——后落的一笔不盖掉先落的。两笔都读到待响应：别人先接了单，这一笔接单答已在处理中、
// 接单人仍是先落的那一位；别人先接了单，这一笔关闭照现在的案件重做一次并关闭成功；别人先关了案，这一笔接单答已关闭。
func TestALateDecisionDoesNotOverwriteTheEarlierOne(t *testing.T) {
	seedStale := func(t *testing.T, fixture *caseFixture, id domain.CaseID) {
		t.Helper()
		stale, err := domain.RehydrateCase(fixture.store.byKey[caseKey(fixture.tenant, id)].Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		fixture.store.staleOnce = stale
	}

	t.Run("接单撞上别人已接单", func(t *testing.T) {
		fixture := newCaseFixture(t)
		id := fixture.seed(t, fixture.tenant, "case-1")
		seedStale(t, fixture, id)
		// staleOnce 让下面这一笔「别人的接单」读到的是旧案件；它先落成处理中。
		if first, err := fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorAlice}); err != nil || first.Outcome() != application.CaseTakenUp {
			t.Fatalf("先落的接单：outcome = %q err = %v", first.Outcome(), err)
		}
		seedStale(t, fixture, id)
		fixture.store.staleOnce, _ = domain.EstablishCase(id, mustNumber(t, "EXC-case-1"),
			mustValue(t, domain.NewTrackedParcelReference, "parcel-1"), mustValue(t, domain.NewImpactScopeReference, "scope-1"),
			mustValue(t, domain.NewResponsibleTeamReference, "team-1"), caseEstablishedAt)
		late, err := fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorBob})
		if err != nil || late.Outcome() != application.CaseAlreadyInProgress {
			t.Fatalf("后落的接单：outcome = %q err = %v, want ALREADY_IN_PROGRESS", late.Outcome(), err)
		}
		if operators := fixture.store.operators[caseKey(fixture.tenant, id)]; len(operators) != 1 || operators[0] != operatorAlice {
			t.Fatalf("接单人 = %v，被后到的那一位盖掉了", operators)
		}
	})

	t.Run("关闭撞上别人刚接单", func(t *testing.T) {
		fixture := newCaseFixture(t)
		id := fixture.seed(t, fixture.tenant, "case-1")
		seedStale(t, fixture, id)
		awaiting := fixture.store.staleOnce
		fixture.store.staleOnce = nil
		if _, err := fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorAlice}); err != nil {
			t.Fatal(err)
		}
		fixture.store.staleOnce = awaiting
		closed, err := fixture.handler.Close(context.Background(), application.CloseCaseCommand{TenantID: fixture.tenant, Case: id, Conclusion: "已恢复", Operator: operatorBob})
		if err != nil || closed.Outcome() != application.CaseClosedWithConclusion {
			t.Fatalf("关闭：outcome = %q err = %v, want CLOSED——案件被接单后关闭的决定仍然成立", closed.Outcome(), err)
		}
		if current, _ := closed.Case(); current.FirstResponseAt().IsZero() {
			t.Fatal("重做的关闭把先落的首次响应时间抹掉了")
		}
	})

	t.Run("接单撞上别人已关闭", func(t *testing.T) {
		fixture := newCaseFixture(t)
		id := fixture.seed(t, fixture.tenant, "case-1")
		seedStale(t, fixture, id)
		awaiting := fixture.store.staleOnce
		fixture.store.staleOnce = nil
		if _, err := fixture.handler.Close(context.Background(), application.CloseCaseCommand{TenantID: fixture.tenant, Case: id, Conclusion: "判定不成立", Operator: operatorAlice}); err != nil {
			t.Fatal(err)
		}
		fixture.store.staleOnce = awaiting
		late, err := fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorBob})
		if err != nil || late.Outcome() != application.CaseAlreadyClosed {
			t.Fatalf("接单：outcome = %q err = %v, want ALREADY_CLOSED", late.Outcome(), err)
		}
	})
}

// Covers: 案件册读写不动是没形成答案，不是业务答复。
func TestAnUnreadableCaseRegisterFormsNoAnswer(t *testing.T) {
	fixture := newCaseFixture(t)
	id := fixture.seed(t, fixture.tenant, "case-1")
	fixture.store.err = errors.New("connection refused")
	result, err := fixture.handler.TakeUp(context.Background(), application.TakeUpCaseCommand{TenantID: fixture.tenant, Case: id, Operator: operatorAlice})
	if err != nil || result.Outcome() != application.CaseUndecided {
		t.Fatalf("outcome = %q err = %v, want UNDECIDED", result.Outcome(), err)
	}
}

func mustNumber(t *testing.T, value string) domain.DocumentNumber {
	t.Helper()
	number, err := domain.NewDocumentNumber(value)
	if err != nil {
		t.Fatal(err)
	}
	return number
}
