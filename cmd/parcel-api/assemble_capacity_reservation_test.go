package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// unwiredCapacityDecisions 是端点表装配测试里预占口与释放口那两格的占位：未配置与渠道换口的用例都钉在编排之前，走到
// 它就是 Intake 漏了一道。
type unwiredCapacityDecisions struct{}

func (unwiredCapacityDecisions) Reserve(context.Context, tfapp.ReserveCapacityCommand) (tfapp.OpportunityResult, error) {
	return tfapp.OpportunityResult{}, errOrchestrationNotWired
}

func (unwiredCapacityDecisions) Release(context.Context, tfapp.ReleaseCapacityCommand) (tfapp.OpportunityResult, error) {
	return tfapp.OpportunityResult{}, errOrchestrationNotWired
}

const (
	capacityTenant = "SYN-TENANT-01"
	capacityPool   = "SYN-SCHED-SZX-SIN-DAILY/2026-10-21/WEIGHT"
)

var (
	capacityReservedAt = time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	capacityValidUntil = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
)

// capacityDB 在一份新库上经生产装配登干线周期安排（声明重量 5000 KG）并物化登记 2026-10-21 那一班，池随之建立。
func capacityDB(t *testing.T) (*pgxpool.Pool, *bentopg.DB) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	schedules, err := buildTransportScheduleRegistration(db)
	if err != nil {
		t.Fatalf("装配班次登记编排：%v", err)
	}
	tenant := chargeValue(t, tfdomain.NewTenantID, capacityTenant)
	template, err := schedules.RegisterTemplate(t.Context(), tfapp.RegisterScheduleTemplateCommand{
		TenantID: tenant, Template: "SYN-SCHED-SZX-SIN-DAILY", Connection: "SYN-CONN-SZX-SIN",
		FromNode: "SYN-NODE-SZX-GATE", ToNode: "SYN-NODE-SIN-HUB",
		Weekdays:           []string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"},
		DepartureLocalTime: "10:00", DurationMinutes: 300, TimeZone: "Asia/Shanghai", ServiceScope: "SYN-SCOPE-01",
		EffectiveFrom: "2026-01-01",
		Capacities:    []tfapp.CapacityDeclarationInput{{Dimension: "WEIGHT", Unit: "KG", Capacity: 5000}},
	})
	if err != nil || template.Outcome() != tfapp.ScheduleTemplateRegistered {
		t.Fatalf("登周期安排：%q %v", template.Outcome(), err)
	}
	schedule, err := schedules.RegisterSchedule(t.Context(), tfapp.RegisterTransportScheduleCommand{
		TenantID: tenant, Template: "SYN-SCHED-SZX-SIN-DAILY", RunningDate: "2026-10-21",
	})
	if err != nil || schedule.Outcome() != tfapp.TransportScheduleRegistered {
		t.Fatalf("物化登记一班：%q %v", schedule.Outcome(), err)
	}
	return pool, db
}

func capacityReserveCommand(t *testing.T, reservation string, quantity int64) tfapp.ReserveCapacityCommand {
	t.Helper()
	return tfapp.ReserveCapacityCommand{
		TenantID: chargeValue(t, tfdomain.NewTenantID, capacityTenant), Pool: capacityPool, Reservation: reservation,
		Quantity: quantity, Unit: "KG", SourceKind: "OPERATOR_DECLARATION", SourceReference: "SYN-DEMAND-DECL-01/v1",
		ValidUntil: capacityValidUntil, At: capacityReservedAt,
	}
}

func wantCapacityOutcome(t *testing.T, label string, result tfapp.OpportunityResult, err error, want tfapp.OpportunityOutcome) {
	t.Helper()
	if err != nil || result.Outcome() != want {
		t.Fatalf("%s：%q（%s）%v, want %q", label, result.Outcome(), result.UndecidedReason(), err, want)
	}
}

