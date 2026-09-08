package domain

// 本文件是服务产品册接进 PCC-1 的那一格（票 admin-write-faces/09「本册规范化判断」）。
//
// 本册**没有正文**：发布的是商业版本壳——它给合同、接单规则包、价格政策一个可引用的版本身份；产品的属性与
// 渠道映射走另一条登记路（register_products），不在发布载荷里。ADR-0126 Decision 一「文档只盖正文不盖壳」对无
// 正文的册读作「正文为空，文档只剩规范化版本与 kind 两格」：这是那一句的边界情形不是例外，因此不换号，也不
// 把壳上的指名引用读进文档——引用与范围、区间一样是壳的一格，已由 sameReleasedContent / SameSubmissionAs
// 逐项比对，盖进摘要只会让「同引用换范围」与「同范围换引用」两种修订长成两张不同的脸。
//
// 于是本册每一版的摘要是同一个串。它不是退化：串带版本、可重算、可比；同键重发靠壳分重放与修订，落到
// 登记册后由同一套判据分重放与冲突，没有一处为本册另开。本册也没有「正文缺席」那一格——它没有正文槽位，
// 缺席与在场是同一件事，所以 PublicationContent 与 canonicalPublicationDocument 都不为它加格。

// canonicalServiceProductContent 把服务产品册折成两格文档并算摘要。调用方已核过壳声明的类别就是本册且没有
// 别册的正文冒名（CanonicalizePublicationContent 的 kind 不符那一格），这里不再判。
func canonicalServiceProductContent() (CanonicalPublicationContent, error) {
	return canonicalDigestOf(canonicalPublicationDocument{
		Canonicalization: publicationCanonicalizationVersion,
		Kind:             ServiceProductObject.String(),
	})
}

// registerHasNoBody 答某一册是不是结构上没有正文：这样的册折回快照时没有「正文缺席」可判。今天只有服务产品。
func registerHasNoBody(kind CommercialObjectKind) bool {
	return kind == ServiceProductObject
}
