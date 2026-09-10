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
        "finalRuleValidity": {"anchor": "CHANNEL_RESULT_OBSERVED", "duration": "P3DT12H"},
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
	// 面单有效期作兄弟键随终局规则翻过去：P3DT12H 是 84 小时，按时刻精度、不折成天。
	if validity := rules.Declarations.FinalRuleValidity; validity == nil ||
		validity.Anchor() != pcdomain.ChannelResultObservedAnchor ||
		validity.Duration() != 84*time.Hour {
		t.Fatalf("面单有效期 = %+v，want CHANNEL_RESULT_OBSERVED / 84h", rules.Declarations.FinalRuleValidity)
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
		"集合外的起算时刻种类": `{"items": [{"tenantId": "t", "kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "r", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED",
			"declarations": {"finalRules": [{"outcome": "EFFECTIVE_DELIVERY", "finalKind": "F"}],
				"finalRuleValidity": {"anchor": "LABEL_ISSUED", "duration": "P3D"}}}]}`,
		"按月计的面单有效期": `{"items": [{"tenantId": "t", "kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "r", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED",
			"declarations": {"finalRules": [{"outcome": "EFFECTIVE_DELIVERY", "finalKind": "F"}],
				"finalRuleValidity": {"anchor": "CHANNEL_RESULT_OBSERVED", "duration": "P1M"}}}]}`,
		"零时长的面单有效期": `{"items": [{"tenantId": "t", "kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "r", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED",
			"declarations": {"finalRules": [{"outcome": "EFFECTIVE_DELIVERY", "finalKind": "F"}],
				"finalRuleValidity": {"anchor": "CHANNEL_RESULT_OBSERVED", "duration": "PT0S"}}}]}`,
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

// Covers: ADR-0120 Decision 六——批文 `sourceDataAmendment{closed, rules[]}`：closed 必填不给默认（缺席整项拒，
// 不是「默认转复核」）；rules 在 closed=true 时可省；allowance 只收 ALLOWED / DISALLOWED，NOT_DECLARED 是缺格的
// 读法不是一格的取值；阶段与意图取 parcel-shipment 原词、集外拒；dataGroup 开放引用只查非空；未知键拒。
func TestSourceDataAmendmentTranslatesExactlyTheDeclaredShape(t *testing.T) {
	item := func(declarations string) string {
		return `{"items": [{"tenantId": "t", "kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "r", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED",
			"declarations": {"sourceDataAmendment": ` + declarations + `}}]}`
	}

	t.Run("未封闭带格逐格翻过去", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(item(`{"closed": false, "rules": [
			{"dataGroup": "consignee.address", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "ALLOWED"},
			{"dataGroup": "consignee.address", "stage": "CUSTOMS_SUBMITTED", "intent": "EXPLICIT_CLEAR", "allowance": "DISALLOWED"}]}`)))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		declared := commands[0].Declarations.SourceDataAmendment
		if declared == nil || declared.Closed || len(declared.Rules) != 2 {
			t.Fatalf("声明 = %+v，want closed=false 两格", declared)
		}
		if declared.Rules[1].DataGroup.String() != "consignee.address" ||
			declared.Rules[1].Stage != pcdomain.DeclaredCustomsSubmitted ||
			declared.Rules[1].Intent != pcdomain.DeclaredExplicitClearIntent ||
			declared.Rules[1].Allowance != pcdomain.AmendmentDisallowed {
			t.Fatalf("第二格变形：%+v", declared.Rules[1])
		}
	})

	t.Run("封闭可以不带格", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(item(`{"closed": true}`)))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		declared := commands[0].Declarations.SourceDataAmendment
		if declared == nil || !declared.Closed || len(declared.Rules) != 0 {
			t.Fatalf("声明 = %+v，want closed=true 零格", declared)
		}
	})

	t.Run("缺键就是没这一节", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(`{"items": [{"tenantId": "t", "kind": "ACCEPTANCE_RULE_PACKAGE", "objectId": "r", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED"}]}`))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		if commands[0].Declarations.SourceDataAmendment != nil {
			t.Fatal("没给这一节却翻出了一份声明")
		}
	})

	refusals := map[string]string{
		"closed 缺席":         `{"rules": [{"dataGroup": "g", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "ALLOWED"}]}`,
		"NOT_DECLARED 登成一格": `{"closed": false, "rules": [{"dataGroup": "g", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "NOT_DECLARED"}]}`,
		"集合外的允许性":           `{"closed": false, "rules": [{"dataGroup": "g", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "MAYBE"}]}`,
		"集合外的阶段":            `{"closed": false, "rules": [{"dataGroup": "g", "stage": "IN_TRANSIT", "intent": "CORRECTION", "allowance": "ALLOWED"}]}`,
		"集合外的意图":            `{"closed": false, "rules": [{"dataGroup": "g", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "REVOKE", "allowance": "ALLOWED"}]}`,
		"资料组为空":             `{"closed": false, "rules": [{"dataGroup": "  ", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "ALLOWED"}]}`,
		"格里的未知键":            `{"closed": false, "rules": [{"dataGroup": "g", "stage": "ACCEPTED_NOT_YET_RECEIVED", "intent": "CORRECTION", "allowance": "ALLOWED", "reason": "x"}]}`,
	}
	for name, declarations := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(item(declarations))); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

