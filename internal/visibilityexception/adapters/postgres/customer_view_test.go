package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件对真实 PostgreSQL 16 证客户视图库的行为：版本推进的乐观锁由 WHERE 条件承担、
// 双重隔离（租户+账户）由 SQL 条件承担、逐维内容在场规则由迁移 CHECK 把关、事务纪律
// 由 RequireExecutor 拦住。断言一律在事务闭包外（Goexit 会挂死连接）。

var publishedAt = time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)

func newCustomerViews(t *testing.T) (*adapter.CustomerViews, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCustomerViews(db)
	if err != nil {
		t.Fatalf("构造视图库：%v", err)
	}
	return repository, db.Transactor()
}

func within2(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func build[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func mixedDimensions(t *testing.T) domain.CustomerViewDimensions {
	t.Helper()
	milestones, err := domain.ShowDimension(build(t, domain.NewViewContentReference, "milestones/v1"))
	if err != nil {
		t.Fatalf("展示维：%v", err)
	}
	return domain.CustomerViewDimensions{
		Milestones: milestones,
		ETA:        domain.PendDimension(),
		Final:      domain.WithholdDimension(),
		Note:       domain.PendDimension(),
	}
}

func firstView(t *testing.T, version string) domain.CustomerTrackingView {
	t.Helper()
	view, err := domain.PublishCustomerView(
		build(t, domain.NewCustomerViewVersionID, version),
		build(t, domain.NewCustomerAccountReference, "customer-a"),
		build(t, domain.NewTrackedParcelReference, "parcel-1"),
		build(t, domain.NewProjectionVersionID, "projection-1"),
		mixedDimensions(t),
		publishedAt,
	)
	if err != nil {
		t.Fatalf("发布首版：%v", err)
	}
	return view
}

func tenantOf(t *testing.T, raw string) domain.TenantID {
	t.Helper()
	return build(t, domain.NewTenantID, raw)
}

// TestACurrentViewIsReadBackUnchanged 证首版原样读回：三态维各归各格（展示带内容、
// 待确认与不展示无内容）、版本与投影锚在场、无前版。
func TestACurrentViewIsReadBackUnchanged(t *testing.T) {
	repository, transactor := newCustomerViews(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	view := firstView(t, "view-1")
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, view)
	})

	found, exists, err := repository.FindCurrent(ctx, tenant, view.Customer(), view.Parcel())
	if err != nil || !exists {
		t.Fatalf("取回当前视图：%v exists=%v", err, exists)
	}
	if found.Version() != view.Version() || found.BasedOn() != view.BasedOn() {
		t.Errorf("版本或投影锚读回变形：%+v", found)
	}
	dimensions := found.Dimensions()
	if content, shown := dimensions.Milestones.Content(); !shown || content.String() != "milestones/v1" {
		t.Errorf("展示维读回变形：%v shown=%v", content, shown)
	}
	if dimensions.ETA.State() != domain.DimensionPendingConfirmation ||
		dimensions.Final.State() != domain.DimensionNotDisclosed {
		t.Errorf("非展示维读回变形：eta=%q final=%q", dimensions.ETA.State(), dimensions.Final.State())
	}
	if _, superseding := found.PriorVersion(); superseding {
		t.Error("首版凭空长出了前版")
	}
	if !found.PublishedAt().Equal(view.PublishedAt()) {
		t.Errorf("发布时间 = %v，应为 %v", found.PublishedAt(), view.PublishedAt())
	}
}

// TestSupersedeAdvancesTheVersionWithOptimisticLock 证替代是同键版本推进：新版本指回
// 前版并成为当前；拿着已被替代的旧前版再替代（乐观锁失手）拒绝而不覆盖；首发撞已有
// 当前同样拒绝。
func TestSupersedeAdvancesTheVersionWithOptimisticLock(t *testing.T) {
	repository, transactor := newCustomerViews(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	first := firstView(t, "view-1")
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, first)
	})

	second, err := first.Supersede(
		build(t, domain.NewCustomerViewVersionID, "view-2"),
		build(t, domain.NewProjectionVersionID, "projection-2"),
		mixedDimensions(t),
		publishedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("形成替代版本：%v", err)
	}
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, second)
	})

	found, exists, err := repository.FindCurrent(ctx, tenant, first.Customer(), first.Parcel())
	if err != nil || !exists {
		t.Fatalf("取回当前视图：%v exists=%v", err, exists)
	}
	if found.Version() != second.Version() {
		t.Errorf("当前版本 = %q，应为替代后的 %q", found.Version(), second.Version())
	}
	prior, superseding := found.PriorVersion()
	if !superseding || prior != first.Version() {
		t.Errorf("前版指回读回变形：%q superseding=%v", prior, superseding)
	}

	// 乐观锁：拿旧前版（view-1）再造一个替代——当前已是 view-2，零行命中即拒。
	stale, err := first.Supersede(
		build(t, domain.NewCustomerViewVersionID, "view-3"),
		build(t, domain.NewProjectionVersionID, "projection-3"),
		mixedDimensions(t),
		publishedAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("形成过期替代：%v", err)
	}
	staleErr := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, stale)
	})
	if !errors.Is(staleErr, adapter.ErrCurrentViewChanged) {
		t.Fatalf("过期替代应拒 ErrCurrentViewChanged，实得：%v", staleErr)
	}

	// 首发撞已有当前：同样拒绝，不覆盖版本链。
	duplicate := firstView(t, "view-9")
	duplicateErr := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, duplicate)
	})
	if !errors.Is(duplicateErr, adapter.ErrCurrentViewChanged) {
		t.Fatalf("首发撞已有应拒 ErrCurrentViewChanged，实得：%v", duplicateErr)
	}

	kept, _, err := repository.FindCurrent(ctx, tenant, first.Customer(), first.Parcel())
	if err != nil {
		t.Fatalf("复读当前视图：%v", err)
	}
	if kept.Version() != second.Version() {
		t.Errorf("输家覆盖了当前版本：%q", kept.Version())
	}
}

// TestOtherScopesAreInvisible 证跨账户与跨租户的否定结果与「不存在」长得完全一样
// ——HTTP 查询端点逐字节同答的持久化半边。
func TestOtherScopesAreInvisible(t *testing.T) {
	repository, transactor := newCustomerViews(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	view := firstView(t, "view-1")
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, view)
	})

	otherCustomer := build(t, domain.NewCustomerAccountReference, "customer-b")
	if _, exists, err := repository.FindCurrent(ctx, tenant, otherCustomer, view.Parcel()); err != nil || exists {
		t.Errorf("跨账户探针：err=%v exists=%v，应与不存在同答", err, exists)
	}
	if _, exists, err := repository.FindCurrent(ctx, tenantOf(t, "tenant-b"), view.Customer(), view.Parcel()); err != nil || exists {
		t.Errorf("跨租户探针：err=%v exists=%v，应与不存在同答", err, exists)
	}
}

// TestWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池。
func TestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _ := newCustomerViews(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	view := firstView(t, "view-1")
	if err := repository.Save(ctx, tenant, view); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}

	_, exists, err := repository.FindCurrent(ctx, tenant, view.Customer(), view.Parcel())
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("被拒绝的写入仍然落库了")
	}
}

// TestRollbackLeavesNothingBehind 证视图提交与它所在的事务同生共死。
func TestRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor := newCustomerViews(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	rollback := errors.New("回滚")

	view := firstView(t, "view-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repository.Save(txCtx, tenant, view); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindCurrent(ctx, tenant, view.Customer(), view.Parcel())
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后视图仍在")
	}
}
