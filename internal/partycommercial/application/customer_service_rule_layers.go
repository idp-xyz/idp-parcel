package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CustomerServiceRuleLayerReader 实现 ports.CustomerServiceRuleLayerView。
//
// 它不自带读法：两层正文都经既有点读口（CustomerServiceRuleContentView）取，另读一遍，点读口里「壳与正文一致」那道核
// 与坏数据判据就在底座这一层缺席。权威视图（CommercialAuthorityView）只为闭包上没有底座一格的旧快照留着，经闭包回落层
// 同一处选法（CustomerServiceRuleProductBase）现选；另写一条取底座的查询会立第二处口径。
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
// 问法先判：立不住时不读任何一层，也不去数候选。现选那一路权威读不到时交空视图给选法，由它答`解析未决`——与第一阶段
// 「读不到权威不向上抛技术错误」同一条分界；而正文读不到仍是 error，判据随点读口。
func (reader *CustomerServiceRuleLayerReader) LoadCustomerServiceRuleLayers(
	ctx context.Context,
	tenant domain.TenantID,
	closure domain.CommercialClosure,
) (ports.CustomerServiceRuleLayers, error) {
	none := ports.CustomerServiceRuleLayers{}
	adopted, err := contractTierAdoption(tenant, closure)
	if err != nil {
		return none, fmt.Errorf("load customer service rule layers: %w", err)
	}
	contractRule := adopted.Version()

	var layers ports.CustomerServiceRuleLayers
	layers.ContractTier, layers.HasContractTier, err = reader.contents.LoadCustomerServiceRule(ctx, tenant, contractRule)
	if err != nil {
		return none, fmt.Errorf("load customer service rule layers: contract tier: %w", err)
	}

	base, outcome := reader.productBase(ctx, tenant, closure, adopted)
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

// productBase 先读闭包上固定的那一版（ADR-0268 决定一）；快照没有这一格时才按闭包锚点与键上声明的服务产品现选
// （决定三）。固定了没有就是`无适用依据`，不再去现选：那会把「接受时没有底座」换成「此刻有一版」。
func (reader *CustomerServiceRuleLayerReader) productBase(
	ctx context.Context,
	tenant domain.TenantID,
	closure domain.CommercialClosure,
	adopted domain.AdoptedBasis,
) (domain.CommercialVersion, domain.ResolutionOutcome) {
	if base, hasBase, fixed := adopted.InheritedProductBase(); fixed {
		if !hasBase {
			return domain.CommercialVersion{}, domain.NoApplicableBasis
		}
		return base, domain.UniquelyResolved
	}
	scope := adopted.Version().Scope()
	registry, err := reader.authority.LoadScope(ctx, tenant, scope)
	if err != nil {
		registry = nil
	}
	return registry.CustomerServiceRuleProductBase(tenant, scope, closure.Anchor(), closure.ResolutionKey().ServiceProduct)
}

// contractTierAdoption 交回闭包采纳的那一版合同版客户服务规则，问法立不住即 error。范围必须是闭包键的范围：别的范围
// 的产品版对这一户不可见，拿它补行就是跨范围套条款。
func contractTierAdoption(tenant domain.TenantID, closure domain.CommercialClosure) (domain.AdoptedBasis, error) {
	var none domain.AdoptedBasis
	if tenant.String() == "" {
		return none, fmt.Errorf("tenant is required")
	}
	if closure.Outcome() != domain.UniquelyResolved {
		return none, fmt.Errorf("the closure is not uniquely resolved (%q)", closure.Outcome())
	}
	if closure.ResolutionKey().TenantID != tenant {
		return none, fmt.Errorf("tenant does not own this closure")
	}
	adopted, ok := closure.AdoptedFor(domain.CustomerServiceRuleObject)
	if !ok {
		return none, fmt.Errorf("the closure adopted no customer service rule")
	}
	rule := adopted.Version()
	if rule.ObjectID().String() == "" || rule.Version().String() == "" {
		return none, fmt.Errorf("the adopted rule identity is required")
	}
	if rule.Kind() != domain.CustomerServiceRuleObject {
		return none, fmt.Errorf("%q is not a customer service rule version", rule.Kind())
	}
	if rule.Tenant() != tenant {
		return none, fmt.Errorf("tenant does not own this rule")
	}
	if rule.Scope() != closure.ResolutionKey().Scope {
		return none, fmt.Errorf("the adopted rule is not in the closure's scope")
	}
	if _, contractTier := rule.ReferenceTo(domain.CustomerContractObject); !contractTier {
		return none, fmt.Errorf("the adopted rule %q is not a contract-tier version", rule.ObjectID())
	}
	return adopted, nil
}
