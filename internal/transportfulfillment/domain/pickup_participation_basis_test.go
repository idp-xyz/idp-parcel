package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证凭有效收寄进段的参与认得出是哪一版收寄（ADR-0224 决定三）：进段那一版认得出，别的版本、别的对象、凭交接进的都
// 认不出；更正落下的替代参与凭更正版认得出、不再凭原版认。夹具全是合成串，证据只记 S。

// Covers: AT-VE-180 的领域半边——补派生只认这一版收寄让对象进的那一段。
func TestAParticipationTellsWhichPickupItEnteredBy(t *testing.T) {
	pickup, err := domain.FormOffsitePickup(pickupSpec(t))
	if err != nil {
		t.Fatalf("揽收：%v", err)
	}
	segment, err := domain.EstablishSegmentWithPickup(
		mustValue(t, domain.NewFulfillmentSegmentReference, "segment-1"), pickup, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	entered := segment.Participations()[0]
	if !entered.EnteredByPickup(pickup) {
		t.Fatal("进段那一版收寄认不出自己立的参与")
	}

	otherVersion := pickupSpec(t)
	otherVersion.Version = mustValue(t, domain.NewPickupResultVersion, "pickup-result/v9")
	otherObject := pickupSpec(t)
	otherObject.Object = mustValue(t, domain.NewCarriedObjectReference, "parcel-2")
	for name, spec := range map[string]domain.OffsitePickupSpec{"别的版本": otherVersion, "别的对象": otherObject} {
		other, err := domain.FormOffsitePickup(spec)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if entered.EnteredByPickup(other) {
			t.Fatalf("%s的收寄认成了这条参与的入场依据", name)
		}
	}

	corrected, err := pickup.Correct(pickupCorrection(t, "pickup-result/v2", pickedUpAt.Add(time.Hour)))
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	rederived, err := segment.RederiveParticipationWithPickup(corrected)
	if err != nil {
		t.Fatalf("重派生：%v", err)
	}
	var current domain.FulfillmentParticipation
	for _, participation := range rederived.Participations() {
		if !participation.Superseded() {
			current = participation
		}
	}
	if !current.EnteredByPickup(corrected) || current.EnteredByPickup(pickup) {
		t.Fatal("替代参与应凭更正版认得出、凭原版认不出")
	}
}
