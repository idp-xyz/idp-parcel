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

// 本文件钉资料修订授权适配器的翻译表（票 ps-port-remainder/03，ADR-0116 Decision 三）。与撤回、
// 拒绝两只的区别只有一处：这一口的答复多一格「实际决定方」，而它**必须从 PC 的裁定里读出来**，
// 不能由 PS 看着请求方自己判——UC-PS-002「登录操作人不能替代实际决定方」。所以用例里 grants 与
// delegations 双方都用 PC 领域对象直接构造，决定方的期望值取自委派方，不取自请求方。

var amendmentAskedAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

type amendmentRequestSource struct {
	coordinates adapter.SourceDataAmendmentRequestCoordinates
	formed      bool
	err         error
}

func (source *amendmentRequestSource) FormRequestCoordinates(
	_ context.Context,
	_ psports.SourceDataAmendmentAuthorizationQuery,
) (adapter.SourceDataAmendmentRequestCoordinates, bool, error) {
	return source.coordinates, source.formed, source.err
}

type delegationViewDouble struct {
	delegations []pcdomain.ContractDelegation
	err         error
}

func (view *delegationViewDouble) LoadEffectiveDelegations(
	context.Context, pcdomain.TenantID, pcdomain.CommercialScopeReference, time.Time,
) ([]pcdomain.ContractDelegation, error) {
	if view.err != nil {
		return nil, view.err
	}
	return view.delegations, nil
}

// Covers: 客户账户自己提出的资料修订——裁定命中该范围的资料修订授权，答`已授权`，授权快照是所采用的
// grant 版本引用，实际决定方就是那个客户账户（ADR-0116 Decision 三第一格）。委派读口装成「不该被问到」：
// 客户自己的决定不经委派解出，编排不该为它多问一次。
func TestSourceDataAmendmentByTheCustomerAccountIsGrantedWithTheAccountAsDecider(t *testing.T) {
	grant := amendmentGrant(t, "auth-amend")
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{grants: []pcdomain.AuthorityGrant{grant}},
			&delegationViewDouble{err: errors.New("不该被问到")},
		),
		&amendmentRequestSource{coordinates: customerCoordinates(t), formed: true},
	)

	result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationGranted {
		t.Fatalf("outcome = %s, want GRANTED", result.Outcome)
	}
	if result.Authority.String() != "auth-amend/v1" {
		t.Fatalf("authority = %q, want the adopted grant version", result.Authority)
	}
	if result.Decider.String() != "customer-1" {
		t.Fatalf("decider = %q, want the customer account that asked", result.Decider)
	}
}

// Covers: 运营角色代客户账户提出资料修订、合同委派在场——答`已授权`，实际决定方是**委派方**，不是提出请求
// 的运营角色（ADR-0116 Decision 三第二格；UC-PS-002「登录操作人不能替代实际决定方」）。两种委派方各一格：
// 客户账户自己委派，与其责任法人委派——后者的决定方引用是法人引用，与询问身份上的客户账户不同串，
// 正好证明 PS 没有拿请求方或身份上的账户顶替。
func TestSourceDataAmendmentByAnOperatorRoleNamesTheDelegatorAsDecider(t *testing.T) {
	cases := map[string]struct {
		delegator   pcdomain.Delegator
		wantDecider string
	}{
		"the customer account delegated": {
			delegator:   accountDelegator(t, "customer-1"),
			wantDecider: "customer-1",
		},
		"the responsible legal entity delegated": {
			delegator:   legalEntityDelegator(t, "legal-1"),
			wantDecider: "legal-1",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
				pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
					&grantStoreDouble{grants: []pcdomain.AuthorityGrant{amendmentGrant(t, "auth-amend")}},
					&delegationViewDouble{delegations: []pcdomain.ContractDelegation{
						amendmentDelegation(t, "contract-1", testCase.delegator),
					}},
				),
				&amendmentRequestSource{coordinates: operatorCoordinates(t, "operator-1"), formed: true},
			)

			result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "operator-1"))
			if err != nil {
				t.Fatalf("authorize: %v", err)
			}
			if result.Outcome != psports.AuthorizationGranted {
				t.Fatalf("outcome = %s, want GRANTED", result.Outcome)
			}
			if result.Authority.String() != "auth-amend/v1" {
				t.Fatalf("authority = %q, want the adopted grant version", result.Authority)
			}
			if result.Decider.String() == "operator-1" {
				t.Fatal("决定方写成了代录的运营角色——登录操作人顶替了实际决定方")
			}
			if result.Decider.String() != testCase.wantDecider {
				t.Fatalf("decider = %q, want the delegator %q", result.Decider, testCase.wantDecider)
			}
		})
	}
}

