package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

func answerRecordOf(t *testing.T, booking domain.BookingRequest, acceptanceID string, outcome domain.CarrierAcceptanceOutcome, at time.Time) ports.BookingAnswerRecord {
	t.Helper()
	var quantity int64
	basis := domain.AcceptanceBasisReference{}
	if outcome == domain.BookingAccepted {
		quantity = 40
	} else {
		basis = deliveryValue(t, domain.NewAcceptanceBasisReference, "basis-"+acceptanceID)
	}
	acceptance, err := domain.FormCarrierAcceptance(booking, deliveryValue(t, domain.NewCarrierAcceptanceReference, acceptanceID),
		outcome, quantity, basis, at)
	if err != nil {
		t.Fatalf("构造答复 %s：%v", outcome, err)
	}
	return ports.BookingAnswerRecord{
		Key:           ports.BookingKey{TenantID: booking.TenantID(), Booking: booking.Booking()},
		ContentDigest: "digest-" + acceptanceID,
		Acceptance:    acceptance,
		RecordedAt:    at,
	}
}

// Covers: `AT-TF-151` 真库面（迁移 0029）——一次订舱至多一份候补、至多一份定局：候补之后落一份接受，两份都在、按答复时刻
// 先后读回，当前那一份是接受；第二份定局、第二份候补、同一答复标识都撞约束答已有应答，册上不多一行；候补带依据往返。
func TestABookingKeepsAWaitlistAndOneSettlingAnswer(t *testing.T) {
	_, bookings, answers, transactor, pool := newCommissionStores(t)
	ctx := t.Context()
	booking := submittedBookingRecord(t, "tenant-a", "booking-1")
	save := func(record ports.BookingAnswerRecord) ports.BookingAnswerSaveOutcome {
		t.Helper()
		var outcome ports.BookingAnswerSaveOutcome
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			saved, err := answers.Save(txCtx, record)
			outcome = saved
			return err
		})
		return outcome
	}
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := bookings.Save(txCtx, booking)
		return err
	})

	waitlisted := answerRecordOf(t, booking.Booking, "answer-waitlist", domain.BookingWaitlisted, answerAt)
	if outcome := save(waitlisted); outcome != ports.BookingAnswerSaved {
		t.Fatalf("候补 = %d", outcome)
	}
	head, found, err := answers.FindByKey(ctx, booking.Key)
	if err != nil || !found || head.Acceptance.Outcome() != domain.BookingWaitlisted {
		t.Fatalf("只有候补时当前那一份 = %+v found=%v err=%v", head.Acceptance, found, err)
	}
	if basis, present := head.Acceptance.Basis(); !present || basis.String() != "basis-answer-waitlist" {
		t.Fatalf("候补的依据没有往返：%q %v", basis, present)
	}

	accepted := answerRecordOf(t, booking.Booking, "answer-accepted", domain.BookingAccepted, answerAt.Add(time.Hour))
	if outcome := save(accepted); outcome != ports.BookingAnswerSaved {
		t.Fatalf("候补之后的接受 = %d", outcome)
	}
	for name, record := range map[string]ports.BookingAnswerRecord{
		"第二份定局":  answerRecordOf(t, booking.Booking, "answer-refused", domain.BookingRefused, answerAt.Add(2*time.Hour)),
		"第二份候补":  answerRecordOf(t, booking.Booking, "answer-waitlist-2", domain.BookingWaitlisted, answerAt.Add(2*time.Hour)),
		"同一答复标识": answerRecordOf(t, booking.Booking, "answer-waitlist", domain.BookingRefused, answerAt.Add(2*time.Hour)),
	} {
		if outcome := save(record); outcome != ports.BookingAlreadyAnswered {
			t.Fatalf("%s = %d, want 已有应答", name, outcome)
		}
	}

	history, err := answers.FindAnswers(ctx, booking.Key)
	if err != nil || len(history) != 2 ||
		history[0].Acceptance.Outcome() != domain.BookingWaitlisted || history[1].Acceptance.Outcome() != domain.BookingAccepted {
		t.Fatalf("名下答复 = %d 份 %v", len(history), err)
	}
	head, found, err = answers.FindByKey(ctx, booking.Key)
	if err != nil || !found || !head.Acceptance.Binds() {
		t.Fatalf("有定局时当前那一份不是定局：%+v", head.Acceptance)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transport_fulfillment.booking_answer`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("答复表 %d 行 %v，want 2", rows, err)
	}
	if none, err := answers.FindAnswers(ctx, ports.BookingKey{TenantID: deliveryValue(t, domain.NewTenantID, "tenant-b"), Booking: booking.Key.Booking}); err != nil || len(none) != 0 {
		t.Fatalf("另一个租户读到了答复：%d %v", len(none), err)
	}
}

// Covers: 迁移 0029 的拒绝门——订舱表有行时不施加，不替任何人猜那次订舱申请的是哪一班。
func TestMigration0029RefusesBookingsItCannotGiveASchedule(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移资产：%v", err)
	}
	var scheduling string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0029_") {
			scheduling = asset.SQL
		}
	}
	if scheduling == "" {
		t.Fatal("找不到迁移 0029")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_request
			(tenant_id, booking_id, commission_id, schedule_id, quantity, unit_ref, requested_at, content_digest, recorded_at)
		 VALUES ('SYN-TENANT-01', 'B-1', 'C-1', 'S-1', 1, 'KG', now(), 'd', now())`); err != nil {
		t.Fatalf("插探针行：%v", err)
	}
	if _, err := tx.Exec(ctx, scheduling); err == nil || !strings.Contains(err.Error(), "已有行") {
		t.Fatalf("订舱表有行时 0029 应拒绝施加：%v", err)
	}
}
