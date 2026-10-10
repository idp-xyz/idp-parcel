package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CustomerServiceRuleLayerReader 实现 ports.CustomerServiceRuleLayerView。
//
// 它不自带读法：底座用闭包解析读的同一份权威视图（CommercialAuthorityView）、经闭包回落层同一处选法
// （CustomerServiceRuleProductBase）选，两层正文都经既有点读口（CustomerServiceRuleContentView）取。另写一条取底座的
// 查询会立第二处口径；正文另读一遍，点读口里「壳与正文一致」那道核与坏数据判据就在底座这一层缺席。
type CustomerServiceRuleLayerReader struct {
	authority ports.CommercialAuthorityView
	contents  ports.CustomerServiceRuleContentView
}

var _ ports.CustomerServiceRuleLayerView = (*CustomerServiceRuleLayerReader)(nil)

func NewCustomerServiceRuleLayerReader(
	authority ports.CommercialAuthorityView,
	contents ports.CustomerServiceRuleContentView,
) (*CustomerServiceRuleLayerReader, error) {
	if authority == nil {
		return nil, fmt.Errorf("party commercial application: commercial authority view is required")
	}
	if contents == nil {
		return nil, fmt.Errorf("party commercial application: customer service rule content view is required")
	}
	return &CustomerServiceRuleLayerReader{authority: authority, contents: contents}, nil
}

// LoadCustomerServiceRuleLayers 各格见 ports.CustomerServiceRuleLayerView。
//
// 问法先判：立不住时不读任何一层，也不去数候选。权威读不到时交空视图给选法，由它答`解析未决`——与第一阶段
// 「读不到权威不向上抛技术错误」同一条分界；而正文读不到仍是 error，判据随点读口。
func (reader *CustomerServiceRuleLayerReader) LoadCustomerServiceRuleLayers(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
	contractRule domain.CommercialVersion,
	anchor domain.SelectionAnchor,
	declaredProduct domain.CommercialObjectID,
) (ports.CustomerServiceRuleLayers, error) {
	none := ports.CustomerServiceRuleLayers{}
	if tenant.String() == "" || scope.String() == "" ||
		contractRule.ObjectID().String() == "" || contractRule.Version().String() == "" {
		return none, fmt.Errorf("load customer service rule layers: tenant, scope and the adopted rule identity are required")
	}
	if contractRule.Kind() != domain.CustomerServiceRuleObject {
		return none, fmt.Errorf("load customer service rule layers: %q is not a customer service rule version", contractRule.Kind())
	}
	if contractRule.Tenant() != tenant {
		return none, fmt.Errorf("load customer service rule layers: tenant does not own this rule")
	}
	if contractRule.Scope() != scope {
		return none, fmt.Errorf("load customer service rule layers: the adopted rule is not in the asked scope")
	}

	var layers ports.CustomerServiceRuleLayers
	var err error
	layers.ContractTier, layers.HasContractTier, err = reader.contents.LoadCustomerServiceRule(ctx, tenant, contractRule)
	if err != nil {
		return none, fmt.Errorf("load customer service rule layers: contract tier: %w", err)
	}

	registry, err := reader.authority.LoadScope(ctx, tenant, scope)
	if err != nil {
		registry = nil
	}
	base, outcome := registry.CustomerServiceRuleProductBase(tenant, scope, anchor, declaredProduct)
	layers.ProductBaseOutcome = outcome
	if outcome != domain.UniquelyResolved {
		return layers, nil
	}
	layers.ProductBase, layers.HasProductBase, err = reader.contents.LoadCustomerServiceRule(ctx, tenant, base)
	if err != nil {
		return none, fmt.Errorf("load customer service rule layers: product base: %w", err)
	}
	return layers, nil
}
