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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证履约段采用运输委托这本册（迁移 TF 0035，ADR-0204）与取消订舱的写口：采用整份往返、同段第二份
// 答已登记不覆盖、段或委托不在册时外键挡住；采用编排接真段册、真委托册与真采用册时，采用把委托标为开始、此后取消委托答运输
// 已开始；取消订舱只落一次取消、不改落册时刻。夹具全为合成登记（S 级）。

type adoptionStores struct {
	segments    *adapter.FulfillmentSegments
	commissions *adapter.TransportCommissions
	bookings    *adapter.BookingRequests
	adoptions   *adapter.SegmentCommissionAdoptions
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newAdoptionStores(t *testing.T) adoptionStores {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	segments, err := adapter.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	commissions, err := adapter.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	bookings, err := adapter.NewBookingRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := adapter.NewSegmentCommissionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	return adoptionStores{segments: segments, commissions: commissions, bookings: bookings, adoptions: adoptions, transactor: db.Transactor(), pool: pool}
}

// seed 立段 SEG-ADOPT（首个参与 segmentEnteredAtFixture）、提交 commission-1（tenant-1，服务方 partner-1）并在它下面订一舱。
func (stores adoptionStores) seed(t *testing.T, ctx context.Context) {
	t.Helper()
	mustSaveSegment(t, stores.transactor, ctx, stores.segments, segmentRecord(t, "SEG-ADOPT", activeMember(t, "parcel-1")))
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		if _, err := stores.commissions.Save(txCtx, submittedCommission(t, "tenant-1", "commission-1")); err != nil {
			return err
		}
		_, err := stores.bookings.Save(txCtx, submittedBookingRecord(t, "tenant-1", "booking-1"))
		return err
	})
}

type fixedAdoptionClock struct{ at time.Time }

func (clock fixedAdoptionClock) Now() time.Time { return clock.at }

// Covers: `AT-TF-181`、`AT-TF-182` 的真库面（ADR-0204 决定二、三）——采用编排接真册：采用成立、委托以段的首个参与标为开始，
// 采用行带着这一段的开始依据；随后取消委托答运输已开始、委托行不被取消；同段原样再交答已有采用、不多一行。
func TestAWiredAdoptionStartsTheCommissionAndCancellationThenAnswersAlreadyStarted(t *testing.T) {
	stores := newAdoptionStores(t)
	ctx := t.Context()
	stores.seed(t, ctx)
	clock := fixedAdoptionClock{at: segmentEnteredAtFixture.Add(24 * time.Hour)}
	adopter := application.NewAdoptSegmentCommissionHandler(application.AdoptSegmentCommissionDeps{
		Adoptions: stores.adoptions, Segments: stores.segments, Commissions: stores.commissions, Clock: clock,
	})
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")
	command := application.AdoptSegmentCommissionCommand{TenantID: tenant, Segment: "SEG-ADOPT", Commission: "commission-1", Provider: "partner-1"}

	var result application.CommissionAdoptionResult
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = adopter.Adopt(txCtx, command)
		return err
	})
	if result.Outcome() != application.CommissionAdopted {
		t.Fatalf("采用 = %q", result.Outcome())
	}
	var startBasis string
	var startedAt time.Time
	if err := stores.pool.QueryRow(ctx,
		`SELECT c.started_basis, c.started_at FROM transport_fulfillment.transport_commission c
		  WHERE c.tenant_id = 'tenant-1' AND c.commission_id = 'commission-1'`).Scan(&startBasis, &startedAt); err != nil {
		t.Fatalf("读委托行：%v", err)
	}
	if startBasis != "OFFSITE-PICKUP/parcel-1" || !startedAt.Equal(segmentEnteredAtFixture) {
		t.Fatalf("委托的开始 = (%s, %s)，want 段的首个参与", startBasis, startedAt)
	}
	found, present, err := stores.adoptions.FindBySegment(ctx, ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: deliveryValue(t, domain.NewFulfillmentSegmentReference, "SEG-ADOPT")})
	if err != nil || !present {
		t.Fatalf("读回采用：%v %v", present, err)
	}
	if basis, at := found.Adoption.Started(); found.Adoption.Commission().String() != "commission-1" || found.Adoption.Provider().String() != "partner-1" ||
		basis.String() != "OFFSITE-PICKUP/parcel-1" || !at.Equal(segmentEnteredAtFixture) || !found.RecordedAt.Equal(clock.at) {
		t.Fatalf("采用往返 = %+v", found)
	}

	handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions: stores.commissions, Bookings: stores.bookings, BookingCancellations: stores.bookings, Clock: clock,
	})
	var cancel application.CommissionResult
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		var err error
		cancel, err = handler.CancelCommission(txCtx, application.CancelCommissionCommand{TenantID: tenant, Commission: "commission-1", CancelledAt: clock.at})
		return err
	})
	if cancel.Outcome().String() != "TRANSPORT_ALREADY_STARTED" {
		t.Fatalf("采用之后取消委托 = %q", cancel.Outcome())
	}
	var cancelled *time.Time
	if err := stores.pool.QueryRow(ctx,
		`SELECT cancelled_at FROM transport_fulfillment.transport_commission WHERE tenant_id = 'tenant-1' AND commission_id = 'commission-1'`).Scan(&cancelled); err != nil || cancelled != nil {
		t.Fatalf("已开始的委托被取消了：%v %v", cancelled, err)
	}

	// 订舱在委托开始之前提出：随那一程开始而可能已被消耗，取消答运输已开始。
	var bookingCancel application.CommissionResult
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		var err error
		bookingCancel, err = handler.CancelBooking(txCtx, application.CancelBookingCommand{TenantID: tenant, Booking: "booking-1", CancelledAt: clock.at})
		return err
	})
	if bookingCancel.Outcome().String() != "TRANSPORT_ALREADY_STARTED" {
		t.Fatalf("开始之前提出的订舱取消 = %q", bookingCancel.Outcome())
	}

	var replay application.CommissionAdoptionResult
	mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
		var err error
		replay, err = adopter.Adopt(txCtx, command)
		return err
	})
	var rows int
	if err := stores.pool.QueryRow(ctx, `SELECT count(*) FROM transport_fulfillment.segment_commission_adoption`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if replay.Outcome() != application.CommissionAdoptionExisting || rows != 1 {
		t.Fatalf("原样再交 = %q，采用 %d 行", replay.Outcome(), rows)
	}
}

// Covers: 迁移 TF 0035 的库面——同段第二份采用答已登记、先到者一字不动；段不在册或委托不在册时外键挡住（编排之外的第二道）。
func TestTheAdoptionRegisterKeepsOneAdoptionPerSegmentAndNeedsBothEnds(t *testing.T) {
	stores := newAdoptionStores(t)
	ctx := t.Context()
	stores.seed(t, ctx)
	tenant := deliveryValue(t, domain.NewTenantID, "tenant-1")
	record := func(segment, commission, digest string) ports.SegmentCommissionAdoptionRecord {
		t.Helper()
		adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
			TenantID:   tenant,
			Segment:    deliveryValue(t, domain.NewFulfillmentSegmentReference, segment),
			Commission: deliveryValue(t, domain.NewTransportCommissionReference, commission),
			Provider:   deliveryValue(t, domain.NewServiceProviderReference, "partner-1"),
			StartBasis: deliveryValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/parcel-1"),
			StartedAt:  segmentEnteredAtFixture,
		})
		if err != nil {
			t.Fatal(err)
		}
		return ports.SegmentCommissionAdoptionRecord{
			Key:           ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: adoption.Segment()},
			ContentDigest: digest,
			Adoption:      adoption,
			RecordedAt:    segmentEnteredAtFixture.Add(time.Hour),
		}
	}
	save := func(record ports.SegmentCommissionAdoptionRecord) (ports.AdoptionSaveOutcome, error) {
		var outcome ports.AdoptionSaveOutcome
		err := stores.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = stores.adoptions.Save(txCtx, record)
			return err
		})
		return outcome, err
	}
	if outcome, err := save(record("SEG-ADOPT", "commission-1", "digest-first")); err != nil || outcome != ports.AdoptionSaved {
		t.Fatalf("首份 = %d %v", outcome, err)
	}
	if outcome, err := save(record("SEG-ADOPT", "commission-1", "digest-second")); err != nil || outcome != ports.AdoptionAlreadyRegistered {
		t.Fatalf("同段第二份 = %d %v", outcome, err)
	}
	found, _, err := stores.adoptions.FindBySegment(ctx, record("SEG-ADOPT", "commission-1", "").Key)
	if err != nil || found.ContentDigest != "digest-first" {
		t.Fatalf("先到者被改：%q %v", found.ContentDigest, err)
	}
	if _, err := save(record("SEG-NOWHERE", "commission-1", "digest-x")); err == nil {
		t.Fatal("段不在册的采用落进了库")
	}
	mustSaveSegment(t, stores.transactor, ctx, stores.segments, segmentRecord(t, "SEG-OTHER", activeMember(t, "parcel-2")))
	if _, err := save(record("SEG-OTHER", "commission-9", "digest-y")); err == nil {
		t.Fatal("委托不在册的采用落进了库")
	}
}

