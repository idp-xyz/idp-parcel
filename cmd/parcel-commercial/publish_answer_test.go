package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// 本文件钉受控 CLI 对同一份批文的答复（票 operator-channel/18 完成判据一）：批文译装从本包下沉到 party-commercial 的
// registrationjson 之后，同一份批文从 run 进去，回显、诊断与退出码逐字不变。期望串取自下沉之前的检出，下沉那一笔
// 不改本文件。
//
// 批文用错 JSON 类型（如 "kind": 5）那一类不进表：解码器的诊断里带 Go 类型名，外壳类型导出之后那个名字随之变，
// 退出码与「技术失败、一个字节没写」不变（票面判断项记了前后两种措辞）。

// publishAnswer 是 publish 子命令对一份批文的整份答复：退出码与两路输出。
func publishAnswer(t *testing.T, dsn string, document []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatalf("写批文：%v", err)
	}
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"publish", "-input", path}, func(key string) string {
		if key == envDatabaseDSN {
			return dsn
		}
		return ""
	}, &out, &errOut)
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, out.String(), errOut.String())
}

// technicalFailure 是译装拒了批文时的整份答复：退出 1、一行诊断，连库之前就停下。
func technicalFailure(diagnostic string) string {
	return "exit 1\n--- stdout\n--- stderr\n" + diagnostic + "\n"
}

func replacedOnce(t *testing.T, document []byte, old, new string) []byte {
	t.Helper()
	if !bytes.Contains(document, []byte(old)) {
		t.Fatalf("批文里没有 %q", old)
	}
	return bytes.Replace(document, []byte(old), []byte(new), 1)
}

// Covers: 票 operator-channel/18 完成判据一的译装那一半——批文在译装里被拒的各格（集合外取值、领域构造门、未知键、
// 缺项、未发布的引用、坏 JSON），与译得过、停在连库之前的那一份，答复与下沉前逐字相同。不连库：译装在开库之前跑完。
func TestTheControlledPublishRefusesTheSameApprovalDocumentsAlike(t *testing.T) {
	seed := demoSeed(t)
	cases := []struct {
		name     string
		document []byte
		answer   string
	}{
		{"demo seed translates and stops before the database", seed,
			technicalFailure("parcel-commercial: IDP_PARCEL_POSTGRES_DSN is required")},
		{"kind outside the closed set", replacedOnce(t, seed, `"kind": "SERVICE_PRODUCT"`, `"kind": "SERVICE_PRODUKT"`),
			technicalFailure(`第 1 项：集合外的商业对象类别 "SERVICE_PRODUKT"`)},
		{"empty tenant", replacedOnce(t, seed, `"tenantId": "SYN-TENANT-01"`, `"tenantId": ""`),
			technicalFailure("第 1 项：party commercial: blank value: tenant ID")},
		{"null tenant", replacedOnce(t, seed, `"tenantId": "SYN-TENANT-01"`, `"tenantId": null`),
			technicalFailure("第 1 项：party commercial: blank value: tenant ID")},
		{"tenant key absent", replacedOnce(t, seed, `"tenantId": "SYN-TENANT-01",`, ``),
			technicalFailure("第 1 项：party commercial: blank value: tenant ID")},
		{"unknown item key", replacedOnce(t, seed, `"kind": "SERVICE_PRODUCT",`, `"kind": "SERVICE_PRODUCT", "kindd": "x",`),
			technicalFailure(`发布批不是本入口的形状：json: unknown field "kindd"`)},
		{"role standing outside the closed set", replacedOnce(t, seed, `"approvalRoleStanding": "CONFIRMED"`, `"approvalRoleStanding": "MAYBE"`),
			technicalFailure(`第 1 项：集合外的批准角色确认 "MAYBE"`)},
		{"empty approval reference", replacedOnce(t, seed, `"reference": "SYN-APPROVAL-2026-001"`, `"reference": ""`),
			technicalFailure("第 1 项：party commercial: blank value: approval reference")},
		{"empty content digest", replacedOnce(t, seed, `"contentDigest": "PCC-1:42054e8ef613c0e7b56b5db2eb50c9657c55a85629eab9a9b2c6fbeeab9922c4"`, `"contentDigest": ""`),
			technicalFailure("第 1 项：party commercial: blank value: commercial content digest")},
		{"joint pass condition outside the closed set", replacedOnce(t, seed, `"jointPassCondition": "ALL_CONTROLS_PASS"`, `"jointPassCondition": "ANY"`),
			technicalFailure(`第 3 项：集合外的共同通过条件 "ANY"`)},
		{"rule category outside the closed set", replacedOnce(t, seed, `"category": "MINIMUM_INGRESS_IDENTITY"`, `"category": "NOPE"`),
			technicalFailure(`第 4 项：集合外的规则分类 "NOPE"`)},
		{"unreleased as-of citation", replacedOnce(t, seed, "REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1", "REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@2"),
			technicalFailure("第 4 项：as-of semantics reference: reference configuration: not released: parcel-shipment/as-of-semantics/submission-receipt@2")},
		{"no items", []byte(`{"items": []}`),
			technicalFailure("发布批没有任何项")},
		{"not JSON", []byte(`items: []`),
			technicalFailure("发布批不是本入口的形状：invalid character 'i' looking for beginning of value")},
	}
	for _, testCase := range cases {
		if got := publishAnswer(t, "", testCase.document); got != testCase.answer {
			t.Errorf("%s:\n got:\n%s\nwant:\n%s", testCase.name, got, testCase.answer)
		}
	}
}

