package partycommercial

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// ContinuedAttemptDecisionRequestCoordinates 是实例半边替一次关闭 / 重开询问补出的商业坐标：提出请求的运营角色、
// 责任法人、权限等级、商业范围、证据（`PAR-COM-13` / `PAR-COM-14`）。动作、租户、所代的客户账户、原因与时点不在
// 这里：动作由询问里的 Kind 钉死（关闭权与重开权是 PC 互不蕴含的两格，映射不得把一格读成另一格）；所代的客户账户
// 只能是询问身份上的那一个；原因与时点是询问自带的事实——让映射再给一份就有了第二个来源。
//
// 请求方一律是运营角色（pc-gaps/13 裁决 ③：例外支首发不开，货主只作 Requester 证据随查询进 PC）：客户账户请求
// 这两格由 PC 的 resolveDecider 显式拒（ErrCustomerAccountCannotDecide），本适配器不给映射一个「客户自己来的」种类。
type ContinuedAttemptDecisionRequestCoordinates struct {
	OperatorRole pcdomain.OperatorRoleReference
	LegalEntity  pcdomain.LegalEntityReference
	Level        pcdomain.AuthorityLevel
	Scope        pcdomain.CommercialScopeReference
	Evidence     pcdomain.EvidenceReference
}

// ContinuedAttemptDecisionAuthorizationRequestSource 替询问补出商业坐标。第二个返回值为 false 即「折不出来」——
// 适配器据以交回 error（未形成），不把缺映射译成授权规则未配置：后者是提供方对自己登记册的答案，缺映射是消费方
// 自己的缺口，两条续办路径不能混（与撤回 / 资料修订两只同一条理由）。
type ContinuedAttemptDecisionAuthorizationRequestSource interface {
	FormRequestCoordinates(
		ctx context.Context,
		query psports.ContinuedAttemptDecisionAuthorizationQuery,
	) (ContinuedAttemptDecisionRequestCoordinates, bool, error)
}

// ContinuedAttemptDecisionAuthorizationAdapter 把 parcel-shipment 的关闭 / 重开授权口接到 party-commercial 的裁定
// 编排上（ADR-0025），形状照 source_data_amendment_authorization.go。三件答复（授权依据快照 / 授权角色 / 实际决定方）
// 全部转写自 PC 的裁定，一处都不自判（ADR-0116 决定四）：pcdomain.Decider 没有公开构造器，PS 想编也编不出。
type ContinuedAttemptDecisionAuthorizationAdapter struct {
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler
	requests   ContinuedAttemptDecisionAuthorizationRequestSource
}

// NewContinuedAttemptDecisionAuthorizationAdapter 接的裁定编排用旧构造器即可：关闭与重开是运营侧凭授权规则自己作的
// 决定，决定方不经委派解出（PC `resolveDecider`），委派读口在这两格不会被问到。
//
// 两口一拒一不拒（票 label-channel/36 条 5）：`adjudicate` 为 nil 是装配缺件——没有裁定编排这只适配器什么都答不了，
// 拖到第一次询问才报只会把一处装配错报成一次业务失败，所以构造期就拒；`requests` 为 nil **不拒**，它是有意的
// 「显式未配置」——请求坐标映射属实例半边（`PAR-COM-13` / `PAR-COM-14`），生产装配今天就是 nil，询问到达时由
// AuthorizeContinuedAttemptDecision 如实答未形成，那一格是这只适配器的正当运行态而不是缺件。
func NewContinuedAttemptDecisionAuthorizationAdapter(
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler,
	requests ContinuedAttemptDecisionAuthorizationRequestSource,
) (*ContinuedAttemptDecisionAuthorizationAdapter, error) {
	if adjudicate == nil {
		return nil, errors.New("continued attempt decision authorization adapter: adjudication handler is nil")
	}
	return &ContinuedAttemptDecisionAuthorizationAdapter{adjudicate: adjudicate, requests: requests}, nil
}

var _ psports.ContinuedAttemptDecisionAuthorizer = (*ContinuedAttemptDecisionAuthorizationAdapter)(nil)

