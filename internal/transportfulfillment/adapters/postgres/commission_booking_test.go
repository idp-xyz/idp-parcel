package postgres_test

import (
	"context"
	"errors"
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

var (
	commissionAt = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	bookingAt    = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	answerAt     = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
)

func TestACommissionRoundTripsAndReplaceOnlyWritesTransitions(t *testing.T) {
	commissions, _, _, transactor, _ := newCommissionStores(t)
	ctx := t.Context()

	record := submittedCommission(t, "tenant-a", "commission-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := commissions.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.CommissionSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := commissions.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.Commission.Agreement().String() != "agreement-snapshot-1" ||
		len(found.Commission.Members()) != 1 {
		t.Fatal("快照或成员往返丢失")
	}

	cancelled, err := found.Commission.Cancel(commissionAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("cancel：%v", err)
	}
	replaced := found
	replaced.Commission = cancelled
	replaced.RecordedAt = commissionAt.Add(time.Hour)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := commissions.Replace(txCtx, replaced)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("取消 Replace 答 false")
		}
		return nil
	})

	after, _, err := commissions.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("取消后读回：%v", err)
	}
	if _, ok := after.Commission.Cancelled(); !ok {
		t.Fatal("取消没有落库")
	}
	if after.Commission.Agreement().String() != "agreement-snapshot-1" {
		t.Fatal("Replace 改写了快照")
	}

	started, err := record.Commission.MarkTransportStarted(
		deliveryValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/v1"),
		commissionAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("mark started in memory：%v", err)
	}
	startAttempt := found
	startAttempt.Commission = started
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := commissions.Replace(txCtx, startAttempt)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("已取消的委托又被开始了")
		}
		return nil
	})
}

func TestASecondCommissionWriterGetsAlreadyRegistered(t *testing.T) {
	commissions, _, _, transactor, _ := newCommissionStores(t)
	ctx := t.Context()

	first := submittedCommission(t, "tenant-a", "commission-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Save(txCtx, first)
		return err
	})

	second := submittedCommission(t, "tenant-a", "commission-1")
	second.ContentDigest = "digest-other"
	var outcome ports.CommissionSaveOutcome
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := commissions.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, found, err := commissions.FindByKey(txCtx, first.Key)
		if err != nil || !found || winner.ContentDigest != first.ContentDigest {
			t.Fatalf("同事务读回赢家失败：found=%v digest=%q err=%v", found, winner.ContentDigest, err)
		}
		return nil
	})
	if outcome != ports.CommissionAlreadyRegistered {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestABookingAndAnswerRoundTripAndAcceptedCannotBeRefused(t *testing.T) {
	_, bookings, answers, transactor, _ := newCommissionStores(t)
	ctx := t.Context()

	booking := submittedBookingRecord(t, "tenant-a", "booking-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := bookings.Save(txCtx, booking)
		if err != nil {
			return err
		}
		if outcome != ports.BookingSaved {
			t.Fatalf("booking save = %d", outcome)
		}
		return nil
	})

	foundBooking, exists, err := bookings.FindByKey(ctx, booking.Key)
	if err != nil || !exists {
		t.Fatalf("订舱读回失败：err=%v exists=%v", err, exists)
	}
	qty, unit := foundBooking.Booking.Quantity()
	if qty != 100 || unit.String() != "kg" {
		t.Fatalf("订舱往返变形：qty=%d unit=%s", qty, unit)
	}

	accepted := acceptedAnswer(t, foundBooking.Booking, "acceptance-1", 40)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := answers.Save(txCtx, accepted)
		if err != nil {
			return err
		}
		if outcome != ports.BookingAnswerSaved {
			t.Fatalf("answer save = %d", outcome)
		}
		return nil
	})

	foundAnswer, exists, err := answers.FindByKey(ctx, booking.Key)
	if err != nil || !exists || !foundAnswer.Acceptance.Binds() || foundAnswer.Acceptance.AcceptedQuantity() != 40 {
		t.Fatalf("应答往返失败：exists=%v binds=%v qty=%d err=%v",
			exists, foundAnswer.Acceptance.Binds(), foundAnswer.Acceptance.AcceptedQuantity(), err)
	}

	refused := refusedAnswer(t, foundBooking.Booking, "acceptance-2")
	var outcome ports.BookingAnswerSaveOutcome
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := answers.Save(txCtx, refused)
		if err != nil {
			return err
		}
		outcome = saved
		winner, found, err := answers.FindByKey(txCtx, booking.Key)
		if err != nil || !found || !winner.Acceptance.Binds() {
			t.Fatalf("已接受被拒绝覆盖：found=%v binds=%v err=%v", found, winner.Acceptance.Binds(), err)
		}
		return nil
	})
	if outcome != ports.BookingAlreadyAnswered {
		t.Fatalf("第二份应答结果 = %d", outcome)
	}
}

