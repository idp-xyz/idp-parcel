package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真库证 ADR-0105 的库面半边：一次评价多条问题项各成一行且与父行同事务；非序列问题项的种类列为空；
// 分组计数只读子表与父表的列、按序列种类数评价。夹具为 SYN 卡（S 级）。

// pendingSeriesEvaluation 造一份因缺燃油序列取值而待判断的评价：方案绑了 fuel-weekly，输入没带读数。
func pendingSeriesEvaluation(t *testing.T, id, tenant string) domain.PricingEvaluation {
	t.Helper()
	base := syntheticPlan(t)
	factor := evaluationValue(t, domain.ParseDecimal, "0.8")
	calculation, err := domain.NewSeriesRateSurcharge(domain.ReferenceSeriesFuelRate, factor, "freight")
	if err != nil {
		t.Fatalf("序列费率附加费：%v", err)
	}
	threshold, err := domain.NewWeight(evaluationValue(t, domain.ParseDecimal, "0"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("阈值：%v", err)
	}
	condition, err := domain.NewWeightFeatureCondition(domain.FeatureActualWeight, domain.ComparisonGreaterThanOrEqual, threshold)
	if err != nil {
		t.Fatalf("条件：%v", err)
	}
	trigger, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("触发条件：%v", err)
	}
	rule, err := domain.NewSurchargeRule("fuel", evaluationValue(t, domain.NewChargeCode, "FUEL"), "fuel", domain.ChargeEffectAdd, trigger, calculation)
	if err != nil {
		t.Fatalf("附加费规则：%v", err)
	}
	rule, err = rule.Standalone()
	if err != nil {
		t.Fatalf("standalone：%v", err)
	}
	dependency, err := domain.NewListedChargeDependency("freight", evaluationValue(t, domain.NewChargeCode, "FUEL"), []domain.ChargeCode{evaluationValue(t, domain.NewChargeCode, "BASE_FREIGHT")}, nil)
	if err != nil {
		t.Fatalf("费用依赖：%v", err)
	}
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesFuelRate, "fuel-weekly")
	if err != nil {
		t.Fatalf("绑定：%v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, []domain.ChargeDependency{dependency}, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("结构：%v", err)
	}
	plan, err := domain.NewPricingPlanVersion(base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("方案：%v", err)
	}
	sides, err := domain.NewDimensions(evaluationValue(t, domain.ParseDecimal, "10"), evaluationValue(t, domain.ParseDecimal, "10"), evaluationValue(t, domain.ParseDecimal, "10"), domain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("尺寸：%v", err)
	}
	subject, err := domain.NewAcceptedPackageSubject(evaluationValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("对象：%v", err)
	}
	actual, err := domain.NewWeight(evaluationValue(t, domain.ParseDecimal, "5"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("实重：%v", err)
	}
	input, err := domain.NewPricingInputSnapshot(evaluationValue(t, domain.NewTenantID, tenant), evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "Z1", actual, &sides, syntheticInput(t, tenant, "Z1").BusinessAt())
	if err != nil {
		t.Fatalf("输入：%v", err)
	}
	request, err := domain.NewEvaluationRequest(evaluationValue(t, domain.NewEvaluationID, id), plan, input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("请求：%v", err)
	}
	evaluation := domain.EvaluatePricing(request)
	if evaluation.Status() != domain.EvaluationPending {
		t.Fatalf("夹具状态 = %s %#v", evaluation.Status(), evaluation.Issues())
	}
	return evaluation
}

// issueFixture 把写口、读口与旁路观察的连接池装在**同一个**测试库上——pgtest.Pool 每次调用都是一个全新的库。
type issueFixture struct {
	repository *adapter.Evaluations
	read       *adapter.PendingSeriesEvaluations
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newIssueFixture(t *testing.T) issueFixture {
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
	read, err := adapter.NewPendingSeriesEvaluations(db)
	if err != nil {
		t.Fatalf("构造读口：%v", err)
	}
	return issueFixture{repository: repository, read: read, transactor: db.Transactor(), pool: pool}
}

func (fixture issueFixture) save(t *testing.T, ctx context.Context, evaluation domain.PricingEvaluation) {
	t.Helper()
	inEvaluationTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := fixture.repository.Save(txCtx, evaluation)
		return err
	})
}

