package domain

import (
	"fmt"
	"sort"
)

// 本文件是客户合同册接进服务端规范化的那一格（加册不换号，仍是 PCC-1——ADR-0126 Decision 一；票
// admin-write-faces/10）。合同版本的正文是两层声明（ADR-0115）：0012 的正文（接单规则包 + 按费用范围的
// 财务控制约定）与 0007 的合同级「要不要接受前财务控制」声明。受控批文把它们写成 declarations 下并列的
// contractContent 与 preAcceptanceControl 两键；这里把两层折进本册一格，键名镜像批文。

// CustomerContractBody 是客户合同版本的正文输入面，以领域值对象给出：NewCustomerContract 与
// DeclarePreAcceptanceControl 各收的那几项，只是不带拥有它们的（已生效）版本——预览与录入发生在发布之前。
//
// Control 可缺：批文里 preAcceptanceControl 缺键是合法的（本版没说「要不要」，下游到接受判断时得到的是
// `未声明`，不是放行），缺席不折进文档、不影响正文那一层的字节。RulePackage 与约定表不可缺席——它们就是
// 0012 的正文，读面「正文未登记」说的正是这一层。
type CustomerContractBody struct {
	RulePackage CommercialObjectID
	Bindings    []FinancialControlBinding
	Control     *PreAcceptanceControlBody
}

// PreAcceptanceControlBody 是合同级声明的输入面：要求二值，加只在`不适用`时在场的依据。
type PreAcceptanceControlBody struct {
	Requirement PreAcceptanceControlRequirement
	Basis       ControlNotApplicableBasis
}

// validate 在折成文档前把两层各过一遍与发布时相同的门：规则包非零（NewCustomerContract 的那一判）、约定表经
// declaredBindingsByScope、声明在场时经 preAcceptanceControlDeclared。不另造校验——三处判的都是同一条规则，
// 而预览要在录入之前就把「恰一」与「不适用必带依据」答给操作者，不能等到发布那一刻。
func (body CustomerContractBody) validate() error {
	if !body.RulePackage.valid() {
		return ErrInvalidCustomerContract
	}
	if _, err := declaredBindingsByScope(body.Bindings); err != nil {
		return err
	}
	if body.Control != nil {
		return preAcceptanceControlDeclared(body.Control.Requirement, body.Control.Basis)
	}
	return nil
}

// canonicalCustomerContractBody 镜像批文 declarations 下 contractContent / preAcceptanceControl 两键的形状。
// 约定表按费用范围排序写出：合同把约定按范围成表（FinancialControlFor 按范围取、Bindings() 按范围序交出），
// 表单里换行序不是换正文，摘要不该跟着变。
type canonicalCustomerContractBody struct {
	ContractContent      canonicalContractContent       `json:"contractContent"`
	PreAcceptanceControl *canonicalPreAcceptanceControl `json:"preAcceptanceControl,omitempty"`
}

type canonicalContractContent struct {
	RulePackage string                    `json:"rulePackage"`
	Bindings    []canonicalControlBinding `json:"bindings,omitempty"`
}

// canonicalControlBinding 一行只带在场的那一格：指名策略行只有 policy，显式不适用行只有 inapplicabilityBasis
// ——恰一由构造门守，文档不给第二格留位置。
type canonicalControlBinding struct {
	ChargeScope          string `json:"chargeScope"`
	Policy               string `json:"policy,omitempty"`
	InapplicabilityBasis string `json:"inapplicabilityBasis,omitempty"`
}

type canonicalPreAcceptanceControl struct {
	Requirement        string `json:"requirement"`
	NotApplicableBasis string `json:"notApplicableBasis,omitempty"`
}

