package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证试算对收件地址性质的接收（ADR-0347 决定四）：读它的卡要发起方声明，没声明就像分区、邮编路线一样点名缺项，
// 不套默认住宅或商业；声明了就进输入快照，读它的规则据此命中。

// residentialDeliveryPlan 是在 minimalPlan 之上加一条「住宅派送费」的卡：地址性质为住宅则加收 6 美元。
func residentialDeliveryPlan(t *testing.T, id string) domain.PricingPlanVersion {
	t.Helper()
	base := minimalPlan(t)
	condition, err := domain.NewCategoryFeatureCondition(domain.FeatureAddressNature, "RESIDENTIAL")
	if err != nil {
		t.Fatalf("condition: %v", err)
	}
	trigger, err := domain.NewTrigger(condition)
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	amount, err := domain.NewMoneyFromString("6", mustValue(t, domain.NewCurrency, "USD"))
	if err != nil {
		t.Fatalf("money: %v", err)
	}
	calculation, err := domain.NewFixedAmountSurcharge(amount)
	if err != nil {
		t.Fatalf("calculation: %v", err)
	}
	rule, err := domain.NewSurchargeRule("residential-delivery", mustValue(t, domain.NewChargeCode, "RESIDENTIAL_DELIVERY_SURCHARGE"),
		"住宅派送费", domain.ChargeEffectAdd, trigger, calculation)
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	rule, err = rule.Standalone()
	if err != nil {
		t.Fatalf("standalone: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures([]domain.SurchargeRule{rule}, nil, nil)
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, id, "v1")
	if err != nil {
		t.Fatalf("plan reference: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(reference, base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func estimateCommandWithSides(t *testing.T) application.FormEstimateEvaluationsCommand {
	t.Helper()
	command := estimateCommand(t)
	sides, err := domain.NewDimensions(mustValue(t, domain.ParseDecimal, "10"), mustValue(t, domain.ParseDecimal, "10"), mustValue(t, domain.ParseDecimal, "10"), domain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("dimensions: %v", err)
	}
	command.Dimensions = &sides
	return command
}

func declaredNature(t *testing.T, text string) *domain.AddressNature {
	t.Helper()
	nature, err := domain.NewAddressNature(text)
	if err != nil {
		t.Fatalf("address nature: %v", err)
	}
	return &nature
}

// Covers: UC-PP-001「输入不全」+ ADR-0347 决定四——声明没给地址性质而这张卡有规则按它判定：这张卡点名缺 ADDRESS_NATURE，
// 别的卡照算；不是评价落待判断，也没有任何默认性质。
func TestEstimateNamesTheMissingAddressNatureForTheCardThatReadsIt(t *testing.T) {
	reading := residentialDeliveryPlan(t, "plan-residential")
	plain := planWithIdentity(t, minimalPlan(t), "plan-plain")
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{reading, plain}})

	result, err := handler.Handle(context.Background(), estimateCommandWithSides(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 2 {
		t.Fatalf("result = %+v", result)
	}
	for _, candidate := range result.Candidates {
		switch candidate.Plan.ID() {
		case "plan-residential":
			if candidate.Answer != application.EstimateCandidateInputIncomplete ||
				len(candidate.Missing) != 1 || candidate.Missing[0] != application.EstimateMissingAddressNature {
				t.Fatalf("the card that reads the nature = %+v, want input incomplete naming ADDRESS_NATURE", candidate)
			}
		case "plan-plain":
			if candidate.Answer != application.EstimateCandidateEvaluated {
				t.Fatalf("the card that never reads the nature = %+v, want evaluated", candidate)
			}
		default:
			t.Fatalf("unexpected candidate %+v", candidate)
		}
	}
}

// Covers: ADR-0347 决定四——声明了地址性质：住宅命中住宅派送费，商业不命中；声明进评价输入快照。
func TestEstimateCarriesTheDeclaredAddressNatureIntoTheEvaluation(t *testing.T) {
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{residentialDeliveryPlan(t, "plan-residential")}})

	residential := estimateCommandWithSides(t)
	residential.AddressNature = declaredNature(t, "RESIDENTIAL")
	result, err := handler.Handle(context.Background(), residential)
	if err != nil || len(result.Candidates) != 1 {
		t.Fatalf("residential: result=%+v err=%v", result, err)
	}
	evaluation := result.Candidates[0].Evaluation
	if result.Candidates[0].Answer != application.EstimateCandidateEvaluated || evaluation.Status() != domain.EvaluationCompleted || len(evaluation.ChargeLines()) != 2 {
		t.Fatalf("residential candidate = %+v lines=%d", result.Candidates[0], len(evaluation.ChargeLines()))
	}
	if nature, declared := evaluation.Input().AddressNature(); !declared || nature != domain.AddressNatureResidential {
		t.Fatalf("evaluation input nature = %q %v", nature, declared)
	}

	commercial := estimateCommandWithSides(t)
	commercial.AddressNature = declaredNature(t, "COMMERCIAL")
	result, err = handler.Handle(context.Background(), commercial)
	if err != nil || len(result.Candidates) != 1 || len(result.Candidates[0].Evaluation.ChargeLines()) != 1 {
		t.Fatalf("commercial: result=%+v err=%v, want the base line only", result, err)
	}
}

// Covers: ADR-0347 决定四——缺项一并点名（分区与地址性质都没给时两格都在，先后固定）；地址性质进试算评价标识：同一份声明
// 派生同一标识，多声明一格地址性质标识就不同（评价内容不同，不能静默同标识）；没声明时不追加任何一段，标识的逐位不变由
// 跨树对拍证（票面完成记录）。
func TestEstimateNamesEveryMissingInputAndKeepsTheIdentityOfDeclarationsWithoutNature(t *testing.T) {
	plain := planWithIdentity(t, minimalPlan(t), "plan-plain")
	reading := residentialDeliveryPlan(t, "plan-residential")
	handler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{reading}})
	noZone := estimateCommandWithSides(t)
	noZone.Zone = ""
	result, err := handler.Handle(context.Background(), noZone)
	if err != nil || len(result.Candidates) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	missing := result.Candidates[0].Missing
	if len(missing) != 2 || missing[0] != application.EstimateMissingZone || missing[1] != application.EstimateMissingAddressNature {
		t.Fatalf("missing = %v, want ZONE then ADDRESS_NATURE", missing)
	}

	plainHandler := newEstimateHandler(t, &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{plain}})
	identityOf := func(command application.FormEstimateEvaluationsCommand) string {
		answer, err := plainHandler.Handle(context.Background(), command)
		if err != nil || len(answer.Candidates) != 1 {
			t.Fatalf("answer=%+v err=%v", answer, err)
		}
		return answer.Candidates[0].Evaluation.ID().String()
	}
	withoutNature := identityOf(estimateCommandWithSides(t))
	again := identityOf(estimateCommandWithSides(t))
	if withoutNature != again {
		t.Fatalf("the same declaration got two identities: %s, %s", withoutNature, again)
	}
	declared := estimateCommandWithSides(t)
	declared.AddressNature = declaredNature(t, "RESIDENTIAL")
	if identityOf(declared) == withoutNature {
		t.Fatal("declaring the address nature left the evaluation identity unchanged")
	}
}
