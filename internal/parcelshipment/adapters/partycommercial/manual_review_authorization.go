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

// ManualReviewAuthorizationRequestSource 把复核授权询问折成提供方的 AuthorizationRequest，
// 形状与 ActiveRejectionRequestSource 同一条理由。
//
// 法人、权限等级、商业范围、结构化原因与业务时点都是实例半边（`PAR-COM-14`）：没有租户时
// 谁也说不出这次复核该在哪个范围、以哪一等权限去问、原因目录里哪一条是「复核完成」。第二个
// 返回值为 false 即「折不出来」——适配器据以交回 error（未形成），不把缺映射译成授权规则未配置。
type ManualReviewAuthorizationRequestSource interface {
	FormAuthorizationRequest(
		ctx context.Context,
		query psports.ManualReviewAuthorizationQuery,
	) (pcdomain.AuthorizationRequest, bool, error)
}

// ManualReviewAuthorizationAdapter 把 parcel-shipment 的复核授权口接到 party-commercial 的
// 裁定编排上（ADR-0025），与主动拒绝适配器同形——UC-PC-003 第四项的裁决：「谁有权复核」不另立
// 谓词，走同一个 Authorize 带 ManualReviewAction。
type ManualReviewAuthorizationAdapter struct {
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler
	requests   ManualReviewAuthorizationRequestSource
}

func NewManualReviewAuthorizationAdapter(
	adjudicate *pcapplication.AdjudicateCommercialAuthorizationHandler,
	requests ManualReviewAuthorizationRequestSource,
) *ManualReviewAuthorizationAdapter {
	return &ManualReviewAuthorizationAdapter{adjudicate: adjudicate, requests: requests}
}

var _ psports.ManualReviewAuthorizer = (*ManualReviewAuthorizationAdapter)(nil)

// AuthorizeManualReview 问提供方这位复核人许不许为这份提交版本完成复核。
//
// 询问折不成提供方请求时停在未形成：缺的是范围/时点/原因映射，不是授权规则本身。
// 提供方四格按恢复动作翻译：已授权带所采用版本引用；不允许；未配置；其余 error。
//
// 映射折出的请求动作必须是 ManualReviewAction。这一道守卫主动拒绝那只适配器没有，这里加上
// 是因为本口的全部意义就在动作上：一份映射若折出了拒绝动作，一条只授拒绝权的规则就会被读成
// 复核权——获准拒单不等于获准复核（PC CONTEXT），而那正是「谁有权复核」要问的东西。
func (adapter *ManualReviewAuthorizationAdapter) AuthorizeManualReview(
	ctx context.Context,
	query psports.ManualReviewAuthorizationQuery,
) (psports.ManualReviewAuthorization, error) {
	if adapter.requests == nil {
		return psports.ManualReviewAuthorization{}, fmt.Errorf(
			"authorize manual review: authorization request mapping is not configured")
	}
	request, formed, err := adapter.requests.FormAuthorizationRequest(ctx, query)
	if err != nil {
		return psports.ManualReviewAuthorization{}, fmt.Errorf("authorize manual review: %w", err)
	}
	if !formed {
		return psports.ManualReviewAuthorization{}, fmt.Errorf(
			"authorize manual review: commercial scope, as-of time or structured reason is not formed")
	}
	if request.Action() != pcdomain.ManualReviewAction {
		return psports.ManualReviewAuthorization{}, fmt.Errorf(
			"%w: request mapping formed action %q, want MANUAL_REVIEW", ErrUntranslatableAnswer, request.Action())
	}

	tenant, err := pcdomain.NewTenantID(query.Identity.TenantID().String())
	if err != nil {
		return psports.ManualReviewAuthorization{}, fmt.Errorf("authorize manual review: %w", err)
	}

	authorized, err := adapter.adjudicate.Handle(ctx, tenant, request)
	if err != nil {
		switch {
		case errors.Is(err, pcdomain.ErrAuthorityRulesNotConfigured):
			return psports.ManualReviewAuthorization{
				Outcome: psports.AuthorizationRulesNotConfigured,
			}, nil
		case errors.Is(err, pcdomain.ErrNotAuthorized):
			return psports.ManualReviewAuthorization{
				Outcome: psports.AuthorizationRefused,
			}, nil
		default:
			return psports.ManualReviewAuthorization{}, fmt.Errorf("authorize manual review: %w", err)
		}
	}

	reference, err := psdomain.NewReviewAuthorityReference(
		authorized.GrantVersion().ObjectID().String() + "/" + authorized.GrantVersion().Version().String())
	if err != nil {
		return psports.ManualReviewAuthorization{}, fmt.Errorf("%w: grant version: %v", ErrUntranslatableAnswer, err)
	}
	return psports.ManualReviewAuthorization{
		Outcome:   psports.AuthorizationGranted,
		Authority: reference,
	}, nil
}
