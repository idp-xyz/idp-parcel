package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证`面单继续尝试决定`登记册的持久化（票 label-channel/10）：决定链
// 整份往返一样不少、空册是有意义的状态而不是缺行、开册只发生一次、并发保存由乐观版本拦住、租户
// 隔离在 SQL 条件上、坏行在重建门上暴露而不是变成一份看着合法的册、无事务拒写、形状由库内 CHECK 钉住。

var continuedAttemptDecidedAt = time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)

func newContinuedAttemptRegisters(t *testing.T) (*adapter.ContinuedAttemptRegisters, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewContinuedAttemptRegisters(db)
	if err != nil {
		t.Fatalf("构造登记册仓储：%v", err)
	}
	return repository, db.Transactor(), pool
}

func openRegisterFixture(t *testing.T, tenant, parcel string) domain.ContinuedAttemptRegister {
	t.Helper()
	register, err := domain.OpenContinuedAttemptRegister(
		mustBuild(t, domain.NewTenantID, tenant),
		mustBuild(t, domain.NewDeclaredParcelID, parcel),
	)
	if err != nil {
		t.Fatalf("开册：%v", err)
	}
	return register
}

// closureDecisionFixture 是一条带请求方的受控关闭：请求方可缺，这里刻意带上，好证它往返不丢。
func closureDecisionFixture(t *testing.T, id string, effectiveAt time.Time) domain.ContinuedAttemptDecisionSpec {
	t.Helper()
	return domain.ContinuedAttemptDecisionSpec{
		ID:                mustBuild(t, domain.NewContinuedAttemptDecisionID, id),
		Kind:              domain.ControlledClosureDecision,
		Requester:         mustBuild(t, domain.NewRequesterReference, "SHIPPER-ACCOUNT-7"),
		Decider:           mustBuild(t, domain.NewDeciderReference, "OPS-MANAGER-1"),
		AuthorityRole:     mustBuild(t, domain.NewContinuedAttemptAuthorityRoleReference, "PC-CLOSURE-ROLE-1"),
		AuthoritySnapshot: mustBuild(t, domain.NewContinuedAttemptAuthoritySnapshot, "PC-AUTH-SNAPSHOT-1"),
		Reason:            mustBuild(t, domain.NewContinuedAttemptReasonReference, "CHANNEL_SUSPENDED"),
		EffectiveAt:       effectiveAt,
		CutoffBoundary:    mustBuild(t, domain.NewAuthoritativeCutoffBoundary, "PS-CUTOFF-1"),
		ClosureResponsibilitySource: mustBuild(t,
			domain.NewClosureResponsibilitySourceReference, "PC-CHANNEL-ACCOUNT-SUSPENSION-1"),
	}
}

func reopeningDecisionFixture(t *testing.T, id, priorClosure string, effectiveAt time.Time) domain.ContinuedAttemptDecisionSpec {
	t.Helper()
	return domain.ContinuedAttemptDecisionSpec{
		ID:                  mustBuild(t, domain.NewContinuedAttemptDecisionID, id),
		Kind:                domain.ReopeningDecision,
		Decider:             mustBuild(t, domain.NewDeciderReference, "OPS-DIRECTOR-1"),
		AuthorityRole:       mustBuild(t, domain.NewContinuedAttemptAuthorityRoleReference, "PC-REOPEN-ROLE-1"),
		AuthoritySnapshot:   mustBuild(t, domain.NewContinuedAttemptAuthoritySnapshot, "PC-AUTH-SNAPSHOT-2"),
		Reason:              mustBuild(t, domain.NewContinuedAttemptReasonReference, "RESTRICTION_LIFTED"),
		EffectiveAt:         effectiveAt,
		RelatedPriorClosure: mustBuild(t, domain.NewContinuedAttemptDecisionID, priorClosure),
	}
}

