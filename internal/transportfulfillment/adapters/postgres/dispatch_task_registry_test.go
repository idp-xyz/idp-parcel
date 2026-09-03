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

// 本文件对真实 PostgreSQL 16 证揽派任务登记：整图往返（任务 + 对象范围）、撞键译`已建立`、
// 作用域隔离、无事务拒，以及库内 CHECK 挡住领域造不出的行。夹具全部为合成登记（S 级）。

var (
	taskOpenedAtFixture = time.Date(2026, 9, 12, 7, 30, 0, 0, time.UTC)
	taskWindowFixture   = taskOpenedAtFixture.Add(90 * time.Minute)
)

// TestADispatchTaskRoundTripsWithItsWholeScope 证任务与它的对象范围同笔落、整图读回。
//
// 断言对象逐个在场而不是只数个数：CONTEXT 要求「每个对象分别保存……任务汇总只能由对象结果
// 派生」，数对了个数说不出是不是同一批对象。
func TestADispatchTaskRoundTripsWithItsWholeScope(t *testing.T) {
	repository, transactor, _ := newDispatchTasks(t)
	ctx := t.Context()

	record := dispatchTaskRecord(t, "DT-0001", "parcel-1", "parcel-2")
	mustSaveDispatchTask(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, dispatchTaskKeyFixture(t, "tenant-1", "DT-0001"))
	if err != nil || !exists {
		t.Fatalf("取回任务：%v exists=%v", err, exists)
	}
	if found.Task.State() != domain.TaskOpen || found.Task.Kind() != domain.PickupDispatch {
		t.Fatalf("任务往返变形：state=%q kind=%q", found.Task.State(), found.Task.Kind())
	}
	if _, _, closed := found.Task.Closure(); closed {
		t.Fatal("开放任务读回来带了关闭三件")
	}
	objects := found.Task.Objects()
	if len(objects) != 2 || objects[0].String() != "parcel-1" || objects[1].String() != "parcel-2" {
		t.Fatalf("对象范围没有原样带回：%v", objects)
	}
	from, to := found.Task.Window()
	if !from.Equal(taskWindowFixture) || !to.Equal(taskWindowFixture.Add(3*time.Hour)) {
		t.Fatalf("时间窗口没有原样带回：%s..%s", from, to)
	}
	if !found.Task.OpenedAt().Equal(taskOpenedAtFixture) {
		t.Fatalf("建立时刻没有原样带回：%s", found.Task.OpenedAt())
	}
}

// TestAClosedDispatchTaskRoundTripsStillClosed 证关闭三件同笔往返，且读回来仍然关着——
// **装回不是重开**。没有依据的关闭与「一次失败尝试自动结束任务」在库里分不开，所以依据必须
// 随行；这里连它一起验，而不是只验状态。
func TestAClosedDispatchTaskRoundTripsStillClosed(t *testing.T) {
	repository, transactor, _ := newDispatchTasks(t)
	ctx := t.Context()

	basis, err := domain.NewTaskClosureBasisReference("closure-basis/object-results")
	if err != nil {
		t.Fatalf("依据：%v", err)
	}
	closedAt := taskOpenedAtFixture.Add(6 * time.Hour)
	record := dispatchTaskRecord(t, "DT-0002", "parcel-1")
	completed, err := record.Task.Complete(basis, closedAt)
	if err != nil {
		t.Fatalf("完成：%v", err)
	}
	record.Task = completed
	mustSaveDispatchTask(t, transactor, ctx, repository, record)

	found, exists, err := repository.FindByKey(ctx, dispatchTaskKeyFixture(t, "tenant-1", "DT-0002"))
	if err != nil || !exists {
		t.Fatalf("取回任务：%v exists=%v", err, exists)
	}
	if found.Task.State() != domain.TaskCompleted {
		t.Fatalf("状态 = %q, want COMPLETED", found.Task.State())
	}
	gotBasis, gotAt, closed := found.Task.Closure()
	if !closed || gotBasis.String() != "closure-basis/object-results" || !gotAt.Equal(closedAt) {
		t.Fatalf("关闭三件没有原样带回：basis=%q at=%v closed=%v", gotBasis, gotAt, closed)
	}
	if _, err := found.Task.Reschedule(taskWindowFixture, taskWindowFixture.Add(time.Hour), closedAt); err == nil {
		t.Fatal("读回来的已关闭任务还能改约——往返把它重开了")
	}
}

