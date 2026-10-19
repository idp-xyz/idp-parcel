package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

func cancelBookingCommand(booking string, at time.Time) application.CancelBookingCommand {
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.CancelBookingCommand{TenantID: tenant, Booking: booking, CancelledAt: at}
}

// startCommission 把 commission-1 标为已开始：at 是开始时刻。
func startCommission(t *testing.T, fixture *commissionFixture, at time.Time) {
	t.Helper()
	key := ports.TransportCommissionKey{}
	key.TenantID, _ = domain.NewTenantID("tenant-1")
	key.Commission, _ = domain.NewTransportCommissionReference("commission-1")
	record := fixture.commissions.records[commissionStoreKey(key)]
	basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SEG-1/parcel-1/v1")
	started, err := record.Commission.MarkTransportStarted(basis, at)
	if err != nil {
		t.Fatalf("mark started: %v", err)
	}
	record.Commission = started
	fixture.commissions.records[commissionStoreKey(key)] = record
}

// Covers: `AT-TF-185`、`AT-TF-186`（ADR-0204 决定三）——取消订舱：尚未消耗的取消成立并回显取消时刻，原样再交答原取消不二取；
// 订舱不在册答订舱不在册；所属委托下的运输开始时这次订舱已经提出，答运输已开始（AT-TF-092）；开始之后才提出的订舱是为
// 剩余范围订的，可以取消；委托已取消不挡取消订舱；取消时刻早于申请答未受理。
func TestBookingCancellationOnlyStopsUnconsumedBookings(t *testing.T) {
	t.Run("an unconsumed booking is cancelled once", func(t *testing.T) {
		fixture := newBookedFixture(t)
		result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(time.Hour)))
		if err != nil || result.Outcome() != application.BookingCancelled || result.Outcome().String() != "BOOKING_CANCELLED" {
			t.Fatalf("outcome = %q %v, want BOOKING_CANCELLED", result.Outcome(), err)
		}
		record, present := result.Booking()
		if at, cancelled := record.Booking.Cancelled(); !present || !cancelled || !at.Equal(bookingSentAt.Add(time.Hour)) {
			t.Fatal("取消成立没有回显取消时刻")
		}
		replay, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(2*time.Hour)))
		if err != nil || replay.Outcome() != application.BookingExistingResult {
			t.Fatalf("replay = %q %v, want EXISTING_BOOKING", replay.Outcome(), err)
		}
		stored, _ := replay.Booking()
		if at, _ := stored.Booking.Cancelled(); !at.Equal(bookingSentAt.Add(time.Hour)) {
			t.Fatal("重放改写了原取消时刻")
		}
	})

	t.Run("an absent booking is not found", func(t *testing.T) {
		fixture := newBookedFixture(t)
		result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-9", bookingSentAt.Add(time.Hour)))
		if err != nil || result.Outcome() != application.BookingNotFound {
			t.Fatalf("outcome = %q %v, want BOOKING_NOT_FOUND", result.Outcome(), err)
		}
	})

	t.Run("a booking requested before the transport started cannot be cancelled", func(t *testing.T) {
		fixture := newBookedFixture(t)
		startCommission(t, fixture, bookingSentAt.Add(10*time.Minute))
		result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(time.Hour)))
		if err != nil || result.Outcome() != application.CommissionTransportStarted {
			t.Fatalf("outcome = %q %v, want TRANSPORT_ALREADY_STARTED", result.Outcome(), err)
		}
		record, _, _ := fixture.bookings.FindByKey(context.Background(), ports.BookingKey{TenantID: cancelBookingCommand("", time.Time{}).TenantID, Booking: mustBooking(t, "booking-1")})
		if _, cancelled := record.Booking.Cancelled(); cancelled {
			t.Fatal("已开始那一程的订舱被取消了")
		}
	})

	t.Run("a booking requested after the start is for the remaining scope", func(t *testing.T) {
		fixture := newCommissionFixture(t)
		if _, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); err != nil {
			t.Fatal(err)
		}
		startCommission(t, fixture, bookingSentAt.Add(-10*time.Minute))
		if result, err := fixture.handler.SubmitBooking(context.Background(), submitBookingCommand(t)); err != nil || result.Outcome() != application.BookingSubmitted {
			t.Fatalf("submit booking: %q %v", result.Outcome(), err)
		}
		result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(time.Hour)))
		if err != nil || result.Outcome() != application.BookingCancelled {
			t.Fatalf("outcome = %q %v, want BOOKING_CANCELLED", result.Outcome(), err)
		}
	})

	t.Run("a cancelled commission does not hold its booking", func(t *testing.T) {
		fixture := newBookedFixture(t)
		tenant, _ := domain.NewTenantID("tenant-1")
		if result, err := fixture.handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
			TenantID: tenant, Commission: "commission-1", CancelledAt: bookingSentAt.Add(time.Minute),
		}); err != nil || result.Outcome() != application.CommissionCancelled {
			t.Fatalf("cancel commission: %q %v", result.Outcome(), err)
		}
		result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(time.Hour)))
		if err != nil || result.Outcome() != application.BookingCancelled {
			t.Fatalf("outcome = %q %v, want BOOKING_CANCELLED", result.Outcome(), err)
		}
	})

	t.Run("a cancellation before the request is not accepted", func(t *testing.T) {
		fixture := newBookedFixture(t)
		result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(-time.Minute)))
		if err != nil || result.Outcome() != application.CommissionNotAccepted {
			t.Fatalf("outcome = %q %v, want INPUT_NOT_ACCEPTED", result.Outcome(), err)
		}
	})

	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		fixture := newBookedFixture(t)
		fixture.bookings.findErr = errors.New("down")
		result, err := fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(time.Hour)))
		if err != nil || result.UndecidedReason() != application.BookingStoreUnavailable {
			t.Fatalf("booking store down = %q / %q %v", result.Outcome(), result.UndecidedReason(), err)
		}
		fixture = newBookedFixture(t)
		fixture.commissions.findErr = errors.New("down")
		result, err = fixture.handler.CancelBooking(context.Background(), cancelBookingCommand("booking-1", bookingSentAt.Add(time.Hour)))
		if err != nil || result.UndecidedReason() != application.CommissionStoreUnavailable {
			t.Fatalf("commission store down = %q / %q %v", result.Outcome(), result.UndecidedReason(), err)
		}
	})
}

