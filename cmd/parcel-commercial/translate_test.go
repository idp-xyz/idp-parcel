package main

import (
	"strings"
	"testing"
	"time"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证进程口的翻译纪律：批文逐字段过领域构造门、各族声明都携带得动；未知字段、
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

// Covers: 票 03 件 3 的进程口输入面——各族声明都有非测试入口；批准责任、有效区间、
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

// Covers: 票 commercial-closure-settlement-key/02 的进程口输入面——发布批表达得出一份
// 结算政策正文：方式两取值的名称镜像加六维适用范围。合同维分两段收、由
// NewQualifiedVersionLabel 拼成闭包要命中的那个串（ADR-0080）；六维缺一维、方式取集合外
// 的值都在触库前拒收，绝不代填。
func TestASettlementPolicyBodyTranslatesWithAllSixDimensions(t *testing.T) {
	commands, err := publishCommandsFromJSON([]byte(`{
	  "items": [
	    {
	      "tenantId": "tenant-1",
	      "kind": "SETTLEMENT_POLICY",
	      "objectId": "settlement-1",
	      "version": "v1",
	      "scope": "scope-1",
	      "contentDigest": "sha256:settlement-1",
	      "effectiveStartsAt": "2026-01-01T00:00:00Z",
	      "references": {"CUSTOMER_CONTRACT": "contract-1"},
	      "approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
	      "approvalRoleStanding": "CONFIRMED",
	      "declarations": {
	        "settlementPolicyBody": {
	          "method": "PREPAID",
	          "legalEntity": "legal-1",
	          "counterparty": "customer-1",
	          "contract": {"objectId": "contract-1", "version": "v1"},
	          "chargeScope": "charge-prepaid",
	          "currency": "CNY",
	          "effectiveStartsAt": "2026-01-01T00:00:00Z"
	        }
	      }
	    }
	  ]
	}`))
	if err != nil {
		t.Fatalf("翻译结算政策批：%v", err)
	}
	body := commands[0].Declarations.SettlementPolicyBody
	if body == nil {
		t.Fatal("结算政策正文没有翻过去")
	}
	if body.Method != pcdomain.PrepaidMethod {
		t.Fatalf("结算方式 = %q, want PREPAID", body.Method)
	}
	// 合同维要与闭包那侧同出一处：拿一个同身份版本的 QualifiedLabel 比对，两边分头拼串
	// 时这一条会立刻红。
	wantContract, err := pcdomain.NewQualifiedVersionLabel(
		mustObjectID(t, "contract-1"), mustVersionLabel(t, "v1"))
	if err != nil {
		t.Fatalf("两段式合同指称：%v", err)
	}
	applicability := body.Applicability
	if applicability.Contract() != wantContract {
		t.Fatalf("合同维 = %q, want %q", applicability.Contract(), wantContract)
	}
	if applicability.LegalEntity().String() != "legal-1" ||
		applicability.Counterparty().String() != "customer-1" ||
		applicability.ChargeScope().String() != "charge-prepaid" ||
		applicability.Currency().String() != "CNY" ||
		!applicability.Effective().Contains(mustTime(t, "2026-02-01T00:00:00Z")) {
		t.Fatalf("六维适用范围变形：%+v", applicability)
	}

	refusals := map[string]string{
		"集合外结算方式": settlementBatchJSON(`"method": "ON_ACCOUNT",
			"legalEntity": "l", "counterparty": "c",
			"contract": {"objectId": "contract-1", "version": "v1"},
			"chargeScope": "x", "currency": "CNY",
			"effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"缺币种一维": settlementBatchJSON(`"method": "PREPAID",
			"legalEntity": "l", "counterparty": "c",
			"contract": {"objectId": "contract-1", "version": "v1"},
			"chargeScope": "x",
			"effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"合同维只给对象不给版本": settlementBatchJSON(`"method": "PREPAID",
			"legalEntity": "l", "counterparty": "c",
			"contract": {"objectId": "contract-1"},
			"chargeScope": "x", "currency": "CNY",
			"effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"合同维收现成串": settlementBatchJSON(`"method": "PREPAID",
			"legalEntity": "l", "counterparty": "c",
			"contract": "contract-1/v1",
			"chargeScope": "x", "currency": "CNY",
			"effectiveStartsAt": "2026-01-01T00:00:00Z"`),
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

func settlementBatchJSON(body string) string {
	return `{"items": [{"tenantId": "t", "kind": "SETTLEMENT_POLICY", "objectId": "settlement-1",
		"version": "v1", "scope": "s", "contentDigest": "d",
		"effectiveStartsAt": "2026-01-01T00:00:00Z",
		"approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
		"approvalRoleStanding": "CONFIRMED",
		"declarations": {"settlementPolicyBody": {` + body + `}}}]}`
}

// Covers: 信用政策正文（票 party-commercial-context-gaps/03）：额度在批文里是 limitMinor 与
// limitRatioBasisPoints 两个键恰一在场——两个都给、一个不给、负值，都在触库前拒收；零额度是
// 合法声明，`"limitMinor": 0` 必须翻成一份零金额额度而不是「没给」。
func TestACreditPolicyBodyTranslatesExactlyOneLimitForm(t *testing.T) {
	t.Run("amount", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(creditBatchJSON(`"legalEntity": "legal-1",
			"authorityLevel": "level-commercial", "chargeType": "charge-freight",
			"limitMinor": 500000, "effectiveStartsAt": "2026-01-01T00:00:00Z"`)))
		if err != nil {
			t.Fatalf("翻译信用政策批：%v", err)
		}
		body := commands[0].Declarations.CreditPolicyBody
		if body == nil {
			t.Fatal("信用政策正文没有翻过去")
		}
		if minor, ok := body.Limit.AmountMinor(); !ok || minor != 500000 {
			t.Fatalf("额度 = (%d, %v), want 500000", minor, ok)
		}
		if body.LegalEntity.String() != "legal-1" || body.Level.String() != "level-commercial" ||
			body.ChargeType.String() != "charge-freight" ||
			!body.Effective.Contains(mustTime(t, "2026-02-01T00:00:00Z")) {
			t.Fatalf("正文变形：%+v", body)
		}
	})

	t.Run("zero amount is a declared limit", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(creditBatchJSON(`"legalEntity": "legal-1",
			"authorityLevel": "level-commercial", "chargeType": "charge-freight",
			"limitMinor": 0, "effectiveStartsAt": "2026-01-01T00:00:00Z"`)))
		if err != nil {
			t.Fatalf("零金额额度被拒收：%v", err)
		}
		if minor, ok := commands[0].Declarations.CreditPolicyBody.Limit.AmountMinor(); !ok || minor != 0 {
			t.Fatalf("零金额额度 = (%d, %v)，被读成了「没给」", minor, ok)
		}
	})

	t.Run("ratio", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(creditBatchJSON(`"legalEntity": "legal-1",
			"authorityLevel": "level-commercial", "chargeType": "charge-freight",
			"limitRatioBasisPoints": 1500, "effectiveStartsAt": "2026-01-01T00:00:00Z"`)))
		if err != nil {
			t.Fatalf("翻译信用政策批：%v", err)
		}
		if bps, ok := commands[0].Declarations.CreditPolicyBody.Limit.RatioBasisPoints(); !ok || bps != 1500 {
			t.Fatalf("额度 = (%d, %v), want 1500 bps", bps, ok)
		}
	})

	refusals := map[string]string{
		"两格都给": creditBatchJSON(`"legalEntity": "l", "authorityLevel": "a", "chargeType": "c",
			"limitMinor": 100, "limitRatioBasisPoints": 100, "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"两格都不给": creditBatchJSON(`"legalEntity": "l", "authorityLevel": "a", "chargeType": "c",
			"effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"负金额": creditBatchJSON(`"legalEntity": "l", "authorityLevel": "a", "chargeType": "c",
			"limitMinor": -1, "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"缺费用类型": creditBatchJSON(`"legalEntity": "l", "authorityLevel": "a",
			"limitMinor": 1, "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

func creditBatchJSON(body string) string {
	return `{"items": [{"tenantId": "t", "kind": "CREDIT_POLICY", "objectId": "credit-1",
		"version": "v1", "scope": "s", "contentDigest": "d",
		"effectiveStartsAt": "2026-01-01T00:00:00Z",
		"approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
		"approvalRoleStanding": "CONFIRMED",
		"declarations": {"creditPolicyBody": {` + body + `}}}]}`
}

// Covers: 供应商商业协议正文（同票）：供应商、责任法人、协议范围、采购定价方案与区间逐项过
// 构造门；批文里没有方向键——领域把它钉死为 BUY，给了就是未知字段、拒收。
func TestASupplierAgreementBodyTranslatesWithoutADirectionKey(t *testing.T) {
	commands, err := publishCommandsFromJSON([]byte(supplierBatchJSON(`"supplier": "supplier-1",
		"legalEntity": "legal-1", "scope": "scope-procurement", "purchasePlan": "plan-buy-1",
		"effectiveStartsAt": "2026-01-01T00:00:00Z", "effectiveEndsAt": "2026-12-31T00:00:00Z"`)))
	if err != nil {
		t.Fatalf("翻译供应商协议批：%v", err)
	}
	body := commands[0].Declarations.SupplierAgreementBody
	if body == nil {
		t.Fatal("供应商协议正文没有翻过去")
	}
	if body.Supplier.String() != "supplier-1" || body.LegalEntity.String() != "legal-1" ||
		body.Scope.String() != "scope-procurement" || body.PurchasePlan.String() != "plan-buy-1" {
		t.Fatalf("正文变形：%+v", body)
	}
	if body.Effective.Contains(mustTime(t, "2027-01-01T00:00:00Z")) {
		t.Fatal("区间终点没有翻过去")
	}

	refusals := map[string]string{
		"带方向键": supplierBatchJSON(`"supplier": "s", "legalEntity": "l", "scope": "sc",
			"purchasePlan": "p", "direction": "BUY", "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"缺采购定价方案": supplierBatchJSON(`"supplier": "s", "legalEntity": "l", "scope": "sc",
			"effectiveStartsAt": "2026-01-01T00:00:00Z"`),
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

func supplierBatchJSON(body string) string {
	return `{"items": [{"tenantId": "t", "kind": "SUPPLIER_AGREEMENT", "objectId": "agreement-1",
		"version": "v1", "scope": "s", "contentDigest": "d",
		"effectiveStartsAt": "2026-01-01T00:00:00Z",
		"approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
		"approvalRoleStanding": "CONFIRMED",
		"declarations": {"supplierAgreementBody": {` + body + `}}}]}`
}

// Covers: 客户服务规则正文（票 party-commercial-context-gaps/05，ADR-0104）：适用对象恰一（serviceProduct
// 或 customerContract）、责任方、范围、期限按种类成行、材料按索赔类型成行带清单，逐项过构造门；
// `CUSTOMER_SERVICE_RULE` 从此是批文认得的对象类别（85c1c7f 只拓宽领域封闭集，没接批文口）。另四项
// （追踪披露、异常响应、客户更新、通知义务）不进首发，批文里出现就是未知字段、拒收。
func TestACustomerServiceRuleBodyTranslatesBothItems(t *testing.T) {
	commands, err := publishCommandsFromJSON([]byte(customerServiceRuleBatchJSON(`"serviceProduct": "product-1",
		"responsible": "operator-1", "scope": "scope-1",
		"claimDeadlines": [
			{"kind": "FIRST_CLAIM", "startEvent": "event-delivered", "days": 30, "calendar": "calendar-cn"},
			{"kind": "CONCLUSION_REVIEW", "startEvent": "event-conclusion-notified", "days": 15, "calendar": "calendar-cn"}
		],
		"minimumMaterials": [{"claimKind": "claim-loss", "materials": ["material-photo", "material-invoice"]}]`)))
	if err != nil {
		t.Fatalf("翻译客户服务规则批：%v", err)
	}
	if commands[0].Spec.Kind != pcdomain.CustomerServiceRuleObject {
		t.Fatalf("kind = %s, want CUSTOMER_SERVICE_RULE", commands[0].Spec.Kind)
	}
	body := commands[0].Declarations.CustomerServiceRuleBody
	if body == nil {
		t.Fatal("客户服务规则正文没有翻过去")
	}
	if product, applies := body.Applicability.ServiceProduct(); !applies || product.String() != "product-1" {
		t.Fatalf("适用声明变形：%#v", body.Applicability)
	}
	if body.Responsible.String() != "operator-1" || body.Scope.String() != "scope-1" {
		t.Fatalf("责任方或范围变形：%+v", body)
	}
	if len(body.Deadlines) != 2 || body.Deadlines[0].Kind() != pcdomain.FirstClaimDeadline ||
		body.Deadlines[0].DurationDays() != 30 || body.Deadlines[0].StartEvent().String() != "event-delivered" ||
		body.Deadlines[1].Kind() != pcdomain.ConclusionReviewDeadline || body.Deadlines[1].DurationDays() != 15 {
		t.Fatalf("期限变形：%+v", body.Deadlines)
	}
	if len(body.Materials) != 1 || body.Materials[0].ClaimKind().String() != "claim-loss" ||
		len(body.Materials[0].Materials()) != 2 {
		t.Fatalf("材料变形：%+v", body.Materials)
	}

	t.Run("contract applicability", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(customerServiceRuleBatchJSON(`"customerContract": "contract-1",
			"responsible": "operator-1", "scope": "scope-1",
			"minimumMaterials": [{"claimKind": "claim-damage", "materials": ["material-photo"]}]`)))
		if err != nil {
			t.Fatalf("翻译按合同适用的规则批：%v", err)
		}
		body := commands[0].Declarations.CustomerServiceRuleBody
		if contract, applies := body.Applicability.CustomerContract(); !applies || contract.String() != "contract-1" {
			t.Fatalf("适用声明变形：%#v", body.Applicability)
		}
		if _, applies := body.Applicability.ServiceProduct(); applies {
			t.Fatal("按合同适用的规则翻出了一格产品")
		}
	})

	deadline := `"claimDeadlines": [{"kind": "FIRST_CLAIM", "startEvent": "event-delivered", "days": 30, "calendar": "calendar-cn"}]`
	refusals := map[string]string{
		"产品与合同都给": customerServiceRuleBatchJSON(`"serviceProduct": "p", "customerContract": "c",
			"responsible": "o", "scope": "s", ` + deadline),
		"产品与合同都不给": customerServiceRuleBatchJSON(`"responsible": "o", "scope": "s", ` + deadline),
		"集外的期限种类": customerServiceRuleBatchJSON(`"serviceProduct": "p", "responsible": "o", "scope": "s",
			"claimDeadlines": [{"kind": "SOMETHING_ELSE", "startEvent": "e", "days": 30, "calendar": "c"}]`),
		"零时长": customerServiceRuleBatchJSON(`"serviceProduct": "p", "responsible": "o", "scope": "s",
			"claimDeadlines": [{"kind": "FIRST_CLAIM", "startEvent": "e", "days": 0, "calendar": "c"}]`),
		"缺日历": customerServiceRuleBatchJSON(`"serviceProduct": "p", "responsible": "o", "scope": "s",
			"claimDeadlines": [{"kind": "FIRST_CLAIM", "startEvent": "e", "days": 30}]`),
		"空材料清单": customerServiceRuleBatchJSON(`"serviceProduct": "p", "responsible": "o", "scope": "s",
			"minimumMaterials": [{"claimKind": "claim-loss", "materials": []}]`),
		"另四项之一（通知义务）": customerServiceRuleBatchJSON(`"serviceProduct": "p", "responsible": "o", "scope": "s",
			"notificationObligation": "policy-1", ` + deadline),
		"缺责任方": customerServiceRuleBatchJSON(`"serviceProduct": "p", "scope": "s", ` + deadline),
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

func customerServiceRuleBatchJSON(body string) string {
	return `{"items": [{"tenantId": "t", "kind": "CUSTOMER_SERVICE_RULE", "objectId": "csr-1",
		"version": "v1", "scope": "s", "contentDigest": "d",
		"effectiveStartsAt": "2026-01-01T00:00:00Z",
		"approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
		"approvalRoleStanding": "CONFIRMED",
		"declarations": {"customerServiceRuleBody": {` + body + `}}}]}`
}

// Covers: 价格政策正文（票 party-commercial-context-gaps/06）：方向、方案绑定、发布当时 parcel-pricing
// 的答复（planDirection / conversion，ADR-0057，由写批文的人照价卡目录抄）、范围与区间逐项过构造门；
// 口径嵌在正文里，税务两格与体积一格随方向耦合，汇率一节可缺——缺席翻成 nil 而不是零口径。
func TestAPricePolicyBodyTranslatesWithItsCaliber(t *testing.T) {
	t.Run("sell with full caliber", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(priceBatchJSON(`"direction": "SELL",
			"pricingPlan": "plan-buy-1", "planDirection": "BUY", "conversion": "FROZEN_BUY_EVALUATION",
			"scope": "scope-1", "effectiveStartsAt": "2026-01-01T00:00:00Z", "effectiveEndsAt": "2026-12-31T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_INCLUSIVE", "taxClassification": "vat-standard",
				"volumetricFactor": "sell-divisor-5000-cm",
				"fx": {"quoteType": "boc-cash-selling", "asOfSemantics": "AT_ORDER_DATE", "asOfPolicyVersion": "asof-policy/v3"}}`)))
		if err != nil {
			t.Fatalf("翻译价格政策批：%v", err)
		}
		body := commands[0].Declarations.PricePolicyBody
		if body == nil {
			t.Fatal("价格政策正文没有翻过去")
		}
		if body.Direction != pcdomain.SellDirection || body.PlanDirection != pcdomain.BuyDirection ||
			body.Conversion != pcdomain.PlanBindingFrozenBuyEvaluation ||
			body.PricingPlan.String() != "plan-buy-1" || body.Scope.String() != "scope-1" ||
			body.Effective.Contains(mustTime(t, "2027-01-01T00:00:00Z")) {
			t.Fatalf("正文变形：%+v", body)
		}
		if body.Caliber == nil {
			t.Fatal("口径没有翻过去")
		}
		classification, ok := body.Caliber.Tax.Classification()
		if body.Caliber.Tax.Disposition() != pcdomain.TaxInclusive || !ok || classification.String() != "vat-standard" {
			t.Fatalf("税务口径变形：%+v", body.Caliber.Tax)
		}
		factor, ok := body.Caliber.Volumetric.Factor()
		if body.Caliber.Volumetric.Direction() != pcdomain.SellDirection || !ok || factor.String() != "sell-divisor-5000-cm" {
			t.Fatalf("体积口径变形：%+v", body.Caliber.Volumetric)
		}
		if body.Caliber.Fx == nil || body.Caliber.Fx.QuoteType().String() != "boc-cash-selling" ||
			body.Caliber.Fx.AsOfSemantics().String() != "AT_ORDER_DATE" ||
			body.Caliber.Fx.AsOfPolicyVersion().String() != "asof-policy/v3" {
			t.Fatalf("汇率口径变形：%+v", body.Caliber.Fx)
		}
	})

	t.Run("buy caliber without fx", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(priceBatchJSON(`"direction": "BUY",
			"pricingPlan": "plan-buy-1", "planDirection": "BUY", "conversion": "NONE",
			"scope": "scope-1", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_NOT_APPLICABLE"}`)))
		if err != nil {
			t.Fatalf("翻译价格政策批：%v", err)
		}
		caliber := commands[0].Declarations.PricePolicyBody.Caliber
		if caliber == nil || caliber.Fx != nil {
			t.Fatalf("没声明汇率口径却翻出了一份：%+v", caliber)
		}
		if _, ok := caliber.Tax.Classification(); ok || caliber.Tax.Disposition() != pcdomain.TaxNotApplicable {
			t.Fatalf("税务口径变形：%+v", caliber.Tax)
		}
		if _, ok := caliber.Volumetric.Factor(); ok || caliber.Volumetric.Direction() != pcdomain.BuyDirection {
			t.Fatalf("采购方向的体积口径不该有系数：%+v", caliber.Volumetric)
		}
	})

	t.Run("body without caliber", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(priceBatchJSON(`"direction": "SELL",
			"pricingPlan": "plan-sell-1", "planDirection": "SELL", "conversion": "NONE",
			"scope": "scope-1", "effectiveStartsAt": "2026-01-01T00:00:00Z"`)))
		if err != nil {
			t.Fatalf("翻译价格政策批：%v", err)
		}
		if commands[0].Declarations.PricePolicyBody.Caliber != nil {
			t.Fatal("没给口径却翻出了一份")
		}
	})

	refusals := map[string]string{
		"集合外方向": priceBatchJSON(`"direction": "SIDEWAYS", "pricingPlan": "p", "planDirection": "SELL",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"集合外转换": priceBatchJSON(`"direction": "SELL", "pricingPlan": "p", "planDirection": "BUY",
			"conversion": "GUESS", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"缺定价方案": priceBatchJSON(`"direction": "SELL", "planDirection": "SELL",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"销售口径缺体积系数": priceBatchJSON(`"direction": "SELL", "pricingPlan": "p", "planDirection": "SELL",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_NOT_APPLICABLE"}`),
		"采购口径带体积系数": priceBatchJSON(`"direction": "BUY", "pricingPlan": "p", "planDirection": "BUY",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_NOT_APPLICABLE", "volumetricFactor": "f"}`),
		"含税缺分类": priceBatchJSON(`"direction": "BUY", "pricingPlan": "p", "planDirection": "BUY",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_INCLUSIVE"}`),
		"不适用带分类": priceBatchJSON(`"direction": "BUY", "pricingPlan": "p", "planDirection": "BUY",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_NOT_APPLICABLE", "taxClassification": "vat"}`),
		"集合外税务三值": priceBatchJSON(`"direction": "BUY", "pricingPlan": "p", "planDirection": "BUY",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_MAYBE"}`),
		"汇率半缺": priceBatchJSON(`"direction": "BUY", "pricingPlan": "p", "planDirection": "BUY",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_NOT_APPLICABLE", "fx": {"quoteType": "q"}}`),
		"口径带加点键": priceBatchJSON(`"direction": "BUY", "pricingPlan": "p", "planDirection": "BUY",
			"conversion": "NONE", "scope": "s", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"caliber": {"taxDisposition": "TAX_NOT_APPLICABLE", "markup": "plus-1pct"}`),
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

func priceBatchJSON(body string) string {
	return `{"items": [{"tenantId": "t", "kind": "PRICE_RULE", "objectId": "price-1",
		"version": "v1", "scope": "s", "contentDigest": "d",
		"effectiveStartsAt": "2026-01-01T00:00:00Z",
		"approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
		"approvalRoleStanding": "CONFIRMED",
		"declarations": {"pricePolicyBody": {` + body + `}}}]}`
}

func mustObjectID(t *testing.T, raw string) pcdomain.CommercialObjectID {
	t.Helper()
	value, err := pcdomain.NewCommercialObjectID(raw)
	if err != nil {
		t.Fatalf("对象标识 %q：%v", raw, err)
	}
	return value
}

func mustVersionLabel(t *testing.T, raw string) pcdomain.CommercialVersionLabel {
	t.Helper()
	value, err := pcdomain.NewCommercialVersionLabel(raw)
	if err != nil {
		t.Fatalf("版本号 %q：%v", raw, err)
	}
	return value
}

func mustTime(t *testing.T, raw string) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("时刻 %q：%v", raw, err)
	}
	return value
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

// Covers: ADR-0080 —— 登记 JSON 的结算节只有三维；合同维在这一层无从表达（多写一个
// contract 字段是未知字段，DisallowUnknownFields 当场拒），三维给一半同样拒。
func TestSettlementSelectorTranslatesWithoutAContractDimension(t *testing.T) {
	registration, err := keyRegistrationFromJSON([]byte(`{
		"tenantId": "tenant-1",
		"customerAccountId": "customer-1",
		"scope": "scope-1",
		"legalEntity": "legal-1",
		"anchorPolicyVersion": "anchor-policy/v1",
		"anchorAt": "2026-07-01T00:00:00Z",
		"requiredBases": ["CUSTOMER_CONTRACT", "ACCEPTANCE_RULE_PACKAGE", "SETTLEMENT_POLICY"],
		"settlement": {"counterparty": "customer-1", "chargeScope": "charge-express", "currency": "SYN"}
	}`))
	if err != nil {
		t.Fatalf("翻译带结算三维的登记：%v", err)
	}
	if registration.SettlementCounterparty.String() != "customer-1" ||
		registration.SettlementChargeScope.String() != "charge-express" ||
		registration.SettlementCurrency.String() != "SYN" {
		t.Fatalf("结算三维变形：%+v", registration)
	}

	refusals := map[string]string{
		"带合同维": `{"tenantId": "t", "customerAccountId": "c", "scope": "s", "legalEntity": "l",
			"anchorPolicyVersion": "a", "anchorAt": "2026-07-01T00:00:00Z",
			"requiredBases": ["CUSTOMER_CONTRACT", "SETTLEMENT_POLICY"],
			"settlement": {"counterparty": "c", "contract": "contract-1/v1",
				"chargeScope": "charge-express", "currency": "SYN"}}`,
		"三维缺一": `{"tenantId": "t", "customerAccountId": "c", "scope": "s", "legalEntity": "l",
			"anchorPolicyVersion": "a", "anchorAt": "2026-07-01T00:00:00Z",
			"requiredBases": ["CUSTOMER_CONTRACT", "SETTLEMENT_POLICY"],
			"settlement": {"counterparty": "c", "chargeScope": "charge-express"}}`,
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := keyRegistrationFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}
