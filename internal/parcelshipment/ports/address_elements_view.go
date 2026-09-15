package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「地址要素」的窄读口（pp-seams/03 裁决 2 / 4 与裁决 7 追裁；PS CONTEXT「地址要素」词条）：`parcel-pricing`
// 造计价输入快照「邮编路线」一格的提供方——PP 消费侧适配器（ADR-0025 消费侧，pp-seams「不在本目录」那张票）以它为
// 前置；邮编 → PP `PostalRoute` 的翻译、邮编 → 分区的解析（ADR-0109，从绑定的计价参考目录解析）全在 PP 那一侧，本口
// 不算分区、不解析服务区域、不查目录。
//
// **它就是 DeliveryPlaceReferenceView 头注预告的那个「按锚解析回地址内容」的第二个消费方的口**——但只交两格值：邮编与
// 国家 / 地区码，**不交地址文本、不交其他要素**（PS CONTEXT Rules「合成日志和证据索引只保留作用域引用、对象引用……
// 不记录客户原文」那条的同一条纪律）。**一口两段**：起点（寄件资料范围）与目的（收件资料范围）各自成格，一段有一段
// 无是常态；起点是寄件人的邮编——PS 只答自己有的，承运商分区表按注入 / 收寄节点分始发区时那一格的提供方是 NR / NO，
// 另票（裁决 3）。不新造「寄件地点引用」：地点引用是给派送任务持有的复合引用（ADR-0130），邮编是资料的一格，本口
// 不落「地点」。
//
// **按（租户，包裹身份）问，不要求消费方持有委托、接受基线或提交版本；不过授权查询作用域**——待遇同
// DeliveryPlaceReferenceView（裁决 4：PP 消费侧适配器是进程内另一个限界上下文、不是客户端）。另立一口而不拓宽写口，
// 判据同 ADR-0077 决定五；与 DeliveryPlaceReferenceView / CommercialResolutionReferenceView / DeclaredMeasurementView
// 分开、一口一问（ADR-0133 头注的先例）：这些口内部走同一条包裹 → 委托的路、同一个「按范围解析资料版本锚」的内部
// 步骤，对外各答各的。
//
// 每段的答法是 domain.AddressElementsResolution 的封闭五格：基线锚带要素 / 已采用版本锚带锚与那一版自己的要素（可缺席）/
// 未定 / 要素缺席 / 无。答哪一格先看锚再看内容（domain.ShipmentRequest.AddressElementsFor）：非成员答无、待复核答未定、
// 已采用答已采用版本锚并从那一版的内容子段读值，锚落在接受基线上的那一段从基线所指那一版提交版本的要素子段读值
// （pp-seams/05：两种版本都携带按封闭要素名挑出的子段）。已采用格上要素缺席（显式清空，或修订改的是本上下文没有
// 词条的条目）时只交锚不交值，绝不回退到基线值；早于那票的旧形快照没有子段，基线格如实答「要素缺席」。
//
// 值原样交：邮编格式与前缀粒度属实例半边（ADR-0109「随首份真实分区表定」），本上下文不校验、不规范化、不去空白——全空白
// 的值原样在场，只有显式清空（值为空串）读作缺席，口径与 domain.AddressElementsOf 头注同一句。五格里没有
// 「不知道」格；同一件包裹被多于一份已接受委托同时声明是读面坏了（ADR-0060 的歧义），上抛而不挑一份。谱系包裹与集运
// 单元落「无」，与 DeliveryPlaceReferenceView 同一今日形状。

// AddressElementsView 按（租户，声明包裹身份）答寄件与收件两段地址要素（邮编、国家 / 地区码）与各自的资料版本锚。
type AddressElementsView interface {
	// LoadAddressElements 答查询时刻两段各自的当前采用判断译成的那一格。error 只表示读面坏了（读不到、坏行、歧义），
	// 任何一格都不是 error。
	LoadAddressElements(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.ShipmentAddressElements, error)
}