// closedThenReopenedFixture 是「关闭 → 重开」两条决定的一册：两种决定各自独有的项都在场，往返验的
// 就是它们一样不少地回来。
func closedThenReopenedFixture(t *testing.T, tenant, parcel string) domain.ContinuedAttemptRegister {
	t.Helper()
	closed, err := openRegisterFixture(t, tenant, parcel).
		Append(closureDecisionFixture(t, "decision-1", continuedAttemptDecidedAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	reopened, err := closed.Append(
		reopeningDecisionFixture(t, "decision-2", "decision-1", continuedAttemptDecidedAt.Add(time.Hour)), false)
	if err != nil {
		t.Fatalf("追加重开：%v", err)
	}
	return reopened
}

func mustInsertRegister(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ContinuedAttemptRegisters,
	register domain.ContinuedAttemptRegister,
) {
	t.Helper()
	var outcome ports.ContinuedAttemptRegisterInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx, register)
		return err
	})
	if outcome != ports.ContinuedAttemptRegisterInserted {
		t.Fatalf("开册写入结果 = %s，want INSERTED", outcome)
	}
}

func mustFindRegister(
	t *testing.T,
	repository *adapter.ContinuedAttemptRegisters,
	tenant, parcel string,
) domain.ContinuedAttemptRegister {
	t.Helper()
	found, exists, err := repository.FindByParcel(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant),
		mustBuild(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("取回登记册：%v", err)
	}
	if !exists {
		t.Fatal("已开的登记册读不回来")
	}
	return found
}

// TestAContinuedAttemptRegisterRoundTripsWholly 证两条决定连同各自独有的项整份往返。丢任何一项
// 都不是小事：丢关闭责任来源，重开时「原限制已解除」就核不了；丢关联此前关闭，重开读回来说不出它
// 解的是哪一份；丢顺序，「最近适用决定是哪一条」就没有依据。
func TestAContinuedAttemptRegisterRoundTripsWholly(t *testing.T) {
	repository, transactor, _ := newContinuedAttemptRegisters(t)
	ctx := t.Context()

	mustInsertRegister(t, transactor, ctx, repository, closedThenReopenedFixture(t, "tenant-a", "parcel-1"))
	found := mustFindRegister(t, repository, "tenant-a", "parcel-1")

	if found.Revision() != 1 {
		t.Fatalf("revision = %d, want 1（Insert 写 1）", found.Revision())
	}
	decisions := found.Decisions()
	if len(decisions) != 2 {
		t.Fatalf("应读回两条决定，实得 %d 条", len(decisions))
	}
	closure, reopening := decisions[0], decisions[1]
	if closure.Kind() != domain.ControlledClosureDecision || reopening.Kind() != domain.ReopeningDecision {
		t.Fatalf("决定顺序往返变形：%s / %s", closure.Kind(), reopening.Kind())
	}
	if closure.ID().String() != "decision-1" ||
		closure.Requester().String() != "SHIPPER-ACCOUNT-7" ||
		closure.Decider().String() != "OPS-MANAGER-1" ||
		closure.AuthorityRole().String() != "PC-CLOSURE-ROLE-1" ||
		closure.AuthoritySnapshot().String() != "PC-AUTH-SNAPSHOT-1" ||
		closure.Reason().String() != "CHANNEL_SUSPENDED" ||
		!closure.EffectiveAt().Equal(continuedAttemptDecidedAt) ||
		closure.CutoffBoundary().String() != "PS-CUTOFF-1" ||
		closure.ClosureResponsibilitySource().String() != "PC-CHANNEL-ACCOUNT-SUSPENSION-1" {
		t.Fatalf("关闭决定往返变形：%+v", closure)
	}
	if reopening.ID().String() != "decision-2" ||
		reopening.Requester().String() != "" ||
		reopening.Decider().String() != "OPS-DIRECTOR-1" ||
		reopening.Reason().String() != "RESTRICTION_LIFTED" ||
		!reopening.EffectiveAt().Equal(continuedAttemptDecidedAt.Add(time.Hour)) ||
		reopening.RelatedPriorClosure().String() != "decision-1" ||
		reopening.CutoffBoundary().String() != "" ||
		reopening.ClosureResponsibilitySource().String() != "" {
		t.Fatalf("重开决定往返变形：%+v", reopening)
	}
	// 读回之后终局才现取：判断不在库里，是拿当前终局对着决定链算出来的。
	if got := found.Judge(false); got != domain.ContinuedAttemptOpen {
		t.Errorf("关过又重开、无终局，应派生开放，实得 %q", got)
	}
	if !found.HasAnyDecision() {
		t.Error("关过又重开的一册应报告有决定历史")
	}
}

