package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件是「包裹 → 结算账户」那条链上、结算要向参与方与商业回指的读口（ADR-0190 决定三：与受理前财务控制同一条链；
// 销售价格约定是同一条链上多取的一格，ADR-0380 决定二）。链分三段：小包托运在
// 委托接受时固定的商业解析 → 参与方与商业的闭包里采用的结算政策 → 本上下文的结算账户册。前两段各归一个提供方，
// 各立一口、各由一个消费侧适配器翻译（ADR-0025）；第三段是本上下文自己的 SettlementAccountView。三段不合成一口：
// 合成就得有一只适配器同时认识两个提供方的词，而三段各自「没有」的恢复动作也不同。

// ParcelCommercialResolutionView 按（租户，包裹）答小包托运在委托接受时固定的商业解析回指。第二个返回值为 false
// 是「这件包裹不属任何已接受委托」，不是读坏了；读坏了与提供方装配缺陷走 error。
type ParcelCommercialResolutionView interface {
	LoadParcelCommercialResolution(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.ParcelReference,
	) (domain.CommercialResolutionReference, bool, error)
}

// AdoptedSettlementTermsView 按（租户，商业解析回指）答那次已固定的闭包采用的结算政策落在账户键上的四维。第二个
// 返回值为 false 是「闭包没采用结算政策」——租户登记的解析键没要这一项，等租户补，不是重试的事；回指读不回闭包、
// 闭包不是唯一已解析之类走 error：那一份回指是本链上一段刚给的，读不回不是「没登记」。
type AdoptedSettlementTermsView interface {
	LoadAdoptedSettlementTerms(
		ctx context.Context,
		tenant domain.TenantID,
		resolution domain.CommercialResolutionReference,
	) (domain.AdoptedSettlementTerms, bool, error)
}

// AdoptedSellPriceArrangementView 按（租户，商业解析回指）答那次已固定的闭包采用的客户合同版本上的销售价格约定
// （ADR-0380 决定二）。与 AdoptedSettlementTermsView 并列——同一份回指、同一份闭包、各取各的成员。不合成一只：
// 结算政策「没采用」是租户去补解析键；销售价格约定的三种答复都是合同正文说的话，空列表不顶「没有约定」。
//
// 成功路径交出三种答复之一。回指译不动、闭包读不回或查无、闭包不是唯一已解析、闭包没采用客户合同、约定正文坏
// → error。这些都不是三种答复之一。
type AdoptedSellPriceArrangementView interface {
	LoadAdoptedSellPriceArrangement(
		ctx context.Context,
		tenant domain.TenantID,
		resolution domain.CommercialResolutionReference,
	) (domain.AdoptedSellPriceArrangement, error)
}
