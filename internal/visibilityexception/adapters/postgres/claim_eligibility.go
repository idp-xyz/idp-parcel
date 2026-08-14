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

// 合同范围不承担该索赔类型时交回的依据前缀。它与 party-commercial 消费侧那个
// `SOURCE_NOT_IN_SERVICE_SHAPE` 同一路数：点名是哪一条判据不成立，而不是笼统说不过。
const reasonKindNotInContractScope = "CLAIM_KIND_NOT_IN_CONTRACT_SCOPE"

// ClaimEligibilityRules 实现 ports.EligibilityRuleView。
//
// 它**只答得了资格审核七维里的一维**，这不是偷工。CONTEXT 要求按申请人授权、客户
// 账户、合同版本、索赔时限、目标范围、重复关系和最低材料要求判断资格，而
// `EligibilityQuery` 连一个时间戳都没带：索赔时限算不了，申请人不在查询里，材料在
// 证据聚合里，重复关系在 ClaimStore 里——后两样都不是目录能答的东西，视图去读仓储
// 会把「规则是什么」和「事实是什么」揉成一个既是目录又能读业务数据的东西。
//
// 答不了的一律交回 found=false，让编排停在可续办的未决。这条选择是硬的，因为
// `EligibilityScreen` 是封闭二值且 `ScreenEligibility` 一次性——`ErrClaimAlreadyScreened`
// 挡住重审。因为「证不了」而答`不通过`，那条索赔就被永久拒掉且再无第二次机会，那正是
// 「默认拒赔是虚构」所禁的，且不可逆。
//
// 特别提防一处同形陷阱：parcel-shipment 的收寄资格视图在证据取不到时如实答「未成立」
// 并点名首项缺口，那个写法在**这里搬过来会变成不可逆的默认拒赔**。两边看着一样，
// 分界在答复代数上——那边有第三格且可续办（编排据以保持未决、等客户补办），这里
// 二值里没有那一格，一答就定终身。改本文件的人若照着那份改，编译与测试都不会拦。
//
// 租户在装配期固定，理由同本包另外几个视图：ScreenClaim 的签名里没有租户。
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

// ScreenClaim 审一项索赔的资格。
//
// 三支：
//   - 合同的索赔声明不在场 → 未配置。缺一个类型此时是「没人声明过」，不是「声明说
//     不保」，凭一张空表拒赔就是虚构。
//   - 声明在场且该类型不在覆盖集合内 → `不通过`带依据。这一判定完整且永久成立：
//     承担与否不随材料补充而变，变了就是换了合同范围，而换范围按 CONTEXT 是另一个
//     索赔项。
//   - 声明在场且该类型在保 → 仍未配置。剩下四维还证不了，而端口把 found=false 定义
//     为「资格目录未配置」并点名它含索赔时限、材料要求与授权目录——那三样确属
//     `PAR-VIS-08` 待提供，所以这一格是如实的，不是搪塞。
func (view *ClaimEligibilityRules) ScreenClaim(
	ctx context.Context,
	query ports.EligibilityQuery,
) (ports.EligibilityAnswer, bool, error) {
	if view.tenant.String() == "" || query.Contract.String() == "" || query.Kind.String() == "" {
		return ports.EligibilityAnswer{}, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.EligibilityAnswer{}, false, fmt.Errorf("screen claim: %w", err)
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
		return ports.EligibilityAnswer{}, false, nil
	}
	if err != nil {
		return ports.EligibilityAnswer{}, false, fmt.Errorf("screen claim: %w", err)
	}

	if covered {
		return ports.EligibilityAnswer{}, false, nil
	}
	return ports.EligibilityAnswer{
		Screen: domain.ClaimIneligible,
		Basis:  reasonKindNotInContractScope + "/" + query.Kind.String() + "/" + ruleVersion,
	}, true, nil
}
