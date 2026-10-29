package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// TestAnEvaluationWithADeclaredAddressNatureIsReadBackWithItAndReplaysEqual 证 ADR-0347 决定四在真库上：声明了收件地址性质的
// 评价写入评价登记册，读回经整图重验，声明随快照回来；读回的评价重放得同一个摘要。
func TestAnEvaluationWithADeclaredAddressNatureIsReadBackWithItAndReplaysEqual(t *testing.T) {
	repository, transactor := newEvaluations(t)
	ctx := t.Context()

	kilograms, err := domain.NewWeightFromString("5", domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("重量：%v", err)
	}
	subject, err := domain.NewAcceptedPackageSubject(evaluationValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("主体：%v", err)
	}
	input, err := domain.NewPricingInputSnapshot(
		evaluationValue(t, domain.NewTenantID, "tenant-a"), evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, "Z1", kilograms, nil, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("输入：%v", err)
	}
	input, err = input.WithAddressNature(domain.AddressNatureResidential)
	if err != nil {
		t.Fatalf("地址性质：%v", err)
	}
	plan := syntheticPlan(t)
	evaluation := pptest.Evaluate(t, "eval-address-nature", plan, input)
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("夹具 = %s", evaluation.Status())
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
	if snapshotOf(t, found) != snapshotOf(t, evaluation) || !strings.Contains(snapshotOf(t, found), `"addressNature":"RESIDENTIAL"`) {
		t.Fatalf("读回快照与写入不同答，或没带地址性质\n写入=%s\n读回=%s", snapshotOf(t, evaluation), snapshotOf(t, found))
	}
	if nature, declared := found.Input().AddressNature(); !declared || nature != domain.AddressNatureResidential {
		t.Fatalf("读回的地址性质 = %q %v", nature, declared)
	}
	replay, err := domain.ReplayPricingEvaluation(evaluationValue(t, domain.NewEvaluationID, "eval-address-nature-replay"), found, plan, domain.EvidenceSynthetic)
	if err != nil || replay.Status() != domain.EvaluationCompleted || replay.SemanticDigest() != found.SemanticDigest() {
		t.Fatalf("读回的评价重放 = %s %v", replay.Status(), err)
	}
}