// TestAnEmptyRegisterIsARowNotAnAbsence 证空册作为一行存在：它是`开放`的常见来源，与「没开过册」
// 是两个不同的答案——读面要说得出「没有人作过决定」，靶的正是这一行。
func TestAnEmptyRegisterIsARowNotAnAbsence(t *testing.T) {
	repository, transactor, _ := newContinuedAttemptRegisters(t)
	ctx := t.Context()

	mustInsertRegister(t, transactor, ctx, repository, openRegisterFixture(t, "tenant-a", "parcel-1"))
	found := mustFindRegister(t, repository, "tenant-a", "parcel-1")

	if found.HasAnyDecision() {
		t.Error("空册读回来不该报告有决定历史")
	}
	if got := found.Judge(false); got != domain.ContinuedAttemptOpen {
		t.Errorf("空册无终局应派生开放，实得 %q", got)
	}
	if got := found.Judge(true); got != domain.ContinuedAttemptControlledClosed {
		t.Errorf("空册但终局在场不该派生开放，实得 %q", got)
	}

	_, exists, err := repository.FindByParcel(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-a"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-never-opened"))
	if err != nil {
		t.Fatalf("查没开过的册：%v", err)
	}
	if exists {
		t.Error("没开过的册不该读回一行")
	}
}

// TestASecondOpenOnTheSameParcelAnswersAlreadyExists 证开册只发生一次：同键第二次落在`已存在`
// 这个业务答案上，而不是抛错也不是覆盖原册。
func TestASecondOpenOnTheSameParcelAnswersAlreadyExists(t *testing.T) {
	repository, transactor, _ := newContinuedAttemptRegisters(t)
	ctx := t.Context()

	mustInsertRegister(t, transactor, ctx, repository, closedThenReopenedFixture(t, "tenant-a", "parcel-1"))

	var outcome ports.ContinuedAttemptRegisterInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx, openRegisterFixture(t, "tenant-a", "parcel-1"))
		return err
	})
	if outcome != ports.ContinuedAttemptRegisterAlreadyExists {
		t.Fatalf("第二次开册结果 = %s，want ALREADY_EXISTS", outcome)
	}
	if found := mustFindRegister(t, repository, "tenant-a", "parcel-1"); len(found.Decisions()) != 2 {
		t.Fatalf("第二次开册覆盖了原册：%d 条决定", len(found.Decisions()))
	}
}

// TestAStaleRegisterSaveAnswersRevisionConflict 证乐观版本：拿一份过期的册保存，答`版本冲突`让
// 调用方重读再重放，而不是把抢先那一方追加的决定盖掉——盖掉一条关闭决定就是静默重开。
func TestAStaleRegisterSaveAnswersRevisionConflict(t *testing.T) {
	repository, transactor, _ := newContinuedAttemptRegisters(t)
	ctx := t.Context()

	mustInsertRegister(t, transactor, ctx, repository, openRegisterFixture(t, "tenant-a", "parcel-1"))
	loaded := mustFindRegister(t, repository, "tenant-a", "parcel-1")

	closed, err := loaded.Append(closureDecisionFixture(t, "decision-1", continuedAttemptDecidedAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	var first ports.ContinuedAttemptRegisterSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		first, err = repository.Save(txCtx, closed)
		return err
	})
	if first != ports.ContinuedAttemptRegisterSaved {
		t.Fatalf("首次保存 = %s，want SAVED", first)
	}

	// loaded 仍停在读出时的版本：在它上面再追加一条并保存，就是「抢先那一方已经落库」的形态。
	stale, err := loaded.Append(closureDecisionFixture(t, "decision-9", continuedAttemptDecidedAt.Add(time.Hour)), false)
	if err != nil {
		t.Fatalf("过期册上的追加：%v", err)
	}
	var second ports.ContinuedAttemptRegisterSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		second, err = repository.Save(txCtx, stale)
		return err
	})
	if second != ports.ContinuedAttemptRegisterRevisionConflict {
		t.Fatalf("过期保存 = %s，want REVISION_CONFLICT", second)
	}

	found := mustFindRegister(t, repository, "tenant-a", "parcel-1")
	if found.Revision() != 2 || len(found.Decisions()) != 1 || found.Decisions()[0].ID().String() != "decision-1" {
		t.Fatalf("过期保存把抢先那一方的写入盖掉了：revision = %d, decisions = %+v",
			found.Revision(), found.Decisions())
	}
	if got := found.Judge(false); got != domain.ContinuedAttemptControlledClosed {
		t.Errorf("保存后的册带一条生效关闭，应派生受控关闭，实得 %q", got)
	}
}

