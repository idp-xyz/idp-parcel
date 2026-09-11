package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 门禁目录里「税费付款」那一道规则行的登记面（票 sa-cc/06，ADR-0137 决定三）：两形各登得进、重放
// `已存在`、换规则`内容冲突`不顶替、两形互斥与集外拒、这一道不再收结论性认定。替身照真库代数——
// 同键只答`已登记`。

type dutyRuleRow struct {
	key  string
	rule domain.DutyPaymentGateRule
}

type dutyRuleStoreDouble struct {
	rows        []dutyRuleRow
	registerErr error
	loadErr     error
}

func (double *dutyRuleStoreDouble) RegisterDutyPaymentGateRule(
	_ context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference,
	rule domain.DutyPaymentGateRule,
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
	double.rows = append(double.rows, dutyRuleRow{key: key, rule: rule})
	return ports.CaseConfigurationRegistered, nil
}

func (double *dutyRuleStoreDouble) LoadDutyPaymentGateRule(
	_ context.Context, tenant domain.TenantID, scope domain.DecisionScopeReference,
	action domain.GuardedAction, boundary domain.CustomsProcedureReference,
) (domain.DutyPaymentGateRule, bool, error) {
	if double.loadErr != nil {
		return domain.DutyPaymentGateRule{}, false, double.loadErr
	}
	key := gateConfigKey(tenant, scope, action, boundary)
	for _, row := range double.rows {
		if row.key == key {
			return row.rule, true, nil
		}
	}
	return domain.DutyPaymentGateRule{}, false, nil
}

func newDutyRuleHandler(gates *gateConfigStoreDouble, rules *dutyRuleStoreDouble) *application.RegisterCaseConfigurationHandler {
	return application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Gates: gates, GateView: gates,
		DutyRules: rules, DutyRuleView: rules,
	})
}

func acceptRuleCommand(t *testing.T) application.RegisterDutyPaymentGateRuleCommand {
	t.Helper()
	catalog := gateCatalogCommand(t)
	return application.RegisterDutyPaymentGateRuleCommand{
		TenantID:       catalog.TenantID,
		Scope:          catalog.Scope,
		Action:         catalog.Action,
		Boundary:       catalog.Boundary,
		AcceptCoverage: []domain.DutyCoverage{domain.CoverageFull},
		AcceptDelta:    []domain.DutyDelta{domain.DeltaNone, domain.DeltaExcess},
		AcceptValidity: []domain.DutyFactValidity{domain.FundsFactValid},
	}
}

// Covers: 两形各登得进；接受集合以去重排序后的形落册（同一条规则不论怎么写只有一种形）。
func TestBothDutyPaymentGateRuleShapesRegister(t *testing.T) {
	rules := &dutyRuleStoreDouble{}
	handler := newDutyRuleHandler(newGateConfigStore(), rules)

	shuffled := acceptRuleCommand(t)
	shuffled.AcceptDelta = []domain.DutyDelta{domain.DeltaExcess, domain.DeltaNone, domain.DeltaExcess}
	outcome, err := handler.RegisterDutyPaymentGateRule(t.Context(), shuffled)
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("接受集合规则登记：err=%v outcome=%v", err, outcome)
	}
	_, delta, _ := rules.rows[0].rule.Accepts()
	if len(delta) != 2 || delta[0] != domain.DeltaNone || delta[1] != domain.DeltaExcess {
		t.Fatalf("接受集合没有去重排序：%v", delta)
	}

	waived := acceptRuleCommand(t)
	waived.Boundary = configValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-EXPORT")
	waived.NotAPrecondition = true
	waived.AcceptCoverage, waived.AcceptDelta, waived.AcceptValidity = nil, nil, nil
	outcome, err = handler.RegisterDutyPaymentGateRule(t.Context(), waived)
	if err != nil || outcome != application.ConfigurationRegistered || !rules.rows[1].rule.NotAPrecondition() {
		t.Fatalf("「不构成前置条件」登记：err=%v outcome=%v rows=%+v", err, outcome, rules.rows)
	}
}

// Covers: 同键同规则重放`已存在`（写法不同也算同一条）；同键换规则`内容冲突`，册面纹丝不动——改规则
// 走复核，已按旧规则折出的门禁记录引用的是那条规则说过的话。
func TestReRegisteringADutyPaymentGateRuleSplitsReplayFromConflict(t *testing.T) {
	rules := &dutyRuleStoreDouble{}
	handler := newDutyRuleHandler(newGateConfigStore(), rules)
	if _, err := handler.RegisterDutyPaymentGateRule(t.Context(), acceptRuleCommand(t)); err != nil {
		t.Fatalf("首登：%v", err)
	}

	replay := acceptRuleCommand(t)
	replay.AcceptDelta = []domain.DutyDelta{domain.DeltaExcess, domain.DeltaNone}
	if outcome, err := handler.RegisterDutyPaymentGateRule(t.Context(), replay); err != nil || outcome != application.ConfigurationExisting {
		t.Fatalf("重放该是`已存在`：err=%v outcome=%v", err, outcome)
	}

	narrowed := acceptRuleCommand(t)
	narrowed.AcceptDelta = []domain.DutyDelta{domain.DeltaNone}
	if outcome, err := handler.RegisterDutyPaymentGateRule(t.Context(), narrowed); err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换接受集合该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	waived := acceptRuleCommand(t)
	waived.NotAPrecondition = true
	waived.AcceptCoverage, waived.AcceptDelta, waived.AcceptValidity = nil, nil, nil
	if outcome, err := handler.RegisterDutyPaymentGateRule(t.Context(), waived); err != nil || outcome != application.ConfigurationContentConflict {
		t.Fatalf("换形该是`内容冲突`：err=%v outcome=%v", err, outcome)
	}
	if len(rules.rows) != 1 {
		t.Fatalf("冲突顶掉了在册规则：%+v", rules.rows)
	}
	if _, delta, _ := rules.rows[0].rule.Accepts(); len(delta) != 2 {
		t.Fatalf("在册规则被改写：%v", delta)
	}
}

