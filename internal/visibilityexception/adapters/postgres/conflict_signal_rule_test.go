package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证冲突信号规则（0025）：登记之前答未配置、登记之后三样原样
// 读回、一租户一条不可覆盖、另一租户不可见、空白列由库拒下。

func conflictRuleView(t *testing.T, fixture *registrarFixture, tenant domain.TenantID) *adapter.ConflictSignalRules {
	t.Helper()
	view, err := adapter.NewConflictSignalRules(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造冲突信号规则视图：%v", err)
	}
	return view
}

func conflictRuleRegistration(t *testing.T, kind, rule, confidence string) ports.ConflictSignalRuleRegistration {
	t.Helper()
	return ports.ConflictSignalRuleRegistration{
		Kind:       build(t, domain.NewExceptionSignalKindReference, kind),
		Rule:       build(t, domain.NewSignalRuleVersionReference, rule),
		Confidence: build(t, domain.NewConfidenceReference, confidence),
		ApprovedBy: "exception-ops",
	}
}

// Covers: 冲突信号规则（`PAR-VIS-04`）的写入方与读口：登记之前未配置（编排据此记「无适用
// 信号规则」而不虚构类型），登记之后类型、规则版本与可信度原样读回；再登交回 AlreadyRegistered
// 且原行不动——换版是治理动作不是覆盖。
func TestRegisteredConflictSignalRuleBecomesReadableAndIsNotOverwritten(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view := conflictRuleView(t, fixture, tenant)

	if _, configured, err := view.ConflictSignalRule(t.Context()); err != nil || configured {
		t.Fatalf("登记之前必须答未配置：err=%v configured=%v", err, configured)
	}

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterConflictSignalRule(txCtx, tenant,
			conflictRuleRegistration(t, "FACT_CONFLICT_UNRESOLVED", "signal-rules/v3", "SOURCE_FORK"))
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记冲突信号规则应成功，实得 %s", outcome)
	}

	rule, configured, err := view.ConflictSignalRule(t.Context())
	if err != nil || !configured {
		t.Fatalf("登记之后应答已配置：err=%v configured=%v", err, configured)
	}
	if rule.Kind.String() != "FACT_CONFLICT_UNRESOLVED" || rule.Rule.String() != "signal-rules/v3" ||
		rule.Confidence.String() != "SOURCE_FORK" {
		t.Fatalf("规则读回走样：%+v", rule)
	}

	again := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterConflictSignalRule(txCtx, tenant,
			conflictRuleRegistration(t, "FACT_CONFLICT_ESCALATED", "signal-rules/v4", "SOURCE_FORK"))
	})
	if again != ports.CatalogVersionAlreadyRegistered {
		t.Fatalf("再登应交回 AlreadyRegistered，实得 %s", again)
	}
	unchanged, _, err := view.ConflictSignalRule(t.Context())
	if err != nil || unchanged.Kind.String() != "FACT_CONFLICT_UNRESOLVED" || unchanged.Rule.String() != "signal-rules/v3" {
		t.Fatalf("再登覆盖了原行：%+v err=%v", unchanged, err)
	}

	if _, configured, err := conflictRuleView(t, fixture, build(t, domain.NewTenantID, "tenant-b")).
		ConflictSignalRule(t.Context()); err != nil || configured {
		t.Fatalf("跨租户可见：err=%v configured=%v", err, configured)
	}
	// 零值租户的视图永远答未配置——多租户进程里绑空租户看起来接了库、永远读不到行。
	if _, configured, err := conflictRuleView(t, fixture, domain.TenantID{}).ConflictSignalRule(t.Context()); err != nil || configured {
		t.Fatalf("空租户视图：err=%v configured=%v", err, configured)
	}
}

// Covers: 0025 的 not_blank——三样缺一都是漏填，库拒下；读口不会读到半条规则。
func TestConflictSignalRuleRowsRejectBlankColumns(t *testing.T) {
	fixture := newRegistrarFixture(t)
	for name, values := range map[string]string{
		"空类型":   `'tenant-a', '  ', 'signal-rules/v3', 'SOURCE_FORK', 'ops'`,
		"空规则版本": `'tenant-a', 'FACT_CONFLICT', '', 'SOURCE_FORK', 'ops'`,
		"空可信度":  `'tenant-a', 'FACT_CONFLICT', 'signal-rules/v3', ' ', 'ops'`,
		"空批准人":  `'tenant-a', 'FACT_CONFLICT', 'signal-rules/v3', 'SOURCE_FORK', ''`,
	} {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.conflict_signal_rule
				(tenant_id, signal_kind, rule_version, confidence_ref, approved_by)
			 VALUES (`+values+`)`); err == nil {
			t.Fatalf("%s 的规则行被库接受了", name)
		}
	}
}
