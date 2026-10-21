package postgres_test

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证接受前财务控制两本账的持久化行为：整册往返后幂等
// 判定与续编号完好、作用域隔离由 SQL 条件承担、释放是同键状态推进、无事务拒、回滚
// 无痕。两本账分别证——它们互不借用（ADR-0047）。

var frozenAtFixture = time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC)

// TestAFreezeLedgerRoundTripsWithItsAlgebra 证整册往返后账本的三条代数原样成立：
// 重放返回原冻结（含原编号）、同身份异内容冲突、续编号不与历史重号。
func TestAFreezeLedgerRoundTripsWithItsAlgebra(t *testing.T) {
	repository, _, transactor, _ := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	ledger := domain.NewFreezeLedger()
	first := mustFreeze(t, ledger, freezeRequest(t, "control-1", 3000), saBalance(t, 10000))
	second := mustFreeze(t, ledger, freezeRequest(t, "control-2", 4000), saBalance(t, 10000))
	if _, err := ledger.Release(second.FreezeID(), frozenAtFixture.Add(time.Hour)); err != nil {
		t.Fatalf("释放：%v", err)
	}
	mustSaveFreezeLedger(t, transactor, ctx, repository, tenant, scope, ledger)

	reloaded, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("整册读回：%v", err)
	}
	if len(reloaded.Entries()) != 2 {
		t.Fatalf("entries = %d, want 2", len(reloaded.Entries()))
	}

	// 重放：同请求同内容返回原冻结（原编号原金额）。
	replayed, err := reloaded.Freeze(freezeRequest(t, "control-1", 3000), saBalance(t, 10000))
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replayed.FreezeID() != first.FreezeID() || replayed.AmountMinor() != 3000 {
		t.Fatalf("重放没有返回原冻结：%+v", replayed)
	}

	// 冲突：同请求身份携带不同金额——指纹随册往返，冲突判定不得退化成放行。
	if _, err := reloaded.Freeze(freezeRequest(t, "control-1", 9999), saBalance(t, 100000)); !errors.Is(err, domain.ErrControlRequestConflict) {
		t.Fatalf("err = %v, want ErrControlRequestConflict（指纹丢了）", err)
	}

	// 续编号：重建后的下一笔不得与历史重号。
	third := mustFreeze(t, reloaded, freezeRequest(t, "control-3", 500), saBalance(t, 10000))
	if third.FreezeID() == first.FreezeID() || third.FreezeID() == second.FreezeID() {
		t.Fatalf("续编号与历史重号：%s", third.FreezeID())
	}

	// 已释放那条的答案随册带回：再次释放返回与首次相同的答案，包括原释放时间。
	releasedAgain, err := reloaded.Release(second.FreezeID(), frozenAtFixture.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("重复释放：%v", err)
	}
	if !releasedAgain.ReleasedAt().Equal(frozenAtFixture.Add(time.Hour)) {
		t.Fatalf("重复释放换了释放时间：%s", releasedAgain.ReleasedAt())
	}
}

// TestFreezeScopesAreInvisibleToEachOther 证否定结果不泄露其他作用域：别的租户、
// 别的结算账户读回的是空册，与「从未冻结过」长得完全一样。
func TestFreezeScopesAreInvisibleToEachOther(t *testing.T) {
	repository, _, transactor, _ := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	ledger := domain.NewFreezeLedger()
	mustFreeze(t, ledger, freezeRequest(t, "control-1", 3000), saBalance(t, 10000))
	mustSaveFreezeLedger(t, transactor, ctx, repository, tenant, scope, ledger)

	otherTenant, err := repository.LoadForScope(ctx, saTenant(t, "tenant-b"), scope)
	if err != nil {
		t.Fatalf("他租户读册：%v", err)
	}
	if len(otherTenant.Entries()) != 0 {
		t.Error("他租户读到了本租户的冻结")
	}

	otherAccount := saScopeWithAccount(t, "account-b")
	otherLedger, err := repository.LoadForScope(ctx, tenant, otherAccount)
	if err != nil {
		t.Fatalf("他账户读册：%v", err)
	}
	if len(otherLedger.Entries()) != 0 {
		t.Error("他账户读到了本账户的冻结")
	}
}