// Covers: grant 在场而委派缺席——运营角色代录被拒（ErrDelegationAbsent Is ErrNotAuthorized，ADR-0116
// Decision 三）：答`不允许`，不答`未配置`；缺的是客户把决定权交出来，不是租户的规则。两项引用都空着。
func TestSourceDataAmendmentByAnOperatorRoleIsRefusedWithoutADelegation(t *testing.T) {
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{grants: []pcdomain.AuthorityGrant{amendmentGrant(t, "auth-amend")}},
			&delegationViewDouble{},
		),
		&amendmentRequestSource{coordinates: operatorCoordinates(t, "operator-1"), formed: true},
	)

	result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "operator-1"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRefused {
		t.Fatalf("outcome = %s, want REFUSED——规则在场、委派缺席是业务拒绝，不是未配置", result.Outcome)
	}
	if result.Authority.String() != "" || result.Decider.String() != "" {
		t.Fatalf("不允许时仍带了授权引用 %q / 决定方 %q", result.Authority, result.Decider)
	}
}

// Covers: 红线「不复用人工复核 / 主动拒绝两格顶替资料修订」——同范围只有复核与拒绝的授权时，资料修订答
// `不允许`（范围已被表过态），既不是`已授权`也不是`未配置`。适配器总以资料修订动作去问，这一格证的是它
// 没有借别的动作走通。
func TestSourceDataAmendmentIsRefusedWhenTheScopeOnlyGrantsTheOtherTwoActions(t *testing.T) {
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{grants: []pcdomain.AuthorityGrant{
				rejectionGrant(t, "auth-review", pcdomain.ManualReviewAction),
				rejectionGrant(t, "auth-reject", pcdomain.ActiveRejectionAction),
			}},
			&delegationViewDouble{},
		),
		&amendmentRequestSource{coordinates: customerCoordinates(t), formed: true},
	)

	result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRefused {
		t.Fatalf("outcome = %s, want REFUSED——获准复核或拒单不等于获准修订资料", result.Outcome)
	}
}

// Covers: 该范围该时点一条规则都没有——答`授权规则未配置`，等租户登记 `PAR-COM-14`；编排据以停在未决，
// 不判客户越权。
func TestSourceDataAmendmentReportsRulesNotConfiguredWhenTheBookIsEmpty(t *testing.T) {
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{}, &delegationViewDouble{},
		),
		&amendmentRequestSource{coordinates: customerCoordinates(t), formed: true},
	)

	result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if result.Outcome != psports.AuthorizationRulesNotConfigured {
		t.Fatalf("outcome = %s, want RULES_NOT_CONFIGURED", result.Outcome)
	}
	if result.Authority.String() != "" || result.Decider.String() != "" {
		t.Fatalf("未配置却带出了授权快照 %q / 决定方 %q", result.Authority, result.Decider)
	}
}

// Covers: 权威读不回上抛——error 格的恢复动作是重试，不得冒充`不允许`或`未配置`。
func TestSourceDataAmendmentSurfacesAuthorityReadFailure(t *testing.T) {
	unavailable := errors.New("权威不可读")
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{err: unavailable}, &delegationViewDouble{},
		),
		&amendmentRequestSource{coordinates: customerCoordinates(t), formed: true},
	)

	_, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v, want wrapped loader failure", err)
	}
}

