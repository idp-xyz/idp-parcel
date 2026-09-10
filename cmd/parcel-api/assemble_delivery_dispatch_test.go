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

// Covers: 票 tf-segment-lifecycle-closure/12 完成判据 3、票 13「接线后停点后移」与票 14 完成判据 3——三条派送要求缝在生产
// 装配下**全部接真**，执行器不再有任何 *_SOURCE_NOT_WIRED 停点：一拍走到向三个所有者取七件，而空库里三个所有者各答
// 「没有」（PS：对象不属任何已接受委托——收件地点与商业解析回指两条缝都从这里答没有；NR：对象没有计划履约段），执行器
// 如实答 REQUIREMENT_MISSING 并按地点、时间窗、条件的固定顺序点名三件，任务待形成、不留续办引用（所有者答「没有」是
// 业务答案不是欠账，ADR-0114 决定三）。这里没有一格是替身或默认值：三件都缺是因为登记册空，不是因为缝没接——若哪条缝
// 又变回 nil，停点会退回该缝的 NOT_WIRED，本用例立刻红。夹具：一个对象凭已交接进入声明为末端派送的段（真库、真编排）。
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredDeliveryDispatchTriggerReachesTheOwnersAndReportsWhatTheyLack(t *testing.T) {
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
	if result.Outcome() == tfapp.DeliveryDispatchUndecided {
		t.Fatalf("一拍仍未决：reason=%s——某条缝没接上真适配器或所有者读不回（三缝齐后不该再有 NOT_WIRED 停点）", result.UndecidedReason())
	}
	if result.Outcome() != tfapp.DeliveryDispatchRequirementMissing {
		t.Fatalf("outcome = %q, want REQUIREMENT_MISSING（refusal=%s）——空库里三个所有者各答没有", result.Outcome(), result.Refusal())
	}
	missing := result.Missing()
	if len(missing) != 3 ||
		missing[0] != tfapp.DeliveryPlaceRequirement ||
		missing[1] != tfapp.DeliveryWindowRequirement ||
		missing[2] != tfapp.DeliveryConditionRequirement {
		t.Fatalf("missing = %v, want [DELIVERY_PLACE DELIVERY_WINDOW DELIVERY_CONDITION]——三个所有者各答没有，按固定顺序逐件点名", missing)
	}
	if result.ContinuationReference() != "" {
		t.Fatal("所有者答「没有」是业务答案不是欠账，不该留续办引用")
	}
	if _, recorded := result.Record(); recorded {
		t.Fatal("三件都缺却形成了任务——填了默认值")
	}
}
