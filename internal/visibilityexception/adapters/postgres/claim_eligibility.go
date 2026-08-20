package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ClaimEligibilityRules 实现 ports.EligibilityRuleView：交出适用于一项索赔的资格
// **规则**，不作资格判断。
//
// 判断已经不在这里了（见 .scratch/ve-claim-eligibility-dimensions 切块 (b)）：重复
// 关系要查同租户已有的索赔项，最低材料要看已收到的证据，两样都不是目录行；让本视图
// 去读它们会把「规则是什么」和「事实是什么」揉成一个既是目录又能读业务数据的东西。
// 编排拿走规则，自己去 ClaimStore 与证据侧逐维核对。
//
// 本适配器今天只登记得出七维里的一维——合同责任范围承不承担这个索赔类型。索赔时限
// 要起算事件与业务日历、最低材料要一份材料清单、授权要一份申请人目录，三样都属
// `PAR-VIS-08` 待登记实例参数，本上下文还没有那个登记面。**凑一份就是发明实例参数**，
// 所以三维一律如实答未登记，由编排停在指名到维的未决。
//
// 特别提防一处同形陷阱：parcel-shipment 的收寄资格视图在证据取不到时如实答「未成立」
// 并点名首项缺口。那一格在 PS 可续办；若把「证不了」写成这里的 `ClaimIneligible`，
// 就会变成不可逆的默认拒赔（ADR-0051：终局格一经写下不可经补充翻案），而编译与测试
// 都不会拦。本视图连 `EligibilityScreen` 都不再交出，那条路在类型上已经走不通。
//
// 租户在装配期固定，理由同本包另外几个视图：RulesForClaim 的签名里没有租户。
type ClaimEligibilityRules struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewClaimEligibilityRules(db *bentopg.DB, tenant domain.TenantID) (*ClaimEligibilityRules, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ClaimEligibilityRules{db: db, tenant: tenant}, nil
}

var _ ports.EligibilityRuleView = (*ClaimEligibilityRules)(nil)

// RulesForClaim 取适用于一项索赔的资格规则。
//
// 第二个返回值为 false 只有一个意思：**合同的索赔资格声明不在场**。那时连「这个类型
// 在不在保」都无从谈起——缺一个类型是「没人声明过」，不是「声明说不保」，凭一张空表
// 拒赔就是虚构。声明在场则一律交回规则，某一维尚未登记由那一维自己的 Registered 交代：
// 「整份声明还没登记」与「只差材料清单」的补法不是一件事，折成同一格会让人去补错东西。
//
// 三个 Registered 恒为 false 不是占位：`claim_contract_scope` / `claim_covered_kind`
// 是本上下文今天仅有的两张资格目录表，时限、材料与授权连登记面都还没有。给它们建空表
// 也点不亮任何路径——四件落点里的当前截止靠资料补充期限，那同样是待登记的实例参数，
// 所以先如实答未登记，等 `PAR-VIS-08` 连同登记面一起落地。
func (view *ClaimEligibilityRules) RulesForClaim(
	ctx context.Context,
	query ports.EligibilityQuery,
) (ports.EligibilityRules, bool, error) {
	if view.tenant.String() == "" || query.Contract.String() == "" || query.Kind.String() == "" {
		return ports.EligibilityRules{}, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.EligibilityRules{}, false, fmt.Errorf("rules for claim: %w", err)
	}

	var ruleVersion string
	var covered bool
	err = querier.QueryRow(ctx,
		`SELECT scope.rule_version,
		        EXISTS (
		            SELECT 1
		              FROM visibility_exception.claim_covered_kind AS kind
		             WHERE kind.tenant_id = scope.tenant_id
		               AND kind.contract_scope_ref = scope.contract_scope_ref
		               AND kind.claim_kind_ref = $3
		        )
		   FROM visibility_exception.claim_contract_scope AS scope
		  WHERE scope.tenant_id = $1 AND scope.contract_scope_ref = $2`,
		view.tenant.String(), query.Contract.String(), query.Kind.String(),
	).Scan(&ruleVersion, &covered)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.EligibilityRules{}, false, nil
	}
	if err != nil {
		return ports.EligibilityRules{}, false, fmt.Errorf("rules for claim: %w", err)
	}

	return ports.EligibilityRules{
		RuleVersion:    ruleVersion,
		KindCovered:    covered,
		FilingDeadline: ports.FilingDeadlineRule{},
		Materials:      ports.MinimumMaterialsRule{},
		Authorization:  ports.AuthorizationCatalogue{},
	}, true, nil
}
