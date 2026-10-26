package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0303 复核记录（迁移 0054，票 demo-chain-gaps/180）的存储半边：订舱的提交人与取消人、委托的
// 取消人、采用的那一代的采用人、各份预占的预占人与各次释放的释放人落得进、读得回；池按值换存（明细整组删了再插回）后证据原样
// 带着；同一个决定换一位操作者再写不顶替在册的那一位；没有操作者的行读回是「没记下」；库面 CHECK 挡空白与错位；补拍与按册重问
// 在问询册一次按下一行，按的人可查。夹具全为合成登记（S 级）。

// Covers: `AT-TF-348`、`AT-TF-349` 存储半边——订舱的提交人随订舱落列、取消人随取消落列，各记各的，取消不动提交人；另一位操作者
// 再交（撞键）、再取消（WHERE 要求尚未取消）都写中 0 行，在册的不被顶替；不带操作者的订舱两列都是 NULL；订舱册读面带出两位；
// CHECK 挡两列写空白、挡取消人落在没取消的订舱上。
func TestTheBookingRegistryKeepsTheSubmittingAndCancellingOperators(t *testing.T) {
	_, bookings, _, transactor, pool := newCommissionStores(t)
	ctx := t.Context()
	record := submittedBookingRecord(t, "tenant-a", "booking-OE1")
	record.SubmittedBy = operatorDB(t, operatorADB)
	save := func(record ports.BookingRecord) ports.BookingSaveOutcome {
		var outcome ports.BookingSaveOutcome
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = bookings.Save(txCtx, record)
			return err
		})
		return outcome
	}
	if outcome := save(record); outcome != ports.BookingSaved {
		t.Fatalf("订舱 outcome = %d", outcome)
	}
	replay := record
	replay.SubmittedBy = operatorDB(t, operatorBDB)
	if outcome := save(replay); outcome != ports.BookingAlreadyRegistered {
		t.Fatalf("另一位操作者再交 outcome = %d", outcome)
	}

	found, _, err := bookings.FindByKey(ctx, record.Key)
	if err != nil || found.SubmittedBy.String() != operatorADB || found.CancelledBy.Present() {
		t.Fatalf("读回 = 提交人 %q 取消人 %q err=%v，want 先交的那一位、没有取消人", found.SubmittedBy.String(), found.CancelledBy.String(), err)
	}
	cancelled, err := found.Booking.Cancel(bookingAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cancel := func(by string) bool {
		attempt := found
		attempt.Booking = cancelled
		attempt.SubmittedBy = operatorDB(t, operatorBDB)
		attempt.CancelledBy = operatorDB(t, by)
		var ok bool
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			ok, err = bookings.Replace(txCtx, attempt)
			return err
		})
		return ok
	}
	if !cancel(operatorADB) {
		t.Fatal("取消应落上")
	}
	if cancel(operatorBDB) {
		t.Fatal("另一位操作者再取消应写中 0 行")
	}
	wantColumn(t, readColumn(t, pool, `SELECT submitted_by FROM transport_fulfillment.booking_request WHERE booking_id = 'booking-OE1'`), operatorADB, "取消之后的提交人")
	wantColumn(t, readColumn(t, pool, `SELECT cancelled_by FROM transport_fulfillment.booking_request WHERE booking_id = 'booking-OE1'`), operatorADB, "取消人")

	if outcome := save(submittedBookingRecord(t, "tenant-a", "booking-OE2")); outcome != ports.BookingSaved {
		t.Fatalf("不带操作者订舱 outcome = %d", outcome)
	}
	for _, column := range []string{"submitted_by", "cancelled_by"} {
		wantColumn(t, readColumn(t, pool, `SELECT `+column+` FROM transport_fulfillment.booking_request WHERE booking_id = 'booking-OE2'`), "", "不带操作者的"+column)
	}

	_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.booking_request SET submitted_by = ' ' WHERE booking_id = 'booking-OE2'`)
	wantCheckViolation(t, err, "booking_request_submitted_by_not_blank")
	_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.booking_request SET cancelled_by = $1 WHERE booking_id = 'booking-OE2'`, operatorADB)
	wantCheckViolation(t, err, "booking_request_cancelled_by_only_when_cancelled")
	_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.booking_request SET cancelled_at = now(), cancelled_by = ' ' WHERE booking_id = 'booking-OE2'`)
	wantCheckViolation(t, err, "booking_request_cancelled_by_only_when_cancelled")

	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("构造查阅册：%v", err)
	}
	rows, err := catalogue.ListBookingRequests(ctx, deliveryValue(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("订舱册：%v", err)
	}
	shown := map[string]ports.BookingRequestCatalogueRow{}
	for _, each := range rows {
		shown[each.Booking] = each
	}
	if shown["booking-OE1"].SubmittedBy != operatorADB || shown["booking-OE1"].CancelledBy != operatorADB ||
		shown["booking-OE2"].SubmittedBy != "" || shown["booking-OE2"].CancelledBy != "" {
		t.Fatalf("订舱册 = %+v", shown)
	}
}

// Covers: `AT-TF-350` 存储半边——委托的取消人随取消那一次 Replace 落列，与提交人各记各的；另一位操作者再取消写中 0 行；委托开始的
// Replace 不碰取消人；CHECK 挡取消人落在没取消的委托上、挡写空白。
func TestTheCommissionRegistryKeepsTheCancellingOperator(t *testing.T) {
	commissions, _, _, transactor, pool := newCommissionStores(t)
	ctx := t.Context()
	record := submittedCommission(t, "tenant-a", "commission-OE1")
	record.SubmittedBy = operatorDB(t, operatorADB)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Save(txCtx, record)
		return err
	})
	found, _, err := commissions.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := found.Commission.Cancel(commissionAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cancel := func(by string) bool {
		attempt := found
		attempt.Commission = cancelled
		attempt.CancelledBy = operatorDB(t, by)
		var ok bool
		mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			var err error
			ok, err = commissions.Replace(txCtx, attempt)
			return err
		})
		return ok
	}
	if !cancel(operatorBDB) {
		t.Fatal("取消应落上")
	}
	if cancel(operatorADB) {
		t.Fatal("另一位操作者再取消应写中 0 行")
	}
	after, _, err := commissions.FindByKey(ctx, record.Key)
	if err != nil || after.CancelledBy.String() != operatorBDB || after.SubmittedBy.String() != operatorADB {
		t.Fatalf("读回 = 取消人 %q 提交人 %q err=%v，want 先取消的那一位与各记各的提交人", after.CancelledBy.String(), after.SubmittedBy.String(), err)
	}

	started := submittedCommission(t, "tenant-a", "commission-OE2")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Save(txCtx, started)
		return err
	})
	startedBody, err := started.Commission.MarkTransportStarted(deliveryValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/v1"), commissionAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	started.Commission = startedBody
	started.CancelledBy = operatorDB(t, operatorADB)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := commissions.Replace(txCtx, started)
		return err
	})
	wantColumn(t, readColumn(t, pool, `SELECT cancelled_by FROM transport_fulfillment.transport_commission WHERE commission_id = 'commission-OE2'`), "", "开始之后的取消人")

	_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.transport_commission SET cancelled_by = $1 WHERE commission_id = 'commission-OE2'`, operatorADB)
	wantCheckViolation(t, err, "transport_commission_cancelled_by_only_when_cancelled")
	_, err = pool.Exec(ctx, `UPDATE transport_fulfillment.transport_commission SET cancelled_by = ' ' WHERE commission_id = 'commission-OE1'`)
	wantCheckViolation(t, err, "transport_commission_cancelled_by_only_when_cancelled")
}

