package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	scheduleDepartsAt = time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	reserveAt         = time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)
	reserveValidUntil = time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
)

type scheduleStoreDouble struct {
	records map[string]ports.ScheduleRecord
	findErr error
}

func newScheduleStore() *scheduleStoreDouble {
	return &scheduleStoreDouble{records: map[string]ports.ScheduleRecord{}}
}

func scheduleKey(key ports.ScheduleKey) string {
	return key.TenantID.String() + "|" + key.Schedule.String()
}

func (double *scheduleStoreDouble) FindByKey(
	_ context.Context,
	key ports.ScheduleKey,
) (ports.ScheduleRecord, bool, error) {
	if double.findErr != nil {
		return ports.ScheduleRecord{}, false, double.findErr
	}
	record, found := double.records[scheduleKey(key)]
	return record, found, nil
}

func (double *scheduleStoreDouble) Save(
	_ context.Context,
	record ports.ScheduleRecord,
) (ports.ScheduleSaveOutcome, error) {
	if _, exists := double.records[scheduleKey(record.Key)]; exists {
		return ports.ScheduleAlreadyEstablished, nil
	}
	double.records[scheduleKey(record.Key)] = record
	return ports.ScheduleSaved, nil
}

type poolStoreDouble struct {
	records map[string]ports.CapacityPoolRecord
	findErr error
}

func newPoolStore() *poolStoreDouble {
	return &poolStoreDouble{records: map[string]ports.CapacityPoolRecord{}}
}

func poolKey(key ports.CapacityPoolKey) string {
	return key.TenantID.String() + "|" + key.Pool.String()
}

func (double *poolStoreDouble) FindByKey(
	_ context.Context,
	key ports.CapacityPoolKey,
) (ports.CapacityPoolRecord, bool, error) {
	if double.findErr != nil {
		return ports.CapacityPoolRecord{}, false, double.findErr
	}
	record, found := double.records[poolKey(key)]
	return record, found, nil
}

func (double *poolStoreDouble) Save(
	_ context.Context,
	record ports.CapacityPoolRecord,
) (ports.PoolSaveOutcome, error) {
	if _, exists := double.records[poolKey(record.Key)]; exists {
		return ports.PoolAlreadyEstablished, nil
	}
	double.records[poolKey(record.Key)] = record
	return ports.PoolSaved, nil
}

