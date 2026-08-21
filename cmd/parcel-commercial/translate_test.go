package main

import (
	"strings"
	"testing"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证进程口的翻译纪律：批文逐字段过领域构造门、九族声明全部可携带；未知字段、
// 集合外取值与缺件在触库之前拒收，绝不代填默认。

const fullBatchJSON = `{
  "items": [
    {
      "tenantId": "tenant-1",
      "kind": "ACCEPTANCE_RULE_PACKAGE",
      "objectId": "rules-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:rules-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {
        "reference": "approval-1",
        "source": "source-1",
        "approvedAt": "2026-01-02T00:00:00Z"
      },
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {
        "asOfPolicies": [
          {"judgment": "NETWORK_REACHABILITY", "semantics": "AT_ACCEPTANCE", "policyVersion": "asof/v1"}
        ],
        "acceptanceContent": {
          "applicableGroups": ["CUSTOMER_RELATIONSHIP", "REQUIRED_DOCUMENT"],
          "manualReview": "REQUIRED"
        },
        "intakeQualification": {
          "sources": ["NODE_INTAKE", "OFFSITE_PICKUP"],
          "qualifications": ["INTAKE-QUAL/customs-precheck"]
        },
        "finalRules": [
          {"outcome": "EFFECTIVE_DELIVERY", "finalKind": "FINAL/effective-delivery"}
        ],
        "rulePackageBody": {
          "serviceProduct": "product-1",
          "contract": "contract-1",
          "legalEntity": "legal-1",
          "scope": "scope-1",
          "effectiveStartsAt": "2026-01-01T00:00:00Z",
          "rules": [
            {"category": "MINIMUM_INGRESS_IDENTITY", "reference": "RULE/ingress"}
          ]
        }
      }
    },
    {
      "tenantId": "tenant-1",
      "kind": "CUSTOMER_CONTRACT",
      "objectId": "contract-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:contract-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "effectiveEndsAt": "2027-01-01T00:00:00Z",
      "references": {"ACCEPTANCE_RULE_PACKAGE": "rules-1"},
      "approval": {
        "reference": "approval-2",
        "source": "source-2",
        "approvedAt": "2026-01-02T00:00:00Z"
      },
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {
        "preAcceptanceControl": {"requirement": "NOT_APPLICABLE", "notApplicableBasis": "CONTRACT-CLAUSE/NO-CONTROL"},
        "contractContent": {
          "rulePackage": "rules-1",
          "bindings": [
            {"chargeScope": "charge-1", "policy": "control-policy-1"},
            {"chargeScope": "charge-2", "inapplicabilityBasis": "CONTRACT-CLAUSE/NO-CONTROL"}
          ]
        }
      }
    },
    {
      "tenantId": "tenant-1",
      "kind": "SERVICE_PRODUCT",
      "objectId": "product-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:product-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {
        "reference": "approval-3",
        "source": "source-3",
        "approvedAt": "2026-01-02T00:00:00Z"
      },
      "approvalRoleStanding": "CONFIRMED",
      "declarations": {"pendingRoutingBasis": "PRODUCT-CLAUSE/PENDING-OK"}
    },
    {
      "tenantId": "tenant-1",
      "kind": "AUTHORIZATION_RULE",
      "objectId": "authz-1",
      "version": "v1",
      "scope": "scope-1",
      "contentDigest": "sha256:authz-1",
      "effectiveStartsAt": "2026-01-01T00:00:00Z",
      "approval": {
        "reference": "approval-4",
        "source": "source-4",
        "approvedAt": "2026-01-02T00:00:00Z"
      },
      "approvalRoleStanding": "UNCONFIRMED",
      "declarations": {
        "cancellationAuthority": [
          {"party": "CUSTOMER", "rule": "CANCEL/customer-before-intake"}
        ]
      }
    }
  ]
}`

// Covers: 票 03 件 3 的进程口输入面——九族声明全部有非测试入口；批准责任、有效区间、
// 指名引用与角色确认逐项过领域构造门后进命令。
func TestAFullBatchTranslatesEveryDeclarationFamily(t *testing.T) {
	commands, err := publishCommandsFromJSON([]byte(fullBatchJSON))
	if err != nil {
		t.Fatalf("翻译发布批：%v", err)
	}
	if len(commands) != 4 {
		t.Fatalf("commands = %d, want 4", len(commands))
	}

	rules := commands[0]
	if rules.Spec.Kind != pcdomain.AcceptanceRulePackageObject ||
		rules.RoleStanding != pcdomain.ApprovalRoleConfirmed {
		t.Fatalf("规则包项变形：%+v", rules.Spec)
	}
	if len(rules.Declarations.AsOfPolicies) != 1 ||
		rules.Declarations.AcceptanceContent == nil ||
		rules.Declarations.IntakeQualification == nil ||
		len(rules.Declarations.FinalRules) != 1 ||
		rules.Declarations.RulePackageBody == nil {
		t.Fatal("规则包项的声明族没有全部翻过去")
	}

	contract := commands[1]
	if _, bounded := contract.Spec.Effective.EndsAt(); !bounded {
		t.Fatal("有界区间被翻成了开放结束")
	}
	if reference, present := contract.Spec.References[pcdomain.AcceptanceRulePackageObject]; !present ||
		reference.String() != "rules-1" {
		t.Fatal("指名引用没有随项翻过去")
	}
	if contract.Declarations.PreAcceptanceControl == nil || contract.Declarations.ContractContent == nil {
		t.Fatal("合同项的声明族没有全部翻过去")
	}
	if len(contract.Declarations.ContractContent.Bindings) != 2 {
		t.Fatalf("控制约定 = %d, want 2", len(contract.Declarations.ContractContent.Bindings))
	}

	product := commands[2]
	if product.Declarations.PendingRoutingBasis == nil ||
		product.Declarations.PendingRoutingBasis.String() != "PRODUCT-CLAUSE/PENDING-OK" {
		t.Fatal("待路由许可依据没有翻过去")
	}

	authorization := commands[3]
	if authorization.RoleStanding != pcdomain.ApprovalRoleUnconfirmed {
		t.Fatal("未确认的批准角色被翻成了别的")
	}
	if len(authorization.Declarations.CancellationAuthority) != 1 {
		t.Fatal("取消授权目录没有翻过去")
	}
}

// Covers: 零默认纪律——未知字段（打错名不得静默变缺席）、集合外取值、两头都带的
// 控制约定都在触库前拒收，错误点名是哪一项坏了。
func TestTranslationRefusesUnknownFieldsAndOutOfSetValues(t *testing.T) {
	refusals := map[string]string{
		"未知字段": `{"items": [{"tenantId": "t", "kind": "SERVICE_PRODUCT", "objectId": "p", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"aproval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED"}]}`,
		"集合外对象类别": `{"items": [{"tenantId": "t", "kind": "SOMETHING_ELSE", "objectId": "p", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED"}]}`,
		"集合外角色确认": `{"items": [{"tenantId": "t", "kind": "SERVICE_PRODUCT", "objectId": "p", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "MAYBE"}]}`,
		"两头都带的控制约定": `{"items": [{"tenantId": "t", "kind": "CUSTOMER_CONTRACT", "objectId": "c", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED",
			"declarations": {"contractContent": {"rulePackage": "r",
				"bindings": [{"chargeScope": "x", "policy": "p", "inapplicabilityBasis": "b"}]}}}]}`,
		"空批": `{"items": []}`,
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

// Covers: 解析键登记的进程口输入面——四项实例参数与必需依据种类逐项过构造门；
// 集合外种类拒收。
func TestKeyRegistrationTranslates(t *testing.T) {
	registration, err := keyRegistrationFromJSON([]byte(`{
		"tenantId": "tenant-1",
		"customerAccountId": "customer-1",
		"scope": "scope-1",
		"legalEntity": "legal-1",
		"anchorPolicyVersion": "anchor-policy/v1",
		"anchorAt": "2026-07-01T00:00:00Z",
		"requiredBases": ["CUSTOMER_CONTRACT", "ACCEPTANCE_RULE_PACKAGE"]
	}`))
	if err != nil {
		t.Fatalf("翻译登记：%v", err)
	}
	if registration.Scope.String() != "scope-1" || len(registration.RequiredBases) != 2 {
		t.Fatalf("登记变形：%+v", registration)
	}

	if _, err := keyRegistrationFromJSON([]byte(`{
		"tenantId": "tenant-1",
		"customerAccountId": "customer-1",
		"scope": "scope-1",
		"legalEntity": "legal-1",
		"anchorPolicyVersion": "anchor-policy/v1",
		"anchorAt": "2026-07-01T00:00:00Z",
		"requiredBases": ["NOT_A_KIND"]
	}`)); err == nil || !strings.Contains(err.Error(), "NOT_A_KIND") {
		t.Fatalf("集合外依据种类没有被点名拒收：%v", err)
	}
}
