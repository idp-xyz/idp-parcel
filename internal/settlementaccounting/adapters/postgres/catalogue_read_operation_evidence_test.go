package postgres_test

import "testing"

// Covers: AT-SA-200（读面持久化半边）——资金事实册上列交回链头那一版的操作证据；不记提交方的一版交回空。
func TestExternalFundsFactCatalogueCarriesTheChainHeadOperationEvidence(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-01", "RECEIPT_CONFIRMED", 100000, "V1", "", catalogueBaseAt)
	seedExternalFundsFactVersion(t, fixture, "SYN-FACT-02", "RECEIPT_CONFIRMED", 5000, "V1", "", catalogueBaseAt)
	fixture.seed(t, `UPDATE settlement_accounting.external_funds_fact_version SET operation_evidence = $2
		 WHERE tenant_id = $1 AND fact_id = 'SYN-FACT-01'`, catalogueTenant, "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01")

	rows, err := fixture.funds.ListExternalFundsFacts(t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列资金事实走样：err=%v rows=%+v", err, rows)
	}
	if rows[0].OperationEvidence != "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01" || rows[1].OperationEvidence != "" {
		t.Fatalf("操作证据读回走样：%q / %q", rows[0].OperationEvidence, rows[1].OperationEvidence)
	}
}