func (double *poolStoreDouble) Replace(
	_ context.Context,
	record ports.CapacityPoolRecord,
) (bool, error) {
	if _, exists := double.records[poolKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[poolKey(record.Key)] = record
	return true, nil
}

type consumptionHandoffDouble struct {
	intents []ports.CapacityConsumptionIntent
	err     error
}

func (double *consumptionHandoffDouble) HandOffCapacityConsumption(
	_ context.Context,
	intent ports.CapacityConsumptionIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type opportunityClock struct{ at time.Time }

func (clock opportunityClock) Now() time.Time { return clock.at }

type opportunityFixture struct {
	schedules *scheduleStoreDouble
	pools     *poolStoreDouble
	handoff   *consumptionHandoffDouble
	handler   *application.PrepareTransportOpportunityHandler
}

func newOpportunityFixture(t *testing.T) *opportunityFixture {
	t.Helper()
	fixture := &opportunityFixture{
		schedules: newScheduleStore(),
		pools:     newPoolStore(),
		handoff:   &consumptionHandoffDouble{},
	}
	fixture.handler = application.NewPrepareTransportOpportunityHandler(application.PrepareTransportOpportunityDeps{
		Schedules:  fixture.schedules,
		Pools:      fixture.pools,
		Downstream: fixture.handoff,
		Clock:      opportunityClock{at: reserveAt},
	})
	return fixture
}

func opportunityTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return tenant
}

func establishedPool(t *testing.T, fixture *opportunityFixture) {
	t.Helper()
	result, err := fixture.handler.EstablishPool(context.Background(), application.EstablishPoolCommand{
		TenantID: opportunityTenant(t),
		Pool:     "pool-1",
		Schedule: "schedule-1",
		Unit:     "kg",
		Capacity: 100,
	})
	if err != nil {
		t.Fatalf("establish pool: %v", err)
	}
	if result.Outcome() != application.PoolEstablished {
		t.Fatalf("outcome = %q", result.Outcome())
	}
}

func reserveCommand(t *testing.T, quantity int64) application.ReserveCapacityCommand {
	t.Helper()
	return application.ReserveCapacityCommand{
		TenantID:    opportunityTenant(t),
		Pool:        "pool-1",
		Reservation: "reservation-1",
		Quantity:    quantity,
		ValidUntil:  reserveValidUntil,
		At:          reserveAt,
	}
}

// 班次是执行体：建立幂等分重放/冲突（班次存在≠容量可用——池是另一入口单独建立）。
func TestAScheduleEstablishesOnce(t *testing.T) {
	fixture := newOpportunityFixture(t)
	command := application.EstablishScheduleCommand{
		TenantID:  opportunityTenant(t),
		Schedule:  "schedule-1",
		Direction: "US-WEST/EXPORT",
		DepartsAt: scheduleDepartsAt,
	}

	first, err := fixture.handler.EstablishSchedule(context.Background(), command)
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	if first.Outcome() != application.ScheduleEstablished {
		t.Fatalf("outcome = %q", first.Outcome())
	}

	replay, err := fixture.handler.EstablishSchedule(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.ScheduleExistingResult {
		t.Fatalf("outcome = %q", replay.Outcome())
	}

	flipped := command
	flipped.Direction = "US-EAST/EXPORT"
	conflict, err := fixture.handler.EstablishSchedule(context.Background(), flipped)
	if err != nil {
		t.Fatalf("conflict: %v", err)
	}
	if conflict.Outcome() != application.ScheduleConflict {
		t.Fatalf("outcome = %q", conflict.Outcome())
	}
}

// Covers: `AT-TF-031` 的编排面——超硬上限拒绝且带当时余量（拒绝带答案）；同引用同量
// 重放返原不重扣、同引用异量冲突。
func TestOverReservationIsRefusedWithTheRemainder(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)

	reserved, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60))
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if reserved.Outcome() != application.CapacityReserved {
		t.Fatalf("outcome = %q", reserved.Outcome())
	}

	t.Run("a replay with the same quantity returns the original", func(t *testing.T) {
		replay, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.ReservationExistingResult {
			t.Fatalf("outcome = %q（重放不重扣）", replay.Outcome())
		}
		record, _ := replay.Pool()
		if available := record.Pool.AvailableAt(reserveAt); available != 40 {
			t.Fatalf("available = %d, want 40（重放扣了第二次）", available)
		}
	})

	t.Run("a different quantity under the same reference is a conflict", func(t *testing.T) {
		flipped := reserveCommand(t, 70)
		result, err := fixture.handler.Reserve(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict reserve: %v", err)
		}
		if result.Outcome() != application.ReservationConflict {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})

	t.Run("exceeding the hard limit carries the remainder", func(t *testing.T) {
		over := reserveCommand(t, 50)
		over.Reservation = "reservation-2"
		result, err := fixture.handler.Reserve(context.Background(), over)
		if err != nil {
			t.Fatalf("over reserve: %v", err)
		}
		if result.Outcome() != application.CapacityExceededOutcome {
			t.Fatalf("outcome = %q, want CAPACITY_EXCEEDED", result.Outcome())
		}
		// 余量锚定本夹具：容量 100，已占 60 → 余 40。
		if result.AvailableQuantity() != 40 {
			t.Fatalf("available = %d, want 40（拒绝要带着答案）", result.AvailableQuantity())
		}
	})
}

