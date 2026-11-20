package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 门禁目录里「毛重差异」那一道规则行的登记面：容差与「不构成前置条件」两形各登得进、同值不同写法重放
// `已存在`、换容差或换形`内容冲突`不顶替。替身照真库代数——同键只答`已登记`，冲突与否由应用层读回再比。

type weightRuleRow struct {
	key  string
	rule domain.WeightDiscrepancyGateRule
}

type weightRuleStoreDouble struct {
	rows        []weightRuleRow
	registerErr error
	loadErr     error
}

func (double *weightRuleStoreDouble) RegisterWeightDiscrepancyGateRule(
	_ context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference,
	rule domain.WeightDiscrepancyGateRule,
) (ports.CaseConfigurationSaveOutcome, error) {
	if double.registerErr != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, double.registerErr
	}
	key := gateConfigKey(tenant, scope, action, boundary)
	for _, row := range double.rows {
		if row.key == key {
			return ports.CaseConfigurationAlreadyRegistered, nil
		}
	}
	double.rows = append(double.rows, weightRuleRow{key: key, rule: rule})
	return ports.CaseConfigurationRegistered, nil
}

func (double *weightRuleStoreDouble) LoadWeightDiscrepancyGateRule(
	_ context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference,
) (domain.WeightDiscrepancyGateRule, bool, error) {
	if double.loadErr != nil {
		return domain.WeightDiscrepancyGateRule{}, false, double.loadErr
	}
	key := gateConfigKey(tenant, scope, action, boundary)
	for _, row := range double.rows {
		if row.key == key {
			return row.rule, true, nil
		}
	}
	return domain.WeightDiscrepancyGateRule{}, false, nil
}

func newWeightRuleHandler(rules *weightRuleStoreDouble) *application.RegisterCaseConfigurationHandler {
	gates := newGateConfigStore()
	return application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Gates: gates, GateView: gates,
		WeightRules: rules, WeightRuleView: rules,
	})
}

func weightRuleCommand(t *testing.T, delta, unit string) application.RegisterWeightDiscrepancyGateRuleCommand {
	t.Helper()
	catalog := gateCatalogCommand(t)
	return application.RegisterWeightDiscrepancyGateRuleCommand{
		TenantID: catalog.TenantID,
		Scope:    catalog.Scope,
		Action:   catalog.Action,
		Boundary: catalog.Boundary,
		MaxDelta: delta,
		Unit:     unit,
	}
}

func TestBothWeightDiscrepancyGateRuleShapesRegister(t *testing.T) {
	rules := &weightRuleStoreDouble{}
	handler := newWeightRuleHandler(rules)

	outcome, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "0.50", "KG"))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("容差规则登记：err=%v outcome=%v", err, outcome)
	}
	delta, unit, ok := rules.rows[0].rule.MaxDelta()
	if !ok || delta != "0.5" || unit != "KG" || rules.rows[0].rule.NotAPrecondition() {
		t.Fatalf("容差没有收成唯一拼写：%q %q ok=%v", delta, unit, ok)
	}

	waived := weightRuleCommand(t, "", "")
	waived.Boundary = configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")
	waived.NotAPrecondition = true
	outcome, err = handler.RegisterWeightDiscrepancyGateRule(t.Context(), waived)
	if err != nil || outcome != application.ConfigurationRegistered || !rules.rows[1].rule.NotAPrecondition() {
		t.Fatalf("「不构成前置条件」登记：err=%v outcome=%v rows=%d", err, outcome, len(rules.rows))
	}
}

// 同键同规则重放`已存在`（"0.50" 与 "0.5" 是同一条）；换容差、换单位或换形是`内容冲突`，册面纹丝不动。
func TestReRegisteringAWeightDiscrepancyGateRuleSplitsReplayFromConflict(t *testing.T) {
	rules := &weightRuleStoreDouble{}
	handler := newWeightRuleHandler(rules)
	if outcome, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "0.50", "KG")); err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("首登：err=%v outcome=%v", err, outcome)
	}

	if outcome, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "0.5", "KG")); err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("同值重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}

	if outcome, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "0.80", "KG")); err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换容差该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	if outcome, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "500", "G")); err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换单位该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	waived := weightRuleCommand(t, "", "")
	waived.NotAPrecondition = true
	if outcome, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), waived); err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换形该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	if len(rules.rows) != 1 {
		t.Fatalf("冲突顶掉了在册规则：%d 行", len(rules.rows))
	}
	delta, unit, ok := rules.rows[0].rule.MaxDelta()
	if !ok || delta != "0.5" || unit != "KG" || rules.rows[0].rule.NotAPrecondition() {
		t.Fatalf("在册规则被改写：%q %q ok=%v waived=%v", delta, unit, ok, rules.rows[0].rule.NotAPrecondition())
	}
}

// 受理门——租户或动作缺席、两形同时给、负数容差、非重量单位，都是`未受理`，被拒的不落册。
func TestWeightDiscrepancyGateRuleRegistrationRefusesAnIllFormedCommand(t *testing.T) {
	rules := &weightRuleStoreDouble{}
	handler := newWeightRuleHandler(rules)

	mutations := map[string]func(*application.RegisterWeightDiscrepancyGateRuleCommand){
		"空白租户": func(command *application.RegisterWeightDiscrepancyGateRuleCommand) {
			command.TenantID = domain.TenantID{}
		},
		"无效动作": func(command *application.RegisterWeightDiscrepancyGateRuleCommand) {
			command.Action = domain.GuardedActionInvalid
		},
		"两形同时给": func(command *application.RegisterWeightDiscrepancyGateRuleCommand) {
			command.NotAPrecondition = true
		},
		"负数容差": func(command *application.RegisterWeightDiscrepancyGateRuleCommand) {
			command.MaxDelta = "-0.5"
		},
		"非重量单位": func(command *application.RegisterWeightDiscrepancyGateRuleCommand) {
			command.Unit = "CM"
		},
	}
	for name, mutate := range mutations {
		command := weightRuleCommand(t, "0.50", "KG")
		mutate(&command)
		outcome, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), command)
		if err != nil || outcome != application.ConfigurationNotAccepted {
			t.Fatalf("%s该`未受理`：err=%v outcome=%v", name, err, outcome)
		}
	}
	if len(rules.rows) != 0 {
		t.Fatal("被拒的规则落了册")
	}
}

// 依赖故障折未决——写口故障；已在册但读不回。
func TestWeightDiscrepancyGateRuleDependencyFailuresAreUndecided(t *testing.T) {
	writerDown := &weightRuleStoreDouble{registerErr: errors.New("writer unavailable")}
	outcome, err := newWeightRuleHandler(writerDown).RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "0.50", "KG"))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("写口故障该未决：err=%v outcome=%v", err, outcome)
	}

	viewDown := &weightRuleStoreDouble{}
	handler := newWeightRuleHandler(viewDown)
	if _, err := handler.RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "0.50", "KG")); err != nil {
		t.Fatalf("首登：%v", err)
	}
	viewDown.loadErr = errors.New("view unavailable")
	outcome, err = handler.RegisterWeightDiscrepancyGateRule(t.Context(), weightRuleCommand(t, "0.50", "KG"))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("已在册但读不回该未决：err=%v outcome=%v", err, outcome)
	}
}