// TestASecondOpenOfTheSameTaskIsAlreadyOpen 证撞键译`已建立`而不是报错，也不顶替原任务：
// 一次重投不该改写工作范围。
func TestASecondOpenOfTheSameTaskIsAlreadyOpen(t *testing.T) {
	repository, transactor, _ := newDispatchTasks(t)
	ctx := t.Context()

	mustSaveDispatchTask(t, transactor, ctx, repository, dispatchTaskRecord(t, "DT-0003", "parcel-1"))

	var outcome ports.DispatchTaskSaveOutcome
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, dispatchTaskRecord(t, "DT-0003", "parcel-9"))
		return err
	})
	if outcome != ports.DispatchTaskAlreadyOpen {
		t.Fatalf("撞键 outcome = %d, want DispatchTaskAlreadyOpen", outcome)
	}

	found, _, err := repository.FindByKey(ctx, dispatchTaskKeyFixture(t, "tenant-1", "DT-0003"))
	if err != nil {
		t.Fatalf("取回任务：%v", err)
	}
	objects := found.Task.Objects()
	if len(objects) != 1 || objects[0].String() != "parcel-1" {
		t.Fatalf("重投顶替了原工作范围：%v", objects)
	}
}

// TestDispatchTasksAreScopedToTheirTenant 证同名任务在两个租户下互不可见。
func TestDispatchTasksAreScopedToTheirTenant(t *testing.T) {
	repository, transactor, _ := newDispatchTasks(t)
	ctx := t.Context()

	mustSaveDispatchTask(t, transactor, ctx, repository, dispatchTaskRecord(t, "DT-0004", "parcel-1"))

	if _, exists, err := repository.FindByKey(ctx, dispatchTaskKeyFixture(t, "tenant-2", "DT-0004")); err != nil || exists {
		t.Fatalf("他租户读到了这项任务：exists=%v err=%v", exists, err)
	}
}

