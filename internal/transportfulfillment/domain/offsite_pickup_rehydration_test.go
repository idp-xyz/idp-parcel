package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func rehydratedPickupSpec(t *testing.T) domain.RehydrateOffsitePickupSpec {
	t.Helper()
	return domain.RehydrateOffsitePickupSpec{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Object:      mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Task:        mustValue(t, domain.NewPickupTaskReference, "pickup-task-1"),
		Attempt:     mustValue(t, domain.NewAttemptReference, "attempt-1"),
		Place:       mustValue(t, domain.NewPickupPlaceReference, "customer-warehouse-2"),
		Control:     mustValue(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-3-RECHECK"),
		ExecutedBy:  mustValue(t, domain.NewExecutingPartyReference, "courier-2"),
		Version:     mustValue(t, domain.NewPickupResultVersion, "pickup-result/v2"),
		OccurredAt:  pickedUpAt,
		Corrects:    mustValue(t, domain.NewPickupResultVersion, "pickup-result/v1"),
		CorrectedAt: pickedUpAt.Add(36 * time.Hour),
	}
}

// Covers: 票 tf-segment-lifecycle-closure/08 裁决 A 的持久化那一半——版本链两字段写在未导出字段上，
// 没有重建入口，一份更正版本读回来会退化成首登，「新版回指前身」在重启后就断了（同交接侧
// RehydrateTransportHandover 的理由）。重建门只挡坏数据，不重算。
func TestARehydratedPickupRestoresItsVersionChain(t *testing.T) {
	pickup, err := domain.RehydrateOffsitePickup(rehydratedPickupSpec(t))
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if predecessor, present := pickup.Corrects(); !present || predecessor.String() != "pickup-result/v1" {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	if at, present := pickup.CorrectedAt(); !present || !at.Equal(pickedUpAt.Add(36*time.Hour)) {
		t.Fatalf("corrected at = %v present=%v", at, present)
	}
	if pickup.Control().String() != "TRANSPORT-CONTROL/TF-3-RECHECK" || pickup.Version().String() != "pickup-result/v2" {
		t.Fatalf("rehydrated pickup = %+v", pickup)
	}

	t.Run("a first registration has no chain", func(t *testing.T) {
		spec := rehydratedPickupSpec(t)
		spec.Corrects = domain.PickupResultVersion{}
		spec.CorrectedAt = time.Time{}
		pickup, err := domain.RehydrateOffsitePickup(spec)
		if err != nil {
			t.Fatalf("rehydrate first registration: %v", err)
		}
		if _, present := pickup.Corrects(); present {
			t.Fatal("首登版本读回来带了前版引用")
		}
	})

	refusals := map[string]func(*domain.RehydrateOffsitePickupSpec){
		"half a chain: predecessor without corrected at": func(spec *domain.RehydrateOffsitePickupSpec) {
			spec.CorrectedAt = time.Time{}
		},
		"half a chain: corrected at without predecessor": func(spec *domain.RehydrateOffsitePickupSpec) {
			spec.Corrects = domain.PickupResultVersion{}
		},
		"a predecessor pointing at the version itself": func(spec *domain.RehydrateOffsitePickupSpec) {
			spec.Corrects = spec.Version
		},
		"no transport control": func(spec *domain.RehydrateOffsitePickupSpec) {
			spec.Control = domain.TransportControlReference{}
		},
		"no attempt": func(spec *domain.RehydrateOffsitePickupSpec) {
			spec.Attempt = domain.AttemptReference{}
		},
		"no occurrence time": func(spec *domain.RehydrateOffsitePickupSpec) {
			spec.OccurredAt = time.Time{}
		},
	}
	for name, mutate := range refusals {
		t.Run(name, func(t *testing.T) {
			spec := rehydratedPickupSpec(t)
			mutate(&spec)
			if _, err := domain.RehydrateOffsitePickup(spec); !errors.Is(err, domain.ErrInvalidRehydratedPickup) {
				t.Fatalf("err = %v, want ErrInvalidRehydratedPickup", err)
			}
		})
	}
}
