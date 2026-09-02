package domain

// 本文件是渠道择优要用到的、对 party-commercial 所拥有事实的引用（票 `label-channel/12`）。
//
// 立在本上下文而不直接用提供方的类型，理由与 acceptance_basis.go 那一组同一条：两边各自
// 保有自己的模型，本上下文只记录所采用的引用。更硬的约束在架构门禁上——`application` 与
// `ports` 不得导入另一个上下文，而择优编排住在应用层，它没有别的办法指认「在哪个商业范围
// 下、按哪笔映射」。

// CommercialScopeReference 指认一次择优在哪个商业范围下解析。范围取值属实例半边，本上下文
// 只要求它被指名——没有范围就问不出「这个产品此刻映射到哪些渠道」。
type CommercialScopeReference struct{ requiredValue }

func NewCommercialScopeReference(value string) (CommercialScopeReference, error) {
	required, err := newRequiredValue("commercial scope reference", value)
	return CommercialScopeReference{required}, err
}

// ProductChannelMappingReference 指认这次择优按哪一笔产品—渠道映射取候选。
//
// 它是引用不是内容：映射的正文（有哪些渠道、有效到几时）留在提供方，本上下文不抄第二份
// ——票 12 红线「只引用 partycommercial 的读口，不在本上下文复制第二套映射」。
type ProductChannelMappingReference struct{ requiredValue }

func NewProductChannelMappingReference(value string) (ProductChannelMappingReference, error) {
	required, err := newRequiredValue("product channel mapping reference", value)
	return ProductChannelMappingReference{required}, err
}
