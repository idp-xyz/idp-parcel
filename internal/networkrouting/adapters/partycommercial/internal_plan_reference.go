package partycommercial

import (
	"context"
	"errors"
	"fmt"
	"strings"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件把内部成本依据引用解成方案引用（票 routing-first-cut/10，ADR-0148 决定四）。
// 位置由 ADR-0025 定死：只有消费侧的 internal/<consumer>/adapters/<provider>/ 可以导入
// 提供方上下文——这一格读 party-commercial 的价格政策正文，所以住在 networkrouting 的
// partycommercial 目录里。它只翻译不判断：政策绑谁就是谁，方向对不对由这里拒译。

var (
	// ErrNotAnInternalCostPolicy 说引用指到了一份方向不是内部的商业价格政策。政策正文
	// 本身合法，错的是目录引用——按它计价会把销售价/采购价当内部标准成本比进路由。
	ErrNotAnInternalCostPolicy = errors.New("network routing: referenced price policy is not an internal cost policy")
	// ErrUntranslatablePlanReference 说政策引用译不出「对象身份/版本标签」两段形状。
	ErrUntranslatablePlanReference = errors.New("network routing: untranslatable internal policy reference")
)

// InternalPlanResolver 把一条内部政策引用（「对象身份/版本标签」）解成绑定方案引用
// （「id/version」）。found=false 是这版政策没有登记价格正文——合法缺席，取数侧答待判断。
func NewInternalPlanResolver(policies pcports.InternalCostPolicyView) *InternalPlanResolver {
	return &InternalPlanResolver{policies: policies}
}

type InternalPlanResolver struct {
	policies pcports.InternalCostPolicyView
}

func (resolver *InternalPlanResolver) PlanReferenceForInternalPolicy(
	ctx context.Context,
	tenant nrdomain.TenantID,
	policyReference string,
) (string, bool, error) {
	objectID, versionLabel, ok := strings.Cut(policyReference, "/")
	if !ok || objectID == "" || versionLabel == "" {
		return "", false, fmt.Errorf("%w: %q", ErrUntranslatablePlanReference, policyReference)
	}
	pcTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return "", false, fmt.Errorf("resolve internal plan reference: %w", err)
	}
	pcObjectID, err := pcdomain.NewCommercialObjectID(objectID)
	if err != nil {
		return "", false, fmt.Errorf("resolve internal plan reference: %w", err)
	}
	pcLabel, err := pcdomain.NewCommercialVersionLabel(versionLabel)
	if err != nil {
		return "", false, fmt.Errorf("resolve internal plan reference: %w", err)
	}
	policy, found, err := resolver.policies.LoadInternalCostPolicy(ctx, pcTenant, pcObjectID, pcLabel)
	if err != nil {
		return "", false, fmt.Errorf("resolve internal plan reference: %w", err)
	}
	if !found {
		return "", false, nil
	}
	if policy.Direction() != pcdomain.InternalDirection {
		return "", false, fmt.Errorf("%w: %q", ErrNotAnInternalCostPolicy, policyReference)
	}
	return policy.PricingPlan().String(), true, nil
}
