package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newExecutionFactView(t *testing.T) (*adapter.ExecutionFactView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewExecutionFactView(fixture.db)
	if err != nil {
		t.Fatalf("构造执行事实读口：%v", err)
	}
	return view, fixture
}

func loadFacts(t *testing.T, view *adapter.ExecutionFactView, tenant, decision string) []domain.ExecutionFact {
	t.Helper()
	facts, err := view.LoadExecutionFacts(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewRegulatoryDecisionID, decision))
	if err != nil {
		t.Fatalf("读执行事实：%v", err)
	}
	return facts
}

// 没有事实是如实答案，核对据此折出`证据不足`——决定推导不出执行。
func TestNoExecutionFactFoldsToInsufficientEvidence(t *testing.T) {
	view, _ := newExecutionFactView(t)
	facts := loadFacts(t, view, "tenant-a", "decision-1")
	if len(facts) != 0 {
		t.Fatalf("空库读出了 %d 条事实", len(facts))
	}

	verification, err := domain.VerifyDispositionExecution(verificationDecision(t), facts, viewBaseAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("空事实集进不了核对：%v", err)
	}
	if verification.Conclusion() != domain.ExecutionEvidenceInsufficient {
		t.Fatalf("空事实集没折成证据不足：%s", verification.Conclusion())
	}
}

// 部分执行与再次执行各是一份，读口不合并——合并会把差异抹平成一个刚好覆盖的总数。
func TestExecutionFactsAreReadBackPerFactAndDriveTheVerification(t *testing.T) {
	view, fixture := newExecutionFactView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.execution_fact
			(tenant_id, decision_id, fact_ref, executor_ref, scope_ref, units, occurred_at)
		 VALUES
			('tenant-a', 'decision-1', 'DESTRUCTION-EXEC/1', 'node-origin', 'parcel-1', 1, $1),
			('tenant-a', 'decision-1', 'DESTRUCTION-EXEC/2', 'node-origin', 'parcel-1', 1, $2)`,
		viewBaseAt.Add(time.Hour), viewBaseAt.Add(2*time.Hour))

	facts := loadFacts(t, view, "tenant-a", "decision-1")
	if len(facts) != 2 {
		t.Fatalf("两条事实读回 %d 条", len(facts))
	}
	// ORDER BY occurred_at：先发生的在前。
	if facts[0].Fact().String() != "DESTRUCTION-EXEC/1" || facts[1].Fact().String() != "DESTRUCTION-EXEC/2" {
		t.Fatalf("次序或内容走样：%s %s", facts[0].Fact(), facts[1].Fact())
	}

	verification, err := domain.VerifyDispositionExecution(verificationDecision(t), facts, viewBaseAt.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("核对：%v", err)
	}
	// 决定要求 2 件，两条各 1 件——正好覆盖。
	if verification.Conclusion() != domain.ExecutionCovered || verification.ExecutedUnits() != 2 {
		t.Fatalf("核对走样：conclusion=%s units=%d", verification.Conclusion(), verification.ExecutedUnits())
	}
}

// 只到一件时是部分覆盖，剩余范围继续保留——这条证明读口没有替核对补齐数量。
func TestPartialExecutionStaysPartial(t *testing.T) {
	view, fixture := newExecutionFactView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.execution_fact
			(tenant_id, decision_id, fact_ref, executor_ref, scope_ref, units, occurred_at)
		 VALUES ('tenant-a', 'decision-1', 'DESTRUCTION-EXEC/1', 'node-origin', 'parcel-1', 1, $1)`,
		viewBaseAt.Add(time.Hour))

	verification, err := domain.VerifyDispositionExecution(
		verificationDecision(t), loadFacts(t, view, "tenant-a", "decision-1"), viewBaseAt.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("核对：%v", err)
	}
	if verification.Conclusion() != domain.ExecutionPartiallyCovered {
		t.Fatalf("一件对两件没折成部分覆盖：%s", verification.Conclusion())
	}
}

func TestExecutionFactsDoNotLeakAcrossDecisionsOrTenants(t *testing.T) {
	view, fixture := newExecutionFactView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.execution_fact
			(tenant_id, decision_id, fact_ref, executor_ref, scope_ref, units, occurred_at)
		 VALUES
			('tenant-a', 'decision-1', 'F/1', 'node-origin', 'parcel-1', 1, $1),
			('tenant-b', 'decision-1', 'F/2', 'node-origin', 'parcel-1', 1, $1)`,
		viewBaseAt.Add(time.Hour))

	if facts := loadFacts(t, view, "tenant-a", "decision-2"); len(facts) != 0 {
		t.Fatalf("另一份决定的事实串了过来：%d 条", len(facts))
	}
	if facts := loadFacts(t, view, "tenant-a", "decision-1"); len(facts) != 1 {
		t.Fatalf("跨租户可见：%d 条", len(facts))
	}
}

func TestExecutionFactCheckRejectsAZeroUnitFact(t *testing.T) {
	_, fixture := newExecutionFactView(t)
	fixture.rejects(t, "零数量的执行事实",
		`INSERT INTO customs_compliance.execution_fact
			(tenant_id, decision_id, fact_ref, executor_ref, scope_ref, units, occurred_at)
		 VALUES ('t', 'd', 'f', 'e', 's', 0, now())`)
}
