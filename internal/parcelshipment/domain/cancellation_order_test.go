package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: ADR-0254 决定一、决定二第 2、3 格的判法本身——取消已提交之后才到的边界事实只按两个业务
// 时间比：严格晚于取消的业务时间是取消在先，不晚于（含相等）是事实冲突；冲突带回两侧各自的那一代
// 与业务时间。任一侧缺时间或缺指名立不起，不替调用方补。
func TestTheCancellationOrderIsJudgedByTheTwoBusinessTimes(t *testing.T) {
	cancellation, err := domain.DecideParcelCancellation(cancellationSpec(t), domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide cancellation: %v", err)
	}
	boundaryAt := func(at time.Time) domain.CancellationBoundaryFact {
		return domain.CancellationBoundaryFact{
			Kind:       domain.NetworkIntakeBoundary,
			Reference:  mustValue(t, domain.NewCancellationBoundaryReference, "NODE_INTAKE/v1"),
			OccurredAt: at,
		}
	}

	if _, conflicted, err := domain.JudgeCancellationOrder(cancellation, boundaryAt(cancellationAt.Add(time.Nanosecond))); err != nil || conflicted {
		t.Fatalf("边界事实晚于取消：conflicted = %v err = %v，want 取消在先", conflicted, err)
	}
	for name, at := range map[string]time.Time{
		"earlier":      cancellationAt.Add(-time.Hour),
		"same instant": cancellationAt,
	} {
		conflict, conflicted, err := domain.JudgeCancellationOrder(cancellation, boundaryAt(at))
		if err != nil || !conflicted {
			t.Fatalf("%s：conflicted = %v err = %v，want 事实冲突", name, conflicted, err)
		}
		if conflict.Parcel().String() != "parcel-1" || conflict.Cancellation().String() != "cancel-1" ||
			!conflict.CancelledAt().Equal(cancellationAt) || !conflict.Boundary().OccurredAt.Equal(at) ||
			conflict.Boundary().Reference.String() != "NODE_INTAKE/v1" {
			t.Fatalf("%s：conflict = %+v", name, conflict)
		}
	}

	missing := boundaryAt(time.Time{})
	if _, _, err := domain.JudgeCancellationOrder(cancellation, missing); !errors.Is(err, domain.ErrInvalidCancellationBoundaryFact) {
		t.Fatalf("边界事实缺发生时间：err = %v", err)
	}
	unnamed := boundaryAt(cancellationAt)
	unnamed.Kind = domain.CancellationBoundaryKindInvalid
	if _, _, err := domain.JudgeCancellationOrder(cancellation, unnamed); !errors.Is(err, domain.ErrInvalidCancellationBoundaryFact) {
		t.Fatalf("边界事实缺种类：err = %v", err)
	}
	if _, _, err := domain.JudgeCancellationOrder(domain.ParcelCancellation{}, boundaryAt(cancellationAt)); !errors.Is(err, domain.ErrInvalidCancellationBoundaryFact) {
		t.Fatalf("取消缺席：err = %v", err)
	}
}