// AuthorizeContinuedAttemptDecision 问提供方这次关闭或重开许不许、许了的话三件是什么。
//
// 询问折不成商业坐标时停在未形成，且不去问提供方：一次已经发出的查询收不回来，它本身就回答了「这个范围有没有规则」。
// 提供方四格按恢复动作翻译：已授权带三件；不允许（含客户账户请求这两格的 ErrCustomerAccountCannotDecide——它 Is
// ErrNotAuthorized，恢复动作在业务侧）；未配置；其余 error。
func (adapter *ContinuedAttemptDecisionAuthorizationAdapter) AuthorizeContinuedAttemptDecision(
	ctx context.Context,
	query psports.ContinuedAttemptDecisionAuthorizationQuery,
) (psports.ContinuedAttemptDecisionAuthorization, error) {
	none := psports.ContinuedAttemptDecisionAuthorization{}
	if adapter.requests == nil {
		return none, fmt.Errorf(
			"authorize continued attempt decision: authorization request mapping is not configured")
	}
	action, err := authorizedActionOf(query.Kind)
	if err != nil {
		return none, fmt.Errorf("authorize continued attempt decision: %w", err)
	}
	coordinates, formed, err := adapter.requests.FormRequestCoordinates(ctx, query)
	if err != nil {
		return none, fmt.Errorf("authorize continued attempt decision: %w", err)
	}
	if !formed {
		return none, fmt.Errorf(
			"authorize continued attempt decision: commercial coordinates are not formed")
	}

	tenant, err := pcdomain.NewTenantID(query.Identity.TenantID().String())
	if err != nil {
		return none, fmt.Errorf("authorize continued attempt decision: %w", err)
	}
	request, err := formContinuedAttemptDecisionRequest(query, action, coordinates)
	if err != nil {
		return none, fmt.Errorf("authorize continued attempt decision: %w", err)
	}

	authorized, err := adapter.adjudicate.Handle(ctx, tenant, request)
	if err != nil {
		switch {
		case errors.Is(err, pcdomain.ErrAuthorityRulesNotConfigured):
			return psports.ContinuedAttemptDecisionAuthorization{
				Outcome: psports.AuthorizationRulesNotConfigured,
			}, nil
		case errors.Is(err, pcdomain.ErrNotAuthorized):
			return psports.ContinuedAttemptDecisionAuthorization{
				Outcome: psports.AuthorizationRefused,
			}, nil
		default:
			return none, fmt.Errorf("authorize continued attempt decision: %w", err)
		}
	}

	authority, err := psdomain.NewContinuedAttemptAuthoritySnapshot(
		authorized.GrantVersion().ObjectID().String() + "/" + authorized.GrantVersion().Version().String())
	if err != nil {
		return none, fmt.Errorf("%w: grant version: %v", ErrUntranslatableAnswer, err)
	}
	// 授权角色取所采用 grant 的权限等级。PC 的 Authorization 只交回 grant 的版本引用不交回等级，而命中的 grant
	// 与请求在等级上逐字相等（PC `permits` 的判据之一）——所以请求上的等级就是 grant 的等级，不是 PS 自报的一格。
	// **这是暂行取法**（票 label-channel/30 裁决 ③）：它成立只因 PC 今天按等级逐字相等命中；PC 若改成「同级或更高」
	// 命中，请求上的等级就不再等于 grant 的等级，届时要 PC 在 Authorization 上交回等级，本处随之改读那一格。
	authorityRole, err := psdomain.NewContinuedAttemptAuthorityRoleReference(request.Level().String())
	if err != nil {
		return none, fmt.Errorf("%w: authority level: %v", ErrUntranslatableAnswer, err)
	}
	// 决定方缺席只可能来自旧构造器形成的请求，而本适配器只走带请求方的构造器；真出现就是提供方的答复与本口的
	// 契约对不上，交 error 让人看，不拿请求方顶上——那正是端口头注禁的事。
	decider, named := authorized.Decider()
	if !named {
		return none, fmt.Errorf("%w: provider granted without naming the decider", ErrUntranslatableAnswer)
	}
	deciderReference, err := psdomain.NewDeciderReference(decider.Reference())
	if err != nil {
		return none, fmt.Errorf("%w: decider: %v", ErrUntranslatableAnswer, err)
	}
	return psports.ContinuedAttemptDecisionAuthorization{
		Outcome:       psports.AuthorizationGranted,
		Authority:     authority,
		AuthorityRole: authorityRole,
		Decider:       deciderReference,
	}, nil
}

// authorizedActionOf 把询问的决定种类钉成 PC 的动作格。逐取值分派、不留兜底：两格互不蕴含，认不出的种类是编程
// 错误，不能落到任何一格上。
func authorizedActionOf(kind psdomain.ContinuedAttemptDecisionKind) (pcdomain.AuthorizedAction, error) {
	switch kind {
	case psdomain.ControlledClosureDecision:
		return pcdomain.ControlledClosureAction, nil
	case psdomain.ReopeningDecision:
		return pcdomain.ReopeningAction, nil
	default:
		return pcdomain.AuthorizedActionInvalid, fmt.Errorf("continued attempt decision kind %d has no authorized action", kind)
	}
}

// formContinuedAttemptDecisionRequest 把询问与实例坐标折成带关闭 / 重开动作的提供方请求。请求方是坐标里的运营角色代
// 询问身份上的客户账户；原因与时点取自询问（它们是请求自带的事实，不由映射补）。
func formContinuedAttemptDecisionRequest(
	query psports.ContinuedAttemptDecisionAuthorizationQuery,
	action pcdomain.AuthorizedAction,
	coordinates ContinuedAttemptDecisionRequestCoordinates,
) (pcdomain.AuthorizationRequest, error) {
	account, err := pcdomain.NewCustomerAccountID(query.Identity.CustomerAccountID().String())
	if err != nil {
		return pcdomain.AuthorizationRequest{}, err
	}
	requester, err := pcdomain.RequestedByOperatorRole(coordinates.OperatorRole, account)
	if err != nil {
		return pcdomain.AuthorizationRequest{}, err
	}
	reason, err := pcdomain.NewStructuredReason(query.Reason.String())
	if err != nil {
		return pcdomain.AuthorizationRequest{}, err
	}
	return pcdomain.NewAuthorizationRequestBy(
		requester,
		action,
		coordinates.LegalEntity,
		coordinates.Level,
		coordinates.Scope,
		reason,
		coordinates.Evidence,
		query.At,
	)
}