func TestCommissionBookingRecordsAreInvisibleAcrossTenants(t *testing.T) {
	commissions, bookings, answers, transactor, _ := newCommissionStores(t)
	ctx := t.Context()

	commission := submittedCommission(t, "tenant-a", "commission-shared")
	booking := submittedBookingRecord(t, "tenant-a", "booking-shared")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := commissions.Save(txCtx, commission); err != nil {
			return err
		}
		if _, err := bookings.Save(txCtx, booking); err != nil {
			return err
		}
		_, err := answers.Save(txCtx, acceptedAnswer(t, booking.Booking, "acceptance-1", 10))
		return err
	})

	other := deliveryValue(t, domain.NewTenantID, "tenant-b")
	if _, exists, err := commissions.FindByKey(ctx, ports.TransportCommissionKey{TenantID: other, Commission: commission.Key.Commission}); err != nil || exists {
		t.Errorf("另一个租户读到了委托：exists=%v err=%v", exists, err)
	}
	if _, exists, err := bookings.FindByKey(ctx, ports.BookingKey{TenantID: other, Booking: booking.Key.Booking}); err != nil || exists {
		t.Errorf("另一个租户读到了订舱：exists=%v err=%v", exists, err)
	}
	if _, exists, err := answers.FindByKey(ctx, ports.BookingKey{TenantID: other, Booking: booking.Key.Booking}); err != nil || exists {
		t.Errorf("另一个租户读到了应答：exists=%v err=%v", exists, err)
	}
}

func TestCommissionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	commissions, bookings, answers, _, _ := newCommissionStores(t)
	ctx := t.Context()

	commission := submittedCommission(t, "tenant-a", "commission-1")
	if _, err := commissions.Save(ctx, commission); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 委托：%v", err)
	}
	if _, err := commissions.Replace(ctx, commission); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Replace 委托：%v", err)
	}
	booking := submittedBookingRecord(t, "tenant-a", "booking-1")
	if _, err := bookings.Save(ctx, booking); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 订舱：%v", err)
	}
	if _, err := answers.Save(ctx, acceptedAnswer(t, booking.Booking, "acceptance-1", 10)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 应答：%v", err)
	}
}

