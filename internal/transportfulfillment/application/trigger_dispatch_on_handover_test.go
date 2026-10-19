package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证派送发起的进程内入口（ADR-0216）：交接信封的键 → 读回交接 → 对象此刻在场的段 → 既有触发执行器；以及两层答复
// 怎样折成对信封的处置（入账 / 重投 / 缺陷）。执行器的各道门由 trigger_delivery_dispatch_test.go 证，这里只证入口不重判、
// 不漏判。夹具全部合成。

// dispatchOnHandoverAt 是派发进程这一拍的时钟读数，故意与交接判断时刻、进段时刻都不同：任务的成立时间要取它。
var dispatchOnHandoverAt = time.Date(2026, 8, 13, 16, 30, 0, 0, time.UTC)

type activeSegmentFinderStub struct {
	keys []ports.FulfillmentSegmentKey
	err  error
}

func (stub activeSegmentFinderStub) FindActiveSegments(
	_ context.Context, _ domain.TenantID, _ domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	return stub.keys, stub.err
}

// countingTrigger 记执行器被问了几次：入口自己答得出的格不该去问执行器。
type countingTrigger struct {
	inner application.DeliveryDispatchTrigger
	calls int
}

func (trigger *countingTrigger) Trigger(
	ctx context.Context, command application.TriggerDeliveryDispatchCommand,
) (application.TriggerDeliveryDispatchResult, error) {
	trigger.calls++
	return trigger.inner.Trigger(ctx, command)
}

type dispatchOnHandoverFixture struct {
	*dispatchTriggerFixture
	trigger *countingTrigger
	deps    application.TriggerDispatchOnHandoverDeps
}

func newDispatchOnHandoverFixture(t *testing.T) *dispatchOnHandoverFixture {
	t.Helper()
	base := newDispatchTriggerFixture(t)
	fixture := &dispatchOnHandoverFixture{dispatchTriggerFixture: base, trigger: &countingTrigger{inner: base.handler()}}
	fixture.deps = application.TriggerDispatchOnHandoverDeps{
		Handovers: base.handovers.handovers,
		Segments:  base.handovers.segments,
		Trigger:   fixture.trigger,
		Clock:     handoverClock{at: dispatchOnHandoverAt},
	}
	return fixture
}

func (fixture *dispatchOnHandoverFixture) run(t *testing.T, object string) application.TriggerDispatchOnHandoverResult {
	t.Helper()
	result, err := application.NewTriggerDispatchOnHandoverHandler(fixture.deps).Trigger(t.Context(), onHandoverCommand(t, object))
	if err != nil {
		t.Fatalf("%s 那封交接信封：%v", object, err)
	}
	return result
}

// onHandoverCommand 是 enterByHandover 登下的那一版交接的键：版本随对象。
func onHandoverCommand(t *testing.T, object string) application.TriggerDispatchOnHandoverCommand {
	t.Helper()
	return application.TriggerDispatchOnHandoverCommand{
		TenantID: value(t, domain.NewTenantID, "tenant-1"),
		Object:   object,
		Scope:    "handover-scope-1",
		Version:  "handover-result/" + object + "/v1",
	}
}

// Covers: AT-TF-191（ADR-0216 决定一、二）——对象凭`已交接`进入派送段、三条要求缝都给：交接信封一到就形成一项任务，段取
// 对象此刻在场的那一段，任务成立时间取派发进程这一拍的时钟。处置是入账。
func TestAHandoverIntoADeliverySegmentFormsTheTaskAtTheDispatchClock(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)

	result := fixture.run(t, "parcel-1")
	if result.Outcome() != application.DispatchOnHandoverTriggered {
		t.Fatalf("outcome = %s（%s）, want DISPATCH_TRIGGERED", result.Outcome(), result.UndecidedReason())
	}
	triggered := result.Trigger()
	if triggered.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("执行器 outcome = %s（refusal=%s undecided=%s）, want DISPATCH_TASK_FORMED",
			triggered.Outcome(), triggered.Refusal(), triggered.UndecidedReason())
	}
	record, _ := triggered.Record()
	if objects := record.Task.Objects(); len(objects) != 1 || objects[0].String() != "parcel-1" {
		t.Fatalf("任务对象集 = %v, want 只有 parcel-1", objects)
	}
	if !record.Task.OpenedAt().Equal(dispatchOnHandoverAt) {
		t.Fatalf("任务成立时间 = %s, want 派发这一拍的时钟 %s——不是交接判断时刻，也不是进段时刻",
			record.Task.OpenedAt(), dispatchOnHandoverAt)
	}
	if result.Disposition() != application.DispatchOnHandoverSettled {
		t.Fatalf("disposition = %s, want SETTLED", result.Disposition())
	}
}

