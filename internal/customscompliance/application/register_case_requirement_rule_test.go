package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 第六本册子登记用例的行为面：受理门逐格拒、幂等重放与内容冲突分得开、依赖故障
// 折未决。写口替身照真库代数（同键只答`已登记`，绝不顶替）。

type requirementStoreDouble struct {
	byKey       map[string]ports.CaseRequirementJudgment
	registerErr error
	loadErr     error
}

func newRequirementStore() *requirementStoreDouble {
	return &requirementStoreDouble{byKey: map[string]ports.CaseRequirementJudgment{}}
}

func requirementStoreKey(
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
) string {
	return tenant.String() + "|" + jurisdiction.String() + "|" + direction.String() + "|" + procedure.String()
}

func (double *requirementStoreDouble) RegisterCaseRequirementRule(
	_ context.Context,
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
	judgment ports.CaseRequirementJudgment,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	key := requirementStoreKey(tenant, jurisdiction, direction, procedure)
	if _, exists := double.byKey[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.byKey[key] = judgment
	return ports.CaseConfigurationRegistered, nil
}

func (double *requirementStoreDouble) JudgeCaseRequirement(
	_ context.Context,
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
) (ports.CaseRequirementJudgment, bool, error) {
	if double.loadErr != nil {
		return ports.CaseRequirementJudgment{}, false, double.loadErr
	}
	judgment, found := double.byKey[requirementStoreKey(tenant, jurisdiction, direction, procedure)]
	return judgment, found, nil
}

func newRequirementHandler(store *requirementStoreDouble) *application.RegisterCaseRequirementRuleHandler {
	return application.NewRegisterCaseRequirementRuleHandler(application.RegisterCaseRequirementRuleDeps{
		Rules: store,
		View:  store,
	})
}

func requirementCommand(t *testing.T, required bool, basis string) application.RegisterCaseRequirementRuleCommand {
	t.Helper()
	return application.RegisterCaseRequirementRuleCommand{
		TenantID:     configValue(t, domain.NewTenantID, "tenant-a"),
		Jurisdiction: configValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
		Direction:    domain.ExportManifest,
		Procedure:    configValue(t, domain.NewCustomsProcedureReference, "PROC/EXPORT-STANDARD"),
		Required:     required,
		Basis:        basis,
	}
}

func TestRegisteringACaseRequirementRuleLands(t *testing.T) {
	store := newRequirementStore()
	handler := newRequirementHandler(store)

	outcome, err := handler.Handle(t.Context(), requirementCommand(t, false, "CONTRACT/NO-CASE-V1"))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("登记：err=%v outcome=%v", err, outcome)
	}
	if len(store.byKey) != 1 {
		t.Fatalf("册上行数 = %d", len(store.byKey))
	}
	for _, judgment := range store.byKey {
		if judgment.Required || judgment.Basis != "CONTRACT/NO-CASE-V1" {
			t.Fatalf("落册内容失真：%+v", judgment)
		}
	}
}

// 受理门逐格拒：租户/辖区/方向/程序/依据任一缺席都不受理——「不要求」也必须带依据，
// 缺依据的登记会让「答否」与「没答」在册面上分不开。
func TestCaseRequirementRegistrationRefusesBlankFields(t *testing.T) {
	store := newRequirementStore()
	handler := newRequirementHandler(store)

	blankDirection := requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1")
	blankDirection.Direction = domain.ManifestDirectionInvalid

	blankBasis := requirementCommand(t, false, "   ")

	blankTenant := requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1")
	blankTenant.TenantID = domain.TenantID{}

	blankJurisdiction := requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1")
	blankJurisdiction.Jurisdiction = domain.RegulatoryJurisdictionReference{}

	blankProcedure := requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1")
	blankProcedure.Procedure = domain.CustomsProcedureReference{}

	for name, command := range map[string]application.RegisterCaseRequirementRuleCommand{
		"方向": blankDirection,
		"依据": blankBasis,
		"租户": blankTenant,
		"辖区": blankJurisdiction,
		"程序": blankProcedure,
	} {
		outcome, err := handler.Handle(t.Context(), command)
		if err != nil || outcome != application.ConfigurationNotAccepted {
			t.Fatalf("缺%s该拒：err=%v outcome=%v", name, err, outcome)
		}
	}
	if len(store.byKey) != 0 {
		t.Fatalf("被拒的登记落了册")
	}
}

func TestReRegisteringTheSameCaseRequirementRuleIsExisting(t *testing.T) {
	store := newRequirementStore()
	handler := newRequirementHandler(store)
	command := requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1")

	if outcome, err := handler.Handle(t.Context(), command); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}
	outcome, err := handler.Handle(t.Context(), command)
	if err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}
}

// 同键换内容是冲突不是换版：required 翻面或换依据都算，册面纹丝不动。
func TestReRegisteringADifferentCaseRequirementRuleConflicts(t *testing.T) {
	store := newRequirementStore()
	handler := newRequirementHandler(store)

	if outcome, err := handler.Handle(t.Context(),
		requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1")); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}

	flippedRequired, err := handler.Handle(t.Context(), requirementCommand(t, false, "REGULATION/CASE-REQUIRED-V1"))
	if err != nil || flippedRequired != application.ConfigurationContentConflict {
		t.Fatalf("翻面该是`内容冲突`：err=%v outcome=%v", err, flippedRequired)
	}
	changedBasis, err := handler.Handle(t.Context(), requirementCommand(t, true, "CONTRACT/NO-CASE-V1"))
	if err != nil || changedBasis != application.ConfigurationContentConflict {
		t.Fatalf("换依据该是`内容冲突`：err=%v outcome=%v", err, changedBasis)
	}

	for _, judgment := range store.byKey {
		if !judgment.Required || judgment.Basis != "REGULATION/CASE-REQUIRED-V1" {
			t.Fatalf("冲突顶掉了在册内容：%+v", judgment)
		}
	}
}

func TestCaseRequirementDependencyFailuresAreUndecided(t *testing.T) {
	writerDown := newRequirementStore()
	writerDown.registerErr = errors.New("writer unavailable")
	outcome, err := newRequirementHandler(writerDown).Handle(t.Context(),
		requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1"))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("写口故障该未决：err=%v outcome=%v", err, outcome)
	}

	viewDown := newRequirementStore()
	viewDown.byKey[requirementStoreKey(
		configValue(t, domain.NewTenantID, "tenant-a"),
		configValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
		domain.ExportManifest,
		configValue(t, domain.NewCustomsProcedureReference, "PROC/EXPORT-STANDARD"),
	)] = ports.CaseRequirementJudgment{Required: true, Basis: "REGULATION/CASE-REQUIRED-V1"}
	viewDown.loadErr = errors.New("view unavailable")
	outcome, err = newRequirementHandler(viewDown).Handle(t.Context(),
		requirementCommand(t, true, "REGULATION/CASE-REQUIRED-V1"))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("已在册但读不回该未决：err=%v outcome=%v", err, outcome)
	}
}
