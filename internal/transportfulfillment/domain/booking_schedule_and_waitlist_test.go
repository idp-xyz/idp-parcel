package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: ADR-0202 决定五「订舱必须引用一个具体班次」的构造面——不指名班次的订舱建不成；班次随订舱读出。
func TestABookingNamesAConcreteSchedule(t *testing.T) {
	spec := domain.BookingRequestSpec{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Booking:     mustValue(t, domain.NewBookingReference, "booking-1"),
		Commission:  mustValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Quantity:    100,
		Unit:        mustValue(t, domain.NewQuantityUnitReference, "kg"),
		RequestedAt: bookingRequestedAt,
	}
	if _, err := domain.SubmitBookingRequest(spec); !errors.Is(err, domain.ErrInvalidBookingRequest) {
		t.Fatalf("不指名班次的订舱：err = %v, want ErrInvalidBookingRequest", err)
	}
	booking := submittedBooking(t)
	if booking.Schedule().String() != "schedule-1" {
		t.Fatalf("班次 = %q", booking.Schedule())
	}
}

func answerOf(t *testing.T, booking domain.BookingRequest, acceptance string, outcome domain.CarrierAcceptanceOutcome, at time.Time) domain.CarrierAcceptance {
	t.Helper()
	var quantity int64
	basis := domain.AcceptanceBasisReference{}
	if outcome == domain.BookingAccepted {
		quantity = 40
	} else {
		basis = mustValue(t, domain.NewAcceptanceBasisReference, "basis-"+acceptance)
	}
	answer, err := domain.FormCarrierAcceptance(booking, mustValue(t, domain.NewCarrierAcceptanceReference, acceptance), outcome, quantity, basis, at)
	if err != nil {
		t.Fatalf("form %s: %v", outcome, err)
	}
	return answer
}

// Covers: `AT-TF-151` 领域面——候补是五值里唯一不定局的一值：它之后可以接一份定局答复（不早于候补、另一个答复标识），
// 定局之后不再有后继，候补之后也不再接候补；后继不改写候补本身。
func TestOnlyAWaitlistAdmitsASettlingSuccessor(t *testing.T) {
	booking := submittedBooking(t)
	at := bookingRequestedAt.Add(time.Hour)
	waitlisted := answerOf(t, booking, "answer-waitlist", domain.BookingWaitlisted, at)
	if waitlisted.Settles() || waitlisted.Binds() || waitlisted.AcceptedQuantity() != 0 {
		t.Fatalf("候补被读成了定局或成约：%+v", waitlisted)
	}

	for name, outcome := range map[string]domain.CarrierAcceptanceOutcome{
		"accepted": domain.BookingAccepted, "refused": domain.BookingRefused,
		"expired": domain.BookingExpired, "withdrawn": domain.BookingWithdrawn,
	} {
		settling := answerOf(t, booking, "answer-"+name, outcome, at.Add(time.Hour))
		if !settling.Settles() {
			t.Fatalf("%s 不是定局", name)
		}
		if !waitlisted.AdmitsSuccessor(settling) {
			t.Fatalf("候补之后接不上 %s", name)
		}
		if settling.AdmitsSuccessor(answerOf(t, booking, "answer-after-"+name, domain.BookingRefused, at.Add(2*time.Hour))) {
			t.Fatalf("%s 之后还能再接一份答复——定局被改写了", name)
		}
	}

	refusals := map[string]domain.CarrierAcceptance{
		"再候补一次":      answerOf(t, booking, "answer-waitlist-2", domain.BookingWaitlisted, at.Add(time.Hour)),
		"早于候补的定局":    answerOf(t, booking, "answer-early", domain.BookingAccepted, at.Add(-time.Minute)),
		"与候补同一个答复标识": answerOf(t, booking, "answer-waitlist", domain.BookingAccepted, at.Add(time.Hour)),
	}
	other := submittedBooking(t)
	otherSpec := domain.BookingRequestSpec{
		TenantID:    other.TenantID(),
		Booking:     mustValue(t, domain.NewBookingReference, "booking-2"),
		Commission:  other.Commission(),
		Schedule:    other.Schedule(),
		Quantity:    100,
		Unit:        mustValue(t, domain.NewQuantityUnitReference, "kg"),
		RequestedAt: bookingRequestedAt,
	}
	otherBooking, err := domain.SubmitBookingRequest(otherSpec)
	if err != nil {
		t.Fatal(err)
	}
	refusals["别的订舱上的答复"] = answerOf(t, otherBooking, "answer-other", domain.BookingAccepted, at.Add(time.Hour))
	for name, next := range refusals {
		if waitlisted.AdmitsSuccessor(next) {
			t.Fatalf("%s 被当成了候补的后继", name)
		}
	}
}

// Covers: `AT-TF-151` 领域面「当前答复是定局」，票 demo-chain-gaps/177 完成判据——当前那一份只由 CurrentCarrierAcceptance 选：
// 有定局取定局，否则取候补，与答复的排列次序无关；没有答复交 -1。名下答复凑不成一段答复史——两份定局、两份候补、定局早于
// 候补、定局在别的订舱上、混进未成形的答复——答 ErrBookingAnswersInconsistent，不挑一份。
func TestTheCurrentAnswerIsTheSettlingOneElseTheWaitlist(t *testing.T) {
	booking := submittedBooking(t)
	at := bookingRequestedAt.Add(time.Hour)
	waitlisted := answerOf(t, booking, "answer-waitlist", domain.BookingWaitlisted, at)
	accepted := answerOf(t, booking, "answer-accepted", domain.BookingAccepted, at.Add(time.Hour))
	refused := answerOf(t, booking, "answer-refused", domain.BookingRefused, at.Add(time.Hour))
	sameTime := answerOf(t, booking, "answer-same-time", domain.BookingExpired, at)
	otherSpec := domain.BookingRequestSpec{
		TenantID:    booking.TenantID(),
		Booking:     mustValue(t, domain.NewBookingReference, "booking-2"),
		Commission:  booking.Commission(),
		Schedule:    booking.Schedule(),
		Quantity:    100,
		Unit:        mustValue(t, domain.NewQuantityUnitReference, "kg"),
		RequestedAt: bookingRequestedAt,
	}
	otherBooking, err := domain.SubmitBookingRequest(otherSpec)
	if err != nil {
		t.Fatal(err)
	}

	for name, testCase := range map[string]struct {
		answers []domain.CarrierAcceptance
		want    string
	}{
		"无答复":            {nil, ""},
		"只有候补":           {[]domain.CarrierAcceptance{waitlisted}, "answer-waitlist"},
		"候补加定局":          {[]domain.CarrierAcceptance{waitlisted, accepted}, "answer-accepted"},
		"定局排在候补之前":       {[]domain.CarrierAcceptance{refused, waitlisted}, "answer-refused"},
		"只有定局（接受）":       {[]domain.CarrierAcceptance{accepted}, "answer-accepted"},
		"只有定局（拒绝）":       {[]domain.CarrierAcceptance{refused}, "answer-refused"},
		"与候补同刻的定局":       {[]domain.CarrierAcceptance{waitlisted, sameTime}, "answer-same-time"},
		"与候补同刻的定局排在候补之前": {[]domain.CarrierAcceptance{sameTime, waitlisted}, "answer-same-time"},
		"只有定局（撤回）":       {[]domain.CarrierAcceptance{answerOf(t, booking, "answer-withdrawn", domain.BookingWithdrawn, at)}, "answer-withdrawn"},
		"只有定局（失效）":       {[]domain.CarrierAcceptance{answerOf(t, booking, "answer-expired", domain.BookingExpired, at)}, "answer-expired"},
	} {
		t.Run(name, func(t *testing.T) {
			current, err := domain.CurrentCarrierAcceptance(testCase.answers)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			got := ""
			if current >= 0 {
				got = testCase.answers[current].Acceptance().String()
			}
			if got != testCase.want {
				t.Fatalf("当前那一份 = %q（位置 %d），want %q", got, current, testCase.want)
			}
		})
	}

	for name, answers := range map[string][]domain.CarrierAcceptance{
		"两份定局":     {accepted, refused},
		"两份候补":     {waitlisted, answerOf(t, booking, "answer-waitlist-2", domain.BookingWaitlisted, at.Add(time.Hour))},
		"定局早于候补":   {answerOf(t, booking, "answer-early", domain.BookingAccepted, at.Add(-time.Minute)), waitlisted},
		"定局在别的订舱上": {waitlisted, answerOf(t, otherBooking, "answer-other", domain.BookingAccepted, at.Add(time.Hour))},
		"混进未成形的答复": {waitlisted, {}},
	} {
		t.Run(name, func(t *testing.T) {
			current, err := domain.CurrentCarrierAcceptance(answers)
			if !errors.Is(err, domain.ErrBookingAnswersInconsistent) || current != -1 {
				t.Fatalf("位置 %d err = %v, want -1 与 ErrBookingAnswersInconsistent", current, err)
			}
		})
	}
}
