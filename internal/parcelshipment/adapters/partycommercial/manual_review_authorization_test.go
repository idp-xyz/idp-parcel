package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type reviewRequestSource struct {
	request pcdomain.AuthorizationRequest
	formed  bool
	err     error
}

func (source *reviewRequestSource) FormAuthorizationRequest(
	_ context.Context,
	_ psports.ManualReviewAuthorizationQuery,
) (pcdomain.AuthorizationRequest, bool, error) {
	return source.request, source.formed, source.err
}

// Covers: UC-PC-003 结果表`已授权`——一条覆盖该范围、法人、等级与时点的人工复核授权命中，
// 交回所采用的授权规则版本引用，供复核留痕保存（UC-PS-001 `AT-PS-034`「只由规则授权的
// 角色按证据完成复核」）。
func TestManualReviewIsGrantedWhenAMatchingRuleExists(t *testing.T) {
	grant := rejectionGrant(t, "auth-review", pcdomain.ManualReviewAction)
	authorizer := adapter.NewManualReviewAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{grants: []pcdomain.AuthorityGrant{grant}}),
		&reviewRequestSource{request: reviewPCRequest(t, pcdomain.ManualReviewAction), formed: true},
	)

	result, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationGranted {
		t.Fatalf("outcome = %s, want GRANTED", result.Outcome)
	}
	if result.Authority.String() != "auth-review/v1" {
		t.Fatalf("authority = %q, want the adopted grant version", result.Authority)
	}
}

// Covers: PC CONTEXT「获准复核并不等于获准直接拒掉这单业务」的反向——该范围只授了主动
// 拒绝权，复核询问答`不允许`：权威已就这个范围表过态，只是没把复核放进去（UC-PC-003 判据
// 「范围加时刻」）。
func TestManualReviewIsRefusedWhenTheScopeOnlyGrantsRejection(t *testing.T) {
	grant := rejectionGrant(t, "auth-reject", pcdomain.ActiveRejectionAction)
	authorizer := adapter.NewManualReviewAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{grants: []pcdomain.AuthorityGrant{grant}}),
		&reviewRequestSource{request: reviewPCRequest(t, pcdomain.ManualReviewAction), formed: true},
	)

	result, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
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

// Covers: UC-PC-003「没有租户就没有任何授权规则，每一次询问都落在`授权规则未配置`」——
// 空册答未配置，不压成不允许。
func TestManualReviewReportsRulesNotConfiguredWhenTheBookIsEmpty(t *testing.T) {
	authorizer := adapter.NewManualReviewAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{}),
		&reviewRequestSource{request: reviewPCRequest(t, pcdomain.ManualReviewAction), formed: true},
	)

	result, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRulesNotConfigured {
		t.Fatalf("outcome = %s, want RULES_NOT_CONFIGURED", result.Outcome)
	}
	if result.Authority.String() != "" {
		t.Fatal("未配置时仍带了授权引用")
	}
}

// Covers: UC-PC-003 结果表`未形成`——权威读取失败原样上抛，不冒充不允许或未配置。
func TestManualReviewSurfacesAuthorityReadFailure(t *testing.T) {
	unavailable := errors.New("权威不可读")
	authorizer := adapter.NewManualReviewAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{err: unavailable}),
		&reviewRequestSource{request: reviewPCRequest(t, pcdomain.ManualReviewAction), formed: true},
	)

	_, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v, want wrapped loader failure", err)
	}
}

// Covers: 实例半边缺席即未形成——映射折不出请求（或压根没配映射）时不去问提供方，也不把
// 缺映射译成`授权规则未配置`：那一格说的是租户没登记规则，而这里是产品还没配映射。
func TestManualReviewDoesNotCallTheProviderWhenTheRequestCannotBeFormed(t *testing.T) {
	adjudicate := pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{
		err: errors.New("不该被问到"),
	})

	t.Run("mapping formed nothing", func(t *testing.T) {
		authorizer := adapter.NewManualReviewAuthorizationAdapter(adjudicate, &reviewRequestSource{formed: false})
		_, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
		if err == nil {
			t.Fatal("范围或时点折不成时仍去问了提供方")
		}
	})

	t.Run("mapping not configured", func(t *testing.T) {
		authorizer := adapter.NewManualReviewAuthorizationAdapter(adjudicate, nil)
		_, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
		if err == nil {
			t.Fatal("映射未配置时仍去问了提供方")
		}
	})
}

// Covers: 本口只问「谁有权复核」——映射若折出了别的动作，一条只授拒绝权的规则会被读成
// 复核权。这是词汇表之外的输入，按 ErrUntranslatableAnswer 拒，且不去问提供方。
func TestManualReviewRejectsAMappingThatFormsAnotherAction(t *testing.T) {
	authorizer := adapter.NewManualReviewAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{
			err: errors.New("不该被问到"),
		}),
		&reviewRequestSource{request: reviewPCRequest(t, pcdomain.ActiveRejectionAction), formed: true},
	)

	_, err := authorizer.AuthorizeManualReview(t.Context(), reviewQuery(t))
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("error = %v, want ErrUntranslatableAnswer——映射折出的不是人工复核动作", err)
	}
}

func reviewQuery(t *testing.T) psports.ManualReviewAuthorizationQuery {
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
	return psports.ManualReviewAuthorizationQuery{
		Identity:          identity,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		Reviewer:          value(t, psdomain.NewReviewerReference, "OPERATOR-1"),
		Evidence:          value(t, psdomain.NewReviewEvidenceReference, "EVID-R1"),
	}
}

func reviewPCRequest(t *testing.T, action pcdomain.AuthorizedAction) pcdomain.AuthorizationRequest {
	t.Helper()
	request, err := pcdomain.NewAuthorizationRequest(
		action,
		pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
		pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		pcValue(t, pcdomain.NewStructuredReason, "MANUAL_REVIEW_COMPLETION"),
		pcValue(t, pcdomain.NewEvidenceReference, "EVID-R1"),
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("authorization request: %v", err)
	}
	return request
}
