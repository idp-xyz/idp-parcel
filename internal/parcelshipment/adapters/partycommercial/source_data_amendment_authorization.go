package partycommercial

import (
	"context"
	"errors"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// SourceDataAmendmentRequestCoordinates 是实例半边替一次资料修订询问补出的商业坐标。
//
// 与撤回那只的 RequestSource 不同，这里交回的不是整份 AuthorizationRequest 而是它的实例半边：
// 责任法人、权限等级、商业范围、结构化原因、证据、业务时点（`PAR-COM-14`），以及 PS 那个不透明的
// RequesterReference 在提供方眼里**是谁**——客户账户自己，还是代录的运营角色（`BD-PS-009`
// 「请求方的采信」与「代录角色」）。动作、租户与所代的客户账户不在这里：动作只能是资料修订，
// 所代的客户账户只能是询问身份上的那一个，两者由适配器钉死，映射改不了——否则一份映射就能把
// 只授复核权或拒绝权的规则读成修订权，或替别的客户提请求。
//
// RequesterKind 取 PC 的两格；OperatorRole 只在运营角色代录时读。用 Kind 而不用「OperatorRole 为空即
// 客户自己」，理由与 PC 两个请求方构造器分立的理由相同：空值分不清「客户自己来的」与「忘了填运营角色」。
type SourceDataAmendmentRequestCoordinates struct {
	RequesterKind pcdomain.RequesterKind
	OperatorRole  pcdomain.OperatorRoleReference
	LegalEntity   pcdomain.LegalEntityReference
	Level         pcdomain.AuthorityLevel
	Scope         pcdomain.CommercialScopeReference
	Reason        pcdomain.StructuredReason
	Evidence      pcdomain.EvidenceReference
	At            time.Time
}

// SourceDataAmendmentAuthorizationRequestSource 替询问补出商业坐标。第二个返回值为 false 即「折不出来」
// ——适配器据以交回 error（未形成），不把缺映射译成授权规则未配置：后者是提供方对自己登记册的答案，
// 缺映射是消费方自己的缺口，两条续办路径不能混。
type SourceDataAmendmentAuthorizationRequestSource interface {
	FormRequestCoordinates(
		ctx context.Context,
		query psports.SourceDataAmendmentAuthorizationQuery,
	) (SourceDataAmendmentRequestCoordinates, bool, error)
}

// SourceDataAmendmentAuthorizationAdapter 把 parcel-shipment 的资料修订授权口接到 party-commercial 的
// 裁定编排上（ADR-0025），与撤回、拒绝两只同形。多出的一格是实际决定方：本适配器**只转写 PC 交回的
// 决定方**，一处都不自判——请求方是客户还是代录的运营角色、代录时决定权经哪条合同委派落在谁身上，
// 都是 PC 在 Authorize 里解出的（ADR-0116 Decision 三）；PS 端口头注禁「调用方声明」，在这里是结构性的：
// pcdomain.Decider 没有公开构造器，PS 想编也编不出。
type SourceDataAmendmentAuthorizationAdapter struct {
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler
	requests   SourceDataAmendmentAuthorizationRequestSource
}

// NewSourceDataAmendmentAuthorizationAdapter 接的裁定编排必须带委派读口（带委派那只构造器）：
// 运营角色代录那一格没有它答不出实际决定方，编排会报错——错会原样落在 error 格，不装成拒绝。
func NewSourceDataAmendmentAuthorizationAdapter(
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler,
	requests SourceDataAmendmentAuthorizationRequestSource,
) *SourceDataAmendmentAuthorizationAdapter {
	return &SourceDataAmendmentAuthorizationAdapter{adjudicate: adjudicate, requests: requests}
}

var _ psports.SourceDataAmendmentAuthorizer = (*SourceDataAmendmentAuthorizationAdapter)(nil)

// AuthorizeSourceDataAmendment 问提供方这次资料修订许不许、许了的话实际决定方是谁。
//
// 询问折不成商业坐标时停在未形成：缺的是坐标映射，不是授权规则本身。提供方四格按恢复动作翻译：
// 已授权带所采用版本引用与 PC 解出的决定方；不允许（含 grant 在场而委派缺席的 ErrDelegationAbsent
// ——它 Is ErrNotAuthorized，恢复动作同在业务侧）；未配置；其余 error。
func (adapter *SourceDataAmendmentAuthorizationAdapter) AuthorizeSourceDataAmendment(
	ctx context.Context,
	query psports.SourceDataAmendmentAuthorizationQuery,
) (psports.SourceDataAmendmentAuthorization, error) {
	none := psports.SourceDataAmendmentAuthorization{}
	if adapter.requests == nil {
		return none, fmt.Errorf(
			"authorize source data amendment: authorization request mapping is not configured")
	}
	coordinates, formed, err := adapter.requests.FormRequestCoordinates(ctx, query)
	if err != nil {
		return none, fmt.Errorf("authorize source data amendment: %w", err)
	}
	if !formed {
		return none, fmt.Errorf(
			"authorize source data amendment: commercial coordinates are not formed")
	}

	tenant, err := pcdomain.NewTenantID(query.Identity.TenantID().String())
	if err != nil {
		return none, fmt.Errorf("authorize source data amendment: %w", err)
	}
	request, err := formSourceDataAmendmentRequest(query, coordinates)
	if err != nil {
		return none, fmt.Errorf("authorize source data amendment: %w", err)
	}

	authorized, err := adapter.adjudicate.Handle(ctx, tenant, request)
	if err != nil {
		switch {
		case errors.Is(err, pcdomain.ErrAuthorityRulesNotConfigured):
			return psports.SourceDataAmendmentAuthorization{
				Outcome: psports.AuthorizationRulesNotConfigured,
			}, nil
		case errors.Is(err, pcdomain.ErrNotAuthorized):
			return psports.SourceDataAmendmentAuthorization{
				Outcome: psports.AuthorizationRefused,
			}, nil
		default:
			return none, fmt.Errorf("authorize source data amendment: %w", err)
		}
	}

	authority, err := psdomain.NewAmendmentAuthoritySnapshot(
		authorized.GrantVersion().ObjectID().String() + "/" + authorized.GrantVersion().Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: grant version: %v", ErrUntranslatableAnswer, err)
	}
	// 决定方缺席只可能来自旧构造器形成的请求，而本适配器只走带请求方的构造器；真出现就是提供方的
	// 答复与本口的契约对不上，交 error 让人看，不拿请求方顶上——那正是端口头注禁的事。
	decider, named := authorized.Decider()
	if !named {
		return none, fmt.Errorf("%w: provider granted without naming the decider", ErrUntranslatableAnswer)
	}
	// PS 的 DeciderReference 是单一引用串，只搬 Reference 不搬 Kind：PS 不按决定方种类分流，
	// 客户账户 / 责任法人的分类留在 PC。
	deciderReference, err := psdomain.NewDeciderReference(decider.Reference())
	if err != nil {
		return none, fmt.Errorf("%w: decider: %v", ErrUntranslatableAnswer, err)
	}
	return psports.SourceDataAmendmentAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: authority,
		Decider:   deciderReference,
	}, nil
}