// Covers: `AT-TF-351` 存储半边——采用的那一代带着采用人落行，按段读与按委托列都读回；另一位操作者对同段再交写中 0 行，在册的
// 不被顶替；不带操作者的采用是 NULL；CHECK 挡写空白。
func TestTheAdoptionRegisterKeepsTheAdoptingOperator(t *testing.T) {
	stores := newAdoptionStores(t)
	ctx := t.Context()
	stores.seed(t, ctx)
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID:   tenant,
		Segment:    deliveryValue(t, domain.NewFulfillmentSegmentReference, "SEG-ADOPT"),
		Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1"),
		Provider:   deliveryValue(t, domain.NewServiceProviderReference, "partner-1"),
		StartBasis: deliveryValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/parcel-1"),
		StartedAt:  segmentEnteredAtFixture,
	})
	if err != nil {
		t.Fatal(err)
	}
	record := ports.SegmentCommissionAdoptionRecord{
		Key:           ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: adoption.Segment()},
		ContentDigest: "digest", Adoption: adoption, RecordedAt: segmentEnteredAtFixture, AdoptedBy: operatorDB(t, operatorADB),
	}
	save := func(record ports.SegmentCommissionAdoptionRecord) ports.AdoptionSaveOutcome {
		var outcome ports.AdoptionSaveOutcome
		mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = stores.adoptions.Save(txCtx, record)
			return err
		})
		return outcome
	}
	if outcome := save(record); outcome != ports.AdoptionSaved {
		t.Fatalf("采用 outcome = %d", outcome)
	}
	replay := record
	replay.AdoptedBy = operatorDB(t, operatorBDB)
	if outcome := save(replay); outcome != ports.AdoptionAlreadyRegistered {
		t.Fatalf("另一位操作者再交 outcome = %d", outcome)
	}
	found, _, err := stores.adoptions.FindBySegment(ctx, record.Key)
	if err != nil || found.AdoptedBy.String() != operatorADB {
		t.Fatalf("按段读回采用人 = %q err=%v，want %q", found.AdoptedBy.String(), err, operatorADB)
	}
	listed, err := stores.adoptions.ListByCommission(ctx, ports.TransportCommissionKey{
		TenantID: tenant, Commission: deliveryValue(t, domain.NewTransportCommissionReference, "commission-1"),
	})
	if err != nil || len(listed) != 1 || listed[0].AdoptedBy.String() != operatorADB {
		t.Fatalf("按委托列 = %+v err=%v，want 一份带采用人", listed, err)
	}

	mustSaveSegment(t, stores.transactor, ctx, stores.segments, segmentRecord(t, "SEG-ADOPT-2", activeMember(t, "parcel-2")))
	second := record
	second.Key.Segment = deliveryValue(t, domain.NewFulfillmentSegmentReference, "SEG-ADOPT-2")
	secondAdoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID: tenant, Segment: second.Key.Segment, Commission: adoption.Commission(), Provider: adoption.Provider(),
		StartBasis: deliveryValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/parcel-2"), StartedAt: segmentEnteredAtFixture,
	})
	if err != nil {
		t.Fatal(err)
	}
	second.Adoption = secondAdoption
	second.AdoptedBy = domain.RegistrantReference{}
	if outcome := save(second); outcome != ports.AdoptionSaved {
		t.Fatalf("不带操作者采用 outcome = %d", outcome)
	}
	wantColumn(t, readColumn(t, stores.pool, `SELECT adopted_by FROM transport_fulfillment.segment_commission_adoption WHERE segment_ref = 'SEG-ADOPT-2'`), "", "不带操作者的采用人")
	_, err = stores.pool.Exec(ctx, `UPDATE transport_fulfillment.segment_commission_adoption SET adopted_by = ' ' WHERE segment_ref = 'SEG-ADOPT-2'`)
	wantCheckViolation(t, err, "segment_commission_adoption_adopted_by_not_blank")
}

