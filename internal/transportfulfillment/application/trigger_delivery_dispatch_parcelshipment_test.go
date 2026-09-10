package application_test

import (
	"context"
	"strings"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是票 tf-segment-lifecycle-closure/12 完成判据 2 的两例：执行器的地点一格接的是**真适配器**（adapters/parcelshipment
// .DeliveryPlaceSource），PS 那一头用读口替身按包裹答四格里的两格。它与 trigger_delivery_dispatch_test.go 里的端口替身
// 不同——那里替身直接扮端口，这里替身扮 PS 读口、翻译由真适配器做，证的是「接上真适配器之后执行器停在哪、任务 Place
// 里是哪个串」。

// parcelShipmentLookupStub 扮 PS 的 DeliveryPlaceReferenceView：按包裹身份答预设的一格。
type parcelShipmentLookupStub struct {
	byParcel map[string]psdomain.DeliveryPlaceResolution
	calls    int
}

func (stub *parcelShipmentLookupStub) LoadDeliveryPlaceReference(
	_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.DeliveryPlaceResolution, error) {
	stub.calls++
	if resolution, known := stub.byParcel[parcel.String()]; known {
		return resolution, nil
	}
	// PS 对不属任何已接受委托成员集合的对象按统一不可见结果答「没有」——集运单元、他租户、从未声明的都在这一格。
	return psdomain.NoDeliveryPlaceResolution(), nil
}

// baselineAnchoredReference 造 PS 会为 request-1 的成员答出的基线锚引用（tenant-1 / request-1 / DELIVERY_PLACE / baseline）。
func baselineAnchoredReference(t *testing.T) psdomain.DeliveryPlaceReference {
	t.Helper()
	requestID, err := psdomain.NewShipmentRequestID("request-1")
	if err != nil {
		t.Fatalf("request id: %v", err)
	}
	scope, err := psdomain.NewShipmentScopedSourceData(requestID, psdomain.DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	tenant, err := psdomain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	reference, err := psdomain.NewDeliveryPlaceReference(tenant, scope, psdomain.NewAcceptanceBaselineAnchor())
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	return reference
}

// wireRealPlaces 把执行器的地点一格换成真适配器 + PS 读口替身，其余两格照旧用端口替身。
func wireRealPlaces(t *testing.T, fixture *dispatchTriggerFixture, lookup *parcelShipmentLookupStub) {
	t.Helper()
	source, err := tfparcelshipment.NewDeliveryPlaceSource(lookup)
	if err != nil {
		t.Fatalf("new delivery place source: %v", err)
	}
	fixture.deps.Places = source
}

// Covers: 票面完成判据 2 前半——PS 答基线锚引用时执行器不再停在 DELIVERY_PLACE_SOURCE_NOT_WIRED：时间窗那条缝未接
// （票 13）就停到 DELIVERY_WINDOW_SOURCE_NOT_WIRED；三条都接上则任务形成，Place 就是 PS 的 `DPR-1:` 串逐字。
func TestAParcelShipmentBaselineReferenceMovesTheStopPastTheDeliveryPlaceSeam(t *testing.T) {
	reference := baselineAnchoredReference(t)
	referenced, err := psdomain.DeliveryPlaceReferenced(reference)
	if err != nil {
		t.Fatalf("referenced: %v", err)
	}

	t.Run("time window seam still unwired: the stop moves to that seam", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		lookup := &parcelShipmentLookupStub{byParcel: map[string]psdomain.DeliveryPlaceResolution{"parcel-1": referenced}}
		wireRealPlaces(t, fixture, lookup)
		fixture.deps.Windows = nil

		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("触发：%v", err)
		}
		if result.Outcome() != application.DeliveryDispatchUndecided || result.UndecidedReason() != application.DeliveryWindowSourceNotWired {
			t.Fatalf("outcome = %q reason = %s, want DISPATCH_UNDECIDED / DELIVERY_WINDOW_SOURCE_NOT_WIRED", result.Outcome(), result.UndecidedReason())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("未决没有续办引用——票 13 接上之后这一拍要能重跑")
		}
		if fixture.tasks.saves != 0 {
			t.Fatalf("时间窗缝未接却开了任务：saves=%d", fixture.tasks.saves)
		}
	})

	t.Run("all three seams wired: the task place is the PS reference verbatim", func(t *testing.T) {
		fixture := newDispatchTriggerFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		lookup := &parcelShipmentLookupStub{byParcel: map[string]psdomain.DeliveryPlaceResolution{"parcel-1": referenced}}
		wireRealPlaces(t, fixture, lookup)

		result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("触发：%v", err)
		}
		if result.Outcome() != application.DeliveryDispatchTaskFormed {
			t.Fatalf("outcome = %q, want DISPATCH_TASK_FORMED（reason=%s missing=%v）", result.Outcome(), result.UndecidedReason(), result.Missing())
		}
		record, recorded := result.Record()
		if !recorded {
			t.Fatal("任务形成了却没交回记录")
		}
		if got := record.Task.Place().String(); got != reference.String() || !strings.HasPrefix(got, "DPR-1:") {
			t.Fatalf("任务地点 = %q, want PS 引用串逐字 %q", got, reference.String())
		}
		if lookup.calls != 1 {
			t.Fatalf("PS 读口被问了 %d 次, want 1", lookup.calls)
		}
	})
}

