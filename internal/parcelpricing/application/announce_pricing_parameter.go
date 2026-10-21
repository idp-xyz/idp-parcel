package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// announcePricingParameter 按受影响的各个计价范围与方向交「参数已登记」，一面一封（ADR-0229 决定五）。内部方向的面不交：
// 结算的评价请求只有 BUY 与 SELL 两路，内部价由网络路由读成本，没有等着它的评价请求。交接失败上抛，信封与登记或复核
// 行同生共死。
func announcePricingParameter(
	ctx context.Context,
	handoff ports.PricingParameterRegisteredHandoff,
	tenant domain.TenantID,
	kind ports.PricingParameterKind,
	registration domain.VersionReference,
	scopes []ports.PricedScope,
) error {
	for _, scope := range scopes {
		if scope.Direction != domain.PricingDirectionBuy && scope.Direction != domain.PricingDirectionSell {
			continue
		}
		if err := handoff.HandOffPricingParameterRegistered(ctx, ports.PricingParameterRegisteredIntent{
			Tenant:       tenant,
			Kind:         kind,
			Registration: registration,
			Scope:        scope.Scope,
			Direction:    scope.Direction,
		}); err != nil {
			return err
		}
	}
	return nil
}

// announceBoundPricingParameter 先问这一版序列或目录绑在哪些卡上，再按那几张卡的范围与方向交。没有卡绑它就一封不交：
// 卡没绑它，问那张卡的请求就不是在等它——绑它的卡后来登记了，由那一次卡登记续办。
func announceBoundPricingParameter(
	ctx context.Context,
	bindings ports.PricingParameterBindingView,
	handoff ports.PricingParameterRegisteredHandoff,
	tenant domain.TenantID,
	kind ports.PricingParameterKind,
	registration domain.VersionReference,
) error {
	scopes, err := bindings.FindPricedScopesBinding(ctx, tenant, kind, registration.ID())
	if err != nil {
		return fmt.Errorf("find cards binding %s %s: %w", kind, registration.ID(), err)
	}
	return announcePricingParameter(ctx, handoff, tenant, kind, registration, scopes)
}
