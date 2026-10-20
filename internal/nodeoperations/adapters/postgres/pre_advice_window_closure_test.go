package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证预报有效时窗的收口（票 ps-external-mark-relations/14 裁定）：收口是追加的另一份事实，在册那一行不被
// 改写；重叠按收口之后的有效终点判，收口之后起点不早于收口时刻的新行登得进；按时点读生效行时收口时刻当终点；对已有终点的行、
// 重复收口、本租户没有的行各答其格。夹具全为合成（S 级）。

type windowRegisterFixture struct {
	windows    *adapter.PreAdviceWindows
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newWindowRegister(t *testing.T) windowRegisterFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	windows, err := adapter.NewPreAdviceWindows(db)
	if err != nil {
		t.Fatal(err)
	}
	return windowRegisterFixture{windows: windows, transactor: db.Transactor(), pool: pool}
}

func (fixture windowRegisterFixture) register(window domain.PreAdviceValidityWindow) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
	var recorded domain.PreAdviceValidityWindow
	var outcome ports.DeliveryScopeAppendOutcome
	err := inTransaction(fixture.transactor, func(txCtx context.Context) (err error) {
		recorded, outcome, err = fixture.windows.RegisterPreAdviceWindow(txCtx, window)
		return err
	})
	return recorded, outcome, err
}

func (fixture windowRegisterFixture) close(closure domain.PreAdviceWindowClosure) (domain.PreAdviceValidityWindow, ports.DeliveryScopeAppendOutcome, error) {
	var recorded domain.PreAdviceValidityWindow
	var outcome ports.DeliveryScopeAppendOutcome
	err := inTransaction(fixture.transactor, func(txCtx context.Context) (err error) {
		recorded, outcome, err = fixture.windows.ClosePreAdviceWindow(txCtx, closure)
		return err
	})
	return recorded, outcome, err
}

func closureOf(t *testing.T, tenant string, windowFrom, closedAt time.Time) domain.PreAdviceWindowClosure {
	t.Helper()
	closure, err := domain.FormPreAdviceWindowClosure(domain.PreAdviceWindowClosureSpec{
		TenantID:     must(t, domain.NewTenantID, tenant),
		WindowFrom:   windowFrom,
		ClosedAt:     closedAt,
		Basis:        "SYN-BASIS-PSXM14-CLOSURE",
		RegisteredAt: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return closure
}

func closedAtOf(window domain.PreAdviceValidityWindow) (time.Time, bool) {
	closure, closed := window.Closure()
	return closure.ClosedAt(), closed
}

// Covers: 完成判据「开放终点的一行收口后，起点不早于收口时刻的新行登得进；收口前登不进（照旧）」与「推导：收口之后的业务时间不再
// 落进前一行的时窗」的读册半边——收口前后继撞开放的那一行；收口落下、回显带着收口；起点早于收口时刻的后继照旧撞上；起点恰在
// 收口时刻的后继登得进；按时点读，收口时刻之前是前一行（带收口），收口时刻起是后继；前一行原样重登仍是同一内容（演示种子重灌
// 不断）。
func TestAClosedOpenWindowAdmitsItsSuccessor(t *testing.T) {
	fixture := newWindowRegister(t)
	ctx := context.Background()
	tenant := must(t, domain.NewTenantID, registerTenant)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	open := windowOf(t, jan, time.Time{}, 6*time.Hour)
	if _, outcome, err := fixture.register(open); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("open = %s %v", outcome, err)
	}
	successor := windowOf(t, jul, time.Time{}, 8*time.Hour)
	if echoed, outcome, err := fixture.register(successor); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || !echoed.SameContent(open) {
		t.Fatalf("收口之前的后继 = %s %v, want ALREADY_RECORDED echoing the open row", outcome, err)
	}

	closed, outcome, err := fixture.close(closureOf(t, registerTenant, jan, jul))
	if err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("close = %s %v", outcome, err)
	}
	if closedAt, isClosed := closedAtOf(closed); !isClosed || !closedAt.Equal(jul) || !closed.SameContent(open) {
		t.Fatalf("close echo = %v %v", closedAt, isClosed)
	}

	early := windowOf(t, jul.Add(-time.Minute), time.Time{}, 8*time.Hour)
	if echoed, outcome, err := fixture.register(early); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || !echoed.SameContent(open) {
		t.Fatalf("起点早于收口时刻的后继 = %s %v, want ALREADY_RECORDED echoing the closed row", outcome, err)
	}
	if _, outcome, err := fixture.register(successor); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("收口之后的后继 = %s %v", outcome, err)
	}

	for name, testCase := range map[string]struct {
		at     time.Time
		grace  time.Duration
		closed bool
	}{
		"收口时刻前一刻": {jul.Add(-time.Microsecond), 6 * time.Hour, true},
		"恰在收口时刻":  {jul, 8 * time.Hour, false},
		"收口之后一年":  {jul.AddDate(1, 0, 0), 8 * time.Hour, false},
	} {
		window, found, err := fixture.windows.FindPreAdviceWindowAt(ctx, tenant, testCase.at)
		if err != nil || !found || window.Grace() != testCase.grace {
			t.Fatalf("%s: window = %v %v %v", name, window.Grace(), found, err)
		}
		if _, isClosed := window.Closure(); isClosed != testCase.closed {
			t.Fatalf("%s: closed = %v, want %v", name, isClosed, testCase.closed)
		}
	}

	if echoed, outcome, err := fixture.register(open); err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || !echoed.SameContent(open) {
		t.Fatalf("前一行原样重登 = %s %v, want ALREADY_RECORDED echoing it", outcome, err)
	}
}

