package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证评价登记册的行为：写入代数由 ON CONFLICT 承担、
// 失败评价照存、读回经领域整图重验（含语义摘要自校）、事务纪律由 RequireExecutor
// 拦住。断言一律在事务闭包外（Goexit 会挂死连接）。
//
// 夹具不手搓评价——评价是闭合计算记录，手搓等于绕过守恒不变量。这里用合成价卡
// 真算一遍 domain.EvaluatePricing，成功与失败两种形态都来自真实计算路径。

func newEvaluations(t *testing.T) (*adapter.Evaluations, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewEvaluations(db)
	if err != nil {
		t.Fatalf("构造评价登记册：%v", err)
	}
	return repository, db.Transactor()
}

func inEvaluationTx(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// evaluationValue 是本包用例的统一失败出口，体只转调 pptest.Value——同一件事不写第二份（票 sa-cc/18）；
// 保留本地名是因为同包十余份用例都在叫它。
func evaluationValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	return pptest.Value(t, construct, raw)
}

// syntheticPlan 造一张最小可评价的合成价卡（S 级证据）：单分区重量段费率表+实重策略，三条引用都带指纹、不声明
// 金额取整策略。构造走本上下文的测试专属夹具 pptest，值全部在这里显式给出（票 sa-cc/17）。
func syntheticPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.FingerprintReference(t, domain.ArtifactPricingPlan, "plan-1", "v1", "sha256:syn-plan"),
		TableReference:        pptest.FingerprintReference(t, domain.ArtifactRateTable, "table-1", "v1", "sha256:syn-table"),
		WeightPolicyReference: pptest.FingerprintReference(t, domain.ArtifactWeightPolicy, "weight-1", "v1", "sha256:syn-weight"),
		Scope:                 "scope-1",
		Direction:             domain.PricingDirectionSell,
		Purpose:               domain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "entry-1",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "10",
		WeightRounding:      domain.RoundingCeiling,
		WeightStepKilograms: "0.5",
		AmountRounding:      nil,
	})
}

func syntheticInput(t *testing.T, tenant, zone string) domain.PricingInputSnapshot {
	t.Helper()
	return pptest.Input(t, pptest.InputSpec{
		Tenant:     tenant,
		Scope:      "scope-1",
		PackageID:  "package-1",
		Zone:       zone,
		Kilograms:  "5",
		BusinessAt: time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	})
}

// evaluatedFixture 真算一份评价：zone=Z1 命中费率得 COMPLETED，Z9 落空得失败形态。
func evaluatedFixture(t *testing.T, id, tenant, zone string) domain.PricingEvaluation {
	t.Helper()
	return pptest.Evaluate(t, id, syntheticPlan(t), syntheticInput(t, tenant, zone))
}

func snapshotOf(t *testing.T, evaluation domain.PricingEvaluation) string {
	t.Helper()
	raw, err := domain.MarshalEvaluationSnapshot(evaluation)
	if err != nil {
		t.Fatalf("折装快照：%v", err)
	}
	return string(raw)
}

// TestCompletedEvaluationIsReadBackUnchanged 证成功评价原样读回：读回经领域整图
// 重验（费用行守恒+语义摘要自校），快照逐字节同答。
func TestCompletedEvaluationIsReadBackUnchanged(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	evaluation := evaluatedFixture(t, "eval-round", "tenant-a", "Z1")
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("夹具状态 = %q；合成价卡没算成", evaluation.Status())
	}

	var outcome ports.EvaluationSaveOutcome
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := repository.Save(txCtx, evaluation)
		outcome = saved
		return err
	})
	if outcome != ports.EvaluationSaved {
		t.Fatalf("首写 outcome = %d", outcome)
	}

	found, exists, err := repository.FindByID(ctx, evaluation.ID())
	if err != nil || !exists {
		t.Fatalf("读回评价：%v exists=%v", err, exists)
	}
	if snapshotOf(t, found) != snapshotOf(t, evaluation) {
		t.Fatalf("读回快照与写入不同答\n写入=%s\n读回=%s", snapshotOf(t, evaluation), snapshotOf(t, found))
	}
	if _, has := found.Total(); !has {
		t.Fatalf("成功评价读回缺总额")
	}
	if _, has := found.MatchedRate(); !has {
		t.Fatalf("成功评价读回缺命中费率")
	}
}

