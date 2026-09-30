package partycommercial

import (
	"context"
	"errors"
	"fmt"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

var (
	// ErrClosureTenantMismatch 表示按调用方租户取回的闭包，其解析键却指着另一个租户。
	// 解析标识不是能力凭证（ADR-0003）：混进未配置会让运维去等一份其实已经写坏的闭包。
	// 初始路由与可达性共用这一哨兵。
	ErrClosureTenantMismatch = errors.New(
		"network routing partycommercial adapter: loaded commercial closure belongs to another tenant")
)

// RoutingApplicability 实现 nrports.RoutingApplicabilityView：同一份形态翻译表
// （UC-NR-001 步骤 3 / ADR-0050）。闭包标识由命令带来的已接受解析引用回指（ADR-0064）。
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
	closure, err := loadClosure(ctx, adapter.closures, key.TenantID, resolution)
	if err != nil {
		return nrdomain.NetworkEligibility{}, err
	}
	return eligibilityFromClosure(closure)
}
