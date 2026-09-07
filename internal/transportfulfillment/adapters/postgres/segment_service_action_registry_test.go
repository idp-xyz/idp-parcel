package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 证段服务动作那一列（ADR-0114 决定一，迁移 0018）：首登时声明的动作原样往返、
// 未声明落 NULL 装回来仍是未声明、库内 CHECK 挡住封闭集合外的词。列只在段首登那一行写入——加入、结束、关闭
// 三个窄口都不碰它，这一点由那三口的 SQL 不含该列保证，不另立用例。
func TestSegmentServiceActionRoundTripsAndNullMeansUndeclared(t *testing.T) {
	repository, transactor, pool := newSegmentRegistry(t)
	ctx := t.Context()

	delivery := segmentRecord(t, "SEG-SA-1", activeMember(t, "parcel-1"))
	declared, err := delivery.Segment.DeclareServiceAction(domain.SegmentServesFinalDelivery)
	if err != nil {
		t.Fatalf("声明：%v", err)
	}
	delivery.Segment = declared
	mustSaveSegment(t, transactor, ctx, repository, delivery)
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-SA-2", activeMember(t, "parcel-2")))

	found, exists, err := repository.FindByKey(ctx, delivery.Key)
	if err != nil || !exists {
		t.Fatalf("取回派送段：%v exists=%v", err, exists)
	}
	if action, has := found.Segment.ServiceAction(); !has || action != domain.SegmentServesFinalDelivery || !found.Segment.IsDeliverySegment() {
		t.Fatalf("声明没有原样带回：%v %s", has, action)
	}

	plain, exists, err := repository.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-SA-2"))
	if err != nil || !exists {
		t.Fatalf("取回未声明的段：%v exists=%v", err, exists)
	}
	if _, has := plain.Segment.ServiceAction(); has || plain.Segment.IsDeliverySegment() {
		t.Fatal("NULL 装回来成了已声明——未声明是答案，不给默认")
	}

	var stored *string
	if err := pool.QueryRow(ctx,
		`SELECT service_action FROM transport_fulfillment.actual_fulfillment_segment WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-SA-2'`,
	).Scan(&stored); err != nil || stored != nil {
		t.Fatalf("未声明应落 NULL：%v %v", err, stored)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.actual_fulfillment_segment (tenant_id, segment_ref, closed, recorded_at, service_action)
		 VALUES ('t', 'BAD-SA', false, now(), 'LAST_MILE')`,
	); err == nil {
		t.Fatal("封闭集合外的动作词落进去了")
	}
}
