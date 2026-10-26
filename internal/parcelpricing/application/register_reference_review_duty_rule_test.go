package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证参考复核职责规则登记的编排（ADR-0262 决定一）：规则册的三格各译一格答案，没有租户的规则不到规则册，依赖故障
// 未决不吞。一租户一条、同内容重放、异内容冲突的判定在规则册，由真库用例证。

type dutyRuleRegistryDouble struct {
	outcome ports.ReferenceReviewDutyRuleSaveOutcome
	err     error
	saved   []domain.ReferenceReviewDutyRule
}

func (double *dutyRuleRegistryDouble) LoadReferenceReviewDutyRule(context.Context, domain.TenantID) (domain.ReferenceReviewDutyRule, bool, error) {
	return domain.ReferenceReviewDutyRule{}, false, nil
}

func (double *dutyRuleRegistryDouble) SaveReferenceReviewDutyRule(_ context.Context, rule domain.ReferenceReviewDutyRule) (ports.ReferenceReviewDutyRuleSaveOutcome, error) {
	double.saved = append(double.saved, rule)
	return double.outcome, double.err
}

func TestARegisteredReviewDutyRuleAnswersInTheRegisterAlgebra(t *testing.T) {
	rule := dutyRuleFor(t, "tenant-1", true, "").rule
	for saved, want := range map[ports.ReferenceReviewDutyRuleSaveOutcome]application.RegisterReferenceReviewDutyRuleOutcome{
		ports.ReferenceReviewDutyRuleSaved:             application.ReferenceReviewDutyRuleRecorded,
		ports.ReferenceReviewDutyRuleAlreadyRegistered: application.ReferenceReviewDutyRuleAlreadyOnRegister,
		ports.ReferenceReviewDutyRuleContentConflict:   application.ReferenceReviewDutyRuleConflict,
	} {
		registry := &dutyRuleRegistryDouble{outcome: saved}
		handler, err := application.NewRegisterReferenceReviewDutyRuleHandler(registry)
		if err != nil {
			t.Fatal(err)
		}
		outcome, err := handler.Handle(context.Background(), application.RegisterReferenceReviewDutyRuleCommand{Rule: rule})
		if err != nil || outcome != want || len(registry.saved) != 1 {
			t.Fatalf("%s：outcome = %s err = %v saved=%d", saved, outcome, err, len(registry.saved))
		}
	}

	registry := &dutyRuleRegistryDouble{outcome: ports.ReferenceReviewDutyRuleSaved}
	handler, _ := application.NewRegisterReferenceReviewDutyRuleHandler(registry)
	if outcome, err := handler.Handle(context.Background(), application.RegisterReferenceReviewDutyRuleCommand{}); err != nil ||
		outcome != application.ReferenceReviewDutyRuleNotAccepted || len(registry.saved) != 0 {
		t.Fatalf("零值规则：outcome = %s err = %v saved=%d", outcome, err, len(registry.saved))
	}
	registry.err = errors.New("rule register down")
	if outcome, err := handler.Handle(context.Background(), application.RegisterReferenceReviewDutyRuleCommand{Rule: rule}); err == nil ||
		outcome != application.ReferenceReviewDutyRuleUndecided {
		t.Fatalf("规则册故障：outcome = %s err = %v", outcome, err)
	}
	if _, err := application.NewRegisterReferenceReviewDutyRuleHandler(nil); err == nil {
		t.Fatal("没有规则册也装起来了")
	}
}
