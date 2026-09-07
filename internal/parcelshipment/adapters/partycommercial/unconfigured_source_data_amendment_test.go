package partycommercial_test

import (
	"testing"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: 票 ps-port-remainder/04 第 2 条——提供方那半没立之前，未配置的授权适配器对任何询问都答
// 最保守的那一格，且答复不随询问内容变：答`规则未配置`（不是`已授权`也不是`拒绝`，两项引用空着）。
// 答复随内容变化是采信了内容的第一个征兆。矩阵那只未配置适配器原先也在这里证，已随票 02 余段接真退场，
// 真读法的用例在 source_data_amendment_allowance_test.go。
func TestUnconfiguredSourceDataAmendmentAuthorizerAnswersConservativelyAndIdentically(t *testing.T) {
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
}
