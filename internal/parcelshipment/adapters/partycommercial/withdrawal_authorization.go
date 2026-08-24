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

// WithdrawalAuthorizationRequestSource 把撤回询问折成提供方的 AuthorizationRequest，
// 形状与 ActiveRejectionRequestSource 同一条理由。
//
// 法人、权限等级、商业范围、证据引用、业务时点与**动作种类**都是实例半边：客户及其授权
// 代表的撤回授权是 `PAR-COM-14` 待提供的参数，PC 的 AuthorizedAction 今天也没有撤回动作
// ——扩它属那笔参数落地时的 PC 侧工作（按 AGENTS 先改 PC CONTEXT 再动代码），本适配器
// 只转交 RequestSource 声明的请求，不代它挑动作。第二个返回值为 false 即「折不出来」——
// 适配器据以交回 error（未形成），不把缺映射译成授权规则未配置。
type WithdrawalAuthorizationRequestSource interface {
	FormAuthorizationRequest(
		ctx context.Context,
		query psports.WithdrawalAuthorizationQuery,
	) (pcdomain.AuthorizationRequest, bool, error)
}

// WithdrawalAuthorizationAdapter 把 parcel-shipment 的撤回授权口接到 party-commercial
// 的裁定编排上（ADR-0025），与主动拒绝适配器同形。两口不合并：一个问的是货主客户或其
// 授权代表，另一个问的是运营侧授权角色（理由钉在 psports.WithdrawalAuthorizer 上）。
type WithdrawalAuthorizationAdapter struct {
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler
	requests   WithdrawalAuthorizationRequestSource
}

func NewWithdrawalAuthorizationAdapter(
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler,
	requests WithdrawalAuthorizationRequestSource,
) *WithdrawalAuthorizationAdapter {
	return &WithdrawalAuthorizationAdapter{adjudicate: adjudicate, requests: requests}
}

var _ psports.WithdrawalAuthorizer = (*WithdrawalAuthorizationAdapter)(nil)

// AuthorizeWithdrawal 问提供方这次撤回许不许。
//
// 询问折不成提供方请求时停在未形成：缺的是范围/时点映射，不是授权规则本身。
// 提供方四格按恢复动作翻译：已授权带所采用版本引用；不允许；未配置；其余 error。
func (adapter *WithdrawalAuthorizationAdapter) AuthorizeWithdrawal(
	ctx context.Context,
	query psports.WithdrawalAuthorizationQuery,
) (psports.WithdrawalAuthorization, error) {
	if adapter.requests == nil {
		return psports.WithdrawalAuthorization{}, fmt.Errorf(
			"authorize withdrawal: authorization request mapping is not configured")
	}
	request, formed, err := adapter.requests.FormAuthorizationRequest(ctx, query)
	if err != nil {
		return psports.WithdrawalAuthorization{}, fmt.Errorf("authorize withdrawal: %w", err)
	}
	if !formed {
		return psports.WithdrawalAuthorization{}, fmt.Errorf(
			"authorize withdrawal: commercial scope or as-of time is not formed")
	}

	tenant, err := pcdomain.NewTenantID(query.Identity.TenantID().String())
	if err != nil {
		return psports.WithdrawalAuthorization{}, fmt.Errorf("authorize withdrawal: %w", err)
	}

	authorized, err := adapter.adjudicate.Handle(ctx, tenant, request)
	if err != nil {
		switch {
		case errors.Is(err, pcdomain.ErrAuthorityRulesNotConfigured):
			return psports.WithdrawalAuthorization{
				Outcome: psports.AuthorizationRulesNotConfigured,
			}, nil
		case errors.Is(err, pcdomain.ErrNotAuthorized):
			return psports.WithdrawalAuthorization{
				Outcome: psports.AuthorizationRefused,
			}, nil
		default:
			return psports.WithdrawalAuthorization{}, fmt.Errorf("authorize withdrawal: %w", err)
		}
	}

	reference, err := psdomain.NewWithdrawalAuthorityReference(
		authorized.GrantVersion().ObjectID().String() + "/" + authorized.GrantVersion().Version().String())
	if err != nil {
		return psports.WithdrawalAuthorization{}, fmt.Errorf("%w: grant version: %v", ErrUntranslatableAnswer, err)
	}
	return psports.WithdrawalAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: reference,
	}, nil
}
