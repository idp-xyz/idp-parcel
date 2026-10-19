package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 票 no-reception-source/04 的真库并发回归（ADR-0199 决定四「串行」）：同一件实物两个来源在两条连接的两笔事务里送到同一节点。
// A 按实物与节点找完各链（握着实物锁）之后挂住，B 此时经生产装配送站；B 要么等在实物锁上、A 提交后读到 A 的收寄而并入，要么
// （修前）径直走完、两条都形成控制起点。看 B 等的是哪一把锁走一条池外的独立连接，按 pg_locks 里咨询锁的锁空间认；等待全部
// 带超时。夹具全为合成（S 级）。

// 两把锁的锁空间名，与收寄库 PG 适配器取锁时用的同一个串：观察方按它认出等的是哪一把。
const (
	unitLockSpace   = "node_operations.reception.handling_unit"
	parcelLockSpace = "node_operations.reception.parcel_association"
)

// unitPausingReceptions 是生产收寄库外面一层：按实物与节点找完各链之后先跑 afterFindCurrent，再把读到的交回编排。
type unitPausingReceptions struct {
	noports.ReceptionStore
	afterFindCurrent func()
}

func (store unitPausingReceptions) FindCurrentJudgmentsAtNode(
	ctx context.Context,
	tenant nodomain.TenantID,
	unit nodomain.HandlingUnitID,
	node nodomain.NodeReference,
) ([]noports.ReceptionRecord, error) {
	records, err := store.ReceptionStore.FindCurrentJudgmentsAtNode(ctx, tenant, unit, node)
	if err == nil {
		store.afterFindCurrent()
	}
	return records, err
}

// awaitAdvisoryWaiters 等到各锁空间上等锁的后端数都到了 want，或 done 里有一个先关了（那一方没被挡住），交回是不是前者。观察走
// 池外的独立连接：测试池只有两条连接，挂住的一方与等锁的一方各占。classid 是两参咨询锁的第一键按无符号存的，比较前把 hashtext
// 的有符号值按同一宽度取模。
func awaitAdvisoryWaiters(t *testing.T, pool *pgxpool.Pool, want map[string]int, done ...<-chan struct{}) bool {
	t.Helper()
	observer, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Errorf("开观察连接：%v", err)
		return false
	}
	defer observer.Close(context.Background())
	deadline := time.Now().Add(lockWaitDeadline)
	for time.Now().Before(deadline) {
		for _, finished := range done {
			select {
			case <-finished:
				t.Logf("有一方没被挡住，已走完")
				return false
			default:
			}
		}
		reached := true
		for space, count := range want {
			var waiting int
			if err := observer.QueryRow(t.Context(),
				`SELECT count(*) FROM pg_locks
				  WHERE locktype = 'advisory' AND NOT granted
				    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
				    AND classid::int8 = (hashtext($1)::int8 & 4294967295)`, space,
			).Scan(&waiting); err != nil {
				t.Errorf("查等锁：%v", err)
				return false
			}
			reached = reached && waiting >= count
		}
		if reached {
			t.Logf("各方都等在该等的锁上，放行")
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("没在时限内等到 %v", want)
	return false
}

// sameUnitConcurrency 备一件带客户包裹件号、已接受的包裹与那个客户账户列着它的送站预报，交回生产装配的收寄编排、照生产装配的依赖
// 与一条按那个号码、带着那份预报明确接收的命令（ADR-0212 决定四：客户包裹件号的唯一识别要有可验证交付范围）。
func sameUnitConcurrency(t *testing.T, number string) (*pgxpool.Pool, *bentopg.DB, nodeopsapp.ReceiveDeliveredUnitDeps, func(sourceID, unit string) nodeopsapp.ReceiveDeliveredUnitCommand) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, number))
	basis := preAdviseOnRealDatabase(t, db, "SYN-CUSTOMER-1", number)
	deps, err := buildReceptionDeps(db)
	if err != nil {
		t.Fatalf("装配收寄依赖：%v", err)
	}
	deliver := func(sourceID, unit string) nodeopsapp.ReceiveDeliveredUnitCommand {
		command := receptionCommand(t, sourceID, nodeopsapp.ExplicitReception)
		command.Unit = mustValue(t, nodomain.NewHandlingUnitID, unit)
		command.Mark = noports.ExternalMarkObservation{Mark: number}
		command.ScopeBasis = basis
		return command
	}
	return pool, db, deps, deliver
}

