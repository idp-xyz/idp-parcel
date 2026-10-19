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
	reserveAt         = time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)
	reserveValidUntil = time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
)

type poolStoreDouble struct {
	records  map[string]ports.CapacityPoolRecord
	findErr  error
	saves    int
	replaces int
	// raceWith 非空时，下一次落池前先让「另一方」落下这一份同身份的池。
	raceWith *ports.CapacityPoolRecord
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
	double.saves++
	if double.raceWith != nil {
		double.records[poolKey(double.raceWith.Key)] = *double.raceWith
		double.raceWith = nil
	}
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
	double.replaces++
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
	pools     *poolStoreDouble
	schedules *registeredSchedulesDouble
	handoff   *consumptionHandoffDouble
	handler   *application.PrepareTransportOpportunityHandler
}

// fixturePool 是夹具池的身份：登在班次 schedule-1 的重量维上，身份由二者派生。
const fixturePool = "schedule-1/WEIGHT"

func newOpportunityFixture(t *testing.T) *opportunityFixture {
	t.Helper()
	fixture := &opportunityFixture{
		pools:     newPoolStore(),
		schedules: registeredSchedules([2]string{"tenant-1", "schedule-1"}),
		handoff:   &consumptionHandoffDouble{},
	}
	fixture.handler = application.NewPrepareTransportOpportunityHandler(application.PrepareTransportOpportunityDeps{
		Pools:      fixture.pools,
		Schedules:  fixture.schedules,
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
	result, err := fixture.handler.EstablishPool(context.Background(), poolCommand(t))
	if err != nil {
		t.Fatalf("establish pool: %v", err)
	}
	if result.Outcome() != application.PoolEstablished {
		t.Fatalf("outcome = %q", result.Outcome())
	}
}

func poolCommand(t *testing.T) application.EstablishPoolCommand {
	t.Helper()
	return application.EstablishPoolCommand{
		TenantID:  opportunityTenant(t),
		Schedule:  "schedule-1",
		Dimension: "WEIGHT",
		Unit:      "kg",
		Capacity:  100,
	}
}

func reserveCommand(t *testing.T, quantity int64) application.ReserveCapacityCommand {
	t.Helper()
	return application.ReserveCapacityCommand{
		TenantID:        opportunityTenant(t),
		Pool:            fixturePool,
		Reservation:     "reservation-1",
		Quantity:        quantity,
		Unit:            "kg",
		SourceKind:      "OPERATOR_DECLARATION",
		SourceReference: "SYN-DEMAND-DECL-1/v1",
		ValidUntil:      reserveValidUntil,
		At:              reserveAt,
	}
}

// releaseCommand 是夹具预占 reservation-1 上的一次释放，单位与来源同 reserveCommand。
func releaseCommand(t *testing.T, release string, quantity int64, at time.Time) application.ReleaseCapacityCommand {
	t.Helper()
	return application.ReleaseCapacityCommand{
		TenantID:        opportunityTenant(t),
		Pool:            fixturePool,
		Reservation:     "reservation-1",
		Release:         release,
		Quantity:        quantity,
		Unit:            "kg",
		SourceKind:      "OPERATOR_DECLARATION",
		SourceReference: "SYN-DEMAND-DECL-1/v2",
		At:              at,
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

// Covers: CONTEXT 生命周期「有效预占 → 部分释放、全部释放、到期或实际消耗：各维度
// 分别转换数量，任何时点都不得重复扣减同一范围」的编排面——消耗成功交装载分配链
// 意图；过期（RESERVATION_EXPIRED）与守恒撞线（RESERVATION_OVERDRAWN）领域哨兵透出
// 各归业务负向一格；无装载分配依据的消耗未受理（消耗必须有装载分配确认）。
func TestConversionsSplitByTheirSentinels(t *testing.T) {
	fixture := newOpportunityFixture(t)
	establishedPool(t, fixture)
	if _, err := fixture.handler.Reserve(context.Background(), reserveCommand(t, 60)); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	released, err := fixture.handler.Release(context.Background(), releaseCommand(t, "release-1", 20, reserveAt.Add(time.Hour)))
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released.Outcome() != application.CapacityReleased {
		t.Fatalf("outcome = %q", released.Outcome())
	}

	consumed, err := fixture.handler.Consume(context.Background(), application.ConsumeCapacityCommand{
		TenantID:    opportunityTenant(t),
		Pool:        fixturePool,
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
		result, err := fixture.handler.Release(context.Background(), releaseCommand(t, "release-2", 1, reserveAt.Add(3*time.Hour)))
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
			Pool:        fixturePool,
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
			Pool:        fixturePool,
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

	t.Run("an unknown reservation is its own grade", func(t *testing.T) {
		result, err := fixture.handler.Release(context.Background(), func() application.ReleaseCapacityCommand {
			command := releaseCommand(t, "release-3", 1, reserveAt.Add(3*time.Hour))
			command.Reservation = "reservation-9"
			return command
		}())
		if err != nil {
			t.Fatalf("release: %v", err)
		}
		// 续办是改指在册的那一份预占，与改载荷不同（ADR-0203 决定六）。
		if result.Outcome() != application.ReservationNotFound {
			t.Fatalf("outcome = %q, want RESERVATION_NOT_FOUND", result.Outcome())
		}
	})
}

// Covers: ADR-0029（依赖故障归未决且指名等谁）、ADR-0031（写入代数封闭）与 ADR-0043
// （投递失败不翻结果、重放重发同一份）在本编排的恢复面；未决原因集封闭。
func TestOpportunityRecoveryDiscipline(t *testing.T) {
	t.Run("store failures are undecided with their reasons", func(t *testing.T) {
		poolFixture := newOpportunityFixture(t)
		poolFixture.pools.findErr = errors.New("store down")
		result, err := poolFixture.handler.Reserve(context.Background(), reserveCommand(t, 10))
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
			Pool:        fixturePool,
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
			application.PoolStoreUnavailable,
			application.PoolScheduleRegistryUnavailable,
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
			t.Fatal("名单外的未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: 方案 D03-6「EstablishPool 改为核对班次在册」与 `AT-TF-105`——池只能挂在登记册上有的班次上：班次没登过、
// 或只在他租户名下登过，都答`班次不在册`且零写入；班次册读不动答未决并指名等的是班次册，同样零写入。
func TestAPoolNeedsARegisteredSchedule(t *testing.T) {
	for name, schedules := range map[string]*registeredSchedulesDouble{
		"没登过":      registeredSchedules(),
		"只登在他租户名下": registeredSchedules([2]string{"tenant-2", "schedule-1"}),
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newOpportunityFixture(t)
			fixture.schedules.registered = schedules.registered
			result, err := fixture.handler.EstablishPool(context.Background(), poolCommand(t))
			if err != nil {
				t.Fatalf("establish: %v", err)
			}
			if result.Outcome() != application.PoolScheduleNotFound || result.Outcome().String() != "SCHEDULE_NOT_FOUND" {
				t.Fatalf("outcome = %s，要 SCHEDULE_NOT_FOUND", result.Outcome())
			}
			if _, present := result.Pool(); present || fixture.pools.saves != 0 {
				t.Fatalf("班次不在册却落了池：present=%v saves=%d", present, fixture.pools.saves)
			}
		})
	}

	t.Run("班次册读不动", func(t *testing.T) {
		fixture := newOpportunityFixture(t)
		fixture.schedules.findErr = errors.New("schedule registry down")
		result, err := fixture.handler.EstablishPool(context.Background(), poolCommand(t))
		if err != nil {
			t.Fatalf("establish: %v", err)
		}
		if result.Outcome() != application.OpportunityUndecided ||
			result.UndecidedReason() != application.PoolScheduleRegistryUnavailable || result.ContinuationReference() == "" {
			t.Fatalf("outcome = %s reason = %s", result.Outcome(), result.UndecidedReason())
		}
		if fixture.pools.saves != 0 {
			t.Fatal("班次册读不动却落了池")
		}
	})
}

// Covers: ADR-0201 补记「池身份」「登记口不改池」——池身份由（班次、维度）派生、不由登记方起名；同一（班次、维度）同内容
// 答`已存在`交回原池，异内容答`池冲突`并交回库里那一个，不成下一版、不改写；重放与冲突都先于班次核对作答（已建的
// 池答的是它是什么，不是去登班次）；文法外的输入零写入。
func TestEstablishingAPoolIsIdempotentPerScheduleAndDimension(t *testing.T) {
	fixture := newOpportunityFixture(t)
	established, err := fixture.handler.EstablishPool(context.Background(), poolCommand(t))
	if err != nil || established.Outcome() != application.PoolEstablished {
		t.Fatalf("首登 = %s %v", established.Outcome(), err)
	}
	record, _ := established.Pool()
	if record.Key.Pool.String() != fixturePool || record.Pool.Dimension() != domain.CapacityWeight {
		t.Fatalf("池身份 = %s 维度 = %s", record.Key.Pool, record.Pool.Dimension())
	}

	readsBefore := fixture.schedules.reads
	replay, err := fixture.handler.EstablishPool(context.Background(), poolCommand(t))
	if err != nil || replay.Outcome() != application.PoolExistingResult {
		t.Fatalf("重放 = %s %v", replay.Outcome(), err)
	}
	changed := poolCommand(t)
	changed.Capacity = 120
	conflict, err := fixture.handler.EstablishPool(context.Background(), changed)
	if err != nil || conflict.Outcome() != application.PoolConflict {
		t.Fatalf("异内容 = %s %v", conflict.Outcome(), err)
	}
	stored, present := conflict.Pool()
	if !present || stored.Pool.Capacity() != 100 {
		t.Fatalf("冲突应交回库里那一池（容量 100）：present=%v", present)
	}
	if fixture.schedules.reads != readsBefore {
		t.Fatal("已建的池先去核了班次——重放与冲突应先于班次核对作答")
	}
	if fixture.pools.saves != 1 {
		t.Fatalf("重放或冲突多落了池：saves=%d", fixture.pools.saves)
	}

	other := poolCommand(t)
	other.Dimension = "PIECES"
	other.Unit = "PCS"
	if result, err := fixture.handler.EstablishPool(context.Background(), other); err != nil || result.Outcome() != application.PoolEstablished {
		t.Fatalf("同一班次另一维 = %s %v（多维分存，AT-TF-030）", result.Outcome(), err)
	}

	for name, mutate := range map[string]func(*application.EstablishPoolCommand){
		"维度不在封闭词里": func(command *application.EstablishPoolCommand) { command.Dimension = "weight" },
		"缺班次":      func(command *application.EstablishPoolCommand) { command.Schedule = " " },
		"缺单位":      func(command *application.EstablishPoolCommand) { command.Unit = "" },
		"容量不为正":    func(command *application.EstablishPoolCommand) { command.Capacity = 0 },
	} {
		command := poolCommand(t)
		mutate(&command)
		fresh := newOpportunityFixture(t)
		result, err := fresh.handler.EstablishPool(context.Background(), command)
		if err != nil || result.Outcome() != application.OpportunityNotAccepted || fresh.pools.saves != 0 {
			t.Fatalf("%s：%s %v saves=%d", name, result.Outcome(), err, fresh.pools.saves)
		}
	}
}

// Covers: ADR-0031 写入代数在建池口的并发面——落池时另一方先建了同一个池：内容同答`已存在`、异答`池冲突`，都交回
// 赢家，不宣称自己建了它。
func TestAPoolRaceIsAnsweredFromTheWinner(t *testing.T) {
	winner := func(t *testing.T, capacity int64) *ports.CapacityPoolRecord {
		t.Helper()
		source := newOpportunityFixture(t)
		command := poolCommand(t)
		command.Capacity = capacity
		result, err := source.handler.EstablishPool(context.Background(), command)
		if err != nil || result.Outcome() != application.PoolEstablished {
			t.Fatalf("备赢家：%s %v", result.Outcome(), err)
		}
		record, _ := result.Pool()
		return &record
	}
	for name, testCase := range map[string]struct {
		capacity int64
		want     application.OpportunityOutcome
	}{
		"同内容": {capacity: 100, want: application.PoolExistingResult},
		"异内容": {capacity: 90, want: application.PoolConflict},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newOpportunityFixture(t)
			fixture.pools.raceWith = winner(t, testCase.capacity)
			result, err := fixture.handler.EstablishPool(context.Background(), poolCommand(t))
			if err != nil || result.Outcome() != testCase.want {
				t.Fatalf("outcome = %s %v，要 %s", result.Outcome(), err, testCase.want)
			}
			if record, _ := result.Pool(); record.Pool.Capacity() != testCase.capacity {
				t.Fatalf("交回的不是赢家：容量 %d", record.Pool.Capacity())
			}
		})
	}
}
