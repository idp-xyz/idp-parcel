package domain

import (
	"errors"
	"sort"
)

var (
	// ErrDeliveryConditionOwner 是拥有对象立不住：交付条件只挂在已生效的服务产品版本（产品层）或客户合同版本
	// （合同层）上（ADR-0133 决定四），两层各走各的门、不互相冒名；接单规则包那一族被 ADR-0133 明确否决。
	ErrDeliveryConditionOwner = errors.New("party commercial: delivery conditions are declared on an effective service product version or customer contract version")
	// ErrDeliveryConditionNotConfigured 是声明缺件：零方式、某一方式或规则引用为空、合同层没指名所收紧的产品版本。
	// 恢复动作是把这一层按 PAR-NET-09 / PAR-COM-05 / PAR-COM-06 登齐（实例半边），不是本上下文代拟默认。
	ErrDeliveryConditionNotConfigured = errors.New("party commercial: delivery condition declaration is not configured")
	// ErrConflictingDeliveryCondition 是同一方式登了两行。
	ErrConflictingDeliveryCondition = errors.New("party commercial: conflicting delivery condition declaration")
	// ErrDeliveryConditionWidened 是合同层出现了产品层没有的方式——合同只能在产品条件之内收紧，不能放宽
	//（PC CONTEXT「交付条件」词条）。
	ErrDeliveryConditionWidened = errors.New("party commercial: customer contract delivery conditions may only tighten within the service product's")
	// ErrDeliveryConditionTighteningTarget 是拿来核收紧的那份产品层不是合同层指名的那一版（或根本不是产品层）：
	// 收紧是对着一份具体的产品层说的话，对着另一版核出来的「子集」什么也证明不了。
	ErrDeliveryConditionTighteningTarget = errors.New("party commercial: the product layer is not the one this contract layer tightens")
	// ErrDeliveryConditionClosureAbsent 是按商业解析回指问「有没有交付条件」时闭包不在场：对一份已接受委托的回指，
	// 这是提供方缺数据，不是「没登条件」（ADR-0133 决定二；判据同 ADR-0080 决定四）。
	ErrDeliveryConditionClosureAbsent = errors.New("party commercial: no commercial closure is held for this resolution reference")
	// ErrDeliveryConditionContractNotAdopted 是闭包在场却没采用客户合同版本（判据同 ADR-0062 决定三）。
	ErrDeliveryConditionContractNotAdopted = errors.New("party commercial: the commercial closure did not adopt a customer contract version")
)

// DeliveryMethodReference 是一种交付方式的引用（安全投放、智能柜、邻居代收、本人签收……）。它是开放引用：词表属
// PAR-NET-09 的实例登记，本上下文只查非空、不校验存在，也不内置任何一种——TF CONTEXT 硬句里那几个词是举例不是
// 封闭集，今天没有一份 CONTEXT 词表能让本上下文替租户封闭它。
type DeliveryMethodReference struct{ requiredValue }

func NewDeliveryMethodReference(value string) (DeliveryMethodReference, error) {
	required, err := newRequiredValue("delivery method reference", value)
	return DeliveryMethodReference{required}, err
}

// DeliveryRuleReference 是收件范围规则或交付证明规则的引用，同为开放引用（PAR-COM-05 / PAR-COM-06 实例半边）：
// 本上下文登引用不登规则正文，也不判两条规则谁更严——收紧判据只落在方式集合上。
type DeliveryRuleReference struct{ requiredValue }

func NewDeliveryRuleReference(value string) (DeliveryRuleReference, error) {
	required, err := newRequiredValue("delivery rule reference", value)
	return DeliveryRuleReference{required}, err
}

// TightenedProductVersion 是合同层所收紧的那一版服务产品（对象标识 + 版本号）。合同层的收紧是对着一份具体的产品层
// 说的话，所以它要指名；类别不带——它只能是服务产品版本。
type TightenedProductVersion struct {
	objectID CommercialObjectID
	version  CommercialVersionLabel
}

func NewTightenedProductVersion(objectID CommercialObjectID, version CommercialVersionLabel) (TightenedProductVersion, error) {
	if !objectID.valid() || !version.valid() {
		return TightenedProductVersion{}, ErrDeliveryConditionNotConfigured
	}
	return TightenedProductVersion{objectID: objectID, version: version}, nil
}

