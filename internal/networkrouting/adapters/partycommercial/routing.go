package partycommercial

import (
	"context"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// RoutingClosureIdentity 把一次初始路由判断键折成已经唯一解析的闭包标识。纪律与
// ReachabilityClosureIdentity 相同：未配置交回错误，不读成不适用。
type RoutingClosureIdentity interface {
	ResolutionID(ctx context.Context, key nrdomain.InitialRouteJudgmentKey) (pcdomain.ResolutionID, bool, error)
}

// RoutingApplicability 实现 nrports.RoutingApplicabilityView：同一份形态翻译表
// （UC-NR-001 步骤 3 / ADR-0050）。
type RoutingApplicability struct {
	closures   pcports.CommercialResolutionStore
	identities RoutingClosureIdentity
}

func NewRoutingApplicability(
	closures pcports.CommercialResolutionStore,
	identities RoutingClosureIdentity,
) (*RoutingApplicability, error) {
	if closures == nil {
		return nil, fmt.Errorf("network routing partycommercial adapter: commercial resolution store is nil")
	}
	return &RoutingApplicability{closures: closures, identities: identities}, nil
}

var _ nrports.RoutingApplicabilityView = (*RoutingApplicability)(nil)

func (adapter *RoutingApplicability) AssessRoutingApplicability(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
) (nrdomain.NetworkEligibility, error) {
	if adapter.identities == nil {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: routing closure identity is not configured", ErrServiceProductUnavailable)
	}
	resolutionID, found, err := adapter.identities.ResolutionID(ctx, key)
	if err != nil {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: identify commercial closure: %v", ErrServiceProductUnavailable, err)
	}
	if !found {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: routing closure identity is not configured", ErrServiceProductUnavailable)
	}
	tenant, err := pcdomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: tenant: %v", ErrServiceProductUnavailable, err)
	}
	closure, loaded, err := adapter.closures.LoadResolution(ctx, tenant, resolutionID)
	if err != nil {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: load commercial closure: %v", ErrServiceProductUnavailable, err)
	}
	if !loaded {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: commercial closure %q was not found", ErrServiceProductUnavailable, resolutionID)
	}
	return eligibilityFromClosure(closure)
}
