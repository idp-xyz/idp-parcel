package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// AsOfPolicyDeclarations 实现 ports.AsOfPolicyDeclaration：按已唯一选出的接单规则包
// 取回它声明的时点锚。
//
// 只读。声明的写入面属实例半边（`PAR-COM-14` 的真实语义与政策版本），本适配器不提供
// 写口，也不在读不到时代拟任何一条——没有租户时它必然交回空声明，编排据以停在`未配置`。
type AsOfPolicyDeclarations struct {
	db *bentopg.DB
}

func NewAsOfPolicyDeclarations(db *bentopg.DB) (*AsOfPolicyDeclarations, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &AsOfPolicyDeclarations{db: db}, nil
}

var _ ports.AsOfPolicyDeclaration = (*AsOfPolicyDeclarations)(nil)

// LoadAsOfPolicies 按（租户+规则包版本身份）取回声明。
//
// 空声明不是错误：端口把「读不回」与「没声明」定义成两回事——前者等依赖恢复，后者等
// `PAR-COM-14` 落地，合成一格调用方就不知道该重试还是该催人去登记。所以查无行时交回
// 空切片配 nil，由 domain.DeclareAsOfPolicies 译`未配置`。
//
// 租户显式入参且进查询条件：按 ADR-0003 运营集团租户是最高数据隔离边界，跨越它必须在
// 签名上看得见，也必须在 WHERE 里挡得住。
func (repository *AsOfPolicyDeclarations) LoadAsOfPolicies(
	ctx context.Context,
	tenant domain.TenantID,
	rulePackage domain.CommercialVersion,
) ([]domain.AsOfPolicy, error) {
	if tenant.String() == "" ||
		rulePackage.ObjectID().String() == "" || rulePackage.Version().String() == "" {
		return nil, fmt.Errorf("load as-of policies: tenant and rule package identity are required")
	}

	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load as-of policies: %w", err)
	}

	// 按 judgment_type 排序让同一份声明每次以同一顺序交回。顺序不参与语义（消费方按判断
	// 类型查），但一个稳定的顺序让重放比对与失败复现少一个变量。
	rows, err := querier.Query(ctx,
		`SELECT judgment_type, semantics_ref, policy_version
		   FROM party_commercial.as_of_policy_declaration
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4
		  ORDER BY judgment_type`,
		tenant.String(),
		uint8(domain.AcceptanceRulePackageObject),
		rulePackage.ObjectID().String(),
		rulePackage.Version().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load as-of policies: %w", err)
	}
	defer rows.Close()

	var policies []domain.AsOfPolicy
	for rows.Next() {
		var judgmentName, semanticsRef, policyVersion string
		if err := rows.Scan(&judgmentName, &semanticsRef, &policyVersion); err != nil {
			return nil, fmt.Errorf("load as-of policies: %w", err)
		}
		policy, err := asOfPolicyFrom(judgmentName, semanticsRef, policyVersion)
		if err != nil {
			return nil, fmt.Errorf("load as-of policies: %w", err)
		}
		policies = append(policies, policy)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load as-of policies: %w", err)
	}
	return policies, nil
}

// asOfPolicyFrom 逐格翻译判断类型，default 报错不吸收：库内 CHECK 与领域封闭集是同一个
// 集合的两份镜像，出现集外取值说明两份已经分叉，那要人来看，不能就地当成缺一条声明。
func asOfPolicyFrom(judgmentName, semanticsRef, policyVersion string) (domain.AsOfPolicy, error) {
	var judgment domain.JudgmentType
	switch judgmentName {
	case domain.NetworkReachabilityJudgment.String():
		judgment = domain.NetworkReachabilityJudgment
	case domain.PreAcceptanceFinancialControlJudgment.String():
		judgment = domain.PreAcceptanceFinancialControlJudgment
	default:
		return domain.AsOfPolicy{}, fmt.Errorf("unknown judgment type %q", judgmentName)
	}

	semantics, err := domain.NewAsOfSemanticsReference(semanticsRef)
	if err != nil {
		return domain.AsOfPolicy{}, err
	}
	version, err := domain.NewAsOfPolicyVersion(policyVersion)
	if err != nil {
		return domain.AsOfPolicy{}, err
	}
	return domain.NewAsOfPolicy(judgment, semantics, version)
}
