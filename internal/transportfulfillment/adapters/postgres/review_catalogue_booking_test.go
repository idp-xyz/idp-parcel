package postgres_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证查阅页订舱与承运答复挡回两格（票 demo-chain-gaps/48、167）：行经写侧适配器真实落库再读回；订舱的状态词
// 按 ADR-0202 补记读出，挡回依据由仓储的码表译回；他租户读不到；读口不写任何行。

type bookingCatalogueStores struct {
	catalogue  *adapter.ReviewCatalogue
	bookings   *adapter.BookingRequests
	answers    *adapter.BookingAnswers
	bounces    *adapter.BookingAnswerBounces
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newBookingCatalogueStores(t *testing.T) bookingCatalogueStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	stores := bookingCatalogueStores{transactor: db.Transactor(), pool: pool}
	if stores.catalogue, err = adapter.NewReviewCatalogue(db); err != nil {
		t.Fatalf("构造查阅目录：%v", err)
	}
	if stores.bookings, err = adapter.NewBookingRequests(db); err != nil {
		t.Fatalf("构造订舱册：%v", err)
	}
	if stores.answers, err = adapter.NewBookingAnswers(db); err != nil {
		t.Fatalf("构造答复册：%v", err)
	}
	if stores.bounces, err = adapter.NewBookingAnswerBounces(db); err != nil {
		t.Fatalf("构造挡回册：%v", err)
	}
	return stores
}

var catalogueBookingAt = time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

// book 落一次申请 120 KG 的订舱，申请时刻按 offset 错开，册上次序因此可断言。
func (stores bookingCatalogueStores) book(t *testing.T, tenant, booking string, offset time.Duration) domain.BookingRequest {
	t.Helper()
	request, err := domain.SubmitBookingRequest(domain.BookingRequestSpec{
		TenantID:    deliveryValue(t, domain.NewTenantID, tenant),
		Booking:     deliveryValue(t, domain.NewBookingReference, booking),
		Commission:  deliveryValue(t, domain.NewTransportCommissionReference, "SYN-COMMISSION-48"),
		Schedule:    deliveryValue(t, domain.NewScheduleReference, "SYN-SCHED-48/2026-10-21"),
		Quantity:    120,
		Unit:        deliveryValue(t, domain.NewQuantityUnitReference, "KG"),
		RequestedAt: catalogueBookingAt.Add(offset),
	})
	if err != nil {
		t.Fatalf("构造订舱：%v", err)
	}
	mustWithinDeliveryTransaction(t, stores.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := stores.bookings.Save(txCtx, ports.BookingRecord{
			Key:           ports.BookingKey{TenantID: request.TenantID(), Booking: request.Booking()},
			ContentDigest: "digest-" + booking,
			Booking:       request,
			RecordedAt:    request.RequestedAt(),
		})
		return err
	})
	return request
}

// answer 落一份答复，答复时刻在申请之后 after。接受带 quantity，其余四值带依据。
func (stores bookingCatalogueStores) answer(
	t *testing.T,
	booking domain.BookingRequest,
	acceptance string,
	outcome domain.CarrierAcceptanceOutcome,
	quantity int64,
	after time.Duration,
) {
	t.Helper()
	basis := domain.AcceptanceBasisReference{}
	if outcome != domain.BookingAccepted {
		basis = deliveryValue(t, domain.NewAcceptanceBasisReference, "basis-"+acceptance)
	}
	decidedAt := booking.RequestedAt().Add(after)
	formed, err := domain.FormCarrierAcceptance(booking, deliveryValue(t, domain.NewCarrierAcceptanceReference, acceptance),
		outcome, quantity, basis, decidedAt)
	if err != nil {
		t.Fatalf("构造答复 %s：%v", outcome, err)
	}
	mustWithinDeliveryTransaction(t, stores.transactor, t.Context(), func(txCtx context.Context) error {
		saved, err := stores.answers.Save(txCtx, ports.BookingAnswerRecord{
			Key:           ports.BookingKey{TenantID: booking.TenantID(), Booking: booking.Booking()},
			ContentDigest: "digest-" + acceptance,
			Acceptance:    formed,
			RecordedAt:    decidedAt,
		})
		if err == nil && saved != ports.BookingAnswerSaved {
			t.Fatalf("答复 %s 没落：%d", acceptance, saved)
		}
		return err
	})
}

