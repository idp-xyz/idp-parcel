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

// ActiveRejectionRequestSource 把消费方询问折成提供方的 AuthorizationRequest。
//
// 法人、权限等级、商业范围、证据引用与业务时点都是实例半边：没有租户时谁也说不出
// 这次拒绝该在哪个范围、以哪一等权限去问。第二个返回值为 false 即「折不出来」——
// 适配器据以交回 error（未形成），不把缺映射译成授权规则未配置。
type ActiveRejectionRequestSource interface {
	FormAuthorizationRequest(
		ctx context.Context,
		query psports.ActiveRejectionAuthorizationQuery,
	) (pcdomain.AuthorizationRequest, bool, error)
}

// ActiveRejectionAdapter 把 parcel-shipment 的主动拒绝授权口接到 party-commercial
// 的裁定编排上（ADR-0025）。
type ActiveRejectionAdapter struct {
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler
	requests   ActiveRejectionRequestSource
}

func NewActiveRejectionAdapter(
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler,
	requests ActiveRejectionRequestSource,
) *ActiveRejectionAdapter {
	return &ActiveRejectionAdapter{adjudicate: adjudicate, requests: requests}
}

var _ psports.ActiveRejectionAuthorizer = (*ActiveRejectionAdapter)(nil)

// AuthorizeActiveRejection 问提供方这次主动拒绝许不许。
//
// 询问折不成提供方请求时停在未形成：缺的是范围/时点映射，不是授权规则本身。
// 提供方四格按恢复动作翻译：已授权带所采用版本引用；不允许；未配置；其余 error。
func (adapter *ActiveRejectionAdapter) AuthorizeActiveRejection(
	ctx context.Context,
	query psports.ActiveRejectionAuthorizationQuery,
) (psports.ActiveRejectionAuthorization, error) {
	if adapter.requests == nil {
		return psports.ActiveRejectionAuthorization{}, fmt.Errorf(
			"authorize active rejection: authorization request mapping is not configured")
	}
	request, formed, err := adapter.requests.FormAuthorizationRequest(ctx, query)
	if err != nil {
		return psports.ActiveRejectionAuthorization{}, fmt.Errorf("authorize active rejection: %w", err)
	}
	if !formed {
		return psports.ActiveRejectionAuthorization{}, fmt.Errorf(
			"authorize active rejection: commercial scope or as-of time is not formed")
	}

	tenant, err := pcdomain.NewTenantID(query.Identity.TenantID().String())
	if err != nil {
		return psports.ActiveRejectionAuthorization{}, fmt.Errorf("authorize active rejection: %w", err)
	}

	authorized, err := adapter.adjudicate.Handle(ctx, tenant, request)
	if err != nil {
		switch {
		case errors.Is(err, pcdomain.ErrAuthorityRulesNotConfigured):
			return psports.ActiveRejectionAuthorization{
				Outcome: psports.AuthorizationRulesNotConfigured,
			}, nil
		case errors.Is(err, pcdomain.ErrNotAuthorized):
			return psports.ActiveRejectionAuthorization{
				Outcome: psports.AuthorizationRefused,
			}, nil
		default:
			return psports.ActiveRejectionAuthorization{}, fmt.Errorf("authorize active rejection: %w", err)
		}
	}

	reference, err := psdomain.NewRejectionAuthorityReference(
		authorized.GrantVersion().ObjectID().String() + "/" + authorized.GrantVersion().Version().String())
	if err != nil {
		return psports.ActiveRejectionAuthorization{}, fmt.Errorf("%w: grant version: %v", ErrUntranslatableAnswer, err)
	}
	return psports.ActiveRejectionAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: reference,
	}, nil
}