func mustBooking(t *testing.T, value string) domain.BookingReference {
	t.Helper()
	booking, err := domain.NewBookingReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return booking
}

// Covers: `AT-TF-187`（ADR-0204 决定三）——取消委托与段采用并发：读到时尚未开始，落取消那一刻采用先标了开始，读回答运输已开始，
// 不答未受理、也不把已开始的委托取消掉；另一笔先取消了它，读回答原取消。
func TestCommissionCancellationRacingAStartAnswersFromTheWinner(t *testing.T) {
	for name, tc := range map[string]struct {
		race func(t *testing.T, record ports.TransportCommissionRecord) ports.TransportCommissionRecord
		want application.CommissionOutcome
	}{
		"an adoption starts it first": {
			race: func(t *testing.T, record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
				basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SEG-1/parcel-1/v1")
				started, err := record.Commission.MarkTransportStarted(basis, commissionSentAt.Add(time.Minute))
				if err != nil {
					t.Fatal(err)
				}
				record.Commission = started
				return record
			},
			want: application.CommissionTransportStarted,
		},
		"another cancellation lands first": {
			race: func(t *testing.T, record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
				cancelled, err := record.Commission.Cancel(commissionSentAt.Add(time.Minute))
				if err != nil {
					t.Fatal(err)
				}
				record.Commission = cancelled
				return record
			},
			want: application.CommissionExistingResult,
		},
	} {
		t.Run(name, func(t *testing.T) {
			base := newCommissionFixture(t)
			if _, err := base.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); err != nil {
				t.Fatal(err)
			}
			racing := &adoptionRacingCommissionStore{commissionStoreDouble: base.commissions}
			racing.race = func(record ports.TransportCommissionRecord) ports.TransportCommissionRecord {
				return tc.race(t, record)
			}
			handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
				Commissions: racing,
				Clock:       commissionClock{at: commissionNowAt},
			})
			tenant, _ := domain.NewTenantID("tenant-1")
			result, err := handler.CancelCommission(context.Background(), application.CancelCommissionCommand{
				TenantID: tenant, Commission: "commission-1", CancelledAt: commissionSentAt.Add(time.Hour),
			})
			if err != nil || result.Outcome() != tc.want {
				t.Fatalf("outcome = %q %v, want %q", result.Outcome(), err, tc.want)
			}
		})
	}
}
