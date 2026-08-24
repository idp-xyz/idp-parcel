package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 第六本册子（case_requirement_rule）的写口用例。做法同 case_config_registry_test.go
// 那五本：断言穿只读视图取回、写入放进环境事务。三条要点——不可覆盖、「答否」也带
// 依据落册（found=false 的分界靠它守住）、无环境事务即拒。

func newCaseRequirementRegistry(t *testing.T) (*adapter.CaseRequirementRegistrations, *adapter.CaseRequirementView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	registry, err := adapter.NewCaseRequirementRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造建案规则写口：%v", err)
	}
	view, err := adapter.NewCaseRequirementView(fixture.db)
	if err != nil {
		t.Fatalf("构造建案规则读口：%v", err)
	}
	return registry, view, fixture
}

func requirementKey(t *testing.T) (domain.TenantID, domain.RegulatoryJurisdictionReference, domain.ManifestDirection, domain.CustomsProcedureReference) {
	t.Helper()
	return viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewRegulatoryJurisdictionReference, "JURIS/DE"),
		domain.ExportManifest,
		viewValue(t, domain.NewCustomsProcedureReference, "PROC/EXPORT-STANDARD")
}

// 登记为「不要求」的那一行也说得出依据并读得回来——「答否」与「没答」在数据上分开，
// 正是本册 found=false 分界的另一半。
func TestRegisteredCaseRequirementRuleIsReadBackThroughItsView(t *testing.T) {
	registry, view, fixture := newCaseRequirementRegistry(t)
	tenant, jurisdiction, direction, procedure := requirementKey(t)

	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCaseRequirementRule(ctx, tenant, jurisdiction, direction, procedure,
			ports.CaseRequirementJudgment{Required: false, Basis: "CONTRACT/NO-CASE-V1"})
	})
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("登记建案规则：err=%v outcome=%v", err, outcome)
	}

	judgment, found, err := view.JudgeCaseRequirement(t.Context(), tenant, jurisdiction, direction, procedure)
	if err != nil || !found {
		t.Fatalf("规则没读回：err=%v found=%v", err, found)
	}
	if judgment.Required || judgment.Basis != "CONTRACT/NO-CASE-V1" {
		t.Fatalf("往返走样：required=%v basis=%q", judgment.Required, judgment.Basis)
	}
}

// 同键再登不同内容改不动已在册那一行：写口只答`已登记`，内容比对是编排的事。
func TestCaseRequirementRuleIsNotOverwritten(t *testing.T) {
	registry, view, fixture := newCaseRequirementRegistry(t)
	tenant, jurisdiction, direction, procedure := requirementKey(t)

	if outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCaseRequirementRule(ctx, tenant, jurisdiction, direction, procedure,
			ports.CaseRequirementJudgment{Required: true, Basis: "REGULATION/CASE-REQUIRED-V1"})
	}); err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}

	outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCaseRequirementRule(ctx, tenant, jurisdiction, direction, procedure,
			ports.CaseRequirementJudgment{Required: false, Basis: "CONTRACT/NO-CASE-V1"})
	})
	if err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("再登：err=%v outcome=%v，要`已登记`", err, outcome)
	}

	judgment, found, err := view.JudgeCaseRequirement(t.Context(), tenant, jurisdiction, direction, procedure)
	if err != nil || !found {
		t.Fatalf("读回：err=%v found=%v", err, found)
	}
	if !judgment.Required || judgment.Basis != "REGULATION/CASE-REQUIRED-V1" {
		t.Fatalf("在册内容被顶替：required=%v basis=%q", judgment.Required, judgment.Basis)
	}
}

// 无环境事务时写口拒绝执行，而不是自己开一笔（同五本册子：登记要能与它所属的业务
// 动作同笔落地或同笔回滚）。
func TestCaseRequirementRegistryRequiresAmbientTransaction(t *testing.T) {
	registry, _, _ := newCaseRequirementRegistry(t)
	tenant, jurisdiction, direction, procedure := requirementKey(t)

	if _, err := registry.RegisterCaseRequirementRule(t.Context(), tenant, jurisdiction, direction, procedure,
		ports.CaseRequirementJudgment{Required: true, Basis: "REGULATION/CASE-REQUIRED-V1"}); err == nil {
		t.Fatalf("无环境事务的写入要拒")
	}
}

// 集合外的方向是调用方编程错误，不是「实例还没登记」：写不进去也不悄悄写成一行
// （同读口与其余写口对枚举零值的态度）。
func TestCaseRequirementRegistryRejectsUnknownDirection(t *testing.T) {
	registry, _, fixture := newCaseRequirementRegistry(t)
	tenant, jurisdiction, _, procedure := requirementKey(t)

	_, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCaseRequirementRule(ctx, tenant, jurisdiction,
			domain.ManifestDirectionInvalid, procedure,
			ports.CaseRequirementJudgment{Required: true, Basis: "REGULATION/CASE-REQUIRED-V1"})
	})
	if err == nil {
		t.Fatalf("集合外方向要报错")
	}
}
