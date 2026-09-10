package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是客户合同册在运营操作者面载荷里的那一格（票 admin-write-faces/10）。一格三层：0012 的正文
// （contractContent）、0007 的合同级接受前控制声明（preAcceptanceControl）与 0030 的合同层交付条件（deliveryConditions，
// 票 admin-write-faces/25），键名镜像受控批文 declarations 下的三键与规范化文档——同一册在三处（批文、载荷、文档）说
// 同一套词。

// CustomerContractBodyPayload 是客户合同册的正文载荷。contractContent 是指针，是为了让「整节没给」与「给了但规则包
// 留白」在逐格问题里落在不同的格：前者点名整节，后者点名 rulePackage。preAcceptanceControl 可缺——本版不声明「要不要」
// 是合法输入，表单不代填一格；deliveryConditions 同理可缺——这一版没有合同层交付条件就是没有，不默认沿用产品层。
type CustomerContractBodyPayload struct {
	ContractContent      *ContractContentPayload      `json:"contractContent"`
	PreAcceptanceControl *PreAcceptanceControlPayload `json:"preAcceptanceControl,omitempty"`
	DeliveryConditions   *DeliveryConditionPayload    `json:"deliveryConditions,omitempty"`
}

// ContractContentPayload 镜像批文 contractContentDocument：接单规则包引用与按费用范围的约定表（可为空——
// 「已登记，未对任何费用范围作约定」是读面认得的一格）。
type ContractContentPayload struct {
	RulePackage string                  `json:"rulePackage"`
	Bindings    []ControlBindingPayload `json:"bindings,omitempty"`
}

// ControlBindingPayload 是一行约定：费用范围 × 「指名策略 / 显式不适用依据」二选一。恰一由解码判并点名这一行；
// 策略侧没有「无控制」取值——「明确无控制」只能经 inapplicabilityBasis 或合同级声明表达（ADR-0115 Decision 一）。
type ControlBindingPayload struct {
	ChargeScope          string `json:"chargeScope"`
	Policy               string `json:"policy,omitempty"`
	InapplicabilityBasis string `json:"inapplicabilityBasis,omitempty"`
}

// PreAcceptanceControlPayload 镜像批文 preAcceptanceControlDocument：要求二值原词 + 只在`不适用`时在场的依据。
// 配对成立不成立（`不适用`必带依据、`要求控制`不得带）不在这里判：那是领域 DeclarePreAcceptanceControl 一族的门，
// 预览与录入都会把它答成`未受理`带成因。
type PreAcceptanceControlPayload struct {
	Requirement        string `json:"requirement"`
	NotApplicableBasis string `json:"notApplicableBasis,omitempty"`
}

// body 把客户合同载荷逐格过领域构造门。约定行逐行点名（bindings[i]），同一范围两行这类跨行的问题留给领域规范化。
func (payload CustomerContractBodyPayload) body(problems *PublicationPayloadProblems) domain.CustomerContractBody {
	var body domain.CustomerContractBody
	if payload.ContractContent == nil {
		problems.add("customerContract.contractContent", fmt.Errorf("合同正文（规则包与按费用范围的约定）须在场"))
	} else {
		body.RulePackage = requireField(problems, "customerContract.contractContent.rulePackage",
			domain.NewCommercialObjectID, payload.ContractContent.RulePackage)
		body.Bindings = make([]domain.FinancialControlBinding, 0, len(payload.ContractContent.Bindings))
		for index, row := range payload.ContractContent.Bindings {
			body.Bindings = append(body.Bindings, row.binding(problems, fmt.Sprintf("customerContract.contractContent.bindings[%d]", index)))
		}
	}
	if payload.PreAcceptanceControl != nil {
		control := payload.PreAcceptanceControl.body(problems)
		body.Control = &control
	}
	if payload.DeliveryConditions != nil {
		conditions := payload.DeliveryConditions.body(problems, "customerContract.deliveryConditions")
		body.DeliveryConditions = &conditions
	}
	return body
}

// binding 把一行约定折成领域值对象。两格恰一：两空与两满都是这一行的问题，不由这里挑一个；范围与被选的那一格各自
// 过构造门后再拼——上面某格已记过问题时不再拼，免得把同一格的零值再记一遍。
func (payload ControlBindingPayload) binding(problems *PublicationPayloadProblems, field string) domain.FinancialControlBinding {
	before := len(problems.Problems)
	scope := requireField(problems, field+".chargeScope", domain.NewChargeScopeReference, payload.ChargeScope)
	switch {
	case payload.Policy != "" && payload.InapplicabilityBasis == "":
		policy := requireField(problems, field+".policy", domain.NewCommercialObjectID, payload.Policy)
		if len(problems.Problems) != before {
			return domain.FinancialControlBinding{}
		}
		binding, err := domain.NewAppliedFinancialControl(scope, policy)
		if err != nil {
			problems.add(field, err)
		}
		return binding
	case payload.Policy == "" && payload.InapplicabilityBasis != "":
		basis := requireField(problems, field+".inapplicabilityBasis", domain.NewInapplicabilityBasis, payload.InapplicabilityBasis)
		if len(problems.Problems) != before {
			return domain.FinancialControlBinding{}
		}
		binding, err := domain.NewInapplicableFinancialControl(scope, basis)
		if err != nil {
			problems.add(field, err)
		}
		return binding
	default:
		problems.add(field, fmt.Errorf("约定须恰一格在场：policy 或 inapplicabilityBasis"))
		return domain.FinancialControlBinding{}
	}
}

// body 把合同级声明逐格过门：要求按 String() 原词反查（集合外含空串都是这一格的问题），依据在场时过构造门。
func (payload PreAcceptanceControlPayload) body(problems *PublicationPayloadProblems) domain.PreAcceptanceControlBody {
	requirement, known := domain.PreAcceptanceControlRequirementNamed(payload.Requirement)
	if !known {
		problems.add("customerContract.preAcceptanceControl.requirement", fmt.Errorf("集合外的控制要求 %q", payload.Requirement))
	}
	body := domain.PreAcceptanceControlBody{Requirement: requirement}
	if payload.NotApplicableBasis != "" {
		body.Basis = requireField(problems, "customerContract.preAcceptanceControl.notApplicableBasis",
			domain.NewControlNotApplicableBasis, payload.NotApplicableBasis)
	}
	return body
}
