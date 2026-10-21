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

// 本文件证按册重问（ADR-0230 决定四、五）：逐件交回同一个派送发起执行器，幂等、不另立任务；到期重问按租户的重问间隔筛，间隔没登记
// 答未配置、一件也不问。夹具全部合成。

var reaskAt = deliveryTriggeredAt.Add(12 * time.Hour)

type cadenceReaderStub struct {
	record ports.DeliveryDispatchReaskCadenceRecord
	found  bool
	err    error
}

func (stub cadenceReaderStub) CurrentCadence(context.Context, domain.TenantID) (ports.DeliveryDispatchReaskCadenceRecord, bool, error) {
	return stub.record, stub.found, stub.err
}

func cadenceOf(t *testing.T, interval time.Duration) cadenceReaderStub {
	t.Helper()
	cadence, err := domain.NewDeliveryDispatchReaskCadence(value(t, domain.NewTenantID, "tenant-1"),
		value(t, domain.NewDeliveryDispatchReaskCadenceVersion, "reask/v1"), interval)
	if err != nil {
		t.Fatal(err)
	}
	return cadenceReaderStub{record: ports.DeliveryDispatchReaskCadenceRecord{Cadence: cadence}, found: true}
}

type failingQueue struct{}

func (failingQueue) ListPending(context.Context, domain.TenantID, time.Time, int) ([]ports.DeliveryDispatchPendingRecord, error) {
	return nil, errors.New("pending register down")
}

func (fixture *dispatchTriggerFixture) reask(
	t *testing.T, cadences ports.DeliveryDispatchReaskCadenceReader, scope application.ReaskScope, limit int,
) application.ReaskPendingDeliveryDispatchesResult {
	t.Helper()
	result, err := application.NewReaskPendingDeliveryDispatchesHandler(application.ReaskPendingDeliveryDispatchesDeps{
		Pending:  fixture.pending,
		Cadences: cadences,
		Trigger:  fixture.handler(),
		Clock:    handoverClock{at: reaskAt},
	}).Reask(t.Context(), application.ReaskPendingDeliveryDispatchesCommand{
		TenantID: value(t, domain.NewTenantID, "tenant-1"), Scope: scope, Limit: limit,
	})
	if err != nil {
		t.Fatalf("重问：%v", err)
	}
	return result
}

func (fixture *dispatchTriggerFixture) askedCalls() int {
	return fixture.places.calls + fixture.windows.calls + fixture.conditions.calls
}

// Covers: AT-TF-285（ADR-0230 决定四）——按册重问全部：册上两件逐件交回执行器，所有者补齐了的那一件形成任务、册上了结，仍缺的那一件
// 刷新最近一次答复时刻；再问一次，了结的不再进队，仍缺的再问一次；任务始终只一项。各件的业务时间取这一次重问的时刻。
func TestReaskingThePendingRegisterRerunsTheExecutorAndNeverFormsASecondTask(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.enterByHandoverWithPlan(t, "parcel-2", "", "planned-without-window", handoverJudgedTime)
	fixture.windows.resolution = ports.RequirementMissing
	fixture.triggerAt(t, "parcel-1", deliveryTriggeredAt)
	fixture.triggerAt(t, "parcel-2", deliveryTriggeredAt.Add(time.Minute))

	// parcel-1 的计划段补上了窗口；parcel-2 的仍没有。
	fixture.windows.resolution = ports.RequirementResolved
	fixture.deps.Windows = windowOnlyFor{inner: fixture.windows}
	first := fixture.reask(t, cadenceReaderStub{}, application.ReaskAllPending, 10)
	items := first.Items()
	if first.Outcome() != application.ReaskAsked || len(items) != 2 {
		t.Fatalf("第一次重问 = %s、%d 件, want PENDING_DELIVERY_DISPATCHES_REASKED、2 件", first.Outcome(), len(items))
	}
	if items[0].Object.String() != "parcel-1" || items[0].Answer.Outcome() != application.DeliveryDispatchTaskFormed ||
		items[1].Object.String() != "parcel-2" || items[1].Answer.Outcome() != application.DeliveryDispatchRequirementMissing {
		t.Fatalf("逐件答复 = %s %s / %s %s, want parcel-1 形成、parcel-2 仍缺", items[0].Object, items[0].Answer.Outcome(),
			items[1].Object, items[1].Answer.Outcome())
	}
	task, _ := items[0].Answer.Record()
	if !task.Task.OpenedAt().Equal(reaskAt) {
		t.Fatalf("任务成立时间 = %s, want 这一次重问的时刻 %s", task.Task.OpenedAt(), reaskAt)
	}
	if row := fixture.pendingRow(t, "parcel-1"); row.State != ports.DeliveryDispatchPendingFormed {
		t.Fatalf("parcel-1 册上 = %+v, want 已形成", row)
	}
	if row := fixture.pendingRow(t, "parcel-2"); row.State != ports.DeliveryDispatchPending || !row.LastAnsweredAt.Equal(reaskAt) {
		t.Fatalf("parcel-2 册上 = %+v, want 仍待形成、最近一次答复是这一次重问", row)
	}

	second := fixture.reask(t, cadenceReaderStub{}, application.ReaskAllPending, 10)
	if items := second.Items(); len(items) != 1 || items[0].Object.String() != "parcel-2" {
		t.Fatalf("第二次重问问了 %v, want 只有仍待形成的 parcel-2", items)
	}
	if fixture.tasks.saves != 1 {
		t.Fatalf("任务写了 %d 次, want 1", fixture.tasks.saves)
	}
}

