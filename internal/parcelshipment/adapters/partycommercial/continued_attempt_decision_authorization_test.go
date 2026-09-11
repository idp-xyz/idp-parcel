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

// 本文件钉关闭 / 重开授权适配器的翻译表（票 label-channel/30 完成判据 3）。与资料修订那只相比多出一格「授权角色」
// ——它与决定方、授权依据快照一样取自 PC 的裁定（ADR-0116 决定四），期望值从 grant 造出，不从询问造出。
// 关闭权与重开权是两格：只授关闭的范围问重开要答`不允许`，证一口两问没有合成一个动作。

var continuedAttemptAskedAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

type continuedAttemptRequestSource struct {
	coordinates adapter.ContinuedAttemptDecisionRequestCoordinates
	formed      bool
	err         error
	asked       int
}

func (source *continuedAttemptRequestSource) FormRequestCoordinates(
	_ context.Context,
	_ psports.ContinuedAttemptDecisionAuthorizationQuery,
) (adapter.ContinuedAttemptDecisionRequestCoordinates, bool, error) {
	source.asked++
	return source.coordinates, source.formed, source.err
}

func continuedAttemptCoordinates(t *testing.T) adapter.ContinuedAttemptDecisionRequestCoordinates {
	t.Helper()
	return adapter.ContinuedAttemptDecisionRequestCoordinates{
		OperatorRole: pcValue(t, pcdomain.NewOperatorRoleReference, "ops-role-7"),
		LegalEntity:  pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		Level:        pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
		Scope:        pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		Evidence:     pcValue(t, pcdomain.NewEvidenceReference, "evidence-1"),
	}
}

func continuedAttemptQuery(t *testing.T, kind psdomain.ContinuedAttemptDecisionKind) psports.ContinuedAttemptDecisionAuthorizationQuery {
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
	return psports.ContinuedAttemptDecisionAuthorizationQuery{
		Identity:  identity,
		Parcel:    value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		Kind:      kind,
		Requester: value(t, psdomain.NewRequesterReference, "shipper-account-1"),
		Reason:    value(t, psdomain.NewContinuedAttemptReasonReference, "SHIPPER_STOP"),
		At:        continuedAttemptAskedAt,
	}
}

func newContinuedAttemptAuthorizer(
	t *testing.T,
	grants *grantStoreDouble,
	requests adapter.ContinuedAttemptDecisionAuthorizationRequestSource,
) *adapter.ContinuedAttemptDecisionAuthorizationAdapter {
	t.Helper()
	authorizer, err := adapter.NewContinuedAttemptDecisionAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(grants),
		requests,
	)
	if err != nil {
		t.Fatalf("构造关闭 / 重开授权适配器：%v", err)
	}
	return authorizer
}

// Covers: 票 label-channel/36 条 5——两口一拒一不拒：裁定编排为 nil 是装配缺件，构造期就拒、不交出适配器；
// 请求坐标映射为 nil 是有意的「显式未配置」，构造成功，询问到达时如实答未形成（那一格在
// TestContinuedAttemptDecisionStopsBeforeTheProviderWhenCoordinatesCannotBeFormed「映射未配置」）。
func TestTheAuthorizerRefusesANilAdjudicatorButAcceptsAnUnconfiguredRequestMapping(t *testing.T) {
	authorizer, err := adapter.NewContinuedAttemptDecisionAuthorizationAdapter(
		nil, &continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true})
	if err == nil {
		t.Fatal("裁定编排为 nil 仍构造出了适配器")
	}
	if authorizer != nil {
		t.Fatal("拒了还交出适配器")
	}

	authorizer, err = adapter.NewContinuedAttemptDecisionAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(&grantStoreDouble{}), nil)
	if err != nil {
		t.Fatalf("映射未配置被当成了装配缺件：%v", err)
	}
	if authorizer == nil {
		t.Fatal("映射未配置却没交出适配器")
	}
}

// Covers: 判据 3 的`已授权`格——命中该范围的关闭授权，三件从裁定取：授权依据快照是所采用 grant 的版本引用、授权角色
// 是那条 grant 的权限等级、实际决定方是提出请求的运营角色（关闭是运营侧自己的决定，PC `resolveDecider` 不经委派）。
// 三件都不是询问里的请求方（货主账户）。
func TestAControlledClosureIsGrantedWithTheProvidersThreeItems(t *testing.T) {
	authorizer := newContinuedAttemptAuthorizer(t,
		&grantStoreDouble{grants: []pcdomain.AuthorityGrant{rejectionGrant(t, "auth-close", pcdomain.ControlledClosureAction)}},
		&continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true},
	)

	result, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ControlledClosureDecision))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationGranted {
		t.Fatalf("outcome = %s, want GRANTED", result.Outcome)
	}
	if result.Authority.String() != "auth-close/v1" {
		t.Fatalf("authority = %q, want the adopted grant version", result.Authority)
	}
	if result.AuthorityRole.String() != "level-commercial" {
		t.Fatalf("authorityRole = %q, want the adopted grant's level", result.AuthorityRole)
	}
	if result.Decider.String() != "ops-role-7" {
		t.Fatalf("decider = %q, want the operator role PC named", result.Decider)
	}
	if result.Decider.String() == "shipper-account-1" {
		t.Fatal("决定方写成了询问里的请求方（货主账户）——请求方不带来任何授权")
	}
}