// TestDispatchTaskWritesRefuseToRunOutsideATransaction 证写口无环境事务即拒——两张表必须
// 同笔落，没有对象的任务是领域读不回来的东西。
//
// 断言指名 ErrTransactionRequired 而不是 err != nil：后者会把「拒得对」与「因为别的原因也
// 失败了」混成一格，而这条用例要钉的恰恰是前者。
func TestDispatchTaskWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newDispatchTasks(t)

	_, err := repository.Save(t.Context(), dispatchTaskRecord(t, "DT-0005", "parcel-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestDispatchTaskRollbackLeavesNothingBehind 证首登与它所在的事务同生共死——半个任务
// （有任务无对象）落不下来。
func TestDispatchTaskRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor, _ := newDispatchTasks(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, dispatchTaskRecord(t, "DT-0006", "parcel-1")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if _, exists, err := repository.FindByKey(ctx, dispatchTaskKeyFixture(t, "tenant-1", "DT-0006")); err != nil || exists {
		t.Errorf("回滚后任务仍在：exists=%v err=%v", exists, err)
	}
}

// TestDispatchTaskCheckConstraintsRejectRowsTheDomainCannotProduce 证库内 CHECK 是第二道门：
// 绕过领域直接 INSERT 也落不进领域造不出的行。
func TestDispatchTaskCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	repository, transactor, pool := newDispatchTasks(t)
	ctx := t.Context()
	mustSaveDispatchTask(t, transactor, ctx, repository, dispatchTaskRecord(t, "DT-0007", "parcel-1"))

	base := `INSERT INTO transport_fulfillment.dispatch_task
	    (tenant_id, task_ref, kind, place_ref, window_from, window_to, conditions_ref,
	     opened_at, reschedules, state, closure_basis, closed_at, recorded_at) VALUES `
	for name, values := range map[string]string{
		"集外任务种类":  `('tenant-1','BAD-1','SCANNED','p',now(),now()+interval '1 hour','c',now(),0,'OPEN',NULL,NULL,now())`,
		"集外状态":    `('tenant-1','BAD-2','PICKUP','p',now(),now()+interval '1 hour','c',now(),0,'CANCELLED',NULL,NULL,now())`,
		"关了却没有依据": `('tenant-1','BAD-3','PICKUP','p',now(),now()+interval '1 hour','c',now(),0,'COMPLETED',NULL,now(),now())`,
		"关了却没有时刻": `('tenant-1','BAD-4','PICKUP','p',now(),now()+interval '1 hour','c',now(),0,'TERMINATED','b',NULL,now())`,
		"开放却带着依据": `('tenant-1','BAD-5','PICKUP','p',now(),now()+interval '1 hour','c',now(),0,'OPEN','b',now(),now())`,
		"窗口首尾颠倒":  `('tenant-1','BAD-6','PICKUP','p',now(),now()-interval '1 hour','c',now(),0,'OPEN',NULL,NULL,now())`,
		"改约次数为负":  `('tenant-1','BAD-7','PICKUP','p',now(),now()+interval '1 hour','c',now(),-1,'OPEN',NULL,NULL,now())`,
		"关闭早于建立":  `('tenant-1','BAD-8','PICKUP','p',now(),now()+interval '1 hour','c',now(),0,'COMPLETED','b',now()-interval '1 day',now())`,
		"缺服务条件":   `('tenant-1','BAD-9','PICKUP','p',now(),now()+interval '1 hour','',now(),0,'OPEN',NULL,NULL,now())`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, base+values); err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}

	// 对象行挂在一个不存在的任务上要撞外键——对象范围不能脱离任务独立存在。
	if _, err := pool.Exec(ctx,
		`INSERT INTO transport_fulfillment.dispatch_task_object
		     (tenant_id, task_ref, object_ref, recorded_at)
		 VALUES ('tenant-1','DT-NOT-THERE','parcel-1',now())`); err == nil {
		t.Fatal("无主的对象行落进去了")
	}
}

func newDispatchTasks(t *testing.T) (*adapter.DispatchTasks, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewDispatchTasks(db)
	if err != nil {
		t.Fatalf("构造任务登记库：%v", err)
	}
	return repository, db.Transactor(), pool
}

func mustWithinDispatchTaskTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func mustSaveDispatchTask(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.DispatchTasks,
	record ports.DispatchTaskRecord,
) {
	t.Helper()
	mustWithinDispatchTaskTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, record)
		return err
	})
}

func dispatchTaskKeyFixture(t *testing.T, tenant, task string) ports.DispatchTaskKey {
	t.Helper()
	return ports.DispatchTaskKey{
		TenantID: segmentRef(t, domain.NewTenantID, tenant),
		Task:     segmentRef(t, domain.NewDispatchTaskReference, task),
	}
}

func dispatchTaskRecord(t *testing.T, task string, objects ...string) ports.DispatchTaskRecord {
	t.Helper()
	members := make([]domain.CarriedObjectReference, 0, len(objects))
	for _, object := range objects {
		members = append(members, segmentRef(t, domain.NewCarriedObjectReference, object))
	}
	opened, err := domain.OpenDispatchTask(domain.DispatchTaskSpec{
		TenantID:   segmentRef(t, domain.NewTenantID, "tenant-1"),
		Task:       segmentRef(t, domain.NewDispatchTaskReference, task),
		Kind:       domain.PickupDispatch,
		Objects:    members,
		Place:      segmentRef(t, domain.NewAttemptPlaceReference, "customer-warehouse-1"),
		WindowFrom: taskWindowFixture,
		WindowTo:   taskWindowFixture.Add(3 * time.Hour),
		Conditions: segmentRef(t, domain.NewServiceConditionReference, "service-condition/v1"),
		OpenedAt:   taskOpenedAtFixture,
	})
	if err != nil {
		t.Fatalf("建立任务夹具：%v", err)
	}
	return ports.DispatchTaskRecord{
		Key:        dispatchTaskKeyFixture(t, "tenant-1", task),
		Task:       opened,
		RecordedAt: taskOpenedAtFixture.Add(time.Minute),
	}
}