// fingerprint 是三本册子此刻的全部行：读口前后各取一次，逐字相同即读口一行没写。
func (stores bookingCatalogueStores) fingerprint(t *testing.T) string {
	t.Helper()
	var digest string
	if err := stores.pool.QueryRow(t.Context(),
		`SELECT concat_ws('|',
		        (SELECT string_agg(r::text, ',' ORDER BY r::text) FROM transport_fulfillment.booking_request r),
		        (SELECT string_agg(a::text, ',' ORDER BY a::text) FROM transport_fulfillment.booking_answer a),
		        (SELECT string_agg(b::text, ',' ORDER BY b::text) FROM transport_fulfillment.booking_answer_bounce b))`,
	).Scan(&digest); err != nil {
		t.Fatalf("取三册指纹：%v", err)
	}
	return digest
}

// Covers: AT-TF-332 真库面（票 demo-chain-gaps/48 完成判据）——一次订舱无答复读作待确认；候补之后读作候补；候补后接受，接受量
// 等于申请量读作已确认、小于读作部分确认；拒绝照答复值；接受之后取消，状态词仍是已确认、取消时刻另列。每行带名下全部答复，候补
// 在前、它的后继在后，当前那一份是定局；新申请的在前。他租户读不到；读口一行不写。
func TestTheBookingRegistryReadsEachBookingsStandingFromItsAnswers(t *testing.T) {
	stores := newBookingCatalogueStores(t)
	stores.book(t, "SYN-TENANT-01", "SYN-BOOKING-PENDING", 6*time.Hour)
	waiting := stores.book(t, "SYN-TENANT-01", "SYN-BOOKING-WAIT", 5*time.Hour)
	full := stores.book(t, "SYN-TENANT-01", "SYN-BOOKING-FULL", 4*time.Hour)
	partial := stores.book(t, "SYN-TENANT-01", "SYN-BOOKING-PART", 3*time.Hour)
	refused := stores.book(t, "SYN-TENANT-01", "SYN-BOOKING-REFUSED", 2*time.Hour)
	cancelled := stores.book(t, "SYN-TENANT-01", "SYN-BOOKING-CANCELLED", time.Hour)
	other := stores.book(t, "SYN-TENANT-02", "SYN-BOOKING-OTHER", 7*time.Hour)

	stores.answer(t, waiting, "SYN-ANSWER-WAIT", domain.BookingWaitlisted, 0, time.Minute)
	stores.answer(t, full, "SYN-ANSWER-FULL-WAIT", domain.BookingWaitlisted, 0, time.Minute)
	stores.answer(t, full, "SYN-ANSWER-FULL", domain.BookingAccepted, 120, 2*time.Minute)
	stores.answer(t, partial, "SYN-ANSWER-PART-WAIT", domain.BookingWaitlisted, 0, time.Minute)
	stores.answer(t, partial, "SYN-ANSWER-PART", domain.BookingAccepted, 80, 2*time.Minute)
	stores.answer(t, refused, "SYN-ANSWER-REFUSED", domain.BookingRefused, 0, time.Minute)
	stores.answer(t, cancelled, "SYN-ANSWER-CANCELLED", domain.BookingAccepted, 120, time.Minute)
	stores.answer(t, other, "SYN-ANSWER-OTHER", domain.BookingAccepted, 120, time.Minute)
	cancelledAt := cancelled.RequestedAt().Add(time.Hour)
	withdrawn, err := cancelled.Cancel(cancelledAt)
	if err != nil {
		t.Fatalf("取消：%v", err)
	}
	mustWithinDeliveryTransaction(t, stores.transactor, t.Context(), func(txCtx context.Context) error {
		replaced, err := stores.bookings.Replace(txCtx, ports.BookingRecord{
			Key:     ports.BookingKey{TenantID: withdrawn.TenantID(), Booking: withdrawn.Booking()},
			Booking: withdrawn,
		})
		if err == nil && !replaced {
			t.Fatal("取消没落")
		}
		return err
	})

	before := stores.fingerprint(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	rows, err := stores.catalogue.ListBookingRequests(t.Context(), tenant, 50)
	if err != nil {
		t.Fatalf("上列订舱册：%v", err)
	}
	type reading struct {
		booking, standing, current, answers string
		cancelled                           bool
	}
	var got []reading
	for _, row := range rows {
		answers := make([]string, 0, len(row.Answers))
		for _, answer := range row.Answers {
			answers = append(answers, answer.Answer+"="+answer.Outcome)
		}
		current := ""
		if row.Current != nil {
			current = row.Current.Answer
		}
		got = append(got, reading{row.Booking, row.Standing, current, strings.Join(answers, ","), row.CancelledAt != nil})
		if row.Quantity != 120 || row.Unit != "KG" || row.Commission != "SYN-COMMISSION-48" || row.Schedule != "SYN-SCHED-48/2026-10-21" {
			t.Fatalf("订舱各格转写走样：%+v", row)
		}
	}
	want := []reading{
		{"SYN-BOOKING-PENDING", "PENDING_CONFIRMATION", "", "", false},
		{"SYN-BOOKING-WAIT", "WAITLISTED", "SYN-ANSWER-WAIT", "SYN-ANSWER-WAIT=WAITLISTED", false},
		{"SYN-BOOKING-FULL", "CONFIRMED", "SYN-ANSWER-FULL", "SYN-ANSWER-FULL-WAIT=WAITLISTED,SYN-ANSWER-FULL=ACCEPTED", false},
		{"SYN-BOOKING-PART", "PARTIALLY_CONFIRMED", "SYN-ANSWER-PART", "SYN-ANSWER-PART-WAIT=WAITLISTED,SYN-ANSWER-PART=ACCEPTED", false},
		{"SYN-BOOKING-REFUSED", "REFUSED", "SYN-ANSWER-REFUSED", "SYN-ANSWER-REFUSED=REFUSED", false},
		{"SYN-BOOKING-CANCELLED", "CONFIRMED", "SYN-ANSWER-CANCELLED", "SYN-ANSWER-CANCELLED=ACCEPTED", true},
	}
	if len(got) != len(want) {
		t.Fatalf("册面 = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("第 %d 行 = %+v, want %+v", index+1, got[index], want[index])
		}
	}
	partialRow := rows[3]
	if partialRow.Current.Quantity != 80 || !partialRow.Current.DecidedAt.Equal(partial.RequestedAt().Add(2*time.Minute)) ||
		partialRow.Answers[0].Basis != "basis-SYN-ANSWER-PART-WAIT" || partialRow.Answers[1].Basis != "" {
		t.Fatalf("部分确认那一行的答复转写走样：%+v", partialRow.Answers)
	}
	if !rows[5].CancelledAt.Equal(cancelledAt) {
		t.Fatalf("取消时刻 = %v, want %v", rows[5].CancelledAt, cancelledAt)
	}

	if first, err := stores.catalogue.ListBookingRequests(t.Context(), tenant, 1); err != nil || len(first) != 1 || first[0].Booking != "SYN-BOOKING-PENDING" {
		t.Fatalf("页大小 1：%+v %v", first, err)
	}
	others, err := stores.catalogue.ListBookingRequests(t.Context(), deliveryValue(t, domain.NewTenantID, "SYN-TENANT-02"), 50)
	if err != nil || len(others) != 1 || others[0].Booking != "SYN-BOOKING-OTHER" || len(others[0].Answers) != 1 {
		t.Fatalf("他租户只看见自己那一行：%+v %v", others, err)
	}
	if empty, err := stores.catalogue.ListBookingRequests(t.Context(), deliveryValue(t, domain.NewTenantID, "SYN-TENANT-03"), 50); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("空册 = %#v %v, want 空列表", empty, err)
	}
	if _, err := stores.catalogue.ListBookingRequests(t.Context(), tenant, 0); err == nil {
		t.Fatal("页大小 0 没被拒")
	}
	if after := stores.fingerprint(t); after != before {
		t.Fatalf("读口写了行：\n前 %s\n后 %s", before, after)
	}
}

