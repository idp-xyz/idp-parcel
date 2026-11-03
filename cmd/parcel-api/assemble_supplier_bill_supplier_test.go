package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// 本文件对真实 PostgreSQL 证 ADR-0358 在接收口上的生产装配：匹配门核账单的供应商与所点名那一版预期成本记下的供应商。夹具同
// assemble_supplier_bill_decisions_test.go（SYN-PARTY-CARRIER-TRUNK-01 的一版 8500 CNY 预期成本），另直写一版别家的与一版没记
// 供应商的旧行。证据只记 `S`。

// insertCostRow 照预期成本表的样子直写一行；supplier 为空即本记录之前的旧行（这一格为空）。
func insertCostRow(t *testing.T, fixture *settlementDecisionsFixture, version, supplier string) {
	t.Helper()
	var supplierRef any
	if supplier != "" {
		supplierRef = supplier
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := fixture.db.RequireExecutor(ctx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(ctx, `
			INSERT INTO settlement_accounting.supplier_expected_cost
				(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version, occurred_at, fee_item, purchase_rule_version,
				 agreement_ref, evaluation_ref, original_currency, original_minor, settlement_currency, settlement_minor, recorded_at,
				 supplier_party_ref)
			VALUES ($1, $2, 'SYN-TCO-'||$2, 'ACTUAL_FULFILLMENT', 'V1', now() - interval '2 days', 'SYN-FEE-ITEM-TRUNK-COST-01',
			        'SYN-PLAN-CN-SG-COST-01@v1', 'SYN-SUPPLIER-LM-01/v1', 'SYN-EVAL-'||$2, 'CNY', 8500, 'CNY', 8500, now(), $3)`,
			decisionsTenant, version, supplierRef)
		return err
	}); err != nil {
		t.Fatalf("直写预期成本：%v", err)
	}
}

func billNamingCost(claim, version string) string {
	return strings.Replace(strings.Replace(supplierBillBody, `"SYN-BILL-PSB22-1"`, `"`+claim+`"`, 1),
		`"expectedVersion":"SYN-BUYSC-PSB22-1"`, `"expectedVersion":"`+version+`"`, 1)
}

// Covers: AT-SA-236、AT-SA-237（生产装配半边：接收）——TRUNK 的账单点名 LM 的预期成本答 200 EXPECTED_COST_SUPPLIER_MISMATCH、
// 带出行、版本与那一版的供应商；点名没记供应商的旧行答 200 EXPECTED_COST_SUPPLIER_UNRECORDED；两种都不落接收行；点名自己那一版的
// 照常 201。
func TestTheReceptionFaceRefusesAnotherSuppliersExpectedCost(t *testing.T) {
	fixture := newSupplierBillFixture(t)
	seedSupplierBillRegisters(t, fixture, true)
	insertCostRow(t, fixture, "SYN-BUYSC-LM-1", "SYN-PARTY-CARRIER-LM-01")
	insertCostRow(t, fixture, "SYN-BUYSC-OLD-1", "")

	status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, billNamingCost("SYN-BILL-CROSS", "SYN-BUYSC-LM-1"))
	refusal, _ := answer["expectedCostSupplier"].(map[string]any)
	if status != http.StatusOK || answer["outcome"] != "EXPECTED_COST_SUPPLIER_MISMATCH" || refusal == nil ||
		refusal["line"] != "L1" || refusal["version"] != "SYN-BUYSC-LM-1" || refusal["supplier"] != "SYN-PARTY-CARRIER-LM-01" {
		t.Fatalf("点名别家成本 = %d %v", status, answer)
	}
	status, answer = fixture.post(t, supplierBillReceptionFace, billClerk, billNamingCost("SYN-BILL-LEGACY", "SYN-BUYSC-OLD-1"))
	refusal, _ = answer["expectedCostSupplier"].(map[string]any)
	if status != http.StatusOK || answer["outcome"] != "EXPECTED_COST_SUPPLIER_UNRECORDED" || refusal == nil ||
		refusal["version"] != "SYN-BUYSC-OLD-1" || refusal["supplier"] != nil {
		t.Fatalf("点名没记供应商的旧行 = %d %v", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.supplier_bill_reception WHERE tenant_id = $1`, decisionsTenant); count != "0" {
		t.Fatalf("接收行 %s，被拒的账单不该落库", count)
	}
	if status, answer := fixture.post(t, supplierBillReceptionFace, billClerk, supplierBillBody); status != http.StatusCreated ||
		answer["outcome"] != "BILL_RECEIVED" {
		t.Fatalf("点名自己那一版 = %d %v", status, answer)
	}
}
