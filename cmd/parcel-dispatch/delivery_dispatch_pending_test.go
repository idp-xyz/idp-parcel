package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证派送待形成册随交接信封那一拍写（ADR-0230 决定一）：要求缺失与 inbox 那一笔同笔落册，重投不重复
// 写册；补齐之后补拍形成任务，册上那一条了结。交接经生产登记编排登记，册、任务、inbox 都是真表；只三条要求缝是替身。夹具全部合成，
// 只记 `S`。

func (fixture *handoverDispatchFixture) pendingRow(t *testing.T, object string) (tfports.DeliveryDispatchPendingRecord, bool) {
	t.Helper()
	register, err := tfpostgres.NewDeliveryDispatchPendings(fixture.db)
	if err != nil {
		t.Fatalf("派送待形成册：%v", err)
	}
	record, found, err := register.FindByKey(t.Context(), tfports.DeliveryDispatchPendingKey{
		TenantID: mustTF(t, tfdomain.NewTenantID, handoverDispatchTenant),
		Segment:  mustTF(t, tfdomain.NewFulfillmentSegmentReference, handoverDispatchSegment),
		Object:   mustTF(t, tfdomain.NewCarriedObjectReference, object),
	})
	if err != nil {
		t.Fatalf("读回册上 %s：%v", object, err)
	}
	return record, found
}

