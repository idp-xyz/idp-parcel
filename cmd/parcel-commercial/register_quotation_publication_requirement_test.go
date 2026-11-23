package main

import "testing"

func TestQuotationPublicationRequirementDocumentRequiresTheBoolean(t *testing.T) {
	if _, err := quotationPublicationRequirementFromJSON([]byte(`{"tenantId":"tenant-1"}`)); err == nil {
		t.Fatal("缺 requires 却译出来了")
	}
	rule, err := quotationPublicationRequirementFromJSON([]byte(`{"tenantId":"tenant-1","requires":false}`))
	if err != nil || rule.Requires() {
		t.Fatalf("rule = %v %v", rule, err)
	}
}

func TestRegisterQuotationPublicationRequirementReplaysAndConflictsLikeSigningEvidence(t *testing.T) {
	dsn := freshMigratedDSN(t)
	requirement := batchFile(t, `{"tenantId":"tenant-1","requires":true}`)
	if code := runCLI(t, dsn, "register-quotation-publication-requirement", "-input", requirement); code != exitLanded {
		t.Fatalf("登记 exit = %d", code)
	}
	if code := runCLI(t, dsn, "register-quotation-publication-requirement", "-input", requirement); code != exitLanded {
		t.Fatalf("重放 exit = %d", code)
	}
	changed := batchFile(t, `{"tenantId":"tenant-1","requires":false}`)
	if code := runCLI(t, dsn, "register-quotation-publication-requirement", "-input", changed); code != exitAttention {
		t.Fatalf("冲突 exit = %d", code)
	}
}
