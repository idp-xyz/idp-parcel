package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var cancellationAt = time.Date(2026, 8, 9, 6, 0, 0, 0, time.UTC)

func cancellationSpec(t *testing.T) domain.ParcelCancellationSpec {
	t.Helper()
	return domain.ParcelCancellationSpec{
		ID:          mustValue(t, domain.NewParcelCancellationID, "cancel-1"),
		Parcel:      mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-1"),
		Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/PC-17"),
		Reason:      mustValue(t, domain.NewCancellationReasonReference, "CUSTOMER_CHANGED_MIND"),
		RequestedAt: cancellationAt,
	}
}

// Covers: `AT-PS-077`「已接受包裹在有效网络收寄前被合法请求取消——形成包裹取消终局，
// 保留身份与接受基线」的领域面——收寄不在场时取消成立且逐件挂在明确包裹上；授权、
// 请求方与原因缺一立不成（没有授权依据的取消与运营误操作分不开）。
func TestACancellationFormsOnlyBeforeTheIntakeBoundary(t *testing.T) {
	cancellation, err := domain.DecideParcelCancellation(cancellationSpec(t), domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide parcel cancellation: %v", err)
	}
	if cancellation.Parcel().String() != "parcel-1" {
		t.Fatalf("parcel = %s; 取消权按包裹判断", cancellation.Parcel())
	}
	if !cancellation.RequestedAt().Equal(cancellationAt) {
		t.Fatalf("requested at = %s", cancellation.RequestedAt())
	}

	missingAuthority := cancellationSpec(t)
	missingAuthority.Authority = domain.CancellationAuthorityReference{}
	if _, err := domain.DecideParcelCancellation(missingAuthority, domain.CurrentIntakeFact{}); !errors.Is(err, domain.ErrInvalidParcelCancellation) {
		t.Fatalf("err = %v; 没有授权依据的取消被收下了", err)
	}
}

// Covers: `AT-PS-079`「收寄与取消并发，收寄先合法成立——不形成取消，进入收寄后处置
// 判断」与 `AT-PS-082` 的领域面——有效网络收寄在场即拒绝形成取消，无论收寄发生在请求
// 前后：本函数在决定提交边界执行，提交前收寄成立就是收寄赢；错误是独立哨兵，调用方
// 据此走处置路而不是重试。
func TestACrossedIntakeBoundaryRefusesCancellation(t *testing.T) {
	for name, occurredAt := range map[string]time.Time{
		"intake before the request": cancellationAt.Add(-time.Hour),
		"intake after the request":  cancellationAt.Add(time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := domain.DecideParcelCancellation(cancellationSpec(t), domain.CurrentIntakeFact{
				Present:    true,
				OccurredAt: occurredAt,
			})
			if !errors.Is(err, domain.ErrIntakeBoundaryCrossed) {
				t.Fatalf("err = %v, want ErrIntakeBoundaryCrossed", err)
			}
		})
	}
}
