package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 tf-segment-lifecycle-closure/12 完成判据 3——执行器在生产装配下不再答 DELIVERY_PLACE_SOURCE_NOT_WIRED：
// 地点缝接了 PS 真适配器（读真库里的委托仓储），时间窗缝（票 13）未接，所以一拍停在 DELIVERY_WINDOW_SOURCE_NOT_WIRED，
// 续办引用非空、一个任务都不开。夹具：一个对象凭已交接进入声明为末端派送的段（真库、真编排）。
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredDeliveryDispatchTriggerStopsAtTheNextUnwiredSeam(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	trigger, err := buildDeliveryDispatchTrigger(db)
	if err != nil {
		t.Fatalf("装配触发执行器：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}

	entering := registerHandoverCommand(t, "SYN-PARCEL-1", "SYN-DELIVERY-SEGMENT-1", "")
	entering.SegmentServiceAction = tfdomain.SegmentServesFinalDelivery.String()
	entered, err := controlFacts.handover.Register(t.Context(), entering)
	if err != nil || entered.Outcome() != tfapp.HandoverRegistered || entered.SegmentContinuationReference() != "" {
		t.Fatalf("凭已交接进派送段：outcome=%v err=%v debt=%q", entered.Outcome(), err, entered.SegmentContinuationReference())
	}

	result, err := trigger.Trigger(t.Context(), tfapp.TriggerDeliveryDispatchCommand{
		TenantID:   mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Segment:    "SYN-DELIVERY-SEGMENT-1",
		Object:     "SYN-PARCEL-1",
		OccurredAt: controlFactJudgedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("触发一拍：%v", err)
	}
	if result.Outcome() != tfapp.DeliveryDispatchUndecided {
		t.Fatalf("outcome = %q, want DISPATCH_UNDECIDED（refusal=%s missing=%v）", result.Outcome(), result.Refusal(), result.Missing())
	}
	if result.UndecidedReason() == tfapp.DeliveryPlaceSourceNotWired {
		t.Fatal("停点仍是 DELIVERY_PLACE_SOURCE_NOT_WIRED——地点缝没接上真适配器")
	}
	if result.UndecidedReason() != tfapp.DeliveryWindowSourceNotWired {
		t.Fatalf("reason = %s, want DELIVERY_WINDOW_SOURCE_NOT_WIRED（票 13 未接是下一处诚实停点）", result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用——票 13 接上之后这一拍要能重跑")
	}
	if _, recorded := result.Record(); recorded {
		t.Fatal("缝未接却形成了任务")
	}
}
