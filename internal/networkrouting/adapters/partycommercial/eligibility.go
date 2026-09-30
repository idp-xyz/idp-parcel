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
// 闭包标识由命令带来的、本轮已经采用的商业解析回指（ADR-0156）。空引用答未配置，
// 编排形成`未形成判断`——不代拟一个解析标识。
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
	closure, err := loadClosure(ctx, adapter.closures, key.TenantID.String(), resolution)
	if err != nil {
		return nrdomain.NetworkEligibility{}, err
	}
	return eligibilityFromClosure(closure)
}

// loadClosure 是两条资格视图共用的取闭包。形态翻译表已经是一份；取闭包再各写一份，
// 下一次只会改到其中一条。
func loadClosure(
	ctx context.Context,
	closures pcports.CommercialResolutionView,
	tenantRaw string,
	resolution nrdomain.CommercialResolutionReference,
) (pcdomain.CommercialClosure, error) {
	if !resolution.Valid() {
		return pcdomain.CommercialClosure{}, fmt.Errorf("%w: commercial resolution reference is not configured",
			ErrServiceProductUnavailable)
	}
	tenant, err := pcdomain.NewTenantID(tenantRaw)
	if err != nil {
		return pcdomain.CommercialClosure{}, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolutionID, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return pcdomain.CommercialClosure{}, fmt.Errorf("%w: resolution: %v", ErrUntranslatableAnswer, err)
	}
	closure, loaded, err := closures.LoadResolution(ctx, tenant, resolutionID)
	if err != nil {
		return pcdomain.CommercialClosure{}, fmt.Errorf("load commercial closure: %w", err)
	}
	if !loaded {
		return pcdomain.CommercialClosure{}, fmt.Errorf("%w: commercial closure %q was not found",
			ErrServiceProductUnavailable, resolutionID)
	}
	if closure.ResolutionKey().TenantID != tenant {
		return pcdomain.CommercialClosure{}, fmt.Errorf("%w: identity %q closure %q",
			ErrClosureTenantMismatch, tenant, closure.ResolutionKey().TenantID)
	}
	return closure, nil
}
