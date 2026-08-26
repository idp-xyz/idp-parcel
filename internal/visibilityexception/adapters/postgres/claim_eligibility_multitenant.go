package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// MultiTenantClaimEligibilityRules 实现 ports.EligibilityRuleView，租户从查询来、
// 不从构造期来——多租户入口（`cmd/parcel-api`）的形状（.scratch/ve-claims-read-seams/01）。
//
// 与按租户现绑的 ClaimEligibilityRules 分列两个类型、不合并：现绑形状为受控登记口
// 而设，登记口只该看见自己那一户，折成一个类型会让它拿到跨租户读。两个形状的答案
// 语义完全相同（共用 claimRulesForTenant），差别只有租户从哪来。
//
// 查询没带租户按「依赖调不通」报错，不折成「声明不在场」：索赔命令的租户由领域
// 校验非空，走到这里还缺租户是接线错误，答业务格会把恢复动作指去登记声明——修的
// 不是那件事。
type MultiTenantClaimEligibilityRules struct {
	db *bentopg.DB
}

func NewMultiTenantClaimEligibilityRules(db *bentopg.DB) (*MultiTenantClaimEligibilityRules, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &MultiTenantClaimEligibilityRules{db: db}, nil
}

var _ ports.EligibilityRuleView = (*MultiTenantClaimEligibilityRules)(nil)

// RulesForClaim 按查询携带的租户取适用规则。第二个返回值的语义随端口：false 只有
// 「合同的索赔资格声明不在场」一个意思——该租户没登记过就是没登记过，与别的租户
// 登没登记无关。
func (view *MultiTenantClaimEligibilityRules) RulesForClaim(
	ctx context.Context,
	query ports.EligibilityQuery,
) (ports.EligibilityRules, bool, error) {
	if query.Tenant.String() == "" {
		return ports.EligibilityRules{}, false, fmt.Errorf(
			"rules for claim: query carries no tenant")
	}
	return claimRulesForTenant(ctx, view.db, query.Tenant, query)
}
