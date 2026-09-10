package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「收件地点引用」的窄读口（ADR-0130 决定二与 Consequences 第一条；PS CONTEXT Rules「收件地点引用
// 按（租户，包裹身份）答」那条；票 ps-port-remainder/06）：`transport-fulfillment` 派送任务地点一格的提供方
// 半边，TF 侧适配器（票 tf/12，ADR-0025 消费侧）以它为前置。
//
// **按（租户，包裹身份）问，不要求消费方持有委托、接受基线或提交版本**——TF 手里只有载运对象引用（CONTEXT-MAP
// `parcel-shipment → transport-fulfillment` 边），委托与接受基线是本上下文内部走到答案的路，不是键。另立一口而
// 不拓宽 ShipmentRequestRepository 一类写口，判据同 ADR-0077 决定五（伴生读端口另立）；今天没有第二个消费方，
// **不预设按委托或按引用反查的方法**——按锚解析回地址内容是第二个消费方，有自己的授权作用域问题，届时另立。
//
// 答法是 domain.DeliveryPlaceResolution 的封闭四格：基线锚引用 / 已采用版本锚引用 / 收件地点未定 / 没有收件地点。
// **没有`不知道`格**：走到答案要翻的册子——接受基线的成员集合、收件范围上的资料版本、由它们派生的当前采用判断
// ——全是本上下文自己的，答不出就是读面坏了，上抛 error；同一件包裹被多于一份已接受委托同时声明为成员也是读面
// 坏了（ADR-0060 的歧义），不挑一份作答。「未定」与「没有」分格交，TF 的 RequirementResolution 今天没有
// 「未定」那一格、怎么落是 TF 的事（ADR-0130 越权风险点 1）。
//
// **谱系未建模的今日形状。** 包裹 → 委托只经接受基线的声明成员走（AcceptanceBaseline.covers）；包裹身份谱系
// （拆分 / 合并后的新包裹）今天领域里没有模型，谱系包裹会落「没有收件地点」——不是它真没有，是本读口今天走不到
// 它的来源包裹。谱系落地那票要补这一路（同 ps-port-remainder/05 NO 半边「从未关联 → 不在」那条越权风险点的
// 处置）。同一格还收着集运单元与不可见对象：按统一不可见结果答，不区分不存在、他租户与未授权。
//
// 派生规则只有一处：当前采用判断由 domain 的 CurrentSourceDataAdoption 给出，适配器不在 SQL 里第二套实现。

// DeliveryPlaceReferenceView 按（租户，声明包裹身份）答收件地点引用。
type DeliveryPlaceReferenceView interface {
	// LoadDeliveryPlaceReference 答查询时刻的当前采用判断译成的那一格。error 只表示读面坏了（读不到、坏行、
	// 歧义），四格里的任何一格都不是 error。
	LoadDeliveryPlaceReference(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.DeliveryPlaceResolution, error)
}
