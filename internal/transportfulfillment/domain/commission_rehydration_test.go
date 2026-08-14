package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func TestRehydrateTransportCommissionRoundTripsStartAndCancel(t *testing.T) {
	base := domain.RehydrateTransportCommissionSpec{
		TenantID:       mustValue(t, domain.NewTenantID, "tenant-1"),
		Commission:     mustValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Provider:       mustValue(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:      mustValue(t, domain.NewAgreementSnapshotReference, "agreement-snapshot-1"),
		Conditions:     mustValue(t, domain.NewConditionsSnapshotReference, "conditions-snapshot-1"),
		Role:           mustValue(t, domain.NewRoleSnapshotReference, "role-snapshot-1"),
		Responsibility: mustValue(t, domain.NewResponsibilitySnapshotReference, "responsibility-snapshot-1"),
		Members:        []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "parcel-1")},
		SubmittedAt:    commissionSubmittedAt,
	}

	plain, err := domain.RehydrateTransportCommission(base)
	if err != nil {
		t.Fatalf("rehydrate plain: %v", err)
	}
	if _, _, started := plain.TransportStarted(); started {
		t.Fatal("刚提交的委托凭空开始了")
	}

	startedSpec := base
	startedSpec.StartedAt = commissionSubmittedAt.Add(time.Hour)
	startedSpec.StartedBasis = mustValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/v1")
	started, err := domain.RehydrateTransportCommission(startedSpec)
	if err != nil {
		t.Fatalf("rehydrate started: %v", err)
	}
	if _, _, ok := started.TransportStarted(); !ok {
		t.Fatal("开始没有落回")
	}

	both := startedSpec
	both.CancelledAt = commissionSubmittedAt.Add(2 * time.Hour)
	if _, err := domain.RehydrateTransportCommission(both); !errors.Is(err, domain.ErrInvalidRehydratedCommission) {
		t.Fatalf("err = %v; 开始且取消的一行重建成功了", err)
	}
}

func TestRehydrateCarrierAcceptanceRejectsMixedShapes(t *testing.T) {
	accepted, err := domain.RehydrateCarrierAcceptance(domain.RehydrateCarrierAcceptanceSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Acceptance: mustValue(t, domain.NewCarrierAcceptanceReference, "acceptance-1"),
		Booking:    mustValue(t, domain.NewBookingReference, "booking-1"),
		Outcome:    domain.BookingAccepted,
		Quantity:   40,
		DecidedAt:  bookingRequestedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("rehydrate accepted: %v", err)
	}
	if !accepted.Binds() || accepted.AcceptedQuantity() != 40 {
		t.Fatal("接受应答往返变形")
	}

	refused := domain.RehydrateCarrierAcceptanceSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Acceptance: mustValue(t, domain.NewCarrierAcceptanceReference, "acceptance-2"),
		Booking:    mustValue(t, domain.NewBookingReference, "booking-1"),
		Outcome:    domain.BookingRefused,
		Quantity:   40,
		Basis:      mustValue(t, domain.NewAcceptanceBasisReference, "no-capacity"),
		DecidedAt:  bookingRequestedAt.Add(time.Hour),
	}
	if _, err := domain.RehydrateCarrierAcceptance(refused); !errors.Is(err, domain.ErrInvalidRehydratedAcceptance) {
		t.Fatalf("err = %v; 拒绝却带着接受量", err)
	}
}
