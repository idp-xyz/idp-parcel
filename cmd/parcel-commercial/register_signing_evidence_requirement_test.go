package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSigningEvidenceRequirementDocumentRequiresBothBooleans(t *testing.T) {
	if _, err := signingEvidenceRequirementFromJSON([]byte(`{"tenantId":"tenant-1","requiresCustomerContract":true}`)); err == nil {
		t.Fatal("缺一个布尔却译出来了")
	}
	rule, err := signingEvidenceRequirementFromJSON([]byte(`{"tenantId":"tenant-1","requiresCustomerContract":false,"requiresSupplierAgreement":true}`))
	if err != nil || rule.RequiresCustomerContract() || !rule.RequiresSupplierAgreement() {
		t.Fatalf("rule = %v %v", rule, err)
	}
}

func TestControlledPublishExitsAttentionWhenSigningEvidenceIsRequired(t *testing.T) {
	dsn := freshMigratedDSN(t)
	requirement := batchFile(t, `{"tenantId":"tenant-1","requiresCustomerContract":true,"requiresSupplierAgreement":false}`)
	if code := runCLI(t, dsn, "register-signing-evidence-requirement", "-input", requirement); code != exitLanded {
		t.Fatalf("登记要求 exit = %d", code)
	}
	if code := runCLI(t, dsn, "register-signing-evidence-requirement", "-input", requirement); code != exitLanded {
		t.Fatalf("重放 exit = %d", code)
	}
	changed := batchFile(t, `{"tenantId":"tenant-1","requiresCustomerContract":false,"requiresSupplierAgreement":false}`)
	if code := runCLI(t, dsn, "register-signing-evidence-requirement", "-input", changed); code != exitAttention {
		t.Fatalf("冲突 exit = %d", code)
	}

	batch := batchFile(t, `{"items":[{
      "tenantId": "tenant-1",
      "kind": "CUSTOMER_CONTRACT",
      "objectId": "contract-sign",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "PCC-1:a40b442d890d4d40635f3d230a763000acf3e2ee3839d55cdac83aee5b4480f4",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {"reference": "approval-contract-sign", "source": "source-1", "approvedAt": "2025-12-15T00:00:00Z"},
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {"contractContent": {"customerAccount": "customer-1", "rulePackage": "rules-1"}}
    }]}`)
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"publish", "-input", batch}, func(key string) string {
		if key == envDatabaseDSN {
			return dsn
		}
		return ""
	}, &out, &errOut)
	if code != exitAttention || !strings.Contains(out.String(), "SIGNING_EVIDENCE_REQUIRED") {
		t.Fatalf("缺证据 exit = %d\n%s\n%s", code, out.String(), errOut.String())
	}
	if n := countVersions(t, dsn, "contract-sign"); n != 0 {
		t.Fatalf("缺证据仍入册 %d 行", n)
	}

	// 补上一条证据之后，同一份批文放行。批文看的是册上有没有证据。
	insertEvidence(t, dsn)
	if code := runCLI(t, dsn, "publish", "-input", batch); code != exitLanded {
		t.Fatalf("补登后 exit = %d", code)
	}
	if n := countVersions(t, dsn, "contract-sign"); n != 1 {
		t.Fatalf("补登后版本行 = %d", n)
	}
}

func insertEvidence(t *testing.T, dsn string) {
	t.Helper()
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("开池：%v", err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx,
		`INSERT INTO party_commercial.signing_evidence (
		    tenant_id, evidence_id, object_kind, object_id, version_label, signed_on,
		    file_name, file_sha256, library_index, source, recorded_by, recorded_at)
		 VALUES ('tenant-1', 'evidence-1', 2, 'contract-sign', 'v1', '2026-01-02',
		         'signed.pdf', '`+strings.Repeat("ab", 32)+`', 'EVD-1', 'OFFLINE_SIGNED', 'op-1', '2026-01-02T00:00:00Z')`)
	if err != nil {
		t.Fatalf("插入证据：%v", err)
	}
}
