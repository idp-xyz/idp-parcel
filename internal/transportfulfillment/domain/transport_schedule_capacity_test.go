package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	reservedAt    = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	reservedUntil = time.Date(2026, 8, 12, 20, 0, 0, 0, time.UTC)
)

func establishedPool(t *testing.T, capacity int64) domain.CapacityPool {
	t.Helper()
	pool, err := domain.EstablishCapacityPool(domain.CapacityPoolSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Pool:     mustValue(t, domain.NewCapacityPoolReference, "pool-weight-1"),
		Schedule: mustValue(t, domain.NewScheduleReference, "schedule-1"),
		Unit:     mustValue(t, domain.NewQuantityUnitReference, "kg"),
		Capacity: capacity,
	})
	if err != nil {
		t.Fatalf("establish pool: %v", err)
	}
	return pool
}

func reservationRef(t *testing.T, value string) domain.CapacityReservationReference {
	t.Helper()
	return mustValue(t, domain.NewCapacityReservationReference, value)
}

// Covers: CONTEXT「具体班次」与「承运接受……不等于容量已经预占」——班次是执行体，
// 班次存在≠容量可用≠已订舱：类型上没有任何容量、预占或订舱字段（结构防线）；周期模板
// 不是班次，身份三件（方向/发运时间/引用）缺一不可（AT-TF-027 的身份半边）。
func TestAScheduleCarriesNoCapacityOrBooking(t *testing.T) {
	scheduleType := reflect.TypeOf(domain.TransportSchedule{})
	for index := 0; index < scheduleType.NumField(); index++ {
		name := strings.ToLower(scheduleType.Field(index).Name)
		for _, forbidden := range []string{"capacity", "reservation", "booking", "pool", "available"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("TransportSchedule 携带 %q——班次存在就能被读成容量可用或已订舱", scheduleType.Field(index).Name)
			}
		}
	}

	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:  mustValue(t, domain.NewTenantID, "tenant-1"),
		Schedule:  mustValue(t, domain.NewScheduleReference, "schedule-1"),
		Direction: "CN-US",
		DepartsAt: reservedAt.Add(12 * time.Hour),
	})
	if err != nil {
		t.Fatalf("form schedule: %v", err)
	}
	if schedule.Direction() != "CN-US" {
		t.Fatalf("direction = %q", schedule.Direction())
	}

	broken := map[string]func(*domain.TransportScheduleSpec){
		"no direction": func(spec *domain.TransportScheduleSpec) { spec.Direction = "  " },
		"no departure": func(spec *domain.TransportScheduleSpec) { spec.DepartsAt = time.Time{} },
		"no schedule":  func(spec *domain.TransportScheduleSpec) { spec.Schedule = domain.ScheduleReference{} },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := domain.TransportScheduleSpec{
				TenantID:  mustValue(t, domain.NewTenantID, "tenant-1"),
				Schedule:  mustValue(t, domain.NewScheduleReference, "schedule-x"),
				Direction: "CN-US",
				DepartsAt: reservedAt,
			}
			breakSpec(&spec)
			if _, err := domain.FormTransportSchedule(spec); !errors.Is(err, domain.ErrInvalidTransportSchedule) {
				t.Fatalf("error = %v, want ErrInvalidTransportSchedule", err)
			}
		})
	}
}

// Covers: `AT-TF-031`「请求触及物理硬容量 → 拒绝超出范围，不因人工批准而突破」与
// CONTEXT「只有当前有效的容量预占扣减可用容量」——池容量有限，超预占拒；同引用不
// 重复预占。计数锚定本夹具：容量 100 kg。
func TestReservationsDrawDownFinitePoolCapacity(t *testing.T) {
	pool := establishedPool(t, 100)

	pool, err := pool.Reserve(reservationRef(t, "reservation-1"), 60, reservedUntil, reservedAt)
	if err != nil {
		t.Fatalf("reserve 60: %v", err)
	}
	if available := pool.AvailableAt(reservedAt); available != 40 {
		t.Fatalf("available = %d, want 40", available)
	}

	if _, err := pool.Reserve(reservationRef(t, "reservation-2"), 50, reservedUntil, reservedAt); !errors.Is(err, domain.ErrCapacityExceeded) {
		t.Fatalf("error = %v, want ErrCapacityExceeded（60+50 > 100）", err)
	}

	pool, err = pool.Reserve(reservationRef(t, "reservation-3"), 40, reservedUntil, reservedAt)
	if err != nil {
		t.Fatalf("reserve exact remainder: %v", err)
	}
	if available := pool.AvailableAt(reservedAt); available != 0 {
		t.Fatalf("available = %d, want 0", available)
	}

	t.Run("a duplicate reservation reference is refused", func(t *testing.T) {
		if _, err := pool.Reserve(reservationRef(t, "reservation-1"), 1, reservedUntil, reservedAt); !errors.Is(err, domain.ErrDuplicateReservation) {
			t.Fatalf("error = %v, want ErrDuplicateReservation", err)
		}
	})

	t.Run("a window not after the reservation instant is refused", func(t *testing.T) {
		if _, err := establishedPool(t, 100).Reserve(reservationRef(t, "reservation-x"), 10, reservedAt, reservedAt); !errors.Is(err, domain.ErrInvalidCapacityPool) {
			t.Fatalf("error = %v; 没有未来窗口的预占占不了任何时段", err)
		}
	})
}

// Covers: CONTEXT 生命周期「有效预占 → 部分释放、全部释放、到期或实际消耗：各维度分别
// 转换数量，任何时点都不得重复扣减同一范围」——释放归还可用量、消耗永远占用、到期自动
// 归还未消耗剩余；三条出路各自分明。
func TestReleaseExpiryAndConsumptionAreSeparateExits(t *testing.T) {
	pool := establishedPool(t, 100)
	pool, err := pool.Reserve(reservationRef(t, "reservation-1"), 60, reservedUntil, reservedAt)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	// 部分释放 20：归还可用量（40 → 60）。
	pool, err = pool.Release(reservationRef(t, "reservation-1"), 20, reservedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("release 20: %v", err)
	}
	if available := pool.AvailableAt(reservedAt.Add(time.Hour)); available != 60 {
		t.Fatalf("available = %d, want 60（释放归还可用容量）", available)
	}

	// 消耗 30（装载分配确认）：仍然占用——实物已进承载范围。
	pool, err = pool.Consume(reservationRef(t, "reservation-1"), 30,
		mustValue(t, domain.NewLoadAssignmentReference, "assignment-1"), reservedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("consume 30: %v", err)
	}
	if available := pool.AvailableAt(reservedAt.Add(2 * time.Hour)); available != 60 {
		t.Fatalf("available = %d, want 60（消耗不归还可用量）", available)
	}

	// 到期后：未消耗剩余（60-20-30=10）自动归还，已消耗 30 永远占用 → 可用 70。
	afterExpiry := reservedUntil.Add(time.Hour)
	if available := pool.AvailableAt(afterExpiry); available != 70 {
		t.Fatalf("available = %d, want 70（到期归还未消耗剩余，消耗仍占用）", available)
	}

	reservation, _ := pool.ReservationFor(reservationRef(t, "reservation-1"))
	reserved, released, consumed := reservation.Quantities()
	if reserved != 60 || released != 20 || consumed != 30 {
		t.Fatalf("quantities = %d/%d/%d, want 60/20/30（三量分别保存守恒）", reserved, released, consumed)
	}

	t.Run("an expired reservation can neither release nor consume", func(t *testing.T) {
		if _, err := pool.Release(reservationRef(t, "reservation-1"), 5, afterExpiry); !errors.Is(err, domain.ErrReservationExpired) {
			t.Fatalf("release error = %v, want ErrReservationExpired", err)
		}
		if _, err := pool.Consume(reservationRef(t, "reservation-1"), 5,
			mustValue(t, domain.NewLoadAssignmentReference, "assignment-2"), afterExpiry); !errors.Is(err, domain.ErrReservationExpired) {
			t.Fatalf("consume error = %v, want ErrReservationExpired", err)
		}
	})

	t.Run("a consumed pool stays occupied for later reservations", func(t *testing.T) {
		full := establishedPool(t, 100)
		full, err := full.Reserve(reservationRef(t, "reservation-a"), 100, reservedUntil, reservedAt)
		if err != nil {
			t.Fatalf("reserve: %v", err)
		}
		full, err = full.Consume(reservationRef(t, "reservation-a"), 100,
			mustValue(t, domain.NewLoadAssignmentReference, "assignment-a"), reservedAt.Add(time.Hour))
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if _, err := full.Reserve(reservationRef(t, "reservation-b"), 10, reservedUntil.Add(24*time.Hour), reservedUntil.Add(time.Hour)); !errors.Is(err, domain.ErrCapacityExceeded) {
			t.Fatalf("error = %v; 已消耗容量被当成了空闲", err)
		}
	})
}

