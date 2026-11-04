package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func queryOne(t *testing.T, dsn, sql string) string {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("连接测试库：%v", err)
	}
	defer pool.Close()
	var value string
	if err := pool.QueryRow(t.Context(), sql).Scan(&value); err != nil {
		t.Fatalf("查询：%v", err)
	}
	return value
}

// Covers: register-approval-duty-rule 的文件翻译——须否不同主体是必填、等级可缺、缺租户与未知键在连库之前就拒。
func TestApprovalDutyRuleDocumentTranslates(t *testing.T) {
	rule, err := approvalDutyRuleFromJSON([]byte(`{"tenantId": "SYN-TENANT-01", "requiresDistinctSubjects": false}`))
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if fmt.Sprint(rule.Tenant()) != "SYN-TENANT-01" || rule.RequiresDistinctSubjects() {
		t.Fatalf("rule = %+v", rule)
	}
	if _, required := rule.RequiredApproverLevel(); required {
		t.Fatalf("没给等级却要求等级")
	}
	withLevel, err := approvalDutyRuleFromJSON([]byte(`{"tenantId": "SYN-TENANT-01", "requiresDistinctSubjects": true, "requiredApproverLevel": "SYN-LEVEL-APPROVER"}`))
	if err != nil {
		t.Fatalf("translate with level: %v", err)
	}
	if level, required := withLevel.RequiredApproverLevel(); !required || fmt.Sprint(level) != "SYN-LEVEL-APPROVER" || !withLevel.RequiresDistinctSubjects() {
		t.Fatalf("rule with level = %+v", withLevel)
	}
	for name, raw := range map[string]string{
		"缺须否不同主体": `{"tenantId": "SYN-TENANT-01"}`,
		"缺租户":     `{"requiresDistinctSubjects": true}`,
		"未知键":     `{"tenantId": "SYN-TENANT-01", "requiresDistinctSubjects": true, "defaultApprover": "x"}`,
		"空等级串":    `{"tenantId": "SYN-TENANT-01", "requiresDistinctSubjects": true, "requiredApproverLevel": "  "}`,
	} {
		if _, err := approvalDutyRuleFromJSON([]byte(raw)); err == nil {
			t.Fatalf("%s：翻译通过了", name)
		}
	}
}

// Covers: 对库登记——首次落册退 0，同内容重放退 0，异内容撞键退 2 且册上仍是第一条。
func TestRegisteringTheApprovalDutyRuleIsIdempotentAndNeverOverwrites(t *testing.T) {
	dsn := freshMigratedDSN(t)
	first := batchFile(t, `{"tenantId": "SYN-TENANT-01", "requiresDistinctSubjects": false}`)
	if code := runCLI(t, dsn, "register-approval-duty-rule", "-input", first); code != exitLanded {
		t.Fatalf("首次登记退出码 = %d", code)
	}
	if code := runCLI(t, dsn, "register-approval-duty-rule", "-input", first); code != exitLanded {
		t.Fatalf("同内容重放退出码 = %d", code)
	}
	changed := batchFile(t, `{"tenantId": "SYN-TENANT-01", "requiresDistinctSubjects": true}`)
	if code := runCLI(t, dsn, "register-approval-duty-rule", "-input", changed); code != exitAttention {
		t.Fatalf("异内容撞键退出码 = %d", code)
	}
	if strings.TrimSpace(queryOne(t, dsn, "SELECT requires_distinct_subjects::text FROM party_commercial.publication_approval_duty_rule WHERE tenant_id = 'SYN-TENANT-01'")) != "false" {
		t.Fatalf("撞键覆盖了第一条规则")
	}
}
