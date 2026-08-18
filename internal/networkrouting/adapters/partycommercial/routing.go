package partycommercial

import (
	"context"
	"errors"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

var (
	// ErrRoutingClosureTenantMismatch 表示按调用方租户取回的闭包，其解析键却指着另一个
	// 租户。解析标识不是能力凭证（ADR-0003 / ADR-0064）：混进未配置会让运维去等一份
	// 其实已经写坏的映射。
	ErrRoutingClosureTenantMismatch = errors.New(
		"network routing partycommercial adapter: loaded commercial closure belongs to another tenant")
)

// RoutingApplicability 实现 nrports.RoutingApplicabilityView：同一份形态翻译表
// （UC-NR-001 步骤 3 / ADR-0050）。闭包标识由命令带来的已接受解析引用回指
// （ADR-0064），不再要一份判断键到 RES 的实例映射。
type RoutingApplicability struct {
	closures pcports.CommercialResolutionView
}

func NewRoutingApplicability(
	closures pcports.CommercialResolutionView,
) (*RoutingApplicability, error) {
	if closures == nil {
		return nil, fmt.Errorf("network routing partycommercial adapter: commercial resolution view is nil")
	}
	return &RoutingApplicability{closures: closures}, nil
}

var _ nrports.RoutingApplicabilityView = (*RoutingApplicability)(nil)

func (adapter *RoutingApplicability) AssessRoutingApplicability(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
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
