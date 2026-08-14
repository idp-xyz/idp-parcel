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
	journeyStartedAt = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	scheduleDeparts  = time.Date(2026, 8, 12, 20, 0, 0, 0, time.UTC)
	reservedAt       = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	reservedUntil    = time.Date(2026, 8, 12, 20, 0, 0, 0, time.UTC)
)

func TestAnAlternateJourneyRoundTripsAndSecondWriterGetsAlreadyStarted(t *testing.T) {
	journeys, _, _, transactor, _ := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()

	record := formedJourney(t, "tenant-a", domain.AlternateJourneyPurpose, domain.ServiceDispositionDecision, "journey-alt-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := journeys.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.AlternateJourneySaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := journeys.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("读回失败：err=%v exists=%v", err, exists)
	}
	if found.Journey.Journey().String() != "journey-alt-1" ||
		found.Journey.Purpose() != domain.AlternateJourneyPurpose ||
		found.Journey.RegulatoryOrigin() ||
		len(found.Journey.Members()) != 1 ||
		found.ContentDigest != record.ContentDigest {
		t.Fatalf("替代旅程往返变形：digest=%q purpose=%s members=%d",
			found.ContentDigest, found.Journey.Purpose(), len(found.Journey.Members()))
	}

	returned := formedJourney(t, "tenant-a", domain.ReturnJourneyPurpose, domain.RegulatoryDispositionDecision, "journey-return-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := journeys.Save(txCtx, returned)
		if err != nil {
			return err
		}
		if outcome != ports.AlternateJourneySaved {
			t.Fatalf("return save = %d", outcome)
		}
		return nil
	})
	foundReturn, exists, err := journeys.FindByKey(ctx, returned.Key)
	if err != nil || !exists || !foundReturn.Journey.RegulatoryOrigin() {
		t.Fatalf("退运往返失败：exists=%v regulatory=%v err=%v", exists, foundReturn.Journey.RegulatoryOrigin(), err)
	}

	second := record
	second.ContentDigest = "digest-other"
	var outcome ports.AlternateJourneySaveOutcome
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := journeys.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, found, err := journeys.FindByKey(txCtx, record.Key)
		if err != nil || !found || winner.ContentDigest != record.ContentDigest {
			t.Fatalf("同事务读回赢家失败：found=%v digest=%q err=%v", found, winner.ContentDigest, err)
		}
		return nil
	})
	if outcome != ports.AlternateJourneyAlreadyStarted {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestAScheduleRoundTripsAndSecondWriterGetsAlreadyEstablished(t *testing.T) {
	_, schedules, _, transactor, _ := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()

	record := formedSchedule(t, "tenant-a", "schedule-1")
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := schedules.Save(txCtx, record)
		if err != nil {
			return err
		}
		if outcome != ports.ScheduleSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := schedules.FindByKey(ctx, record.Key)
	if err != nil || !exists {
		t.Fatalf("班次读回失败：err=%v exists=%v", err, exists)
	}
	if found.Schedule.Direction() != "CN-US" ||
		!found.Schedule.DepartsAt().Equal(scheduleDeparts) ||
		found.ContentDigest != record.ContentDigest {
		t.Fatalf("班次往返变形：direction=%q digest=%q", found.Schedule.Direction(), found.ContentDigest)
	}

	second := record
	second.ContentDigest = "digest-other"
	var outcome ports.ScheduleSaveOutcome
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := schedules.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, found, err := schedules.FindByKey(txCtx, record.Key)
		if err != nil || !found || winner.ContentDigest != record.ContentDigest {
			t.Fatalf("同事务读回赢家失败：found=%v digest=%q err=%v", found, winner.ContentDigest, err)
		}
		return nil
	})
	if outcome != ports.ScheduleAlreadyEstablished {
		t.Fatalf("第二份写入结果 = %d", outcome)
	}
}

