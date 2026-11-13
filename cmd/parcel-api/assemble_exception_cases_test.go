package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Covers: 异常案件三口的第二参是真编排（UC-VE-005）——案件册与事务壳在真实 PostgreSQL 上装得起来。册上两份待响应的案件：
// 接下主案件，首次响应时间与接单人落进那一行；再接一次答已在处理中、行不变；把重复案件并进主案件，它以已归并关闭并
// 指向主案件；以结论关闭主案件，关闭人落下而接单人不被盖掉；并进一份已关闭的案件答主案件不合格。测试输入是隔离合成，
// 只记 `S`。
func TestTheWiredExceptionCasesAreTakenUpMergedAndClosedAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	faces, err := buildExceptionCaseFaces(db)
	if err != nil {
		t.Fatalf("装配异常案件编排：%v", err)
	}
	const tenant = "SYN-TENANT-1"
	establishedAt := time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)
	for _, id := range []string{"SYN-CASE-MAIN", "SYN-CASE-DUP", "SYN-CASE-LATE"} {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.exception_case
				(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at)
			 VALUES ($1, $2, 'SYN-PARCEL-1', 'SYN-SCOPE/v1', 'SYN-TEAM-1', 'AWAITING_RESPONSE', $3)`,
			tenant, id, establishedAt); err != nil {
			t.Fatalf("写入案件 %s：%v", id, err)
		}
	}
	tenantID := mustValue(t, visibilitydomain.NewTenantID, tenant)
	main := mustValue(t, visibilitydomain.NewCaseID, "SYN-CASE-MAIN")
	duplicate := mustValue(t, visibilitydomain.NewCaseID, "SYN-CASE-DUP")
	late := mustValue(t, visibilitydomain.NewCaseID, "SYN-CASE-LATE")
	const alice, bob = "https://issuer#alice", "https://issuer#bob"

	taken, err := faces.TakeUp(t.Context(), visibilityapp.TakeUpCaseCommand{TenantID: tenantID, Case: main, Operator: alice})
	if err != nil || taken.Outcome() != visibilityapp.CaseTakenUp {
		t.Fatalf("接单：outcome = %v err = %v", taken.Outcome(), err)
	}
	again, err := faces.TakeUp(t.Context(), visibilityapp.TakeUpCaseCommand{TenantID: tenantID, Case: main, Operator: bob})
	if err != nil || again.Outcome() != visibilityapp.CaseAlreadyInProgress {
		t.Fatalf("再接单：outcome = %v err = %v, want ALREADY_IN_PROGRESS", again.Outcome(), err)
	}

	merged, err := faces.Merge(t.Context(), visibilityapp.MergeCaseCommand{TenantID: tenantID, Case: duplicate, Main: main, Operator: bob})
	if err != nil || merged.Outcome() != visibilityapp.CaseMerged {
		t.Fatalf("归并：outcome = %v err = %v", merged.Outcome(), err)
	}

	closed, err := faces.Close(t.Context(), visibilityapp.CloseCaseCommand{TenantID: tenantID, Case: main, Conclusion: "已恢复：改派后妥投", Operator: bob})
	if err != nil || closed.Outcome() != visibilityapp.CaseClosedWithConclusion {
		t.Fatalf("关闭：outcome = %v err = %v", closed.Outcome(), err)
	}

	refused, err := faces.Merge(t.Context(), visibilityapp.MergeCaseCommand{TenantID: tenantID, Case: late, Main: main, Operator: alice})
	if err != nil || refused.Outcome() != visibilityapp.CaseMainNotEligible {
		t.Fatalf("并进已关闭的案件：outcome = %v err = %v, want MAIN_CASE_NOT_ELIGIBLE", refused.Outcome(), err)
	}

	type row struct {
		phase, conclusion, mergedInto, takenBy, closedBy string
		firstResponse                                    *time.Time
	}
	read := func(id string) row {
		t.Helper()
		var got row
		var conclusion, mergedInto, takenBy, closedBy *string
		if err := pool.QueryRow(t.Context(),
			`SELECT phase, conclusion, merged_into, first_response_by, closed_by, first_response
			   FROM visibility_exception.exception_case WHERE tenant_id = $1 AND case_id = $2`, tenant, id,
		).Scan(&got.phase, &conclusion, &mergedInto, &takenBy, &closedBy, &got.firstResponse); err != nil {
			t.Fatalf("读案件 %s：%v", id, err)
		}
		for target, value := range map[*string]*string{&got.conclusion: conclusion, &got.mergedInto: mergedInto, &got.takenBy: takenBy, &got.closedBy: closedBy} {
			if value != nil {
				*target = *value
			}
		}
		return got
	}
	if got := read("SYN-CASE-MAIN"); got.phase != "CLOSED" || got.conclusion != "已恢复：改派后妥投" || got.takenBy != alice || got.closedBy != bob || got.firstResponse == nil {
		t.Fatalf("主案件在册：%+v——接单人该是先接的那一位，关闭人是关闭的那一位", got)
	}
	if got := read("SYN-CASE-DUP"); got.phase != "CLOSED" || got.conclusion != "MERGED" || got.mergedInto != "SYN-CASE-MAIN" || got.closedBy != bob || got.takenBy != "" {
		t.Fatalf("重复案件在册：%+v", got)
	}
	if got := read("SYN-CASE-LATE"); got.phase != "AWAITING_RESPONSE" {
		t.Fatalf("被拒的归并动了案件：%+v", got)
	}
}

// Covers: 对向的两笔归并不会把两份案件都关掉——「甲并进乙」与「乙并进甲」同时到达，真库上反复跑：每一轮恰有一笔成、
// 另一笔按先到那一笔落下之后的案件作答（自己已关闭，或主案件不合格），册上恰有一份仍在处置、另一份指向它。两份案件
// 在各自的事务里按固定顺序一起锁住，所以两笔排成先后，也不互相等死。
func TestOpposingMergesNeverCloseBothCases(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	faces, err := buildExceptionCaseFaces(db)
	if err != nil {
		t.Fatalf("装配异常案件编排：%v", err)
	}
	const tenant = "SYN-TENANT-1"
	tenantID := mustValue(t, visibilitydomain.NewTenantID, tenant)
	establishedAt := time.Date(2026, 11, 12, 6, 0, 0, 0, time.UTC)
	for round := 0; round < 20; round++ {
		first, second := fmt.Sprintf("SYN-CASE-A-%02d", round), fmt.Sprintf("SYN-CASE-B-%02d", round)
		for _, id := range []string{first, second} {
			if _, err := pool.Exec(t.Context(),
				`INSERT INTO visibility_exception.exception_case
					(tenant_id, case_id, root_parcel, impact_scope, responsible_team, phase, established_at)
				 VALUES ($1, $2, 'SYN-PARCEL-1', 'SYN-SCOPE/v1', 'SYN-TEAM-1', 'AWAITING_RESPONSE', $3)`,
				tenant, id, establishedAt); err != nil {
				t.Fatalf("写入案件 %s：%v", id, err)
			}
		}
		firstID, secondID := mustValue(t, visibilitydomain.NewCaseID, first), mustValue(t, visibilitydomain.NewCaseID, second)
		outcomes := make([]visibilityapp.ManageCaseOutcome, 2)
		errs := make([]error, 2)
		var ready, done sync.WaitGroup
		start := make(chan struct{})
		for index, command := range []visibilityapp.MergeCaseCommand{
			{TenantID: tenantID, Case: firstID, Main: secondID, Operator: "https://issuer#alice"},
			{TenantID: tenantID, Case: secondID, Main: firstID, Operator: "https://issuer#bob"},
		} {
			ready.Add(1)
			done.Add(1)
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				result, err := faces.Merge(context.Background(), command)
				outcomes[index], errs[index] = result.Outcome(), err
			}()
		}
		ready.Wait()
		close(start)
		done.Wait()

		merged := 0
		for index := range outcomes {
			if errs[index] != nil {
				t.Fatalf("第 %d 轮：归并报错：%v", round, errs[index])
			}
			switch outcomes[index] {
			case visibilityapp.CaseMerged:
				merged++
			case visibilityapp.CaseAlreadyClosed, visibilityapp.CaseMainNotEligible:
			default:
				t.Fatalf("第 %d 轮：outcome = %v——只该是已归并、已关闭或主案件不合格", round, outcomes[index])
			}
		}
		var open, pointing int
		if err := pool.QueryRow(t.Context(),
			`SELECT count(*) FILTER (WHERE phase <> 'CLOSED'), count(*) FILTER (WHERE merged_into IS NOT NULL)
			   FROM visibility_exception.exception_case WHERE tenant_id = $1 AND case_id IN ($2, $3)`,
			tenant, first, second,
		).Scan(&open, &pointing); err != nil {
			t.Fatalf("读案件：%v", err)
		}
		if merged != 1 || open != 1 || pointing != 1 {
			t.Fatalf("第 %d 轮：成了 %d 笔归并，仍在处置 %d 份，指向别的案件 %d 份——该是 1、1、1", round, merged, open, pointing)
		}
	}
}