// Covers: 裁定编排没接委派读口（旧构造器）时，运营角色代录那一格是 error 而不是拒绝——「没人问过委派」与
// 「客户没委派」要人做的事相反，PC 编排把前者报成错，适配器原样落在 error 格。
func TestSourceDataAmendmentByAnOperatorRoleIsAnErrorWhenDelegationsAreNotWired(t *testing.T) {
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandler(
			&grantStoreDouble{grants: []pcdomain.AuthorityGrant{amendmentGrant(t, "auth-amend")}},
		),
		&amendmentRequestSource{coordinates: operatorCoordinates(t, "operator-1"), formed: true},
	)

	result, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "operator-1"))
	if err == nil {
		t.Fatalf("outcome = %s；委派读口未接线被答成了业务答复", result.Outcome)
	}
}

// Covers: 商业坐标折不成（实例半边缺映射）——error（未形成），且不去问提供方：一次已经发出的查询收不回来，
// 它本身就回答了「这个范围有没有规则」。
func TestSourceDataAmendmentDoesNotCallTheProviderWhenTheCoordinatesCannotBeFormed(t *testing.T) {
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{err: errors.New("不该被问到")}, &delegationViewDouble{},
		),
		&amendmentRequestSource{formed: false},
	)

	_, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
	if err == nil {
		t.Fatal("坐标折不成时仍去问了提供方")
	}
}

// Covers: 坐标折出来了却没说请求方是谁（种类零值）——同样是 error 且不问提供方。种类不可空的理由见适配器
// 头注：空值分不清「客户自己来的」与「忘了填运营角色」，而两者的决定方不同。
func TestSourceDataAmendmentDoesNotCallTheProviderWhenTheMappingDoesNotNameTheRequesterKind(t *testing.T) {
	coordinates := customerCoordinates(t)
	coordinates.RequesterKind = pcdomain.RequesterKindInvalid
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{err: errors.New("不该被问到")}, &delegationViewDouble{},
		),
		&amendmentRequestSource{coordinates: coordinates, formed: true},
	)

	_, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
	if err == nil {
		t.Fatal("映射没说请求方是谁，适配器却替它挑了一种")
	}
}

// 生产装配今天就是这一格：映射属实例半边（`BD-PS-009` / `PAR-COM-14`），nil 是「显式未配置」的诚实表达。
// 它交回 error 而不是`授权规则未配置`——后者是提供方对自己登记册的答案，缺映射是消费方自己的缺口，冒用会把
// 「等租户登记规则」与「等消费侧接映射」两条续办路径混成一条（与撤回那只同一条理由）。
func TestSourceDataAmendmentStopsWhenTheRequestMappingIsNotConfigured(t *testing.T) {
	authorizer := adapter.NewSourceDataAmendmentAuthorizationAdapter(
		pcapplication.NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
			&grantStoreDouble{err: errors.New("不该被问到")}, &delegationViewDouble{},
		),
		nil,
	)

	_, err := authorizer.AuthorizeSourceDataAmendment(t.Context(), amendmentAuthorizationQuery(t, "customer-1"))
	if err == nil {
		t.Fatal("映射未配置时没有停下")
	}
}

func amendmentAuthorizationQuery(t *testing.T, requester string) psports.SourceDataAmendmentAuthorizationQuery {
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
	scope, err := psdomain.NewShipmentScopedSourceData(
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		value(t, psdomain.NewSourceDataGroupReference, "GROUP-CONSIGNEE"),
	)
	if err != nil {
		t.Fatalf("source data scope: %v", err)
	}
	return psports.SourceDataAmendmentAuthorizationQuery{
		Identity:  identity,
		Scope:     scope,
		Requester: value(t, psdomain.NewRequesterReference, requester),
		Reason:    value(t, psdomain.NewAmendmentReasonReference, "CUSTOMER_CORRECTION"),
	}
}