// Covers: `AT-NO-049` 的并发半边（ADR-0199 决定四「串行：认同一实物之前，按（租户，作业实物）取事务级咨询锁」）：同一件实物
// 两个来源并发送到同一节点。A 找完各链后挂起，B 经生产装配送站，等在实物锁上（不是包裹锁：B 还没走到身份核对）；A 提交后
// B 读到 A 的收寄，答已有结果并入它——原判断种类 INTAKE_FORMED、当前有效版本是 A 那一版；册上一个控制起点、意图一封。
func TestConcurrentDeliveriesOfOneUnitSerializeOnTheUnit(t *testing.T) {
	pool, db, deps, deliver := sameUnitConcurrency(t, "SYN-CUSTOMER-PARCEL-NRS04C")
	production, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	first := deliver("SYN-DELIVERY-NRS04C-A", "SYN-UNIT-NRS04C")
	second := deliver("SYN-DELIVERY-NRS04C-B", "SYN-UNIT-NRS04C")
	second.OccurredAt = first.OccurredAt.Add(time.Minute)

	var (
		secondResult nodeopsapp.ReceiveDeliveredUnitResult
		secondErr    error
		secondDone   chan struct{}
		waited       bool
	)
	deps.Receptions = unitPausingReceptions{ReceptionStore: deps.Receptions, afterFindCurrent: func() {
		if secondDone != nil {
			return
		}
		secondDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			secondResult, secondErr = production.Handle(t.Context(), second)
		}(secondDone)
		waited = awaitAdvisoryWaiters(t, pool, map[string]int{unitLockSpace: 1}, secondDone)
	}}
	paused, err := receptionWithin(db, deps)
	if err != nil {
		t.Fatalf("装配挂住的收寄编排：%v", err)
	}

	firstResult, firstErr := paused.Handle(t.Context(), first)
	if secondDone == nil {
		t.Fatal("A 没走到按实物与节点找各链那一步")
	}
	awaitDone(t, secondDone)
	if firstErr != nil || secondErr != nil {
		t.Fatalf("A err = %v，B err = %v", firstErr, secondErr)
	}
	if !waited {
		t.Fatal("B 没在实物锁上等就走完了")
	}
	formed, _ := firstResult.Record()
	if firstResult.Outcome() != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("A outcome = %v, want INTAKE_FORMED", firstResult.Outcome())
	}
	if secondResult.Outcome() != nodeopsapp.ReceptionExistingResult || secondResult.RecordedOutcome() != nodeopsapp.NodeIntakeFormed ||
		secondResult.CurrentVersion() != formed.JudgmentVersion || secondResult.MergedInto() != formed.JudgmentVersion {
		t.Fatalf("B outcome = %v 原判断种类 = %v 当前有效版本 = %q 并入 = %q, want EXISTING_RESULT 并入 A 那一版 %q",
			secondResult.Outcome(), secondResult.RecordedOutcome(), secondResult.CurrentVersion(), secondResult.MergedInto(), formed.JudgmentVersion)
	}
	if starts := countRows(t, pool, `SELECT count(DISTINCT control ->> 'basis') FROM node_operations.reception
		  WHERE tenant_id = $1 AND control IS NOT NULL AND intake ->> 'unit' = $2`, first.TenantID.String(), first.Unit.String()); starts != 1 {
		t.Fatalf("控制起点 %d 个, want 1——两笔事务互相看不到对方的收寄", starts)
	}
	if intents := nodeIntakeIntents(t, pool); intents != 1 {
		t.Fatalf("意图 %d 封, want 1", intents)
	}
}