// Covers: AT-TF-333 真库面（票 demo-chain-gaps/167 完成判据，ADR-0239 决定二）——一个不是服务方的答复与一个订舱不在册的答复各落一行，
// 查阅两行依据不同（码由仓储译回词）；同一份答复重交只见次数与最近一次时刻变、不多一行；最近一次挡回的在前；按订舱、按答复方
// 筛；他租户读不到；读口一行不写。
func TestTheBounceRegistryTellsTheTwoBasesApartAndCountsResubmissions(t *testing.T) {
	stores := newBookingCatalogueStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	bounce := func(tenant domain.TenantID, source, booking, acceptance string, basis ports.BookingAnswerBounceBasis, at time.Time) {
		t.Helper()
		mustWithinDeliveryTransaction(t, stores.transactor, t.Context(), func(txCtx context.Context) error {
			return stores.bounces.RecordBounce(txCtx, ports.BookingAnswerBounce{
				Key:        ports.BookingKey{TenantID: tenant, Booking: deliveryValue(t, domain.NewBookingReference, booking)},
				Source:     source,
				Acceptance: deliveryValue(t, domain.NewCarrierAcceptanceReference, acceptance),
				Basis:      basis,
				BouncedAt:  at,
			})
		})
	}
	bounce(tenant, "SYN-PARTY-CARRIER-LM-01", "SYN-BOOKING-TRUNK", "SYN-ANSWER-1", ports.BouncedSourceNotServiceProvider, catalogueBookingAt)
	bounce(tenant, "SYN-PARTY-CARRIER-TRUNK-01", "SYN-BOOKING-NOWHERE", "SYN-ANSWER-2", ports.BouncedBookingNotRegistered, catalogueBookingAt.Add(time.Hour))
	bounce(deliveryValue(t, domain.NewTenantID, "SYN-TENANT-02"), "SYN-PARTY-CARRIER-LM-01", "SYN-BOOKING-TRUNK", "SYN-ANSWER-1",
		ports.BouncedBookingNotRegistered, catalogueBookingAt.Add(3*time.Hour))
	list := func(filter ports.BookingAnswerBounceFilter) []string {
		t.Helper()
		rows, err := stores.catalogue.ListBookingAnswerBounces(t.Context(), tenant, filter, 50)
		if err != nil {
			t.Fatalf("上列挡回册：%v", err)
		}
		got := make([]string, 0, len(rows))
		for _, row := range rows {
			got = append(got, strings.Join([]string{row.Source, row.Booking, row.Answer, row.Basis,
				row.FirstBouncedAt.Format(time.RFC3339), row.LastBouncedAt.Format(time.RFC3339), strconv.FormatInt(row.Count, 10)}, "|"))
		}
		return got
	}
	at := func(offset time.Duration) string { return catalogueBookingAt.Add(offset).Format(time.RFC3339) }
	notProvider := func(last time.Duration, count string) string {
		return "SYN-PARTY-CARRIER-LM-01|SYN-BOOKING-TRUNK|SYN-ANSWER-1|SOURCE_NOT_SERVICE_PROVIDER|" + at(0) + "|" + at(last) + "|" + count
	}
	notRegistered := "SYN-PARTY-CARRIER-TRUNK-01|SYN-BOOKING-NOWHERE|SYN-ANSWER-2|BOOKING_NOT_REGISTERED|" + at(time.Hour) + "|" + at(time.Hour) + "|1"
	if got, want := list(ports.BookingAnswerBounceFilter{}), []string{notRegistered, notProvider(0, "1")}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("挡回册 =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	bounce(tenant, "SYN-PARTY-CARRIER-LM-01", "SYN-BOOKING-TRUNK", "SYN-ANSWER-1", ports.BouncedSourceNotServiceProvider, catalogueBookingAt.Add(2*time.Hour))
	before := stores.fingerprint(t)
	if got, want := list(ports.BookingAnswerBounceFilter{}), []string{notProvider(2*time.Hour, "2"), notRegistered}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("重交之后 =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for name, test := range map[string]struct {
		filter ports.BookingAnswerBounceFilter
		want   []string
	}{
		"按订舱":          {ports.BookingAnswerBounceFilter{Booking: "SYN-BOOKING-NOWHERE"}, []string{notRegistered}},
		"按答复方":         {ports.BookingAnswerBounceFilter{Source: "SYN-PARTY-CARRIER-LM-01"}, []string{notProvider(2*time.Hour, "2")}},
		"两维都给":         {ports.BookingAnswerBounceFilter{Booking: "SYN-BOOKING-TRUNK", Source: "SYN-PARTY-CARRIER-TRUNK-01"}, nil},
		"他租户同键那一行不混进来": {ports.BookingAnswerBounceFilter{Booking: "SYN-BOOKING-TRUNK", Source: "SYN-PARTY-CARRIER-LM-01"}, []string{notProvider(2*time.Hour, "2")}},
	} {
		if got := list(test.filter); strings.Join(got, "\n") != strings.Join(test.want, "\n") {
			t.Fatalf("%s =\n%s\nwant\n%s", name, strings.Join(got, "\n"), strings.Join(test.want, "\n"))
		}
	}
	others, err := stores.catalogue.ListBookingAnswerBounces(t.Context(), deliveryValue(t, domain.NewTenantID, "SYN-TENANT-02"), ports.BookingAnswerBounceFilter{}, 50)
	if err != nil || len(others) != 1 || others[0].Basis != "BOOKING_NOT_REGISTERED" {
		t.Fatalf("他租户只看见自己那一行：%+v %v", others, err)
	}
	if page, err := stores.catalogue.ListBookingAnswerBounces(t.Context(), tenant, ports.BookingAnswerBounceFilter{}, 1); err != nil || len(page) != 1 || page[0].Count != 2 {
		t.Fatalf("页大小 1：%+v %v", page, err)
	}
	if _, err := stores.catalogue.ListBookingAnswerBounces(t.Context(), tenant, ports.BookingAnswerBounceFilter{}, 0); err == nil {
		t.Fatal("页大小 0 没被拒")
	}
	if after := stores.fingerprint(t); after != before {
		t.Fatalf("读口写了行：\n前 %s\n后 %s", before, after)
	}
}
