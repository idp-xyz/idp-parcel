package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证可达性判断库的行为。用真实引擎而非替身，是因为要证
// 的恰是替身给不出的那几件：写入代数由主键冲突翻译（ADR-0031 落库第一例）、租户
// 隔离由 SQL 条件承担、事务缺失由框架的 RequireExecutor 拦住。

var judgedAt = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

func newJudgments(t *testing.T) (*adapter.ReachabilityJudgments, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewReachabilityJudgments(db)
	if err != nil {
		t.Fatalf("构造判断库：%v", err)
	}
	return repository, db.Transactor()
}

func within(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func scalar[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func judgmentKey(t *testing.T, tenant string) domain.ReachabilityJudgmentKey {
	t.Helper()
	asOf, err := domain.NewJudgmentAsOf(
		scalar(t, domain.NewAsOfSemantic, "ACCEPTANCE_TIME"),
		judgedAt.Add(-time.Hour),
		scalar(t, domain.NewAsOfStrategyVersion, "asof-strategy/v1"),
	)
	if err != nil {
		t.Fatalf("构造 asOf：%v", err)
	}
	return domain.ReachabilityJudgmentKey{
		TenantID:          scalar(t, domain.NewTenantID, tenant),
		CustomerAccountID: scalar(t, domain.NewCustomerAccountID, "customer-a"),
		ShipmentRequestID: scalar(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: scalar(t, domain.NewSubmissionVersionID, "submission-1"),
		DeclaredParcelID:  scalar(t, domain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:    scalar(t, domain.NewServicePurpose, "LAST_MILE_DELIVERY"),
		AsOf:              asOf,
	}
}

// mixedFinding 造一份三态齐备的判断：一格合格、一格淘汰、一格证据未知加候选内缺口
// ——正好落在矩阵的`资料不足`行，序列化要保住的每一类内容它都有。
func mixedFinding(t *testing.T) domain.ReachabilityFinding {
	t.Helper()
	qualified, err := domain.NewRouteCandidate(
		scalar(t, domain.NewCandidateID, "candidate-1"),
		domain.CandidateQualified,
		domain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("构造合格候选：%v", err)
	}
	eliminated, err := domain.NewRouteCandidate(
		scalar(t, domain.NewCandidateID, "candidate-2"),
		domain.CandidateEliminated,
		scalar(t, domain.NewCandidateReason, "customs-restriction"),
	)
	if err != nil {
		t.Fatalf("构造淘汰候选：%v", err)
	}
	unknown, err := domain.NewRouteCandidate(
		scalar(t, domain.NewCandidateID, "candidate-3"),
		domain.CandidateEvidenceUnknown,
		domain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("构造未知候选：%v", err)
	}
	gap, err := domain.NewEvidenceGap(
		scalar(t, domain.NewEvidenceGapReference, "service-area-coverage"),
		domain.CandidateScopedGap,
		[]domain.CandidateID{scalar(t, domain.NewCandidateID, "candidate-3")},
		scalar(t, domain.NewReassessmentCondition, "coverage-registered"),
	)
	if err != nil {
		t.Fatalf("构造缺口：%v", err)
	}
	finding, err := domain.ConcludeReachability(
		[]domain.RouteCandidate{qualified, eliminated, unknown},
		[]domain.EvidenceGap{gap},
	)
	if err != nil {
		t.Fatalf("形成判断：%v", err)
	}
	return finding
}

func record(t *testing.T, tenant string) ports.ReachabilityJudgmentRecord {
	t.Helper()
	return ports.ReachabilityJudgmentRecord{
		Key:          judgmentKey(t, tenant),
		Finding:      mixedFinding(t),
		JudgedAt:     judgedAt,
		ViewRevision: scalar(t, domain.NewNetworkViewRevision, "network-view/rev-42"),
	}
}

// TestAJudgmentIsReadBackUnchanged 证一次判断越过提交边界后原样读回：键逐维、三值
// 结论、候选三态、缺口及其再判条件、视图修订与判断时间，一样不丢不变形。
func TestAJudgmentIsReadBackUnchanged(t *testing.T) {
	repository, transactor := newJudgments(t)
	ctx := t.Context()
	correlation := scalar(t, domain.NewRequestCorrelationID, "corr-1")
	saved := record(t, "tenant-a")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, correlation, saved)
		if err != nil {
			return err
		}
		if outcome != ports.ReachabilityJudgmentSaved {
			t.Fatalf("首存结果 = %q，应为 SAVED", outcome)
		}
		return nil
	})

	found, exists, err := repository.FindByCorrelation(ctx, saved.Key.TenantID, correlation)
	if err != nil {
		t.Fatalf("取回判断：%v", err)
	}
	if !exists {
		t.Fatal("已提交的判断读不回来")
	}
	if !found.Key.SameJudgmentScope(saved.Key) {
		t.Errorf("键读回变形：%+v，应为 %+v", found.Key, saved.Key)
	}
	if found.Finding.Value() != domain.InsufficientEvidence {
		t.Errorf("三值结论 = %q，应为 INSUFFICIENT_EVIDENCE", found.Finding.Value())
	}
	if len(found.Finding.QualifiedCandidates()) != 1 ||
		len(found.Finding.EliminatedCandidates()) != 1 ||
		len(found.Finding.UnknownCandidates()) != 1 {
		t.Errorf("候选三态读回变形：%+v", found.Finding.Candidates())
	}
	if reason := found.Finding.EliminatedCandidates()[0].Reason().String(); reason != "customs-restriction" {
		t.Errorf("淘汰理由 = %q，应为 customs-restriction", reason)
	}
	gaps := found.Finding.EvidenceGaps()
	if len(gaps) != 1 || gaps[0].Reference().String() != "service-area-coverage" ||
		gaps[0].ReassessmentCondition().String() != "coverage-registered" {
		t.Errorf("缺口读回变形：%+v", gaps)
	}
	if found.ViewRevision != saved.ViewRevision {
		t.Errorf("视图修订 = %q，应为 %q", found.ViewRevision, saved.ViewRevision)
	}
	if !found.JudgedAt.Equal(saved.JudgedAt) {
		t.Errorf("判断时间 = %v，应为 %v", found.JudgedAt, saved.JudgedAt)
	}
}

// TestASecondWriterGetsAlreadyRecorded 证同关联第二份写入拿到`已有记录`而不是错误，
// 且先到者原样保留——迟到结果不按到达顺序覆盖原判断（AT-NR-028，ADR-0031 的写入
// 代数落到真库）。
func TestASecondWriterGetsAlreadyRecorded(t *testing.T) {
	repository, transactor := newJudgments(t)
	ctx := t.Context()
	correlation := scalar(t, domain.NewRequestCorrelationID, "corr-1")
	first := record(t, "tenant-a")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, correlation, first)
		return err
	})

	// 第二份换了判断时间与视图修订——内容不同也不覆盖，谁先越过提交边界谁是判断。
	second := record(t, "tenant-a")
	second.JudgedAt = judgedAt.Add(time.Hour)
	second.ViewRevision = scalar(t, domain.NewNetworkViewRevision, "network-view/rev-43")
	within(t, transactor, ctx, func(txCtx context.Context) error {
		outcome, err := repository.Save(txCtx, correlation, second)
		if err != nil {
			return err
		}
		if outcome != ports.ReachabilityJudgmentAlreadyRecorded {
			t.Fatalf("第二份写入结果 = %q，应为 ALREADY_RECORDED", outcome)
		}
		return nil
	})

	found, exists, err := repository.FindByCorrelation(ctx, first.Key.TenantID, correlation)
	if err != nil || !exists {
		t.Fatalf("取回判断：%v exists=%v", err, exists)
	}
	if !found.JudgedAt.Equal(first.JudgedAt) || found.ViewRevision != first.ViewRevision {
		t.Errorf("先到者被改写：读回 judgedAt=%v revision=%q", found.JudgedAt, found.ViewRevision)
	}
}