// formSourceDataAmendmentRequest 把询问与实例坐标折成带资料修订动作的提供方请求。动作与所代客户账户
// 由本函数钉死（理由见 SourceDataAmendmentRequestCoordinates）；请求方按坐标里的种类经 PC 的两个构造器
// 形成——客户账户自己，或该运营角色代询问身份上的客户账户。
func formSourceDataAmendmentRequest(
	query psports.SourceDataAmendmentAuthorizationQuery,
	coordinates SourceDataAmendmentRequestCoordinates,
) (pcdomain.AuthorizationRequest, error) {
	account, err := pcdomain.NewCustomerAccountID(query.Identity.CustomerAccountID().String())
	if err != nil {
		return pcdomain.AuthorizationRequest{}, err
	}
	var requester pcdomain.AuthorizationRequester
	switch coordinates.RequesterKind {
	case pcdomain.CustomerAccountRequester:
		requester, err = pcdomain.RequestedByCustomerAccount(account)
	case pcdomain.OperatorRoleRequester:
		requester, err = pcdomain.RequestedByOperatorRole(coordinates.OperatorRole, account)
	default:
		return pcdomain.AuthorizationRequest{}, fmt.Errorf(
			"request mapping did not say whether the customer account or an operator role is requesting")
	}
	if err != nil {
		return pcdomain.AuthorizationRequest{}, err
	}
	return pcdomain.NewAuthorizationRequestBy(
		requester,
		pcdomain.SourceDataAmendmentAction,
		coordinates.LegalEntity,
		coordinates.Level,
		coordinates.Scope,
		coordinates.Reason,
		coordinates.Evidence,
		coordinates.At,
	)
}
