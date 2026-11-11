package domain

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrInvalidCustomerContract            = errors.New("party commercial: invalid customer contract")
	ErrInvalidFinancialControlBinding     = errors.New("party commercial: invalid financial control binding")
	ErrConflictingFinancialControlBinding = errors.New("party commercial: one charge scope is both applied and inapplicable")
	// ErrRulePackageReferenceMismatch 是装载核对：版本壳 ReferenceTo(AcceptanceRulePackageObject) 指名了规则包，
	// 正文必须指名同一个。两处不是一处派生另一处（declaredReferences 不强制合同指名规则包），但壳上写了、正文没写
	// 或写了另一个，消费方就会各读各的——壳上那一格解析拿去确认（ADR-0032），正文那一格目录拿去展示。壳上没指名
	// 时正文写不写都放行：正文的这一格只是说明，不参与选择。
	ErrRulePackageReferenceMismatch = errors.New("party commercial: contract content rule package disagrees with the version shell")
	// ErrRulePackageMissingFromContent 是同一道核对的另一格：版本壳指名了规则包而正文漏写。与「写了却不一样」分开说，要改的
	// 地方不同；包住 ErrRulePackageReferenceMismatch，按那一格认的调用方照旧认得。
	ErrRulePackageMissingFromContent = fmt.Errorf("%w: the version shell names a rule package the contract content leaves out",
		ErrRulePackageReferenceMismatch)
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

// CustomerContract 是一个客户合同版本的正文：它属于哪个货主客户账户、（可缺地）指名哪个接单规则包，以及它按
// 费用范围对财务控制作了什么约定。
type CustomerContract struct {
	version         CommercialVersion
	customerAccount CustomerAccountID
	rulePackage     CommercialObjectID
	bindings        map[ChargeScopeReference]FinancialControlBinding
}

// NewCustomerContract 要求在构造时就给出货主客户账户。缺了客户账户，闭包就答不出这份合同是不是键上那一户的
// （CONTEXT「合同版本必须明确关联…货主客户账户」，ADR-0177 决定一），不从委派人、客户关系或范围推。
//
// 接单规则包可缺（ADR-0376 决定三）：规则包随服务产品选——规则包版本指名它的服务产品，解析按委托申报的产品收窄
// （ADR-0153 决定四）——一个货主账户同时用几个各带规则包的服务产品时，合同指名哪一份都不对。「规则缺失不得被解释为
// 允许接受」照旧成立，守它的是解析：申报的产品解不出规则包，受理就停在无适用依据，不在合同这一格。rulePackage 传
// 零值即不指名。
func NewCustomerContract(
	version CommercialVersion,
	customerAccount CustomerAccountID,
	rulePackage CommercialObjectID,
	bindings []FinancialControlBinding,
) (CustomerContract, error) {
	if version.kind != CustomerContractObject ||
		version.status != CommercialVersionEffective ||
		!customerAccount.valid() {
		return CustomerContract{}, ErrInvalidCustomerContract
	}

	declared, err := declaredBindingsByScope(bindings)
	if err != nil {
		return CustomerContract{}, err
	}
	return CustomerContract{
		version: version, customerAccount: customerAccount, rulePackage: rulePackage, bindings: declared,
	}, nil
}

// CustomerAccount 报出这份合同属于哪个货主客户账户。
func (contract CustomerContract) CustomerAccount() CustomerAccountID {
	return contract.customerAccount
}

// declaredBindingsByScope 是约定表的那两道判：每条都经过了构造门（declared 且范围非零）、同一范围只约定一次。
// 抽成一处是让发布前的正文输入面（CustomerContractBody）与已生效版本的正文过同一道门——规则只在这里。
func declaredBindingsByScope(bindings []FinancialControlBinding) (map[ChargeScopeReference]FinancialControlBinding, error) {
	declared := make(map[ChargeScopeReference]FinancialControlBinding, len(bindings))
	for _, binding := range bindings {
		if !binding.declared || !binding.scope.valid() {
			return nil, ErrInvalidFinancialControlBinding
		}
		// 同一范围被两种方式各约定一次，会让两种读法都说得通，而其中一种允许在
		// 没有控制结果的情况下接受。
		if _, exists := declared[binding.scope]; exists {
			return nil, ErrConflictingFinancialControlBinding
		}
		declared[binding.scope] = binding
	}
	return declared, nil
}

func (contract CustomerContract) Version() CommercialVersion {
	return contract.version
}

// AcceptanceRulePackage 的第二个返回值为假即这份合同没指名接单规则包（ADR-0376 决定三），不是读不到。
func (contract CustomerContract) AcceptanceRulePackage() (CommercialObjectID, bool) {
	return contract.rulePackage, contract.rulePackage.valid()
}

// FinancialControlFor 报出合同对某个范围的约定。未绑定的范围答「不存在」而不是
// 「不适用」：「合同没说」与「合同说了此处无控制」是两个不同的事实，只有后者
// 允许在没有控制结果的情况下继续。
func (contract CustomerContract) FinancialControlFor(scope ChargeScopeReference) (FinancialControlBinding, bool) {
	binding, found := contract.bindings[scope]
	return binding, found
}

// Bindings 按费用范围的稳定顺序交回全部约定（副本）。范围是开放引用、没有可枚举的
// 封闭集，发布写入面只能由本访问器交出集合；逐范围取用仍走 FinancialControlFor。
func (contract CustomerContract) Bindings() []FinancialControlBinding {
	bindings := make([]FinancialControlBinding, 0, len(contract.bindings))
	for _, binding := range contract.bindings {
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(left, right int) bool {
		return bindings[left].scope.String() < bindings[right].scope.String()
	})
	return bindings
}

// ConsistentAcceptanceRulePackage 核版本壳与正文的规则包引用，登记与装载两处共用。壳上没指名时放行，正文写不写都行——正文
// 那一格只是说明，确认只读壳上的引用（ADR-0032）；壳上指名了，正文要写同一个：正文漏写答 ErrRulePackageMissingFromContent，
// 写了另一个答 ErrRulePackageReferenceMismatch，不静默选一处。content 零值即正文没指名。
func ConsistentAcceptanceRulePackage(version CommercialVersion, content CommercialObjectID) error {
	named, present := version.ReferenceTo(AcceptanceRulePackageObject)
	if !present {
		return nil
	}
	if !content.valid() {
		return ErrRulePackageMissingFromContent
	}
	if named != content {
		return ErrRulePackageReferenceMismatch
	}
	return nil
}
