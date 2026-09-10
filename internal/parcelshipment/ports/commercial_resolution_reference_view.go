package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「商业解析回指」的窄读口（ADR-0133 决定二，与其 Consequences「PS `ports` 另立一个按（租户，包裹身份）答
// 商业解析回指的窄读口……一口一问、不合并；不拓宽既有写口」那条；票 ps-port-remainder/07）：
// `transport-fulfillment` 交付条件缝里对象走到合同的那一跳的提供方半边——TF 侧适配器（票 tf/14，ADR-0025 消费侧）
// 拿回指再走 party-commercial 的读口解闭包，本上下文不替它解。
//
// **按（租户，包裹身份）问，不要求消费方持有来源身份或委托。** 本上下文既有的那条路（ADR-0062：SourceIdentity →
// 已接受委托 → 接受决定上的解析标识，adapters/partycommercial/adopted_stage_owner.go 走的就是它）键是来源身份，
// TF 手里只有载运对象引用；本口是它的包裹键姊妹，答的是同一个回指。另立一口而不拓宽写口，判据同 ADR-0077 决定五。
// **与 DeliveryPlaceReferenceView 分开、一口一问**：两者内部走同一条包裹 → 委托的路，但 ADR-0130 与 0133 都写了
// 读口不预设按委托或按引用反查——合成一个宽口就是在预设消费方同时要两样；不预设按回指反查包裹或委托。
//
// 三格：回指（第二个返回值为真）/ 没有（为假、error 为空——对象不属任何已接受委托的成员集合，含集运单元与不可见
// 对象，按统一不可见结果，不区分不存在、他租户与未授权；委托未接受同格）/ error（`已接受`而接受决定缺回指是接受流
// 的装配缺陷，domain.ErrAcceptedWithoutCommercialResolution；读面坏了、歧义同为 error）。**没有`不知道`格**。
//
// **谱系未建模的今日形状**：包裹 → 委托只经接受基线的声明成员走；拆分 / 合并后的新包裹落「没有」，谱系落地那票要
// 补这一路（同 06 与 ps-port-remainder/05 NO 半边的处置）。
//
// 交回的就是 domain.CommercialResolutionID：不拆、不拼合同版本（「对象/版本」两段式只许 party-commercial 一处拼，
// ADR-0080 决定七）；回指不是能力凭证（ADR-0062 决定三 / ADR-0003），租户由调用方给。

// CommercialResolutionReferenceView 按（租户，声明包裹身份）答委托接受时固定的商业解析回指。
type CommercialResolutionReferenceView interface {
	LoadCommercialResolutionReference(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.CommercialResolutionID, bool, error)
}