// TestFailedEvaluationIsRecordedWithIssues 证失败评价照存——失败是版本化的计算事实
// 不是丢弃品：issues 原样读回，无总额无命中。
func TestFailedEvaluationIsRecordedWithIssues(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	failed := evaluatedFixture(t, "eval-fail", "tenant-a", "Z9")
	if failed.Status() == domain.EvaluationCompleted {
		t.Fatalf("夹具没落空；zone Z9 不该命中")
	}
	if len(failed.Issues()) == 0 {
		t.Fatalf("失败夹具没有 issues")
	}

	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, failed)
		return err
	})

	found, exists, err := repository.FindByID(ctx, failed.ID())
	if err != nil || !exists {
		t.Fatalf("读回失败评价：%v exists=%v", err, exists)
	}
	if found.Status() != failed.Status() {
		t.Fatalf("状态 = %q, 想要 %q", found.Status(), failed.Status())
	}
	if len(found.Issues()) != len(failed.Issues()) ||
		found.Issues()[0].Code() != failed.Issues()[0].Code() {
		t.Fatalf("issues 没原样读回：%+v", found.Issues())
	}
	if _, has := found.Total(); has {
		t.Fatalf("失败评价读回带总额")
	}
}

// TestSecondWriterGetsAlreadyRecorded 证写入代数：同标识第二份答`已有记录`且原评价
// 不被顶替；ON CONFLICT DO NOTHING 保事务可用——同一事务内还能读回原评价作答。
func TestSecondWriterGetsAlreadyRecorded(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	original := evaluatedFixture(t, "eval-dup", "tenant-a", "Z1")
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		_, err := repository.Save(txCtx, original)
		return err
	})

	// 同标识装不同输入的冒名份——冒名判定属应用层，库只答`已有记录`不顶替。
	impostor := evaluatedFixture(t, "eval-dup", "tenant-a", "Z9")

	var outcome ports.EvaluationSaveOutcome
	var foundInTx domain.PricingEvaluation
	var existsInTx bool
	inEvaluationTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := repository.Save(txCtx, impostor)
		if err != nil {
			return err
		}
		outcome = saved
		found, exists, err := repository.FindByID(txCtx, original.ID())
		if err != nil {
			return err
		}
		foundInTx, existsInTx = found, exists
		return nil
	})
	if outcome != ports.EvaluationAlreadyRecorded {
		t.Fatalf("重写 outcome = %d, 想要 EvaluationAlreadyRecorded", outcome)
	}
	if !existsInTx {
		t.Fatalf("同事务读回原评价不存在")
	}
	if snapshotOf(t, foundInTx) != snapshotOf(t, original) {
		t.Fatalf("原评价被顶替了")
	}
}

// TestSaveOutsideTransactionIsRejected 证事务纪律：评价落库必须与同一步的意图发布
// 同生共死，无事务写一律拒。
func TestSaveOutsideTransactionIsRejected(t *testing.T) {
	repository, _ := newEvaluations(t)
	ctx := t.Context()

	if _, err := repository.Save(ctx, evaluatedFixture(t, "eval-notx", "tenant-a", "Z1")); err == nil {
		t.Fatalf("无事务写入被接受了")
	}
}

// TestRollbackLeavesNothing 证回滚干净：事务失败后库里没有半份评价。
func TestRollbackLeavesNothing(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	evaluation := evaluatedFixture(t, "eval-rollback", "tenant-a", "Z1")
	rollback := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := repository.Save(txCtx, evaluation); err != nil {
			return err
		}
		return context.Canceled
	})
	if rollback == nil {
		t.Fatalf("事务该失败没失败")
	}

	if _, exists, err := repository.FindByID(ctx, evaluation.ID()); err != nil || exists {
		t.Fatalf("回滚后仍有残留：err=%v exists=%v", err, exists)
	}
}