// Covers: `AT-TF-352`、`AT-TF-353` 存储半边——预占人与释放人经编排落进池的预占与释放明细、随池读回；池按值换存（预占与释放明细整组
// 删了再插回）之后已有的证据原样带着，另一位操作者重交不顶替；不带操作者的预占与释放是 NULL；CHECK 挡写空白。
func TestTheCapacityPoolKeepsTheReservingAndReleasingOperatorsAcrossReplacements(t *testing.T) {
	stores := newScheduleRegistryStores(t)
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-01")
	pool := registeredWeightPool(t, stores, tenant)

	first := registryReserveCommand(tenant, pool, "SYN-RES-1", 1200)
	first.ReservedBy = operatorADB
	wantPoolOutcome(t, stores.reserve(t, first), application.CapacityReserved)
	replay := first
	replay.ReservedBy = operatorBDB
	wantPoolOutcome(t, stores.reserve(t, replay), application.ReservationExistingResult)
	wantPoolOutcome(t, stores.reserve(t, registryReserveCommand(tenant, pool, "SYN-RES-2", 300)), application.CapacityReserved)

	releasedAt := registryReservedAt.Add(24 * time.Hour)
	release := registryReleaseCommand(tenant, pool, "SYN-RES-1", "SYN-REL-1", 200, releasedAt)
	release.ReleasedBy = operatorBDB
	wantPoolOutcome(t, stores.release(t, release), application.CapacityReleased)
	again := release
	again.ReleasedBy = operatorADB
	wantPoolOutcome(t, stores.release(t, again), application.ReleaseExistingResult)
	wantPoolOutcome(t, stores.release(t, registryReleaseCommand(tenant, pool, "SYN-RES-1", "SYN-REL-2", 100, releasedAt)), application.CapacityReleased)

	found, exists, err := stores.pools.FindByKey(t.Context(), ports.CapacityPoolKey{TenantID: tenant, Pool: deliveryValue(t, domain.NewCapacityPoolReference, pool)})
	if err != nil || !exists {
		t.Fatalf("读回池：exists=%v err=%v", exists, err)
	}
	reservedBy := func(reference string) string {
		return found.ReservedBy[deliveryValue(t, domain.NewCapacityReservationReference, reference)].String()
	}
	releasedBy := func(reference string) string {
		return found.ReleasedBy[deliveryValue(t, domain.NewCapacityReleaseReference, reference)].String()
	}
	if reservedBy("SYN-RES-1") != operatorADB || reservedBy("SYN-RES-2") != "" || releasedBy("SYN-REL-1") != operatorBDB || releasedBy("SYN-REL-2") != "" {
		t.Fatalf("读回 = 预占人 %v 释放人 %v，want 各份只记各自首投的那一位，没带的缺席", found.ReservedBy, found.ReleasedBy)
	}
	wantColumn(t, readColumn(t, stores.pool, `SELECT reserved_by FROM transport_fulfillment.capacity_reservation WHERE reservation_id = 'SYN-RES-2'`), "", "不带操作者的预占人")
	wantColumn(t, readColumn(t, stores.pool, `SELECT released_by FROM transport_fulfillment.capacity_release WHERE release_id = 'SYN-REL-2'`), "", "不带操作者的释放人")

	_, err = stores.pool.Exec(t.Context(), `UPDATE transport_fulfillment.capacity_reservation SET reserved_by = ' ' WHERE reservation_id = 'SYN-RES-2'`)
	wantCheckViolation(t, err, "capacity_reservation_reserved_by_not_blank")
	_, err = stores.pool.Exec(t.Context(), `UPDATE transport_fulfillment.capacity_release SET released_by = ' ' WHERE release_id = 'SYN-REL-2'`)
	wantCheckViolation(t, err, "capacity_release_released_by_not_blank")
}

