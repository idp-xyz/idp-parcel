package domain

import "errors"

// ErrInvalidFxCaliber 拒绝立不住的汇率口径。
var ErrInvalidFxCaliber = errors.New("party commercial: invalid fx caliber")

// FxQuoteTypeReference 指向一种牌价类型（如某行现汇卖出价、央行中间价）。它是引用而不是
// 封闭枚举：牌价类型是租户与其银行、平台之间的约定，开发方不是当事人，本仓无从替租户列出
// 取值，更不得写死一个当默认——按红线，这类实例半边的参数只能保持可配置或显式未决。
type FxQuoteTypeReference struct{ requiredValue }

func NewFxQuoteTypeReference(value string) (FxQuoteTypeReference, error) {
	required, err := newRequiredValue("fx quote type reference", value)
	return FxQuoteTypeReference{required}, err
}

// FxCaliber 是商业价格政策声明的汇率口径：按哪种牌价、锚在哪个业务时点取值。
//
// 取值时点**复用**本上下文既有的时点语义引用与政策版本，而不借 `AsOfPolicy`/`JudgmentType`
// 那层壳：那一套按 ADR-0042 的归属纪律钉在接单规则包上（`DeclareAsOfPolicies` 的构造门、
// 0005 迁移的 `object_kind` CHECK、读口的 SQL 三处同一判据），而汇率取值时点按 CONTEXT 是
// **价格政策**声明的口径，归属对象不同。往 `JudgmentType` 加一格会把一个不属于规则包的判断
// 塞进规则包的封闭集，还要跨上下文改 PS 的译表——两边都不是本口径该付的价。
//
// 两格缺一不可。只有牌价类型没有时点，或只有时点没有牌价类型，各自都能让同一份原清单在两个
// 时刻算出两个数——那不是「重放不一致」这种可发现的失败，是重放本身失去意义（parcel-pricing
// CONTEXT：「同一评价输入……版本清单和版本内容摘要必须产生相同结果」）。
//
// **这里没有加点规则那一格，且不是漏掉。** 本上下文按立场不持数值（见 VolumetricFactorReference
// 的注释），而做成引用则被引处今天不存在——parcel-pricing 没有承载加点正文的工件，那会造出
// 「引用是强制的、被引的那份是空的」这个形状。裁决与重启条件记在
// .scratch/party-commercial-context-gaps/issues/02 票面「加点规则的裁决」一节。缺这一格的
// 含义是「加点未声明」，消费方据以停在未决，不得读成零加点。
type FxCaliber struct {
	quoteType         FxQuoteTypeReference
	asOfSemantics     AsOfSemanticsReference
	asOfPolicyVersion AsOfPolicyVersion
}

func NewFxCaliber(
	quoteType FxQuoteTypeReference,
	asOfSemantics AsOfSemanticsReference,
	asOfPolicyVersion AsOfPolicyVersion,
) (FxCaliber, error) {
	if !quoteType.valid() || !asOfSemantics.valid() || !asOfPolicyVersion.valid() {
		return FxCaliber{}, ErrInvalidFxCaliber
	}
	return FxCaliber{
		quoteType:         quoteType,
		asOfSemantics:     asOfSemantics,
		asOfPolicyVersion: asOfPolicyVersion,
	}, nil
}

func (caliber FxCaliber) QuoteType() FxQuoteTypeReference {
	return caliber.quoteType
}

func (caliber FxCaliber) AsOfSemantics() AsOfSemanticsReference {
	return caliber.asOfSemantics
}

func (caliber FxCaliber) AsOfPolicyVersion() AsOfPolicyVersion {
	return caliber.asOfPolicyVersion
}
