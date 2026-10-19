package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证订舱与取消两扇发生项门（票 demo-chain-gaps/28，ADR-0207 决定一与文末补记）：依据、范围与业务时间取自事实，事实
// 彼此对不上即拒，委托的协议版本随提交记下、随重建读回。夹具全是合成串，证据只记 S。

var (
	bookingFactSubmittedAt = time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	bookingFactRequestedAt = time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	bookingFactAcceptedAt  = time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)
	bookingFactCancelledAt = time.Date(2026, 10, 18, 5, 0, 0, 0, time.UTC)
)

func bookingChargeFact(t *testing.T, commissionRef string, cancelled bool) domain.BookingChargeFact {
	t.Helper()
	tenant := mustValue(t, domain.NewTenantID, "SYN-TENANT-01")
	agreement, err := domain.NewSupplierAgreementVersion("SYN-SUPPLIER-TRUNK-CN-SG-01", "v2")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := domain.DraftTransportCommission(domain.TransportCommissionDraftSpec{
		TenantID:       tenant,
		Commission:     mustValue(t, domain.NewTransportCommissionReference, "SYN-COMMISSION-TRUNK-01"),
		Provider:       mustValue(t, domain.NewServiceProviderReference, "SYN-PARTY-CARRIER-TRUNK-01"),
		Agreement:      agreement,
		Conditions:     mustValue(t, domain.NewConditionsSnapshotReference, "SYN-COND-01"),
		Role:           mustValue(t, domain.NewRoleSnapshotReference, "SYN-ROLE-01"),
		Responsibility: mustValue(t, domain.NewResponsibilitySnapshotReference, "SYN-RESP-01"),
		Members:        []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")},
		SubmittedAt:    bookingFactSubmittedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	commission, err := draft.Submit(mustValue(t, domain.NewAgreementSnapshotReference, "SYN-SUPPLIER-TRUNK-CN-SG-01/v2"),
		mustValue(t, domain.NewProcurementLegalEntityReference, "SYN-LE-01"))
	if err != nil {
		t.Fatal(err)
	}
	booking, err := domain.SubmitBookingRequest(domain.BookingRequestSpec{
		TenantID:    tenant,
		Booking:     mustValue(t, domain.NewBookingReference, "SYN-BOOKING-01"),
		Commission:  mustValue(t, domain.NewTransportCommissionReference, commissionRef),
		Schedule:    mustValue(t, domain.NewScheduleReference, "SYN-SCHED-SZX-SIN-DAILY/2026-10-21"),
		Quantity:    120,
		Unit:        mustValue(t, domain.NewQuantityUnitReference, "KG"),
		RequestedAt: bookingFactRequestedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled {
		if booking, err = booking.Cancel(bookingFactCancelledAt); err != nil {
			t.Fatal(err)
		}
	}
	answer, err := domain.FormCarrierAcceptance(booking, mustValue(t, domain.NewCarrierAcceptanceReference, "SYN-ANSWER-01"),
		domain.BookingAccepted, 120, domain.AcceptanceBasisReference{}, bookingFactAcceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	return domain.BookingChargeFact{Booking: booking, Commission: commission, Answer: answer, Answered: true}
}

// Covers: 订舱与取消的依据、范围与业务时间各取自事实；两种原因同一范围（一次订舱），依据分写前缀、出处不同。
func TestBookingAndCancellationGroundsComeFromTheFacts(t *testing.T) {
	members := []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")}
	fact := bookingChargeFact(t, "SYN-COMMISSION-TRUNK-01", true)

	basis, scope, at, err := domain.BookingOccurrenceGrounds(fact, members)
	if err != nil || basis.String() != "CARRIER-ACCEPTANCE/SYN-BOOKING-01/SYN-ANSWER-01" || scope.String() != "SYN-BOOKING-01" ||
		!at.Equal(bookingFactAcceptedAt) {
		t.Fatalf("订舱 = %q %q %v %v", basis, scope, at, err)
	}
	cancelBasis, cancelScope, cancelAt, err := domain.CancellationOccurrenceGrounds(fact, members)
	if err != nil || cancelBasis.String() != "BOOKING-CANCELLATION/SYN-BOOKING-01" || cancelScope != scope ||
		!cancelAt.Equal(bookingFactCancelledAt) || cancelBasis.Source() == basis.Source() {
		t.Fatalf("取消 = %q %q %v %v", cancelBasis, cancelScope, cancelAt, err)
	}
}

// Covers: 订舱指名的委托不是一同交来的那一份是读口交错了东西，答 ErrInvalidChargeOccurrence，不读成哪一种业务否定格；
// 形成门的租户与事实租户不同同样拒。
func TestBookingFactsThatDoNotBelongTogetherAreRefused(t *testing.T) {
	members := []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")}
	mismatched := bookingChargeFact(t, "SYN-COMMISSION-OTHER-01", false)
	if _, _, _, err := domain.BookingOccurrenceGrounds(mismatched, members); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Fatalf("err = %v, want ErrInvalidChargeOccurrence", err)
	}
	fact := bookingChargeFact(t, "SYN-COMMISSION-TRUNK-01", false)
	spec := occurrenceSpec(t, "TCO-1", "SYN-JRN-01")
	spec.Members = members
	if _, err := domain.ChargeOccurrenceForBooking(spec, fact); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Fatalf("他租户 err = %v, want ErrInvalidChargeOccurrence", err)
	}
}

// Covers: 委托提交时记下登记方指名、核过的那一版协议；重建可缺这两格（迁移 TF 0038 之前的委托），缺时如实答没记下。
func TestACommissionRemembersTheAgreementVersionItWasSubmittedUnder(t *testing.T) {
	fact := bookingChargeFact(t, "SYN-COMMISSION-TRUNK-01", false)
	version, recorded := fact.Commission.AgreementVersion()
	if !recorded || version.Object() != "SYN-SUPPLIER-TRUNK-CN-SG-01" || version.Version() != "v2" {
		t.Fatalf("提交记下的版本 = %v %v", version, recorded)
	}
	legacy, err := domain.RehydrateTransportCommission(domain.RehydrateTransportCommissionSpec{
		TenantID:       fact.Commission.TenantID(),
		Commission:     fact.Commission.Commission(),
		Provider:       fact.Commission.Provider(),
		Agreement:      fact.Commission.Agreement(),
		LegalEntity:    fact.Commission.LegalEntity(),
		Conditions:     fact.Commission.Conditions(),
		Role:           fact.Commission.Role(),
		Responsibility: fact.Commission.Responsibility(),
		Members:        fact.Commission.Members(),
		SubmittedAt:    fact.Commission.SubmittedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, recorded := legacy.AgreementVersion(); recorded {
		t.Fatal("没记下的委托读出了协议版本")
	}
}