// Covers: AT-TF-192（ADR-0216 决定三）——同一封信重跑（入口被调第二次）撞`已在册`，交回同一项任务，不重建；处置仍是入账。
func TestRerunningTheHandoverBeatFindsTheTaskAlreadyFormed(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)

	first := fixture.run(t, "parcel-1")
	second := fixture.run(t, "parcel-1")
	if second.Trigger().Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
		t.Fatalf("重跑执行器 outcome = %s, want DISPATCH_TASK_ALREADY_FORMED", second.Trigger().Outcome())
	}
	firstRecord, _ := first.Trigger().Record()
	secondRecord, _ := second.Trigger().Record()
	if secondRecord.Key.Task != firstRecord.Key.Task || fixture.tasks.saves != 1 {
		t.Fatalf("重跑交回 %s、任务写了 %d 次, want 首拍那一项、写 1 次", secondRecord.Key.Task, fixture.tasks.saves)
	}
	if second.Disposition() != application.DispatchOnHandoverSettled {
		t.Fatalf("disposition = %s, want SETTLED", second.Disposition())
	}
}

// Covers: AT-TF-193（ADR-0216 决定一、三）——拒收与待确认没有让控制移入接收方，不是触发事实：不问段、不问执行器，入账。
func TestARefusedOrPendingHandoverIsNotATrigger(t *testing.T) {
	for name, verdict := range map[string]domain.HandoverVerdict{
		"拒收":  domain.HandoverRefused,
		"待确认": domain.HandoverPendingConfirmation,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDispatchOnHandoverFixture(t)
			command := registerHandoverCommand(t)
			command.Version = "handover-result/parcel-1/v1"
			command.Verdict = verdict
			command.Basis = "handover-basis-1"
			command.Segment = "segment-1"
			command.SegmentServiceAction = "FINAL_DELIVERY"
			if registered, err := fixture.handovers.handler.Register(t.Context(), command); err != nil ||
				registered.Outcome() != application.HandoverRegistered {
				t.Fatalf("登记%s：%v %s", name, err, registered.Outcome())
			}

			result := fixture.run(t, "parcel-1")
			if result.Outcome() != application.DispatchOnHandoverNotHandedOver {
				t.Fatalf("outcome = %s, want HANDOVER_NOT_HANDED_OVER", result.Outcome())
			}
			if result.Disposition() != application.DispatchOnHandoverSettled || fixture.trigger.calls != 0 {
				t.Fatalf("disposition = %s、执行器被问 %d 次, want SETTLED、0 次", result.Disposition(), fixture.trigger.calls)
			}
		})
	}
}

// Covers: AT-TF-194（ADR-0216 决定一、三）——对象此刻不在任何段里在场（交接没声明段，或之后又被带走），入口自己答，入账；
// 进的段不是派送段，交执行器答`不是触发事实`，同样入账。两格都不形成任务。
func TestAnObjectNotInADeliverySegmentSettlesWithoutATask(t *testing.T) {
	t.Run("不在任何段里", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		command := registerHandoverCommand(t)
		if registered, err := fixture.handovers.handler.Register(t.Context(), command); err != nil ||
			registered.Outcome() != application.HandoverRegistered {
			t.Fatalf("登记不带段的交接：%v %s", err, registered.Outcome())
		}
		result := fixture.run(t, "parcel-1")
		if result.Outcome() != application.DispatchOnHandoverNoActiveSegment || fixture.trigger.calls != 0 {
			t.Fatalf("outcome = %s、执行器被问 %d 次, want NO_ACTIVE_SEGMENT、0 次", result.Outcome(), fixture.trigger.calls)
		}
		if result.Disposition() != application.DispatchOnHandoverSettled {
			t.Fatalf("disposition = %s, want SETTLED", result.Disposition())
		}
	})
	t.Run("段不是派送段", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "LINEHAUL", handoverJudgedTime)
		result := fixture.run(t, "parcel-1")
		if result.Trigger().Outcome() != application.DeliveryDispatchNotTriggered ||
			result.Trigger().Refusal() != application.DeliveryTriggerRefusedSegmentNotDelivery {
			t.Fatalf("执行器 = %s/%s, want NOT_A_DELIVERY_TRIGGER/SEGMENT_NOT_DELIVERY",
				result.Trigger().Outcome(), result.Trigger().Refusal())
		}
		if result.Disposition() != application.DispatchOnHandoverSettled || fixture.tasks.saves != 0 {
			t.Fatalf("disposition = %s、任务写 %d 次, want SETTLED、0 次", result.Disposition(), fixture.tasks.saves)
		}
	})
}

// Covers: AT-TF-195（ADR-0216 决定三）——所有者答「没有」是业务答案：入账不重投，任务保持待形成。
func TestARequirementMissingSettlesTheEnvelope(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.windows.resolution = ports.RequirementMissing

	result := fixture.run(t, "parcel-1")
	if result.Trigger().Outcome() != application.DeliveryDispatchRequirementMissing {
		t.Fatalf("执行器 outcome = %s, want REQUIREMENT_MISSING", result.Trigger().Outcome())
	}
	if result.Disposition() != application.DispatchOnHandoverSettled || fixture.tasks.saves != 0 {
		t.Fatalf("disposition = %s、任务写 %d 次, want SETTLED、0 次", result.Disposition(), fixture.tasks.saves)
	}
}