// 释放/消耗/过期分格：消耗成功交装载分配链意图；过期与守恒撞线各归业务负向一格
// （领域哨兵透出）；无装载分配依据的消耗未受理。
func TestConversionsSplitByTheirSentinels(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	if _, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60)); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	released, err := fixture.handler.Release(context.Background(), application.ReleaseCapacityCommand{
		TenantID:    opportunityTenant(t),
		Pool:        "pool-1",
		Reservation: "reservation-1",
		Quantity:    20,
		At:          reserveAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Outcome() != application.CapacityReleased {
		t.Fatalf("outcome = %q", released.Outcome())
	}

	consumed, err := fixture.handler.Consume(context.Background(), application.ConsumeCapacityCommand{
		TenantID:    opportunityTenant(t),
		Pool:        "pool-1",
		Reservation: "reservation-1",
		Quantity:    40,
		Assignment:  "load-assignment-1",
		At:          reserveAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if consumed.Outcome() != application.CapacityConsumed {
		t.Fatalf("outcome = %q", consumed.Outcome())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1（消耗交装载分配链）", len(fixture.handoff.intents))
	}

	t.Run("a second conversion over the reserved range hits conservation", func(t *testing.T) {
		result, err := fixture.handler.Release(context.Background(), application.ReleaseCapacityCommand{
			TenantID:    opportunityTenant(t),
			Pool:        "pool-1",
			Reservation: "reservation-1",
			Quantity:    1,
			At:          reserveAt.Add(3 * time.Hour),
		})
		if err != nil {
			t.Fatalf("second release: %v", err)
		}
		if result.Outcome() != application.ReservationOverdrawnOutcome {
			t.Fatalf("outcome = %q, want RESERVATION_OVERDRAWN（同一范围不得二次转换）", result.Outcome())
		}
	})

	t.Run("an expired reservation cannot convert", func(t *testing.T) {
		expiredFixture := newOpportunityFixture(t)
		establishedPool(t, expiredFixture)
		if _, err := expiredFixture.handler.Reserve(context.Background(), reserveCommand(t, 30)); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		result, err := expiredFixture.handler.Consume(context.Background(), application.ConsumeCapacityCommand{
			TenantID:    opportunityTenant(t),
			Pool:        "pool-1",
			Reservation: "reservation-1",
			Quantity:    10,
			Assignment:  "load-assignment-1",
			At:          reserveValidUntil.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("expired consume: %v", err)
		}
		if result.Outcome() != application.ReservationExpiredOutcome {
			t.Fatalf("outcome = %q, want RESERVATION_EXPIRED", result.Outcome())
		}
	})

	t.Run("consumption without a load assignment is not accepted", func(t *testing.T) {
		result, err := fixture.handler.Consume(context.Background(), application.ConsumeCapacityCommand{
			TenantID:    opportunityTenant(t),
			Pool:        "pool-1",
			Reservation: "reservation-1",
			Quantity:    1,
			Assignment:  " ",
			At:          reserveAt.Add(3 * time.Hour),
		})
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if result.Outcome() != application.OpportunityNotAccepted {
			t.Fatalf("outcome = %q（没有装载分配依据的消耗无从谈起）", result.Outcome())
		}
	})

	t.Run("an unknown reservation is not accepted", func(t *testing.T) {
		result, err := fixture.handler.Release(context.Background(), application.ReleaseCapacityCommand{
			TenantID:    opportunityTenant(t),
			Pool:        "pool-1",
			Reservation: "reservation-9",
			Quantity:    1,
			At:          reserveAt.Add(3 * time.Hour),
		})
		if err != nil {
			t.Fatalf("release: %v", err)
		}
		if result.Outcome() != application.OpportunityNotAccepted {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})
}

// 恢复纪律：库故障各归未决；消耗投递失败不翻结果；原因集封闭。
func TestOpportunityRecoveryDiscipline(t *testing.T) {
	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		fixture := newOpportunityFixture(t)
		fixture.schedules.findErr = errors.New("store down")
		result, err := fixture.handler.EstablishSchedule(context.Background(), application.EstablishScheduleCommand{
			TenantID:  opportunityTenant(t),
			Schedule:  "schedule-1",
			Direction: "US-WEST/EXPORT",
			DepartsAt: scheduleDepartsAt,
		})
		if err != nil {
			t.Fatalf("establish: %v", err)
		}
		if result.UndecidedReason() != application.ScheduleStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		poolFixture := newOpportunityFixture(t)
		poolFixture.pools.findErr = errors.New("store down")
		result, err = poolFixture.handler.Reserve(context.Background(), reserveCommand(t, 10))
		if err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if result.UndecidedReason() != application.PoolStoreUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a consumption handoff failure keeps the outcome", func(t *testing.T) {
		fixture := newOpportunityFixture(t)
		establishedPool(t, fixture)
		if _, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 30)); err != nil {
			t.Fatalf("reserve: %v", err)
		}
		fixture.handoff.err = errors.New("downstream unavailable")
		result, err := fixture.handler.Consume(context.Background(), application.ConsumeCapacityCommand{
			TenantID:    opportunityTenant(t),
			Pool:        "pool-1",
			Reservation: "reservation-1",
			Quantity:    10,
			Assignment:  "load-assignment-1",
			At:          reserveAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if result.Outcome() != application.CapacityConsumed || result.ConsumptionHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果——消耗已经落定）", result.Outcome(), result.ConsumptionHandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.OpportunityUndecidedReason{
			application.ScheduleStoreUnavailable, application.PoolStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 2 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.OpportunityUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第三个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