// TestSavingTwiceIsIdempotentAndReleaseAdvancesInPlace 证重复保存同一册幂等（行数
// 稳定）、释放是同键状态推进（不长第二行、原金额原冻结时间不动）。
func TestSavingTwiceIsIdempotentAndReleaseAdvancesInPlace(t *testing.T) {
	repository, _, transactor, pool := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	ledger := domain.NewFreezeLedger()
	frozen := mustFreeze(t, ledger, freezeRequest(t, "control-1", 3000), saBalance(t, 10000))
	mustSaveFreezeLedger(t, transactor, ctx, repository, tenant, scope, ledger)
	mustSaveFreezeLedger(t, transactor, ctx, repository, tenant, scope, ledger)
	if count := countFreezeRows(t, pool); count != 1 {
		t.Fatalf("重复保存后行数 = %d, want 1", count)
	}

	if _, err := ledger.Release(frozen.FreezeID(), frozenAtFixture.Add(time.Hour)); err != nil {
		t.Fatalf("释放：%v", err)
	}
	mustSaveFreezeLedger(t, transactor, ctx, repository, tenant, scope, ledger)
	if count := countFreezeRows(t, pool); count != 1 {
		t.Fatalf("释放后行数 = %d, want 1（同键状态推进不长行）", count)
	}

	reloaded, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	entry := reloaded.Entries()[0]
	if entry.Status() != domain.FreezeReleased ||
		entry.AmountMinor() != 3000 ||
		!entry.FrozenAt().Equal(frozenAtFixture) {
		t.Fatalf("状态推进动了不该动的列：%+v", entry)
	}
}

// TestRestrictedControlsNeverReachTheTable 证`业务限制`从不入库：超额冻结交回结果但
// 账本无记录，保存后表里没有它的行——那次控制没占用资金，也就没有可释放的东西。
func TestRestrictedControlsNeverReachTheTable(t *testing.T) {
	repository, _, transactor, pool := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	ledger := domain.NewFreezeLedger()
	restricted, err := ledger.Freeze(freezeRequest(t, "control-over", 999999), saBalance(t, 10000))
	if err != nil {
		t.Fatalf("超额冻结：%v", err)
	}
	if restricted.Status() != domain.FreezeRestricted {
		t.Fatalf("status = %s", restricted.Status())
	}
	mustSaveFreezeLedger(t, transactor, ctx, repository, tenant, scope, ledger)
	if count := countFreezeRows(t, pool); count != 0 {
		t.Fatalf("业务限制入了库：rows = %d", count)
	}
}

// TestCreditExposureLedgerRoundTripsSeparately 证另一本账独立往返：暴露、释放、指纹
// 与续编号同代数，但表与类型都与冻结账本分立（ADR-0047 两轨）。
func TestCreditExposureLedgerRoundTripsSeparately(t *testing.T) {
	_, repository, transactor, _ := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	ledger := domain.NewCreditExposureLedger()
	first, err := ledger.Expose(exposureRequest(t, "control-1", 2000), saStanding(t, 8000, false))
	if err != nil {
		t.Fatalf("暴露：%v", err)
	}
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, scope, ledger)
	}); err != nil {
		t.Fatalf("保存暴露册：%v", err)
	}

	reloaded, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回暴露册：%v", err)
	}
	replayed, err := reloaded.Expose(exposureRequest(t, "control-1", 2000), saStanding(t, 8000, false))
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replayed.ExposureID() != first.ExposureID() {
		t.Fatal("重放没有返回原暴露")
	}
	if _, err := reloaded.Expose(exposureRequest(t, "control-1", 2500), saStanding(t, 8000, false)); !errors.Is(err, domain.ErrControlRequestConflict) {
		t.Fatalf("err = %v, want ErrControlRequestConflict", err)
	}
	second, err := reloaded.Expose(exposureRequest(t, "control-2", 100), saStanding(t, 8000, false))
	if err != nil {
		t.Fatalf("续编号暴露：%v", err)
	}
	if second.ExposureID() == first.ExposureID() {
		t.Fatal("续编号与历史重号")
	}
}

