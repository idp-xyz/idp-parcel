package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pgadapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrars 装配的整条登记链（隔离合成 S）：
// 三条命令各自贯通「翻译 → 用例 → 真库 → 留痕」，暂停重放留第二痕、恢复挂上真实
// 暂停、权威区间落进桥要读的那张册。Outbox 交接在链上——enqueue 失败会翻整笔事务，
// 退出码 0 本身就证明意图入了队。
func TestGovernanceRegisterVerticalOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	regs, err := buildRegistrars(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()
	identity := channelIdentity{osUser: "OPSHOST\\operator-a", hostname: "ops-host-01"}

	countTraces := func(command string) int {
		t.Helper()
		querier, err := db.ReadExecutor(ctx)
		if err != nil {
			t.Fatalf("取读执行器：%v", err)
		}
		var count int
		if err := querier.QueryRow(ctx,
			`SELECT count(*) FROM pilot_governance.channel_execution WHERE command = $1`,
			command,
		).Scan(&count); err != nil {
			t.Fatalf("数留痕行：%v", err)
		}
		return count
	}

	// 暂停：落册 + 重放，两次执行两条痕、册上一条。
	if message, code := execute(ctx, commandSuspend, suspendInput(), identity, regs); code != exitRegistered {
		t.Fatalf("暂停退出码 = %d（%s）", code, message)
	}
	if message, code := execute(ctx, commandSuspend, suspendInput(), identity, regs); code != exitRegistered ||
		!strings.Contains(message, "SUSPENSION_EXISTING") {
		t.Fatalf("暂停重放 = %d（%s）", code, message)
	}
	suspensions, err := pgadapter.NewSuspensions(db)
	if err != nil {
		t.Fatalf("构造暂停库：%v", err)
	}
	suspensionID, err := domain.NewSuspensionID("suspension-9")
	if err != nil {
		t.Fatalf("构造暂停标识：%v", err)
	}
	if _, found, err := suspensions.FindByID(ctx, suspensionID); err != nil || !found {
		t.Fatalf("暂停不在册（found=%v err=%v）", found, err)
	}
	if count := countTraces(commandSuspend); count != 2 {
		t.Fatalf("暂停留痕 = %d，要 2", count)
	}

	// 恢复：挂上刚落册的暂停，四件齐备走通。
	resume := []byte(`{
		"suspensionId": "suspension-9",
		"releaseEvidence": "evidence-pack/release-9",
		"consistencyCheck": "consistency-report/r9",
		"inventory": {
			"takenAt": "2026-08-24T02:00:00Z",
			"entries": [{
				"objectIdentity": "parcel-object/p1",
				"currentFacts": "accepted-fact/p1",
				"currentAuthority": "parcel-product",
				"responsibleParty": "ops-owner-1",
				"nextAction": "resume-normal-processing",
				"reviewBy": "2026-08-25T02:00:00Z"
			}]
		},
		"decidedBy": "pilot-business-owner",
		"decidedAt": "2026-08-24T03:00:00Z",
		"effectiveAt": "2026-08-24T03:05:00Z"
	}`)
	if message, code := execute(ctx, commandResume, resume, identity, regs); code != exitRegistered ||
		!strings.Contains(message, "RESUMPTION_RECORDED") {
		t.Fatalf("恢复 = %d（%s）", code, message)
	}
	if count := countTraces(commandResume); count != 1 {
		t.Fatalf("恢复留痕 = %d，要 1", count)
	}

	// 权威区间：落进生产归属桥要读的那张册（票 13 恢复动作的落点）。
	if message, code := execute(ctx, commandAuthorityInterval,
		intervalInput("parcel-product", "2026-08-24T00:00:00Z"), identity, regs); code != exitRegistered {
		t.Fatalf("权威区间退出码 = %d（%s）", code, message)
	}
	intervals, err := pgadapter.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatalf("构造权威区间册：%v", err)
	}
	current, err := intervals.ListCurrent(ctx)
	if err != nil {
		t.Fatalf("读权威区间册：%v", err)
	}
	if len(current) != 1 || current[0].ObjectScope != "pilot-members/v1" ||
		current[0].Authority != "parcel-product" {
		t.Fatalf("权威区间没落进册：%+v", current)
	}
	if count := countTraces(commandAuthorityInterval); count != 1 {
		t.Fatalf("区间留痕 = %d，要 1", count)
	}
}