// Covers: 票 operator-channel/18 完成判据一的落库那一半——演示种子整批在空库上发一遍、再重放一遍，两遍的逐项回显与
// 退出码与下沉前逐字相同。摘要在登记册一侧按译出的正文重算（ADR-0126 决定二）：译装改动了正文，摘要对不上，那一项
// 的回显就从已发布换成未受理。
func TestTheControlledPublishAnswersTheDemoSeedAlike(t *testing.T) {
	dsn := freshMigratedDSN(t)
	seed := demoSeed(t)
	if got := publishAnswer(t, dsn, seed); got != demoSeedFirstPublication {
		t.Errorf("首发:\n got:\n%s\nwant:\n%s", got, demoSeedFirstPublication)
	}
	if got := publishAnswer(t, dsn, seed); got != demoSeedReplay {
		t.Errorf("重放:\n got:\n%s\nwant:\n%s", got, demoSeedReplay)
	}
}

const demoSeedFirstPublication = `exit 0
--- stdout
第 1/18 项 SERVICE_PRODUCT SYN-PROD-CN-SG-EXPRESS/v1：PUBLISHED_EFFECTIVE；声明：PENDING_ROUTING=SAVED DELIVERY_CONDITION=SAVED
第 2/18 项 SERVICE_PRODUCT SYN-PROD-CN-SG-ECON/v1：PUBLISHED_EFFECTIVE；声明：DELIVERY_CONDITION=SAVED
第 3/18 项 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY SYN-FIN-CONTROL-01/v1：PUBLISHED_EFFECTIVE；声明：PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY=SAVED
第 4/18 项 ACCEPTANCE_RULE_PACKAGE SYN-RULEPKG-01/v1：PUBLISHED_EFFECTIVE；声明：AS_OF_POLICY=SAVED ACCEPTANCE_CONTENT=SAVED INTAKE_QUALIFICATION=SAVED FINAL_RULE=SAVED RULE_PACKAGE_BODY=SAVED SOURCE_DATA_AMENDMENT=SAVED
第 5/18 项 CUSTOMER_CONTRACT SYN-CONTRACT-01/v1：PUBLISHED_EFFECTIVE；声明：PRE_ACCEPTANCE_CONTROL=SAVED CONTRACT_CONTENT=SAVED CONTRACT_DELEGATION=SAVED DELIVERY_CONDITION=SAVED
第 6/18 项 SETTLEMENT_POLICY SYN-SETTLEMENT-PREPAID-01/v1：PUBLISHED_EFFECTIVE；声明：SETTLEMENT_POLICY_BODY=SAVED
第 7/18 项 CUSTOMER_CONTRACT SYN-CONTRACT-02/v1：PUBLISHED_EFFECTIVE；声明：PRE_ACCEPTANCE_CONTROL=SAVED CONTRACT_CONTENT=SAVED CONTRACT_DELEGATION=SAVED
第 8/18 项 SETTLEMENT_POLICY SYN-SETTLEMENT-PREPAID-02/v1：PUBLISHED_EFFECTIVE；声明：SETTLEMENT_POLICY_BODY=SAVED
第 9/18 项 SUPPLIER_AGREEMENT SYN-SUPPLIER-TRUNK-CN-SG-01/v1：PUBLISHED_EFFECTIVE；声明：SUPPLIER_AGREEMENT_BODY=SAVED
第 10/18 项 SUPPLIER_AGREEMENT SYN-SUPPLIER-TRUNK-CN-SG-01/v2：PUBLISHED_EFFECTIVE；声明：SUPPLIER_AGREEMENT_BODY=SAVED
第 11/18 项 SUPPLIER_AGREEMENT SYN-SUPPLIER-LASTMILE-SG-01/v1：PUBLISHED_EFFECTIVE；声明：SUPPLIER_AGREEMENT_BODY=SAVED
第 12/18 项 PRICE_RULE SYN-PRICE-RULE-CN-SG/v1：PUBLISHED_EFFECTIVE；声明：PRICE_POLICY_BODY=SAVED PRICE_POLICY_CALIBER=SAVED
第 13/18 项 CREDIT_POLICY SYN-CREDIT-01/v1：PUBLISHED_EFFECTIVE；声明：CREDIT_POLICY_BODY=SAVED
第 14/18 项 CUSTOMER_SERVICE_RULE SYN-SERVICE-RULE-01/v1：PUBLISHED_EFFECTIVE；声明：CUSTOMER_SERVICE_RULE_BODY=SAVED
第 15/18 项 CUSTOMER_SERVICE_RULE SYN-SERVICE-RULE-CONTRACT-02/v1：PUBLISHED_EFFECTIVE；声明：CUSTOMER_SERVICE_RULE_BODY=SAVED
第 16/18 项 AUTHORIZATION_RULE SYN-AUTH-PRICE-DIR-01/v1：PUBLISHED_EFFECTIVE
第 17/18 项 AUTHORIZATION_RULE SYN-AUTH-COST-DIR-01/v1：PUBLISHED_EFFECTIVE
第 18/18 项 AUTHORIZATION_RULE SYN-AUTH-CANCEL-01/v1：PUBLISHED_EFFECTIVE；声明：CANCELLATION_AUTHORITY=SAVED
--- stderr
`

