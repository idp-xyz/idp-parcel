package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var collaborationFormedAt = time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)

func collaborationSpec(t *testing.T, kind domain.DutyObligationKind) domain.DutyCollaborationSpec {
	t.Helper()
	spec := domain.DutyCollaborationSpec{
		Kind:        kind,
		Scope:       mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		Obligor:     mustValue(t, domain.NewLegalObligorReference, "importer-of-record-1"),
		Requirement: mustValue(t, domain.NewPaymentRequirementSource, "US-IMPORT/TYPE-86/duty-notice"),
		Target:      mustValue(t, domain.NewResponsibilityTargetReference, "settlement-accounting"),
		FormedAt:    collaborationFormedAt,
	}
	switch kind {
	case domain.ObligationFromAssessedDuty:
		spec.Duty = mustValue(t, domain.NewAssessedDutyReference, "assessed-duty/v1")
	case domain.ObligationExplicitlyNotRequired:
		spec.NoPayBasis = "DE_MINIMIS/procedure-rule-7"
	}
	return spec
}

// Covers: CC CONTEXT「税费付款协作事项」语言——两格各有形状（核定税费格必带税费引用、
// 明确无需付款格必带真实程序依据），互相的字段两向拦；类型上没有支付指令/付款交易/
// 客户回收/放行字段（协作事项不等于那些）。
func TestCollaborationTakesExactlyTwoObligationShapes(t *testing.T) {
	assessed, err := domain.FormDutyCollaboration(collaborationSpec(t, domain.ObligationFromAssessedDuty))
	if err != nil {
		t.Fatalf("form assessed collaboration: %v", err)
	}
	duty, has := assessed.Duty()
	if !has || duty.String() != "assessed-duty/v1" {
		t.Fatalf("duty = %v has = %v", duty, has)
	}
	if _, has := assessed.NoPayBasis(); has {
		t.Fatal("核定税费格凭空带了无需付款依据")
	}

	notRequired, err := domain.FormDutyCollaboration(collaborationSpec(t, domain.ObligationExplicitlyNotRequired))
	if err != nil {
		t.Fatalf("form not-required collaboration: %v", err)
	}
	basis, has := notRequired.NoPayBasis()
	if !has || basis != "DE_MINIMIS/procedure-rule-7" {
		t.Fatalf("basis = %q has = %v", basis, has)
	}

	crossed := collaborationSpec(t, domain.ObligationFromAssessedDuty)
	crossed.NoPayBasis = "leftover"
	if _, err := domain.FormDutyCollaboration(crossed); !errors.Is(err, domain.ErrInvalidDutyCollaboration) {
		t.Fatalf("err = %v; 核定格带上了无需付款依据", err)
	}
	bare := collaborationSpec(t, domain.ObligationExplicitlyNotRequired)
	bare.NoPayBasis = ""
	if _, err := domain.FormDutyCollaboration(bare); !errors.Is(err, domain.ErrInvalidDutyCollaboration) {
		t.Fatalf("err = %v; 说不出程序依据的无需付款被收下了", err)
	}
}

// Covers: CC CONTEXT「缺少税费结果不能被解释为无需付款」——没有义务依据的输入走不进
// 任何一格（独立哨兵，编排据以保持未决：不形成支付指令，也不默认无需付款）；法定
// 义务人必备（法定义务人、实际付款方与最终承担客户不能互相推导——这里只记第一个，
// CONTEXT「不能互相推导」）。
func TestMissingDutyResultsCannotMeanNoPaymentNeeded(t *testing.T) {
	unfounded := domain.DutyCollaborationSpec{
		Scope:       mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		Obligor:     mustValue(t, domain.NewLegalObligorReference, "importer-of-record-1"),
		Requirement: mustValue(t, domain.NewPaymentRequirementSource, "US-IMPORT/TYPE-86/duty-notice"),
		Target:      mustValue(t, domain.NewResponsibilityTargetReference, "settlement-accounting"),
		FormedAt:    collaborationFormedAt,
	}
	if _, err := domain.FormDutyCollaboration(unfounded); !errors.Is(err, domain.ErrCollaborationNotFundable) {
		t.Fatalf("err = %v, want ErrCollaborationNotFundable——没有结果不等于不用付", err)
	}

	missingObligor := collaborationSpec(t, domain.ObligationFromAssessedDuty)
	missingObligor.Obligor = domain.LegalObligorReference{}
	if _, err := domain.FormDutyCollaboration(missingObligor); !errors.Is(err, domain.ErrInvalidDutyCollaboration) {
		t.Fatalf("err = %v; 没有法定义务人的协作被收下了", err)
	}
}
