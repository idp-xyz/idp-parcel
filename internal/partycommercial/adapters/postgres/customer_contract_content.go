package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CustomerContractContents 实现 ports.CustomerContractContentView：按已唯一选出的
// 客户合同版本取回正文。
//
// 只读。正文属实例半边，本适配器不提供写口，也不在读不到时代拟任何绑定——无父行
// 是未配置，有父行零子行才是「明确没约定任何费用范围」。不进 CommercialRegistry /
// ViewRevision：绑定改动与选择无关。
type CustomerContractContents struct {
	db *bentopg.DB
}

func NewCustomerContractContents(db *bentopg.DB) (*CustomerContractContents, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &CustomerContractContents{db: db}, nil
}

var _ ports.CustomerContractContentView = (*CustomerContractContents)(nil)

// LoadCustomerContract 取回合同正文。
//
// found=false = **正文未登记**（无父行）。父行在场即走 NewCustomerContract 重建，
// 零子行是合法的空约定。版本壳与正文件规则包引用都在场且不等 → error，不折成
// found=false（open-decisions F-3）。显式租户与合同对象必须同一身份，否则 error
// 且不交内容。父行与绑定由一条左连接取回，不拆成两次查询。
func (repository *CustomerContractContents) LoadCustomerContract(
	ctx context.Context,
	tenant domain.TenantID,
	contract domain.CommercialVersion,
) (domain.CustomerContract, bool, error) {
	none := domain.CustomerContract{}
	if tenant.String() == "" ||
		contract.ObjectID().String() == "" || contract.Version().String() == "" {
		return none, false, fmt.Errorf("load customer contract: tenant and contract identity are required")
	}
	// 显式租户与合同对象必须是同一个身份：按租户查库、按合同重建，两处各写各的就会
	// 把 A 的行装进 B 的合同（ADR-0003/0040，租户是身份不是过滤器）。
	if tenant != contract.Tenant() {
		return none, false, fmt.Errorf("load customer contract: tenant does not own this contract")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}

	// 父 LEFT JOIN 子一次取回：ReadExecutor 不保证两条语句同一快照，分两次会拼出从未
	// 同时存在的父子状态。无父行 = 零行（found=false）；有父零子 = 一行空 json 数组。
	var rulePackageID string
	var bindingsJSON []byte
	err = querier.QueryRow(ctx,
		`SELECT parent.rule_package_id,
		        COALESCE(
		            json_agg(
		                json_build_object(
		                    'scope',  child.charge_scope_ref,
		                    'policy', child.policy_id,
		                    'basis',  child.inapplicability_basis
		                )
		                ORDER BY child.charge_scope_ref
		            ) FILTER (WHERE child.charge_scope_ref IS NOT NULL),
		            '[]'::json
		        )
		   FROM party_commercial.customer_contract_content AS parent
		   LEFT JOIN party_commercial.customer_contract_control_binding AS child
		          ON child.tenant_id     = parent.tenant_id
		         AND child.object_kind   = parent.object_kind
		         AND child.object_id     = parent.object_id
		         AND child.version_label = parent.version_label
		  WHERE parent.tenant_id     = $1
		    AND parent.object_kind   = $2
		    AND parent.object_id     = $3
		    AND parent.version_label = $4
		  GROUP BY parent.tenant_id, parent.object_kind, parent.object_id,
		           parent.version_label, parent.rule_package_id`,
		tenant.String(),
		uint8(domain.CustomerContractObject),
		contract.ObjectID().String(),
		contract.Version().String(),
	).Scan(&rulePackageID, &bindingsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}

	rulePackage, err := domain.NewCommercialObjectID(rulePackageID)
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}
	if err := domain.ConsistentAcceptanceRulePackage(contract, rulePackage); err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}

	bindings, err := financialControlBindingsFromJSON(bindingsJSON)
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}

	content, err := domain.NewCustomerContract(contract, rulePackage, bindings)
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}
	return content, true, nil
}

type contractBindingDocument struct {
	Scope  string  `json:"scope"`
	Policy *string `json:"policy"`
	Basis  *string `json:"basis"`
}

func financialControlBindingsFromJSON(raw []byte) ([]domain.FinancialControlBinding, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var documents []contractBindingDocument
	if err := json.Unmarshal(raw, &documents); err != nil {
		return nil, fmt.Errorf("bindings are not this adapter's shape: %w", err)
	}
	bindings := make([]domain.FinancialControlBinding, 0, len(documents))
	for _, document := range documents {
		binding, err := financialControlBindingFrom(document.Scope, document.Policy, document.Basis)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

// financialControlBindingFrom 把子表一行折回领域构造门。CHECK 保证恰一列在场，
// 走到两空/两满说明两处已经分叉，报错不吸收。
func financialControlBindingFrom(
	scopeRef string,
	policyID, basis *string,
) (domain.FinancialControlBinding, error) {
	scope, err := domain.NewChargeScopeReference(scopeRef)
	if err != nil {
		return domain.FinancialControlBinding{}, err
	}
	switch {
	case policyID != nil && basis == nil:
		policy, err := domain.NewCommercialObjectID(*policyID)
		if err != nil {
			return domain.FinancialControlBinding{}, err
		}
		return domain.NewAppliedFinancialControl(scope, policy)
	case policyID == nil && basis != nil:
		inapplicable, err := domain.NewInapplicabilityBasis(*basis)
		if err != nil {
			return domain.FinancialControlBinding{}, err
		}
		return domain.NewInapplicableFinancialControl(scope, inapplicable)
	default:
		return domain.FinancialControlBinding{}, fmt.Errorf(
			"control binding for %q is neither applied nor inapplicable", scopeRef)
	}
}
