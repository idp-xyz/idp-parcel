package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 证票 tf-face-algebra/14 的判零一半，挑两口：揽派任务的时间窗起点与派送尝试的计划窗起点。
// 编排接真登记册、每次调用一笔事务（同生产装配的事务包装）。登记册上为零点的时刻答`未受理`，事务照常提交，册上零行；
// 零点后恰一微秒照常登记，原样重放读得回。夹具沿用 storable_values_regression_test.go，全为合成值（S 级）。

func (fixture *storableFixture) openTask(task string, windowFrom time.Time) application.OpenDispatchTaskResult {
	fixture.t.Helper()
	return inTransaction(fixture.t, fixture.transactor, func(ctx context.Context) (application.OpenDispatchTaskResult, error) {
		return fixture.delivery.tasks.Open(ctx, application.OpenDispatchTaskCommand{
			TenantID: fixture.tenant, Task: task, Kind: domain.DeliveryDispatch, Objects: []string{"SYN-PARCEL-D1"},
			Place: "SYN-PLACE-1", WindowFrom: windowFrom, WindowTo: nsFrom, Conditions: "SYN-CONDITIONS-1", OpenedAt: nsFrom,
		})
	})
}

// Covers: 票 tf-face-algebra/14 完成判据「真库回归」——揽派任务时间窗起点在登记册上是零点答`未受理`，任务行与对象行零行；
// 零点后一微秒开立后原样重放答`已开立`且读回的起点是它在登记册上的样子。
func TestADispatchTaskWindowAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry(t *testing.T) {
	fixture := newStorableFixture(t)
	for name, at := range registryZeroWindow {
		if got := fixture.openTask("SYN-TASK-ZERO", at).Outcome(); got != application.DispatchTaskNotAccepted {
			t.Fatalf("%s = %s，期望 INPUT_NOT_ACCEPTED", name, got)
		}
	}
	if tasks, objects := fixture.rows("dispatch_task"), fixture.rows("dispatch_task_object"); tasks != 0 || objects != 0 {
		t.Fatalf("零点格落了库：任务行 %d、对象行 %d", tasks, objects)
	}

	atMicro := time.Time{}.Add(time.Microsecond)
	if got := fixture.openTask("SYN-TASK-MICRO", atMicro).Outcome(); got != application.DispatchTaskOpened {
		t.Fatalf("零点后 1µs 开立 = %s，期望 DISPATCH_TASK_OPENED", got)
	}
	replay := fixture.openTask("SYN-TASK-MICRO", atMicro)
	if replay.Outcome() != application.DispatchTaskAlreadyRegistered {
		t.Fatalf("零点后 1µs 原样重放 = %s，期望 DISPATCH_TASK_ALREADY_OPEN", replay.Outcome())
	}
	stored, _ := replay.Record()
	from, _ := stored.Task.Window()
	pinRoundTrip(t, "任务时间窗起点", from, atMicro)
}

// Covers: 票 tf-face-algebra/14 完成判据「真库回归」——派送尝试计划窗起点在登记册上是零点答`未受理`，尝试行与结果行零行；
// 零点后一微秒首次回传后原样重放答`原结果`且读回的起点是它在登记册上的样子。
func TestADeliveryAttemptPlanAtTheRegistryZeroIsNotAcceptedAgainstARealRegistry(t *testing.T) {
	fixture := newStorableFixture(t)
	delivery := fixture.delivery
	if got := delivery.open("SYN-TASK-D1", nsFrom.Add(-time.Hour), nsFrom.Add(12*time.Hour)).Outcome(); got != application.DispatchTaskOpened {
		t.Fatalf("开立派送任务 = %s", got)
	}
	for name, at := range registryZeroWindow {
		command := delivery.nsDeliveryCommand("SYN-ATTEMPT-ZERO")
		command.PlannedFrom = at
		if got := delivery.record(command).Outcome(); got != application.DeliveryAttemptNotAccepted {
			t.Fatalf("%s = %s，期望 INPUT_NOT_ACCEPTED", name, got)
		}
	}
	if attempts, results := fixture.rows("delivery_attempt"), fixture.rows("delivery_attempt_result"); attempts != 0 || results != 0 {
		t.Fatalf("零点格落了库：尝试行 %d、结果行 %d", attempts, results)
	}

	command := delivery.nsDeliveryCommand("SYN-ATTEMPT-MICRO")
	command.PlannedFrom = time.Time{}.Add(time.Microsecond)
	if got := delivery.record(command).Outcome(); got != application.DeliveryAttemptRecorded {
		t.Fatalf("零点后 1µs 首次回传 = %s", got)
	}
	replay := delivery.record(command)
	if replay.Outcome() != application.DeliveryAttemptExistingResult {
		t.Fatalf("零点后 1µs 原样重放 = %s，期望 EXISTING_RESULT", replay.Outcome())
	}
	stored, _ := replay.Record()
	pinRoundTrip(t, "计划窗起点", stored.Attempt.PlannedFrom(), command.PlannedFrom)
}
