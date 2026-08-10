package domain

import "errors"

var (
	ErrInvalidCustomerContract            = errors.New("party commercial: invalid customer contract")
	ErrInvalidFinancialControlBinding     = errors.New("party commercial: invalid financial control binding")
	ErrConflictingFinancialControlBinding = errors.New("party commercial: one charge scope is both applied and inapplicable")
)

// InapplicabilityBasis is why a charge scope carries no pre-acceptance financial
// control. The context requires an explicit basis rather than an omission,
// because settlement-accounting is forbidden from inventing "no control" on its
// own — a zero-amount freeze may not stand in for this answer, so this answer has
// to exist.
type InapplicabilityBasis struct{ requiredValue }

func NewInapplicabilityBasis(value string) (InapplicabilityBasis, error) {
	required, err := newRequiredValue("inapplicability basis", value)
	return InapplicabilityBasis{required}, err
}

// FinancialControlBinding says what a contract arranges for one charge scope:
// either a named policy applies, or control is explicitly inapplicable with a
// recorded basis. Its zero value is neither, which is what keeps an unbound
// scope from reading as permission.
type FinancialControlBinding struct {
	scope    ChargeScopeReference
	policy   CommercialObjectID
	basis    InapplicabilityBasis
	declared bool
}

func NewAppliedFinancialControl(scope ChargeScopeReference, policy CommercialObjectID) (FinancialControlBinding, error) {
	if !scope.valid() || !policy.valid() {
		return FinancialControlBinding{}, ErrInvalidFinancialControlBinding
	}
	return FinancialControlBinding{scope: scope, policy: policy, declared: true}, nil
}

func NewInapplicableFinancialControl(scope ChargeScopeReference, basis InapplicabilityBasis) (FinancialControlBinding, error) {
	if !scope.valid() || !basis.valid() {
		return FinancialControlBinding{}, ErrInvalidFinancialControlBinding
	}
	return FinancialControlBinding{scope: scope, basis: basis, declared: true}, nil
}

func (binding FinancialControlBinding) Scope() ChargeScopeReference {
	return binding.scope
}

func (binding FinancialControlBinding) Applies() bool {
	return binding.declared && binding.policy.valid()
}

func (binding FinancialControlBinding) ExplicitlyInapplicable() bool {
	return binding.declared && binding.basis.valid()
}

func (binding FinancialControlBinding) Policy() (CommercialObjectID, bool) {
	if !binding.Applies() {
		return CommercialObjectID{}, false
	}
	return binding.policy, true
}

func (binding FinancialControlBinding) InapplicabilityBasis() InapplicabilityBasis {
	return binding.basis
}

// CustomerContract is the content of one customer contract version: which
// acceptance rule package governs it, and what it arranges for financial control
// per charge scope.
type CustomerContract struct {
	version     CommercialVersion
	rulePackage CommercialObjectID
	bindings    map[ChargeScopeReference]FinancialControlBinding
}

// NewCustomerContract requires the rule package reference up front. A contract
// missing it could not be used to accept anything, and discovering that during
// an acceptance decision would be discovering it too late; the context states
// that a missing rule or policy must never be read as permission to accept.
func NewCustomerContract(
	version CommercialVersion,
	rulePackage CommercialObjectID,
	bindings []FinancialControlBinding,
) (CustomerContract, error) {
	if version.kind != CustomerContractObject ||
		version.status != CommercialVersionEffective ||
		!rulePackage.valid() {
		return CustomerContract{}, ErrInvalidCustomerContract
	}

	declared := make(map[ChargeScopeReference]FinancialControlBinding, len(bindings))
	for _, binding := range bindings {
		if !binding.declared || !binding.scope.valid() {
			return CustomerContract{}, ErrInvalidFinancialControlBinding
		}
		// A scope arranged both ways would make both readings defensible, and one
		// of them permits acceptance without control.
		if _, exists := declared[binding.scope]; exists {
			return CustomerContract{}, ErrConflictingFinancialControlBinding
		}
		declared[binding.scope] = binding
	}
	return CustomerContract{version: version, rulePackage: rulePackage, bindings: declared}, nil
}

func (contract CustomerContract) Version() CommercialVersion {
	return contract.version
}

func (contract CustomerContract) AcceptanceRulePackage() CommercialObjectID {
	return contract.rulePackage
}

// FinancialControlFor reports what the contract arranges for a scope. An unbound
// scope answers absent rather than inapplicable: "the contract says nothing" and
// "the contract says no control applies" are different facts, and only the
// second one permits proceeding without a control result.
func (contract CustomerContract) FinancialControlFor(scope ChargeScopeReference) (FinancialControlBinding, bool) {
	binding, found := contract.bindings[scope]
	return binding, found
}