// Covers: ADR-0133 决定四——批文 `deliveryConditions{tightens?, methods[], recipientScopeRule, proofOfDeliveryRule}`：方式与
// 规则引用是开放引用只查非空、照字面搬运；tightens 在场即两格都要非空；未知键拒；缺键就是没这一节。零方式、同方式两行、
// 层与 tightens 对不上留给领域（用例测试钉），翻译层不代判。
func TestDeliveryConditionsTranslateExactlyTheDeclaredShape(t *testing.T) {
	item := func(kind, declarations string) string {
		return `{"items": [{"tenantId": "t", "kind": "` + kind + `", "objectId": "o", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED",
			"declarations": {"deliveryConditions": ` + declarations + `}}]}`
	}

	t.Run("产品层三格照字面翻过去", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(item("SERVICE_PRODUCT",
			`{"methods": ["METHOD/safe-drop", "METHOD/in-person"], "recipientScopeRule": "RULE/scope", "proofOfDeliveryRule": "RULE/proof"}`)))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		declared := commands[0].Declarations.DeliveryConditions
		if declared == nil || declared.Tightens != nil || len(declared.Terms.Methods) != 2 ||
			declared.Terms.Methods[0].String() != "METHOD/safe-drop" || declared.Terms.Methods[1].String() != "METHOD/in-person" ||
			declared.Terms.RecipientScopeRule.String() != "RULE/scope" || declared.Terms.ProofOfDeliveryRule.String() != "RULE/proof" {
			t.Fatalf("声明 = %+v，翻译变形", declared)
		}
	})

	t.Run("合同层带所收紧的产品版本", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(item("CUSTOMER_CONTRACT",
			`{"tightens": {"objectId": "product-1", "version": "v3"}, "methods": ["METHOD/in-person"], "recipientScopeRule": "RULE/scope", "proofOfDeliveryRule": "RULE/proof"}`)))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		declared := commands[0].Declarations.DeliveryConditions
		if declared == nil || declared.Tightens == nil ||
			declared.Tightens.ObjectID().String() != "product-1" || declared.Tightens.Version().String() != "v3" || len(declared.Terms.Methods) != 1 {
			t.Fatalf("声明 = %+v，所收紧的产品版本没翻过去", declared)
		}
	})

	t.Run("缺键就是没这一节", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(`{"items": [{"tenantId": "t", "kind": "SERVICE_PRODUCT", "objectId": "o", "version": "v1",
			"scope": "s", "contentDigest": "d", "effectiveStartsAt": "2026-01-01T00:00:00Z",
			"approval": {"reference": "a", "source": "s", "approvedAt": "2026-01-02T00:00:00Z"},
			"approvalRoleStanding": "CONFIRMED"}]}`))
		if err != nil {
			t.Fatalf("翻译：%v", err)
		}
		if commands[0].Declarations.DeliveryConditions != nil {
			t.Fatal("没给这一节却翻出了一份声明")
		}
	})

	refusals := map[string]string{
		"方式为空串":         `{"methods": ["  "], "recipientScopeRule": "RULE/scope", "proofOfDeliveryRule": "RULE/proof"}`,
		"收件范围规则引用缺席":    `{"methods": ["METHOD/in-person"], "proofOfDeliveryRule": "RULE/proof"}`,
		"交付证明规则引用为空":    `{"methods": ["METHOD/in-person"], "recipientScopeRule": "RULE/scope", "proofOfDeliveryRule": ""}`,
		"tightens 缺版本号": `{"tightens": {"objectId": "product-1"}, "methods": ["METHOD/in-person"], "recipientScopeRule": "RULE/scope", "proofOfDeliveryRule": "RULE/proof"}`,
		"未知键":           `{"methods": ["METHOD/in-person"], "recipientScopeRule": "RULE/scope", "proofOfDeliveryRule": "RULE/proof", "default": "METHOD/in-person"}`,
	}
	for name, declarations := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(item("SERVICE_PRODUCT", declarations))); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}