// TestACreditExposureRoundTripsTheAdoptedCreditPolicy 证暴露行上的政策引用随册往返——CONTEXT
// 「每项信用暴露保存实际采用的政策」的落库半边（ADR-0127 Consequences 点名留给 SA 迁移的那一格）：
// 按授权额度形成的暴露读回后仍答得出出自哪一版信用政策；迁移之前写下的存量行没有这一列的值，读回
// 为空而不是拒——可空只为存量行，新写入非空由应用层保证（编排只经 WithAuthorizedLimit 进 Expose）。
// 混着存量行的整册再保存同样成立：释放存量行那条不碰政策列，也不给它编一个出处。
func TestACreditExposureRoundTripsTheAdoptedCreditPolicy(t *testing.T) {
	_, repository, transactor, pool := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)
	policy := saValue(t, domain.NewCreditPolicyReference, "PC-CREDIT-POLICY/v3")

	authorized, err := saStanding(t, 0, false).WithAuthorizedLimit(8000, policy)
	if err != nil {
		t.Fatalf("授权额度：%v", err)
	}
	ledger := domain.NewCreditExposureLedger()
	judged, err := ledger.Expose(exposureRequest(t, "control-1", 2000), authorized)
	if err != nil {
		t.Fatalf("暴露：%v", err)
	}
	if judged.Policy() != policy {
		t.Fatalf("形成时就没带出处：%q", judged.Policy())
	}
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, scope, ledger)
	}); err != nil {
		t.Fatalf("保存暴露册：%v", err)
	}

	// 存量行：政策引用列加进来之前的行，直接以 SQL 落一条、不经适配器——适配器今天不会再写出这种行。
	// 指纹只要求非空且是本适配器的十六进制形，内容对本例无关（不重放它）。
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.credit_exposure
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 exposure_id, status, amount_minor, association, request_digest, exposed_at, released_at,
			 credit_policy_ref)
		 VALUES ($1, $2, $3, $4, 'control-legacy', 'EXP-0002', 1, 300, 'SAC-control-legacy', $5, $6, NULL, NULL)`,
		tenant.String(), scope.LegalEntity().String(), scope.Account().String(), scope.Currency().String(),
		hex.EncodeToString([]byte("legacy-digest")), frozenAtFixture,
	); err != nil {
		t.Fatalf("写存量行：%v", err)
	}

	reloaded, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回暴露册：%v", err)
	}
	if len(reloaded.Entries()) != 2 {
		t.Fatalf("entries = %d, want 2", len(reloaded.Entries()))
	}
	kept, found := reloaded.FindByRequest(judged.RequestID())
	if !found || kept.Policy() != policy {
		t.Fatalf("读回的暴露 policy = %q, want %q——政策引用没有随行落库", kept.Policy(), policy)
	}
	legacy, found := reloaded.FindByRequest(saValue(t, domain.NewControlRequestID, "control-legacy"))
	if !found || legacy.Policy().String() != "" || legacy.Status() != domain.ExposureRecorded {
		t.Fatalf("存量行读回 = %+v / policy %q, want RECORDED 且出处为空", legacy, legacy.Policy())
	}

	// 混册再保存：释放存量行，整册写回；政策列两行都不动。
	if _, err := reloaded.Release(legacy.ExposureID(), frozenAtFixture.Add(time.Hour)); err != nil {
		t.Fatalf("释放存量行：%v", err)
	}
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, scope, reloaded)
	}); err != nil {
		t.Fatalf("混册再保存：%v", err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM settlement_accounting.credit_exposure`).Scan(&rows); err != nil {
		t.Fatalf("统计暴露行数：%v", err)
	}
	if rows != 2 {
		t.Fatalf("混册再保存后行数 = %d, want 2", rows)
	}
	again, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("再读回：%v", err)
	}
	kept, _ = again.FindByRequest(judged.RequestID())
	legacy, _ = again.FindByRequest(saValue(t, domain.NewControlRequestID, "control-legacy"))
	if kept.Policy() != policy || legacy.Policy().String() != "" || legacy.Status() != domain.ExposureReleased {
		t.Fatalf("再保存改了政策列：kept %q / legacy %q (%s)", kept.Policy(), legacy.Policy(), legacy.Status())
	}
}

