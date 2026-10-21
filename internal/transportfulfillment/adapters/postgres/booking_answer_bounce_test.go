package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: ADR-0239 决定三的读口——订舱连同所属委托的服务提供方一条语句读回：不在册（含只登在他租户名下）答不在；在册带回
// 服务方；委托行读不回时照样交回订舱、标出委托不在，由编排认数据违约，不落成「订舱不在册」。
func TestABookingIsReadBackWithItsCommissionsProviderInOneStatement(t *testing.T) {
	commissions, bookings, _, transactor, pool := newCommissionStores(t)
	ctx := t.Context()
	commission := submittedCommission(t, "tenant-a", "commission-1")
	booking := submittedBookingRecord(t, "tenant-a", "booking-1")
	orphanBooking := submittedBookingRecord(t, "tenant-a", "booking-orphan")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := commissions.Save(txCtx, commission); err != nil {
			return err
		}
		if _, err := bookings.Save(txCtx, booking); err != nil {
			return err
		}
		_, err := bookings.Save(txCtx, orphanBooking)
		return err
	})
	if _, err := pool.Exec(ctx,
		`UPDATE transport_fulfillment.booking_request SET commission_id = 'commission-gone' WHERE booking_id = 'booking-orphan'`); err != nil {
		t.Fatalf("让订舱指向读不回的委托：%v", err)
	}
	key := func(tenant, booking string) ports.BookingKey {
		return ports.BookingKey{TenantID: deliveryValue(t, domain.NewTenantID, tenant), Booking: deliveryValue(t, domain.NewBookingReference, booking)}
	}

	found, present, err := bookings.FindBookingWithProvider(ctx, key("tenant-a", "booking-1"))
	if err != nil || !present || !found.CommissionRegistered || found.Provider.String() != "partner-1" {
		t.Fatalf("在册订舱 = %+v present=%v err=%v", found, present, err)
	}
	plain, _, err := bookings.FindByKey(ctx, key("tenant-a", "booking-1"))
	if err != nil || found.Booking.ContentDigest != plain.ContentDigest || found.Booking.Booking.Schedule() != plain.Booking.Schedule() ||
		!found.Booking.RecordedAt.Equal(plain.RecordedAt) {
		t.Fatalf("连读交回的订舱与单读不同：%+v vs %+v（%v）", found.Booking, plain, err)
	}

	for name, missing := range map[string]ports.BookingKey{
		"从未登记":    key("tenant-a", "booking-9"),
		"只登在别的租户": key("tenant-b", "booking-1"),
	} {
		if _, present, err := bookings.FindBookingWithProvider(ctx, missing); err != nil || present {
			t.Fatalf("%s：present=%v err=%v", name, present, err)
		}
	}

	orphan, present, err := bookings.FindBookingWithProvider(ctx, key("tenant-a", "booking-orphan"))
	if err != nil || !present || orphan.CommissionRegistered || orphan.Provider.String() != "" {
		t.Fatalf("委托读不回的订舱 = %+v present=%v err=%v", orphan, present, err)
	}
}

// Covers: `AT-TF-327`（存取面）、ADR-0239 决定二的挡回册——一次挡回立一行；同键再来只加次数、推后最近一次时刻，首次时刻不动；依据不同另起一行；
// 只在事务里写，事务回滚不留痕；封闭集外的依据与空来源在写之前就拒。
func TestABouncedAnswerIsCountedOnItsOwnRowPerBasis(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	bounces, err := adapter.NewBookingAnswerBounces(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	bounce := ports.BookingAnswerBounce{
		Key: ports.BookingKey{
			TenantID: deliveryValue(t, domain.NewTenantID, "tenant-a"),
			Booking:  deliveryValue(t, domain.NewBookingReference, "booking-1"),
		},
		Source:     "partner-2",
		Acceptance: deliveryValue(t, domain.NewCarrierAcceptanceReference, "answer-1"),
		Basis:      ports.BouncedSourceNotServiceProvider,
		BouncedAt:  at,
	}
	record := func(bounce ports.BookingAnswerBounce) {
		t.Helper()
		mustWithinDeliveryTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
			return bounces.RecordBounce(txCtx, bounce)
		})
	}

	record(bounce)
	again := bounce
	again.BouncedAt = at.Add(time.Hour)
	record(again)
	otherBasis := bounce
	otherBasis.Basis = ports.BouncedBookingNotRegistered
	otherBasis.BouncedAt = at.Add(2 * time.Hour)
	record(otherBasis)

	type row struct {
		basis       string
		first, last time.Time
		count       int64
	}
	rows, err := pool.Query(ctx,
		`SELECT basis, first_bounced_at, last_bounced_at, bounce_count
		   FROM transport_fulfillment.booking_answer_bounce
		  WHERE tenant_id = 'tenant-a' AND source_ref = 'partner-2' AND booking_id = 'booking-1' AND acceptance_id = 'answer-1'
		  ORDER BY basis`)
	if err != nil {
		t.Fatal(err)
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.basis, &r.first, &r.last, &r.count); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	rows.Close()
	if len(got) != 2 ||
		got[0].basis != "BOOKING_NOT_REGISTERED" || got[0].count != 1 ||
		got[1].basis != "SOURCE_NOT_SERVICE_PROVIDER" || got[1].count != 2 ||
		!got[1].first.Equal(at) || !got[1].last.Equal(at.Add(time.Hour)) {
		t.Fatalf("挡回册 = %+v", got)
	}

	if err := bounces.RecordBounce(ctx, bounce); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外写挡回册：%v，want ErrTransactionRequired", err)
	}
	errRollback := errors.New("回滚")
	stray := bounce
	stray.Acceptance = deliveryValue(t, domain.NewCarrierAcceptanceReference, "answer-rolled-back")
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := bounces.RecordBounce(txCtx, stray); err != nil {
			return err
		}
		return errRollback
	}); !errors.Is(err, errRollback) {
		t.Fatalf("回滚那一笔：%v", err)
	}
	var strays int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.booking_answer_bounce WHERE acceptance_id = 'answer-rolled-back'`).Scan(&strays); err != nil || strays != 0 {
		t.Fatalf("回滚的挡回留下了 %d 行（%v）", strays, err)
	}

	unnamedBasis, blankSource := bounce, bounce
	unnamedBasis.Basis = ports.BookingAnswerBounceBasisInvalid
	blankSource.Source = " "
	for name, broken := range map[string]ports.BookingAnswerBounce{
		"依据不在封闭集": unnamedBasis,
		"来源为空":    blankSource,
	} {
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return bounces.RecordBounce(txCtx, broken)
		}); err == nil {
			t.Fatalf("%s 照样写进了挡回册", name)
		}
	}
}
