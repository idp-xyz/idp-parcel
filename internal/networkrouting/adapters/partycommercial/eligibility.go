package partycommercial

import (
	"context"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ReachabilityClosureIdentity 把一次可达性判断键折成已经唯一解析的闭包标识。
//
// 实例半边：范围映射属试点参数。nil 或 found=false 都是「显式未配置」，本适配器交回
// 错误，由编排形成`未形成判断`——不代拟一个解析标识，也不把缺席读成不要求判断。
type ReachabilityClosureIdentity interface {
	ResolutionID(ctx context.Context, key nrdomain.ReachabilityJudgmentKey) (pcdomain.ResolutionID, bool, error)
}

// CommercialEligibility 实现 nrports.CommercialEligibilityView：读闭包里已选出的
// 服务产品形态，译成要求/不要求（ADR-0050 / UC-NR-002 步骤 4）。
type CommercialEligibility struct {
	closures   pcports.CommercialResolutionStore
	identities ReachabilityClosureIdentity
}

func NewCommercialEligibility(
	closures pcports.CommercialResolutionStore,
	identities ReachabilityClosureIdentity,
) (*CommercialEligibility, error) {
	if closures == nil {
		return nil, fmt.Errorf("network routing partycommercial adapter: commercial resolution store is nil")
	}
	return &CommercialEligibility{closures: closures, identities: identities}, nil
}

var _ nrports.CommercialEligibilityView = (*CommercialEligibility)(nil)

func (adapter *CommercialEligibility) AssessNetworkEligibility(
	ctx context.Context,
	key nrdomain.ReachabilityJudgmentKey,
) (nrdomain.NetworkEligibility, error) {
	if adapter.identities == nil {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: reachability closure identity is not configured", ErrServiceProductUnavailable)
	}
	resolutionID, found, err := adapter.identities.ResolutionID(ctx, key)
	if err != nil {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: identify commercial closure: %v", ErrServiceProductUnavailable, err)
	}
	if !found {
		return nrdomain.NetworkEligibility{}, fmt.Errorf("%w: reachability closure identity is not configured", ErrServiceProductUnavailable)
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