// Covers: 完成判据「收口时刻、已闭合行、重复收口各一格」的真库半边——登记时带终点的行收不了口，什么也不落、回显那一行；原样重复
// 收口与换时刻再收都回显第一份收口、不覆盖；本租户没有那一行（起点对不上、别的租户）答查无此行；收口时刻不晚于那一行的起点，
// 库面 CHECK 再守一道。
func TestClosingAWindowAnswersByWhatTheRowAlreadyHas(t *testing.T) {
	fixture := newWindowRegister(t)
	ctx := context.Background()
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	bounded := windowOf(t, jan, jul, 6*time.Hour)
	if _, outcome, err := fixture.register(bounded); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("bounded = %s %v", outcome, err)
	}
	echoed, outcome, err := fixture.close(closureOf(t, registerTenant, jan, jan.AddDate(0, 3, 0)))
	if err != nil || outcome != ports.DeliveryScopeAlreadyRecorded || !echoed.SameContent(bounded) {
		t.Fatalf("登记时带终点的行 = %s %v, want ALREADY_RECORDED echoing it", outcome, err)
	}
	if _, isClosed := echoed.Closure(); isClosed {
		t.Fatal("登记时带终点的行被落了收口")
	}

	open := windowOf(t, jul, time.Time{}, 8*time.Hour)
	if _, outcome, err := fixture.register(open); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("open = %s %v", outcome, err)
	}
	dec := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	if _, outcome, err := fixture.close(closureOf(t, registerTenant, jul, dec)); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("close = %s %v", outcome, err)
	}
	for name, closure := range map[string]domain.PreAdviceWindowClosure{
		"原样重复收口": closureOf(t, registerTenant, jul, dec),
		"换时刻再收":  closureOf(t, registerTenant, jul, dec.AddDate(0, 1, 0)),
	} {
		echoed, outcome, err := fixture.close(closure)
		if err != nil || outcome != ports.DeliveryScopeAlreadyRecorded {
			t.Fatalf("%s = %s %v, want ALREADY_RECORDED", name, outcome, err)
		}
		if closedAt, isClosed := closedAtOf(echoed); !isClosed || !closedAt.Equal(dec) {
			t.Fatalf("%s: echo closedAt = %v %v, want the first closure", name, closedAt, isClosed)
		}
	}
	window, found, err := fixture.windows.FindPreAdviceWindowAt(ctx, must(t, domain.NewTenantID, registerTenant), dec)
	if err != nil || found {
		t.Fatalf("第一份收口之后不该有生效行: found=%v err=%v grace=%v", found, err, window.Grace())
	}

	for name, closure := range map[string]domain.PreAdviceWindowClosure{
		"起点对不上": closureOf(t, registerTenant, jul.Add(time.Hour), dec),
		"别的租户":  closureOf(t, "SYN-TENANT-OTHER", jul, dec),
	} {
		if _, outcome, err := fixture.close(closure); err != nil || outcome != ports.DeliveryScopeTargetNotFound {
			t.Fatalf("%s = %s %v, want TARGET_NOT_FOUND", name, outcome, err)
		}
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO node_operations.pre_advice_window (tenant_id, effective_from, lead_minutes, grace_minutes, basis_ref, registered_at)
		 VALUES ('SYN-TENANT-RAW', $1, 60, 60, 'SYN-B', $1)`, jan); err != nil {
		t.Fatalf("raw window: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO node_operations.pre_advice_window_closure (tenant_id, window_effective_from, closed_at, basis_ref, registered_at)
		 VALUES ('SYN-TENANT-RAW', $1, $1, 'SYN-B', $1)`, jan); err == nil {
		t.Fatal("库面放进了收口时刻恰在起点的收口")
	}
}

