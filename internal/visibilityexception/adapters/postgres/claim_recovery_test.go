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
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.claims.Save(txCtx, claimValue(t, domain.NewTenantID, tenant), claim)
	})
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

	err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.claims.Save(txCtx, claimValue(t, domain.NewTenantID, "tenant-a"), stale)
	})
	if !errors.Is(err, adapter.ErrClaimHistoryStale) {
		t.Fatalf("落后快照的写入 err = %v，应为 ErrClaimHistoryStale", err)
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

// TestResavingTheSameClaimKeepsOneHistoryPerVersion 证上一条的拒绝没有把重放一并拒掉：
// 同一份快照再落一次是重放，历史不重复也不报错。
func TestResavingTheSameClaimKeepsOneHistoryPerVersion(t *testing.T) {
	fixture := newClaimRecoveryFixture(t)
	ctx := t.Context()
	claim := receivedClaim(t, "batch-1", "item-1")
	if err := claim.AwaitSupplement("materials incomplete",
		claimSupplement(t, claimBaseAt.Add(7*24*time.Hour)), claimBaseAt.Add(time.Hour)); err != nil {
		t.Fatalf("await: %v", err)
	}
	fixture.saveClaim(t, ctx, "tenant-a", claim)
	fixture.saveClaim(t, ctx, "tenant-a", claim)

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
	if err := fixture.claims.Save(ctx, tenant, receivedClaim(t, "batch-1", "item-1")); err == nil {
		t.Fatalf("无事务写索赔被接受了")
	}

	matter := openedMatter(t, "recovery-1", "case-1", "supplier-1", "parcel-1/loss")
	rollback := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.claims.Save(txCtx, tenant, receivedClaim(t, "batch-1", "item-1")); err != nil {
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
