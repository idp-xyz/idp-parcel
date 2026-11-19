package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证「并发提交同一修订号的两笔，至多一笔落册，另一笔按内容答重放或冲突」（UC-WF-001「一致性、幂等与并发」）靠的是库上的
// 主键，不靠应用层加锁：第一笔在自己的事务里插入后先不提交，第二笔随后保存同一修订号、被主键卡在行锁上，观察连接确认它真的卡住了，
// 才放第一笔提交——这样走到的是「等锁、锁放开后改答已在册或冲突」那条路，不是两笔碰巧一前一后。

type blockedSessionObserver struct {
	conn     *pgx.Conn
	database string
}

func newBlockedSessionObserver(t *testing.T, pool *pgxpool.Pool) *blockedSessionObserver {
	t.Helper()
	ctx := t.Context()
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatalf("读测试库名：%v", err)
	}
	conn, err := pgx.Connect(ctx, pgtest.AdminDSN(t))
	if err != nil {
		t.Fatalf("打开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return &blockedSessionObserver{conn: conn, database: database}
}

// waitForBlockedSession 轮询到测试库里有会话卡在锁上，或超时。
func (observer *blockedSessionObserver) waitForBlockedSession(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var waiting int
		err := observer.conn.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND wait_event_type = 'Lock'`,
			observer.database).Scan(&waiting)
		if err == nil && waiting > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// raceOutcome 是并发里一笔的结局。
type raceOutcome struct {
	outcome ports.WarehouseSaveOutcome
	err     error
}

// raceSaves 让 first 先在事务里插入并按住不提交，再让 second 保存、等它卡在主键上，然后放 first 提交。各册的登记册共用它：
// 修订的类型是参数，落点代数各册本就是同一套。
func raceSaves[Revision any](
	t *testing.T,
	transactor bentoapp.Transactor,
	save func(ctx context.Context, revision Revision) (ports.RegistrySaveOutcome, error),
	observer *blockedSessionObserver,
	first, second Revision,
) (raceOutcome, raceOutcome) {
	t.Helper()
	ctx := t.Context()
	inserted := make(chan struct{})
	release := make(chan struct{})
	var firstOutcome, secondOutcome raceOutcome
	var wait sync.WaitGroup

	wait.Add(1)
	go func() {
		defer wait.Done()
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			firstOutcome.outcome, saveErr = save(txCtx, first)
			close(inserted)
			if saveErr == nil {
				<-release
			}
			return saveErr
		})
		firstOutcome.err = err
	}()
	<-inserted

	wait.Add(1)
	go func() {
		defer wait.Done()
		secondOutcome.err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var saveErr error
			secondOutcome.outcome, saveErr = save(txCtx, second)
			return saveErr
		})
	}()
	blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
	close(release)
	wait.Wait()

	if !blocked {
		t.Errorf("第二笔没有卡在主键上，没有走到「等锁」那条路")
	}
	return firstOutcome, secondOutcome
}

// Covers: AT-WF-001（仓的部分）——并发提交同一修订号、内容相同的两笔，一笔 SAVED、一笔 ALREADY_REGISTERED；内容不同则一笔 SAVED、
// 一笔 CONTENT_CONFLICT。两种情形库里都只有先提交的那一份，登记责任方是先落册者的。
func TestWarehouseRegistryConcurrentSavesOfOneRevisionLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		edit func(*domain.WarehouseContentSpec)
		want ports.WarehouseSaveOutcome
	}{
		"内容相同": {nil, ports.WarehouseRevisionAlreadyRegistered},
		"内容不同": {func(spec *domain.WarehouseContentSpec) {
			spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓（另一份）")
		}, ports.WarehouseRevisionContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			warehouses, _, transactor, pool := newWarehouseStores(t)
			observer := newBlockedSessionObserver(t, pool)
			first := revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, nil)
			second := registeredBy(t, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-01", 1, test.edit), "operator-2", baseInstant.Add(time.Hour))

			firstOutcome, secondOutcome := raceSaves(t, transactor, warehouses.SaveWarehouseRevision, observer, first, second)
			if firstOutcome.err != nil || secondOutcome.err != nil {
				t.Fatalf("两笔的错误：%v、%v", firstOutcome.err, secondOutcome.err)
			}
			if firstOutcome.outcome != ports.WarehouseRevisionSaved || secondOutcome.outcome != test.want {
				t.Fatalf("两笔的落点 = %s、%s，want SAVED、%s", firstOutcome.outcome, secondOutcome.outcome, test.want)
			}

			var rows int
			var registrant string
			if err := pool.QueryRow(t.Context(),
				`SELECT count(*), min(registrant) FROM warehouse_fulfillment.warehouse_revision`).Scan(&rows, &registrant); err != nil {
				t.Fatalf("读册：%v", err)
			}
			if rows != 1 || registrant != "operator-1" {
				t.Fatalf("册里 %d 笔、登记责任方 %q，want 只有先提交的那 1 笔（operator-1）", rows, registrant)
			}
		})
	}
}

// holdingRegistry 在第一次保存返回之前停住，让登记用例的事务悬着、不提交。
type holdingRegistry struct {
	ports.WarehouseRegistry
	afterSave func()
}

func (registry holdingRegistry) SaveWarehouseRevision(
	ctx context.Context,
	revision domain.WarehouseRevision,
) (ports.WarehouseSaveOutcome, error) {
	outcome, err := registry.WarehouseRegistry.SaveWarehouseRevision(ctx, revision)
	if registry.afterSave != nil {
		registry.afterSave()
	}
	return outcome, err
}

// Covers: AT-WF-001（仓的部分）端到端——登记用例在真库上并发提交同一修订号、内容相同的两笔：一笔 REGISTERED、一笔
// ALREADY_REGISTERED，答重放的那一笔回显先落册者的登记责任方；换内容则另一笔答 CONTENT_CONFLICT。每笔各起自己的事务。
func TestRegisterWarehouseConcurrentRegistrationsOnTheRealRegisterLandAtMostOnce(t *testing.T) {
	for name, test := range map[string]struct {
		secondName string
		want       application.RegisterWarehouseOutcome
	}{
		"内容相同": {"合成一号仓", application.WarehouseAlreadyRegistered},
		"内容不同": {"合成一号仓（另一份）", application.WarehouseContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			warehouses, _, transactor, pool := newWarehouseStores(t)
			observer := newBlockedSessionObserver(t, pool)
			ctx := t.Context()
			clock := fixedClock{at: baseInstant}

			commandFor := func(warehouseName, registrant string) application.RegisterWarehouseCommand {
				return application.RegisterWarehouseCommand{
					Tenant:   valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
					Code:     valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
					Revision: 1,
					Content: contentWith(t, func(spec *domain.WarehouseContentSpec) {
						spec.Name = valueOf(t, domain.NewWarehouseName, warehouseName)
					}),
					Registrant: valueOf(t, domain.NewRegistrant, registrant),
				}
			}
			// 命令在起 goroutine 之前造好：valueOf 失败走 t.Fatalf，不该发生在别的 goroutine 里。
			firstCommand := commandFor("合成一号仓", "operator-1")
			secondCommand := commandFor(test.secondName, "operator-2")

			inserted := make(chan struct{})
			release := make(chan struct{})
			holding := holdingRegistry{WarehouseRegistry: warehouses, afterSave: func() {
				close(inserted)
				<-release
			}}
			var firstResult, secondResult application.RegisterWarehouseResult
			var firstErr, secondErr error
			var wait sync.WaitGroup

			wait.Add(1)
			go func() {
				defer wait.Done()
				handler := application.NewRegisterWarehouseHandler(holding, clock)
				firstErr = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
					var registerErr error
					firstResult, registerErr = handler.Register(txCtx, firstCommand)
					return registerErr
				})
			}()
			select {
			case <-inserted:
			case <-time.After(10 * time.Second):
				t.Fatalf("第一笔没有走到保存")
			}

			wait.Add(1)
			go func() {
				defer wait.Done()
				handler := application.NewRegisterWarehouseHandler(warehouses, clock)
				secondErr = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
					var registerErr error
					secondResult, registerErr = handler.Register(txCtx, secondCommand)
					return registerErr
				})
			}()
			blocked := observer.waitForBlockedSession(ctx, 10*time.Second)
			close(release)
			wait.Wait()

			if !blocked {
				t.Errorf("第二笔没有卡在主键上，没有走到「等锁」那条路")
			}
			if firstErr != nil || secondErr != nil {
				t.Fatalf("两笔的错误：%v、%v", firstErr, secondErr)
			}
			if firstResult.Outcome() != application.WarehouseRegistered || secondResult.Outcome() != test.want {
				t.Fatalf("两笔的结果 = %s、%s，want REGISTERED、%s", firstResult.Outcome(), secondResult.Outcome(), test.want)
			}
			if test.want == application.WarehouseAlreadyRegistered {
				echoed, has := secondResult.Revision()
				if !has || echoed.Registrant().String() != "operator-1" {
					t.Fatalf("答重放的一笔回显的登记责任方 = %q（%v），want 先落册者的 operator-1", echoed.Registrant(), has)
				}
			}
			var rows int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM warehouse_fulfillment.warehouse_revision`).Scan(&rows); err != nil || rows != 1 {
				t.Fatalf("册里 %d 笔（%v），want 1", rows, err)
			}
		})
	}
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }
