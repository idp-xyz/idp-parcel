package postgres_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证段册与班次册按键那一读把「行读回来了却装不回」与「连不上」分开报（票 demo-chain-gaps/121，
// ADR-0305 的前置条件）：坏行包住各自的 RecordUnreadable 哨兵，出发门禁读数据它另记一个停点、补问不为它再问；连不上不带哨兵。
// 坏行由先拆掉守它的 CHECK、再直写库面造出——那正是「库面被绕过构造门写过」。夹具全为合成登记（S 级）。

// Covers: AT-TF-342 存储半边（段册）——段行的服务动作被写成封闭集外的词，按键读回报错并包住 ErrFulfillmentSegmentRecordUnreadable；
// 读的那一问在取消了的上下文里失败时不带它。
func TestASegmentRowThatCannotBeReadBackIsToldApartFromAnUnreachableRegistry(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()
	record := segmentRecord(t, "SEG-BAD-ROW", activeMember(t, "parcel-1"))
	mustSaveSegment(t, transactor, ctx, repository, record)
	for _, statement := range []string{
		`ALTER TABLE transport_fulfillment.actual_fulfillment_segment DROP CONSTRAINT actual_fulfillment_segment_service_action_closed`,
		`UPDATE transport_fulfillment.actual_fulfillment_segment SET service_action = 'TELEPORT' WHERE segment_ref = 'SEG-BAD-ROW'`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("改坏段行：%v", err)
		}
	}

	if _, _, err := repository.FindByKey(ctx, record.Key); !errors.Is(err, ports.ErrFulfillmentSegmentRecordUnreadable) {
		t.Fatalf("坏段行 err = %v, want ErrFulfillmentSegmentRecordUnreadable", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := repository.FindByKey(canceled, record.Key); err == nil || errors.Is(err, ports.ErrFulfillmentSegmentRecordUnreadable) {
		t.Fatalf("连不上 err = %v，不该带坏行哨兵", err)
	}
}

// Covers: AT-TF-342 存储半边（班次册）——班次行的起讫被写成同一节点，按键读回报错并包住 ErrTransportScheduleRecordUnreadable；
// 读的那一问在取消了的上下文里失败时不带它。
func TestAScheduleRowThatCannotBeReadBackIsToldApartFromAnUnreachableRegistry(t *testing.T) {
	_, schedules, _, transactor, pool := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()
	record := formedSchedule(t, "tenant-a", "schedule-bad-row")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := schedules.Save(txCtx, record)
		return err
	})
	for _, statement := range []string{
		`ALTER TABLE transport_fulfillment.transport_schedule DROP CONSTRAINT transport_schedule_direction_distinct`,
		`UPDATE transport_fulfillment.transport_schedule SET to_node = from_node WHERE schedule_id = 'schedule-bad-row'`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("改坏班次行：%v", err)
		}
	}

	if _, _, err := schedules.FindByKey(ctx, record.Key); !errors.Is(err, ports.ErrTransportScheduleRecordUnreadable) {
		t.Fatalf("坏班次行 err = %v, want ErrTransportScheduleRecordUnreadable", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := schedules.FindByKey(canceled, record.Key); err == nil || errors.Is(err, ports.ErrTransportScheduleRecordUnreadable) {
		t.Fatalf("连不上 err = %v，不该带坏行哨兵", err)
	}
	if _, err := domain.NewScheduleDirection("SYN-CONN-1", "SYN-NODE-A", "SYN-NODE-A"); err == nil {
		t.Fatal("前提不成立：领域认得起讫同一节点的方向，坏行就不坏")
	}
}
