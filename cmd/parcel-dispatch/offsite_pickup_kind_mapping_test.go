package main

import (
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 MAP-KIND-SYN-TF 揽收路：测试内登记一份 offsite-pickup 映射后，对象级揽收
// 投影按类型命中 SYN 里程碑。隔离 S，不进 assemble.go，不种 PAR-VIS-01。

const (
	synPickupMappingVersion = "SYN-MAP-PICKUP/v1"
	synPickupMilestone      = "SYN-MILESTONE-OFFSITE-PICKUP"
)

// Covers: TRANSPORT_FULFILLMENT + offsite-pickup 一行覆盖此后同类型事实；PS 资格墙
// 仍让整封 Publish 失败。未插入映射行的既有 SYN 用例继续走未归类。
func TestAMappedOffsitePickupKindClassifiesTheProjectionWhileAdoptionStaysUnproven(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}

	seedSYNPCEligibility(t, fixture)
	seedSYNOffsitePickupKindMapping(t, fixture)
	eventID := recordRegisteredOffsitePickup(t, fixture)
	assertPickupAdoptionPreconditions(t, fixture)
	assertIntakeEligibilityUnproven(t, fixture, psdomain.OffsitePickupSource)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("PS 未决却定稿了 %d 条；揽收信封失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided", got)
	}

	assertClassifiedPickupProjection(t, fixture)
	if n := fixture.countInbox(t, derivePickupConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
	assertNoPickupAdoptionTrace(t, fixture, eventID)
}

func seedSYNOffsitePickupKindMapping(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	from := time.Now().UTC().Add(-24 * time.Hour)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.milestone_mapping_version
			(tenant_id, mapping_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, NULL, 'SYN-MAP-KIND-TF')`,
		tenant, synPickupMappingVersion, from); err != nil {
		t.Fatalf("登记 SYN 揽收映射版本：%v", err)
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.milestone_mapping_entry
			(tenant_id, mapping_version, source_context, source_fact_kind, milestone_ref)
		 VALUES ($1, $2, 'TRANSPORT_FULFILLMENT', 'offsite-pickup', $3)`,
		tenant, synPickupMappingVersion, synPickupMilestone); err != nil {
		t.Fatalf("登记 SYN 揽收映射条目：%v", err)
	}
}

func assertClassifiedPickupProjection(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, offsitePickupObject)

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
	if !classified || milestone.String() != synPickupMilestone {
		t.Fatalf("milestone classified=%v value=%q, want %s", classified, milestone, synPickupMilestone)
	}
	if entry.MappingVersion().String() != synPickupMappingVersion {
		t.Fatalf("mapping = %q, want %s", entry.MappingVersion(), synPickupMappingVersion)
	}
	if entry.Fact().Kind().String() != "offsite-pickup" {
		t.Fatalf("kind = %q, want offsite-pickup", entry.Fact().Kind())
	}
}