// Covers: 受理门——租户 / 动作 / 范围 / 边界缺席拒；两形互斥（说不构成前置又带接受集合）拒；空集合、
// `待确认` / `冲突` 登为接受、集外取值由领域拒并翻成`未受理`；被拒的都不落册。
func TestDutyPaymentGateRuleRegistrationRefusesWhatTheDomainRefuses(t *testing.T) {
	rules := &dutyRuleStoreDouble{}
	handler := newDutyRuleHandler(newGateConfigStore(), rules)

	mutations := map[string]func(*application.RegisterDutyPaymentGateRuleCommand){
		"租户":     func(c *application.RegisterDutyPaymentGateRuleCommand) { c.TenantID = domain.TenantID{} },
		"动作":     func(c *application.RegisterDutyPaymentGateRuleCommand) { c.Action = domain.GuardedActionInvalid },
		"两形互斥":   func(c *application.RegisterDutyPaymentGateRuleCommand) { c.NotAPrecondition = true },
		"覆盖集合为空": func(c *application.RegisterDutyPaymentGateRuleCommand) { c.AcceptCoverage = nil },
		"差额待确认": func(c *application.RegisterDutyPaymentGateRuleCommand) {
			c.AcceptDelta = []domain.DutyDelta{domain.DeltaPending}
		},
		"有效性冲突": func(c *application.RegisterDutyPaymentGateRuleCommand) {
			c.AcceptValidity = []domain.DutyFactValidity{domain.FundsFactConflicting}
		},
		"覆盖集外": func(c *application.RegisterDutyPaymentGateRuleCommand) {
			c.AcceptCoverage = []domain.DutyCoverage{domain.DutyCoverage(7)}
		},
	}
	for name, mutate := range mutations {
		command := acceptRuleCommand(t)
		mutate(&command)
		outcome, err := handler.RegisterDutyPaymentGateRule(t.Context(), command)
		if err != nil || outcome != application.ConfigurationNotAccepted {
			t.Fatalf("%s该`未受理`：err=%v outcome=%v", name, err, outcome)
		}
	}
	if len(rules.rows) != 0 {
		t.Fatal("被拒的规则落了册")
	}
}

// Covers: 依赖故障折未决——写口故障；已在册但读不回。
func TestDutyPaymentGateRuleDependencyFailuresAreUndecided(t *testing.T) {
	writerDown := &dutyRuleStoreDouble{registerErr: errors.New("writer unavailable")}
	outcome, err := newDutyRuleHandler(newGateConfigStore(), writerDown).RegisterDutyPaymentGateRule(t.Context(), acceptRuleCommand(t))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("写口故障该未决：err=%v outcome=%v", err, outcome)
	}

	viewDown := &dutyRuleStoreDouble{}
	handler := newDutyRuleHandler(newGateConfigStore(), viewDown)
	if _, err := handler.RegisterDutyPaymentGateRule(t.Context(), acceptRuleCommand(t)); err != nil {
		t.Fatalf("首登：%v", err)
	}
	viewDown.loadErr = errors.New("view unavailable")
	outcome, err = handler.RegisterDutyPaymentGateRule(t.Context(), acceptRuleCommand(t))
	if err != nil || outcome != application.ConfigurationUndecided {
		t.Fatalf("已在册但读不回该未决：err=%v outcome=%v", err, outcome)
	}
}

// Covers: 「税费付款」那一道不再收结论性认定（ADR-0137 决定三：这一道的目录行登规则不登认定）——
// 以那一道的引用登认定被受理门拒，其余前置条件的认定照旧登得进。
func TestTheDutyPaymentGateNoLongerAcceptsARegisteredFinding(t *testing.T) {
	gates := newGateConfigStore()
	handler := newDutyRuleHandler(gates, &dutyRuleStoreDouble{})
	if _, err := handler.RegisterGateCatalog(t.Context(), gateCatalogCommand(t)); err != nil {
		t.Fatalf("登记门禁目录：%v", err)
	}

	dutyFinding := gateFindingCommand(t, domain.PreconditionMet)
	dutyFinding.Finding.Precondition = domain.DutyPaymentPrecondition
	outcome, err := handler.RegisterGateFinding(t.Context(), dutyFinding)
	if err != nil || outcome != application.ConfigurationNotAccepted {
		t.Fatalf("税费付款那一道的认定该`未受理`：err=%v outcome=%v", err, outcome)
	}
	outcome, err = handler.RegisterGateFinding(t.Context(), gateFindingCommand(t, domain.PreconditionMet))
	if err != nil || outcome != application.ConfigurationRegistered {
		t.Fatalf("其余前置条件的认定照旧：err=%v outcome=%v", err, outcome)
	}
}
