package domain

import (
	"fmt"
	"sort"
)

// 本文件是交付条件声明（ADR-0133 决定四，票 party-commercial-context-gaps/11）折进 PCC-1 的那一节（票
// admin-write-faces/25）。它不是第十一册：交付条件挂在服务产品版本（产品层）或客户合同版本（合同层）上，所以
// 在规范化文档里是那两册各自正文下的一节 deliveryConditions，键名镜像受控批文 declarations.deliveryConditions
// ——同一节在三处（批文、载荷、文档）说同一套词。加的全是 omitempty 键，不换号（ADR-0126 Decision 一）：不带
// 这一节的文档字节与接进之前逐字节同。
//
// 为什么必须折进文档而不是只加载荷：表单路径的发布不重读载荷——待批准载体存的正文快照就是这份文档（ADR-0126
// Decision 三），发布那一步从文档折回正文再交发布用例。不在文档里的声明，发布时就不存在。

// DeliveryConditionBody 是一层交付条件声明的正文输入面：正文三格加合同层独有的「所收紧的产品版本」。它就是
// DeclareProductDeliveryConditions / DeclareContractDeliveryConditions 各收的那几项，只是不带拥有它的（已生效）
// 版本——预览与录入发生在发布之前，那时没有已生效版本可挂。Tightens 缺席即产品层、在场即合同层：层由它说，
// 与发布用例按声明里有没有 tightens 选层是同一条判据。
type DeliveryConditionBody struct {
	Tightens *TightenedProductVersion
	Terms    DeliveryConditionTerms
}

// validate 在折成文档前把这一节过一遍与发布时相同的门，判据与发布用例按版本类别选层那一段逐字同：客户合同版本走合同层
// ——缺 tightens 是声明缺件（ErrDeliveryConditionNotConfigured，合同层必须指名所收紧的产品版本）；服务产品版本走产品层
// ——带 tightens 是类别错误（ErrDeliveryConditionOwner，只有合同能收紧产品）；两册之外没有交付条件可挂。三格齐、方式至少
// 一种且不重由 declareDeliveryConditions 同一处判，不另造校验。「只能收紧」不在这里：那要产品层在手，归持久化写口
// （pc-gaps/11 判断题 1）——预览与批准那两步因此看不出放宽，发布那一步才拒。
func (body DeliveryConditionBody) validate(register CommercialObjectKind) error {
	switch register {
	case CustomerContractObject:
		if body.Tightens == nil || !body.Tightens.valid() {
			return ErrDeliveryConditionNotConfigured
		}
		_, err := declareDeliveryConditions(CommercialVersion{}, *body.Tightens, body.Terms)
		return err
	case ServiceProductObject:
		if body.Tightens != nil {
			return ErrDeliveryConditionOwner
		}
		_, err := declareDeliveryConditions(CommercialVersion{}, TightenedProductVersion{}, body.Terms)
		return err
	default:
		return ErrDeliveryConditionOwner
	}
}

// canonicalDeliveryConditions 镜像批文 deliveryConditionDocument 的键名。方式按引用字面的稳定顺序写出——表单里换
// 行序不是换正文，摘要不该跟着变；tightens 只在合同层在场，产品层整键缺席。
type canonicalDeliveryConditions struct {
	Tightens            *canonicalTightenedProduct `json:"tightens,omitempty"`
	Methods             []string                   `json:"methods"`
	RecipientScopeRule  string                     `json:"recipientScopeRule"`
	ProofOfDeliveryRule string                     `json:"proofOfDeliveryRule"`
}

type canonicalTightenedProduct struct {
	ObjectID string `json:"objectId"`
	Version  string `json:"version"`
}

func canonicalDeliveryConditionsOf(body DeliveryConditionBody) *canonicalDeliveryConditions {
	methods := make([]string, 0, len(body.Terms.Methods))
	for _, method := range body.Terms.Methods {
		methods = append(methods, method.String())
	}
	sort.Strings(methods)
	document := &canonicalDeliveryConditions{
		Methods:             methods,
		RecipientScopeRule:  body.Terms.RecipientScopeRule.String(),
		ProofOfDeliveryRule: body.Terms.ProofOfDeliveryRule.String(),
	}
	if body.Tightens != nil {
		document.Tightens = &canonicalTightenedProduct{
			ObjectID: body.Tightens.ObjectID().String(),
			Version:  body.Tightens.Version().String(),
		}
	}
	return document
}

// body 把文档里的一节折回领域正文。每一格过构造门；零方式、同方式两行、层与 tightens 的配对这类跨格的问题留给
// validate——快照是数据，正文立不立得住仍由构造门说。
func (document canonicalDeliveryConditions) body() (DeliveryConditionBody, error) {
	var body DeliveryConditionBody
	if document.Tightens != nil {
		objectID, err := NewCommercialObjectID(document.Tightens.ObjectID)
		if err != nil {
			return DeliveryConditionBody{}, fmt.Errorf("tightens.objectId: %w", err)
		}
		version, err := NewCommercialVersionLabel(document.Tightens.Version)
		if err != nil {
			return DeliveryConditionBody{}, fmt.Errorf("tightens.version: %w", err)
		}
		tightens, err := NewTightenedProductVersion(objectID, version)
		if err != nil {
			return DeliveryConditionBody{}, fmt.Errorf("tightens: %w", err)
		}
		body.Tightens = &tightens
	}
	body.Terms.Methods = make([]DeliveryMethodReference, 0, len(document.Methods))
	for index, raw := range document.Methods {
		method, err := NewDeliveryMethodReference(raw)
		if err != nil {
			return DeliveryConditionBody{}, fmt.Errorf("methods[%d]: %w", index, err)
		}
		body.Terms.Methods = append(body.Terms.Methods, method)
	}
	recipientScope, err := NewDeliveryRuleReference(document.RecipientScopeRule)
	if err != nil {
		return DeliveryConditionBody{}, fmt.Errorf("recipientScopeRule: %w", err)
	}
	proofOfDelivery, err := NewDeliveryRuleReference(document.ProofOfDeliveryRule)
	if err != nil {
		return DeliveryConditionBody{}, fmt.Errorf("proofOfDeliveryRule: %w", err)
	}
	body.Terms.RecipientScopeRule = recipientScope
	body.Terms.ProofOfDeliveryRule = proofOfDelivery
	return body, nil
}
