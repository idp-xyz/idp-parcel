package main

import (
	"strings"
	"testing"
)

// Covers: register-version-closures 的批文翻译——两节逐项各成一条命令，租户取批文整批那一格；空批、缺租户、退役项带
// 时刻（未知键）在连库之前就拒。
func TestVersionClosureBatchTranslatesBothSections(t *testing.T) {
	batch := `{"tenantId": "SYN-TENANT-01",
		"cuts": [{"kind": "CUSTOMER_CONTRACT", "scope": "SYN-SCOPE", "objectId": "SYN-CONTRACT-01", "version": "v1",
			"successorVersion": "v2", "reference": "SYN-CUT-01", "correctedAt": "2026-10-20T00:00:00Z"}],
		"retirements": [{"kind": "SUPPLIER_AGREEMENT", "scope": "SYN-SCOPE", "objectId": "SYN-AGREEMENT-01", "version": "v1",
			"successorVersion": "v2", "reference": "SYN-REISSUE-01"}]}`
	commands, err := versionClosureBatchFromJSON([]byte(batch))
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(commands) != 2 || !strings.HasPrefix(commands[0].label, "截止") || !strings.HasPrefix(commands[1].label, "退役") {
		t.Fatalf("commands = %+v", commands)
	}
	for name, raw := range map[string]string{
		"空批":     `{"tenantId": "SYN-TENANT-01"}`,
		"缺租户":    strings.Replace(batch, `"tenantId": "SYN-TENANT-01",`, "", 1),
		"退役项带时刻": strings.Replace(batch, `"reference": "SYN-REISSUE-01"`, `"reference": "SYN-REISSUE-01", "retiredAt": "2026-10-20T00:00:00Z"`, 1),
	} {
		if _, err := versionClosureBatchFromJSON([]byte(raw)); err == nil {
			t.Fatalf("%s：翻译通过了", name)
		}
	}
}
