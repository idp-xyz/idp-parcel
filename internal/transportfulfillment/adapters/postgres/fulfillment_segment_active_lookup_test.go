package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 tf-segment-lifecycle-closure/06 裁决 (a)——「对象此刻在哪个段」由段登记册按对象答，不由
// 命令给（交付是关于对象的事实，回传方不知道段）。在场一条答那一个键；已离场不算；他租户不可见。
// 多于一条是库面不一致，读口如实全交回，由编排响亮报错——这里只证读口不吞。
func TestActiveSegmentsOfAnObjectAreLookedUpByObject(t *testing.T) {
	repository, transactor, _ := newSegmentRegistry(t)
	ctx := t.Context()
	tenant := segmentRef(t, domain.NewTenantID, "tenant-1")
	parcel1 := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")

	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-A001", activeMember(t, "parcel-1"), deliveredMember(t, "parcel-2")))

	active, err := repository.FindActiveSegments(ctx, tenant, parcel1)
	if err != nil {
		t.Fatalf("按对象找在场参与：%v", err)
	}
	if len(active) != 1 || active[0].Segment.String() != "SEG-A001" {
		t.Fatalf("active = %+v，want 恰为 SEG-A001", active)
	}

	delivered, err := repository.FindActiveSegments(ctx, tenant, segmentRef(t, domain.NewCarriedObjectReference, "parcel-2"))
	if err != nil {
		t.Fatalf("已离场对象：%v", err)
	}
	if len(delivered) != 0 {
		t.Fatalf("已离场的参与不该算在场：%+v", delivered)
	}

	stranger, err := repository.FindActiveSegments(ctx, segmentRef(t, domain.NewTenantID, "tenant-2"), parcel1)
	if err != nil {
		t.Fatalf("他租户：%v", err)
	}
	if len(stranger) != 0 {
		t.Fatalf("他租户看见了本租户的参与：%+v", stranger)
	}

	// 同一对象再进第二个段（库面不一致的形状）：读口全交回，不挑一个。
	mustSaveSegment(t, transactor, ctx, repository, segmentRecord(t, "SEG-A002", activeMember(t, "parcel-1")))
	both, err := repository.FindActiveSegments(ctx, tenant, parcel1)
	if err != nil {
		t.Fatalf("两段在场：%v", err)
	}
	if len(both) != 2 {
		t.Fatalf("两段在场应全交回，得到 %+v", both)
	}
}