// Covers: ADR-0119 Decision 五——批文时长只认 ISO-8601 的 `P[nD][T[nH][nM][nS]]` 子集：整数、按序至多一次、
// 至少一段；年 / 月 / 周 / 小数 / 逆序 / 重复 / 空 T 都拒。零时长由领域拒，不在本表。
func TestISODurationSubsetParsesExactlyTheDeclaredShape(t *testing.T) {
	accepted := map[string]time.Duration{
		"P3D":          72 * time.Hour,
		"PT72H":        72 * time.Hour,
		"P1DT2H30M15S": 26*time.Hour + 30*time.Minute + 15*time.Second,
		"PT90M":        90 * time.Minute,
		"P0D":          0,
	}
	for raw, want := range accepted {
		t.Run(raw, func(t *testing.T) {
			got, err := parseISODurationSubset(raw)
			if err != nil || got != want {
				t.Fatalf("parse %q = %v, %v; want %v", raw, got, err, want)
			}
		})
	}
	rejected := []string{"", "P", "PT", "3D", "P1Y", "P1M", "P1W", "P1.5D", "PT2H1H", "PT1M2H", "P1DT", "P1D2H", "PT1D", "P-1D", "P1DT2Hx"}
	for _, raw := range rejected {
		t.Run("拒 "+raw, func(t *testing.T) {
			if _, err := parseISODurationSubset(raw); err == nil {
				t.Fatalf("parse %q 被收下了", raw)
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

	// 比例随基数（ADR-0129）：ratioBase 是领域封闭集的原词，批文照译不另定语义。
	t.Run("ratio", func(t *testing.T) {
		commands, err := publishCommandsFromJSON([]byte(creditBatchJSON(`"legalEntity": "legal-1",
			"authorityLevel": "level-commercial", "chargeType": "charge-freight",
			"limitRatioBasisPoints": 1500, "ratioBase": "PRIOR_PERIOD_CONFIRMED_CHARGES",
			"effectiveStartsAt": "2026-01-01T00:00:00Z"`)))
		if err != nil {
			t.Fatalf("翻译信用政策批：%v", err)
		}
		if bps, ok := commands[0].Declarations.CreditPolicyBody.Limit.RatioBasisPoints(); !ok || bps != 1500 {
			t.Fatalf("额度 = (%d, %v), want 1500 bps", bps, ok)
		}
		if base, ok := commands[0].Declarations.CreditPolicyBody.Limit.RatioBase(); !ok || base != pcdomain.PriorPeriodConfirmedChargesBase {
			t.Fatalf("基数 = (%s, %v), want PRIOR_PERIOD_CONFIRMED_CHARGES", base, ok)
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
		"比例缺基数": creditBatchJSON(`"legalEntity": "l", "authorityLevel": "a", "chargeType": "c",
			"limitRatioBasisPoints": 1500, "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"比例基数集外": creditBatchJSON(`"legalEntity": "l", "authorityLevel": "a", "chargeType": "c",
			"limitRatioBasisPoints": 1500, "ratioBase": "DEPOSIT_BALANCE", "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
		"金额带基数": creditBatchJSON(`"legalEntity": "l", "authorityLevel": "a", "chargeType": "c",
			"limitMinor": 100, "ratioBase": "POSTED_BALANCE", "effectiveStartsAt": "2026-01-01T00:00:00Z"`),
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

// Covers: 接受前财务控制策略正文（票 party-commercial-context-gaps/07，ADR-0115）：共同通过条件、控制项按
// 种类 × 范围 × 顺序 × 处置 × 责任逐项过构造门；三个封闭集集外拒收，尤其是 NO_CONTROL——「无控制」由
// 客户合同声明，不是策略正文的一项；零项由发布用例里的构造门拒，翻译层不代判。
func TestAPreAcceptanceFinancialControlPolicyBodyTranslatesItsControls(t *testing.T) {
	commands, err := publishCommandsFromJSON([]byte(controlPolicyBatchJSON(`"jointPassCondition": "ALL_CONTROLS_PASS",
		"controls": [
			{"control": "CREDIT_CHECK", "chargeScope": "charge-scope-a", "order": 2, "onFailure": "AUTHORIZED_DISPOSITION", "responsibility": "operator-legal-1"},
			{"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}
		]`)))
	if err != nil {
		t.Fatalf("翻译策略正文批：%v", err)
	}
	if commands[0].Spec.Kind != pcdomain.PreAcceptanceFinancialControlPolicyObject {
		t.Fatalf("kind = %s, want PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", commands[0].Spec.Kind)
	}
	body := commands[0].Declarations.PreAcceptanceFinancialControlPolicyBody
	if body == nil {
		t.Fatal("策略正文没有翻过去")
	}
	if body.JointPass != pcdomain.AllControlsPass {
		t.Fatalf("共同通过条件 = %v", body.JointPass)
	}
	if len(body.Items) != 2 {
		t.Fatalf("控制项 %d 项, want 2", len(body.Items))
	}
	// 翻译层保留批文顺序，按判断顺序归档是构造门的事。
	if body.Items[0].Kind() != pcdomain.CreditCheckControl || body.Items[0].EvaluationOrder() != 2 ||
		body.Items[0].FailureDisposition() != pcdomain.AuthorizedDispositionOnControlFailure ||
		body.Items[0].Responsibility().String() != "operator-legal-1" || body.Items[0].Scope().String() != "charge-scope-a" {
		t.Fatalf("第一项变形：%#v", body.Items[0])
	}
	if body.Items[1].Kind() != pcdomain.PrepaidFreezeControl || body.Items[1].EvaluationOrder() != 1 ||
		body.Items[1].FailureDisposition() != pcdomain.RejectOnControlFailure {
		t.Fatalf("第二项变形：%#v", body.Items[1])
	}

	for name, body := range map[string]string{
		"no-control is not a control kind": `"jointPassCondition": "ALL_CONTROLS_PASS",
			"controls": [{"control": "NO_CONTROL", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}]`,
		"a disposition outside the closed set": `"jointPassCondition": "ALL_CONTROLS_PASS",
			"controls": [{"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "ALLOW", "responsibility": "customer-1"}]`,
		"a joint pass condition outside the closed set": `"jointPassCondition": "ANY_CONTROL_PASSES",
			"controls": [{"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}]`,
		"a missing joint pass condition is not defaulted": `"controls": [{"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}]`,
		"a zero order": `"jointPassCondition": "ALL_CONTROLS_PASS",
			"controls": [{"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "onFailure": "REJECT", "responsibility": "customer-1"}]`,
		"a blank responsibility": `"jointPassCondition": "ALL_CONTROLS_PASS",
			"controls": [{"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "REJECT", "responsibility": " "}]`,
		"an unknown field": `"jointPassCondition": "ALL_CONTROLS_PASS", "creditPolicy": "credit-1",
			"controls": [{"control": "PREPAID_FREEZE", "chargeScope": "charge-scope-a", "order": 1, "onFailure": "REJECT", "responsibility": "customer-1"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(controlPolicyBatchJSON(body))); err == nil {
				t.Fatal("立不住的策略正文批被翻过去了")
			}
		})
	}
}

// Covers: 合同委派（票 party-commercial-context-gaps/08，ADR-0116 Decision 二）：委派方（种类 × 引用）、动作、范围、
// 等级、区间逐条过构造门；两个封闭集集外拒收——委派方种类只认客户账户 / 责任法人，动作在翻译层只认授权动作
// 封闭集的名字，「客户委派不了的动作」（人工复核、主动拒绝）由发布用例里的构造门拒，翻译层不代判。
func TestContractDelegationsTranslateTheirFiveDimensions(t *testing.T) {
	commands, err := publishCommandsFromJSON([]byte(contractDelegationBatchJSON(`
		{"delegatorKind": "LEGAL_ENTITY", "delegator": "legal-1", "action": "SOURCE_DATA_AMENDMENT", "scope": "scope-1", "level": "level-clerk",
		 "effectiveStartsAt": "2026-01-01T00:00:00Z", "effectiveEndsAt": "2026-07-01T00:00:00Z"},
		{"delegatorKind": "CUSTOMER_ACCOUNT", "delegator": "account-1", "action": "SOURCE_DATA_AMENDMENT", "scope": "scope-1", "level": "level-commercial",
		 "effectiveStartsAt": "2026-01-01T00:00:00Z"}`)))
	if err != nil {
		t.Fatalf("翻译合同委派批：%v", err)
	}
	if commands[0].Spec.Kind != pcdomain.CustomerContractObject {
		t.Fatalf("kind = %s, want CUSTOMER_CONTRACT", commands[0].Spec.Kind)
	}
	delegations := commands[0].Declarations.ContractDelegations
	if len(delegations) != 2 {
		t.Fatalf("委派 %d 条, want 2", len(delegations))
	}
	// 翻译层保留批文顺序，按键归档是构造门的事。
	first := delegations[0]
	if first.Delegator.Kind() != pcdomain.LegalEntityDelegator || first.Delegator.Reference() != "legal-1" ||
		first.Action != pcdomain.SourceDataAmendmentAction || first.Scope.String() != "scope-1" || first.Level.String() != "level-clerk" {
		t.Fatalf("第一条变形：%#v", first)
	}
	if ends, bounded := first.Effective.EndsAt(); !bounded || !ends.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("第一条区间终点变形：%v %v", ends, bounded)
	}
	second := delegations[1]
	if second.Delegator.Kind() != pcdomain.CustomerAccountDelegator || second.Delegator.Reference() != "account-1" ||
		second.Level.String() != "level-commercial" {
		t.Fatalf("第二条变形：%#v", second)
	}
	if _, bounded := second.Effective.EndsAt(); bounded {
		t.Fatal("没给终点的区间翻成了有界")
	}

	for name, body := range map[string]string{
		"a delegator kind outside the closed set": `{"delegatorKind": "OPERATOR_ROLE", "delegator": "operator-1", "action": "SOURCE_DATA_AMENDMENT", "scope": "scope-1", "level": "level-clerk", "effectiveStartsAt": "2026-01-01T00:00:00Z"}`,
		"an action outside the closed set":        `{"delegatorKind": "CUSTOMER_ACCOUNT", "delegator": "account-1", "action": "WITHDRAWAL", "scope": "scope-1", "level": "level-clerk", "effectiveStartsAt": "2026-01-01T00:00:00Z"}`,
		"a blank delegator":                       `{"delegatorKind": "CUSTOMER_ACCOUNT", "delegator": " ", "action": "SOURCE_DATA_AMENDMENT", "scope": "scope-1", "level": "level-clerk", "effectiveStartsAt": "2026-01-01T00:00:00Z"}`,
		"a missing start":                         `{"delegatorKind": "CUSTOMER_ACCOUNT", "delegator": "account-1", "action": "SOURCE_DATA_AMENDMENT", "scope": "scope-1", "level": "level-clerk"}`,
		"an unknown field":                        `{"delegatorKind": "CUSTOMER_ACCOUNT", "delegator": "account-1", "action": "SOURCE_DATA_AMENDMENT", "scope": "scope-1", "level": "level-clerk", "dataGroup": "consignee", "effectiveStartsAt": "2026-01-01T00:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := publishCommandsFromJSON([]byte(contractDelegationBatchJSON(body))); err == nil {
				t.Fatal("立不住的合同委派批被翻过去了")
			}
		})
	}
}

func contractDelegationBatchJSON(rows string) string {
	return `{"items": [{"tenantId": "t", "kind": "CUSTOMER_CONTRACT", "objectId": "contract-1",
		"version": "v1", "scope": "s", "contentDigest": "d",
		"effectiveStartsAt": "2026-01-01T00:00:00Z",
		"approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
		"approvalRoleStanding": "CONFIRMED",
		"declarations": {"contractDelegations": [` + rows + `]}}]}`
}

func controlPolicyBatchJSON(body string) string {
	return `{"items": [{"tenantId": "t", "kind": "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", "objectId": "fcp-1",
		"version": "v1", "scope": "s", "contentDigest": "d",
		"effectiveStartsAt": "2026-01-01T00:00:00Z",
		"approval": {"reference": "a", "source": "s", "approvedAt": "2025-12-15T00:00:00Z"},
		"approvalRoleStanding": "CONFIRMED",
		"declarations": {"preAcceptanceFinancialControlPolicyBody": {` + body + `}}}]}`
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

// Covers: ADR-0127 决定二 —— 登记 JSON 的信用节只有两维（商业权限等级 × 费用类型）。法人与时点
// 在这一层无从表达：键上已有法人候选与锚点，信用节里再写一个 legalEntity 是未知字段，
// DisallowUnknownFields 当场拒；两维给一半同样拒，不折成缺席。
//
// 这一节是 ADR-0127 那条路在进程口的入口：登记面 validateCredit 要求含 CREDIT_POLICY 就两维必填，
// 而 register-resolution-key 是唯一构造 ResolutionKeyRegistration 的生产入口——文档面没有这一节，
// 登记面放行了也没有任何租户能把两维送进去。
func TestCreditSelectorTranslatesAsTwoDimensions(t *testing.T) {
	registration, err := keyRegistrationFromJSON([]byte(`{
		"tenantId": "tenant-1",
		"customerAccountId": "customer-1",
		"scope": "scope-1",
		"legalEntity": "legal-1",
		"anchorPolicyVersion": "anchor-policy/v1",
		"anchorAt": "2026-07-01T00:00:00Z",
		"requiredBases": ["CUSTOMER_CONTRACT", "ACCEPTANCE_RULE_PACKAGE", "CREDIT_POLICY"],
		"credit": {"level": "level-commercial", "chargeType": "charge-freight"}
	}`))
	if err != nil {
		t.Fatalf("翻译带信用两维的登记：%v", err)
	}
	if registration.CreditLevel.String() != "level-commercial" ||
		registration.CreditChargeType.String() != "charge-freight" {
		t.Fatalf("信用两维变形：%+v", registration)
	}
	// 结算三维不得被顺手带上：本文档没有结算节。
	if registration.SettlementCounterparty.String() != "" ||
		registration.SettlementChargeScope.String() != "" ||
		registration.SettlementCurrency.String() != "" {
		t.Fatalf("没有结算节的登记带上了结算维度：%+v", registration)
	}

	refusals := map[string]string{
		"带法人维": `{"tenantId": "t", "customerAccountId": "c", "scope": "s", "legalEntity": "l",
			"anchorPolicyVersion": "a", "anchorAt": "2026-07-01T00:00:00Z",
			"requiredBases": ["CUSTOMER_CONTRACT", "CREDIT_POLICY"],
			"credit": {"level": "level-commercial", "chargeType": "charge-freight", "legalEntity": "l"}}`,
		"两维缺一": `{"tenantId": "t", "customerAccountId": "c", "scope": "s", "legalEntity": "l",
			"anchorPolicyVersion": "a", "anchorAt": "2026-07-01T00:00:00Z",
			"requiredBases": ["CUSTOMER_CONTRACT", "CREDIT_POLICY"],
			"credit": {"level": "level-commercial"}}`,
		"两维写成空串": `{"tenantId": "t", "customerAccountId": "c", "scope": "s", "legalEntity": "l",
			"anchorPolicyVersion": "a", "anchorAt": "2026-07-01T00:00:00Z",
			"requiredBases": ["CUSTOMER_CONTRACT", "CREDIT_POLICY"],
			"credit": {"level": "", "chargeType": ""}}`,
	}
	for name, raw := range refusals {
		t.Run(name, func(t *testing.T) {
			if _, err := keyRegistrationFromJSON([]byte(raw)); err == nil {
				t.Fatal("坏输入被翻译收下了")
			}
		})
	}
}