func pendingRowsOf(t *testing.T, fixture *handoverDispatchFixture) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.delivery_dispatch_pending WHERE tenant_id = $1`, handoverDispatchTenant,
	).Scan(&count); err != nil {
		t.Fatalf("数册上的行：%v", err)
	}
	return count
}

// Covers: AT-TF-281、AT-TF-282（ADR-0230 决定一）——交接信封那一拍所有者答「没有」时间窗：inbox 一行、册上一条（段、对象、入场
// 依据、缺时间窗、答复时刻是派发这一拍的时钟），任务 0 项；同一封重投在消费门就停下，册仍一条、最近一次答复时刻不动。所有者补齐
// 之后经补拍口再打一拍，任务形成，册上那一条了结、指名那一项。
func TestARequirementMissingOnTheHandoverLaneIsRecordedWithTheInboxAndSettledByTheRetick(t *testing.T) {
	fixture := newHandoverDispatchFixture(t)
	fixture.owners.missingWindow = true
	fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-70-A", 1, handoverDispatchSegment,
		tfdomain.SegmentServesFinalDelivery.String()))

	for range 2 {
		if err := fixture.deliver(t, "SYN-PARCEL-70-A", 1); err != nil {
			t.Fatalf("投递交接信封 = %v, want 入账", err)
		}
	}
	if n := inboxRowsOf(t, fixture.db, handoverDispatchConsumerName, handoverDispatchEventID("SYN-PARCEL-70-A", 1)); n != 1 {
		t.Fatalf("inbox 行 = %d, want 1", n)
	}
	row, found := fixture.pendingRow(t, "SYN-PARCEL-70-A")
	if !found || pendingRowsOf(t, fixture) != 1 || dispatchTaskRows(t, fixture.db) != 0 {
		t.Fatalf("册上 %d 行、任务 %d 项, want 1、0", pendingRowsOf(t, fixture), dispatchTaskRows(t, fixture.db))
	}
	wantBasis := "TRANSPORT-HANDOVER-KEY/"
	for index, part := range []string{"SYN-PARCEL-70-A", handoverDispatchScope, handoverDispatchVersion("SYN-PARCEL-70-A", 1)} {
		if index > 0 {
			wantBasis += "/"
		}
		wantBasis += strconv.Itoa(len(part)) + ":" + part
	}
	if row.State != tfports.DeliveryDispatchPending || !slices.Equal(row.Missing, []string{"DELIVERY_WINDOW"}) ||
		row.EntryBasis.String() != wantBasis ||
		!row.FirstAnsweredAt.Equal(handoverDispatchAt) || !row.LastAnsweredAt.Equal(handoverDispatchAt) {
		t.Fatalf("册上那一条 = %+v, want 待形成、缺时间窗、入场依据 %s、答复时刻 %s", row, wantBasis, handoverDispatchAt)
	}

	fixture.owners.missingWindow = false
	retickAt := handoverDispatchAt.Add(3 * time.Hour)
	result, err := fixture.retick(t, "SYN-PARCEL-70-A", retickAt)
	if err != nil || result.Outcome() != tfapplication.DeliveryDispatchTaskFormed {
		t.Fatalf("补拍 = %s err=%v, want DISPATCH_TASK_FORMED", result.Outcome(), err)
	}
	task, _ := result.Record()
	row, _ = fixture.pendingRow(t, "SYN-PARCEL-70-A")
	if row.State != tfports.DeliveryDispatchPendingFormed || row.Task != task.Key.Task.String() || !row.SettledAt.Equal(retickAt) {
		t.Fatalf("补拍之后册上那一条 = %+v, want 已形成、指名 %s", row, task.Key.Task)
	}
}

// Covers: AT-TF-281（ADR-0230 决定一）——某条要求缝读不动时这一拍未决：信封登哨兵重投，inbox 不落、册也不落（同一笔回滚）；顺利
// 形成的对象不进册。
func TestAnUndecidedOrFormedBeatLeavesThePendingRegisterAlone(t *testing.T) {
	fixture := newHandoverDispatchFixture(t)
	fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-70-B", 1, handoverDispatchSegment,
		tfdomain.SegmentServesFinalDelivery.String()))
	fixture.owners.placeErr = errors.New("SYN place owner unreachable")
	if err := fixture.deliver(t, "SYN-PARCEL-70-B", 1); err == nil {
		t.Fatal("缝读不动却入账了")
	}
	if pendingRowsOf(t, fixture) != 0 {
		t.Fatalf("未决的一拍写了册：%d 行", pendingRowsOf(t, fixture))
	}
	fixture.owners.placeErr = nil
	if err := fixture.deliver(t, "SYN-PARCEL-70-B", 1); err != nil {
		t.Fatalf("缝恢复后重投：%v", err)
	}
	if pendingRowsOf(t, fixture) != 0 || dispatchTaskRows(t, fixture.db) != 1 {
		t.Fatalf("顺利形成之后册 %d 行、任务 %d 项, want 0、1", pendingRowsOf(t, fixture), dispatchTaskRows(t, fixture.db))
	}
}

// failingLane 扮演派送发起那一路的处理方交回某个错误：用来证本路的未决哨兵名单怎样分它。
type failingLane struct{ err error }

func (lane failingLane) Consume(context.Context, eventing.Envelope) error { return lane.err }

// Covers: AT-TF-281（ADR-0230 决定一）——册写不动是等得来的依赖：本路的未决哨兵名单认它，译成派发的未决、整笔回滚重投；装配没交册
// 是缺陷，不在名单里，落发布失败。
func TestThePendingRegisterBeingUnwritableIsRetriedAndItsAbsenceIsADefect(t *testing.T) {
	for name, testCase := range map[string]struct {
		err     error
		retried bool
	}{
		"册写不动":  {fmt.Errorf("%w: pending register down", tfapplication.ErrDeliveryDispatchPendingUnavailable), true},
		"装配没交册": {tfapplication.ErrDeliveryDispatchPendingRegisterNotWired, false},
	} {
		lane, err := dispatch.WithUndecidedSentinels(failingLane{err: testCase.err}, deliveryDispatchOnHandoverUndecidedSentinels...)
		if err != nil {
			t.Fatal(err)
		}
		got := lane.Consume(t.Context(), eventing.Envelope{})
		if errors.Is(got, dispatch.ErrConsumerUndecided) != testCase.retried {
			t.Fatalf("%s：%v，want 译成未决 = %v", name, got, testCase.retried)
		}
	}
}
