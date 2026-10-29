package postgres_test

import (
	"bytes"
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件在真库上证 ADR-0348 决定四：声明了总额下限的价卡写 PPC-6，登记入册、按适用条件装回时规范化版本与内容摘要不变；
// 它评出的带补差行的评价写入评价登记册、读回经整图重验（补差行的码与方向配对、语义摘要自校）并重放得同一个摘要。
// 价卡与评价各是一份快照文档，库里规范化列只要求非空，所以两版并存不需要迁移。

func boundedCatalogPlan(t *testing.T, planID string) domain.PricingPlanVersion {
	t.Helper()
	base := catalogPlan(t, planID, "v1", domain.PricingDirectionSell, "10")
	usd := evaluationValue(t, domain.NewCurrency, "USD")
	minimum := catalogMoney(t, "25", usd)
	bounds, err := domain.NewTotalChargeBounds(&minimum, nil)
	if err != nil {
		t.Fatalf("总额下限：%v", err)
	}
	structures, err := (domain.PricingPlanStructures{}).WithTotalChargeBounds(bounds)
	if err != nil {
		t.Fatalf("方案结构：%v", err)
	}
	reference, err := domain.NewVersionReference(domain.ArtifactPricingPlan, planID+"-bounded", "v1", "sha256:syn-"+planID+"-bounded")
	if err != nil {
		t.Fatalf("方案引用：%v", err)
	}
	plan, err := domain.NewPricingPlanVersion(reference, base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("带总额下限的价卡：%v", err)
	}
	return plan
}

// Covers: AT-PP-039
func TestAPriceCardWithTotalBoundsIsRegisteredAndLoadedBackAsPPC6(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	plan := boundedCatalogPlan(t, "plan-bounds-card")
	if plan.CanonicalizationVersion() != "PPC-6" {
		t.Fatalf("带总额下限的价卡写 %s，应是 PPC-6", plan.CanonicalizationVersion())
	}
	if outcome := registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", plan)); outcome != ports.PriceCardRegistered {
		t.Fatalf("首登 outcome = %d，想要 PriceCardRegistered", outcome)
	}
	// 同一份内容重登是幂等的重放，不是内容冲突：规范化版本与摘要在库里随这一版往返。
	if outcome := registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", plan)); outcome == ports.PriceCardRegistered {
		t.Fatalf("同一份 PPC-6 价卡重登不该再答首登")
	}

	loaded, err := catalog.LoadApplicable(ctx, evaluationValue(t, domain.NewTenantID, "tenant-a"), domain.PricingDirectionSell,
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"), catalogAsOf)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("装载适用价卡 = %d 张，err = %v", len(loaded), err)
	}
	if loaded[0].CanonicalizationVersion() != "PPC-6" || loaded[0].ContentDigest() != plan.ContentDigest() ||
		!bytes.Equal(planSnapshotBytes(t, loaded[0]), planSnapshotBytes(t, plan)) {
		t.Fatalf("装回的价卡版本 %s、摘要或快照与登记不同", loaded[0].CanonicalizationVersion())
	}
	bounds, declared := loaded[0].Structures().TotalChargeBounds()
	minimum, hasMinimum := bounds.Minimum()
	if !declared || !hasMinimum || minimum.Amount().String() != "25" {
		t.Fatalf("装回的总额下限 = %v %v", declared, minimum)
	}
}

// Covers: AT-PP-030、AT-PP-039
func TestAnEvaluationWithAMinimumChargeSupplementIsReadBackAndReplaysEqual(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	plan := boundedCatalogPlan(t, "plan-bounds-eval")
	evaluation := pptest.Evaluate(t, "eval-bounds", plan, syntheticInput(t, "tenant-a", "Z1"))
	total, completed := evaluation.Total()
	if evaluation.Status() != domain.EvaluationCompleted || !completed || total.Amount().String() != "25" {
		t.Fatalf("夹具 = %s 合计 %v；10 的基础运费应被下限 25 抬起", evaluation.Status(), total)
	}
	lines := evaluation.ChargeLines()
	if len(lines) != 2 || lines[1].Code() != domain.MinimumChargeCode() || lines[1].Amount().Amount().String() != "15" {
		t.Fatalf("费用行 = %d 条，想要基础运费加 MINIMUM_CHARGE 15", len(lines))
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
	if found.PlanCanonicalizationVersion() != "PPC-6" || snapshotOf(t, found) != snapshotOf(t, evaluation) {
		t.Fatalf("读回的评价版本 %s 或快照与写入不同答", found.PlanCanonicalizationVersion())
	}
	replay, err := domain.ReplayPricingEvaluation(evaluationValue(t, domain.NewEvaluationID, "eval-bounds-replay"), found, plan, domain.EvidenceSynthetic)
	if err != nil || replay.Status() != domain.EvaluationCompleted || replay.SemanticDigest() != found.SemanticDigest() {
		t.Fatalf("读回的评价重放 = %s %v", replay.Status(), err)
	}
}
