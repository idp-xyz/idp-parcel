package domain

import "errors"

var (
	ErrInvalidAssembledRule            = errors.New("party commercial: invalid assembled rule")
	ErrInvalidRulePackageApplicability = errors.New("party commercial: invalid rule package applicability")
	ErrInvalidAcceptanceRulePackage    = errors.New("party commercial: invalid acceptance rule package")
)

// RuleReference points at a rule owned by whichever context enforces it. It is
// only a reference: the rule's content, thresholds and values stay with that
// owner, so assembling a package can never amount to choosing a judgement value
// for a specific shipment.
type RuleReference struct{ requiredValue }

func NewRuleReference(value string) (RuleReference, error) {
	required, err := newRequiredValue("rule reference", value)
	return RuleReference{required}, err
}

// RuleCategory is the closed set of partitions the context requires a package to
// distinguish. Regulatory source documents are a document requirement only:
// the formal customs determination stays with customs-compliance and must not be
// pulled forward into a commercial object.
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

// AssembledRule is one rule reference filed under one category. It holds a
// category and a reference and nothing else — there is structurally nowhere to
// put a threshold or a value, which is how "assembles references, never chooses
// values" stays true rather than merely intended.
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

// RulePackageApplicability is the five-dimension range a package is selected by:
// service product, customer contract, legal entity, service or customs scope and
// effective interval.
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

// AcceptanceRulePackage is the content of one acceptance rule package version:
// which rules apply, filed by category, over which range. It assembles other
// contexts' rules and holds no determination of its own.
type AcceptanceRulePackage struct {
	version       CommercialVersion
	applicability RulePackageApplicability
	rules         map[RuleCategory][]AssembledRule
}

// NewAcceptanceRulePackage refuses a package with no rules at all. An empty
// package would mean every shipment passes, and the context states that a
// missing rule must never be read as permission to accept.
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
