package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件对真实 PostgreSQL 16 证集运成员关系的反向只读口（ADR-0299）：按成员列出来源事实层里曾以它为成员移入的单元。单元行与事实行
// 直接播种，沿用成员关系按时点那一份用例的夹具。

func newHoldingFixture(t *testing.T) (*membershipFixture, *adapter.ConsolidationUnitsHoldingMember) {
	t.Helper()
	fixture := newMembershipFixture(t)
	db, err := bentopg.NewDB(fixture.pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	view, err := adapter.NewConsolidationUnitsHoldingMember(db)
	if err != nil {
		t.Fatalf("构造反向读口：%v", err)
	}
	return fixture, view
}

func holdersOf(t *testing.T, view *adapter.ConsolidationUnitsHoldingMember, tenant, member string) []string {
	t.Helper()
	units, err := view.ReadUnitsEverHoldingMember(context.Background(), ref(t, domain.NewTenantID, tenant), member)
	if err != nil {
		t.Fatalf("读 %s 曾被装进的单元：%v", member, err)
	}
	got := []string{}
	for _, unit := range units {
		got = append(got, unit.String())
	}
	return got
}

// Covers: `AT-NO-132`（ADR-0299 决定二）——按成员列出曾被直接装入的单元：移出了的、单元已关闭的、移入晚于任何问询时点的都在
// 候选里（候选一口放宽，「那一刻在不在」由按时点那一口收紧）；嵌套单元也是别的单元的成员，同样可以问；只有移出、没有移入的不算；
// 没被装过的成员答空、不是错；别的租户的同名成员看不见；结果按单元标识定序、同一单元多次移入只出现一次。
func TestUnitsThatEverHeldAMemberAreListedWhetherOrNotItIsStillInThem(t *testing.T) {
	fixture, view := newHoldingFixture(t)
	for _, unit := range []string{"bag-1", "bag-2", "bag-3", "cage-1"} {
		fixture.openUnit(unit)
	}
	before := membershipAt.Add(-time.Hour)
	fixture.fact("add-1", "ADD_MEMBER", "bag-1", "hu-a", before, before)
	fixture.fact("remove-1", "REMOVE_MEMBER", "bag-1", "hu-a", before.Add(10*time.Minute), before.Add(10*time.Minute))
	fixture.fact("add-1-again", "ADD_MEMBER", "bag-1", "hu-a", before.Add(20*time.Minute), before.Add(20*time.Minute))
	fixture.fact("add-2", "ADD_MEMBER", "bag-2", "hu-a", before, before)
	fixture.fact("close-2", "CLOSE_UNIT", "bag-2", "", before.Add(30*time.Minute), before.Add(30*time.Minute))
	fixture.fact("add-3-late", "ADD_MEMBER", "bag-3", "hu-a", membershipAt.Add(time.Hour), membershipAt.Add(time.Hour))
	fixture.fact("remove-only", "REMOVE_MEMBER", "cage-1", "hu-a", before, before)
	fixture.fact("nest", "ADD_MEMBER", "cage-1", "bag-1", before, before)

	if got := holdersOf(t, view, "tenant-a", "hu-a"); !sameMembers(got, "bag-1", "bag-2", "bag-3") {
		t.Fatalf("hu-a 曾被装进 %v，要 bag-1、bag-2、bag-3（移出、关闭、晚移入都算，只有移出的 cage-1 不算，bag-1 只出现一次）", got)
	}
	if got := holdersOf(t, view, "tenant-a", "bag-1"); !sameMembers(got, "cage-1") {
		t.Fatalf("嵌套单元 bag-1 曾被装进 %v，要 cage-1", got)
	}
	if got := holdersOf(t, view, "tenant-a", "hu-never"); len(got) != 0 {
		t.Fatalf("从没被装过的成员 = %v，要空", got)
	}
	if got := holdersOf(t, view, "tenant-b", "hu-a"); len(got) != 0 {
		t.Fatalf("别的租户问同一个成员 = %v，要空", got)
	}
}

// Covers: `AT-NO-132` 失败格（ADR-0299 决定二）——读不动作为错误交回、不答「没被装过」：调用方据空答案认定这件实物没随过任何袋，
// 把读不动答成它，这件实物就永远错过它随袋走过的移动。空白成员不问。
func TestAnUnreadableHolderListIsAnErrorNotAnEmptyAnswer(t *testing.T) {
	fixture, view := newHoldingFixture(t)
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	if _, err := view.ReadUnitsEverHoldingMember(context.Background(), tenant, " "); err == nil {
		t.Fatal("空白成员要报错")
	}
	if _, err := fixture.pool.Exec(context.Background(),
		`ALTER TABLE node_operations.consolidation_fact RENAME TO consolidation_fact_unreachable`); err != nil {
		t.Fatalf("改名：%v", err)
	}
	if got, err := view.ReadUnitsEverHoldingMember(context.Background(), tenant, "hu-a"); err == nil {
		t.Fatalf("事实层读不动 = %v，要报错", got)
	}
}

// Covers: `AT-NO-132` 索引面（迁移 0023）——按成员问走 consolidation_fact_by_member，不扫全租户的事实：关掉顺序扫描后查询计划仍
// 能走索引，且计划里用到的就是这一条。
func TestTheHolderListIsServedByTheMemberIndex(t *testing.T) {
	fixture, _ := newHoldingFixture(t)
	fixture.openUnit("bag-1")
	before := membershipAt.Add(-time.Hour)
	fixture.fact("add-1", "ADD_MEMBER", "bag-1", "hu-a", before, before)

	ctx := context.Background()
	tx, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开事务：%v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("关顺序扫描：%v", err)
	}
	rows, err := tx.Query(ctx,
		`EXPLAIN SELECT DISTINCT unit_id FROM node_operations.consolidation_fact
		  WHERE tenant_id = 'tenant-a' AND member_id = 'hu-a' AND action = 'ADD_MEMBER' ORDER BY unit_id`)
	if err != nil {
		t.Fatalf("EXPLAIN：%v", err)
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("读计划：%v", err)
		}
		plan += line + "\n"
	}
	if !strings.Contains(plan, "consolidation_fact_by_member") {
		t.Fatalf("按成员问没有走 consolidation_fact_by_member：\n%s", plan)
	}
}