// TestEvaluationIssuesLandInTheChildTableWithTheParent 证子表行与父行同事务落下：一次评价多条问题项各成一行，
// 带主体的行有种类与标识，没有主体的行两列为空；`已有记录`不重复写子行。
func TestEvaluationIssuesLandInTheChildTableWithTheParent(t *testing.T) {
	fixture := newIssueFixture(t)
	pool := fixture.pool
	ctx := t.Context()

	pending := pendingSeriesEvaluation(t, "eval-issue-series", "tenant-issue")
	fixture.save(t, ctx, pending)
	fixture.save(t, ctx, pending)

	rows, err := pool.Query(ctx,
		`SELECT ordinal, code, series_kind, series_id FROM parcel_pricing.evaluation_issue WHERE evaluation_id = $1 ORDER BY ordinal`,
		"eval-issue-series")
	if err != nil {
		t.Fatalf("查子表：%v", err)
	}
	defer rows.Close()
	type issueRow struct {
		ordinal    int
		code       string
		seriesKind *string
		seriesID   *string
	}
	var found []issueRow
	for rows.Next() {
		var row issueRow
		if err := rows.Scan(&row.ordinal, &row.code, &row.seriesKind, &row.seriesID); err != nil {
			t.Fatalf("扫子表：%v", err)
		}
		found = append(found, row)
	}
	if len(found) != len(pending.Issues()) || len(found) == 0 {
		t.Fatalf("子表行数 = %d，想要与问题项数 %d 一致且重放不重复", len(found), len(pending.Issues()))
	}
	if found[0].code != "REFERENCE_SERIES_UNRESOLVED" || found[0].seriesKind == nil || *found[0].seriesKind != "FUEL_RATE" || found[0].seriesID == nil || *found[0].seriesID != "fuel-weekly" {
		t.Fatalf("带主体的行变形：%#v", found[0])
	}

	// 完成评价带 AMOUNT_PRECISION_UNDECLARED（非序列问题项）：种类与标识两列为空。
	completed := evaluatedFixture(t, "eval-issue-plain", "tenant-issue", "Z1")
	if completed.Status() != domain.EvaluationCompleted {
		t.Fatalf("夹具状态 = %s", completed.Status())
	}
	fixture.save(t, ctx, completed)
	var kindIsNull, idIsNull bool
	if err := pool.QueryRow(ctx,
		`SELECT series_kind IS NULL, series_id IS NULL FROM parcel_pricing.evaluation_issue WHERE evaluation_id = $1 AND code = $2`,
		"eval-issue-plain", "AMOUNT_PRECISION_UNDECLARED").Scan(&kindIsNull, &idIsNull); err != nil {
		t.Fatalf("查非序列问题项：%v", err)
	}
	if !kindIsNull || !idIsNull {
		t.Fatal("非序列问题项的种类 / 标识列不为空")
	}
}

// TestPendingSeriesEvaluationsAreCountedByKindFromTheChildTable 证分组计数：按租户与待判断筛、按种类分组数评价；
// 别的租户与非待判断评价不入数；空租户答空列表。
func TestPendingSeriesEvaluationsAreCountedByKindFromTheChildTable(t *testing.T) {
	fixture := newIssueFixture(t)
	ctx := t.Context()

	fixture.save(t, ctx, pendingSeriesEvaluation(t, "eval-count-1", "tenant-count"))
	fixture.save(t, ctx, pendingSeriesEvaluation(t, "eval-count-2", "tenant-count"))
	fixture.save(t, ctx, pendingSeriesEvaluation(t, "eval-count-other", "tenant-other"))
	fixture.save(t, ctx, evaluatedFixture(t, "eval-count-completed", "tenant-count", "Z1"))

	counts, err := fixture.read.CountPendingSeriesEvaluations(ctx, evaluationValue(t, domain.NewTenantID, "tenant-count"))
	if err != nil {
		t.Fatalf("计数：%v", err)
	}
	if len(counts) != 1 || counts[0] != (ports.PendingSeriesEvaluationCount{Kind: "FUEL_RATE", EvaluationCount: 2}) {
		t.Fatalf("counts = %#v，想要 FUEL_RATE 两次评价", counts)
	}
	empty, err := fixture.read.CountPendingSeriesEvaluations(ctx, evaluationValue(t, domain.NewTenantID, "tenant-empty"))
	if err != nil || len(empty) != 0 {
		t.Fatalf("空租户：%#v err=%v", empty, err)
	}
}
