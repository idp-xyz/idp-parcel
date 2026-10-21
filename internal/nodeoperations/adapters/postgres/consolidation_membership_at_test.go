package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证集运成员关系按时点的只读口（ADR-0225）。单元行与事实行直接播种：这一口只读来源事实层，
// 写侧怎样一步步推进单元不在它的问法里。

var membershipAt = time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC)

type membershipFixture struct {
	t    *testing.T
	pool *pgxpool.Pool
	view *adapter.ConsolidationMembershipsAt
}

func newMembershipFixture(t *testing.T) *membershipFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	view, err := adapter.NewConsolidationMembershipsAt(db)
	if err != nil {
		t.Fatalf("构造成员关系读口：%v", err)
	}
	return &membershipFixture{t: t, pool: pool, view: view}
}

func (fixture *membershipFixture) openUnit(unit string) {
	fixture.t.Helper()
	if _, err := fixture.pool.Exec(context.Background(),
		`INSERT INTO node_operations.consolidation_unit (tenant_id, unit_id, asset_ref, phase, members, snapshots, opened_source)
		 VALUES ('tenant-a', $1, 'asset-' || $1, 'OPEN', '[]'::jsonb, '[]'::jsonb,
		         jsonb_build_object('sourceId', 'open-' || $1, 'performedBy', 'packer', 'evidence', 'scan', 'occurredAt', $2::timestamptz))`,
		unit, membershipAt.Add(-6*time.Hour)); err != nil {
		fixture.t.Fatalf("播种单元 %s：%v", unit, err)
	}
}

// fact 落一条集运作业事实；member 为空即不带成员（开启、关闭那几格）。
func (fixture *membershipFixture) fact(source, action, unit, member string, occurredAt, recordedAt time.Time) {
	fixture.t.Helper()
	var memberColumn *string
	if member != "" {
		memberColumn = &member
	}
	if _, err := fixture.pool.Exec(context.Background(),
		`INSERT INTO node_operations.consolidation_fact
			(tenant_id, source_id, content_digest, action, unit_id, member_id, performed_by, evidence, occurred_at, recorded_at)
		 VALUES ('tenant-a', $1, 'digest-' || $1, $2, $3, $4, 'packer', 'scan', $5, $6)`,
		source, action, unit, memberColumn, occurredAt, recordedAt); err != nil {
		fixture.t.Fatalf("播种事实 %s：%v", source, err)
	}
}

func (fixture *membershipFixture) read(object string, at time.Time) ports.ConsolidationMembershipAt {
	fixture.t.Helper()
	membership, err := fixture.view.ReadConsolidationMembershipAt(context.Background(),
		ref(fixture.t, domain.NewTenantID, "tenant-a"), object, at)
	if err != nil {
		fixture.t.Fatalf("读 %s 在 %s 的成员：%v", object, at, err)
	}
	return membership
}

func membersOf(membership ports.ConsolidationMembershipAt) []string {
	var members []string
	for _, member := range membership.Members {
		label := member.Member.String()
		if member.NestedUnit {
			label += "(nested)"
		}
		members = append(members, label)
	}
	return members
}