// Covers: 票面完成判据 2 后半——PS 对集运单元答「没有收件地点」→ 执行器答 REQUIREMENT_MISSING / DELIVERY_PLACE，任务
// 待形成、段与交接一行不动、不补默认地点。「没有」是业务答案不是欠账：与 TestAnOwnerAnsweringMissingKeepsTheTaskUnformed
// 同一条纪律，不留续办引用（ADR-0114 决定三）。
func TestAConsolidationUnitWithoutADeliveryPlaceLeavesTheTaskUnformed(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "consolidation-unit-7", "FINAL_DELIVERY", handoverJudgedTime)
	lookup := &parcelShipmentLookupStub{}
	wireRealPlaces(t, fixture, lookup)
	segmentSaves, segmentJoins := fixture.handovers.segments.saves, fixture.handovers.segments.joins
	handoverSaves := fixture.handovers.handovers.saves

	result, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "consolidation-unit-7"))
	if err != nil {
		t.Fatalf("触发：%v", err)
	}
	if result.Outcome() != application.DeliveryDispatchRequirementMissing {
		t.Fatalf("outcome = %q, want REQUIREMENT_MISSING（reason=%s）", result.Outcome(), result.UndecidedReason())
	}
	if missing := result.Missing(); len(missing) != 1 || missing[0] != application.DeliveryPlaceRequirement {
		t.Fatalf("missing = %v, want [DELIVERY_PLACE]", missing)
	}
	if result.ContinuationReference() != "" {
		t.Fatal("所有者答「没有」是业务答案不是欠账，不该留续办引用")
	}
	if fixture.tasks.saves != 0 {
		t.Fatalf("PS 说没有收件地点却开了任务——填了默认地点：saves=%d", fixture.tasks.saves)
	}
	if fixture.handovers.segments.saves != segmentSaves || fixture.handovers.segments.joins != segmentJoins || fixture.handovers.handovers.saves != handoverSaves {
		t.Fatal("这一拍改动了段或交接——执行器对段登记册只读，不回滚交接")
	}
	if lookup.calls != 1 {
		t.Fatalf("PS 读口被问了 %d 次, want 1", lookup.calls)
	}
	if _, resolution, err := fixture.deps.Places.LoadDeliveryPlace(t.Context(), triggerCommand(t, "x").TenantID, objectRef(t, "consolidation-unit-7")); err != nil || resolution != ports.RequirementMissing {
		t.Fatalf("适配器对集运单元的答法 = %q %v, want MISSING", resolution, err)
	}
}
