package main

import (
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证 CONS-PROJ-TF-B 揽收路：对象级揽收登记经 FanOut 先投 VE 投影、再投 PS 采用。
// 映射目录未配置必须未归类入账；PS 仍停在资格未证明。不种 PAR-VIS-01。

const (
	derivePickupConsumerName = "visibility-exception/derive-projection-from-offsite-pickup"
	pickupFactRef            = "offsite-pickup/" + offsitePickupObject + "/" + offsitePickupAttempt
	pickupFactVersion        = "SYN-PICKUP-V1"
)

// Covers: 生产 wireDispatcher 一拍——VE 投影未归类入账，PS 资格未证明让整封 Publish
// 失败。FanOut 先 VE 后 PS。
func TestARegisteredOffsitePickupDerivesAnUnclassifiedProjectionAndStopsAdoption(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}

	seedSYNPCEligibility(t, fixture)
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

	assertUnclassifiedPickupProjection(t, fixture)
	if n := fixture.countInbox(t, derivePickupConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
	assertNoPickupAdoptionTrace(t, fixture, eventID)
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 1 {
		t.Fatalf("投影交接信封 = %d, want 1", n)
	}
}

func assertUnclassifiedPickupProjection(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, offsitePickupObject)
	factRef := mustVE(t, vedomain.NewSourceFactReference, pickupFactRef)
	version := mustVE(t, vedomain.NewSourceFactVersion, pickupFactVersion)

	facts, err := vepostgres.NewAcceptedFacts(fixture.db)
	if err != nil {
		t.Fatalf("构造已接受事实读口：%v", err)
	}
	record, found, err := facts.FindByKey(t.Context(), veports.FactKey{
		Tenant:  tenant,
		Source:  vedomain.SourceTransportFulfillment,
		Fact:    factRef,
		Version: version,
	})
	if err != nil {
		t.Fatalf("读已接受事实：%v", err)
	}
	if !found {
		t.Fatal("映射未配置却没有事实行——未归类也必须入账")
	}
	if record.Fact.Source() != vedomain.SourceTransportFulfillment ||
		record.Fact.Parcel().String() != offsitePickupObject ||
		record.Fact.Fact().String() != pickupFactRef {
		t.Fatalf("事实维 source=%s parcel=%s fact=%s",
			record.Fact.Source(), record.Fact.Parcel(), record.Fact.Fact())
	}

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
	if _, classified := entry.Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明里程碑")
	}
	if entry.MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q, want MAPPING_NOT_CONFIGURED", entry.MappingVersion())
	}
	if entry.Fact().Fact().String() != pickupFactRef {
		t.Fatalf("条目 fact = %s, want %s", entry.Fact().Fact(), pickupFactRef)
	}
}