func sameMembers(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// Covers: `AT-NO-129`（ADR-0225 决定二）——成员按移入、移出事实的业务发生时间取：时点之前移入的在、之后才移入的不在、时点之前
// 移出的不在；同一刻的移入与移出按记录时刻定先后；嵌套成员带「它本身是单元」；关闭之后的时点不装任何东西；不是单元的对象答
// 「不是单元」，不答空单元。
func TestMembersAreReadAtTheGivenMomentFromTheSourceFacts(t *testing.T) {
	fixture := newMembershipFixture(t)
	fixture.openUnit("bag-1")
	fixture.openUnit("cage-1")

	before := membershipAt.Add(-time.Hour)
	fixture.fact("add-a", "ADD_MEMBER", "bag-1", "hu-a", before, before)
	fixture.fact("add-b", "ADD_MEMBER", "bag-1", "hu-b", before, before)
	fixture.fact("remove-b", "REMOVE_MEMBER", "bag-1", "hu-b", before.Add(30*time.Minute), before.Add(30*time.Minute))
	fixture.fact("add-late", "ADD_MEMBER", "bag-1", "hu-late", membershipAt.Add(time.Minute), membershipAt.Add(time.Minute))
	// 同一刻先移出后移入：记录时刻晚的那一条说的是那一刻之后的样子。
	fixture.fact("remove-c", "REMOVE_MEMBER", "bag-1", "hu-c", before, before.Add(time.Second))
	fixture.fact("add-c-first", "ADD_MEMBER", "bag-1", "hu-c", before.Add(-time.Minute), before.Add(-time.Minute))
	fixture.fact("add-c-again", "ADD_MEMBER", "bag-1", "hu-c", before, before.Add(2*time.Second))
	fixture.fact("nest", "ADD_MEMBER", "cage-1", "bag-1", before, before)
	fixture.fact("cage-parcel", "ADD_MEMBER", "cage-1", "hu-d", before, before)

	if got := fixture.read("bag-1", membershipAt); !got.Unit || !sameMembers(membersOf(got), "hu-a", "hu-c") {
		t.Fatalf("bag-1 出发那一刻 = %+v，要 hu-a、hu-c", got)
	}
	if got := fixture.read("bag-1", before.Add(-2*time.Minute)); !got.Unit || len(got.Members) != 0 {
		t.Fatalf("bag-1 在任何移入之前 = %+v，要在册的空单元", got)
	}
	if got := fixture.read("cage-1", membershipAt); !sameMembers(membersOf(got), "bag-1(nested)", "hu-d") {
		t.Fatalf("cage-1 = %v，嵌套成员要带「它本身是单元」", membersOf(got))
	}

	fixture.fact("close-cage", "CLOSE_UNIT", "cage-1", "", membershipAt.Add(-time.Minute), membershipAt.Add(-time.Minute))
	if got := fixture.read("cage-1", membershipAt); !got.Unit || len(got.Members) != 0 {
		t.Fatalf("关闭之后的 cage-1 = %+v，要在册而不装任何东西", got)
	}
	if got := fixture.read("cage-1", membershipAt.Add(-2*time.Minute)); len(got.Members) != 2 {
		t.Fatalf("关闭之前的 cage-1 = %v，要照成员事实答", membersOf(got))
	}

	if got := fixture.read("PCL-NOT-A-UNIT", membershipAt); got.Unit || len(got.Members) != 0 {
		t.Fatalf("不是单元的对象 = %+v", got)
	}
	other, err := fixture.view.ReadConsolidationMembershipAt(context.Background(),
		ref(t, domain.NewTenantID, "tenant-b"), "bag-1", membershipAt)
	if err != nil || other.Unit {
		t.Fatalf("别的租户问同一个号 = %+v / %v，要「不是单元」", other, err)
	}
}

// Covers: `AT-NO-130`（ADR-0225 决定二）——读不动作为错误交回，不答「不是单元」：调用方据「不是单元」去按包裹问别处，把读不动答成
// 它，就是一袋被当成一件包裹放过去。空白对象不问。
func TestAnUnreadableMembershipIsAnErrorNotANonUnit(t *testing.T) {
	fixture := newMembershipFixture(t)
	fixture.openUnit("bag-1")
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	if _, err := fixture.view.ReadConsolidationMembershipAt(context.Background(), tenant, "  ", membershipAt); err == nil {
		t.Fatal("空白对象要报错")
	}
	if _, err := fixture.pool.Exec(context.Background(),
		`ALTER TABLE node_operations.consolidation_fact RENAME TO consolidation_fact_unreachable`); err != nil {
		t.Fatalf("改名：%v", err)
	}
	if got, err := fixture.view.ReadConsolidationMembershipAt(context.Background(), tenant, "bag-1", membershipAt); err == nil {
		t.Fatalf("事实层读不动 = %+v，要报错", got)
	}
}