// Covers: 同租户的写口串行——两份不同的收口并发投向同一行，只落一份，另一份回显它；收口之后两段并发的后继各自查不到对方，也只
// 登进一段。重叠不再由排他约束拒（约束读不到收口），靠的是同一把租户锁让后一笔在前一笔提交之后才查。
func TestConcurrentClosuresAndSuccessorsLandOnlyOnce(t *testing.T) {
	fixture := newWindowRegister(t)
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	jul := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, outcome, err := fixture.register(windowOf(t, jan, time.Time{}, 6*time.Hour)); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("open = %s %v", outcome, err)
	}

	closures := []domain.PreAdviceWindowClosure{
		closureOf(t, registerTenant, jan, jul),
		closureOf(t, registerTenant, jan, jul.AddDate(0, 1, 0)),
	}
	closureOutcomes := make([]ports.DeliveryScopeAppendOutcome, len(closures))
	closureErrs := make([]error, len(closures))
	var wait sync.WaitGroup
	for index, closure := range closures {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, closureOutcomes[index], closureErrs[index] = fixture.close(closure)
		}()
	}
	wait.Wait()
	if appended := countAppended(t, closureOutcomes, closureErrs); appended != 1 {
		t.Fatalf("closures appended = %d, want exactly one (%v)", appended, closureOutcomes)
	}

	successors := []domain.PreAdviceValidityWindow{
		windowOf(t, jul.AddDate(0, 1, 0), time.Time{}, 7*time.Hour),
		windowOf(t, jul.AddDate(0, 2, 0), time.Time{}, 8*time.Hour),
	}
	successorOutcomes := make([]ports.DeliveryScopeAppendOutcome, len(successors))
	successorErrs := make([]error, len(successors))
	for index, window := range successors {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, successorOutcomes[index], successorErrs[index] = fixture.register(window)
		}()
	}
	wait.Wait()
	if appended := countAppended(t, successorOutcomes, successorErrs); appended != 1 {
		t.Fatalf("successors appended = %d, want exactly one (%v)", appended, successorOutcomes)
	}
}

func countAppended(t *testing.T, outcomes []ports.DeliveryScopeAppendOutcome, errs []error) int {
	t.Helper()
	appended := 0
	for index := range outcomes {
		if errs[index] != nil {
			t.Fatalf("candidate %d: %v", index, errs[index])
		}
		if outcomes[index] == ports.DeliveryScopeAppended {
			appended++
		}
	}
	return appended
}

// Covers: PBC-08 负向证据——收口写口在无事务上下文里一律拒，册上什么也不落。
func TestClosingAWindowRefusesOutsideATransaction(t *testing.T) {
	fixture := newWindowRegister(t)
	ctx := context.Background()
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, outcome, err := fixture.register(windowOf(t, jan, time.Time{}, 6*time.Hour)); err != nil || outcome != ports.DeliveryScopeAppended {
		t.Fatalf("open = %s %v", outcome, err)
	}
	if _, _, err := fixture.windows.ClosePreAdviceWindow(ctx, closureOf(t, registerTenant, jan, jan.AddDate(0, 6, 0))); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ClosePreAdviceWindow: err = %v", err)
	}
	window, found, err := fixture.windows.FindPreAdviceWindowAt(ctx, must(t, domain.NewTenantID, registerTenant), jan.AddDate(1, 0, 0))
	if err != nil || !found {
		t.Fatalf("find = %v %v", found, err)
	}
	if _, isClosed := window.Closure(); isClosed {
		t.Fatal("被拒的收口落了")
	}
}
