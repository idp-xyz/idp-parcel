package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证外部结果登记册的行为：写入代数由 ON CONFLICT 承担、
// 留存不猜的形状由迁移 CHECK 把关、租户隔离由 SQL 条件承担、事务纪律由
// RequireExecutor 拦住。断言一律在事务闭包外（Goexit 会挂死连接）。

var externalReceivedAt = time.Date(2026, 8, 13, 16, 0, 0, 0, time.UTC)

func newExternalResults(t *testing.T) (*adapter.ExternalResults, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewExternalResults(db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	return repository, db.Transactor()
}

func inTx2(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func make2[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func attributedResult(t *testing.T, sourceID, version string, layer domain.ResultLayer) domain.ExternalResult {
	t.Helper()
	result, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     sourceID,
		Role:         make2(t, domain.NewSourceAuthorityRole, "REGULATOR"),
		RawSemantics: "DECLARATION_ACCEPTED",
		Rule:         make2(t, domain.NewInterpretationRuleReference, "interpretation/v1"),
		Version:      make2(t, domain.NewSubmissionVersionID, version),
		Attempt:      1,
		Scope:        make2(t, domain.NewDecisionScopeReference, "declaration/full"),
		OccurredAt:   externalReceivedAt.Add(-time.Hour),
		ReceivedAt:   externalReceivedAt,
	})
	if err != nil {
		t.Fatalf("构造外部结果：%v", err)
	}
	return result
}

func attributedRecord(t *testing.T, tenant, sourceID, version string) ports.ExternalResultRecord {
	t.Helper()
	return ports.ExternalResultRecord{
		Key:           ports.ExternalResultKey{TenantID: make2(t, domain.NewTenantID, tenant), SourceID: sourceID},
		ContentDigest: "digest-" + sourceID,
		Result:        attributedResult(t, sourceID, version, domain.BusinessAcceptanceLayer),
		RecordedAt:    externalReceivedAt.Add(time.Minute),
	}
}

func unattributableRecord(t *testing.T, tenant, sourceID string) ports.ExternalResultRecord {
	t.Helper()
	return ports.ExternalResultRecord{
		Key:            ports.ExternalResultKey{TenantID: make2(t, domain.NewTenantID, tenant), SourceID: sourceID},
		ContentDigest:  "digest-" + sourceID,
		Unattributable: true,
		RawSemantics:   "UNKNOWN_DECLARATION_REFERENCE",
		ClaimedVersion: "version-nobody-knows",
		RecordedAt:     externalReceivedAt.Add(time.Minute),
	}
}

// TestRecordsAreReadBackUnchanged 证两种形态原样读回：可归属记录带经领域构造函数
// 重建的八件事实；归属不上的记录只带原始语义与声称版本（留存不猜——库里没有事实
// 可编）。
func TestRecordsAreReadBackUnchanged(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()

	attributed := attributedRecord(t, "tenant-a", "resp-1", "submission-1")
	orphan := unattributableRecord(t, "tenant-a", "resp-2")
	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ExternalResultRecord{attributed, orphan} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	foundAttributed, exists, err := repository.FindByKey(ctx, attributed.Key)
	if err != nil || !exists {
		t.Fatalf("取回可归属记录：%v exists=%v", err, exists)
	}
	if foundAttributed.Unattributable ||
		foundAttributed.Result.Layer() != domain.BusinessAcceptanceLayer ||
		foundAttributed.Result.Version().String() != "submission-1" ||
		foundAttributed.Result.Attempt() != 1 ||
		foundAttributed.Result.RawSemantics() != "DECLARATION_ACCEPTED" {
		t.Errorf("可归属记录读回变形：%+v", foundAttributed)
	}
	if !foundAttributed.Result.ReceivedAt().Equal(externalReceivedAt) {
		t.Errorf("接收时间 = %v", foundAttributed.Result.ReceivedAt())
	}

	foundOrphan, exists, err := repository.FindByKey(ctx, orphan.Key)
	if err != nil || !exists {
		t.Fatalf("取回留存记录：%v exists=%v", err, exists)
	}
	if !foundOrphan.Unattributable ||
		foundOrphan.RawSemantics != "UNKNOWN_DECLARATION_REFERENCE" ||
		foundOrphan.ClaimedVersion != "version-nobody-knows" {
		t.Errorf("留存记录读回变形：%+v", foundOrphan)
	}
	if foundOrphan.Result.RawSemantics() != "" {
		t.Error("归属不上的记录凭空长出了监管事实")
	}
}

// TestASecondWriterGetsAlreadyRecorded 证同键第二份写入拿到`已有记录`且事务保持可用
// ——同一事务里紧接着读回先到者作答。
func TestASecondWriterGetsAlreadyRecorded(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()

	first := attributedRecord(t, "tenant-a", "resp-1", "submission-1")
	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, first)
		return err
	})

	second := attributedRecord(t, "tenant-a", "resp-1", "submission-1")
	second.ContentDigest = "digest-second"
	var outcome ports.ExternalResultSaveOutcome
	var winner ports.ExternalResultRecord
	var winnerFound bool
	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := repository.Save(txCtx, second)
		if err != nil {
			return err
		}
		outcome = saved
		winner, winnerFound, err = repository.FindByKey(txCtx, first.Key)
		return err
	})

	if outcome != ports.ExternalResultAlreadyRecorded {
		t.Fatalf("第二份写入结果 = %q，应为 ALREADY_RECORDED", outcome)
	}
	if !winnerFound || winner.ContentDigest != first.ContentDigest {
		t.Fatalf("同事务读回赢家失败：found=%v digest=%q", winnerFound, winner.ContentDigest)
	}
}