func (target TightenedProductVersion) ObjectID() CommercialObjectID {
	return target.objectID
}

func (target TightenedProductVersion) Version() CommercialVersionLabel {
	return target.version
}

func (target TightenedProductVersion) valid() bool {
	return target.objectID.valid() && target.version.valid()
}

// DeliveryConditionTerms 是一层交付条件的正文三格：允许的交付方式集合、收件范围规则引用、交付证明规则引用。三格在
// 两层都必填——一层登了方式却没说凭什么算有效交付，等于让消费方自己补一条规则，那正是「不以通用签名规则替代合同」
//（UC-TF-006）要拦的事。
type DeliveryConditionTerms struct {
	Methods             []DeliveryMethodReference
	RecipientScopeRule  DeliveryRuleReference
	ProofOfDeliveryRule DeliveryRuleReference
}

// DeliveryConditionContent 是一层交付条件声明：产品层（服务产品版本声明产品级条件）或合同层（客户合同版本在产品
// 条件之内收紧）。两层同形，差别只在拥有对象的类别与合同层多出的「所收紧的产品版本」一格。
//
// 合同层经 DeclareContractDeliveryConditions 立住的是「登记方说了什么」，还没对着产品层核过——核那一步要产品层在手，
// 由 TightensWithin 单独做：登记路上持久化面读回产品层后核；读回路上不重核，登记时核过的那份就是它。「没有默认」：
// 两层都缺席就是没有交付条件，本上下文不默认「本人签收」也不默认「任何方式」。
type DeliveryConditionContent struct {
	owner               CommercialVersion
	tightens            TightenedProductVersion
	methods             []DeliveryMethodReference
	recipientScopeRule  DeliveryRuleReference
	proofOfDeliveryRule DeliveryRuleReference
}

// DeclareProductDeliveryConditions 立产品层：拥有对象必须是已生效的服务产品版本，正文三格齐、方式至少一种且不重。
func DeclareProductDeliveryConditions(owner CommercialVersion, terms DeliveryConditionTerms) (DeliveryConditionContent, error) {
	if owner.kind != ServiceProductObject || owner.status != CommercialVersionEffective {
		return DeliveryConditionContent{}, ErrDeliveryConditionOwner
	}
	return declareDeliveryConditions(owner, TightenedProductVersion{}, terms)
}

// DeclareContractDeliveryConditions 立合同层：拥有对象必须是已生效的客户合同版本，并指名所收紧的服务产品版本。
// 方式是否真在那一版产品层之内由 TightensWithin 核——这里没有产品层可对。
func DeclareContractDeliveryConditions(
	owner CommercialVersion,
	tightens TightenedProductVersion,
	terms DeliveryConditionTerms,
) (DeliveryConditionContent, error) {
	if owner.kind != CustomerContractObject || owner.status != CommercialVersionEffective {
		return DeliveryConditionContent{}, ErrDeliveryConditionOwner
	}
	if !tightens.valid() {
		return DeliveryConditionContent{}, ErrDeliveryConditionNotConfigured
	}
	return declareDeliveryConditions(owner, tightens, terms)
}

func declareDeliveryConditions(
	owner CommercialVersion,
	tightens TightenedProductVersion,
	terms DeliveryConditionTerms,
) (DeliveryConditionContent, error) {
	if len(terms.Methods) == 0 || !terms.RecipientScopeRule.valid() || !terms.ProofOfDeliveryRule.valid() {
		return DeliveryConditionContent{}, ErrDeliveryConditionNotConfigured
	}
	seen := make(map[string]struct{}, len(terms.Methods))
	methods := make([]DeliveryMethodReference, 0, len(terms.Methods))
	for _, method := range terms.Methods {
		if !method.valid() {
			return DeliveryConditionContent{}, ErrDeliveryConditionNotConfigured
		}
		if _, duplicate := seen[method.String()]; duplicate {
			return DeliveryConditionContent{}, ErrConflictingDeliveryCondition
		}
		seen[method.String()] = struct{}{}
		methods = append(methods, method)
	}
	sort.Slice(methods, func(left, right int) bool { return methods[left].String() < methods[right].String() })
	return DeliveryConditionContent{
		owner:               owner,
		tightens:            tightens,
		methods:             methods,
		recipientScopeRule:  terms.RecipientScopeRule,
		proofOfDeliveryRule: terms.ProofOfDeliveryRule,
	}, nil
}