func newDeliveryDispatchAsks(t *testing.T) (*adapter.DeliveryDispatchAsks, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	asks, err := adapter.NewDeliveryDispatchAsks(db)
	if err != nil {
		t.Fatalf("构造问询册：%v", err)
	}
	return asks, db, pool
}

// Covers: `AT-TF-354`、`AT-TF-355` 存储半边——补拍（段与对象）与按册重问（问法与件数上限）各一次按下一行，按的人可查，由近及远；
// 同一件被两位操作者各补拍一次是两行；形状互斥的 CHECK 挡补拍带问法、重问带段与对象、问法不在封闭集内、件数上限非正、
// 按的人写空白；任务登记册不因此长出建任务人；他租户的按下不串。
func TestTheDeliveryDispatchAskLedgerKeepsWhoPressedWhat(t *testing.T) {
	asks, db, pool := newDeliveryDispatchAsks(t)
	ctx := t.Context()
	tenant := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-ASK")
	other := deliveryValue(t, domain.NewTenantID, "SYN-TENANT-ASK-2")
	segment := deliveryValue(t, domain.NewFulfillmentSegmentReference, "SYN-SEG-ASK")
	object := deliveryValue(t, domain.NewCarriedObjectReference, "SYN-OBJ-ASK")
	at := time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC)

	record := func(record ports.DeliveryDispatchAskRecord) {
		t.Helper()
		mustWithinDeliveryTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error { return asks.RecordAsk(txCtx, record) })
	}
	record(ports.DeliveryDispatchAskRecord{TenantID: tenant, Kind: ports.DeliveryDispatchAskTrigger, AskedBy: operatorDB(t, operatorADB), AskedAt: at, Segment: segment, Object: object})
	record(ports.DeliveryDispatchAskRecord{TenantID: tenant, Kind: ports.DeliveryDispatchAskTrigger, AskedBy: operatorDB(t, operatorBDB), AskedAt: at.Add(time.Minute), Segment: segment, Object: object})
	record(ports.DeliveryDispatchAskRecord{TenantID: tenant, Kind: ports.DeliveryDispatchAskReask, AskedBy: operatorDB(t, operatorADB), AskedAt: at.Add(2 * time.Minute), Scope: "DUE", Limit: 50})
	record(ports.DeliveryDispatchAskRecord{TenantID: other, Kind: ports.DeliveryDispatchAskReask, AskedBy: operatorDB(t, operatorBDB), AskedAt: at, Scope: "ALL", Limit: 10})

	listed, err := asks.ListAsks(ctx, tenant, 10)
	if err != nil || len(listed) != 3 {
		t.Fatalf("ListAsks = %+v err=%v，want 本租户三次按下", listed, err)
	}
	if listed[0].Kind != ports.DeliveryDispatchAskReask || listed[0].AskedBy.String() != operatorADB || listed[0].Scope != "DUE" || listed[0].Limit != 50 ||
		listed[0].Segment.String() != "" || !listed[0].AskedAt.Equal(at.Add(2*time.Minute)) {
		t.Fatalf("最近一次（重问）= %+v", listed[0])
	}
	if listed[1].AskedBy.String() != operatorBDB || listed[2].AskedBy.String() != operatorADB ||
		listed[1].Segment != segment || listed[1].Object != object || listed[1].Scope != "" {
		t.Fatalf("两次补拍 = %+v、%+v，want 各记各的人，带段与对象", listed[1], listed[2])
	}
	if limited, err := asks.ListAsks(ctx, tenant, 1); err != nil || len(limited) != 1 {
		t.Fatalf("limit 1 = %d err=%v", len(limited), err)
	}

	for _, statement := range map[string]string{
		"trigger carrying a scope":    `INSERT INTO transport_fulfillment.delivery_dispatch_ask (tenant_id, kind, asked_by, asked_at, segment_ref, object_ref, scope, batch_limit, recorded_at) VALUES ('T', 'TRIGGER', 'i#s', now(), 'S', 'O', 'ALL', 1, now())`,
		"trigger without an object":   `INSERT INTO transport_fulfillment.delivery_dispatch_ask (tenant_id, kind, asked_by, asked_at, segment_ref, recorded_at) VALUES ('T', 'TRIGGER', 'i#s', now(), 'S', now())`,
		"reask carrying a segment":    `INSERT INTO transport_fulfillment.delivery_dispatch_ask (tenant_id, kind, asked_by, asked_at, segment_ref, scope, batch_limit, recorded_at) VALUES ('T', 'REASK', 'i#s', now(), 'S', 'ALL', 1, now())`,
		"reask with an unknown scope": `INSERT INTO transport_fulfillment.delivery_dispatch_ask (tenant_id, kind, asked_by, asked_at, scope, batch_limit, recorded_at) VALUES ('T', 'REASK', 'i#s', now(), 'ANY', 1, now())`,
		"reask with a zero limit":     `INSERT INTO transport_fulfillment.delivery_dispatch_ask (tenant_id, kind, asked_by, asked_at, scope, batch_limit, recorded_at) VALUES ('T', 'REASK', 'i#s', now(), 'ALL', 0, now())`,
		"unknown kind":                `INSERT INTO transport_fulfillment.delivery_dispatch_ask (tenant_id, kind, asked_by, asked_at, scope, batch_limit, recorded_at) VALUES ('T', 'OTHER', 'i#s', now(), 'ALL', 1, now())`,
	} {
		_, err := pool.Exec(ctx, statement)
		wantCheckViolation(t, err, "delivery_dispatch_ask_shape")
	}
	_, err = pool.Exec(ctx, `INSERT INTO transport_fulfillment.delivery_dispatch_ask (tenant_id, kind, asked_by, asked_at, scope, batch_limit, recorded_at) VALUES ('T', 'REASK', ' ', now(), 'ALL', 1, now())`)
	wantCheckViolation(t, err, "delivery_dispatch_ask_not_blank")
}

// TestTheDeliveryDispatchAskLedgerRefusesToRunOutsideATransaction 证问询册的写口无环境事务即拒：补拍的记录要与执行器那一拍同笔，
// 事务外写进去就可能留下一次没有效果的按下。断言指名 ErrTransactionRequired；没有操作者的按下也不记。
func TestTheDeliveryDispatchAskLedgerRefusesToRunOutsideATransaction(t *testing.T) {
	asks, db, _ := newDeliveryDispatchAsks(t)
	record := ports.DeliveryDispatchAskRecord{
		TenantID: deliveryValue(t, domain.NewTenantID, "SYN-TENANT-ASK"), Kind: ports.DeliveryDispatchAskReask,
		AskedBy: operatorDB(t, operatorADB), AskedAt: time.Now(), Scope: "ALL", Limit: 10,
	}
	if err := asks.RecordAsk(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("RecordAsk 无事务应返回 ErrTransactionRequired，实得：%v", err)
	}
	record.AskedBy = domain.RegistrantReference{}
	mustWithinDeliveryTransaction(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if err := asks.RecordAsk(txCtx, record); err == nil {
			t.Error("没有操作者的按下不该记")
		}
		return nil
	})
}
