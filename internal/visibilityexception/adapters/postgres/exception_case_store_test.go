package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// Covers: 案件册的读回与带条件的推进（迁移 visibility_exception 0035）——待响应的案件读回、接单后落成处理中并记下接单人；
// 基于同一份旧读数的第二笔推进一行不动、答案件已动；随后关闭落下结论与关闭人，接单人不被关闭那一笔盖掉；另一个租户
// 拿同名案件标识读不到也推不动。
func TestACaseIsReadBackAndAdvancedOnlyFromThePhaseItWasReadIn(t *testing.T) {
	fixture := newCaseFixture(t)
	fixture.insertCase(t, "tenant-a", "case-1", "AWAITING_RESPONSE")
	store, err := adapter.NewExceptionCaseStore(fixture.db)
	if err != nil {
		t.Fatalf("构造案件册：%v", err)
	}
	ctx := t.Context()
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")
	id := projectionValue(t, domain.NewCaseID, "case-1")
	advance := func(t *testing.T, tenant domain.TenantID, exceptionCase *domain.ExceptionCase, from domain.CasePhase, operator string) ports.CaseAdvanceOutcome {
		t.Helper()
		var outcome ports.CaseAdvanceOutcome
		if err := fixture.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = store.AdvanceCase(txCtx, ports.CaseAdvance{Tenant: tenant, Case: exceptionCase, From: from, Operator: operator})
			return err
		}); err != nil {
			t.Fatalf("推进案件：%v", err)
		}
		return outcome
	}

	first, found, err := store.FindCase(ctx, tenant, id)
	if err != nil || !found || first.Phase() != domain.CaseAwaitingResponse {
		t.Fatalf("读回待响应案件：phase=%v found=%v err=%v", first, found, err)
	}
	stale, _, err := store.FindCase(ctx, tenant, id)
	if err != nil {
		t.Fatal(err)
	}
	takenAt := caseBaseAt.Add(time.Hour)
	if err := first.TakeUp(takenAt); err != nil {
		t.Fatal(err)
	}
	if outcome := advance(t, tenant, first, domain.CaseAwaitingResponse, "https://issuer#alice"); outcome != ports.CaseAdvanced {
		t.Fatalf("接单 outcome = %d，want CaseAdvanced", outcome)
	}
	if err := stale.TakeUp(takenAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if outcome := advance(t, tenant, stale, domain.CaseAwaitingResponse, "https://issuer#bob"); outcome != ports.CasePhaseMoved {
		t.Fatalf("基于旧读数的接单 outcome = %d，want CasePhaseMoved", outcome)
	}

	current, _, err := store.FindCase(ctx, tenant, id)
	if err != nil || current.Phase() != domain.CaseInProgress || !current.FirstResponseAt().Equal(takenAt) {
		t.Fatalf("接单后读回：%+v err=%v——首次响应时间该是先落的那一笔", current, err)
	}
	if err := current.Close("已恢复", takenAt.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if outcome := advance(t, tenant, current, domain.CaseInProgress, "https://issuer#carol"); outcome != ports.CaseAdvanced {
		t.Fatalf("关闭 outcome = %d，want CaseAdvanced", outcome)
	}
	var takenBy, closedBy, conclusion string
	if err := fixture.pool.QueryRow(ctx,
		`SELECT first_response_by, closed_by, conclusion FROM visibility_exception.exception_case WHERE tenant_id = 'tenant-a' AND case_id = 'case-1'`,
	).Scan(&takenBy, &closedBy, &conclusion); err != nil {
		t.Fatalf("读决定人：%v", err)
	}
	if takenBy != "https://issuer#alice" || closedBy != "https://issuer#carol" || conclusion != "已恢复" {
		t.Fatalf("接单人 %q 关闭人 %q 结论 %q", takenBy, closedBy, conclusion)
	}

	other := projectionValue(t, domain.NewTenantID, "tenant-b")
	if _, found, err := store.FindCase(ctx, other, id); err != nil || found {
		t.Fatalf("另一个租户读到了案件：found=%v err=%v", found, err)
	}
	fixture.insertCase(t, "tenant-a", "case-2", "AWAITING_RESPONSE")
	second, _, err := store.FindCase(ctx, tenant, projectionValue(t, domain.NewCaseID, "case-2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := second.TakeUp(takenAt); err != nil {
		t.Fatal(err)
	}
	if outcome := advance(t, other, second, domain.CaseAwaitingResponse, "https://issuer#mallory"); outcome != ports.CasePhaseMoved {
		t.Fatalf("另一个租户推进 outcome = %d，want CasePhaseMoved——一行都不该动", outcome)
	}
}

// Covers: 案件册的写口在没有事务的上下文里被拒——事务边界归装配点，推进不会在事务之外悄悄落库。
func TestCaseAdvanceRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newCaseFixture(t)
	store, err := adapter.NewExceptionCaseStore(fixture.db)
	if err != nil {
		t.Fatalf("构造案件册：%v", err)
	}
	if _, err := store.AdvanceCase(t.Context(), ports.CaseAdvance{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务推进案件应返回 ErrTransactionRequired，实得：%v", err)
	}
	// 行锁只在事务里才留得住：事务之外锁一下就放，等于没锁。
	if err := store.LockCases(t.Context(), domain.TenantID{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务锁案件应返回 ErrTransactionRequired，实得：%v", err)
	}
}