func TestCommissionRollbackLeavesNothingBehind(t *testing.T) {
	commissions, bookings, _, transactor, _ := newCommissionStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	commission := submittedCommission(t, "tenant-a", "commission-1")
	booking := submittedBookingRecord(t, "tenant-a", "booking-1")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := commissions.Save(txCtx, commission); err != nil {
			return err
		}
		if _, err := bookings.Save(txCtx, booking); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := commissions.FindByKey(ctx, commission.Key); err != nil || exists {
		t.Errorf("回滚后委托仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := bookings.FindByKey(ctx, booking.Key); err != nil || exists {
		t.Errorf("回滚后订舱仍在：exists=%v err=%v", exists, err)
	}
}

func TestCommissionBookingCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, _, pool := newCommissionStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_commission
			(tenant_id, commission_id, provider_ref, agreement_ref, conditions_ref,
			 role_ref, responsibility_ref, members, submitted_at, started_at, started_basis,
			 cancelled_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'c-bad-1', 'p', 'a', 'c', 'r', 's', '["parcel-1"]', now(),
		         now(), 'basis', now(), 'd', now())`); err == nil {
		t.Fatal("一行「既开始又取消」溜进了委托库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_commission
			(tenant_id, commission_id, provider_ref, agreement_ref, conditions_ref,
			 role_ref, responsibility_ref, members, submitted_at, started_at, started_basis,
			 content_digest, recorded_at)
		 VALUES ('tenant-a', 'c-bad-2', 'p', 'a', 'c', 'r', 's', '["parcel-1"]', now(),
		         now(), NULL, 'd', now())`); err == nil {
		t.Fatal("一行「开始却没有依据」溜进了委托库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_commission
			(tenant_id, commission_id, provider_ref, agreement_ref, conditions_ref,
			 role_ref, responsibility_ref, members, submitted_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'c-bad-3', 'p', 'a', 'c', 'r', 's', NULL, now(), 'd', now())`); err == nil {
		t.Fatal("一行「成员列为 NULL」按 jsonb 三值缝溜进了委托库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_request
			(tenant_id, booking_id, commission_id, quantity, unit_ref, requested_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'b-bad-1', 'c-1', 0, 'kg', now(), 'd', now())`); err == nil {
		t.Fatal("一行「订舱数量为零」溜进了订舱库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_answer
			(tenant_id, booking_id, acceptance_id, outcome, quantity, basis, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-1', 'acc-1', 'ACCEPTED', 10, 'why', now(), 'd', now())`); err == nil {
		t.Fatal("一行「接受却带着原因」溜进了应答库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_answer
			(tenant_id, booking_id, acceptance_id, outcome, quantity, basis, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-2', 'acc-1', 'REFUSED', 10, 'why', now(), 'd', now())`); err == nil {
		t.Fatal("一行「拒绝却带着接受量」溜进了应答库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.booking_answer
			(tenant_id, booking_id, acceptance_id, outcome, quantity, basis, decided_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'a-bad-3', 'acc-1', 'REFUSED', 0, NULL, now(), 'd', now())`); err == nil {
		t.Fatal("一行「拒绝却没有原因」溜进了应答库")
	}
}

func newCommissionStores(t *testing.T) (
	*adapter.TransportCommissions,
	*adapter.BookingRequests,
	*adapter.BookingAnswers,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	commissions, err := adapter.NewTransportCommissions(db)
	if err != nil {
		t.Fatalf("构造委托库：%v", err)
	}
	bookings, err := adapter.NewBookingRequests(db)
	if err != nil {
		t.Fatalf("构造订舱库：%v", err)
	}
	answers, err := adapter.NewBookingAnswers(db)
	if err != nil {
		t.Fatalf("构造应答库：%v", err)
	}
	return commissions, bookings, answers, db.Transactor(), pool
}

func submittedCommission(t *testing.T, tenant, id string) ports.TransportCommissionRecord {
	t.Helper()
	commission, err := domain.SubmitTransportCommission(domain.TransportCommissionSpec{
		TenantID:       deliveryValue(t, domain.NewTenantID, tenant),
		Commission:     deliveryValue(t, domain.NewTransportCommissionReference, id),
		Provider:       deliveryValue(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:      deliveryValue(t, domain.NewAgreementSnapshotReference, "agreement-snapshot-1"),
		Conditions:     deliveryValue(t, domain.NewConditionsSnapshotReference, "conditions-snapshot-1"),
		Role:           deliveryValue(t, domain.NewRoleSnapshotReference, "role-snapshot-1"),
		Responsibility: deliveryValue(t, domain.NewResponsibilitySnapshotReference, "responsibility-snapshot-1"),
		Members:        []domain.CarriedObjectReference{deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1")},
		SubmittedAt:    commissionAt,
	})
	if err != nil {
		t.Fatalf("构造委托：%v", err)
	}
	return ports.TransportCommissionRecord{
		Key:           ports.TransportCommissionKey{TenantID: commission.TenantID(), Commission: commission.Commission()},
		ContentDigest: "digest-" + id,
		Commission:    commission,
		RecordedAt:    commissionAt,
	}
}

func submittedBookingRecord(t *testing.T, tenant, id string) ports.BookingRecord {
	t.Helper()
	booking, err := domain.SubmitBookingRequest(domain.BookingRequestSpec{
		TenantID:    deliveryValue(t, domain.NewTenantID, tenant),
		Booking:     deliveryValue(t, domain.NewBookingReference, id),
		Commission:  deliveryValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Quantity:    100,
		Unit:        deliveryValue(t, domain.NewQuantityUnitReference, "kg"),
		RequestedAt: bookingAt,
	})
	if err != nil {
		t.Fatalf("构造订舱：%v", err)
	}
	return ports.BookingRecord{
		Key:           ports.BookingKey{TenantID: booking.TenantID(), Booking: booking.Booking()},
		ContentDigest: "digest-" + id,
		Booking:       booking,
		RecordedAt:    bookingAt,
	}
}

func acceptedAnswer(t *testing.T, booking domain.BookingRequest, acceptanceID string, qty int64) ports.BookingAnswerRecord {
	t.Helper()
	acceptance, err := domain.FormCarrierAcceptance(
		booking,
		deliveryValue(t, domain.NewCarrierAcceptanceReference, acceptanceID),
		domain.BookingAccepted,
		qty,
		domain.AcceptanceBasisReference{},
		answerAt,
	)
	if err != nil {
		t.Fatalf("构造成接受应答：%v", err)
	}
	return ports.BookingAnswerRecord{
		Key:           ports.BookingKey{TenantID: booking.TenantID(), Booking: booking.Booking()},
		ContentDigest: "digest-accepted-" + acceptanceID,
		Acceptance:    acceptance,
		RecordedAt:    answerAt,
	}
}

func refusedAnswer(t *testing.T, booking domain.BookingRequest, acceptanceID string) ports.BookingAnswerRecord {
	t.Helper()
	acceptance, err := domain.FormCarrierAcceptance(
		booking,
		deliveryValue(t, domain.NewCarrierAcceptanceReference, acceptanceID),
		domain.BookingRefused,
		0,
		deliveryValue(t, domain.NewAcceptanceBasisReference, "no-capacity"),
		answerAt,
	)
	if err != nil {
		t.Fatalf("构造拒绝应答：%v", err)
	}
	return ports.BookingAnswerRecord{
		Key:           ports.BookingKey{TenantID: booking.TenantID(), Booking: booking.Booking()},
		ContentDigest: "digest-refused-" + acceptanceID,
		Acceptance:    acceptance,
		RecordedAt:    answerAt,
	}
}
