package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func poundInput(t *testing.T, zone string) domain.PricingInputSnapshot {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	actual, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "5"), domain.WeightUnitPound)
	if err != nil {
		t.Fatalf("actual weight: %v", err)
	}
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, zone, actual, nil, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("input snapshot: %v", err)
	}
	return input
}

func poundCommand(t *testing.T, id string) application.EvaluatePricingCommand {
	t.Helper()
	request, err := domain.NewEvaluationRequest(
		mustValue(t, domain.NewEvaluationID, id), minimalPlan(t), poundInput(t, "Z1"), domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("evaluation request: %v", err)
	}
	return application.EvaluatePricingCommand{Request: request}
}

// Covers: ADR-0346 决定一、三——价表按 KG 查、输入按 LB 给的评价经编排完成并入册，换算记录随评价留下；
// 同标识重复请求认作同一次计算（借原评价记下的系数比对），不冲突、不重复入册。
func TestAPoundWeightIsConvertedAndTheSameRequestIsRecognisedAgain(t *testing.T) {
	fixture := newEvaluateFixture(t)

	first, err := fixture.handler.Handle(context.Background(), poundCommand(t, "eval-pound"))
	if err != nil || first.Outcome() != application.EvaluationRecorded {
		t.Fatalf("first = %q %v", first.Outcome(), err)
	}
	evaluation, _ := first.Evaluation()
	if evaluation.Status() != domain.EvaluationCompleted || len(evaluation.UnitConversions()) != 1 {
		t.Fatalf("status %s conversions %d issues %v", evaluation.Status(), len(evaluation.UnitConversions()), evaluation.Issues())
	}

	again, err := fixture.handler.Handle(context.Background(), poundCommand(t, "eval-pound"))
	if err != nil || again.Outcome() != application.EvaluationExistingResult || fixture.store.saved != 1 {
		t.Fatalf("again = %q %v saved %d", again.Outcome(), err, fixture.store.saved)
	}
}

// Covers: ADR-0346 决定二、三——换算接入之前答过单位不一致的评价（冲突）已在册：同标识重复请求照旧认作同一次
// 计算，不因为此刻能换算了就改判成冲突；新标识的同一输入则按换算评价。
func TestAnEvaluationRecordedAsAUnitMismatchBeforeConversionIsNotRewrittenByTheSameRequest(t *testing.T) {
	fixture := newEvaluateFixture(t)

	legacy := domain.EvaluatePricing(poundCommand(t, "eval-legacy").Request.WithRecordedUnitConversions(nil))
	if legacy.Status() != domain.EvaluationConflict || legacy.Issues()[0].Code() != "WEIGHT_UNIT_MISMATCH" {
		t.Fatalf("旧答复 = %s %v", legacy.Status(), legacy.Issues())
	}
	fixture.store.byID["eval-legacy"] = legacy

	repeated, err := fixture.handler.Handle(context.Background(), poundCommand(t, "eval-legacy"))
	if err != nil || repeated.Outcome() != application.EvaluationExistingResult {
		t.Fatalf("repeated = %q %v；此前答单位不一致的评价被此刻的目录改判了", repeated.Outcome(), err)
	}
	stored, _ := repeated.Evaluation()
	if stored.Status() != domain.EvaluationConflict {
		t.Fatalf("原评价被改写：%s", stored.Status())
	}

	fresh, err := fixture.handler.Handle(context.Background(), poundCommand(t, "eval-fresh"))
	if err != nil {
		t.Fatalf("fresh: %v", err)
	}
	if evaluation, _ := fresh.Evaluation(); evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("新标识的同一输入应按换算评价完成，得到 %s", evaluation.Status())
	}
}
