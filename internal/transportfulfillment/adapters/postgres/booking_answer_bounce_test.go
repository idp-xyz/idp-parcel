package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	"go.idp.xyz/idp-parcel/migrations"
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

// Covers: `AT-TF-327`（存取面）、ADR-0239 决定二的挡回册——一次挡回立一行；同键再来只加次数、推后最近一次时刻，首次时刻不动；
// 依据不同另起一行，库里存的是定宽码；只在事务里写，事务回滚不留痕；封闭集外的依据与空来源在写之前就拒。
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
		basis       int16
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
		got[0].basis != 1 || got[0].count != 1 ||
		got[1].basis != 2 || got[1].count != 2 ||
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

func newBounceLedger(t *testing.T) (*adapter.BookingAnswerBounces, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	bounces, err := adapter.NewBookingAnswerBounces(db)
	if err != nil {
		t.Fatal(err)
	}
	return bounces, db, pool
}

// Covers: ADR-0239 决定三的定宽依据——订舱标识与答复标识都在字节界上，两种依据各写一行都写得进，两行的键逐字节等长：键上唯一由
// 挡回路决定的那一格是定宽码，答复方挑什么长度的标识，两条路写的键都一样长。
func TestBothBasesWriteKeysOfTheSameWidthAtTheByteBound(t *testing.T) {
	bounces, db, pool := newBounceLedger(t)
	ctx := t.Context()
	// 随机十六进制串：重复的字符可压缩，索引行压缩之后远短于界，测不到键最长的那一形。
	booking, acceptance := randomHex(t, 512), randomHex(t, 512)
	for _, basis := range []ports.BookingAnswerBounceBasis{ports.BouncedBookingNotRegistered, ports.BouncedSourceNotServiceProvider} {
		bounce := ports.BookingAnswerBounce{
			Key: ports.BookingKey{
				TenantID: deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01"),
				Booking:  deliveryValue(t, domain.NewBookingReference, booking),
			},
			Source:     "SYN-PARTY-CARRIER-LM-01",
			Acceptance: deliveryValue(t, domain.NewCarrierAcceptanceReference, acceptance),
			Basis:      basis,
			BouncedAt:  time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC),
		}
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return bounces.RecordBounce(txCtx, bounce)
		}); err != nil {
			t.Fatalf("依据 %s 在字节界上写不进：%v", basis, err)
		}
	}
	var widths []int
	rows, err := pool.Query(ctx,
		`SELECT pg_column_size(ROW(tenant_id, source_ref, booking_id, acceptance_id, basis))
		   FROM transport_fulfillment.booking_answer_bounce ORDER BY basis`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var width int
		if err := rows.Scan(&width); err != nil {
			t.Fatal(err)
		}
		widths = append(widths, width)
	}
	rows.Close()
	if len(widths) != 2 || widths[0] != widths[1] {
		t.Fatalf("两种依据的键宽 = %v，want 两行等宽", widths)
	}
}

// Covers: ADR-0239 决定二——同一次挡回并发到达两份：先到的一笔立行未提交时，后到的一笔停在主键上；先到那笔提交之后它并进同一行，
// 次数为二、首次时刻取两份里早的那一刻（后到那份记得更早）。
func TestTwoConcurrentIdenticalBouncesCountTwiceOnOneRow(t *testing.T) {
	bounces, db, pool := newBounceLedger(t)
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
		BouncedAt:  at.Add(time.Minute),
	}
	earlier := bounce
	earlier.BouncedAt = at

	observer, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = observer.Close(context.Background()) })
	database := pool.Config().ConnConfig.Database

	held, release, firstDone, secondDone := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan error, 1)
	// 放行登记进清理：放行之前任何一处 Fatal，先到那笔会一直占着连接，连接池清理挂到 go test 超时。
	var releaseOnce sync.Once
	releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseFirst)
	go func() {
		firstDone <- db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			err := bounces.RecordBounce(txCtx, bounce)
			close(held)
			<-release
			return err
		})
	}()
	<-held
	go func() {
		secondDone <- db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			return bounces.RecordBounce(txCtx, earlier)
		})
	}()
	for deadline := time.Now().Add(10 * time.Second); ; {
		select {
		case early := <-secondDone:
			t.Fatalf("先到那笔提交之前后到的一笔就写完了：%v", early)
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
			t.Fatal("后到的一笔既没写完也没停在主键上")
		}
		time.Sleep(10 * time.Millisecond)
	}
	releaseFirst()
	if err := <-firstDone; err != nil {
		t.Fatalf("先到那笔：%v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("后到那笔：%v", err)
	}
	var first, last time.Time
	var count int64
	if err := pool.QueryRow(ctx,
		`SELECT first_bounced_at, last_bounced_at, bounce_count FROM transport_fulfillment.booking_answer_bounce`).Scan(&first, &last, &count); err != nil {
		t.Fatalf("读挡回册：%v", err)
	}
	if count != 2 || !first.Equal(at) || !last.Equal(at.Add(time.Minute)) {
		t.Fatalf("并发两份之后：次数 %d、首次 %s、最近 %s", count, first, last)
	}
}

// Covers: 迁移 0047 的拒绝门——订舱册、答复册或承运总单里有超过字节界的订舱标识或答复标识时不施加：界立起来之后，这样的行从
// 门里读不回来，不替任何人截短。答复依据不受界，长的答复依据不挡迁移（走过拒绝门、撞上已建的表）。
func TestMigration0047RefusesStoredReferencesOverTheByteBound(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	assets, err := migrations.TransportFulfillment()
	if err != nil {
		t.Fatalf("读迁移资产：%v", err)
	}
	var bounceRegister string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0047_") {
			bounceRegister = asset.SQL
		}
	}
	if bounceRegister == "" {
		t.Fatal("找不到迁移 0047")
	}
	long := strings.Repeat("A", 513)
	answerRow := func(acceptance, basis string) string {
		return `INSERT INTO transport_fulfillment.booking_answer
			(tenant_id, booking_id, acceptance_id, outcome, quantity, basis, decided_at, content_digest, recorded_at)
			VALUES ('SYN-TENANT-01', 'B-1', '` + acceptance + `', 'REFUSED', 0, '` + basis + `', now(), 'd', now())`
	}
	for name, testCase := range map[string]struct {
		probe   string
		refused bool
	}{
		"订舱册的订舱标识": {`INSERT INTO transport_fulfillment.booking_request
			(tenant_id, booking_id, commission_id, schedule_id, quantity, unit_ref, requested_at, content_digest, recorded_at)
			VALUES ('SYN-TENANT-01', '` + long + `', 'C-1', 'S-1', 1, 'KG', now(), 'd', now())`, true},
		"答复册的答复标识": {answerRow(long, "no-capacity"), true},
		"答复册的答复依据": {answerRow("A-1", strings.Repeat("订", 1000)), false},
	} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("开事务：%v", err)
		}
		if _, err := tx.Exec(ctx, testCase.probe); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("%s：插探针行：%v", name, err)
		}
		_, err = tx.Exec(ctx, bounceRegister)
		_ = tx.Rollback(context.Background())
		refused := err != nil && strings.Contains(err.Error(), "超过 512 字节")
		if refused != testCase.refused || err == nil {
			t.Fatalf("%s 超长时 0047：%v，want 拒绝门%v", name, err, map[bool]string{true: "拦下", false: "放过"}[testCase.refused])
		}
	}
}

// randomHex 交 n 个字符的随机十六进制串。
func randomHex(t *testing.T, n int) string {
	t.Helper()
	raw := make([]byte, (n+1)/2)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw)[:n]
}