// Covers: 预占口与释放口的第二参是真编排（ADR-0203 决定三至五）——在真实 PostgreSQL 上：对物化班次随周期安排建的池预占
// 答已预占，重交答已有（证首笔真的提交了）；释放答已释放，重交答已有释放；超量预占答容量超出并带此刻余量。测试输入
// 是隔离合成，只记 `S`。
func TestTheWiredCapacityDecisionsReserveAndReleaseAgainstARealDatabase(t *testing.T) {
	_, db := capacityDB(t)
	decisions, err := buildCapacityDecisions(db)
	if err != nil {
		t.Fatalf("装配预占与释放编排：%v", err)
	}
	result, err := decisions.Reserve(t.Context(), capacityReserveCommand(t, "SYN-RES-1", 1200))
	wantCapacityOutcome(t, "预占", result, err, tfapp.CapacityReserved)
	result, err = decisions.Reserve(t.Context(), capacityReserveCommand(t, "SYN-RES-1", 1200))
	wantCapacityOutcome(t, "原样重交", result, err, tfapp.ReservationExistingResult)

	release := tfapp.ReleaseCapacityCommand{
		TenantID: chargeValue(t, tfdomain.NewTenantID, capacityTenant), Pool: capacityPool, Reservation: "SYN-RES-1",
		Release: "SYN-REL-1", Quantity: 200, Unit: "KG", SourceKind: "OPERATOR_DECLARATION",
		SourceReference: "SYN-DEMAND-DECL-01/v2", At: capacityReservedAt.Add(time.Hour),
	}
	result, err = decisions.Release(t.Context(), release)
	wantCapacityOutcome(t, "释放", result, err, tfapp.CapacityReleased)
	result, err = decisions.Release(t.Context(), release)
	wantCapacityOutcome(t, "释放原样重交", result, err, tfapp.ReleaseExistingResult)

	over := capacityReserveCommand(t, "SYN-RES-2", 4001)
	over.At = release.At
	result, err = decisions.Reserve(t.Context(), over)
	wantCapacityOutcome(t, "超量预占", result, err, tfapp.CapacityExceededOutcome)
	if result.AvailableQuantity() != 4000 {
		t.Fatalf("余量 = %d, want 4000（5000 − 1200 + 200）", result.AvailableQuantity())
	}
}

// Covers: 装配点在编排答`未决`时回滚再交回那一份答复——池上一份预占的已释放列与释放明细对不上（库面不钉这一条），读回
// 过不了重建门：预占答 200 未决，不是 500，库上什么也没多。
func TestTheWiredCapacityDecisionsAnswerUndecidedWhenThePoolDoesNotRehydrate(t *testing.T) {
	pool, db := capacityDB(t)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.capacity_reservation
			(tenant_id, pool_id, reservation_id, quantity, valid_until, released, consumed, source_kind, source_ref, reserved_at)
		 VALUES ($1, $2, 'SYN-RES-BROKEN', 10, $3, 5, 0, 'OPERATOR_DECLARATION', 'D', $4)`,
		capacityTenant, capacityPool, capacityValidUntil, capacityReservedAt); err != nil {
		t.Fatalf("插探针行：%v", err)
	}
	decisions, err := buildCapacityDecisions(db)
	if err != nil {
		t.Fatal(err)
	}
	result, err := decisions.Reserve(t.Context(), capacityReserveCommand(t, "SYN-RES-1", 10))
	wantCapacityOutcome(t, "读不回的池上预占", result, err, tfapp.OpportunityUndecided)
	if result.UndecidedReason() != tfapp.PoolStoreUnavailable || result.ContinuationReference() == "" {
		t.Fatalf("未决原因 = %q 续办 = %q", result.UndecidedReason(), result.ContinuationReference())
	}
	var reservations int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.capacity_reservation`).Scan(&reservations); err != nil || reservations != 1 {
		t.Fatalf("预占 %d 行（%v），want 只有探针那一行", reservations, err)
	}
}

