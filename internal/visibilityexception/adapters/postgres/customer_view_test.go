package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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
	repository, transactor, _ := newCustomerViewsOnPool(t)
	return repository, transactor
}

func newCustomerViewsOnPool(t *testing.T) (*adapter.CustomerViews, bentoapp.Transactor, *pgxpool.Pool) {
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
	return repository, db.Transactor(), pool
}

func countCustomerViewVersions(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.customer_view_version`).Scan(&n); err != nil {
		t.Fatalf("数版本行：%v", err)
	}
	return n
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
	repository, transactor, pool := newCustomerViewsOnPool(t)
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
	// 输家写下的版本行随它的事务回滚：不留一版从未成为当前的「已发布」视图。
	if n := countCustomerViewVersions(t, pool); n != 2 {
		t.Errorf("版本行 = %d，应仍为 2：输家不得留下版本行", n)
	}
	for _, loser := range []domain.CustomerViewVersionID{stale.Version(), duplicate.Version()} {
		if _, exists, err := repository.FindVersion(ctx, tenant, first.Customer(), first.Parcel(), loser); err != nil || exists {
			t.Errorf("输家签出的版本 %q 读得回：err=%v exists=%v", loser, err, exists)
		}
	}
}

// TestRewritingAStoredVersionIsRefusedByItsKey 证同一版本号再写一次以主键冲突报错暴露，
// 不静默覆盖已发布的那一版，也不被读成乐观锁失手（那会让调用方重读后再走一遍）。
func TestRewritingAStoredVersionIsRefusedByItsKey(t *testing.T) {
	repository, transactor := newCustomerViews(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	first := firstView(t, "view-1")
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, first)
	})
	rewritten, err := domain.PublishCustomerView(
		first.Version(), first.Customer(), first.Parcel(),
		build(t, domain.NewProjectionVersionID, "projection-2"),
		domain.CustomerViewDimensions{
			Milestones: domain.PendDimension(),
			ETA:        domain.PendDimension(),
			Final:      domain.PendDimension(),
			Note:       domain.PendDimension(),
		},
		publishedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("形成同号的另一版：%v", err)
	}
	rewriteErr := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, rewritten)
	})
	var pgErr *pgconn.PgError
	if !errors.As(rewriteErr, &pgErr) || pgErr.Code != "23505" ||
		pgErr.ConstraintName != "customer_view_version_pkey" {
		t.Fatalf("同号重写应以版本主键冲突报错，实得：%v", rewriteErr)
	}
	if errors.Is(rewriteErr, adapter.ErrCurrentViewChanged) {
		t.Fatal("同号重写被读成了乐观锁失手")
	}
	kept, exists, err := repository.FindVersion(ctx, tenant, first.Customer(), first.Parcel(), first.Version())
	if err != nil || !exists || kept.Dimensions() != first.Dimensions() || kept.BasedOn() != first.BasedOn() {
		t.Fatalf("已发布的那一版被改写：%+v exists=%v err=%v", kept, exists, err)
	}
}

// TestASupersededVersionStaysReadableByVersion 证替代不改写已发布的那一版：替代之后前一版
// 按版本原样读得回（四维、投影锚、发布时间都是它发布时的样子），当前版的前版指回指着
// 一行存下来的版本；按版本读回同样带着账户，别的账户拿同一版本号读不到。
// Covers: VE CONTEXT「已经向客户发布的里程碑、ETA、异常说明或通知出现更正时，必须以追加
// 更新或明确替代关系表达，不得静默删除或改写历史披露」；点名 `AT-VE-161` 的持久化半边
// 「原版本保留且不再作为当前结果」。
func TestASupersededVersionStaysReadableByVersion(t *testing.T) {
	repository, transactor, pool := newCustomerViewsOnPool(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	first := firstView(t, "view-1")
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, first)
	})
	final, err := domain.ShowDimension(build(t, domain.NewViewContentReference, "final/v1"))
	if err != nil {
		t.Fatalf("展示维：%v", err)
	}
	changed := mixedDimensions(t)
	changed.Final = final
	second, err := first.Supersede(
		build(t, domain.NewCustomerViewVersionID, "view-2"),
		build(t, domain.NewProjectionVersionID, "projection-2"),
		changed,
		publishedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("形成替代版本：%v", err)
	}
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, second)
	})

	current, exists, err := repository.FindCurrent(ctx, tenant, first.Customer(), first.Parcel())
	if err != nil || !exists {
		t.Fatalf("取回当前视图：%v exists=%v", err, exists)
	}
	prior, superseding := current.PriorVersion()
	if current.Version() != second.Version() || !superseding || prior != first.Version() {
		t.Fatalf("当前版 = %q 前版 = %q，应为 view-2 指回 view-1", current.Version(), prior)
	}

	kept, exists, err := repository.FindVersion(ctx, tenant, first.Customer(), first.Parcel(), prior)
	if err != nil || !exists {
		t.Fatalf("前版指回指着一行读不回的版本：%v exists=%v", err, exists)
	}
	if kept.Dimensions() != first.Dimensions() || kept.BasedOn() != first.BasedOn() ||
		!kept.PublishedAt().Equal(first.PublishedAt()) {
		t.Errorf("已发布的前一版被改写：%+v", kept)
	}
	if _, superseding := kept.PriorVersion(); superseding {
		t.Error("首版读回时凭空长出了前版")
	}
	if n := countCustomerViewVersions(t, pool); n != 2 {
		t.Errorf("版本行 = %d，应为 2：替代追加一行，不改写原行", n)
	}

	otherCustomer := build(t, domain.NewCustomerAccountReference, "customer-b")
	if _, exists, err := repository.FindVersion(ctx, tenant, otherCustomer, first.Parcel(), prior); err != nil || exists {
		t.Errorf("跨账户按版本探针：err=%v exists=%v，应与不存在同答", err, exists)
	}
	if _, exists, err := repository.FindVersion(ctx, tenantOf(t, "tenant-b"), first.Customer(), first.Parcel(), prior); err != nil || exists {
		t.Errorf("跨租户按版本探针：err=%v exists=%v，应与不存在同答", err, exists)
	}
}

// TestReaffirmMovesOnlyTheCurrentBasis 证四维不变时视图不换代：重新确认之后当前版仍是
// 同一版、确认依据跟上新投影，版本行不多一行也不改一列（形成时依据不动）；
// 拿着已过时的原依据再确认——别的派生已先跟上——零行命中即拒，不覆盖。
// Covers: VE CONTEXT「客户全程追踪视图的版本由披露更正与替代关系驱动，不与内部投影版本
// 一一对应；投影重新派生本身不要求形成新的客户视图版本」的持久化半边。
func TestReaffirmMovesOnlyTheCurrentBasis(t *testing.T) {
	repository, transactor, pool := newCustomerViewsOnPool(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	first := firstView(t, "view-1")
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, first)
	})
	reaffirmed, err := first.Reaffirm(build(t, domain.NewProjectionVersionID, "projection-2"), first.Dimensions())
	if err != nil {
		t.Fatalf("重新确认：%v", err)
	}
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return repository.Reaffirm(txCtx, tenant, reaffirmed, first.BasedOn())
	})

	current, exists, err := repository.FindCurrent(ctx, tenant, first.Customer(), first.Parcel())
	if err != nil || !exists {
		t.Fatalf("取回当前视图：%v exists=%v", err, exists)
	}
	if current.Version() != first.Version() || current.ConfirmedOn().String() != "projection-2" {
		t.Fatalf("当前版 = %q 确认依据 = %q，应为同一版跟上 projection-2", current.Version(), current.ConfirmedOn())
	}
	if current.BasedOn() != first.BasedOn() {
		t.Errorf("当前版的形成时依据 = %q，应仍是 %q", current.BasedOn(), first.BasedOn())
	}
	if current.Dimensions() != first.Dimensions() || !current.PublishedAt().Equal(first.PublishedAt()) {
		t.Error("重新确认改写了已发布的四维或发布时间")
	}
	formed, exists, err := repository.FindVersion(ctx, tenant, first.Customer(), first.Parcel(), first.Version())
	if err != nil || !exists {
		t.Fatalf("按版本读回：%v exists=%v", err, exists)
	}
	if formed.BasedOn() != first.BasedOn() || formed.ConfirmedOn().String() != "projection-2" {
		t.Errorf("按版本读回当前版：形成时依据 = %q 确认依据 = %q，应为 %q / projection-2",
			formed.BasedOn(), formed.ConfirmedOn(), first.BasedOn())
	}
	if n := countCustomerViewVersions(t, pool); n != 1 {
		t.Errorf("版本行 = %d，应为 1：不换代就不多一行", n)
	}

	stale, err := first.Reaffirm(build(t, domain.NewProjectionVersionID, "projection-3"), first.Dimensions())
	if err != nil {
		t.Fatalf("形成过期的重新确认：%v", err)
	}
	staleErr := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Reaffirm(txCtx, tenant, stale, first.BasedOn())
	})
	if !errors.Is(staleErr, adapter.ErrCurrentViewChanged) {
		t.Fatalf("按过时依据重新确认应拒 ErrCurrentViewChanged，实得：%v", staleErr)
	}
	kept, _, err := repository.FindCurrent(ctx, tenant, first.Customer(), first.Parcel())
	if err != nil {
		t.Fatalf("复读当前视图：%v", err)
	}
	if kept.ConfirmedOn().String() != "projection-2" {
		t.Errorf("输家覆盖了当前确认依据：%q", kept.ConfirmedOn())
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

// TestWritesRefuseToRunOutsideATransaction 证写入（保存与重新确认）不会在缺少事务时改用连接池。
func TestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _ := newCustomerViews(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")

	view := firstView(t, "view-1")
	if err := repository.Save(ctx, tenant, view); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}
	reaffirmed, err := view.Reaffirm(build(t, domain.NewProjectionVersionID, "projection-2"), view.Dimensions())
	if err != nil {
		t.Fatalf("重新确认：%v", err)
	}
	if err := repository.Reaffirm(ctx, tenant, reaffirmed, view.BasedOn()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务重新确认应返回 ErrTransactionRequired，实得：%v", err)
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