// TestControlWritesRefuseToRunOutsideATransaction 证两本账的写入都不会在缺少事务时
// 改用连接池。
func TestControlWritesRefuseToRunOutsideATransaction(t *testing.T) {
	freezes, exposures, _, _ := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	ledger := domain.NewFreezeLedger()
	mustFreeze(t, ledger, freezeRequest(t, "control-1", 3000), saBalance(t, 10000))
	if err := freezes.Save(ctx, tenant, scope, ledger); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存冻结册应返回 ErrTransactionRequired，实得：%v", err)
	}

	exposureLedger := domain.NewCreditExposureLedger()
	if _, err := exposureLedger.Expose(exposureRequest(t, "control-1", 2000), saStanding(t, 8000, false)); err != nil {
		t.Fatalf("暴露：%v", err)
	}
	if err := exposures.Save(ctx, tenant, scope, exposureLedger); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存暴露册应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestTheScopeLockRefusesToRunOutsideATransaction 证作用域锁不在连接池上取：在池上取，锁随那条语句的自动提交当场放掉，读改写
// 照样两路并发而调用方以为排过队了。
func TestTheScopeLockRefusesToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	locks, err := adapter.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		t.Fatalf("构造作用域锁：%v", err)
	}
	if err := locks.LockControlScope(t.Context(), saTenant(t, "tenant-1"), saScope(t)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务取作用域锁应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return locks.LockControlScope(txCtx, saTenant(t, "tenant-1"), saScope(t))
	}); err != nil {
		t.Fatalf("事务内取作用域锁：%v", err)
	}
}

// Covers: SA CONTEXT「重复请求、查询和补偿不得形成重复冻结或重复释放」在账本写口这一层的半边——一份读得比库里旧的整册快照
// 写回时，库里已释放的那一行不被改回占着、原释放时间不动；没变的行一行都不写（行版本不变），只有新入册的那一笔与真正的
// 释放推进落库。编排层排队之外还守这一层：绕过编排、拿着旧快照写回的那一路也压不回别人的释放。
func TestAStaleSnapshotNeitherRevivesAReleaseNorRewritesUnchangedEntries(t *testing.T) {
	freezes, exposures, transactor, pool := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)
	releasedAt := frozenAtFixture.Add(time.Hour)

	t.Run("冻结", func(t *testing.T) {
		ledger := domain.NewFreezeLedger()
		target := mustFreeze(t, ledger, freezeRequest(t, "control-X", 3000), saBalance(t, 10000))
		mustFreeze(t, ledger, freezeRequest(t, "control-W", 2000), saBalance(t, 10000))
		mustSaveFreezeLedger(t, transactor, ctx, freezes, tenant, scope, ledger)

		stale, err := freezes.LoadForScope(ctx, tenant, scope)
		if err != nil {
			t.Fatalf("读旧快照：%v", err)
		}
		current, err := freezes.LoadForScope(ctx, tenant, scope)
		if err != nil {
			t.Fatalf("读当前账：%v", err)
		}
		if _, err := current.Release(target.FreezeID(), releasedAt); err != nil {
			t.Fatalf("释放 X：%v", err)
		}
		mustSaveFreezeLedger(t, transactor, ctx, freezes, tenant, scope, current)
		before := rowVersions(t, pool, "funds_freeze")

		mustFreeze(t, stale, freezeRequest(t, "control-Y", 1000), saBalance(t, 10000))
		mustSaveFreezeLedger(t, transactor, ctx, freezes, tenant, scope, stale)

		reloaded, err := freezes.LoadForScope(ctx, tenant, scope)
		if err != nil {
			t.Fatalf("读回：%v", err)
		}
		kept, _ := reloaded.FindByRequest(target.RequestID())
		if kept.Status() != domain.FreezeReleased || !kept.ReleasedAt().Equal(releasedAt) {
			t.Fatalf("X = %s（释放于 %v），want RELEASED 于 %v——旧快照把释放压回去了", kept.Status(), kept.ReleasedAt(), releasedAt)
		}
		if added, found := reloaded.FindByRequest(saValue(t, domain.NewControlRequestID, "control-Y")); !found || added.Status() != domain.FreezeHeld {
			t.Fatalf("Y 在册 %t、状态 %s，want 新入册的那一笔落库", found, added.Status())
		}
		after := rowVersions(t, pool, "funds_freeze")
		for _, request := range []string{"control-X", "control-W"} {
			if before[request] != after[request] {
				t.Errorf("%s 没变却被写了一遍（行版本 %s → %s）", request, before[request], after[request])
			}
		}

		mustSaveFreezeLedger(t, transactor, ctx, freezes, tenant, scope, reloaded)
		if again := rowVersions(t, pool, "funds_freeze"); !sameVersions(after, again) {
			t.Errorf("原样再存一遍改了行版本：%v → %v", after, again)
		}
	})

	t.Run("暴露", func(t *testing.T) {
		ledger := domain.NewCreditExposureLedger()
		target, err := ledger.Expose(exposureRequest(t, "control-X", 3000), saStanding(t, 10000, false))
		if err != nil {
			t.Fatalf("暴露 X：%v", err)
		}
		if _, err := ledger.Expose(exposureRequest(t, "control-W", 2000), saStanding(t, 10000, false)); err != nil {
			t.Fatalf("暴露 W：%v", err)
		}
		saveExposures := func(ledger *domain.CreditExposureLedger) {
			t.Helper()
			saWithin(t, transactor, ctx, func(txCtx context.Context) error {
				return exposures.Save(txCtx, tenant, scope, ledger)
			})
		}
		saveExposures(ledger)

		stale, err := exposures.LoadForScope(ctx, tenant, scope)
		if err != nil {
			t.Fatalf("读旧快照：%v", err)
		}
		current, err := exposures.LoadForScope(ctx, tenant, scope)
		if err != nil {
			t.Fatalf("读当前账：%v", err)
		}
		if _, err := current.Release(target.ExposureID(), releasedAt); err != nil {
			t.Fatalf("释放 X：%v", err)
		}
		saveExposures(current)
		before := rowVersions(t, pool, "credit_exposure")

		if _, err := stale.Expose(exposureRequest(t, "control-Y", 1000), saStanding(t, 10000, false)); err != nil {
			t.Fatalf("暴露 Y：%v", err)
		}
		saveExposures(stale)

		reloaded, err := exposures.LoadForScope(ctx, tenant, scope)
		if err != nil {
			t.Fatalf("读回：%v", err)
		}
		kept, _ := reloaded.FindByRequest(target.RequestID())
		if kept.Status() != domain.ExposureReleased || !kept.ReleasedAt().Equal(releasedAt) {
			t.Fatalf("X = %s（释放于 %v），want RELEASED 于 %v——旧快照把释放压回去了", kept.Status(), kept.ReleasedAt(), releasedAt)
		}
		if added, found := reloaded.FindByRequest(saValue(t, domain.NewControlRequestID, "control-Y")); !found || added.Status() != domain.ExposureRecorded {
			t.Fatalf("Y 在册 %t、状态 %s，want 新入册的那一笔落库", found, added.Status())
		}
		after := rowVersions(t, pool, "credit_exposure")
		for _, request := range []string{"control-X", "control-W"} {
			if before[request] != after[request] {
				t.Errorf("%s 没变却被写了一遍（行版本 %s → %s）", request, before[request], after[request])
			}
		}

		saveExposures(reloaded)
		if again := rowVersions(t, pool, "credit_exposure"); !sameVersions(after, again) {
			t.Errorf("原样再存一遍改了行版本：%v → %v", after, again)
		}
	})
}

