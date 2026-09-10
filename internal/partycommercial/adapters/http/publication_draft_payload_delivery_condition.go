package commercialhttp

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是交付条件声明（ADR-0133 决定四）在运营操作者面载荷里的那一节（票 admin-write-faces/25）。它不是第十一册：挂在
// 服务产品册（serviceProduct.deliveryConditions，产品层）或客户合同册（customerContract.deliveryConditions，合同层）各自
// 的正文格下；节内键名镜像受控批文 declarations.deliveryConditions 与规范化文档——同一节在三处说同一套词。

// ServiceProductBodyPayload 是服务产品册的正文格：今天只有一节可缺的产品层交付条件（票 09 时本册没有正文格，票 25 长出
// 这一格）。整格缺席与格在而节缺席都是「这一版没有交付条件」，折出的仍是两格文档。
type ServiceProductBodyPayload struct {
	DeliveryConditions *DeliveryConditionPayload `json:"deliveryConditions,omitempty"`
}

// DeliveryConditionPayload 镜像批文 deliveryConditionDocument：允许的交付方式集合、收件范围规则引用、交付证明规则引用，
// 三格都是开放引用（PAR-NET-09 / PAR-COM-05 / PAR-COM-06 实例半边）——表单收串不校验存在性、不内置任何一种方式；
// tightens 只在合同层：所收紧的服务产品版本（对象标识 + 版本号），操作者手填而不是从壳上的指名引用推（壳上只有对象没有
// 版本号，pc-gaps/11 判断题 2）。层与 tightens 的配对、零方式、同方式两行不在这里判：那是领域折成文档时的门，预览与录入
// 都会把它答成`未受理`带成因。
type DeliveryConditionPayload struct {
	Tightens            *TightenedProductPayload `json:"tightens,omitempty"`
	Methods             []string                 `json:"methods"`
	RecipientScopeRule  string                   `json:"recipientScopeRule"`
	ProofOfDeliveryRule string                   `json:"proofOfDeliveryRule"`
}

// TightenedProductPayload 是合同层指名的所收紧的服务产品版本。
type TightenedProductPayload struct {
	ObjectID string `json:"objectId"`
	Version  string `json:"version"`
}

// body 把服务产品正文格逐格过领域构造门；节缺席交回零值正文（领域对它折两格文档）。
func (payload ServiceProductBodyPayload) body(problems *PublicationPayloadProblems) domain.ServiceProductBody {
	var body domain.ServiceProductBody
	if payload.DeliveryConditions != nil {
		conditions := payload.DeliveryConditions.body(problems, "serviceProduct.deliveryConditions")
		body.DeliveryConditions = &conditions
	}
	return body
}

// body 把一节交付条件逐格过领域构造门。方式逐项点名（methods[i]），两条规则各点名自己那一格，tightens 在场时两格各点名；
// 空方式集合不在这里点名——「至少一种」是领域那道门的话（ErrDeliveryConditionNotConfigured），预览上答成`未受理`带成因。
func (payload DeliveryConditionPayload) body(problems *PublicationPayloadProblems, root string) domain.DeliveryConditionBody {
	var body domain.DeliveryConditionBody
	if payload.Tightens != nil {
		before := len(problems.Problems)
		objectID := requireField(problems, root+".tightens.objectId", domain.NewCommercialObjectID, payload.Tightens.ObjectID)
		version := requireField(problems, root+".tightens.version", domain.NewCommercialVersionLabel, payload.Tightens.Version)
		if len(problems.Problems) == before {
			tightens, err := domain.NewTightenedProductVersion(objectID, version)
			if err != nil {
				problems.add(root+".tightens", err)
			}
			body.Tightens = &tightens
		}
	}
	body.Terms.Methods = make([]domain.DeliveryMethodReference, 0, len(payload.Methods))
	for index, raw := range payload.Methods {
		body.Terms.Methods = append(body.Terms.Methods,
			requireField(problems, fmt.Sprintf("%s.methods[%d]", root, index), domain.NewDeliveryMethodReference, raw))
	}
	body.Terms.RecipientScopeRule = requireField(problems, root+".recipientScopeRule", domain.NewDeliveryRuleReference, payload.RecipientScopeRule)
	body.Terms.ProofOfDeliveryRule = requireField(problems, root+".proofOfDeliveryRule", domain.NewDeliveryRuleReference, payload.ProofOfDeliveryRule)
	return body
}
