package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
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

// answersForUpdate 在一笔自己的事务里取回订舱名下的全部答复：FindAnswersForUpdate 只在事务里答。
func answersForUpdate(
	t *testing.T,
	transactor bentoapp.Transactor,
	answers *adapter.BookingAnswers,
	ctx context.Context,
	key ports.BookingKey,
) []ports.BookingAnswerRecord {
	t.Helper()
	var history []ports.BookingAnswerRecord
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		found, err := answers.FindAnswersForUpdate(txCtx, key)
		history = found
		return err
	})
	return history
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

	history := answersForUpdate(t, transactor, answers, ctx, booking.Key)
	if len(history) != 2 ||
		history[0].Acceptance.Outcome() != domain.BookingWaitlisted || history[1].Acceptance.Outcome() != domain.BookingAccepted {
		t.Fatalf("名下答复 = %d 份", len(history))
	}
	head, found, err = answers.FindByKey(ctx, booking.Key)
	if err != nil || !found || !head.Acceptance.Binds() {
		t.Fatalf("有定局时当前那一份不是定局：%+v", head.Acceptance)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM transport_fulfillment.booking_answer`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("答复表 %d 行 %v，want 2", rows, err)
	}
	if none := answersForUpdate(t, transactor, answers, ctx,
		ports.BookingKey{TenantID: deliveryValue(t, domain.NewTenantID, "tenant-b"), Booking: booking.Key.Booking}); len(none) != 0 {
		t.Fatalf("另一个租户读到了答复：%d 份", len(none))
	}
	if _, err := answers.FindAnswersForUpdate(ctx, booking.Key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外判后继的读应被拒：%v", err)
	}
}

// Covers: `AT-TF-154`——同一订舱的判定与写入按订舱串行。先到的一笔持锁落下接受、尚未提交时，另一笔对同一订舱的
// 判定读挂在锁上，不读到空册；先到那笔提交之后它读得见那份接受，于是候补落不到定局之后。别的订舱不受这把锁牵连。
func TestABookingAnswerIsJudgedAfterTheConcurrentOneSettles(t *testing.T) {
	_, bookings, answers, transactor, pool := newCommissionStores(t)
	ctx := t.Context()
	booking := submittedBookingRecord(t, "tenant-a", "booking-1")
	unrelated := submittedBookingRecord(t, "tenant-a", "booking-2")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := bookings.Save(txCtx, booking); err != nil {
			return err
		}
		_, err := bookings.Save(txCtx, unrelated)
		return err
	})
	accepted := answerRecordOf(t, booking.Booking, "answer-accepted", domain.BookingAccepted, answerAt)

	observer, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })
	database := pool.Config().ConnConfig.Database

	held, release, committed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	// 放行登记进清理：放行之前任何一处 Fatal，先到那笔会一直占着连接，连接池清理挂到 go test 超时，报不出干净的红。
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFirst)
	go func() {
		committed <- transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			prior, findErr := answers.FindAnswersForUpdate(txCtx, booking.Key)
			if findErr == nil && len(prior) != 0 {
				findErr = errors.New("先到那笔读到了答复")
			}
			if findErr == nil {
				var saved ports.BookingAnswerSaveOutcome
				saved, findErr = answers.Save(txCtx, accepted)
				if findErr == nil && saved != ports.BookingAnswerSaved {
					findErr = errors.New("先到那笔的接受没有落下")
				}
			}
			close(held)
			<-release
			return findErr
		})
	}()
	<-held

	if others := answersForUpdate(t, transactor, answers, ctx, unrelated.Key); len(others) != 0 {
		t.Fatalf("别的订舱读到了答复：%d 份", len(others))
	}

	type judged struct {
		answers []ports.BookingAnswerRecord
		err     error
	}
	judgedCh := make(chan judged, 1)
	go func() {
		var seen []ports.BookingAnswerRecord
		err := transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			found, findErr := answers.FindAnswersForUpdate(txCtx, booking.Key)
			seen = found
			return findErr
		})
		judgedCh <- judged{seen, err}
	}()

	for deadline := time.Now().Add(10 * time.Second); ; {
		select {
		case early := <-judgedCh:
			t.Fatalf("先到那笔提交之前，同一订舱的判定读就答了：%d 份 %v", len(early.answers), early.err)
		default:
		}
		var waiting int
		if err := observer.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND wait_event_type = 'Lock'`, database).Scan(&waiting); err != nil {
			t.Fatalf("看锁等待：%v", err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("同一订舱的判定读既没答也没挂在锁上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseFirst()
	if err := <-committed; err != nil {
		t.Fatalf("先到那笔：%v", err)
	}
	got := <-judgedCh
	if got.err != nil {
		t.Fatalf("后到那笔的判定读：%v", got.err)
	}
	if len(got.answers) != 1 || !got.answers[0].Acceptance.Binds() {
		t.Fatalf("后到那笔应读见先到的接受：%d 份", len(got.answers))
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
