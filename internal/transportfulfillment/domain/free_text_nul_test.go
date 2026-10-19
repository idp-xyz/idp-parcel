package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件钉票 tf-face-algebra/14 的 NUL 一半：不经 newRequiredValue 的三处自由文本——承运商依据的名称素材、
// 承运商首次有效收寄待确认的名称素材、受监管处置的拒因——含 NUL 都在构造门拒（text 列收不下 NUL）。
// 夹具全为合成值（S 级）。

// nulPlacements 是 NUL 在值里的三种位置：首尾与中间。首尾那两格同时证明去空白不会把 NUL 当空白剥掉。
var nulPlacements = map[string]string{
	"开头": "\x00SYN-CARRIER-NAME",
	"中间": "SYN-CARRIER-\x00NAME",
	"末尾": "SYN-CARRIER-NAME\x00",
}

func pinRefusesNUL(t *testing.T, want error, gate func(value string) error) {
	t.Helper()
	for name, value := range nulPlacements {
		err := gate(value)
		if !errors.Is(err, want) || !errors.Is(err, domain.ErrValueContainsNUL) {
			t.Errorf("NUL 在%s：err = %v，期望 %v 且 ErrValueContainsNUL", name, err, want)
		}
	}
	if err := gate("SYN-CARRIER-NAME"); err != nil {
		t.Errorf("不含 NUL：err = %v，期望照常", err)
	}
}

// Covers: 票 tf-face-algebra/14 NUL——承运商依据的名称素材（落 name_material text）。
func TestCarrierEvidenceMaterialWithNULIsRefused(t *testing.T) {
	pinRefusesNUL(t, domain.ErrInvalidCarrierEvidence, func(material string) error {
		_, err := domain.NewCarrierEvidence(domain.CarrierEvidenceSpec{
			Source:     domain.CarrierDirectPickupScan,
			Reference:  mustRef(t, domain.NewCarrierEvidenceReference, "SYN-EVIDENCE-1"),
			OccurredAt: judgmentSegmentEstablishedAt,
			Material:   material,
		})
		return err
	})
}

// Covers: 票 tf-face-algebra/14 NUL——承运商首次有效收寄待确认版本的名称素材（落 claimed_material text），首登与
// 链尾再长一版待确认两条路。
func TestCarrierPickupPendingMaterialWithNULIsRefused(t *testing.T) {
	judgedAt := time.Date(2026, 9, 10, 9, 5, 0, 0, time.UTC)
	t.Run("首登待确认", func(t *testing.T) {
		pinRefusesNUL(t, domain.ErrInvalidCarrierFirstEffectivePickup, func(material string) error {
			_, err := domain.HoldCarrierFirstEffectivePickupPending(pendingCarrierPickupSpec(t, material, judgedAt))
			return err
		})
	})
	t.Run("链尾再长一版待确认", func(t *testing.T) {
		pending, err := domain.HoldCarrierFirstEffectivePickupPending(pendingCarrierPickupSpec(t, "SYN-CARRIER-NAME", judgedAt))
		if err != nil {
			t.Fatalf("首登待确认：%v", err)
		}
		pinRefusesNUL(t, domain.ErrInvalidCarrierFirstEffectivePickup, func(material string) error {
			_, err := pending.HoldPending(domain.CarrierPickupPendingSupersession{
				Version:  mustValue(t, domain.NewCarrierFirstEffectivePickupVersion, "SYN-CFEV-2"),
				Reason:   domain.PickupCarrierIdentityNotRegistered,
				Material: material,
				JudgedAt: judgedAt.Add(time.Minute),
				Bases:    []domain.CarrierPickupBasis{carrierPickupBasis(t, "SYN-EXTF-1", "SYN-EXTV-2")},
			})
			return err
		})
	})
}

// Covers: 票 tf-face-algebra/14 NUL——受监管处置的拒因（落 decline_basis text），拒接与部分承接两格；部分承接带齐
// 移动授权，NUL 答的是未受理而不是「待补授权」。
func TestRegulatoryDispositionDeclineBasisWithNULIsRefused(t *testing.T) {
	t.Run("拒接", func(t *testing.T) {
		pinRefusesNUL(t, domain.ErrInvalidRegulatoryDisposition, func(basis string) error {
			_, err := domain.FormRegulatoryTransportDisposition(declinedDispositionSpec(t, basis))
			return err
		})
	})
	t.Run("部分承接", func(t *testing.T) {
		pinRefusesNUL(t, domain.ErrInvalidRegulatoryDisposition, func(basis string) error {
			spec := declinedDispositionSpec(t, basis)
			spec.Kind = domain.DispositionPartiallyAccepted
			spec.AcceptedObjects = []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-1")}
			spec.MovementAuthority = mustValue(t, domain.NewMovementAuthorityReference, "SYN-AUTHORITY-1")
			_, err := domain.FormRegulatoryTransportDisposition(spec)
			return err
		})
	})
}