// customerCoordinates 是实例半边替「客户账户自己请求」补出的商业坐标：法人、等级、范围、原因、证据、时点
// 与 amendmentGrant 造的授权同键，请求方种类为客户账户自己。
func customerCoordinates(t *testing.T) adapter.SourceDataAmendmentRequestCoordinates {
	t.Helper()
	return adapter.SourceDataAmendmentRequestCoordinates{
		RequesterKind: pcdomain.CustomerAccountRequester,
		LegalEntity:   pcValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		Level:         pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
		Scope:         pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		Reason:        pcValue(t, pcdomain.NewStructuredReason, "CUSTOMER_CORRECTION"),
		Evidence:      pcValue(t, pcdomain.NewEvidenceReference, "evidence-1"),
		At:            amendmentAskedAt,
	}
}

// operatorCoordinates 与 customerCoordinates 同键，只把请求方换成代录的运营角色；所代的客户账户不在坐标里
// ——适配器取询问身份上的那一个。
func operatorCoordinates(t *testing.T, operator string) adapter.SourceDataAmendmentRequestCoordinates {
	t.Helper()
	coordinates := customerCoordinates(t)
	coordinates.RequesterKind = pcdomain.OperatorRoleRequester
	coordinates.OperatorRole = pcValue(t, pcdomain.NewOperatorRoleReference, operator)
	return coordinates
}

// amendmentGrant 造一条允许资料修订的已生效授权，键与 customerCoordinates 相同；借 rejectionGrant 的
// 版本与区间夹具，只换动作。
func amendmentGrant(t *testing.T, objectID string) pcdomain.AuthorityGrant {
	t.Helper()
	return rejectionGrant(t, objectID, pcdomain.SourceDataAmendmentAction)
}

func accountDelegator(t *testing.T, account string) pcdomain.Delegator {
	t.Helper()
	delegator, err := pcdomain.DelegatedByCustomerAccount(pcValue(t, pcdomain.NewCustomerAccountID, account))
	if err != nil {
		t.Fatalf("customer account delegator: %v", err)
	}
	return delegator
}

func legalEntityDelegator(t *testing.T, entity string) pcdomain.Delegator {
	t.Helper()
	delegator, err := pcdomain.DelegatedByLegalEntity(pcValue(t, pcdomain.NewLegalEntityReference, entity))
	if err != nil {
		t.Fatalf("legal entity delegator: %v", err)
	}
	return delegator
}

// amendmentDelegation 造一条已生效客户合同版本上的委派：委派方把 scope-a 上的资料修订决定权在 2026 年内
// 委派给持 level-commercial 的运营角色——与 amendmentGrant / customerCoordinates 同键。
func amendmentDelegation(t *testing.T, contractID string, delegator pcdomain.Delegator) pcdomain.ContractDelegation {
	t.Helper()
	interval, err := pcdomain.NewEffectiveInterval(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		pcValue(t, pcdomain.NewApprovalReference, "approval-"+contractID),
		pcValue(t, pcdomain.NewCommercialSourceReference, "source-"+contractID),
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	contract, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      pcValue(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          pcdomain.CustomerContractObject,
		ObjectID:      pcValue(t, pcdomain.NewCommercialObjectID, contractID),
		Version:       pcValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		ContentDigest: pcValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+contractID),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		EffectiveAt:   time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("rehydrate customer contract: %v", err)
	}
	delegation, err := pcdomain.NewContractDelegation(
		contract,
		delegator,
		pcdomain.SourceDataAmendmentAction,
		pcValue(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		pcValue(t, pcdomain.NewAuthorityLevel, "level-commercial"),
		interval,
	)
	if err != nil {
		t.Fatalf("contract delegation: %v", err)
	}
	return delegation
}
