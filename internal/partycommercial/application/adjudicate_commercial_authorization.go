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
// 它是提供方公开口：消费方带着已构造的 AuthorizationRequest 来问「许不许」，并从答复里
// 读「实际决定方是谁」（ADR-0116 Decision 三）。装载 grants 与合同委派的端口都是内部协作者，
// 消费方不得绕过本编排自己判。
type AdjudicateCommercialAuthorizationHandler struct {
	grants      ports.AuthorityGrantStore
	delegations ports.EffectiveContractDelegationView
}

// NewAdjudicateCommercialAuthorizationHandler 只接授权治理册，是三步法里保留的旧构造器：
// parcel-shipment 两只既有适配器的测试与 cmd/parcel-api 两处装配仍走这里，迁到
// NewAdjudicateCommercialAuthorizationHandlerWithDelegations 之后删。
//
// 经它装配的编排没有委派读口：运营角色代客户请求资料修订时停在 error（委派读口未接线），
// 不答许也不答拒——那一格不是「没有委派」，是「没人问过委派」，两者要人做的事相反。
func NewAdjudicateCommercialAuthorizationHandler(
	grants ports.AuthorityGrantStore,
) *AdjudicateCommercialAuthorizationHandler {
	return &AdjudicateCommercialAuthorizationHandler{grants: grants}
}

// NewAdjudicateCommercialAuthorizationHandlerWithDelegations 是接满两个读口的构造器。资料修订
// 那一格的裁定要它——委派读口缺席时那一格答不出实际决定方。
func NewAdjudicateCommercialAuthorizationHandlerWithDelegations(
	grants ports.AuthorityGrantStore,
	delegations ports.EffectiveContractDelegationView,
) *AdjudicateCommercialAuthorizationHandler {
	return &AdjudicateCommercialAuthorizationHandler{grants: grants, delegations: delegations}
}

// Handle 按租户装载该范围该时点的现行授权与有效委派，再交给 domain.Authorize。
//
// 最小身份不成立时不查询权威：一次已经发出的查询无法收回，它本身就回答了「这个范围
// 有没有规则」。装载失败包装上抛，不得冒充`不允许`或`未配置`；消费方把它留在 error
// 格。范围或时点折不成是消费方自己的事，不经过本编排。
//
// 委派只在要用到它时装载：客户自己请求、或运营角色请求它自己拥有的动作，决定方不经委派
// 解出，不为它多问一次库。旧构造器装配的编排走到要委派的那一格，报错而不是装成空切片——
// 空切片会被 Authorize 读成「客户没委派」。
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

	var delegations []domain.ContractDelegation
	if request.ResolvesDeciderByDelegation() {
		if handler.delegations == nil {
			return domain.Authorization{}, fmt.Errorf(
				"adjudicate commercial authorization: contract delegation view is not wired; an operator role cannot be adjudicated on a customer-owned action")
		}
		delegations, err = handler.delegations.LoadEffectiveDelegations(ctx, tenant, request.Scope(), request.At())
		if err != nil {
			return domain.Authorization{}, fmt.Errorf("adjudicate commercial authorization: %w", err)
		}
	}
	return domain.Authorize(grants, delegations, request)
}
