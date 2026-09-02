package domain

import "errors"

// ErrInvalidCustomerServiceRuleVersion 拒绝立不住的客户服务规则版本。
var ErrInvalidCustomerServiceRuleVersion = errors.New("party commercial: invalid customer service rule version")

// CustomerServiceRuleApplicability 是规则版本挂在哪个商业对象上的封闭两格：服务产品版本或
// 客户合同版本（CONTEXT「按服务产品和客户合同明确适用范围」）。
//
// declared 让零值立不住。「没说挂在哪」与「挂在一个尚未指明的对象上」在裸的引用零值里长得
// 一样，而前者是输入缺件、后者是一句该被拒的声明——判据同 ProductChannelBinding 的两格封闭。
type CustomerServiceRuleApplicability struct {
	declared bool
	product  CommercialObjectID
	contract CommercialObjectID
}

// CustomerServiceRuleAppliesToServiceProduct 声明本规则版本随某个服务产品适用。
func CustomerServiceRuleAppliesToServiceProduct(product CommercialObjectID) CustomerServiceRuleApplicability {
	return CustomerServiceRuleApplicability{declared: true, product: product}
}

// CustomerServiceRuleAppliesToCustomerContract 声明本规则版本随某个客户合同适用。
//
// 与上一格分开而不合成「一个标识加一个类别字段」：同一个标识串作产品与作合同是两件事，而合成
// 之后它们在那一列里长得一模一样，类别设错时没有任何东西能分辨——判据同票 03 对额度取值形态
// 的裁断（并存两格，落在哪一格本身就是判别式）。
func CustomerServiceRuleAppliesToCustomerContract(contract CommercialObjectID) CustomerServiceRuleApplicability {
	return CustomerServiceRuleApplicability{declared: true, contract: contract}
}

func (applicability CustomerServiceRuleApplicability) ServiceProduct() (CommercialObjectID, bool) {
	return applicability.product, applicability.product.valid()
}

func (applicability CustomerServiceRuleApplicability) CustomerContract() (CommercialObjectID, bool) {
	return applicability.contract, applicability.contract.valid()
}

func (applicability CustomerServiceRuleApplicability) valid() bool {
	if !applicability.declared {
		return false
	}
	// 恰一格有值。两格都填不是「更明确」，是两条适用声明挤在一条记录上，续办时答不出该按哪条。
	return applicability.product.valid() != applicability.contract.valid()
}

// CustomerServiceRuleVersion 是一个客户服务规则版本的正文骨架：它挂在哪个商业对象上、责任方
// 是谁、适用范围是什么。规则内容（追踪披露、异常响应、客户更新、通知义务、索赔期限、最低材料）
// 属后续切片，见票 party-commercial-context-gaps/05。
//
// **本类型刻意不带内部异常检测阈值、事实有效性与最终赔付金额。** CONTEXT 明写这三样「不得写成
// 客户可以直接覆盖的商业配置」——不建字段是唯一守得住的办法：留一个字段再靠约定不填，下一个人
// 看到的是一个可填的口子，而那时没有任何东西会拦他。
type CustomerServiceRuleVersion struct {
	version       CommercialVersion
	applicability CustomerServiceRuleApplicability
	responsible   PartyID
	scope         CommercialScopeReference
}

// NewCustomerServiceRuleVersion 在版本已生效且类别正确时形成一个规则版本。
//
// 类别必须是 CustomerServiceRuleObject。挂错类别的版本仍是一个合法的商业版本，入册与被解析
// 选中都不报错，只是解析按错的类别去找；那个错要到下游取不到规则依据时才显形，而那时它长得
// 像「这个客户没配规则」（ADR-0093 否决复用 AuthorizationRuleObject 时给的正是这条理由）。
func NewCustomerServiceRuleVersion(
	version CommercialVersion,
	applicability CustomerServiceRuleApplicability,
	responsible PartyID,
	scope CommercialScopeReference,
) (CustomerServiceRuleVersion, error) {
	if version.kind != CustomerServiceRuleObject ||
		version.status != CommercialVersionEffective ||
		!applicability.valid() || !responsible.valid() || !scope.valid() {
		return CustomerServiceRuleVersion{}, ErrInvalidCustomerServiceRuleVersion
	}
	return CustomerServiceRuleVersion{
		version:       version,
		applicability: applicability,
		responsible:   responsible,
		scope:         scope,
	}, nil
}

func (rule CustomerServiceRuleVersion) Version() CommercialVersion {
	return rule.version
}

func (rule CustomerServiceRuleVersion) Applicability() CustomerServiceRuleApplicability {
	return rule.applicability
}

// ResponsibleParty 是本规则版本的责任方。CONTEXT 把它与适用范围、有效期间并列为必需项：
// 没有责任方的服务承诺在异常响应时答不出该找谁。
func (rule CustomerServiceRuleVersion) ResponsibleParty() PartyID {
	return rule.responsible
}

func (rule CustomerServiceRuleVersion) Scope() CommercialScopeReference {
	return rule.scope
}