// Covers: AT-TF-196（ADR-0216 决定三）——读不动的依赖是重投：交接登记册、交接还读不回、段登记册、执行器的要求缝。修好后
// 原样重跑就形成任务。
func TestUnreadableDependenciesAreRetried(t *testing.T) {
	t.Run("交接登记册读不动", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.handovers.handovers.findErr = errors.New("handover registry down")
		result := fixture.run(t, "parcel-1")
		assertDispatchOnHandoverRetry(t, result, application.DispatchOnHandoverHandoverRegistryUnavailable)
	})
	t.Run("交接还读不回", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		result := fixture.run(t, "parcel-1")
		assertDispatchOnHandoverRetry(t, result, application.DispatchOnHandoverHandoverNotVisible)
	})
	t.Run("段登记册读不动", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		fixture.deps.Segments = activeSegmentFinderStub{err: errors.New("segment registry down")}
		result := fixture.run(t, "parcel-1")
		assertDispatchOnHandoverRetry(t, result, application.DispatchOnHandoverSegmentRegistryUnavailable)
	})
	t.Run("要求缝读不动，修好后重跑形成任务", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		fixture.places.err = errors.New("parcel shipment down")
		result := fixture.run(t, "parcel-1")
		if result.Trigger().Outcome() != application.DeliveryDispatchUndecided ||
			result.Trigger().UndecidedReason() != application.DeliveryPlaceSourceUnavailable {
			t.Fatalf("执行器 = %s/%s, want DISPATCH_UNDECIDED/DELIVERY_PLACE_SOURCE_UNAVAILABLE",
				result.Trigger().Outcome(), result.Trigger().UndecidedReason())
		}
		if result.Disposition() != application.DispatchOnHandoverRetry {
			t.Fatalf("disposition = %s, want RETRY", result.Disposition())
		}
		fixture.places.err = nil
		if rerun := fixture.run(t, "parcel-1"); rerun.Trigger().Outcome() != application.DeliveryDispatchTaskFormed {
			t.Fatalf("修好后重跑 = %s, want DISPATCH_TASK_FORMED", rerun.Trigger().Outcome())
		}
	})
}

// Covers: AT-TF-198（ADR-0216 决定三）——本上下文自己的缺陷不入账也不重投：键过不了自己的构造门、装配缺一条要求缝。
// 一对象同时在两个段里在场是库面不一致，原样上抛，不挑一个。
func TestOwnDefectsAreNeitherSettledNorRetried(t *testing.T) {
	t.Run("键过不了构造门", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		command := onHandoverCommand(t, "parcel-1")
		command.Version = "   "
		result, err := application.NewTriggerDispatchOnHandoverHandler(fixture.deps).Trigger(t.Context(), command)
		if err != nil || result.Outcome() != application.DispatchOnHandoverNotAccepted {
			t.Fatalf("outcome = %s err = %v, want INPUT_NOT_ACCEPTED", result.Outcome(), err)
		}
		if result.Disposition() != application.DispatchOnHandoverDefect {
			t.Fatalf("disposition = %s, want DEFECT", result.Disposition())
		}
	})
	t.Run("装配缺一条要求缝", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		fixture.dispatchTriggerFixture.deps.Conditions = nil
		fixture.trigger.inner = fixture.handler()
		result := fixture.run(t, "parcel-1")
		if result.Trigger().UndecidedReason() != application.DeliveryConditionSourceNotWired {
			t.Fatalf("执行器未决理由 = %s, want DELIVERY_CONDITION_SOURCE_NOT_WIRED", result.Trigger().UndecidedReason())
		}
		if result.Disposition() != application.DispatchOnHandoverDefect {
			t.Fatalf("disposition = %s, want DEFECT——缝没接是装配缺件，原样重投接不上", result.Disposition())
		}
	})
	t.Run("同时在两个段里在场", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		tenant := value(t, domain.NewTenantID, "tenant-1")
		fixture.deps.Segments = activeSegmentFinderStub{keys: []ports.FulfillmentSegmentKey{
			{TenantID: tenant, Segment: value(t, domain.NewFulfillmentSegmentReference, "segment-1")},
			{TenantID: tenant, Segment: value(t, domain.NewFulfillmentSegmentReference, "segment-2")},
		}}
		_, err := application.NewTriggerDispatchOnHandoverHandler(fixture.deps).Trigger(t.Context(), onHandoverCommand(t, "parcel-1"))
		if !errors.Is(err, application.ErrObjectActiveInSeveralSegments) || fixture.trigger.calls != 0 {
			t.Fatalf("err = %v、执行器被问 %d 次, want ErrObjectActiveInSeveralSegments、0 次", err, fixture.trigger.calls)
		}
	})
}

func assertDispatchOnHandoverRetry(
	t *testing.T,
	result application.TriggerDispatchOnHandoverResult,
	reason application.DispatchOnHandoverUndecidedReason,
) {
	t.Helper()
	if result.Outcome() != application.DispatchOnHandoverUndecided || result.UndecidedReason() != reason {
		t.Fatalf("outcome = %s/%s, want DISPATCH_ON_HANDOVER_UNDECIDED/%s", result.Outcome(), result.UndecidedReason(), reason)
	}
	if result.Disposition() != application.DispatchOnHandoverRetry {
		t.Fatalf("disposition = %s, want RETRY", result.Disposition())
	}
}
