package domain

import "errors"

var (
	ErrInvalidAssembledRule            = errors.New("party commercial: invalid assembled rule")
	ErrInvalidRulePackageApplicability = errors.New("party commercial: invalid rule package applicability")
	ErrInvalidAcceptanceRulePackage    = errors.New("party commercial: invalid acceptance rule package")
)

// RuleReference 指向一条规则，其所有权属于执行该规则的那个上下文。它只是引用：
// 规则正文、阈值和取值都留在权威方那里，所以装配规则包永远不等于替某个具体委托
// 选择判断值。
type RuleReference struct{ requiredValue }

func NewRuleReference(value string) (RuleReference, error) {
	required, err := newRequiredValue("rule reference", value)
	return RuleReference{required}, err
}

// RuleCategory 是本上下文要求规则包必须区分的封闭分区集合。监管原始资料只是资料
// 要求：正式关务判断留在 customs-compliance，不得前移到商业对象里。
type RuleCategory uint8

const (
	RuleCategoryInvalid RuleCategory = iota
	MinimumIngressIdentityRules
	ShipmentInvariantRules
	ProductAndContractDocumentRules
	RegulatorySourceDocumentRules
	CrossFieldConditionRules
)

func (category RuleCategory) valid() bool {
	return category >= MinimumIngressIdentityRules && category <= CrossFieldConditionRules
}

func (category RuleCategory) String() string {
	switch category {
	case MinimumIngressIdentityRules:
		return "MINIMUM_INGRESS_IDENTITY"
	case ShipmentInvariantRules:
		return "SHIPMENT_INVARIANT"
	case ProductAndContractDocumentRules:
		return "PRODUCT_AND_CONTRACT_DOCUMENT"
	case RegulatorySourceDocumentRules:
		return "REGULATORY_SOURCE_DOCUMENT"
	case CrossFieldConditionRules:
		return "CROSS_FIELD_CONDITION"
	default:
		return ""
	}
}

// AssembledRule 是归入某一分类的一条规则引用。它只持有分类和引用，别无他物——
// 结构上就没有地方放阈值或取值，「只装配引用、绝不选择取值」因此是事实而不只是意图。
type AssembledRule struct {
	category  RuleCategory
	reference RuleReference
}

func NewAssembledRule(category RuleCategory, reference RuleReference) (AssembledRule, error) {
	if !category.valid() || !reference.valid() {
		return AssembledRule{}, ErrInvalidAssembledRule
	}
	return AssembledRule{category: category, reference: reference}, nil
}

func (rule AssembledRule) Category() RuleCategory {
	return rule.category
}

func (rule AssembledRule) Reference() RuleReference {
	return rule.reference
}

// RulePackageApplicability 是选择规则包所依据的五个维度：服务产品、客户合同、
// 责任法人、服务或关务范围，以及有效期间。
type RulePackageApplicability struct {
	serviceProduct CommercialObjectID
	contract       CommercialObjectID
	legalEntity    LegalEntityReference
	scope          CommercialScopeReference
	effective      EffectiveInterval
}

func NewRulePackageApplicability(
	serviceProduct CommercialObjectID,
	contract CommercialObjectID,
	legalEntity LegalEntityReference,
	scope CommercialScopeReference,
	effective EffectiveInterval,
) (RulePackageApplicability, error) {
	if !serviceProduct.valid() || !contract.valid() || !legalEntity.valid() ||
		!scope.valid() || !effective.valid() {
		return RulePackageApplicability{}, ErrInvalidRulePackageApplicability
	}
	return RulePackageApplicability{
		serviceProduct: serviceProduct,
		contract:       contract,
		legalEntity:    legalEntity,
		scope:          scope,
		effective:      effective,
	}, nil
}

func (applicability RulePackageApplicability) ServiceProduct() CommercialObjectID {
	return applicability.serviceProduct
}

func (applicability RulePackageApplicability) Contract() CommercialObjectID {
	return applicability.contract
}

func (applicability RulePackageApplicability) Scope() CommercialScopeReference {
	return applicability.scope
}

func (applicability RulePackageApplicability) Effective() EffectiveInterval {
	return applicability.effective
}

// AcceptanceRulePackage 是一个接单规则包版本的正文：在哪个适用范围内、按分类归档
// 的哪些规则适用。它只装配其他上下文的规则，自身不持有任何判断。
type AcceptanceRulePackage struct {
	version       CommercialVersion
	applicability RulePackageApplicability
	rules         map[RuleCategory][]AssembledRule
}

// NewAcceptanceRulePackage 拒绝一条规则都没有的规则包。空规则包意味着任何委托都能
// 通过，而本上下文规定：规则缺失不得被解释为允许接受。
func NewAcceptanceRulePackage(
	version CommercialVersion,
	applicability RulePackageApplicability,
	rules []AssembledRule,
) (AcceptanceRulePackage, error) {
	if version.kind != AcceptanceRulePackageObject ||
		version.status != CommercialVersionEffective ||
		len(rules) == 0 {
		return AcceptanceRulePackage{}, ErrInvalidAcceptanceRulePackage
	}

	filed := make(map[RuleCategory][]AssembledRule, len(rules))
	for _, rule := range rules {
		if !rule.category.valid() || !rule.reference.valid() {
			return AcceptanceRulePackage{}, ErrInvalidAssembledRule
		}
		filed[rule.category] = append(filed[rule.category], rule)
	}
	return AcceptanceRulePackage{version: version, applicability: applicability, rules: filed}, nil
}

func (pack AcceptanceRulePackage) Version() CommercialVersion {
	return pack.version
}

func (pack AcceptanceRulePackage) Applicability() RulePackageApplicability {
	return pack.applicability
}

func (pack AcceptanceRulePackage) RulesIn(category RuleCategory) []AssembledRule {
	return append([]AssembledRule(nil), pack.rules[category]...)
}
