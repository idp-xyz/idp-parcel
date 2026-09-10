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

// Covers: 票 tf-segment-lifecycle-closure/12 完成判据 3 与票 13 完成判据「接线后停点后移」——执行器在生产装配下不再答
// DELIVERY_PLACE_SOURCE_NOT_WIRED 也不再答 DELIVERY_WINDOW_SOURCE_NOT_WIRED：地点缝接了 PS 真适配器（读真库里的委托仓储），
// 时间窗缝接了 NR 真适配器（读真库里判断本体的那一段），条件缝（票 14）未接，所以一拍停在
// DELIVERY_CONDITION_SOURCE_NOT_WIRED，续办引用非空、一个任务都不开。夹具：一个对象凭已交接进入声明为末端派送的段
// （真库、真编排，无计划段——缝没接是装配事实，执行器先核缝再问所有者，所以这里连时间窗都还没去问）。
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
	if result.UndecidedReason() == tfapp.DeliveryWindowSourceNotWired {
		t.Fatal("停点仍是 DELIVERY_WINDOW_SOURCE_NOT_WIRED——时间窗缝没接上真适配器")
	}
	if result.UndecidedReason() != tfapp.DeliveryConditionSourceNotWired {
		t.Fatalf("reason = %s, want DELIVERY_CONDITION_SOURCE_NOT_WIRED（票 14 未接是下一处诚实停点）", result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决没有续办引用——票 14 接上之后这一拍要能重跑")
	}
	if _, recorded := result.Record(); recorded {
		t.Fatal("缝未接却形成了任务")
	}
}