// TestLoadForSubmissionReturnsOnlyThatSubmissionsFacts 证同层一致性读面只回（租户+
// 提交版本）名下的可归属事实：他租户、他提交与归属不上的留存记录都不进来。
func TestLoadForSubmissionReturnsOnlyThatSubmissionsFacts(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()
	tenant := make2(t, domain.NewTenantID, "tenant-a")

	mine1 := attributedRecord(t, "tenant-a", "resp-1", "submission-1")
	mine2 := attributedRecord(t, "tenant-a", "resp-2", "submission-1")
	mine2.Result = attributedResult(t, "resp-2", "submission-1", domain.RegulatoryReceiptLayer)
	otherSubmission := attributedRecord(t, "tenant-a", "resp-3", "submission-9")
	otherTenant := attributedRecord(t, "tenant-b", "resp-4", "submission-1")
	orphan := unattributableRecord(t, "tenant-a", "resp-5")
	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ExternalResultRecord{mine1, mine2, otherSubmission, otherTenant, orphan} {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	results, err := repository.LoadForSubmission(ctx, tenant, make2(t, domain.NewSubmissionVersionID, "submission-1"))
	if err != nil {
		t.Fatalf("读回提交事实：%v", err)
	}
	if len(results) != 2 {
		t.Fatalf("读回 %d 条，应为 2（他租户、他提交与留存记录都不进来）", len(results))
	}
	layers := map[domain.ResultLayer]bool{}
	for _, result := range results {
		if result.Version().String() != "submission-1" {
			t.Errorf("混入了别的提交：%q", result.Version())
		}
		layers[result.Layer()] = true
	}
	if !layers[domain.BusinessAcceptanceLayer] || !layers[domain.RegulatoryReceiptLayer] {
		t.Errorf("各层事实缺席：%v", layers)
	}
}

// TestOtherTenantsAreInvisible 证否定结果不泄露另一个租户是否收到过同一来源响应。
func TestOtherTenantsAreInvisible(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()

	saved := attributedRecord(t, "tenant-a", "resp-shared", "submission-1")
	inTx2(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, saved)
		return err
	})

	elsewhere := ports.ExternalResultKey{TenantID: make2(t, domain.NewTenantID, "tenant-b"), SourceID: "resp-shared"}
	_, exists, err := repository.FindByKey(ctx, elsewhere)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("另一个租户读到了不属于它的接收记录")
	}
}

// TestWritesRefuseToRunOutsideATransaction 证写入不会在缺少事务时改用连接池。
func TestWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _ := newExternalResults(t)
	ctx := t.Context()

	record := attributedRecord(t, "tenant-a", "resp-1", "submission-1")
	if _, err := repository.Save(ctx, record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}

	_, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("被拒绝的写入仍然落库了")
	}
}

// TestRollbackLeavesNothingBehind 证接收记录与它所在的事务同生共死。
func TestRollbackLeavesNothingBehind(t *testing.T) {
	repository, transactor := newExternalResults(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	record := attributedRecord(t, "tenant-a", "resp-1", "submission-1")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, record); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	_, exists, err := repository.FindByKey(ctx, record.Key)
	if err != nil {
		t.Fatalf("查询出错：%v", err)
	}
	if exists {
		t.Error("回滚后接收记录仍在")
	}
}