// TestAContinuedAttemptRegisterIsInvisibleToAnotherTenant 证租户隔离在 SQL 条件上（ADR-0003）：
// 另一个租户读同一个包裹号得到否定结果，且否定不区分「没开过」与「属别的租户」。
func TestAContinuedAttemptRegisterIsInvisibleToAnotherTenant(t *testing.T) {
	repository, transactor, _ := newContinuedAttemptRegisters(t)
	ctx := t.Context()

	mustInsertRegister(t, transactor, ctx, repository, closedThenReopenedFixture(t, "tenant-a", "parcel-1"))

	_, exists, err := repository.FindByParcel(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-b"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if err != nil {
		t.Fatalf("他租户查询：%v", err)
	}
	if exists {
		t.Fatal("他租户读到了本租户的登记册")
	}
	// 同号在他租户下另开一册不是冲突：键含租户维。
	mustInsertRegister(t, transactor, ctx, repository, openRegisterFixture(t, "tenant-b", "parcel-1"))
	if found := mustFindRegister(t, repository, "tenant-b", "parcel-1"); found.HasAnyDecision() {
		t.Fatal("他租户的新册读到了本租户的决定")
	}
}

// TestACorruptRegisterRowFailsRehydrationLoudly 证坏行在重建门上暴露：一条没有截断边界的「关闭」
// 不可能是本上下文判出来的，读回时如实报错，而不是变成一份看着合法、实则违反不变量的册
// （ADR-0028）。
func TestACorruptRegisterRowFailsRehydrationLoudly(t *testing.T) {
	repository, _, pool := newContinuedAttemptRegisters(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.continued_attempt_register (tenant_id, parcel_id, revision, snapshot)
		 VALUES ('tenant-a', 'parcel-x', 1,
		         '{"decisions":[{"id":"decision-1","kind":1,"decider":"OPS-1","authorityRole":"ROLE-1",
		                         "authoritySnapshot":"SNAP-1","reason":"R-1","effectiveAt":"2026-09-03T10:00:00Z"}]}')`); err != nil {
		t.Fatalf("裸写坏行：%v", err)
	}

	_, _, err := repository.FindByParcel(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-a"),
		mustBuild(t, domain.NewDeclaredParcelID, "parcel-x"))
	if !errors.Is(err, domain.ErrInvalidRehydratedContinuedAttemptRegister) {
		t.Fatalf("一条没有截断边界也没有关闭责任来源的关闭读回来应在重建门上报错，实得 %v", err)
	}
}

// TestContinuedAttemptRegisterWritesRefuseToRunOutsideATransaction 证两条写口都要求事务：开册与
// 追加后的保存都不该在自动提交下悄悄落库。
func TestContinuedAttemptRegisterWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newContinuedAttemptRegisters(t)
	ctx := t.Context()
	register := openRegisterFixture(t, "tenant-a", "parcel-1")

	if _, err := repository.Insert(ctx, register); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务开册应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.Save(ctx, register); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestContinuedAttemptRegisterShapesArePinnedInTheDatabase 证形状矩阵的库面：版本从 1 起、键不得
// 为空白，两条都由库内 CHECK 钉住，绕过适配器裸写也进不去。
func TestContinuedAttemptRegisterShapesArePinnedInTheDatabase(t *testing.T) {
	_, _, pool := newContinuedAttemptRegisters(t)
	ctx := t.Context()

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.continued_attempt_register (tenant_id, parcel_id, revision, snapshot)
		 VALUES ('tenant-a', 'parcel-x', 0, '{}')`); err == nil {
		t.Error("revision 0 的行溜进了登记册——那是没写完的行，不是一份聚合")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.continued_attempt_register (tenant_id, parcel_id, revision, snapshot)
		 VALUES ('tenant-a', '   ', 1, '{}')`); err == nil {
		t.Error("空白包裹号的行溜进了登记册")
	}
}
