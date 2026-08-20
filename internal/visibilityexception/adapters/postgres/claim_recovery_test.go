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
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证索赔与追偿库的行为：三判分步的判断历史原样往返、
// 复核前版保留、三判形状由迁移 CHECK 把关、事项幂等由唯一约束承担、动作只增且
// attempt 按（事项+种类）各自递增、租户隔离由 SQL 条件承担。断言一律在事务闭包外。

var claimBaseAt = time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)

type claimRecoveryFixture struct {
	claims     *adapter.Claims
	recoveries *adapter.Recoveries
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newClaimRecoveryFixture(t *testing.T) *claimRecoveryFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	claims, err := adapter.NewClaims(db)
	if err != nil {
		t.Fatalf("构造索赔库：%v", err)
	}
	recoveries, err := adapter.NewRecoveries(db)
	if err != nil {
		t.Fatalf("构造追偿库：%v", err)
	}
	return &claimRecoveryFixture{claims: claims, recoveries: recoveries, transactor: db.Transactor(), pool: pool}
}

func (fixture *claimRecoveryFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func claimValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func receivedClaim(t *testing.T, batch, item string) *domain.ClaimItem {
	t.Helper()
	claim, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:          claimValue(t, domain.NewClaimItemID, item),
		Batch:       claimValue(t, domain.NewClaimBatchReference, batch),
		Customer:    claimValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract:    claimValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      claimValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Kind:        claimValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: claimBaseAt,
	})
	if err != nil {
		t.Fatalf("受理索赔：%v", err)
	}
	return claim
}

func (fixture *claimRecoveryFixture) saveClaim(t *testing.T, ctx context.Context, tenant string, claim *domain.ClaimItem) {
	t.Helper()
	if outcome := fixture.saveClaimOutcome(t, ctx, tenant, claim); outcome != ports.ClaimSaved {
		t.Fatalf("落索赔的写入代数 = %d，应为 ClaimSaved", outcome)
	}
}

// saveClaimOutcome 落索赔并交回写入代数。修订冲突不是 error，`inTx` 那条路看不见它，
// 要断言输家拿到哪一格只能从这里取。
func (fixture *claimRecoveryFixture) saveClaimOutcome(
	t *testing.T,
	ctx context.Context,
	tenant string,
	claim *domain.ClaimItem,
) ports.ClaimSaveOutcome {
	t.Helper()
	var outcome ports.ClaimSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = fixture.claims.Save(txCtx, claimValue(t, domain.NewTenantID, tenant), claim)
		return err
	})
	return outcome
}

func (fixture *claimRecoveryFixture) loadClaim(t *testing.T, ctx context.Context, tenant, batch, item string) *domain.ClaimItem {
	t.Helper()
	claim, exists, err := fixture.claims.FindByBatchItem(ctx,
		claimValue(t, domain.NewTenantID, tenant),
		claimValue(t, domain.NewClaimBatchReference, batch),
		claimValue(t, domain.NewClaimItemID, item))
	if err != nil || !exists {
		t.Fatalf("读回索赔：%v exists=%v", err, exists)
	}
	return claim
}

