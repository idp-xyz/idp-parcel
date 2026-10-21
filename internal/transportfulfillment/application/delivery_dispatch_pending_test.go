package application_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证执行器写派送待形成册（ADR-0230 决定一、二）：`要求缺失`立或刷新一条，任务形成了结，对象已不在段里作罢；同段同对象的
// 替代参与是同一条。夹具全部合成。

func (fixture *dispatchTriggerFixture) pendingRow(t *testing.T, object string) ports.DeliveryDispatchPendingRecord {
	t.Helper()
	row, found := fixture.pending.row("tenant-1", "segment-1", object)
	if !found {
		t.Fatalf("册上没有 %s 那一条", object)
	}
	return row
}

func (fixture *dispatchTriggerFixture) triggerAt(t *testing.T, object string, at time.Time) application.TriggerDeliveryDispatchResult {
	t.Helper()
	command := triggerCommand(t, object)
	command.OccurredAt = at
	result, err := fixture.handler().Trigger(t.Context(), command)
	if err != nil {
		t.Fatalf("%s 那一拍：%v", object, err)
	}
	return result
}

// Covers: AT-TF-281（ADR-0230 决定一）——所有者答「没有」：执行器答`要求缺失`的同一拍把这一件记进册，段、对象、链尾的入场依据、
// 缺哪几件、答复时刻都在；再问一次仍缺，册上仍一条，缺件与最近一次答复时刻换成这一次，首次答复时刻不动。
func TestARequirementMissingAnswerIsRecordedOnThePendingRegister(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.places.resolution = ports.RequirementMissing
	fixture.conditions.resolution = ports.RequirementMissing

	first := fixture.triggerAt(t, "parcel-1", deliveryTriggeredAt)
	if first.Outcome() != application.DeliveryDispatchRequirementMissing {
		t.Fatalf("outcome = %s, want REQUIREMENT_MISSING", first.Outcome())
	}
	row := fixture.pendingRow(t, "parcel-1")
	if row.State != ports.DeliveryDispatchPending ||
		!slices.Equal(row.Missing, []string{"DELIVERY_PLACE", "DELIVERY_CONDITION"}) ||
		!row.FirstAnsweredAt.Equal(deliveryTriggeredAt) || !row.LastAnsweredAt.Equal(deliveryTriggeredAt) {
		t.Fatalf("册上那一条 = %+v, want 待形成、缺地点与条件、两个答复时刻都是这一拍", row)
	}
	if want := "TRANSPORT-HANDOVER-KEY/8:parcel-1/16:handover-scope-1/27:handover-result/parcel-1/v1"; row.EntryBasis.String() != want {
		t.Fatalf("入场依据 = %s, want 链尾那一版 %s", row.EntryBasis, want)
	}

	fixture.places.resolution = ports.RequirementResolved
	later := deliveryTriggeredAt.Add(6 * time.Hour)
	fixture.triggerAt(t, "parcel-1", later)
	row = fixture.pendingRow(t, "parcel-1")
	if len(fixture.pending.rows) != 1 || !slices.Equal(row.Missing, []string{"DELIVERY_CONDITION"}) ||
		!row.FirstAnsweredAt.Equal(deliveryTriggeredAt) || !row.LastAnsweredAt.Equal(later) {
		t.Fatalf("再问之后册 = %d 条、那一条 %+v, want 仍一条、只缺条件、首次不动、最近一次是这一拍", len(fixture.pending.rows), row)
	}
}

// Covers: AT-TF-282（ADR-0230 决定一）——补齐之后再问一拍，任务形成，册上那一条了结为已形成、指名那一项任务；再打一拍答`已在册`，
// 册不改指。顺利形成、从没缺过的对象不进册。
func TestAPendingEntryIsSettledWhenTheTaskForms(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.enterByHandover(t, "parcel-2", "", handoverJudgedTime)
	fixture.windows.resolution = ports.RequirementMissing
	fixture.triggerAt(t, "parcel-1", deliveryTriggeredAt)

	fixture.windows.resolution = ports.RequirementResolved
	settledAt := deliveryTriggeredAt.Add(time.Hour)
	formed := fixture.triggerAt(t, "parcel-1", settledAt)
	task, _ := formed.Record()
	row := fixture.pendingRow(t, "parcel-1")
	if formed.Outcome() != application.DeliveryDispatchTaskFormed || row.State != ports.DeliveryDispatchPendingFormed ||
		row.Task != task.Key.Task.String() || !row.SettledAt.Equal(settledAt) {
		t.Fatalf("形成之后 = %s，册上那一条 %+v, want 已形成、指名 %s、了结时刻是这一拍", formed.Outcome(), row, task.Key.Task)
	}

	if again := fixture.triggerAt(t, "parcel-1", settledAt.Add(time.Hour)); again.Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
		t.Fatalf("再打一拍 = %s, want DISPATCH_TASK_ALREADY_FORMED", again.Outcome())
	}
	if row := fixture.pendingRow(t, "parcel-1"); !row.SettledAt.Equal(settledAt) {
		t.Fatalf("已形成的那一条被改写：%+v", row)
	}

	if result := fixture.triggerAt(t, "parcel-2", deliveryTriggeredAt); result.Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("parcel-2 = %s, want DISPATCH_TASK_FORMED", result.Outcome())
	}
	if _, found := fixture.pending.row("tenant-1", "segment-1", "parcel-2"); found {
		t.Fatal("从没缺过的对象进了册")
	}
}

