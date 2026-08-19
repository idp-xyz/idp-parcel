package main

import (
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 MAP-KIND：测试内登记一份 node-intake 映射后，节点收寄投影按类型命中
// SYN 里程碑。隔离 S，不进 assemble.go，不种 PAR-VIS-01。

const (
	synNodeIntakeMappingVersion = "SYN-MAP/v1"
	synNodeIntakeMilestone      = "SYN-MILESTONE-NODE-INTAKE"
)

// Covers: 目录按事实类型建键后，一行覆盖此后同类型事实；PS 资格墙仍让整封 Publish
// 失败。未插入映射行的既有 SYN 用例继续走未归类。
func TestAMappedNodeIntakeKindClassifiesTheProjectionWhileAdoptionStaysUnproven(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}

	seedSYNPCEligibility(t, fixture)
	seedSYNNodeIntakeKindMapping(t, fixture)
	eventID := recordFormedNodeIntake(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("PS 未决却定稿了 %d 条；收寄信封失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided", got)
	}

	assertClassifiedNodeIntakeProjection(t, fixture)
	if n := fixture.countInbox(t, deriveProjectionConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
	assertNoAdoptionTrace(t, fixture, eventID)
}

func seedSYNNodeIntakeKindMapping(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	from := time.Now().UTC().Add(-24 * time.Hour)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.milestone_mapping_version
			(tenant_id, mapping_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, NULL, 'SYN-MAP-KIND')`,
		tenant, synNodeIntakeMappingVersion, from); err != nil {
		t.Fatalf("登记 SYN 映射版本：%v", err)
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.milestone_mapping_entry
			(tenant_id, mapping_version, source_context, source_fact_kind, milestone_ref)
		 VALUES ($1, $2, 'NODE_OPERATIONS', 'node-intake', $3)`,
		tenant, synNodeIntakeMappingVersion, synNodeIntakeMilestone); err != nil {
		t.Fatalf("登记 SYN 映射条目：%v", err)
	}
}

func assertClassifiedNodeIntakeProjection(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, nodeIntakeAssociation)

	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatal("没有当前投影")
	}
	if len(projection.Entries()) != 1 {
		t.Fatalf("entries = %d, want 1", len(projection.Entries()))
	}
	entry := projection.Entries()[0]
	milestone, classified := entry.Milestone()
	if !classified || milestone.String() != synNodeIntakeMilestone {
		t.Fatalf("milestone classified=%v value=%q, want %s", classified, milestone, synNodeIntakeMilestone)
	}
	if entry.MappingVersion().String() != synNodeIntakeMappingVersion {
		t.Fatalf("mapping = %q, want %s", entry.MappingVersion(), synNodeIntakeMappingVersion)
	}
	if entry.Fact().Kind().String() != "node-intake" {
		t.Fatalf("kind = %q, want node-intake", entry.Fact().Kind())
	}
}
