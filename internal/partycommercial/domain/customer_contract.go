package domain

import "errors"

var (
	ErrInvalidCustomerContract            = errors.New("party commercial: invalid customer contract")
	ErrInvalidFinancialControlBinding     = errors.New("party commercial: invalid financial control binding")
	ErrConflictingFinancialControlBinding = errors.New("party commercial: one charge scope is both applied and inapplicable")
)

// InapplicabilityBasis 是某个费用范围不带接受前财务控制的原因。本上下文要求显式
// 记录不适用依据而不是留空，因为 settlement-accounting 不得自行发明「无控制」——
// 零金额冻结不能顶替这个答复，所以这个答复必须存在。
type InapplicabilityBasis struct{ requiredValue }

func NewInapplicabilityBasis(value string) (InapplicabilityBasis, error) {
	required, err := newRequiredValue("inapplicability basis", value)
	return InapplicabilityBasis{required}, err
}

// FinancialControlBinding 表达合同对一个费用范围的约定：要么适用一份指名的策略，
// 要么显式不适用并记录依据。它的零值两者都不是，未绑定的范围因此读不成「允许通过」。
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

// CustomerContract 是一个客户合同版本的正文：它引用哪个接单规则包，以及它按费用
// 范围对财务控制作了什么约定。
type CustomerContract struct {
	version     CommercialVersion
	rulePackage CommercialObjectID
	bindings    map[ChargeScopeReference]FinancialControlBinding
}

// NewCustomerContract 要求在构造时就给出接单规则包引用。缺了它的合同不能用于接受
// 任何委托，而等到形成接受判断时才发现已经太晚；本上下文规定：规则或策略缺失不得
// 被解释为允许接受。
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
		// 同一范围被两种方式各约定一次，会让两种读法都说得通，而其中一种允许在
		// 没有控制结果的情况下接受。
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

// FinancialControlFor 报出合同对某个范围的约定。未绑定的范围答「不存在」而不是
// 「不适用」：「合同没说」与「合同说了此处无控制」是两个不同的事实，只有后者
// 允许在没有控制结果的情况下继续。
func (contract CustomerContract) FinancialControlFor(scope ChargeScopeReference) (FinancialControlBinding, bool) {
	binding, found := contract.bindings[scope]
	return binding, found
}