// Covers: ADR-0199 决定四「次序固定为先实物后包裹，两把锁不会交叉等待」：A 唯一识别、读完别的实物关联后挂起，此刻握着实物锁与
// 包裹锁；同一件实物换来源的 B 等在实物锁上，贴同一个号的另一件实物 C 已取得自己的实物锁、等在包裹锁上。A 提交后两方都在
// 时限内走完、没有死锁：B 并入那件实物此刻的当前收寄，C 答待识别、记身份冲突。那件实物的控制起点仍是一个。
func TestConcurrentDeliveriesOnTheUnitAndTheParcelLocksDoNotDeadlock(t *testing.T) {
	pool, db, deps, deliver := sameUnitConcurrency(t, "SYN-CUSTOMER-PARCEL-NRS04D")
	production, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	// 三方各占一条连接：测试池封顶两条，C 走同一个库上另开的一只池，免得等的是连接而不是锁。
	spare, err := pgxpool.NewWithConfig(t.Context(), pool.Config().Copy())
	if err != nil {
		t.Fatalf("另开连接池：%v", err)
	}
	t.Cleanup(spare.Close)
	spareDB, err := bentopg.NewDB(spare, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造另一只框架 DB：%v", err)
	}
	otherProduction, err := buildReceptionOrchestration(spareDB)
	if err != nil {
		t.Fatalf("装配另一只收寄编排：%v", err)
	}
	first := deliver("SYN-DELIVERY-NRS04D-A", "SYN-UNIT-NRS04D")
	sameUnit := deliver("SYN-DELIVERY-NRS04D-B", "SYN-UNIT-NRS04D")
	otherUnit := deliver("SYN-DELIVERY-NRS04D-C", "SYN-UNIT-NRS04D-OTHER")

	var (
		sameResult, otherResult nodeopsapp.ReceiveDeliveredUnitResult
		sameErr, otherErr       error
		sameDone, otherDone     chan struct{}
		waited                  bool
	)
	deps.Receptions = pausingReceptions{ReceptionStore: deps.Receptions, afterFindOthers: func() {
		if sameDone != nil {
			return
		}
		sameDone, otherDone = make(chan struct{}), make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			sameResult, sameErr = production.Handle(t.Context(), sameUnit)
		}(sameDone)
		go func(done chan struct{}) {
			defer close(done)
			otherResult, otherErr = otherProduction.Handle(t.Context(), otherUnit)
		}(otherDone)
		waited = awaitAdvisoryWaiters(t, pool, map[string]int{unitLockSpace: 1, parcelLockSpace: 1}, sameDone, otherDone)
	}}
	paused, err := receptionWithin(db, deps)
	if err != nil {
		t.Fatalf("装配挂住的收寄编排：%v", err)
	}

	firstResult, firstErr := paused.Handle(t.Context(), first)
	if sameDone == nil {
		t.Fatal("A 没走到按包裹读别的实物关联那一步——号码没唯一识别出包裹")
	}
	awaitDone(t, sameDone)
	awaitDone(t, otherDone)
	if firstErr != nil || sameErr != nil || otherErr != nil {
		t.Fatalf("A err = %v，B err = %v，C err = %v", firstErr, sameErr, otherErr)
	}
	if !waited {
		t.Fatal("B、C 没各自等在实物锁与包裹锁上")
	}
	if firstResult.Outcome() != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("A outcome = %v, want INTAKE_FORMED", firstResult.Outcome())
	}
	if sameResult.Outcome() != nodeopsapp.ReceptionExistingResult || sameResult.MergedInto().String() == "" {
		t.Fatalf("B outcome = %v 并入 = %q, want EXISTING_RESULT 并入那件实物的当前收寄", sameResult.Outcome(), sameResult.MergedInto())
	}
	if late, _ := otherResult.Record(); otherResult.Outcome() != nodeopsapp.UnitPendingIdentification || !late.IdentityConflict {
		t.Fatalf("C outcome = %v 冲突 = %v, want PENDING_IDENTIFICATION 记身份冲突", otherResult.Outcome(), late.IdentityConflict)
	}
	if starts := countRows(t, pool, `SELECT count(DISTINCT control ->> 'basis') FROM node_operations.reception
		  WHERE tenant_id = $1 AND control IS NOT NULL AND intake ->> 'unit' = $2`, first.TenantID.String(), first.Unit.String()); starts != 1 {
		t.Fatalf("A 那件的控制起点 %d 个, want 1", starts)
	}
}