func TestACapacityPoolRoundTripsReservationsAndReplaceDoesNotRewriteIdentity(t *testing.T) {
	_, _, pools, transactor, _ := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()

	empty := establishedPoolRecord(t, "tenant-a", "pool-1", 100)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := pools.Save(txCtx, empty)
		if err != nil {
			return err
		}
		if outcome != ports.PoolSaved {
			t.Fatalf("save outcome = %d", outcome)
		}
		return nil
	})

	found, exists, err := pools.FindByKey(ctx, empty.Key)
	if err != nil || !exists || found.Pool.Capacity() != 100 || len(found.Pool.ReservationSnapshots()) != 0 {
		t.Fatalf("空池往返失败：exists=%v capacity=%d reservations=%d err=%v",
			exists, found.Pool.Capacity(), len(found.Pool.ReservationSnapshots()), err)
	}

	reserved, err := found.Pool.Reserve(
		deliveryValue(t, domain.NewCapacityReservationReference, "reservation-1"),
		60, reservedUntil, reservedAt,
	)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	reserved, err = reserved.Release(
		deliveryValue(t, domain.NewCapacityReservationReference, "reservation-1"),
		20, reservedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	reserved, err = reserved.Consume(
		deliveryValue(t, domain.NewCapacityReservationReference, "reservation-1"),
		30,
		deliveryValue(t, domain.NewLoadAssignmentReference, "assignment-1"),
		reservedAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	converted := found
	converted.Pool = reserved
	converted.ContentDigest = "digest-converted"
	converted.RecordedAt = reservedAt.Add(3 * time.Hour)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := pools.Replace(txCtx, converted)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("Replace 答 false")
		}
		return nil
	})

	after, _, err := pools.FindByKey(ctx, empty.Key)
	if err != nil {
		t.Fatalf("转换后读回：%v", err)
	}
	reservation, ok := after.Pool.ReservationFor(deliveryValue(t, domain.NewCapacityReservationReference, "reservation-1"))
	if !ok {
		t.Fatal("预占没有落库")
	}
	qty, released, consumed := reservation.Quantities()
	if qty != 60 || released != 20 || consumed != 30 {
		t.Fatalf("三量 = %d/%d/%d, want 60/20/30", qty, released, consumed)
	}
	if after.Pool.AvailableAt(reservedUntil.Add(time.Hour)) != 70 {
		t.Fatalf("到期可用 = %d, want 70", after.Pool.AvailableAt(reservedUntil.Add(time.Hour)))
	}
	if after.Pool.Capacity() != 100 || after.Pool.Schedule().String() != "schedule-1" {
		t.Fatal("Replace 改写了池身份")
	}

	mutated, err := domain.RehydrateCapacityPool(domain.CapacityPoolSpec{
		TenantID: empty.Key.TenantID,
		Pool:     empty.Key.Pool,
		Schedule: deliveryValue(t, domain.NewScheduleReference, "schedule-forged"),
		Unit:     deliveryValue(t, domain.NewQuantityUnitReference, "lb"),
		Capacity: 1,
	}, after.Pool.ReservationSnapshots())
	if err != nil {
		t.Fatalf("伪造身份：%v", err)
	}
	forged := after
	forged.Pool = mutated
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := pools.Replace(txCtx, forged)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("伪造身份的 Replace 答 false")
		}
		return nil
	})
	again, _, err := pools.FindByKey(ctx, empty.Key)
	if err != nil {
		t.Fatalf("伪造后读回：%v", err)
	}
	if again.Pool.Capacity() != 100 ||
		again.Pool.Schedule().String() != "schedule-1" ||
		again.Pool.Unit().String() != "kg" {
		t.Fatal("Replace 把班次/单位/容量写进了库")
	}

	missing := establishedPoolRecord(t, "tenant-a", "pool-missing", 50)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		ok, err := pools.Replace(txCtx, missing)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("没有可转换的池却答 true")
		}
		return nil
	})
}

func TestJourneyScheduleCapacityRecordsAreInvisibleAcrossTenants(t *testing.T) {
	journeys, schedules, pools, transactor, _ := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()

	journey := formedJourney(t, "tenant-a", domain.AlternateJourneyPurpose, domain.ServiceDispositionDecision, "journey-alt-1")
	schedule := formedSchedule(t, "tenant-a", "schedule-1")
	pool := establishedPoolRecord(t, "tenant-a", "pool-1", 100)
	mustWithinDeliveryTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		if _, err := journeys.Save(txCtx, journey); err != nil {
			return err
		}
		if _, err := schedules.Save(txCtx, schedule); err != nil {
			return err
		}
		_, err := pools.Save(txCtx, pool)
		return err
	})

	other := deliveryValue(t, domain.NewTenantID, "tenant-b")
	if _, exists, err := journeys.FindByKey(ctx, ports.AlternateJourneyKey{
		TenantID: other, Original: journey.Key.Original, Purpose: journey.Key.Purpose, Basis: journey.Key.Basis,
	}); err != nil || exists {
		t.Errorf("另一个租户读到了替代旅程：exists=%v err=%v", exists, err)
	}
	if _, exists, err := schedules.FindByKey(ctx, ports.ScheduleKey{TenantID: other, Schedule: schedule.Key.Schedule}); err != nil || exists {
		t.Errorf("另一个租户读到了班次：exists=%v err=%v", exists, err)
	}
	if _, exists, err := pools.FindByKey(ctx, ports.CapacityPoolKey{TenantID: other, Pool: pool.Key.Pool}); err != nil || exists {
		t.Errorf("另一个租户读到了容量池：exists=%v err=%v", exists, err)
	}
}

