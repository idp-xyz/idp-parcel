package domain

import "errors"

var (
	// ErrPreAcceptanceControlNotDeclared 是声明缺件：要求未明确，或声明`不适用`却说不出
	// 依据。恢复动作是把客户合同正文声明补齐（实例半边），不是本上下文代拟默认。
	ErrPreAcceptanceControlNotDeclared = errors.New("party commercial: pre-acceptance control is not declared")
	// ErrUnusableContract：非已生效客户合同承载不了这份声明。与 ErrUnusableRulePackage
	// 同判据同恢复动作（换当前可用的版本），只是拥有对象不同。
	ErrUnusableContract = errors.New("party commercial: customer contract cannot carry declarations")
)

// PreAcceptanceControlRequirement 是客户合同对「这个范围要不要接受前财务控制」的封闭
// 二值。零值 = 未声明。
//
// 它只答「要不要」。**控制用什么方式、实际采用哪一版政策不在这里**——那属结算政策的
// 解析（ADR-0044），两层压成一层就会从结算方式倒推出控制要不要，而 `pn-02-w03` 明写
// 账期不能推导无需信用校验。
type PreAcceptanceControlRequirement uint8

const (
	PreAcceptanceControlUndeclared PreAcceptanceControlRequirement = iota
	PreAcceptanceControlRequired
	PreAcceptanceControlNotApplicable
)

func (requirement PreAcceptanceControlRequirement) Declared() bool {
	return requirement == PreAcceptanceControlRequired ||
		requirement == PreAcceptanceControlNotApplicable
}

func (requirement PreAcceptanceControlRequirement) String() string {
	switch requirement {
	case PreAcceptanceControlRequired:
		return "REQUIRED"
	case PreAcceptanceControlNotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

// ControlNotApplicableBasis 指名合同凭什么说这个范围不需要接受前财务控制。
//
// CONTEXT 要求「合同明确无接受前财务控制时必须保存商业不适用依据，不能用缺失结果或
// 默认通过代替」——没有依据的`不适用`与一次默认放行分不开，而后者正是那句话禁的。
type ControlNotApplicableBasis struct{ requiredValue }

func NewControlNotApplicableBasis(value string) (ControlNotApplicableBasis, error) {
	required, err := newRequiredValue("control not-applicable basis", value)
	return ControlNotApplicableBasis{required}, err
}

// PreAcceptanceControlDeclaration 是一份客户合同版本对接受前财务控制的声明。
//
// 拥有对象是客户合同版本而不是控制策略版本：`PAR-COM-15` 列在合同版本下，而且`不适用`
// 的依据本身也是合同层面的话——策略版本只回答「控制怎么做」，回答不了「这份合同要不要」。
type PreAcceptanceControlDeclaration struct {
	contract    CommercialVersion
	requirement PreAcceptanceControlRequirement
	basis       ControlNotApplicableBasis
}

// DeclarePreAcceptanceControl 把声明绑定到客户合同版本上。
//
// 按取值分片校验：`不适用`必须带依据（见 ControlNotApplicableBasis）；`要求控制`不得带
// 依据——那条依据的含义就是「凭什么不控制」，挂在要求控制上读不出任何东西，两头都带的
// 声明分不出它到底是哪一格。
func DeclarePreAcceptanceControl(
	contract CommercialVersion,
	requirement PreAcceptanceControlRequirement,
	basis ControlNotApplicableBasis,
) (PreAcceptanceControlDeclaration, error) {
	if contract.kind != CustomerContractObject ||
		contract.status != CommercialVersionEffective {
		return PreAcceptanceControlDeclaration{}, ErrUnusableContract
	}
	switch requirement {
	case PreAcceptanceControlRequired:
		if basis.valid() {
			return PreAcceptanceControlDeclaration{}, ErrPreAcceptanceControlNotDeclared
		}
	case PreAcceptanceControlNotApplicable:
		if !basis.valid() {
			return PreAcceptanceControlDeclaration{}, ErrPreAcceptanceControlNotDeclared
		}
	default:
		return PreAcceptanceControlDeclaration{}, ErrPreAcceptanceControlNotDeclared
	}
	return PreAcceptanceControlDeclaration{
		contract:    contract,
		requirement: requirement,
		basis:       basis,
	}, nil
}

func (declaration PreAcceptanceControlDeclaration) Contract() CommercialVersion {
	return declaration.contract
}

func (declaration PreAcceptanceControlDeclaration) Requirement() PreAcceptanceControlRequirement {
	return declaration.requirement
}

// NotApplicableBasis 只在`不适用`时给出依据。第二个返回值为 false 即「这份声明要求控制」
// ——不是「依据丢了」。
func (declaration PreAcceptanceControlDeclaration) NotApplicableBasis() (ControlNotApplicableBasis, bool) {
	return declaration.basis, declaration.requirement == PreAcceptanceControlNotApplicable
}
