package partycommercial_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件镜像 active_rejection_test.go 钉撤回授权适配器的翻译表。授权请求里的动作种类由
// RequestSource（实例半边）声明，适配器只转交不检查——PC 的 AuthorizedAction 今天没有
// 撤回动作，测试因此借现有动作占位钉翻译，不是宣称「撤回凭主动拒绝授权成立」；PC 侧扩
// 枚举属 `PAR-COM-14` 落地那笔（先改 PC CONTEXT 再动代码），见适配器文件注释。

type withdrawalRequestSource struct {
	request pcdomain.AuthorizationRequest
	formed  bool
	err     error
}

func (source *withdrawalRequestSource) FormAuthorizationRequest(
	_ context.Context,
	_ psports.WithdrawalAuthorizationQuery,
) (pcdomain.AuthorizationRequest, bool, error) {
	return source.request, source.formed, source.err
}

func TestWithdrawalIsGrantedWhenAMatchingRuleExists(t *testing.T) {
	grant := rejectionGrant(t, "auth-withdraw", pcdomain.ActiveRejectionAction)
	authorizer := adapter.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{grants: []pcdomain.AuthorityGrant{grant}}),
		&withdrawalRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	result, err := authorizer.AuthorizeWithdrawal(t.Context(), withdrawalQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationGranted {
		t.Fatalf("outcome = %s, want GRANTED", result.Outcome)
	}
	if result.Authority.String() != "auth-withdraw/v1" {
		t.Fatalf("authority = %q, want the adopted grant version", result.Authority)
	}
}

func TestWithdrawalIsRefusedWhenTheScopeHasOtherRules(t *testing.T) {
	grant := rejectionGrant(t, "auth-review", pcdomain.ManualReviewAction)
	authorizer := adapter.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{grants: []pcdomain.AuthorityGrant{grant}}),
		&withdrawalRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	result, err := authorizer.AuthorizeWithdrawal(t.Context(), withdrawalQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRefused {
		t.Fatalf("outcome = %s, want REFUSED", result.Outcome)
	}
	if result.Authority.String() != "" {
		t.Fatal("不允许时仍带了授权引用")
	}
}

func TestWithdrawalReportsRulesNotConfiguredWhenTheBookIsEmpty(t *testing.T) {
	authorizer := adapter.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{}),
		&withdrawalRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	result, err := authorizer.AuthorizeWithdrawal(t.Context(), withdrawalQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRulesNotConfigured {
		t.Fatalf("outcome = %s, want RULES_NOT_CONFIGURED", result.Outcome)
	}
}

func TestWithdrawalSurfacesAuthorityReadFailure(t *testing.T) {
	unavailable := errors.New("权威不可读")
	authorizer := adapter.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{err: unavailable}),
		&withdrawalRequestSource{request: rejectionPCRequest(t), formed: true},
	)

	_, err := authorizer.AuthorizeWithdrawal(t.Context(), withdrawalQuery(t))
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v, want wrapped loader failure", err)
	}
}

func TestWithdrawalDoesNotCallTheProviderWhenTheRequestCannotBeFormed(t *testing.T) {
	authorizer := adapter.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{
			err: errors.New("不该被问到"),
		}),
		&withdrawalRequestSource{formed: false},
	)

	_, err := authorizer.AuthorizeWithdrawal(t.Context(), withdrawalQuery(t))
	if err == nil {
		t.Fatal("范围或时点折不成时仍去问了提供方")
	}
}

// 生产装配今天就是这一格：映射属实例半边，nil 是「显式未配置」的诚实表达。它交回 error
// 而不是`授权规则未配置`——后者是提供方对自己登记册的答案，缺映射是消费方自己的缺口，
// 冒用会把「等租户登记规则」与「等消费侧接映射」两条续办路径混成一条。
func TestWithdrawalStopsWhenTheRequestMappingIsNotConfigured(t *testing.T) {
	authorizer := adapter.NewWithdrawalAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{
			err: errors.New("不该被问到"),
		}),
		nil,
	)

	_, err := authorizer.AuthorizeWithdrawal(t.Context(), withdrawalQuery(t))
	if err == nil {
		t.Fatal("映射未配置时没有停下")
	}
}

func withdrawalQuery(t *testing.T) psports.WithdrawalAuthorizationQuery {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("source identity: %v", err)
	}
	return psports.WithdrawalAuthorizationQuery{
		Identity:          identity,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		Requester:         value(t, psdomain.NewWithdrawalRequesterReference, "CUSTOMER-1"),
		Reason:            value(t, psdomain.NewWithdrawalReasonReference, "ORDER_CANCELLED"),
	}
}
