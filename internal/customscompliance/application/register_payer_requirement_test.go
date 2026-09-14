package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 「真实程序核对税费付款时要不要求付款人」那一格规则的登记面（票 sa-cc/12 裁决 1，形照 sa-cc/06 那一格）：
// 两值各登得进、重放`已存在`、换值`内容冲突`不顶替、形状缺格拒、依赖故障未决。替身照真库代数——同键只答
// `已登记`，无 UPDATE 路径。册上没有任何预填：哪个程序要、哪个不要都由用例一条条登进来。

type payerRuleStoreDouble struct {
	rows        map[string]domain.PayerRequirement
	registerErr error
	loadErr     error
}

func newPayerRuleStore() *payerRuleStoreDouble {
	return &payerRuleStoreDouble{rows: map[string]domain.PayerRequirement{}}
}

func (double *payerRuleStoreDouble) RegisterPayerRequirement(
	_ context.Context, tenant domain.TenantID, procedure domain.CustomsProcedureReference,
	requirement domain.PayerRequirement,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	key := payerRuleKey(tenant, procedure)
	if _, exists := double.rows[key]; exists {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	double.rows[key] = requirement
	return ports.CaseConfigurationRegistered, nil
}

func (double *payerRuleStoreDouble) LoadPayerRequirement(
	_ context.Context, tenant domain.TenantID, procedure domain.CustomsProcedureReference,
) (domain.PayerRequirement, bool, error) {
	if double.loadErr != nil {
		return domain.PayerRequirementInvalid, false, double.loadErr
	}
	requirement, found := double.rows[payerRuleKey(tenant, procedure)]
	return requirement, found, nil
}

func newPayerRuleHandler(rules *payerRuleStoreDouble) *application.RegisterCaseConfigurationHandler {
	return application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		PayerRules: rules, PayerRuleView: rules,
	})
}

func payerRuleCommand(t *testing.T, procedure string, requirement domain.PayerRequirement) application.RegisterPayerRequirementCommand {
	t.Helper()
	return application.RegisterPayerRequirementCommand{
		TenantID:    configValue(t, domain.NewTenantID, "tenant-a"),
		Procedure:   configValue(t, domain.NewCustomsProcedureReference, procedure),
		Requirement: requirement,
	}
}

// 两值各是一条独立的登记：要求与不要求按程序各登一行，读回即登进去的那一格；册上没登的程序读不到——
// 「未登记」由读口的 found=false 说，不由任何默认值顶。
func TestBothPayerRequirementValuesRegisterPerProcedure(t *testing.T) {
	rules := newPayerRuleStore()
	handler := newPayerRuleHandler(rules)

	for procedure, requirement := range map[string]domain.PayerRequirement{
		"SYN-PROC-REQUIRING": domain.PayerRequired,
		"SYN-PROC-WAIVING":   domain.PayerNotRequired,
	} {
		outcome, err := handler.RegisterPayerRequirement(t.Context(), payerRuleCommand(t, procedure, requirement))
		if err != nil || outcome != application.ConfigurationRegistered {
			t.Fatalf("%s：err=%v outcome=%v", procedure, err, outcome)
		}
		loaded, found, err := rules.LoadPayerRequirement(t.Context(),
			configValue(t, domain.NewTenantID, "tenant-a"), configValue(t, domain.NewCustomsProcedureReference, procedure))
		if err != nil || !found || loaded != requirement {
			t.Fatalf("%s 读回 = %v（found=%v err=%v），要 %v", procedure, loaded, found, err, requirement)
		}
	}
	if _, found, _ := rules.LoadPayerRequirement(t.Context(),
		configValue(t, domain.NewTenantID, "tenant-a"), configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-SILENT")); found {
		t.Fatal("没登的程序不该读到任何一格")
	}
}

// 同键同值是重放`已存在`；同键换值是`内容冲突`且册上那一格纹丝不动——改规则走复核另登，不顶替。
func TestReRegisteringAPayerRequirementSplitsReplayFromConflict(t *testing.T) {
	rules := newPayerRuleStore()
	handler := newPayerRuleHandler(rules)
	if _, err := handler.RegisterPayerRequirement(t.Context(), payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequired)); err != nil {
		t.Fatalf("首登：%v", err)
	}

	if outcome, err := handler.RegisterPayerRequirement(t.Context(), payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequired)); err != nil ||
		outcome != application.ConfigurationExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}
	if outcome, err := handler.RegisterPayerRequirement(t.Context(), payerRuleCommand(t, "SYN-PROC-01", domain.PayerNotRequired)); err != nil ||
		outcome != application.ConfigurationContentConflict {
		t.Fatalf("换值该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	if rules.rows["tenant-a|SYN-PROC-01"] != domain.PayerRequired {
		t.Fatalf("冲突顶替了册上那一格：%v", rules.rows["tenant-a|SYN-PROC-01"])
	}
}

// 形状缺格在受理门拒：租户缺席、监管程序零值、规则零值（两格都不是——「未登记」不是一个可登的值）；
// 三格都不落册。
func TestPayerRequirementRegistrationRefusesMissingShape(t *testing.T) {
	rules := newPayerRuleStore()
	handler := newPayerRuleHandler(rules)

	blankTenant := payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequired)
	blankTenant.TenantID = domain.TenantID{}
	zeroProcedure := payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequired)
	zeroProcedure.Procedure = domain.CustomsProcedureReference{}
	zeroRequirement := payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequirementInvalid)

	for name, command := range map[string]application.RegisterPayerRequirementCommand{
		"租户缺席":   blankTenant,
		"监管程序零值": zeroProcedure,
		"规则零值":   zeroRequirement,
	} {
		outcome, err := handler.RegisterPayerRequirement(t.Context(), command)
		if err != nil || outcome != application.ConfigurationNotAccepted {
			t.Fatalf("%s该`未受理`：err=%v outcome=%v", name, err, outcome)
		}
	}
	if len(rules.rows) != 0 {
		t.Fatalf("被拒的登记落了册：%d 行", len(rules.rows))
	}
}

// 写口故障与「已在册却读不回」都折未决：登记与否未知，重跑同一命令续办。
func TestPayerRequirementDependencyFailuresAreUndecided(t *testing.T) {
	writerDown := newPayerRuleStore()
	writerDown.registerErr = errors.New("payer rule registry down")
	if outcome, err := newPayerRuleHandler(writerDown).RegisterPayerRequirement(t.Context(),
		payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequired)); err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("写口故障该未决：err=%v outcome=%v", err, outcome)
	}

	readerDown := newPayerRuleStore()
	handler := newPayerRuleHandler(readerDown)
	if _, err := handler.RegisterPayerRequirement(t.Context(), payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequired)); err != nil {
		t.Fatalf("首登：%v", err)
	}
	readerDown.loadErr = errors.New("payer rule view down")
	if outcome, err := handler.RegisterPayerRequirement(t.Context(),
		payerRuleCommand(t, "SYN-PROC-01", domain.PayerRequired)); err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("已在册却读不回该未决，不猜是重放还是冲突：err=%v outcome=%v", err, outcome)
	}
}
