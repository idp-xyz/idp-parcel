package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// projection_test.go 的构造器钉死单一包裹;列表读面要跨包裹取证,这里另备带包裹
// 参数的构造器,不动既有测试的形状。

func listEntry(t *testing.T, parcel, factRef string) domain.MilestoneClassification {
	t.Helper()
	fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceNodeOperations,
		Parcel:      projectionValue(t, domain.NewTrackedParcelReference, parcel),
		Fact:        projectionValue(t, domain.NewSourceFactReference, factRef),
		Kind:        projectionValue(t, domain.NewSourceFactKind, "node-intake"),
		Version:     projectionValue(t, domain.NewSourceFactVersion, "v1"),
		OccurredAt:  projectionBaseAt,
		EffectiveAt: projectionBaseAt.Add(time.Hour),
		ReceivedAt:  projectionBaseAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造事实：%v", err)
	}
	classification, err := domain.ClassifyMilestone(fact,
		projectionValue(t, domain.NewMilestoneReference, "PICKED_UP"),
		projectionValue(t, domain.NewMappingVersionReference, "milestone-map/v1"))
	if err != nil {
		t.Fatalf("归类：%v", err)
	}
	return classification
}

func listProjection(t *testing.T, parcel, version string, derivedAt time.Time) domain.TrackingProjection {
	t.Helper()
	projection, err := domain.DeriveTrackingProjection(
		projectionValue(t, domain.NewProjectionVersionID, version),
		projectionValue(t, domain.NewTrackedParcelReference, parcel),
		[]domain.MilestoneClassification{listEntry(t, parcel, "scan/origin/"+version)},
		derivedAt,
	)
	if err != nil {
		t.Fatalf("派生投影：%v", err)
	}
	return projection
}

// Covers: ADR-0076 决定五 — 列表读面按租户列当前投影:每包裹只出当前版(历史版留给
// FindByVersion)、新派生在前、跨租户不可见、空租户答空列表。
func TestListCurrentReturnsPerParcelCurrentVersions(t *testing.T) {
	fixture := newProjectionFixture(t)
	ctx := t.Context()
	tenantA := projectionTenant(t, "tenant-list-a")
	tenantB := projectionTenant(t, "tenant-list-b")

	first := listProjection(t, "parcel-1", "list-projection-1", projectionBaseAt.Add(time.Hour))
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenantA, first)
	})
	rederived, err := first.Rederive(
		projectionValue(t, domain.NewProjectionVersionID, "list-projection-2"),
		[]domain.MilestoneClassification{listEntry(t, "parcel-1", "scan/corrected")},
		projectionBaseAt.Add(4*time.Hour),
	)
	if err != nil {
		t.Fatalf("重派生：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenantA, rederived)
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenantA,
			listProjection(t, "parcel-2", "list-projection-3", projectionBaseAt.Add(2*time.Hour)))
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenantB,
			listProjection(t, "parcel-9", "list-projection-9", projectionBaseAt.Add(5*time.Hour)))
	})

	listed, err := fixture.projections.ListCurrent(ctx, tenantA, 10)
	if err != nil {
		t.Fatalf("ListCurrent：%v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("行数 = %d, want 2（每包裹一行且不含他租户）", len(listed))
	}
	// 新派生在前:parcel-1 的当前版派生于 t+4h,parcel-2 派生于 t+2h。
	if listed[0].Version().String() != "list-projection-2" || listed[0].Parcel().String() != "parcel-1" {
		t.Fatalf("首行 = %s/%s, want parcel-1 的当前版 list-projection-2",
			listed[0].Parcel(), listed[0].Version())
	}
	if prior, present := listed[0].PriorVersion(); !present || prior.String() != "list-projection-1" {
		t.Fatalf("当前版指回 = %s present=%v", prior, present)
	}
	if listed[1].Version().String() != "list-projection-3" || listed[1].Parcel().String() != "parcel-2" {
		t.Fatalf("次行 = %s/%s", listed[1].Parcel(), listed[1].Version())
	}
	for _, projection := range listed {
		if projection.Version().String() == "list-projection-1" {
			t.Fatal("被替代的历史版进了当前列表")
		}
	}

	// limit 截断:只回最新的一行。
	limited, err := fixture.projections.ListCurrent(ctx, tenantA, 1)
	if err != nil || len(limited) != 1 || limited[0].Version().String() != "list-projection-2" {
		t.Fatalf("limit=1：err=%v got=%v", err, limited)
	}

	// 无投影的租户答空列表:对租户内已授权的运营查阅这是如实业务答案,不是错误。
	empty, err := fixture.projections.ListCurrent(ctx, projectionTenant(t, "tenant-list-none"), 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("空租户：err=%v rows=%d", err, len(empty))
	}
}

// Covers: OperationsProjectionRead 的 limit 约定 — 非正 limit 是调用方编程错误。
func TestListCurrentRejectsNonPositiveLimit(t *testing.T) {
	fixture := newProjectionFixture(t)
	if _, err := fixture.projections.ListCurrent(t.Context(),
		projectionTenant(t, "tenant-list-a"), 0); err == nil {
		t.Fatal("limit=0 被接受了")
	}
}
