package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var pickedUpAt = time.Date(2026, 8, 9, 8, 15, 0, 0, time.UTC)

func mustValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func pickupSpec(t *testing.T) domain.OffsitePickupSpec {
	t.Helper()
	return domain.OffsitePickupSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Object:     mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Task:       mustValue(t, domain.NewPickupTaskReference, "pickup-task-1"),
		Attempt:    mustValue(t, domain.NewAttemptReference, "attempt-1"),
		Place:      mustValue(t, domain.NewPickupPlaceReference, "customer-warehouse-1"),
		Control:    mustValue(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-3"),
		ExecutedBy: mustValue(t, domain.NewExecutingPartyReference, "courier-1"),
		Version:    mustValue(t, domain.NewPickupResultVersion, "pickup-result/v1"),
		OccurredAt: pickedUpAt,
	}
}

// Covers: transport-fulfillment CONTEXT「场外揽收只有在明确载运对象形成有效收寄或权威
// 交接并由运输方取得控制时，才建立履约参与关系」——对象级揽收结果锚在具体一次履约尝试
// 上（改约重派新尝试不覆盖），实际接货时间随结果保全。
func TestAnOffsitePickupAnchorsObjectAttemptAndControl(t *testing.T) {
	pickup, err := domain.FormOffsitePickup(pickupSpec(t))
	if err != nil {
		t.Fatalf("form offsite pickup: %v", err)
	}
	if pickup.Object().String() != "parcel-1" || pickup.Attempt().String() != "attempt-1" {
		t.Fatalf("pickup = %#v; 对象与尝试没有随结果锚定", pickup)
	}
	if !pickup.OccurredAt().Equal(pickedUpAt) {
		t.Fatalf("occurred at = %s", pickup.OccurredAt())
	}
	if pickup.Control().String() != "TRANSPORT-CONTROL/TF-3" {
		t.Fatal("控制依据没有随结果保全")
	}
}

// Covers: CONTEXT「客户不在、货物未备好、包装不合格或其他失败结果不制造实际履约段」——
// 分界是控制依据：没有它的到场立不成揽收；任务、尝试、地点、执行方、版本与时间同为
// 必备件（任务不等于到场，尝试才是）。
func TestAFailedVisitCannotBecomeAPickup(t *testing.T) {
	cases := map[string]func(domain.OffsitePickupSpec) domain.OffsitePickupSpec{
		"no transport control": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.Control = domain.TransportControlReference{}
			return spec
		},
		"no attempt": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.Attempt = domain.AttemptReference{}
			return spec
		},
		"no task": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.Task = domain.PickupTaskReference{}
			return spec
		},
		"no carried object": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.Object = domain.CarriedObjectReference{}
			return spec
		},
		"no place": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.Place = domain.PickupPlaceReference{}
			return spec
		},
		"no executing party": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.ExecutedBy = domain.ExecutingPartyReference{}
			return spec
		},
		"no result version": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.Version = domain.PickupResultVersion{}
			return spec
		},
		"no occurrence time": func(spec domain.OffsitePickupSpec) domain.OffsitePickupSpec {
			spec.OccurredAt = time.Time{}
			return spec
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.FormOffsitePickup(mutate(pickupSpec(t))); !errors.Is(err, domain.ErrInvalidOffsitePickup) {
				t.Fatalf("err = %v, want ErrInvalidOffsitePickup", err)
			}
		})
	}
}