func canonicalCustomerContractBodyOf(body CustomerContractBody) *canonicalCustomerContractBody {
	bindings := make([]FinancialControlBinding, len(body.Bindings))
	copy(bindings, body.Bindings)
	sort.Slice(bindings, func(left, right int) bool {
		return bindings[left].Scope().String() < bindings[right].Scope().String()
	})
	document := &canonicalCustomerContractBody{
		ContractContent: canonicalContractContent{RulePackage: body.RulePackage.String()},
	}
	for _, binding := range bindings {
		row := canonicalControlBinding{ChargeScope: binding.Scope().String()}
		if policy, applies := binding.Policy(); applies {
			row.Policy = policy.String()
		} else {
			row.InapplicabilityBasis = binding.InapplicabilityBasis().String()
		}
		document.ContractContent.Bindings = append(document.ContractContent.Bindings, row)
	}
	if body.Control != nil {
		document.PreAcceptanceControl = &canonicalPreAcceptanceControl{Requirement: body.Control.Requirement.String()}
		if body.Control.Requirement == PreAcceptanceControlNotApplicable {
			document.PreAcceptanceControl.NotApplicableBasis = body.Control.Basis.String()
		}
	}
	return document
}

// body 把文档里的一节折回领域正文。每一格过构造门；约定行两格恰一由 financialControlBindingOf 判；同一范围
// 两行、声明缺依据这类跨格的问题留给 validate——快照是数据，正文立不立得住仍由构造门说。
func (document canonicalCustomerContractBody) body() (CustomerContractBody, error) {
	rulePackage, err := NewCommercialObjectID(document.ContractContent.RulePackage)
	if err != nil {
		return CustomerContractBody{}, fmt.Errorf("contractContent.rulePackage: %w", err)
	}
	body := CustomerContractBody{RulePackage: rulePackage}
	for _, row := range document.ContractContent.Bindings {
		binding, err := financialControlBindingOf(row.ChargeScope, row.Policy, row.InapplicabilityBasis)
		if err != nil {
			return CustomerContractBody{}, fmt.Errorf("contractContent.bindings: %w", err)
		}
		body.Bindings = append(body.Bindings, binding)
	}
	if document.PreAcceptanceControl != nil {
		requirement, known := PreAcceptanceControlRequirementNamed(document.PreAcceptanceControl.Requirement)
		if !known {
			return CustomerContractBody{}, fmt.Errorf("preAcceptanceControl.requirement: %w: %q",
				ErrPreAcceptanceControlNotDeclared, document.PreAcceptanceControl.Requirement)
		}
		control := PreAcceptanceControlBody{Requirement: requirement}
		if document.PreAcceptanceControl.NotApplicableBasis != "" {
			if control.Basis, err = NewControlNotApplicableBasis(document.PreAcceptanceControl.NotApplicableBasis); err != nil {
				return CustomerContractBody{}, fmt.Errorf("preAcceptanceControl.notApplicableBasis: %w", err)
			}
		}
		body.Control = &control
	}
	if err := body.validate(); err != nil {
		return CustomerContractBody{}, err
	}
	return body, nil
}

// financialControlBindingOf 把文档里并存的两键折回两格封闭的约定：恰一在场才立得住，两空或两满是文档与领域分叉
// （判据同 creditLimitOf）。
func financialControlBindingOf(chargeScope, policy, inapplicabilityBasis string) (FinancialControlBinding, error) {
	scope, err := NewChargeScopeReference(chargeScope)
	if err != nil {
		return FinancialControlBinding{}, err
	}
	switch {
	case policy != "" && inapplicabilityBasis == "":
		named, err := NewCommercialObjectID(policy)
		if err != nil {
			return FinancialControlBinding{}, err
		}
		return NewAppliedFinancialControl(scope, named)
	case policy == "" && inapplicabilityBasis != "":
		basis, err := NewInapplicabilityBasis(inapplicabilityBasis)
		if err != nil {
			return FinancialControlBinding{}, err
		}
		return NewInapplicableFinancialControl(scope, basis)
	default:
		return FinancialControlBinding{}, fmt.Errorf("%w: charge scope %q must name exactly one of policy or inapplicabilityBasis",
			ErrInvalidFinancialControlBinding, chargeScope)
	}
}
