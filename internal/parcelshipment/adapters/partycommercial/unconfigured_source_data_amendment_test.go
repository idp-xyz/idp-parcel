package partycommercial_test

import (
	"testing"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: 票 ps-port-remainder/04 第 2 条——提供方那半没立之前，两只未配置适配器对任何询问都答
// 最保守的那一格，且答复不随询问内容变：授权答`规则未配置`（不是`已授权`也不是`拒绝`，两项引用
// 空着），矩阵答 `NotDeclared`（零值）。答复随内容变化是采信了内容的第一个征兆。
func TestUnconfiguredSourceDataAmendmentSeamsAnswerConservativelyAndIdentically(t *testing.T) {
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		value(t, psdomain.NewCustomerAccountID, "SYN-CUSTOMER-01"),
		value(t, psdomain.NewSource, "SYN-SOURCE-01"),
		value(t, psdomain.NewSourceRequestKey, "SYN-KEY-01"),
	)
	if err != nil {
		t.Fatalf("来源身份：%v", err)
	}
	scope, err := psdomain.NewShipmentScopedSourceData(
		value(t, psdomain.NewShipmentRequestID, "SYN-REQ-01"),
		value(t, psdomain.NewSourceDataGroupReference, "SYN-GROUP-CONSIGNEE"),
	)
	if err != nil {
		t.Fatalf("资料范围：%v", err)
	}

	authorizer := pspartycommercial.UnconfiguredSourceDataAmendmentAuthorizer{}
	for name, requester := range map[string]string{
		"customer": "SYN-CUSTOMER-01",
		"operator": "SYN-OPERATOR-ROLE-01",
		"anyone":   "SYN-ANYONE",
		"blankish": "-",
	} {
		authorization, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), psports.SourceDataAmendmentAuthorizationQuery{
			Identity:  identity,
			Scope:     scope,
			Requester: value(t, psdomain.NewRequesterReference, requester),
			Reason:    value(t, psdomain.NewAmendmentReasonReference, "SYN-REASON-01"),
		})
		if err != nil {
			t.Fatalf("%s：未配置适配器不该报错：%v", name, err)
		}
		if authorization.Outcome != psports.AuthorizationRulesNotConfigured {
			t.Fatalf("%s：outcome = %v, want RULES_NOT_CONFIGURED——没有规则时既不能放行也不能判越权", name, authorization.Outcome)
		}
		if authorization.Authority.String() != "" || authorization.Decider.String() != "" {
			t.Fatalf("%s：未配置却带出了授权快照 %q / 决定方 %q", name, authorization.Authority, authorization.Decider)
		}
	}

	rules := pspartycommercial.UnconfiguredSourceDataRuleDeclaration{}
	for _, intent := range []psdomain.AmendmentIntent{psdomain.SupplementIntent, psdomain.CorrectionIntent, psdomain.ExplicitClearIntent} {
		allowance, err := rules.DeclareSourceDataAmendment(t.Context(), psports.SourceDataAmendmentQuery{
			Identity: identity,
			Scope:    scope,
			Intent:   intent,
			Reason:   value(t, psdomain.NewAmendmentReasonReference, "SYN-REASON-01"),
		})
		if err != nil {
			t.Fatalf("%s：未配置适配器不该报错：%v", intent, err)
		}
		if allowance != psports.SourceDataAmendmentNotDeclared {
			t.Fatalf("%s：allowance = %v, want NOT_DECLARED——矩阵不在就是没人说过，不是允许也不是拒绝", intent, allowance)
		}
	}
}