// Covers: 一口两问不合成一个动作——同范围只授关闭权时问重开答`不允许`（范围已被表过态），反过来亦然；
// 获准关闭不等于获准重开（PC AuthorizedAction 头注、pc-gaps/13）。
func TestClosureAndReopeningAreTwoGrantsThatDoNotImplyEachOther(t *testing.T) {
	cases := map[string]struct {
		granted pcdomain.AuthorizedAction
		asked   psdomain.ContinuedAttemptDecisionKind
	}{
		"只授关闭，问重开": {granted: pcdomain.ControlledClosureAction, asked: psdomain.ReopeningDecision},
		"只授重开，问关闭": {granted: pcdomain.ReopeningAction, asked: psdomain.ControlledClosureDecision},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			authorizer := newContinuedAttemptAuthorizer(t,
				&grantStoreDouble{grants: []pcdomain.AuthorityGrant{rejectionGrant(t, "auth-one", testCase.granted)}},
				&continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true},
			)

			result, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, testCase.asked))
			if err != nil {
				t.Fatalf("authorize: %v", err)
			}
			if result.Outcome != psports.AuthorizationRefused {
				t.Fatalf("outcome = %s, want REFUSED——范围已被表过态，只是没把这一格放进去", result.Outcome)
			}
			if result.Authority.String() != "" || result.AuthorityRole.String() != "" || result.Decider.String() != "" {
				t.Fatalf("不允许时仍带了三件：%q / %q / %q", result.Authority, result.AuthorityRole, result.Decider)
			}
		})
	}
}

// Covers: 重开走 REOPENING 那一格——授了重开权就答`已授权`，三件同样取自裁定。
func TestAReopeningIsGrantedAgainstTheReopeningGrant(t *testing.T) {
	authorizer := newContinuedAttemptAuthorizer(t,
		&grantStoreDouble{grants: []pcdomain.AuthorityGrant{rejectionGrant(t, "auth-reopen", pcdomain.ReopeningAction)}},
		&continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true},
	)

	result, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ReopeningDecision))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationGranted || result.Authority.String() != "auth-reopen/v1" {
		t.Fatalf("outcome / authority = %s / %q", result.Outcome, result.Authority)
	}
}

// Covers: 该范围该时点一条规则都没有——答`授权规则未配置`（等租户登记 `PAR-COM-13`），不判越权、三件全空。
func TestContinuedAttemptDecisionReportsRulesNotConfiguredWhenTheBookIsEmpty(t *testing.T) {
	authorizer := newContinuedAttemptAuthorizer(t,
		&grantStoreDouble{},
		&continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true},
	)

	result, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ControlledClosureDecision))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRulesNotConfigured {
		t.Fatalf("outcome = %s, want RULES_NOT_CONFIGURED", result.Outcome)
	}
	if result.Authority.String() != "" || result.AuthorityRole.String() != "" || result.Decider.String() != "" {
		t.Fatalf("未配置却带出了三件：%q / %q / %q", result.Authority, result.AuthorityRole, result.Decider)
	}
}

// Covers: 权威读不回上抛——error 格的恢复动作是重试，不得冒充`不允许`或`未配置`。
func TestContinuedAttemptDecisionSurfacesAuthorityReadFailure(t *testing.T) {
	unavailable := errors.New("权威不可读")
	authorizer := newContinuedAttemptAuthorizer(t,
		&grantStoreDouble{err: unavailable},
		&continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true},
	)

	_, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ControlledClosureDecision))
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v, want wrapped loader failure", err)
	}
}

// Covers: 判据 3「RequestSource 折不出 → error 不译成未配置」，且不去问提供方；映射整个未配置（nil）同格。
func TestContinuedAttemptDecisionStopsBeforeTheProviderWhenCoordinatesCannotBeFormed(t *testing.T) {
	t.Run("映射答折不出", func(t *testing.T) {
		grants := &grantStoreDouble{err: errors.New("不该被问到")}
		authorizer := newContinuedAttemptAuthorizer(t, grants, &continuedAttemptRequestSource{formed: false})

		result, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ControlledClosureDecision))
		if err == nil {
			t.Fatalf("outcome = %s；坐标折不成被答成了业务答复", result.Outcome)
		}
	})
	t.Run("映射出错", func(t *testing.T) {
		authorizer := newContinuedAttemptAuthorizer(t,
			&grantStoreDouble{err: errors.New("不该被问到")},
			&continuedAttemptRequestSource{err: errors.New("mapping store down")},
		)
		if _, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ControlledClosureDecision)); err == nil {
			t.Fatal("映射出错时没有停下")
		}
	})
	t.Run("映射未配置", func(t *testing.T) {
		authorizer := newContinuedAttemptAuthorizer(t, &grantStoreDouble{err: errors.New("不该被问到")}, nil)
		if _, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ControlledClosureDecision)); err == nil {
			t.Fatal("映射未配置时没有停下")
		}
	})
}

// Covers: 认不出的决定种类是编程错误——error，且映射与提供方都不被问。
func TestAnUnknownDecisionKindIsAnErrorBeforeAnyoneIsAsked(t *testing.T) {
	requests := &continuedAttemptRequestSource{coordinates: continuedAttemptCoordinates(t), formed: true}
	authorizer := newContinuedAttemptAuthorizer(t, &grantStoreDouble{err: errors.New("不该被问到")}, requests)

	_, err := authorizer.AuthorizeContinuedAttemptDecision(t.Context(), continuedAttemptQuery(t, psdomain.ContinuedAttemptDecisionKindInvalid))
	if err == nil {
		t.Fatal("种类零值被译成了某个动作")
	}
	if requests.asked != 0 {
		t.Fatal("种类都认不出还去问了映射")
	}
}
