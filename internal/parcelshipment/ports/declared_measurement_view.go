package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「申报测量」的窄读口（pp-seams/02 裁决 5；spec「边界」「提供方只开只读口，不为消费方派生判断」）：
// `parcel-pricing` 造计价输入快照「实重 + 尺寸」一格的 PS 半边——PP 消费侧适配器（ADR-0025 消费侧，pp-seams
// 「不在本目录」那张票）以它为前置。NO 半边（实际测量）在 pp-seams/04 另立，两口互不知对方存在；两源并存按谁是 PP
// 消费侧按已登记规则做的事，本口只答客户报了什么。
//
// **按（租户，包裹身份）问，不要求消费方持有委托、接受基线或提交版本；不过授权查询作用域**——待遇同
// DeliveryPlaceReferenceView（03 裁决 4 的理由：PP 消费侧适配器是进程内另一个限界上下文、不是客户端，PS CONTEXT
// 「查询只消费……授权查询作用域」说的是对外查询面）。ShipmentRequestViews.FindVisibleByID 上的 DeclaredParcelViewRecord
// 是给人查阅的读面，键是作用域 + 委托 ID，不是本口的先例。另立一口而不拓宽写口，判据同 ADR-0077 决定五；**与
// DeliveryPlaceReferenceView / CommercialResolutionReferenceView 分开、一口一问**（ADR-0133 头注的先例）：这些口
// 内部走同一条包裹 → 委托的路、同一个「按范围解析资料版本锚」的内部步骤，对外各答各的。
//
// 答法是 domain.DeclaredMeasurementResolution 的封闭五格：基线锚带测量 / 已采用版本锚只带锚 / 未定 / 未申报 / 无。
// 五格里没有一格是「不知道」：走到答案要翻的册子——接受基线的成员集合、基线所指那一版上的画像、该包裹申报测量范围
// 上的资料版本——全是本上下文自己的，答不出就是读面坏了，上抛 error；同一件包裹被多于一份已接受委托同时声明同为
// 读面坏了（ADR-0060 的歧义），不挑一份作答。
//
// 值原样交：数字是客户申报的字面串（"2.50" 不规范化），单位是 PS 的 MeasurementUnitReference 自由串——对到 PP
// WeightUnit / LengthUnit 封闭集归 PP 消费侧。缺尺寸如实答缺，不填默认。
//
// **已采用版本锚那一格只交锚不交测量**（domain.DeclaredMeasurementAnchoredOnAdoptedVersion 头注写了为什么）：
// 客户原始资料版本今天只留痕不留内容，本上下文说不出那一版报了多少。持口方拿到它该停在「输入不可得」。
//
// **谱系未建模的今日形状**：包裹 → 委托只经接受基线的声明成员走，拆分 / 合并后的新包裹落「无」，与
// DeliveryPlaceReferenceView / CommercialResolutionReferenceView 同一处置；集运单元同落「无」（02 裁决 4：PS 口按
// 包裹身份答，集运单元 found=false）。

// DeclaredMeasurementView 按（租户，声明包裹身份）答客户申报的重量 / 尺寸与其资料版本锚。
type DeclaredMeasurementView interface {
	// LoadDeclaredMeasurement 答查询时刻的当前采用判断译成的那一格。error 只表示读面坏了（读不到、坏行、歧义），
	// 五格里的任何一格都不是 error。
	LoadDeclaredMeasurement(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.DeclaredMeasurementResolution, error)
}
