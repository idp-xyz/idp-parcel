package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证出发门禁阻断信号规则（0027）：登记之前答未配置、登记之后三样原样读回、一租户一条不可覆盖、
// 另一租户不可见、空白列由库拒下。

func departureGateRuleView(t *testing.T, fixture *registrarFixture, tenant domain.TenantID) *adapter.DepartureGateSignalRules {
	t.Helper()
	view, err := adapter.NewDepartureGateSignalRules(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造出发门禁信号规则视图：%v", err)
	}
	return view
}

func departureGateRuleRegistration(t *testing.T, kind, rule, confidence string) ports.DepartureGateSignalRuleRegistration {
	t.Helper()
	return ports.DepartureGateSignalRuleRegistration{
		Kind:       build(t, domain.NewExceptionSignalKindReference, kind),
		Rule:       build(t, domain.NewSignalRuleVersionReference, rule),
		Confidence: build(t, domain.NewConfidenceReference, confidence),
		ApprovedBy: "exception-ops",
	}
}

// Covers: AT-TF-158 的规则册半边——登记之前未配置（阻断那一件不形成信号），登记之后类型、规则版本与可信度原样读回；再登交回
// AlreadyRegistered 且原行不动；别的租户与空租户都看不见。
func TestRegisteredDepartureGateSignalRuleBecomesReadableAndIsNotOverwritten(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view := departureGateRuleView(t, fixture, tenant)

	if _, configured, err := view.DepartureGateSignalRule(t.Context()); err != nil || configured {
		t.Fatalf("登记之前必须答未配置：err=%v configured=%v", err, configured)
	}

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterDepartureGateSignalRule(txCtx, tenant,
			departureGateRuleRegistration(t, "DEPARTURE_GATE_BLOCKED", "departure-gate-signal/v1", "SOURCE_CONFIRMED"))
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记应成功，实得 %s", outcome)
	}
	rule, configured, err := view.DepartureGateSignalRule(t.Context())
	if err != nil || !configured {
		t.Fatalf("登记之后应答已配置：err=%v configured=%v", err, configured)
	}
	if rule.Kind.String() != "DEPARTURE_GATE_BLOCKED" || rule.Rule.String() != "departure-gate-signal/v1" ||
		rule.Confidence.String() != "SOURCE_CONFIRMED" {
		t.Fatalf("规则读回走样：%+v", rule)
	}

	again := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterDepartureGateSignalRule(txCtx, tenant,
			departureGateRuleRegistration(t, "DEPARTURE_GATE_BREACH", "departure-gate-signal/v2", "SOURCE_CONFIRMED"))
	})
	if again != ports.CatalogVersionAlreadyRegistered {
		t.Fatalf("再登应交回 AlreadyRegistered，实得 %s", again)
	}
	if unchanged, _, err := view.DepartureGateSignalRule(t.Context()); err != nil ||
		unchanged.Kind.String() != "DEPARTURE_GATE_BLOCKED" || unchanged.Rule.String() != "departure-gate-signal/v1" {
		t.Fatalf("再登覆盖了原行：%+v err=%v", unchanged, err)
	}

	if _, configured, err := departureGateRuleView(t, fixture, build(t, domain.NewTenantID, "tenant-b")).
		DepartureGateSignalRule(t.Context()); err != nil || configured {
		t.Fatalf("跨租户可见：err=%v configured=%v", err, configured)
	}
	if _, configured, err := departureGateRuleView(t, fixture, domain.TenantID{}).DepartureGateSignalRule(t.Context()); err != nil || configured {
		t.Fatalf("空租户视图：err=%v configured=%v", err, configured)
	}
}

// Covers: 0027 的 not_blank——三样缺一都是漏填，库拒下；读口不会读到半条规则。
func TestDepartureGateSignalRuleRowsRejectBlankColumns(t *testing.T) {
	fixture := newRegistrarFixture(t)
	for name, values := range map[string]string{
		"空类型":   `'tenant-a', '  ', 'departure-gate-signal/v1', 'SOURCE_CONFIRMED', 'ops'`,
		"空规则版本": `'tenant-a', 'DEPARTURE_GATE_BLOCKED', '', 'SOURCE_CONFIRMED', 'ops'`,
		"空可信度":  `'tenant-a', 'DEPARTURE_GATE_BLOCKED', 'departure-gate-signal/v1', ' ', 'ops'`,
		"空批准人":  `'tenant-a', 'DEPARTURE_GATE_BLOCKED', 'departure-gate-signal/v1', 'SOURCE_CONFIRMED', ''`,
	} {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.departure_gate_signal_rule
				(tenant_id, signal_kind, rule_version, confidence_ref, approved_by)
			 VALUES (`+values+`)`); err == nil {
			t.Fatalf("%s 的规则行被库接受了", name)
		}
	}
}