// pausingPools 是生产池册外面一层：第一次换存之后先跑 afterReplace——此刻握着池那一行的锁、换存还没提交。
type pausingPools struct {
	tfports.CapacityPoolStore
	afterReplace func()
}

func (pools pausingPools) Replace(ctx context.Context, record tfports.CapacityPoolRecord) (bool, error) {
	ok, err := pools.CapacityPoolStore.Replace(ctx, record)
	if pools.afterReplace != nil {
		pools.afterReplace()
	}
	return ok, err
}

// awaitPoolLockWaiter 在池外的独立连接上等到有后端卡在锁上，或另一方已经走完。测试池只有两条连接，挂住的一方与等锁的
// 一方各占一条，从池里再取就会排在它们后面。
func awaitPoolLockWaiter(t *testing.T, pool *pgxpool.Pool, done <-chan struct{}) bool {
	t.Helper()
	observer, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Errorf("开观察连接：%v", err)
		return false
	}
	defer observer.Close(context.Background())
	deadline := time.Now().Add(lockWaitDeadline)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return false
		default:
		}
		var waiting int
		if err := observer.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`,
		).Scan(&waiting); err != nil {
			t.Errorf("查等锁：%v", err)
			return false
		}
		if waiting > 0 {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("另一方既没走完也没在等锁")
	return false
}

// Covers: `AT-TF-143`——同一个池上两笔并发预占（各 3000，池 5000）：A 落了池、还没提交；B 经生产装配预占，卡在池那一行
// 的锁上，A 提交后读到含 A 的池，答容量超出、余量 2000。库上只有 A 那一份：池按值换存而读不锁时，B 会按旧池放行，
// 换存时把 A 整份删掉。
func TestConcurrentReservationsOnOnePoolHoldTheHardCap(t *testing.T) {
	pool, db := capacityDB(t)
	production, err := buildCapacityDecisions(db)
	if err != nil {
		t.Fatal(err)
	}
	pools, err := tfpostgres.NewCapacityPools(db)
	if err != nil {
		t.Fatal(err)
	}
	var (
		second     tfapp.OpportunityResult
		secondErr  error
		secondDone chan struct{}
		waited     bool
	)
	paused := pausingPools{CapacityPoolStore: pools}
	paused.afterReplace = func() {
		if secondDone != nil {
			return
		}
		secondDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			second, secondErr = production.Reserve(t.Context(), capacityReserveCommand(t, "SYN-RES-B", 3000))
		}(secondDone)
		waited = awaitPoolLockWaiter(t, pool, secondDone)
	}
	first := transactionalCapacityDecisions{
		transactor: db.Transactor(),
		inner: tfapp.NewPrepareTransportOpportunityHandler(tfapp.PrepareTransportOpportunityDeps{
			Pools: paused, Clock: systemClock{},
		}),
	}
	result, err := first.Reserve(t.Context(), capacityReserveCommand(t, "SYN-RES-A", 3000))
	if secondDone == nil {
		t.Fatalf("A 没走到换存那一步：%q %v", result.Outcome(), err)
	}
	awaitDone(t, secondDone)
	wantCapacityOutcome(t, "A", result, err, tfapp.CapacityReserved)
	if !waited {
		t.Fatal("B 没在池那一行的锁上等就走完了")
	}
	wantCapacityOutcome(t, "B", second, secondErr, tfapp.CapacityExceededOutcome)
	if second.AvailableQuantity() != 2000 {
		t.Fatalf("B 的余量 = %d, want 2000", second.AvailableQuantity())
	}
	var reservations []string
	rows, err := pool.Query(t.Context(), `SELECT reservation_id FROM transport_fulfillment.capacity_reservation ORDER BY reservation_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		reservations = append(reservations, id)
	}
	if len(reservations) != 1 || reservations[0] != "SYN-RES-A" {
		t.Fatalf("库上的预占 = %v, want 只有 SYN-RES-A", reservations)
	}
}