// TestClaimJudgmentHistoryRoundTripsStepByStep 证三判分步的判断历史逐步往返：每步
// 落库后重建，在重建出的对象上继续下一判——重建不重演，但生命周期方法照常工作；
// 复核换版后前版保留（CONTEXT 253）。
func TestClaimJudgmentHistoryRoundTripsStepByStep(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	received := receivedClaim(t, "batch-1", "item-1")
	fixture.saveClaim(t, ctx, "tenant-a", received)

	bare := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if _, screened := bare.Screen(); screened {
		t.Fatalf("受理读回凭空长出资格审核")
	}
	if bare.SubmittedAt() != claimBaseAt || bare.Customer().String() != "customer-1" {
		t.Fatalf("提交事实没原样读回")
	}

	if err := bare.ScreenEligibility(domain.ClaimEligible, "eligibility-rules/v1", claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("重建后过审：%v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", bare)

	screened := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if screen, ok := screened.Screen(); !ok || screen != domain.ClaimEligible {
		t.Fatalf("资格审核没原样读回")
	}
	if err := screened.ConcludeLiability(domain.LiabilityPartiallyEstablished,
		claimBaseAt.Add(30*24*time.Hour), claimBaseAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("重建后成结论：%v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", screened)

	concluded := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if conclusion, ok := concluded.Conclusion(); !ok || conclusion != domain.LiabilityPartiallyEstablished {
		t.Fatalf("责任结论没原样读回")
	}
	if _, hasPrior := concluded.PriorConclusion(); hasPrior {
		t.Fatalf("未复核的结论凭空长出前版")
	}
	if err := concluded.ReviewConclusion(domain.LiabilityFullyEstablished, claimBaseAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("重建后复核：%v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", concluded)

	reviewed := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	conclusion, _ := reviewed.Conclusion()
	prior, hasPrior := reviewed.PriorConclusion()
	if conclusion != domain.LiabilityFullyEstablished ||
		!hasPrior || prior != domain.LiabilityPartiallyEstablished {
		t.Fatalf("复核换版没保留前版：conclusion=%v prior=%v", conclusion, prior)
	}
}

// TestWithdrawnClaimStaysWithdrawnAfterReload 证撤回往返：读回的撤回索赔不再吸收
// 审核——提交与证据保留，后续审核以已撤回结束。
func TestWithdrawnClaimStaysWithdrawnAfterReload(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	claim := receivedClaim(t, "batch-1", "item-1")
	if err := claim.Withdraw(claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("撤回：%v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", claim)

	withdrawn := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if !withdrawn.Withdrawn() {
		t.Fatalf("撤回状态没读回")
	}
	if err := withdrawn.ScreenEligibility(domain.ClaimEligible, "basis", claimBaseAt.Add(2*time.Hour)); err == nil {
		t.Fatalf("撤回后的索赔仍被过审")
	}
}

// TestClaimShapeIsPinnedByChecks 证三判形状入库内 CHECK：没过审的结论、半截审核
// 都被库拦住——绕过适配器的裸写同样进不来。
func TestClaimShapeIsPinnedByChecks(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.claim_item
			(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref,
			 submitted_at, conclusion, concluded_at, review_by)
		 VALUES ('tenant-a', 'batch-x', 'item-x', 'c', 'c', 't', 'k',
		         now(), 'FULLY_ESTABLISHED', now(), now() + interval '1 day')`); err == nil {
		t.Fatalf("没过审的结论被库接受了")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.claim_item
			(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref,
			 submitted_at, screen)
		 VALUES ('tenant-a', 'batch-x', 'item-x', 'c', 'c', 't', 'k', now(), 'ELIGIBLE')`); err == nil {
		t.Fatalf("没有依据的审核被库接受了")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.claim_item
			(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref,
			 submitted_at, screen, screen_basis)
		 VALUES ('tenant-a', 'batch-x', 'item-y', 'c', 'c', 't', 'k', now(),
		         'AWAITING_SUPPLEMENT', 'materials incomplete')`); err == nil {
		t.Fatalf("等待补充缺少四件落点被库接受了")
	}
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.claim_item
			(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref,
			 submitted_at, screen, screen_basis,
			 missing_materials_ref, supplement_scope_ref, supplement_notice_ref, supplement_deadline)
		 VALUES ('tenant-a', 'batch-x', 'item-z', 'c', 'c', 't', 'k', now(),
		         'ELIGIBLE', 'ok', 'photos', 'scope', 'notice', now() + interval '7 days')`); err == nil {
		t.Fatalf("终局格带着四件落点被库接受了")
	}
}

func claimSupplement(t *testing.T, deadline time.Time) domain.SupplementRequirement {
	t.Helper()
	requirement, err := domain.NewSupplementRequirement(
		claimValue(t, domain.NewMissingMaterialsReference, "photos/damage"),
		claimValue(t, domain.NewSupplementScopeReference, "parcel-1/DAMAGE"),
		claimValue(t, domain.NewSupplementNoticeReference, "notify-policy/v1"),
		deadline,
	)
	if err != nil {
		t.Fatalf("补充要求：%v", err)
	}
	return requirement
}

// TestAwaitingSupplementRoundTripsWithDeadlineHistory 证等待补充四件落点与期限版本
// 往返：延期后原截止仍在历史上，当前截止是新版。
func TestAwaitingSupplementRoundTripsWithDeadlineHistory(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	claim := receivedClaim(t, "batch-1", "item-1")
	first := claimBaseAt.Add(7 * 24 * time.Hour)
	second := claimBaseAt.Add(14 * 24 * time.Hour)
	if err := claim.AwaitSupplement("materials incomplete", claimSupplement(t, first), claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("await: %v", err)
	}
	if err := claim.ExtendSupplementDeadline(second, claimBaseAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("extend: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", claim)

	loaded := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	screen, ok := loaded.Screen()
	if !ok || screen != domain.ClaimAwaitingSupplement {
		t.Fatalf("screen = %q ok = %v", screen, ok)
	}
	requirement, present := loaded.Supplement()
	if !present || !requirement.Deadline.Equal(second.UTC()) || requirement.MissingMaterials.String() != "photos/damage" {
		t.Fatalf("四件落点没读回：present=%v deadline=%s", present, requirement.Deadline)
	}
	history := loaded.SupplementDeadlineHistory()
	if len(history) != 2 || !history[0].Deadline.Equal(first.UTC()) || !history[1].Deadline.Equal(second.UTC()) {
		t.Fatalf("期限历史 = %#v", history)
	}
}

// TestAStaleClaimSnapshotCannotEraseAnApprovedExtension 证期限历史只增不覆盖。
//
// 两个写入方各自持有同一索赔的旧快照：一个批了延期先落库，另一个手里的历史还停在
// 原期限。后者若照自己的快照重写整段历史，那次已批延期就没了——而它是对客户作过的
// 承诺，不是可以按到达顺序覆盖的中间态（CONTEXT「所有尝试和内容版本保留」同一条）。
// 落后的写入必须被拒，由调用方读回赢家重放，不是静默截断。
//
// 先答的是修订守卫：落后方手里的修订停在读出那一版，条件更新命中零行，交回`版本冲突`
// 而不是报错——它是业务答案。历史守卫守的是另一格，见
// TestAMatchingRevisionStillCannotTruncateHistory。
func TestAStaleClaimSnapshotCannotEraseAnApprovedExtension(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	first := claimBaseAt.Add(7 * 24 * time.Hour)
	extended := claimBaseAt.Add(14 * 24 * time.Hour)

	claim := receivedClaim(t, "batch-1", "item-1")
	if err := claim.AwaitSupplement("materials incomplete", claimSupplement(t, first), claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("await: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", claim)

	stale := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	extender := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if err := extender.ExtendSupplementDeadline(extended, claimBaseAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("extend: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", extender)

	if outcome := fixture.saveClaimOutcome(t, ctx, "tenant-a", stale); outcome != ports.ClaimRevisionConflict {
		t.Fatalf("落后快照的写入代数 = %d，应为 ClaimRevisionConflict", outcome)
	}

	loaded := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	history := loaded.SupplementDeadlineHistory()
	if len(history) != 2 || !history[1].Deadline.Equal(extended.UTC()) {
		t.Fatalf("已批延期被落后快照擦掉了：%#v", history)
	}
	requirement, present := loaded.Supplement()
	if !present || !requirement.Deadline.Equal(extended.UTC()) {
		t.Fatalf("当前截止回退到了旧版：present=%v deadline=%s", present, requirement.Deadline)
	}
}

// TestAMatchingRevisionStillCannotTruncateHistory 证两道守卫拦的不是同一件事。
//
// 修订守卫答的是「手里不是当前那一版」，落后的写入都止于它。历史守卫守的是剩下那一
// 格：修订对得上、历史却少一版——快照不是读回来的，是手工拼的。这一格里条件更新会
// 命中，历史守卫若不在，已批延期就被这次「合法」写入截掉了。
func TestAMatchingRevisionStillCannotTruncateHistory(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	first := claimBaseAt.Add(7 * 24 * time.Hour)
	extended := claimBaseAt.Add(14 * 24 * time.Hour)

	claim := receivedClaim(t, "batch-1", "item-1")
	if err := claim.AwaitSupplement("materials incomplete", claimSupplement(t, first), claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("await: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", claim)

	extender := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if err := extender.ExtendSupplementDeadline(extended, claimBaseAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("extend: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", extender)

	// 拼一份「当前修订、旧历史」的快照：期限与历史一起退回第一版。重建门只校末版与
	// 当前期限一致，这样拼出来的它放行，于是修订守卫也放行——正是要留给历史守卫的那格。
	truncated := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1").Snapshot()
	truncated.Supplement = claimSupplement(t, first)
	truncated.DeadlineHistory = truncated.DeadlineHistory[:1]
	forged, err := domain.RehydrateClaimItem(truncated)
	if err != nil {
		t.Fatalf("重建截断快照：%v", err)
	}

	saveErr := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := fixture.claims.Save(txCtx, claimValue(t, domain.NewTenantID, "tenant-a"), forged)
		return err
	})
	if !errors.Is(saveErr, adapter.ErrClaimHistoryStale) {
		t.Fatalf("截断历史的写入 err = %v，应为 ErrClaimHistoryStale", saveErr)
	}

	history := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1").SupplementDeadlineHistory()
	if len(history) != 2 || !history[1].Deadline.Equal(extended.UTC()) {
		t.Fatalf("已批延期被截断的历史抹掉了：%#v", history)
	}
}

// TestCompetingClaimWritersSerializeOnTheClaimRow 证并发保护的另一半。上一条治的是
// 先后到达的落后快照；本条治两个写入方**同时在场**：赢家事务未提交时输家必须停在
// 索赔行锁上（appendSupplementDeadlines 的注释所倚仗的就是这次排队——没有它，两个
// append 会各自读到同一份历史再交错追加），赢家提交后输家按已提交的那一版被判冲突，
// 已批延期完好。评审 081701 #5 点名的正是这一幕。
//
// 排队这一半由行锁承担：条件更新同样要先拿行锁，锁在赢家手里就得等；赢家提交后 PG
// 按更新后的那一行重判 WHERE，修订已经推进，于是命中零行。
func TestCompetingClaimWritersSerializeOnTheClaimRow(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	first := claimBaseAt.Add(7 * 24 * time.Hour)
	extended := claimBaseAt.Add(14 * 24 * time.Hour)

	claim := receivedClaim(t, "batch-1", "item-1")
	if err := claim.AwaitSupplement("materials incomplete", claimSupplement(t, first), claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("await: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", claim)

	stale := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	extender := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if err := extender.ExtendSupplementDeadline(extended, claimBaseAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("extend: %v", err)
	}

	tenant := claimValue(t, domain.NewTenantID, "tenant-a")
	winnerHoldsLock := make(chan struct{})
	releaseWinner := make(chan struct{})
	winnerDone := make(chan error, 1)
	loserDone := make(chan error, 1)
	var winnerOutcome, loserOutcome ports.ClaimSaveOutcome

	go func() {
		winnerDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			outcome, err := fixture.claims.Save(txCtx, tenant, extender)
			if err != nil {
				return err
			}
			winnerOutcome = outcome
			close(winnerHoldsLock) // 条件更新已命中，行锁在手，事务保持敞开
			<-releaseWinner        // 等输家撞上锁再提交
			return nil
		})
	}()

	<-winnerHoldsLock
	go func() {
		loserDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			outcome, err := fixture.claims.Save(txCtx, tenant, stale)
			loserOutcome = outcome
			return err
		})
	}()

	// 输家此刻必须停在行锁上。它若在赢家提交前就返回，说明两笔写入根本没有排队——
	// 那正是删光重插时代静默丢历史的前提。等待窗只用来给输家跑到 UPSERT：锁在场时
	// 慢机器只会让它更晚返回，不会误报。
	select {
	case err := <-loserDone:
		t.Fatalf("输家在赢家提交前就返回了（err=%v）——并发写入没有在行锁上排队", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseWinner)

	if err := <-winnerDone; err != nil {
		t.Fatalf("赢家提交失败：%v", err)
	}
	if winnerOutcome != ports.ClaimSaved {
		t.Fatalf("赢家写入代数 = %d，应为 ClaimSaved", winnerOutcome)
	}
	if err := <-loserDone; err != nil {
		t.Fatalf("输家 err = %v——修订冲突是业务答案，不该把事务带成故障", err)
	}
	if loserOutcome != ports.ClaimRevisionConflict {
		t.Fatalf("输家写入代数 = %d，应为 ClaimRevisionConflict", loserOutcome)
	}

	loaded := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	history := loaded.SupplementDeadlineHistory()
	if len(history) != 2 || !history[1].Deadline.Equal(extended.UTC()) {
		t.Fatalf("并发竞争下已批延期没有保住：%#v", history)
	}
	if requirement, present := loaded.Supplement(); !present || !requirement.Deadline.Equal(extended.UTC()) {
		t.Fatalf("当前截止不是延期后的版本：present=%v deadline=%s", present, requirement.Deadline)
	}
}

// TestConcurrentClaimTransitionsLetOnlyOneWin 证并发两转换只有一个生效、另一个拿到
// 冲突结果。资格审核与撤回各自从同一版快照出发，领域门只在各自加载的旧快照上校验过
// ——没有修订守卫时后写者的整行重写会把先落的那个转换抹掉，而两边都以为自己成功。
func TestConcurrentClaimTransitionsLetOnlyOneWin(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	fixture.saveClaim(t, ctx, "tenant-a", receivedClaim(t, "batch-1", "item-1"))

	screener := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	withdrawer := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if err := screener.ScreenEligibility(domain.ClaimEligible, "eligibility-rules/v1", claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("过审：%v", err)
	}
	if err := withdrawer.Withdraw(claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("撤回：%v", err)
	}

	loadedRevision := screener.Revision()
	fixture.saveClaim(t, ctx, "tenant-a", screener)
	if outcome := fixture.saveClaimOutcome(t, ctx, "tenant-a", withdrawer); outcome != ports.ClaimRevisionConflict {
		t.Fatalf("后到的旧快照写入代数 = %d，应为 ClaimRevisionConflict", outcome)
	}

	loaded := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1")
	if screen, ok := loaded.Screen(); !ok || screen != domain.ClaimEligible {
		t.Fatalf("先落库的资格审核被后到的旧快照整行盖掉了：screen=%q ok=%v", screen, ok)
	}
	if loaded.Withdrawn() {
		t.Fatalf("撞了冲突的撤回还是落进了库")
	}
	// 赢家推进一版正是输家撞墙的原因；只断言冲突不断言递增，守卫恒真也照样绿。
	if loaded.Revision() != loadedRevision+1 {
		t.Fatalf("修订没随成功写入推进：读出 %d，落库后 %d", loadedRevision, loaded.Revision())
	}
}

// TestResavingTheSameClaimKeepsOneHistoryPerVersion 证上一条的拒绝没有把重放一并拒掉：
// 同一份快照再落一次是重放，历史不重复也不报错。
//
// 重放必须从读回的那一版出发。拿着已落过库的旧对象再落一次，修订守卫会先答`版本冲突`
// ——那是对的（它手里的确不是当前那一版），但这一条要证的历史追加就走不到了。
func TestResavingTheSameClaimKeepsOneHistoryPerVersion(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	claim := receivedClaim(t, "batch-1", "item-1")
	if err := claim.AwaitSupplement("materials incomplete",
		claimSupplement(t, claimBaseAt.Add(7*24*time.Hour)), claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("await: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", claim)
	fixture.saveClaim(t, ctx, "tenant-a", fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1"))

	if history := fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1").SupplementDeadlineHistory(); len(history) != 1 {
		t.Fatalf("重放把同一版期限记了 %d 次", len(history))
	}
}

// TestClaimsOfAnotherTenantAreInvisible 证租户隔离：同名（批次+项）在另一租户不可见，
// 各租户各自一行互不搅动。
func TestClaimsOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	fixture.saveClaim(t, ctx, "tenant-a", receivedClaim(t, "batch-1", "item-1"))

	if _, exists, err := fixture.claims.FindByBatchItem(ctx,
		claimValue(t, domain.NewTenantID, "tenant-b"),
		claimValue(t, domain.NewClaimBatchReference, "batch-1"),
		claimValue(t, domain.NewClaimItemID, "item-1")); err != nil || exists {
		t.Fatalf("跨租户可见：err=%v exists=%v", err, exists)
	}

	other := receivedClaim(t, "batch-1", "item-1")
	if err := other.Withdraw(claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("撤回：%v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-b", other)
	if fixture.loadClaim(t, ctx, "tenant-a", "batch-1", "item-1").Withdrawn() {
		t.Fatalf("另一租户的撤回搅动了本租户的索赔")
	}
}

// scopedClaim 受理一项指定（客户账户+目标范围+索赔类型）的索赔，供重复关系那一维
// 逐样错开来证——receivedClaim 把这三样钉死了，用它证不出「差一样就不算重复」。
func scopedClaim(t *testing.T, batch, item, customer, target, kind string) *domain.ClaimItem {
	t.Helper()
	claim, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:          claimValue(t, domain.NewClaimItemID, item),
		Batch:       claimValue(t, domain.NewClaimBatchReference, batch),
		Customer:    claimValue(t, domain.NewCustomerAccountReference, customer),
		Contract:    claimValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      claimValue(t, domain.NewRequestScopeReference, target),
		Kind:        claimValue(t, domain.NewClaimKindReference, kind),
		SubmittedAt: claimBaseAt,
	})
	if err != nil {
		t.Fatalf("受理索赔：%v", err)
	}
	return claim
}

// TestCountLiveScopeClaimsCountsOnlyTheSameLiveScope 证重复关系那一维取到的事实：
// 跨批次数得到（同一客户把同一范围分两批提交正是要认出来的情形）、本项自己不算自己、
// 已撤回不算（`AT-VE-123`：撤回后重新提交同一范围要重新检查重复关系，把撤回那项算
// 进来同一范围就再也提不了第二次）、跨租户不算（ADR-0003），客户/范围/类型差一样
// 就不是同一重复。
func TestCountLiveScopeClaimsCountsOnlyTheSameLiveScope(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	count := func(item string) int {
		t.Helper()
		got, err := fixture.claims.CountLiveScopeClaims(ctx,
			claimValue(t, domain.NewTenantID, "tenant-a"),
			claimValue(t, domain.NewCustomerAccountReference, "customer-1"),
			claimValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
			claimValue(t, domain.NewClaimKindReference, "LOSS"),
			claimValue(t, domain.NewClaimItemID, item))
		if err != nil {
			t.Fatalf("数重复：%v", err)
		}
		return got
	}

	fixture.saveClaim(t, ctx, "tenant-a", receivedClaim(t, "batch-1", "item-1"))
	if got := count("item-1"); got != 0 {
		t.Fatalf("只有本项时 count = %d，本项不该数成自己的重复", got)
	}

	// 同范围、另一批次：重复是跨批次的事，按批次收窄就永远数不到。
	fixture.saveClaim(t, ctx, "tenant-a", receivedClaim(t, "batch-2", "item-2"))
	if got := count("item-1"); got != 1 {
		t.Fatalf("跨批次同范围 count = %d，want 1", got)
	}

	// 客户、目标范围、索赔类型各差一样：都不是同一重复。
	fixture.saveClaim(t, ctx, "tenant-a", scopedClaim(t, "batch-3", "item-3", "customer-2", "parcel-1/loss", "LOSS"))
	fixture.saveClaim(t, ctx, "tenant-a", scopedClaim(t, "batch-3", "item-4", "customer-1", "parcel-9/loss", "LOSS"))
	fixture.saveClaim(t, ctx, "tenant-a", scopedClaim(t, "batch-3", "item-5", "customer-1", "parcel-1/loss", "DAMAGE"))
	// 另一租户的同范围索赔：租户是最高数据隔离边界。
	fixture.saveClaim(t, ctx, "tenant-b", receivedClaim(t, "batch-1", "item-6"))
	if got := count("item-1"); got != 1 {
		t.Fatalf("差一样或跨租户被数成了重复：count = %d，want 1", got)
	}

	withdrawn := fixture.loadClaim(t, ctx, "tenant-a", "batch-2", "item-2")
	if err := withdrawn.Withdraw(claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("撤回：%v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", withdrawn)
	if got := count("item-1"); got != 0 {
		t.Fatalf("撤回那项仍被数成重复：count = %d", got)
	}
}

func openedMatter(t *testing.T, id, caseID, counterparty, scope string) domain.RecoveryMatter {
	t.Helper()
	matter, err := domain.OpenRecoveryMatter(domain.RecoveryMatterSpec{
		ID:           claimValue(t, domain.NewRecoveryMatterID, id),
		Case:         claimValue(t, domain.NewCaseID, caseID),
		Counterparty: claimValue(t, domain.NewCounterpartyReference, counterparty),
		Basis:        claimValue(t, domain.NewLiabilityBasisReference, "supplier-agreement/v1"),
		LegalEntity:  claimValue(t, domain.NewLegalEntityReference, "entity-1"),
		Scope:        claimValue(t, domain.NewRequestScopeReference, scope),
		Evidence:     claimValue(t, domain.NewRequestEvidenceReference, "evidence/loss-1"),
		Deadline:     claimBaseAt.Add(14 * 24 * time.Hour),
		OpenedAt:     claimBaseAt,
	})
	if err != nil {
		t.Fatalf("建立追偿事项：%v", err)
	}
	return matter
}

func (fixture *claimRecoveryFixture) appendAction(t *testing.T, ctx context.Context, tenant string, matter domain.RecoveryMatterID, kind domain.RecoveryActionKind, milestone domain.RecoveryActionMilestone, attempt int) {
	t.Helper()
	action, err := domain.RecordRecoveryAction(matter, kind, "content/v1", milestone,
		claimValue(t, domain.NewLiabilityBasisReference, "supplier-agreement/v1"),
		claimBaseAt.Add(time.Duration(attempt)*time.Hour), attempt)
	if err != nil {
		t.Fatalf("构造追偿动作：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.recoveries.AppendAction(txCtx, claimValue(t, domain.NewTenantID, tenant), action)
	})
}

// TestRecoveryMatterRoundTripsAndStaysIdempotent 证事项往返与幂等：FindByID 与
// FindCurrent 读回同一事项（重建重走 OpenRecoveryMatter 全部不变量）；同（案件+
// 相对方+范围）第二份 ON CONFLICT DO NOTHING，事务仍可用。
func TestRecoveryMatterRoundTripsAndStaysIdempotent(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	matter := openedMatter(t, "recovery-1", "case-1", "supplier-1", "parcel-1/loss")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.recoveries.Save(txCtx, claimValue(t, domain.NewTenantID, "tenant-a"), matter)
		return err
	})

	byID, exists, err := fixture.recoveries.FindByID(ctx,
		claimValue(t, domain.NewTenantID, "tenant-a"),
		claimValue(t, domain.NewRecoveryMatterID, "recovery-1"))
	if err != nil || !exists {
		t.Fatalf("按标识读回：%v exists=%v", err, exists)
	}
	if byID.Case() != matter.Case() || byID.Deadline() != matter.Deadline() ||
		byID.LegalEntity() != matter.LegalEntity() || byID.Evidence() != matter.Evidence() {
		t.Fatalf("事项没原样读回：%+v", byID)
	}

	current, exists, err := fixture.recoveries.FindCurrent(ctx,
		claimValue(t, domain.NewTenantID, "tenant-a"),
		claimValue(t, domain.NewCaseID, "case-1"),
		claimValue(t, domain.NewCounterpartyReference, "supplier-1"),
		claimValue(t, domain.NewRequestScopeReference, "parcel-1/loss"))
	if err != nil || !exists || current.ID() != matter.ID() {
		t.Fatalf("按幂等键读回：%v exists=%v id=%v", err, exists, current.ID())
	}

	// 另一租户不可见，同键另建互不干扰。
	if _, exists, err := fixture.recoveries.FindCurrent(ctx,
		claimValue(t, domain.NewTenantID, "tenant-b"),
		claimValue(t, domain.NewCaseID, "case-1"),
		claimValue(t, domain.NewCounterpartyReference, "supplier-1"),
		claimValue(t, domain.NewRequestScopeReference, "parcel-1/loss")); err != nil || exists {
		t.Fatalf("跨租户可见：err=%v exists=%v", err, exists)
	}
}

// TestSecondRecoverySaveInTheSameTransactionKeepsTheTxUsable 证 ADR-0031：同幂等
// 键二次 Save 不得走 23505（那会中止事务）；冲突后本事务还能继续语句，赢家原行仍在。
func TestSecondRecoverySaveInTheSameTransactionKeepsTheTxUsable(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	tenant := claimValue(t, domain.NewTenantID, "tenant-a")

	first := openedMatter(t, "recovery-1", "case-1", "supplier-1", "parcel-1/loss")
	impostor := openedMatter(t, "recovery-2", "case-1", "supplier-1", "parcel-1/loss")

	var foundAfter bool
	var winnerID domain.RecoveryMatterID
	var secondOutcome ports.RecoverySaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		if _, err := fixture.recoveries.Save(txCtx, tenant, first); err != nil {
			return err
		}
		saved, err := fixture.recoveries.Save(txCtx, tenant, impostor)
		if err != nil {
			return err
		}
		secondOutcome = saved
		current, exists, err := fixture.recoveries.FindCurrent(txCtx, tenant,
			claimValue(t, domain.NewCaseID, "case-1"),
			claimValue(t, domain.NewCounterpartyReference, "supplier-1"),
			claimValue(t, domain.NewRequestScopeReference, "parcel-1/loss"))
		if err != nil {
			return err
		}
		foundAfter = exists
		winnerID = current.ID()
		return nil
	})
	if secondOutcome != ports.RecoveryAlreadyRecorded {
		t.Fatalf("第二份写入 outcome = %d，应为 ALREADY_RECORDED", secondOutcome)
	}
	if !foundAfter || winnerID.String() != "recovery-1" {
		t.Fatalf("冲突后读回 found=%v id=%s，事务应仍可用且赢家是第一份", foundAfter, winnerID)
	}
}

