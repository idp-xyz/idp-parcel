package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// TestAConvertedEvaluationIsReadBackWithItsConversionRecordAndReplaysEqual 证 ADR-0346 决定三在真库上：
// 价表按 KG、输入按 LB 的评价写入评价登记册，读回经整图重验（含换算记录逐条自洽与语义摘要自校），
// 换算的原值、换算值与系数随快照回来；读回的评价按记下的系数重放得同一个摘要。
func TestAConvertedEvaluationIsReadBackWithItsConversionRecordAndReplaysEqual(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	pounds, err := domain.NewWeightFromString("5", domain.WeightUnitPound)
	if err != nil {
		t.Fatalf("重量：%v", err)
	}
	subject, err := domain.NewAcceptedPackageSubject(evaluationValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("主体：%v", err)
	}
	input, err := domain.NewPricingInputSnapshot(
		evaluationValue(t, domain.NewTenantID, "tenant-a"), evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "Z1", pounds, nil, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("输入：%v", err)
	}
	plan := syntheticPlan(t)
	evaluation := pptest.Evaluate(t, "eval-converted", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted || len(evaluation.UnitConversions()) != 1 {
		t.Fatalf("夹具 = %s，换算 %d 条；合成价卡没把磅换成千克", evaluation.Status(), len(evaluation.UnitConversions()))
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
	steps := found.UnitConversions()
	if len(steps) != 1 || steps[0].Factor() != "0.45359237" || steps[0].Original()[0].String() != "5" || steps[0].Converted()[0].String() != "2.26796185" {
		t.Fatalf("换算记录读回 = %+v", steps)
	}

	replay, err := domain.ReplayPricingEvaluation(evaluationValue(t, domain.NewEvaluationID, "eval-converted-replay"), found, plan, domain.EvidenceSynthetic)
	if err != nil || replay.Status() != domain.EvaluationCompleted || replay.SemanticDigest() != found.SemanticDigest() {
		t.Fatalf("读回的评价重放 = %s %v", replay.Status(), err)
	}
}