// TestControlRollbackLeavesNothingBehind 证保存与它所在的事务同生共死。
func TestControlRollbackLeavesNothingBehind(t *testing.T) {
	repository, _, transactor, _ := newControlLedgers(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)
	rollback := errors.New("回滚")

	ledger := domain.NewFreezeLedger()
	mustFreeze(t, ledger, freezeRequest(t, "control-1", 3000), saBalance(t, 10000))
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repository.Save(txCtx, tenant, scope, ledger); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	reloaded, err := repository.LoadForScope(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if len(reloaded.Entries()) != 0 {
		t.Error("回滚后冻结仍在")
	}
}

// ---- 夹具 ----

func newControlLedgers(t *testing.T) (*adapter.FreezeLedgers, *adapter.CreditExposureLedgers, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	freezes, err := adapter.NewFreezeLedgers(db)
	if err != nil {
		t.Fatalf("构造冻结册仓储：%v", err)
	}
	exposures, err := adapter.NewCreditExposureLedgers(db)
	if err != nil {
		t.Fatalf("构造暴露册仓储：%v", err)
	}
	return freezes, exposures, db.Transactor(), pool
}

func mustSaveFreezeLedger(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.FreezeLedgers,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	ledger *domain.FreezeLedger,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return repository.Save(txCtx, tenant, scope, ledger)
	}); err != nil {
		t.Fatalf("事务内保存冻结册失败：%v", err)
	}
}

