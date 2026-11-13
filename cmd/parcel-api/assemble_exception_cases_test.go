package main

import (
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
