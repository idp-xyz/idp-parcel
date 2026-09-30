package partycommercial

import (
	"context"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CommercialEligibility 实现 nrports.CommercialEligibilityView：读闭包里已选出的
// 服务产品形态，译成要求/不要求（ADR-0050 / UC-NR-002 步骤 4）。
//
// 闭包标识由命令带来的、本轮已经采用的商业解析回指（ADR-0156），不再要一份判断键到
// 解析的实例映射。空引用答未配置，编排形成`未形成判断`——不代拟一个解析标识。
type CommercialEligibility struct {
	closures pcports.CommercialResolutionView
}

func NewCommercialEligibility(
	closures pcports.CommercialResolutionView,
) (*CommercialEligibility, error) {
	if closures == nil {
		return nil, fmt.Errorf("network routing partycommercial adapter: commercial resolution view is nil")
	}
	return &CommercialEligibility{closures: closures}, nil
}

var _ nrports.CommercialEligibilityView = (*CommercialEligibility)(nil)

func (adapter *CommercialEligibility) AssessNetworkEligibility(
	ctx context.Context,
	key nrdomain.ReachabilityJudgmentKey,
	resolution nrdomain.CommercialResolutionReference,
) (nrdomain.NetworkEligibility, error) {
	none := nrdomain.NetworkEligibility{}
	if !resolution.Valid() {
		return none, fmt.Errorf("%w: commercial resolution reference is not configured",
			ErrServiceProductUnavailable)
	}
	tenant, err := pcdomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolutionID, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return none, fmt.Errorf("%w: resolution: %v", ErrUntranslatableAnswer, err)
	}
	closure, loaded, err := adapter.closures.LoadResolution(ctx, tenant, resolutionID)
	if err != nil {
		return none, fmt.Errorf("load commercial closure: %w", err)
	}
	if !loaded {
		return none, fmt.Errorf("%w: commercial closure %q was not found",
			ErrServiceProductUnavailable, resolutionID)
	}
	if closure.ResolutionKey().TenantID != tenant {
		return none, fmt.Errorf("%w: identity %q closure %q",
			ErrRoutingClosureTenantMismatch, tenant, closure.ResolutionKey().TenantID)
	}
	return eligibilityFromClosure(closure)
}