func mustFreeze(
	t *testing.T,
	ledger *domain.FreezeLedger,
	request domain.FreezeRequest,
	balance domain.OperationalBalance,
) domain.FundsFreeze {
	t.Helper()
	frozen, err := ledger.Freeze(request, balance)
	if err != nil {
		t.Fatalf("冻结：%v", err)
	}
	if frozen.Status() != domain.FreezeHeld {
		t.Fatalf("status = %s, want HELD", frozen.Status())
	}
	return frozen
}

func saValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func saTenant(t *testing.T, tenant string) domain.TenantID {
	t.Helper()
	return saValue(t, domain.NewTenantID, tenant)
}

func saScope(t *testing.T) domain.SettlementScope {
	t.Helper()
	return saScopeWithAccount(t, "account-1")
}

func saScopeWithAccount(t *testing.T, account string) domain.SettlementScope {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		saValue(t, domain.NewLegalEntityReference, "legal-entity-1"),
		saValue(t, domain.NewSettlementAccountID, account),
		saValue(t, domain.NewCurrencyCode, "USD"),
	)
	if err != nil {
		t.Fatalf("结算作用域：%v", err)
	}
	return scope
}

func saBalance(t *testing.T, posted int64) domain.OperationalBalance {
	t.Helper()
	balance, err := domain.NewOperationalBalance(saScope(t), posted, 0, 0, 0)
	if err != nil {
		t.Fatalf("运营余额：%v", err)
	}
	return balance
}

// saStanding 造一份已换上授权额度的信用状况：暴露账本不收没有政策出处的登记状况（ADR-0127
// 决定四），额度出自哪一版由字面量固定，用例要证别的出处时再自己换。
func saStanding(t *testing.T, limit int64, overdue bool) domain.CreditStanding {
	t.Helper()
	registered, err := domain.NewCreditStanding(saScope(t), 0, 0, overdue)
	if err != nil {
		t.Fatalf("信用状况：%v", err)
	}
	standing, err := registered.WithAuthorizedLimit(limit, saValue(t, domain.NewCreditPolicyReference, "PC-CREDIT-POLICY/v1"))
	if err != nil {
		t.Fatalf("授权额度：%v", err)
	}
	return standing
}

func freezeRequest(t *testing.T, requestID string, amount int64) domain.FreezeRequest {
	t.Helper()
	request, err := domain.NewFreezeRequest(
		saValue(t, domain.NewControlRequestID, requestID),
		saScope(t),
		amount,
		saValue(t, domain.NewBusinessAssociationReference, "SAC-"+requestID),
		frozenAtFixture,
	)
	if err != nil {
		t.Fatalf("冻结请求：%v", err)
	}
	return request
}

func exposureRequest(t *testing.T, requestID string, amount int64) domain.ExposureRequest {
	t.Helper()
	request, err := domain.NewExposureRequest(
		saValue(t, domain.NewControlRequestID, requestID),
		saScope(t),
		amount,
		saValue(t, domain.NewBusinessAssociationReference, "SAC-"+requestID),
		frozenAtFixture,
	)
	if err != nil {
		t.Fatalf("暴露请求：%v", err)
	}
	return request
}

// rowVersions 按控制请求读回每一行的 xmin：行被 UPDATE 过 xmin 就换，ON CONFLICT 撞上而 WHERE 不放行时只加行锁、xmin 不动。
func rowVersions(t *testing.T, pool *pgxpool.Pool, table string) map[string]string {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT control_request_id, xmin::text FROM settlement_accounting.`+table)
	if err != nil {
		t.Fatalf("读行版本：%v", err)
	}
	defer rows.Close()
	versions := map[string]string{}
	for rows.Next() {
		var request, version string
		if err := rows.Scan(&request, &version); err != nil {
			t.Fatalf("读行版本：%v", err)
		}
		versions[request] = version
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读行版本：%v", err)
	}
	return versions
}

func sameVersions(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for request, version := range left {
		if right[request] != version {
			return false
		}
	}
	return true
}

func countFreezeRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.funds_freeze`).Scan(&count); err != nil {
		t.Fatalf("统计冻结行数：%v", err)
	}
	return count
}