// Covers: CONTEXT「同一业务范围……不得把同一容量重复累计」与预占三量守恒——释放+消耗
// 不得超过预占量；消耗必须有装载分配依据；类型上没有任何反向方法（不可逆结构防线）。
func TestConsumptionAndReleaseAreIrreversibleAndConserved(t *testing.T) {
	poolType := reflect.TypeOf(domain.CapacityPool{})
	for index := 0; index < poolType.NumMethod(); index++ {
		name := strings.ToLower(poolType.Method(index).Name)
		for _, banned := range []string{"unrelease", "unconsume", "restore", "revert", "rollback"} {
			if strings.Contains(name, banned) {
				t.Fatalf("CapacityPool 带方法 %q——释放/消耗就有了反向入口", name)
			}
		}
	}

	pool := establishedPool(t, 100)
	pool, err := pool.Reserve(reservationRef(t, "reservation-1"), 50, reservedUntil, reservedAt)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	pool, err = pool.Release(reservationRef(t, "reservation-1"), 30, reservedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("release: %v", err)
	}

	t.Run("over-conversion of the same range is refused", func(t *testing.T) {
		if _, err := pool.Release(reservationRef(t, "reservation-1"), 30, reservedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrReservationOverdrawn) {
			t.Fatalf("release error = %v, want ErrReservationOverdrawn（30+30 > 50）", err)
		}
		if _, err := pool.Consume(reservationRef(t, "reservation-1"), 30,
			mustValue(t, domain.NewLoadAssignmentReference, "assignment-1"), reservedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrReservationOverdrawn) {
			t.Fatalf("consume error = %v, want ErrReservationOverdrawn", err)
		}
	})

	t.Run("consumption without a load assignment is refused", func(t *testing.T) {
		if _, err := pool.Consume(reservationRef(t, "reservation-1"), 10,
			domain.LoadAssignmentReference{}, reservedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrInvalidCapacityPool) {
			t.Fatalf("error = %v; 没有装载分配确认的消耗无从谈起", err)
		}
	})

	t.Run("an unknown reservation cannot be converted", func(t *testing.T) {
		if _, err := pool.Release(reservationRef(t, "reservation-9"), 1, reservedAt.Add(time.Hour)); !errors.Is(err, domain.ErrReservationNotFound) {
			t.Fatalf("error = %v, want ErrReservationNotFound", err)
		}
	})

	t.Run("conversions do not mutate the prior pool value", func(t *testing.T) {
		reservation, _ := pool.ReservationFor(reservationRef(t, "reservation-1"))
		_, released, _ := reservation.Quantities()
		if released != 30 {
			t.Fatalf("released = %d, want 30", released)
		}
		if _, err := pool.Release(reservationRef(t, "reservation-1"), 20, reservedAt.Add(3*time.Hour)); err != nil {
			t.Fatalf("release remainder: %v", err)
		}
		again, _ := pool.ReservationFor(reservationRef(t, "reservation-1"))
		_, releasedAgain, _ := again.Quantities()
		if releasedAgain != 30 {
			t.Fatal("转换改写了原池值——值语义破了")
		}
	})
}

// Covers: 读回已有预占不能重放 Reserve——转换门要调用期的有效期与装载分配，行上只有
// 三量。重建门验守恒与引用不重，不按时点重算可用量。
func TestRehydrateCapacityPoolRestoresReservationsWithoutReplayingConversions(t *testing.T) {
	spec := domain.CapacityPoolSpec{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Pool:     mustValue(t, domain.NewCapacityPoolReference, "pool-weight-1"),
		Schedule: mustValue(t, domain.NewScheduleReference, "schedule-1"),
		Unit:     mustValue(t, domain.NewQuantityUnitReference, "kg"),
		Capacity: 100,
	}
	snapshots := []domain.CapacityReservationSnapshot{{
		Reference:  reservationRef(t, "reservation-1"),
		Quantity:   60,
		Released:   20,
		Consumed:   30,
		ValidUntil: reservedUntil,
	}}
	pool, err := domain.RehydrateCapacityPool(spec, snapshots)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	reservation, found := pool.ReservationFor(reservationRef(t, "reservation-1"))
	if !found {
		t.Fatal("重建后预占丢失")
	}
	reserved, released, consumed := reservation.Quantities()
	if reserved != 60 || released != 20 || consumed != 30 {
		t.Fatalf("quantities = %d/%d/%d, want 60/20/30", reserved, released, consumed)
	}
	if available := pool.AvailableAt(reservedUntil.Add(time.Hour)); available != 70 {
		t.Fatalf("available after expiry = %d, want 70", available)
	}

	t.Run("an overdrawn snapshot is refused", func(t *testing.T) {
		bad := snapshots[0]
		bad.Released = 40
		if _, err := domain.RehydrateCapacityPool(spec, []domain.CapacityReservationSnapshot{bad}); !errors.Is(err, domain.ErrReservationOverdrawn) {
			t.Fatalf("error = %v, want ErrReservationOverdrawn", err)
		}
	})

	t.Run("a duplicate reservation reference is refused", func(t *testing.T) {
		dup := []domain.CapacityReservationSnapshot{snapshots[0], snapshots[0]}
		if _, err := domain.RehydrateCapacityPool(spec, dup); !errors.Is(err, domain.ErrDuplicateReservation) {
			t.Fatalf("error = %v, want ErrDuplicateReservation", err)
		}
	})
}
