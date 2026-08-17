package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// AdjudicateCommercialAuthorizationHandler 裁定一次商业授权询问（CONTEXT：主动拒绝
// 授权按责任法人、角色、适用范围和有效期间版本化，并要求结构化原因和证据）。
//
// 它是提供方公开口：消费方带着已构造的 AuthorizationRequest 来问「许不许」。装载
// grants 的端口是内部协作者，消费方不得绕过本编排自己判。
type AdjudicateCommercialAuthorizationHandler struct {
	grants ports.AuthorityGrantStore
}

func NewAdjudicateCommercialAuthorizationHandler(
	grants ports.AuthorityGrantStore,
) *AdjudicateCommercialAuthorizationHandler {
	return &AdjudicateCommercialAuthorizationHandler{grants: grants}
}

// Handle 按租户装载该范围该时点的现行授权，再交给 domain.Authorize。
//
// 最小身份不成立时不查询权威：一次已经发出的查询无法收回，它本身就回答了「这个范围
// 有没有规则」。装载失败包装上抛，不得冒充`不允许`或`未配置`；消费方把它留在 error
// 格。范围或时点折不成是消费方自己的事，不经过本编排。
func (handler *AdjudicateCommercialAuthorizationHandler) Handle(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.AuthorizationRequest,
) (domain.Authorization, error) {
	if tenant.String() == "" {
		return domain.Authorization{}, fmt.Errorf("adjudicate commercial authorization: tenant is required")
	}

	grants, err := handler.grants.LoadEffectiveGrants(ctx, tenant, request.Scope(), request.At())
	if err != nil {
		return domain.Authorization{}, fmt.Errorf("adjudicate commercial authorization: %w", err)
	}
	return domain.Authorize(grants, request)
}