func TestJourneyScheduleCapacityWritesRefuseToRunOutsideATransaction(t *testing.T) {
	journeys, schedules, pools, _, _ := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()

	journey := formedJourney(t, "tenant-a", domain.AlternateJourneyPurpose, domain.ServiceDispositionDecision, "journey-alt-1")
	if _, err := journeys.Save(ctx, journey); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 替代旅程：%v", err)
	}
	schedule := formedSchedule(t, "tenant-a", "schedule-1")
	if _, err := schedules.Save(ctx, schedule); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 班次：%v", err)
	}
	pool := establishedPoolRecord(t, "tenant-a", "pool-1", 100)
	if _, err := pools.Save(ctx, pool); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Save 容量池：%v", err)
	}
	if _, err := pools.Replace(ctx, pool); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务 Replace 容量池：%v", err)
	}
}

func TestJourneyScheduleCapacityRollbackLeavesNothingBehind(t *testing.T) {
	journeys, schedules, pools, transactor, _ := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()
	rollback := errors.New("回滚")
	journey := formedJourney(t, "tenant-a", domain.AlternateJourneyPurpose, domain.ServiceDispositionDecision, "journey-alt-1")
	schedule := formedSchedule(t, "tenant-a", "schedule-1")
	pool := establishedPoolRecord(t, "tenant-a", "pool-1", 100)

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := journeys.Save(txCtx, journey); err != nil {
			return err
		}
		if _, err := schedules.Save(txCtx, schedule); err != nil {
			return err
		}
		if _, err := pools.Save(txCtx, pool); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := journeys.FindByKey(ctx, journey.Key); err != nil || exists {
		t.Errorf("回滚后替代旅程仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := schedules.FindByKey(ctx, schedule.Key); err != nil || exists {
		t.Errorf("回滚后班次仍在：exists=%v err=%v", exists, err)
	}
	if _, exists, err := pools.FindByKey(ctx, pool.Key); err != nil || exists {
		t.Errorf("回滚后容量池仍在：exists=%v err=%v", exists, err)
	}
}

func TestJourneyScheduleCapacityCheckConstraintsRejectImpossibleRows(t *testing.T) {
	_, _, _, _, pool := newJourneyScheduleCapacityStores(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.alternate_journey
			(tenant_id, original_journey, purpose, basis, journey_id, basis_kind,
			 members, started_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'journey-1', 'ALTERNATE', 'DISPOSITION/d1', 'journey-1',
		         'SERVICE_DISPOSITION', '["parcel-1"]', now(), 'd', now())`); err == nil {
		t.Fatal("一行「新身份等于原旅程」溜进了替代旅程库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.alternate_journey
			(tenant_id, original_journey, purpose, basis, journey_id, basis_kind,
			 members, started_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'journey-1', 'ALTERNATE', 'DISPOSITION/d1', 'journey-2',
		         'SERVICE_DISPOSITION', '[]', now(), 'd', now())`); err == nil {
		t.Fatal("一行「空成员」溜进了替代旅程库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.alternate_journey
			(tenant_id, original_journey, purpose, basis, journey_id, basis_kind,
			 members, started_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 'journey-1', 'ALTERNATE', 'DISPOSITION/d1', 'journey-2',
		         'SERVICE_DISPOSITION', NULL, now(), 'd', now())`); err == nil {
		t.Fatal("一行「成员列为 NULL」按 jsonb 三值缝溜进了替代旅程库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_schedule
			(tenant_id, schedule_id, direction, departs_at, content_digest, recorded_at)
		 VALUES ('tenant-a', 's-bad-1', '   ', now(), 'd', now())`); err == nil {
		t.Fatal("一行「空方向」溜进了班次库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ('tenant-a', 'p-bad-1', 's-1', 'kg', 0, 'd', now())`); err == nil {
		t.Fatal("一行「容量为零」溜进了容量池库")
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.capacity_pool
			(tenant_id, pool_id, schedule_id, unit_ref, capacity, content_digest, recorded_at)
		 VALUES ('tenant-a', 'p-ok', 's-1', 'kg', 100, 'd', now())`); err != nil {
		t.Fatalf("探针父行插不进去：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.capacity_reservation
			(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed)
		 VALUES ('tenant-a', 'p-ok', 'r-1', 10, now(), 6, 5)`); err == nil {
		t.Fatal("一行「释放+消耗超过预占」溜进了预占子表")
	}
}

func newJourneyScheduleCapacityStores(t *testing.T) (
	*adapter.AlternateJourneys,
	*adapter.TransportSchedules,
	*adapter.CapacityPools,
	bentoapp.Transactor,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	journeys, err := adapter.NewAlternateJourneys(db)
	if err != nil {
		t.Fatalf("构造替代旅程库：%v", err)
	}
	schedules, err := adapter.NewTransportSchedules(db)
	if err != nil {
		t.Fatalf("构造班次库：%v", err)
	}
	pools, err := adapter.NewCapacityPools(db)
	if err != nil {
		t.Fatalf("构造容量池库：%v", err)
	}
	return journeys, schedules, pools, db.Transactor(), pool
}

func formedJourney(
	t *testing.T,
	tenant string,
	purpose domain.JourneyPurpose,
	kind domain.DispositionBasisKind,
	journeyID string,
) ports.AlternateJourneyRecord {
	t.Helper()
	journey, err := domain.FormAlternateJourney(domain.AlternateJourneySpec{
		TenantID:        deliveryValue(t, domain.NewTenantID, tenant),
		Journey:         deliveryValue(t, domain.NewJourneyReference, journeyID),
		Purpose:         purpose,
		OriginalJourney: deliveryValue(t, domain.NewJourneyReference, "journey-original-1"),
		BasisKind:       kind,
		Basis:           deliveryValue(t, domain.NewDispositionBasisReference, "DISPOSITION/decision-1"),
		Members:         []domain.CarriedObjectReference{deliveryValue(t, domain.NewCarriedObjectReference, "parcel-1")},
		StartedAt:       journeyStartedAt,
	})
	if err != nil {
		t.Fatalf("构造替代旅程：%v", err)
	}
	return ports.AlternateJourneyRecord{
		Key: ports.AlternateJourneyKey{
			TenantID: journey.TenantID(),
			Original: journey.OriginalJourney(),
			Purpose:  journey.Purpose(),
			Basis:    journey.Basis(),
		},
		ContentDigest: "digest-" + journeyID,
		Journey:       journey,
		RecordedAt:    journeyStartedAt,
	}
}

func formedSchedule(t *testing.T, tenant, id string) ports.ScheduleRecord {
	t.Helper()
	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:  deliveryValue(t, domain.NewTenantID, tenant),
		Schedule:  deliveryValue(t, domain.NewScheduleReference, id),
		Direction: "CN-US",
		DepartsAt: scheduleDeparts,
	})
	if err != nil {
		t.Fatalf("构造班次：%v", err)
	}
	return ports.ScheduleRecord{
		Key:           ports.ScheduleKey{TenantID: schedule.TenantID(), Schedule: schedule.Schedule()},
		ContentDigest: "digest-" + id,
		Schedule:      schedule,
		RecordedAt:    scheduleDeparts,
	}
}

func establishedPoolRecord(t *testing.T, tenant, id string, capacity int64) ports.CapacityPoolRecord {
	t.Helper()
	pool, err := domain.EstablishCapacityPool(domain.CapacityPoolSpec{
		TenantID: deliveryValue(t, domain.NewTenantID, tenant),
		Pool:     deliveryValue(t, domain.NewCapacityPoolReference, id),
		Schedule: deliveryValue(t, domain.NewScheduleReference, "schedule-1"),
		Unit:     deliveryValue(t, domain.NewQuantityUnitReference, "kg"),
		Capacity: capacity,
	})
	if err != nil {
		t.Fatalf("构造容量池：%v", err)
	}
	return ports.CapacityPoolRecord{
		Key:           ports.CapacityPoolKey{TenantID: deliveryValue(t, domain.NewTenantID, tenant), Pool: pool.Pool()},
		ContentDigest: "digest-" + id,
		Pool:          pool,
		RecordedAt:    reservedAt,
	}
}
