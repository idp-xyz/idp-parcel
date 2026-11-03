package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证补记口（ADR-0358 决定三，AT-SA-237；隔离合成 S）：本记录之前形成、没记供应商的预期成本，经
// 补记口按各自的协议问 party-commercial 的真读口补上；PC 不认的协议留空并列出、退出码 2，其余照写；只动点名的租户；重跑不改
// 已记的。party-commercial 那两行协议是夹具直写的——这里证的是补记口，不是协议的发布。

// Covers: AT-SA-237（生产装配半边：补记口）。
func TestTheBackfillRecordsLegacyCostSuppliersFromTheirAgreementsOnRealPostgres(t *testing.T) {
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
	for _, agreement := range []struct{ tenant, supplier string }{{"SYN-T1", "SYN-PARTY-TRUNK"}, {"SYN-T2", "SYN-PARTY-OTHER"}} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO party_commercial.commercial_version
				(tenant_id, object_kind, object_id, version_label, scope_ref, content_digest, effective_starts_at, status, snapshot, published_at)
			VALUES ($1, 3, 'SYN-SUPPLIER-TRUNK', 'v1', 'SYN-SCOPE-01', 'PCC-1:syn', '2026-01-01T00:00:00Z', 3, '{}', '2025-12-15T00:00:00Z')`,
			agreement.tenant); err != nil {
			t.Fatalf("落协议版本夹具：%v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO party_commercial.supplier_agreement
				(tenant_id, object_kind, object_id, version_label, supplier_party_id, legal_entity_ref, agreement_scope_ref,
				 purchase_plan_ref, effective_starts_at)
			VALUES ($1, 3, 'SYN-SUPPLIER-TRUNK', 'v1', $2, 'SYN-LE-01', 'SYN-SCOPE-01', 'SYN-PLAN-01', '2026-01-01T00:00:00Z')`,
			agreement.tenant, agreement.supplier); err != nil {
			t.Fatalf("落协议夹具：%v", err)
		}
	}
	for _, cost := range []struct{ tenant, version, agreement string }{
		{"SYN-T1", "SYN-COST-A", "SYN-SUPPLIER-TRUNK/v1"},
		{"SYN-T1", "SYN-COST-B", "SYN-SUPPLIER-GONE/v1"},
		{"SYN-T2", "SYN-COST-A", "SYN-SUPPLIER-TRUNK/v1"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO settlement_accounting.supplier_expected_cost
				(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version, occurred_at, fee_item, purchase_rule_version,
				 agreement_ref, evaluation_ref, original_currency, original_minor, settlement_currency, settlement_minor, recorded_at)
			VALUES ($1, $2, 'SYN-OCC-'||$2, 'ACTUAL_FULFILLMENT', 'V1', now(), 'SYN-FEE-1', 'SYN-CARD/v1', $3, 'SYN-EVAL-'||$2,
			        'CNY', 8500, 'CNY', 8500, now())`,
			cost.tenant, cost.version, cost.agreement); err != nil {
			t.Fatalf("落旧行：%v", err)
		}
	}
	supplierOf := func(tenant, version string) string {
		t.Helper()
		var supplier *string
		if err := pool.QueryRow(ctx, `SELECT supplier_party_ref FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1 AND version = $2`,
			tenant, version).Scan(&supplier); err != nil {
			t.Fatal(err)
		}
		if supplier == nil {
			return ""
		}
		return *supplier
	}

	message, code := execute(ctx, commandExpectedCostSupplier, []byte(`{"tenantId": "SYN-T1"}`), registrar)
	if code != exitConflict || !strings.Contains(message, "补记 SYN-COST-A → SYN-PARTY-TRUNK") ||
		!strings.Contains(message, "补不上 SYN-COST-B（协议 SYN-SUPPLIER-GONE/v1）：AGREEMENT_NOT_REGISTERED") {
		t.Fatalf("补记 = %d\n%s", code, message)
	}
	if supplierOf("SYN-T1", "SYN-COST-A") != "SYN-PARTY-TRUNK" || supplierOf("SYN-T1", "SYN-COST-B") != "" {
		t.Fatal("补得上的没写下，或补不上的被填了值")
	}
	if supplierOf("SYN-T2", "SYN-COST-A") != "" {
		t.Fatal("补记 SYN-T1 动了 SYN-T2 的旧行")
	}

	again, code := execute(ctx, commandExpectedCostSupplier, []byte(`{"tenantId": "SYN-T1"}`), registrar)
	if code != exitConflict || !strings.Contains(again, "补记 0 份") || strings.Contains(again, "SYN-COST-A") {
		t.Fatalf("重跑 = %d\n%s", code, again)
	}
	other, code := execute(ctx, commandExpectedCostSupplier, []byte(`{"tenantId": "SYN-T2"}`), registrar)
	if code != exitRegistered || supplierOf("SYN-T2", "SYN-COST-A") != "SYN-PARTY-OTHER" || supplierOf("SYN-T1", "SYN-COST-A") != "SYN-PARTY-TRUNK" {
		t.Fatalf("SYN-T2 补记 = %d\n%s", code, other)
	}
	if done, code := execute(ctx, commandExpectedCostSupplier, []byte(`{"tenantId": "SYN-T2"}`), registrar); code != exitRegistered ||
		!strings.Contains(done, "没有待补记的预期成本") {
		t.Fatalf("补完再跑 = %d\n%s", code, done)
	}

	for name, raw := range map[string]string{
		"没有租户": `{"tenantId": ""}`,
		"多给一格": `{"tenantId": "SYN-T1", "supplier": "SYN-PARTY-TRUNK"}`,
	} {
		if _, code := execute(ctx, commandExpectedCostSupplier, []byte(raw), registrar); code != exitUsage {
			t.Fatalf("%s：退出码 %d，want 1", name, code)
		}
	}
}
