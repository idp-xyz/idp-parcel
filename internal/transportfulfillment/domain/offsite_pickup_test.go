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

func pickupCorrection(t *testing.T, version string, correctedAt time.Time) domain.PickupCorrection {
	t.Helper()
	return domain.PickupCorrection{
		Place:       mustValue(t, domain.NewPickupPlaceReference, "customer-warehouse-2"),
		Control:     mustValue(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-3-RECHECK"),
		ExecutedBy:  mustValue(t, domain.NewExecutingPartyReference, "courier-2"),
		OccurredAt:  pickedUpAt.Add(-2 * time.Hour),
		Version:     mustValue(t, domain.NewPickupResultVersion, version),
		CorrectedAt: correctedAt,
	}
}

// Covers: CONTEXT 生命周期「来源证据被更正 → 保留原段、原参与关系和原判断，形成失效或替代关系」与
// 票 tf-segment-lifecycle-closure/08 裁决 A——更正形成新版本回指被更正版本，原版本一字不动；更正只带
// 「证据说了什么」四格（地点、控制依据、执行方、发生时刻），对象、任务、尝试沿用被更正版本。
func TestAPickupCorrectionFormsANewVersionWithoutOverwriting(t *testing.T) {
	original, err := domain.FormOffsitePickup(pickupSpec(t))
	if err != nil {
		t.Fatalf("form original pickup: %v", err)
	}
	correctedAt := pickedUpAt.Add(36 * time.Hour)

	corrected, err := original.Correct(pickupCorrection(t, "pickup-result/v2", correctedAt))
	if err != nil {
		t.Fatalf("correct pickup: %v", err)
	}
	predecessor, present := corrected.Corrects()
	if !present || predecessor != original.Version() {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	if at, present := corrected.CorrectedAt(); !present || !at.Equal(correctedAt) {
		t.Fatalf("corrected at = %v present=%v", at, present)
	}
	if corrected.Version().String() != "pickup-result/v2" {
		t.Fatalf("version = %q, want v2", corrected.Version())
	}
	if corrected.Place().String() != "customer-warehouse-2" ||
		corrected.Control().String() != "TRANSPORT-CONTROL/TF-3-RECHECK" ||
		corrected.ExecutedBy().String() != "courier-2" ||
		!corrected.OccurredAt().Equal(pickedUpAt.Add(-2*time.Hour)) {
		t.Fatalf("更正给出的四格没有进新版本：%+v", corrected)
	}
	if corrected.TenantID() != original.TenantID() ||
		corrected.Object() != original.Object() ||
		corrected.Task() != original.Task() ||
		corrected.Attempt() != original.Attempt() {
		t.Fatal("对象、任务、尝试必须沿用被更正版本——改了它们就是另一次揽收")
	}

	if original.Control().String() != "TRANSPORT-CONTROL/TF-3" || !original.OccurredAt().Equal(pickedUpAt) {
		t.Fatal("更正改写了原版本")
	}
	if _, present := original.Corrects(); present {
		t.Fatal("原版本被更正动作反向打上了更正标记")
	}
	if _, present := original.CorrectedAt(); present {
		t.Fatal("首登版本不该有更正时刻")
	}

	t.Run("a chain keeps every predecessor", func(t *testing.T) {
		third, err := corrected.Correct(pickupCorrection(t, "pickup-result/v3", correctedAt.Add(time.Hour)))
		if err != nil {
			t.Fatalf("second correction: %v", err)
		}
		if predecessor, _ := third.Corrects(); predecessor != corrected.Version() {
			t.Fatalf("chain broke: corrects = %q, want v2", predecessor)
		}
	})

	t.Run("reusing the corrected version is an overwrite and is refused", func(t *testing.T) {
		correction := pickupCorrection(t, original.Version().String(), correctedAt)
		if _, err := original.Correct(correction); !errors.Is(err, domain.ErrInvalidOffsitePickup) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("a correction is as complete as a first registration", func(t *testing.T) {
		// 四格与两个链字段各自单独缺席，逐一隔离——一次全缺只证明得了「至少查了一件」。控制依据
		// 那一格是分界：更正不能把一次揽收更正成一次失败到访，那是另一种事实，走别的口。
		missing := map[string]func(*domain.PickupCorrection){
			"control":      func(c *domain.PickupCorrection) { c.Control = domain.TransportControlReference{} },
			"place":        func(c *domain.PickupCorrection) { c.Place = domain.PickupPlaceReference{} },
			"executed by":  func(c *domain.PickupCorrection) { c.ExecutedBy = domain.ExecutingPartyReference{} },
			"occurred at":  func(c *domain.PickupCorrection) { c.OccurredAt = time.Time{} },
			"version":      func(c *domain.PickupCorrection) { c.Version = domain.PickupResultVersion{} },
			"corrected at": func(c *domain.PickupCorrection) { c.CorrectedAt = time.Time{} },
		}
		for name, drop := range missing {
			t.Run(name, func(t *testing.T) {
				correction := pickupCorrection(t, "pickup-result/v4", correctedAt)
				drop(&correction)
				if _, err := original.Correct(correction); !errors.Is(err, domain.ErrInvalidOffsitePickup) {
					t.Fatalf("error = %v; 更正的完备性不得低于首登（缺 %s）", err, name)
				}
			})
		}
	})

	t.Run("a zero pickup cannot be corrected into existence", func(t *testing.T) {
		if _, err := (domain.OffsitePickup{}).Correct(pickupCorrection(t, "pickup-result/v2", correctedAt)); !errors.Is(err, domain.ErrInvalidOffsitePickup) {
			t.Fatalf("error = %v; 更正不出无中生有的揽收", err)
		}
	})
}
