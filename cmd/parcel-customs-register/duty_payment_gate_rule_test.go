package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

type droppedHandoff struct{}

func (droppedHandoff) HandOffGate(context.Context, ports.GateVerificationHandoffIntent) error {
	return nil
}

// 演示种子里「税费付款」那一道的规则行、门禁目录与逐项认定照原样灌进真库（证据 S）：登记前放行门禁核对答
// `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`，登记后答出 MET 并落册；规则同键同值重放`已存在`，换规则是`内容冲突`
// （改规则走复核，不顶替）。规则取值是演示采用的，不是产品默认。
// Covers: UC-CC-009 门禁规则行的登记面半边（票 launch-wiring/06）——读的是 scripts/demo-seeds 里的文件本身。
func TestDemoDutyPaymentGateRuleSeedLetsTheReleaseGateBeVerified(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()
	seed := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "customs", name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		return raw
	}

	gates, err := adapter.NewGateConditionView(db)
	if err != nil {
		t.Fatal(err)
	}
	duty, err := adapter.NewDutyPaymentReconciliation(db)
	if err != nil {
		t.Fatal(err)
	}
	verifications, err := adapter.NewGateVerifications(db)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := application.NewVerifyReleaseGateHandler(application.VerifyReleaseGateDeps{
		Conditions: gates, DutyRules: gates, DutyVerifications: duty,
		Store: verifications, Downstream: droppedHandoff{}, Clock: nodeZoneClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	scope, _ := domain.NewDecisionScopeReference("SYN-SCOPE-01")
	boundary, _ := domain.NewCustomsProcedureReference("SYN-PROC-CN-EXPORT")
	verify := func() application.VerifyReleaseGateResult {
		t.Helper()
		var result application.VerifyReleaseGateResult
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = verifier.Handle(txCtx, application.VerifyReleaseGateCommand{
				TenantID: tenant, Scope: scope, Action: domain.CrossCustomsMovement, Boundary: boundary,
			})
			return handleErr
		})
		if err != nil {
			t.Fatalf("门禁核对：%v", err)
		}
		return result
	}

	for _, name := range []string{"08-gate-catalog.json", "09-gate-finding-met.json"} {
		command := commandGateCatalog
		if name == "09-gate-finding-met.json" {
			command = commandGateFinding
		}
		if message, code := execute(ctx, command, seed(name), registrar); code != exitRegistered {
			t.Fatalf("%s：%s（%d）", name, message, code)
		}
	}
	if result := verify(); result.Outcome() != application.GateVerificationUndecided ||
		result.UndecidedReason() != application.DutyPaymentGateRuleNotConfigured {
		t.Fatalf("规则未登 = %v / %v，要 DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED", result.Outcome(), result.UndecidedReason())
	}

	rule := seed("32-duty-payment-gate-rule-not-a-precondition.json")
	for _, want := range []string{"REGISTERED", "EXISTING"} {
		message, code := execute(ctx, commandDutyPaymentGateRule, rule, registrar)
		if code != exitRegistered || len(message) < len(want) || message[len(message)-len(want):] != want {
			t.Fatalf("规则种子：%s（%d），要 %s", message, code, want)
		}
	}
	other := []byte(`{"tenantId": "SYN-TENANT-01", "scopeRef": "SYN-SCOPE-01", "action": "CROSS_CUSTOMS_MOVEMENT",
		"boundaryRef": "SYN-PROC-CN-EXPORT", "acceptCoverage": ["COVERED"], "acceptDelta": ["NO_DELTA"], "acceptValidity": ["VALID"]}`)
	if message, code := execute(ctx, commandDutyPaymentGateRule, other, registrar); code != exitConflict {
		t.Fatalf("换规则 = %s（%d），要内容冲突", message, code)
	}
	// 两形混写、集合带待确认，编排拒收，不进库。
	mixed := []byte(`{"tenantId": "SYN-TENANT-01", "scopeRef": "SYN-SCOPE-02", "action": "CROSS_CUSTOMS_MOVEMENT",
		"boundaryRef": "SYN-PROC-CN-EXPORT", "notAPrecondition": true, "acceptCoverage": ["COVERED"]}`)
	if message, code := execute(ctx, commandDutyPaymentGateRule, mixed, registrar); code == exitRegistered {
		t.Fatalf("两形混写不该登记：%s", message)
	}

	result := verify()
	if result.Outcome() != application.GateVerificationRecorded {
		t.Fatalf("规则登后 = %v / %v，要 RECORDED", result.Outcome(), result.UndecidedReason())
	}
	gate, ok := result.Gate()
	if !ok || gate.Conclusion() != domain.GateMet {
		t.Fatalf("结论 = %v，要 MET", gate.Conclusion())
	}
}
