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
// 本册承载两角：合同责任范围承不承担这个索赔类型（0011），申请人授权目录（0018，
// 切块 (c)）。索赔时限与最低材料两维**不在本册**：它们的登记面从 ADR-0104 起在
// party-commercial 的客户服务规则册，由消费侧适配器 adapters/partycommercial 叠在本视图
// 的答案上（票 ve-claims-read-seams/03）。本视图对那两维恒交零值——那是「本册不承载」，
// 不是「已核过未登记」；单独装本视图（受控登记口）时两维因此如实答未登记。**凑一份就是
// 发明实例参数**：这里不拿任何期限或清单顶位。授权目录同理——没有目录行就答未登记，
// 不拿空名单冒充「无人获授权」。
//
// 特别提防一处同形陷阱：parcel-shipment 的收寄资格视图在证据取不到时如实答「未成立」
// 并点名首项缺口。那一格在 PS 可续办；若把「证不了」写成这里的 `ClaimIneligible`，
// 就会变成不可逆的默认拒赔（ADR-0051：终局格一经写下不可经补充翻案），而编译与测试
// 都不会拦。本视图连 `EligibilityScreen` 都不再交出，那条路在类型上已经走不通。
//
// 租户在装配期固定，形状为受控登记口而设：登记口只该看见自己那一户。查询自带租户
// 之后（.scratch/ve-claims-read-seams/01），多租户入口走 MultiTenantClaimEligibilityRules，
// 两个形状不合并——合并会让登记口拿到跨租户读。本视图收到带租户的查询时要求与钉住的
// 一致，不一致按「依赖调不通」报错：沉默地用钉住租户作答会把接错装成接对。
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
// 时限与材料两维本视图恒交零值：登记面在 party-commercial 的客户服务规则册（ADR-0104），
// 读它是跨上下文翻译，只能落在 adapters/partycommercial；本视图是持久化适配器，不该也
// 不能去读另一个上下文的册。多租户入口由那边的 ClaimServiceRules 叠上两维；受控登记口
// 单独装本视图，两维因此如实答未登记，不给它们建空表——四件落点里的当前截止靠资料
// 补充期限派生，起算事实与日历今天 VE 也还没有。
func (view *ClaimEligibilityRules) RulesForClaim(
	ctx context.Context,
	query ports.EligibilityQuery,
) (ports.EligibilityRules, bool, error) {
	if query.Tenant.String() != "" && query.Tenant != view.tenant {
		return ports.EligibilityRules{}, false, fmt.Errorf(
			"rules for claim: view is pinned to tenant %q but the query carries %q",
			view.tenant, query.Tenant)
	}
	return claimRulesForTenant(ctx, view.db, view.tenant, query)
}

// claimRulesForTenant 是两个形状共用的查询本体：答案语义一份，差别只在租户从哪来。
// 守卫沿单租户形状既有的样子：租户、合同或类型为零值即「声明不在场」。
func claimRulesForTenant(
	ctx context.Context,
	db *bentopg.DB,
	tenant domain.TenantID,
	query ports.EligibilityQuery,
) (ports.EligibilityRules, bool, error) {
	if tenant.String() == "" || query.Contract.String() == "" || query.Kind.String() == "" {
		return ports.EligibilityRules{}, false, nil
	}

	querier, err := db.ReadExecutor(ctx)
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
		tenant.String(), query.Contract.String(), query.Kind.String(),
	).Scan(&ruleVersion, &covered)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.EligibilityRules{}, false, nil
	}
	if err != nil {
		return ports.EligibilityRules{}, false, fmt.Errorf("rules for claim: %w", err)
	}

	authorization, err := claimAuthorizationForTenant(ctx, querier, tenant, query)
	if err != nil {
		return ports.EligibilityRules{}, false, err
	}

	// 两维交候处：零值交给 adapters/partycommercial 的 ClaimServiceRules 去叠 PC 正文的答案；
	// 本册不承载它们，也不在这里替它们答「已核过未登记」。
	return ports.EligibilityRules{
		RuleVersion:    ruleVersion,
		KindCovered:    covered,
		FilingDeadline: ports.FilingDeadlineRule{},
		Materials:      ports.MinimumMaterialsRule{},
		Authorization:  authorization,
	}, true, nil
}

// claimAuthorizationForTenant 取申请人授权目录。没有目录行即未登记（实例半边，编排
// 停在未决）。名单按查询里的申请人收窄到相关那一行——端口注释允许收窄且语义不变：
// 在列即获授权。查询没带申请人（存量索赔）时只答登记情况，整份名单没有读者：编排在
// 核对之前就会停在「申请人缺席」那一维。
func claimAuthorizationForTenant(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	query ports.EligibilityQuery,
) (ports.AuthorizationCatalogue, error) {
	var ruleVersion string
	err := querier.QueryRow(ctx,
		`SELECT rule_version
		   FROM visibility_exception.claim_authorization_catalogue
		  WHERE tenant_id = $1 AND customer_ref = $2`,
		tenant.String(), query.Customer.String(),
	).Scan(&ruleVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.AuthorizationCatalogue{}, nil
	}
	if err != nil {
		return ports.AuthorizationCatalogue{}, fmt.Errorf("authorization catalogue: %w", err)
	}

	catalogue := ports.AuthorizationCatalogue{Registered: true, RuleVersion: ruleVersion}
	if query.Applicant.String() == "" {
		return catalogue, nil
	}
	var listed bool
	err = querier.QueryRow(ctx,
		`SELECT EXISTS (
		            SELECT 1
		              FROM visibility_exception.claim_authorized_applicant
		             WHERE tenant_id = $1 AND customer_ref = $2 AND applicant_ref = $3
		        )`,
		tenant.String(), query.Customer.String(), query.Applicant.String(),
	).Scan(&listed)
	if err != nil {
		return ports.AuthorizationCatalogue{}, fmt.Errorf("authorization catalogue: %w", err)
	}
	if listed {
		catalogue.AuthorizedApplicants = []domain.ApplicantReference{query.Applicant}
	}
	return catalogue, nil
}