// TestOtherTenantsAreInvisible 证否定结果不泄露另一个租户是否发起过同关联请求。
func TestOtherTenantsAreInvisible(t *testing.T) {
	repository, transactor := newJudgments(t)
	ctx := t.Context()
	correlation := scalar(t, domain.NewRequestCorrelationID, "corr-shared")

	within(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, correlation, record(t, "tenant-a"))
		return err
	})

	_, exists, err := repository.FindByCorrelation(ctx, scalar(t, domain.NewTenantID, "tenant-b"), correlation)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("另一个租户读到了不属于它的判断")
	}
}

// TestWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池
// （PBC-08「不绕过 DB.RequireExecutor」的行为面）。
func TestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _ := newJudgments(t)
	ctx := t.Context()
	correlation := scalar(t, domain.NewRequestCorrelationID, "corr-1")

	if _, err := repository.Save(ctx, correlation, record(t, "tenant-a")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}

	_, exists, err := repository.FindByCorrelation(ctx, scalar(t, domain.NewTenantID, "tenant-a"), correlation)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("被拒绝的写入仍然落库了")
	}
}

// TestRollbackLeavesNothingBehind 证判断与它所在的事务同生共死。
func TestRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor := newJudgments(t)
	ctx := t.Context()
	correlation := scalar(t, domain.NewRequestCorrelationID, "corr-1")
	rollback := errors.New("回滚")

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, correlation, record(t, "tenant-a")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindByCorrelation(ctx, scalar(t, domain.NewTenantID, "tenant-a"), correlation)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后判断仍在")
	}
}