// TightensWithin 核合同层是否在给定的产品层之内收紧：那份产品层必须正是本层指名的那一版（同租户、同对象、同版本号），
// 且本层每一种方式都在产品层的集合里；多出一种即放宽。产品层调它是个类别错误——产品层无所谓收紧。
func (content DeliveryConditionContent) TightensWithin(product DeliveryConditionContent) error {
	if content.owner.kind != CustomerContractObject {
		return ErrDeliveryConditionOwner
	}
	if product.owner.kind != ServiceProductObject ||
		product.owner.tenant != content.owner.tenant ||
		product.owner.objectID != content.tightens.objectID ||
		product.owner.version != content.tightens.version {
		return ErrDeliveryConditionTighteningTarget
	}
	for _, method := range content.methods {
		if !product.Allows(method) {
			return ErrDeliveryConditionWidened
		}
	}
	return nil
}

func (content DeliveryConditionContent) Owner() CommercialVersion {
	return content.owner
}

// Tightens 在合同层交回所收紧的服务产品版本；产品层答无。
func (content DeliveryConditionContent) Tightens() (TightenedProductVersion, bool) {
	return content.tightens, content.tightens.valid()
}

// Methods 按引用字面的稳定顺序交回允许的交付方式（副本）。
func (content DeliveryConditionContent) Methods() []DeliveryMethodReference {
	return append([]DeliveryMethodReference(nil), content.methods...)
}

// Allows 答某一方式是否在本层允许的集合内。空引用一律不允许——那不是一种方式。
func (content DeliveryConditionContent) Allows(method DeliveryMethodReference) bool {
	if !method.valid() {
		return false
	}
	for _, allowed := range content.methods {
		if allowed == method {
			return true
		}
	}
	return false
}

func (content DeliveryConditionContent) RecipientScopeRule() DeliveryRuleReference {
	return content.recipientScopeRule
}

func (content DeliveryConditionContent) ProofOfDeliveryRule() DeliveryRuleReference {
	return content.proofOfDeliveryRule
}

// DeliveryConditionReference 是交付条件对外的引用——就是委托接受时固定的商业解析回指本身（ADR-0133 决定一），
// 不另铸标识；消费方持它不持内容，读内容时按它到闭包取已采用的两个版本。单独成类型是让「这是交付条件引用」在
// 端口签名上读得出来，而它的拼写就是回指的拼写。
type DeliveryConditionReference struct {
	resolution ResolutionID
}

func (reference DeliveryConditionReference) Resolution() ResolutionID {
	return reference.resolution
}

func (reference DeliveryConditionReference) String() string {
	return reference.resolution.String()
}

// DeliveryConditionReferenceFor 由闭包与两层声明的在场情况答「有没有交付条件」（ADR-0133 决定二的 PC 半边）：
// 闭包必须唯一解析成功且采用了客户合同版本，否则是 error（提供方缺数据 / 装配缺陷，不冒充「没登」）；两层至少
// 一层有声明答引用；两层都无答没有——商业责任方去登。只答有没有，不交内容：内容读口是第二个消费方的事。
//
// 服务产品版本未被采用不单列一格：那一层只是没有可读的声明，答案落在合同层身上；闭包能不能不采用产品是解析键
// 的事，不在这里裁。
func DeliveryConditionReferenceFor(
	closure CommercialClosure,
	productDeclared bool,
	contractDeclared bool,
) (DeliveryConditionReference, bool, error) {
	if closure.Outcome() != UniquelyResolved || !closure.ResolutionID().valid() {
		return DeliveryConditionReference{}, false, ErrDeliveryConditionClosureAbsent
	}
	if _, adopted := closure.AdoptedFor(CustomerContractObject); !adopted {
		return DeliveryConditionReference{}, false, ErrDeliveryConditionContractNotAdopted
	}
	if !productDeclared && !contractDeclared {
		return DeliveryConditionReference{}, false, nil
	}
	return DeliveryConditionReference{resolution: closure.ResolutionID()}, true, nil
}
