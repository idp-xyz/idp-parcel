package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// AcceptanceContentDeclarations 实现 ports.AcceptanceContentDeclaration：按已唯一选出的
// 规则包与服务产品，取回它们各自声明的接受内容。
//
// 只读。声明正文属实例半边，本适配器不提供写口，读不到时也不代拟任何一条。两个方法的
// found=false 语义相反，逐个方法在注释里各自写明——共用一句话正是端口注释警告过的事。
type AcceptanceContentDeclarations struct {
	db *bentopg.DB
}

func NewAcceptanceContentDeclarations(db *bentopg.DB) (*AcceptanceContentDeclarations, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &AcceptanceContentDeclarations{db: db}, nil
}

var _ ports.AcceptanceContentDeclaration = (*AcceptanceContentDeclarations)(nil)

// LoadAcceptanceRuleContent 取回规则包的适用校验组与人工复核指令。
//
// found=false = **实例未配置**：缺声明不等于没有组适用、也不等于免复核——无从知道该判
// 哪些组，消费方据以停在未决。所以查无行时不拼一份空声明交出去：领域也拦着（空组集合
// 等于无条件接受，规则包表达不了那种东西），但拦在这里更早，且理由要写在这一层。
func (repository *AcceptanceContentDeclarations) LoadAcceptanceRuleContent(
	ctx context.Context,
	tenant domain.TenantID,
	rulePackage domain.CommercialVersion,
) (domain.AcceptanceRuleContent, bool, error) {
	none := domain.AcceptanceRuleContent{}
	if tenant.String() == "" ||
		rulePackage.ObjectID().String() == "" || rulePackage.Version().String() == "" {
		return none, false, fmt.Errorf("load acceptance rule content: tenant and rule package identity are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule content: %w", err)
	}

	var directiveName string
	err = querier.QueryRow(ctx,
		`SELECT manual_review
		   FROM party_commercial.acceptance_rule_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.AcceptanceRulePackageObject),
		rulePackage.ObjectID().String(),
		rulePackage.Version().String(),
	).Scan(&directiveName)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule content: %w", err)
	}
	directive, err := manualReviewDirectiveFrom(directiveName)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule content: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT check_group
		   FROM party_commercial.acceptance_rule_check_group
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4
		  ORDER BY check_group`,
		tenant.String(),
		uint8(domain.AcceptanceRulePackageObject),
		rulePackage.ObjectID().String(),
		rulePackage.Version().String(),
	)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule content: %w", err)
	}
	defer rows.Close()

	var groups []domain.AcceptanceCheckGroupType
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return none, false, fmt.Errorf("load acceptance rule content: %w", err)
		}
		group, err := checkGroupTypeFrom(name)
		if err != nil {
			return none, false, fmt.Errorf("load acceptance rule content: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("load acceptance rule content: %w", err)
	}

	// 主行在而组集合空：外键保得住归属，保不住「至少一组」。领域会拒，但那会让调用方
	// 收到一个说不清是缺声明还是坏声明的答复——在这里点名它是坏声明。
	content, err := domain.DeclareAcceptanceRuleContent(rulePackage, groups, directive)
	if err != nil {
		return none, false, fmt.Errorf("load acceptance rule content: %w", err)
	}
	return content, true, nil
}

// LoadPendingRoutingPermission 取回服务产品的待路由许可。
//
// found=false = **未许可**（零值语义），不是未决：待路由是例外许可，UC 只认「服务产品明确
// 允许」，没有声明就是没有许可，消费方照常推进、只是不得走待路由。读取失败（error）才是
// 未决——那是谁也没回答过，不得冒充「产品说了不许」。
func (repository *AcceptanceContentDeclarations) LoadPendingRoutingPermission(
	ctx context.Context,
	tenant domain.TenantID,
	serviceProduct domain.CommercialVersion,
) (domain.PendingRoutingPermission, bool, error) {
	none := domain.PendingRoutingPermission{}
	if tenant.String() == "" ||
		serviceProduct.ObjectID().String() == "" || serviceProduct.Version().String() == "" {
		return none, false, fmt.Errorf("load pending routing permission: tenant and product identity are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load pending routing permission: %w", err)
	}

	var basisRef string
	err = querier.QueryRow(ctx,
		`SELECT basis_ref
		   FROM party_commercial.pending_routing_permission
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.ServiceProductObject),
		serviceProduct.ObjectID().String(),
		serviceProduct.Version().String(),
	).Scan(&basisRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load pending routing permission: %w", err)
	}

	basis, err := domain.NewPendingRoutingBasisReference(basisRef)
	if err != nil {
		return none, false, fmt.Errorf("load pending routing permission: %w", err)
	}
	permission, err := domain.DeclarePendingRoutingPermission(serviceProduct, basis)
	if err != nil {
		return none, false, fmt.Errorf("load pending routing permission: %w", err)
	}
	return permission, true, nil
}

// manualReviewDirectiveFrom 逐格翻译，default 报错不吸收：库内 CHECK 只放行已声明两值，
// 出现第三个取值说明两处封闭集已分叉，那要人来看。
func manualReviewDirectiveFrom(raw string) (domain.ManualReviewDirective, error) {
	switch raw {
	case domain.ManualReviewRequired.String():
		return domain.ManualReviewRequired, nil
	case domain.ManualReviewNotRequired.String():
		return domain.ManualReviewNotRequired, nil
	default:
		return domain.ManualReviewUndeclared, fmt.Errorf("unknown manual review directive %q", raw)
	}
}

func checkGroupTypeFrom(raw string) (domain.AcceptanceCheckGroupType, error) {
	for _, group := range []domain.AcceptanceCheckGroupType{
		domain.CustomerRelationshipCheckGroup,
		domain.LegalEntityAndContractCheckGroup,
		domain.ProductAndServiceCheckGroup,
		domain.MemberBaselineCheckGroup,
		domain.RequiredDocumentCheckGroup,
		domain.PreAcceptanceFinancialControlCheckGroup,
		domain.NetworkReachabilityCheckGroup,
	} {
		if group.String() == raw {
			return group, nil
		}
	}
	return domain.AcceptanceCheckGroupTypeInvalid, fmt.Errorf("unknown acceptance check group %q", raw)
}