// Covers: AT-TF-285（ADR-0230 决定四）——册上一件的任务其实已在册（例如在两次重问之间被补拍形成、册却停在待形成）：重问答`已在册`
// 并了结它，不另立；limit 截断从最近一次答复最早的那一件起。
func TestReaskingAnEntryWhoseTaskAlreadyExistsSettlesIt(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.enterByHandover(t, "parcel-2", "", handoverJudgedTime)
	fixture.triggerAt(t, "parcel-1", deliveryTriggeredAt)
	tenant := value(t, domain.NewTenantID, "tenant-1")
	for index, object := range []string{"parcel-1", "parcel-2"} {
		key := ports.DeliveryDispatchPendingKey{TenantID: tenant, Segment: value(t, domain.NewFulfillmentSegmentReference, "segment-1"),
			Object: value(t, domain.NewCarriedObjectReference, object)}
		if err := fixture.pending.RecordMissing(t.Context(), key, value(t, domain.NewParticipationBasisReference, "basis"),
			[]string{"DELIVERY_PLACE"}, deliveryTriggeredAt.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	result := fixture.reask(t, cadenceReaderStub{}, application.ReaskAllPending, 1)
	items := result.Items()
	if len(items) != 1 || items[0].Object.String() != "parcel-1" || items[0].Answer.Outcome() != application.DeliveryDispatchTaskAlreadyFormed {
		t.Fatalf("重问 = %v, want 只问最早的 parcel-1、答已在册", items)
	}
	if row := fixture.pendingRow(t, "parcel-1"); row.State != ports.DeliveryDispatchPendingFormed || fixture.tasks.saves != 1 {
		t.Fatalf("parcel-1 册上 = %+v、任务写了 %d 次, want 已形成、1 次", row, fixture.tasks.saves)
	}
}

// Covers: AT-TF-286（ADR-0230 决定五）——到期重问而本租户没登记重问间隔：答`未配置`，一件也不问、所有者一次也不被问；不替租户选
// 一个间隔。全部重问不看间隔，照问。
func TestReaskingWhatIsDueWithoutARegisteredCadenceAnswersNotConfigured(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.places.resolution = ports.RequirementMissing
	fixture.triggerAt(t, "parcel-1", deliveryTriggeredAt)
	asked := fixture.askedCalls()

	result := fixture.reask(t, cadenceReaderStub{}, application.ReaskDuePending, 10)
	if result.Outcome() != application.ReaskCadenceNotConfigured || len(result.Items()) != 0 || fixture.askedCalls() != asked {
		t.Fatalf("到期重问 = %s、%d 件、所有者多被问 %d 次, want REASK_CADENCE_NOT_CONFIGURED、0、0",
			result.Outcome(), len(result.Items()), fixture.askedCalls()-asked)
	}
	if _, _, has := result.DueBy(); has {
		t.Fatal("未配置却给了到期时刻")
	}
	if all := fixture.reask(t, cadenceReaderStub{}, application.ReaskAllPending, 10); len(all.Items()) != 1 {
		t.Fatalf("全部重问 = %d 件, want 1——它不看间隔", len(all.Items()))
	}
}

// Covers: AT-TF-291（ADR-0230 决定五）——登记了重问间隔：只问最近一次答复已过间隔的那几件，到期时刻与所凭的那一版随结果交回。
func TestReaskingWhatIsDueAsksOnlyEntriesPastTheCadence(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.enterByHandover(t, "parcel-2", "", handoverJudgedTime)
	fixture.conditions.resolution = ports.RequirementMissing
	fixture.triggerAt(t, "parcel-1", reaskAt.Add(-7*time.Hour))
	fixture.triggerAt(t, "parcel-2", reaskAt.Add(-time.Hour))

	result := fixture.reask(t, cadenceOf(t, 6*time.Hour), application.ReaskDuePending, 10)
	dueBy, cadence, has := result.DueBy()
	if items := result.Items(); result.Outcome() != application.ReaskAsked || len(items) != 1 || items[0].Object.String() != "parcel-1" {
		t.Fatalf("到期重问 = %s %v, want 只问 parcel-1", result.Outcome(), items)
	}
	if !has || !dueBy.Equal(reaskAt.Add(-6*time.Hour)) || cadence.Cadence.Version().String() != "reask/v1" {
		t.Fatalf("到期时刻 = %s（%v）凭 %s, want 重问时刻减六小时、凭 reask/v1", dueBy, has, cadence.Cadence.Version())
	}
}

// 重问不受理与未决：问法不在封闭集、limit 越界答不受理；间隔册或待形成册读不动答未决、点名哪一本。
func TestReaskRefusesBadInputAndNamesTheRegisterItCannotRead(t *testing.T) {
	fixture := newDispatchTriggerFixture(t)
	for name, command := range map[string]application.ReaskPendingDeliveryDispatchesCommand{
		"问法缺":       {Scope: application.ReaskScopeInvalid, Limit: 10},
		"limit 为零":  {Scope: application.ReaskAllPending, Limit: 0},
		"limit 超上限": {Scope: application.ReaskAllPending, Limit: application.MaxReaskBatch + 1},
	} {
		command.TenantID = value(t, domain.NewTenantID, "tenant-1")
		result, err := application.NewReaskPendingDeliveryDispatchesHandler(application.ReaskPendingDeliveryDispatchesDeps{
			Pending: fixture.pending, Cadences: cadenceReaderStub{}, Trigger: fixture.handler(), Clock: handoverClock{at: reaskAt},
		}).Reask(t.Context(), command)
		if err != nil || result.Outcome() != application.ReaskNotAccepted {
			t.Errorf("%s：%s err=%v, want INPUT_NOT_ACCEPTED", name, result.Outcome(), err)
		}
	}
	if result := fixture.reask(t, cadenceReaderStub{err: errors.New("cadence register down")}, application.ReaskDuePending, 10); result.Outcome() != application.ReaskUndecided ||
		result.UndecidedReason() != application.ReaskCadenceRegisterUnavailable {
		t.Errorf("间隔册读不动 = %s %s", result.Outcome(), result.UndecidedReason())
	}
	result, err := application.NewReaskPendingDeliveryDispatchesHandler(application.ReaskPendingDeliveryDispatchesDeps{
		Pending: failingQueue{}, Cadences: cadenceReaderStub{}, Trigger: fixture.handler(), Clock: handoverClock{at: reaskAt},
	}).Reask(t.Context(), application.ReaskPendingDeliveryDispatchesCommand{
		TenantID: value(t, domain.NewTenantID, "tenant-1"), Scope: application.ReaskAllPending, Limit: 10,
	})
	if err != nil || result.Outcome() != application.ReaskUndecided || result.UndecidedReason() != application.ReaskPendingRegisterUnavailable {
		t.Errorf("待形成册读不动 = %s %s err=%v", result.Outcome(), result.UndecidedReason(), err)
	}
}

// windowOnlyFor 让时间窗那一缝只对带指名计划段的参与答缺失、其余照内层答：扮演「一件补上了、另一件还没有」。
type windowOnlyFor struct {
	inner *deliveryWindowStub
}

func (window windowOnlyFor) LoadDeliveryWindow(
	ctx context.Context, tenant domain.TenantID, planned domain.PlannedSegmentReference, present bool,
) (time.Time, time.Time, ports.RequirementResolution, error) {
	if present && planned.String() == "planned-without-window" {
		return time.Time{}, time.Time{}, ports.RequirementMissing, nil
	}
	return window.inner.LoadDeliveryWindow(ctx, tenant, planned, present)
}
