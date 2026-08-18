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
// found=false（open-decisions F-3）。
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
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}

	var rulePackageID string
	err = querier.QueryRow(ctx,
		`SELECT rule_package_id
		   FROM party_commercial.customer_contract_content
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.CustomerContractObject),
		contract.ObjectID().String(),
		contract.Version().String(),
	).Scan(&rulePackageID)
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

	rows, err := querier.Query(ctx,
		`SELECT charge_scope_ref, policy_id, inapplicability_basis
		   FROM party_commercial.customer_contract_control_binding
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4
		  ORDER BY charge_scope_ref`,
		tenant.String(),
		uint8(domain.CustomerContractObject),
		contract.ObjectID().String(),
		contract.Version().String(),
	)
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}
	defer rows.Close()

	var bindings []domain.FinancialControlBinding
	for rows.Next() {
		var scopeRef string
		var policyID, basis *string
		if err := rows.Scan(&scopeRef, &policyID, &basis); err != nil {
			return none, false, fmt.Errorf("load customer contract: %w", err)
		}
		binding, err := financialControlBindingFrom(scopeRef, policyID, basis)
		if err != nil {
			return none, false, fmt.Errorf("load customer contract: %w", err)
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}

	content, err := domain.NewCustomerContract(contract, rulePackage, bindings)
	if err != nil {
		return none, false, fmt.Errorf("load customer contract: %w", err)
	}
	return content, true, nil
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