const demoSeedReplay = `exit 0
--- stdout
第 1/18 项 SERVICE_PRODUCT SYN-PROD-CN-SG-EXPRESS/v1：REPLAYED；声明：PENDING_ROUTING=ALREADY_REGISTERED DELIVERY_CONDITION=ALREADY_REGISTERED
第 2/18 项 SERVICE_PRODUCT SYN-PROD-CN-SG-ECON/v1：REPLAYED；声明：DELIVERY_CONDITION=ALREADY_REGISTERED
第 3/18 项 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY SYN-FIN-CONTROL-01/v1：REPLAYED；声明：PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY=ALREADY_REGISTERED
第 4/18 项 ACCEPTANCE_RULE_PACKAGE SYN-RULEPKG-01/v1：REPLAYED；声明：AS_OF_POLICY=ALREADY_REGISTERED ACCEPTANCE_CONTENT=ALREADY_REGISTERED INTAKE_QUALIFICATION=ALREADY_REGISTERED FINAL_RULE=ALREADY_REGISTERED RULE_PACKAGE_BODY=ALREADY_REGISTERED SOURCE_DATA_AMENDMENT=ALREADY_REGISTERED
第 5/18 项 CUSTOMER_CONTRACT SYN-CONTRACT-01/v1：REPLAYED；声明：PRE_ACCEPTANCE_CONTROL=ALREADY_REGISTERED CONTRACT_CONTENT=ALREADY_REGISTERED CONTRACT_DELEGATION=ALREADY_REGISTERED DELIVERY_CONDITION=ALREADY_REGISTERED
第 6/18 项 SETTLEMENT_POLICY SYN-SETTLEMENT-PREPAID-01/v1：REPLAYED；声明：SETTLEMENT_POLICY_BODY=ALREADY_REGISTERED
第 7/18 项 CUSTOMER_CONTRACT SYN-CONTRACT-02/v1：REPLAYED；声明：PRE_ACCEPTANCE_CONTROL=ALREADY_REGISTERED CONTRACT_CONTENT=ALREADY_REGISTERED CONTRACT_DELEGATION=ALREADY_REGISTERED
第 8/18 项 SETTLEMENT_POLICY SYN-SETTLEMENT-PREPAID-02/v1：REPLAYED；声明：SETTLEMENT_POLICY_BODY=ALREADY_REGISTERED
第 9/18 项 SUPPLIER_AGREEMENT SYN-SUPPLIER-TRUNK-CN-SG-01/v1：REPLAYED；声明：SUPPLIER_AGREEMENT_BODY=ALREADY_REGISTERED
第 10/18 项 SUPPLIER_AGREEMENT SYN-SUPPLIER-TRUNK-CN-SG-01/v2：REPLAYED；声明：SUPPLIER_AGREEMENT_BODY=ALREADY_REGISTERED
第 11/18 项 SUPPLIER_AGREEMENT SYN-SUPPLIER-LASTMILE-SG-01/v1：REPLAYED；声明：SUPPLIER_AGREEMENT_BODY=ALREADY_REGISTERED
第 12/18 项 PRICE_RULE SYN-PRICE-RULE-CN-SG/v1：REPLAYED；声明：PRICE_POLICY_BODY=ALREADY_REGISTERED PRICE_POLICY_CALIBER=ALREADY_REGISTERED
第 13/18 项 CREDIT_POLICY SYN-CREDIT-01/v1：REPLAYED；声明：CREDIT_POLICY_BODY=ALREADY_REGISTERED
第 14/18 项 CUSTOMER_SERVICE_RULE SYN-SERVICE-RULE-01/v1：REPLAYED；声明：CUSTOMER_SERVICE_RULE_BODY=ALREADY_REGISTERED
第 15/18 项 CUSTOMER_SERVICE_RULE SYN-SERVICE-RULE-CONTRACT-02/v1：REPLAYED；声明：CUSTOMER_SERVICE_RULE_BODY=ALREADY_REGISTERED
第 16/18 项 AUTHORIZATION_RULE SYN-AUTH-PRICE-DIR-01/v1：REPLAYED
第 17/18 项 AUTHORIZATION_RULE SYN-AUTH-COST-DIR-01/v1：REPLAYED
第 18/18 项 AUTHORIZATION_RULE SYN-AUTH-CANCEL-01/v1：REPLAYED；声明：CANCELLATION_AUTHORITY=ALREADY_REGISTERED
--- stderr
`
