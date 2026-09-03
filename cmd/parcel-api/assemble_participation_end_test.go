package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: 票 tf-segment-lifecycle-closure/06 在装配点上——两处内部触发经真库同事务成立：对象凭第一次交接
// 进 SEG-1（NO_ACTIVE_PARTICIPATION）；下一次交接把它送进 SEG-2，前段 SEG-1 的参与随之结束
// （PARTICIPATION_ENDED）；有效交付落库后 SEG-2 的参与结束。段由登记册按对象找，命令从头到尾不带前段。
// 测试输入是隔离合成，只记 `S`。
func TestTheWiredTriggersEndParticipationsAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		t.Fatalf("装配交付编排：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatalf("段登记册读面：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1")
	activeIn := func(t *testing.T, segment, object string) bool {
		t.Helper()
		record, found, err := segments.FindByKey(t.Context(), tfports.FulfillmentSegmentKey{
			TenantID: tenant, Segment: mustValue(t, tfdomain.NewFulfillmentSegmentReference, segment),
		})
		if err != nil || !found {
			t.Fatalf("读回 %s：found=%v err=%v", segment, found, err)
		}
		participation, present := record.Segment.ParticipationFor(mustValue(t, tfdomain.NewCarriedObjectReference, object))
		return present && participation.Active()
	}

	// 参与起点要早于交付时刻（领域拒绝终点早于起点），而交付夹具的到场时刻是固定的 deliveryAttemptArrivedAt，
	// 所以两次交接都排在它之前。
	firstCommand := registerHandoverCommand(t, "SYN-PARCEL-1", "SYN-SEG-1", "")
	firstCommand.JudgedAt = deliveryAttemptArrivedAt.Add(-6 * time.Hour)
	first, err := controlFacts.handover.Register(t.Context(), firstCommand)
	if err != nil || first.Outcome() != tfapp.HandoverRegistered {
		t.Fatalf("首次交接：outcome=%v err=%v", first.Outcome(), err)
	}
	if first.ParticipationEnd() != tfapp.ParticipationNoActiveParticipation {
		t.Fatalf("首次交接 participationEnd = %v, want NO_ACTIVE_PARTICIPATION", first.ParticipationEnd())
	}

	next := registerHandoverCommand(t, "SYN-PARCEL-1", "SYN-SEG-2", "")
	next.Scope = "SYN-HANDOVER-SCOPE-2"
	next.Version = "SYN-HANDOVER-RESULT/PARCEL-1/v2"
	next.JudgedAt = deliveryAttemptArrivedAt.Add(-3 * time.Hour)
	moved, err := controlFacts.handover.Register(t.Context(), next)
	if err != nil || moved.Outcome() != tfapp.HandoverRegistered {
		t.Fatalf("下一次交接：outcome=%v err=%v", moved.Outcome(), err)
	}
	if moved.ParticipationEnd() != tfapp.ParticipationEndedNow {
		t.Fatalf("下一次交接 participationEnd = %v, want PARTICIPATION_ENDED", moved.ParticipationEnd())
	}
	if activeIn(t, "SYN-SEG-1", "SYN-PARCEL-1") {
		t.Fatal("下一次交接后 SYN-PARCEL-1 在 SYN-SEG-1 的参与仍在场")
	}
	if !activeIn(t, "SYN-SEG-2", "SYN-PARCEL-1") {
		t.Fatal("下一次交接后 SYN-PARCEL-1 没有在 SYN-SEG-2 在场")
	}

	seedDeliveryAttemptRow(t, pool, "SYN-TENANT-1", "SYN-ATTEMPT-END-1", []string{"SYN-PARCEL-1"})
	seedDeliveryResultRow(t, pool, "SYN-TENANT-1", "SYN-ATTEMPT-END-1", "SYN-PARCEL-1", "DELIVERED", nil)
	delivered, err := delivery.Register(t.Context(), registerDeliveryCommand(t, "SYN-ATTEMPT-END-1", "SYN-PARCEL-1"))
	if err != nil || delivered.Outcome() != tfapp.DeliveryRegistered {
		t.Fatalf("交付：outcome=%v err=%v", delivered.Outcome(), err)
	}
	if delivered.ParticipationEnd() != tfapp.ParticipationEndedNow {
		t.Fatalf("交付 participationEnd = %v, want PARTICIPATION_ENDED", delivered.ParticipationEnd())
	}
	if activeIn(t, "SYN-SEG-2", "SYN-PARCEL-1") {
		t.Fatal("交付后 SYN-PARCEL-1 在 SYN-SEG-2 的参与仍在场")
	}
}