// TestAdoptionAndBookingCancellationWritesRefuseToRunOutsideATransaction 证两个写口无环境事务即拒：采用与委托的开始、取消与它
// 所在那一笔都要同笔落，事务边界归装配点。断言指名 ErrTransactionRequired，不把「拒得对」与别的失败混成一格。
func TestAdoptionAndBookingCancellationWritesRefuseToRunOutsideATransaction(t *testing.T) {
	stores := newAdoptionStores(t)
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
		ContentDigest: "digest",
		Adoption:      adoption,
		RecordedAt:    segmentEnteredAtFixture,
	}
	if _, err := stores.adoptions.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("采用写口无事务应返回 ErrTransactionRequired，实得：%v", err)
	}
	booking := submittedBookingRecord(t, "tenant-1", "booking-1")
	booking.Booking, err = booking.Booking.Cancel(bookingAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores.bookings.Replace(t.Context(), booking); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("取消订舱写口无事务应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: `AT-TF-185` 的真库面——取消订舱的写口只落一次取消：第二笔答 false、原取消时刻不变；落册时刻不随取消改写；订舱
// 不在册答 false。
func TestABookingCancellationLandsOnceAndKeepsTheRecordingTime(t *testing.T) {
	stores := newAdoptionStores(t)
	ctx := t.Context()
	stores.seed(t, ctx)
	stored, _, err := stores.bookings.FindByKey(ctx, submittedBookingRecord(t, "tenant-1", "booking-1").Key)
	if err != nil {
		t.Fatal(err)
	}
	replace := func(record ports.BookingRecord) bool {
		t.Helper()
		var ok bool
		mustWithinDeliveryTransaction(t, stores.transactor, ctx, func(txCtx context.Context) error {
			var err error
			ok, err = stores.bookings.Replace(txCtx, record)
			return err
		})
		return ok
	}
	cancelledOnce := stored
	cancelledOnce.Booking, err = stored.Booking.Cancel(bookingAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cancelledOnce.RecordedAt = bookingAt.Add(48 * time.Hour)
	if !replace(cancelledOnce) {
		t.Fatal("第一笔取消没落上")
	}
	cancelledTwice := stored
	cancelledTwice.Booking, _ = stored.Booking.Cancel(bookingAt.Add(2 * time.Hour))
	if replace(cancelledTwice) {
		t.Fatal("第二笔取消也落上了")
	}
	after, _, err := stores.bookings.FindByKey(ctx, stored.Key)
	if err != nil {
		t.Fatal(err)
	}
	if at, cancelled := after.Booking.Cancelled(); !cancelled || !at.Equal(bookingAt.Add(time.Hour)) || !after.RecordedAt.Equal(stored.RecordedAt) {
		t.Fatalf("取消后 = (%s, %v)，落册时刻 %s（原 %s）", at, cancelled, after.RecordedAt, stored.RecordedAt)
	}
	absent := cancelledOnce
	absent.Key = submittedBookingRecord(t, "tenant-1", "booking-9").Key
	if replace(absent) {
		t.Fatal("不在册的订舱答落上了")
	}
}