// Covers: AT-TF-283（ADR-0230 决定一）——待形成的那一件，对象之后不在段里了（交接更正为拒收、链尾失效）：再问一拍答`不是触发事实`，
// 册上那一条作罢、记下执行器答的那一格；作罢之后不再列为待形成。
func TestAPendingEntryLapsesWhenTheObjectIsNoLongerInTheSegment(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.conditions.resolution = ports.RequirementMissing
	fixture.run(t, "parcel-1")

	refused := correctHandoverCommand(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")
	if result, err := fixture.handovers.handler.Correct(t.Context(), refused); err != nil ||
		result.Outcome() != application.HandoverCorrected {
		t.Fatalf("更正为拒收：%v %s", err, result.Outcome())
	}
	lapsedAt := deliveryTriggeredAt.Add(3 * time.Hour)
	result := fixture.triggerAt(t, "parcel-1", lapsedAt)
	row := fixture.pendingRow(t, "parcel-1")
	if result.Refusal() != application.DeliveryTriggerRefusedObjectNotInSegment ||
		row.State != ports.DeliveryDispatchPendingLapsed || row.LapsedBy != "OBJECT_NOT_IN_SEGMENT" || !row.SettledAt.Equal(lapsedAt) {
		t.Fatalf("链尾失效之后 = %s，册上那一条 %+v, want 作罢、记 OBJECT_NOT_IN_SEGMENT", result.Refusal(), row)
	}
	if pending, _ := fixture.pending.ListPending(t.Context(), row.Key.TenantID, time.Time{}, 10); len(pending) != 0 {
		t.Fatalf("作罢之后仍列为待形成：%v", pending)
	}
}

// Covers: AT-TF-290 的册那一半（ADR-0230 决定二；票 demo-chain-gaps/129）——前版那一拍答了要求缺失，交接更正后仍为`已交接`：
// 更正版信封再答要求缺失，册上仍是同一条，入场依据换成链尾那一版；补齐后形成的那一项了结它。
func TestASupersedingParticipationIsTheSamePendingEntry(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.windows.resolution = ports.RequirementMissing
	fixture.run(t, "parcel-1")
	fixture.correctToHandedOver(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")

	fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v2")
	row := fixture.pendingRow(t, "parcel-1")
	if want := "TRANSPORT-HANDOVER-KEY/8:parcel-1/16:handover-scope-1/27:handover-result/parcel-1/v2"; len(fixture.pending.rows) != 1 ||
		row.EntryBasis.String() != want || row.State != ports.DeliveryDispatchPending {
		t.Fatalf("更正之后册 = %d 条、那一条 %+v, want 仍一条、入场依据 %s", len(fixture.pending.rows), row, want)
	}

	fixture.windows.resolution = ports.RequirementResolved
	fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v2")
	if row := fixture.pendingRow(t, "parcel-1"); row.State != ports.DeliveryDispatchPendingFormed || fixture.tasks.saves != 1 {
		t.Fatalf("补齐之后 = %+v、任务写了 %d 次, want 已形成、1 次", row, fixture.tasks.saves)
	}
}

// Covers: AT-TF-281（ADR-0230 决定一）——册读写不动时这一拍不答：执行器交回 ErrDeliveryDispatchPendingUnavailable，派送发起把它当
// 错误往上交、整拍随事务回滚；装配没交册是另一个错误，与读写不动分开。
func TestTheBeatGivesNoAnswerWhenThePendingRegisterCannotBeWritten(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.places.resolution = ports.RequirementMissing
	fixture.pending.err = errors.New("pending register down")

	_, err := application.NewTriggerDispatchOnHandoverHandler(fixture.deps).Trigger(t.Context(), onHandoverCommand(t, "parcel-1"))
	if !errors.Is(err, application.ErrDeliveryDispatchPendingUnavailable) {
		t.Fatalf("err = %v, want ErrDeliveryDispatchPendingUnavailable", err)
	}

	fixture.deps.Trigger = func() application.DeliveryDispatchTrigger {
		deps := fixture.dispatchTriggerFixture.deps
		deps.Pending = nil
		return application.NewTriggerDeliveryDispatchHandler(deps)
	}()
	_, err = application.NewTriggerDispatchOnHandoverHandler(fixture.deps).Trigger(t.Context(), onHandoverCommand(t, "parcel-1"))
	if !errors.Is(err, application.ErrDeliveryDispatchPendingRegisterNotWired) {
		t.Fatalf("err = %v, want ErrDeliveryDispatchPendingRegisterNotWired", err)
	}
}