// TestRecoveryActionsKeepSeparateAttemptSequences 证动作序列：预先通知与正式主张
// 各有各的尝试序列；同一尝试的多个过程节点各占一行不虚增序号（CountActions 取
// MAX 不数行）；动作指不到事项就进不来（外键）。
func TestRecoveryActionsKeepSeparateAttemptSequences(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	matter := openedMatter(t, "recovery-1", "case-1", "supplier-1", "parcel-1/loss")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.recoveries.Save(txCtx, claimValue(t, domain.NewTenantID, "tenant-a"), matter)
		return err
	})

	matterID := claimValue(t, domain.NewRecoveryMatterID, "recovery-1")
	// 通知第一次尝试的两个过程节点 + 提交失败后的第二次尝试；主张独立起序。
	fixture.appendAction(t, ctx, "tenant-a", matterID, domain.PreliminaryNotice, domain.ActionPrepared, 1)
	fixture.appendAction(t, ctx, "tenant-a", matterID, domain.PreliminaryNotice, domain.SubmissionFailed, 1)
	fixture.appendAction(t, ctx, "tenant-a", matterID, domain.PreliminaryNotice, domain.ActionSubmitted, 2)
	fixture.appendAction(t, ctx, "tenant-a", matterID, domain.FormalAssertion, domain.ActionPrepared, 1)

	notices, err := fixture.recoveries.CountActions(ctx,
		claimValue(t, domain.NewTenantID, "tenant-a"), matterID, domain.PreliminaryNotice)
	if err != nil || notices != 2 {
		t.Fatalf("通知序列 = %d（err=%v），想要 2——三行两次尝试，数行会虚增", notices, err)
	}
	assertions, err := fixture.recoveries.CountActions(ctx,
		claimValue(t, domain.NewTenantID, "tenant-a"), matterID, domain.FormalAssertion)
	if err != nil || assertions != 1 {
		t.Fatalf("主张序列 = %d（err=%v），想要 1", assertions, err)
	}
	foreign, err := fixture.recoveries.CountActions(ctx,
		claimValue(t, domain.NewTenantID, "tenant-b"), matterID, domain.PreliminaryNotice)
	if err != nil || foreign != 0 {
		t.Fatalf("跨租户序列 = %d（err=%v），想要 0", foreign, err)
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.recovery_action
			(tenant_id, matter_id, kind, attempt, milestone, content_ref, obligation, occurred_at)
		 VALUES ('tenant-a', 'recovery-ghost', 'PRELIMINARY_NOTICE', 1, 'PREPARED', 'c', 'o', now())`); err == nil {
		t.Fatalf("没有事项的动作被库接受了")
	}
}

// TestClaimRecoveryWritesRequireTransactionAndRollBack 证事务纪律：无事务写一律拒；
// 事务失败后索赔、事项与动作都不存在。
func TestClaimRecoveryWritesRequireTransactionAndRollBack(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()

	tenant := claimValue(t, domain.NewTenantID, "tenant-a")
	if _, err := fixture.claims.Save(ctx, tenant, receivedClaim(t, "batch-1", "item-1")); err == nil {
		t.Fatalf("无事务写索赔被接受了")
	}

	matter := openedMatter(t, "recovery-1", "case-1", "supplier-1", "parcel-1/loss")
	rollback := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.claims.Save(txCtx, tenant, receivedClaim(t, "batch-1", "item-1")); err != nil {
			return err
		}
		if _, err := fixture.recoveries.Save(txCtx, tenant, matter); err != nil {
			return err
		}
		return context.Canceled
	})
	if rollback == nil {
		t.Fatalf("事务该失败没失败")
	}

	if _, exists, err := fixture.claims.FindByBatchItem(ctx, tenant,
		claimValue(t, domain.NewClaimBatchReference, "batch-1"),
		claimValue(t, domain.NewClaimItemID, "item-1")); err != nil || exists {
		t.Fatalf("回滚后索赔仍在：err=%v exists=%v", err, exists)
	}
	if _, exists, err := fixture.recoveries.FindByID(ctx, tenant,
		claimValue(t, domain.NewRecoveryMatterID, "recovery-1")); err != nil || exists {
		t.Fatalf("回滚后事项仍在：err=%v exists=%v", err, exists)
	}
}
