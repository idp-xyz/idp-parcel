package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「受理基线地理投影」的窄读口（ADR-0301，票 demo-chain-gaps/143）：`network-routing` 受理之后、不由本上下文
// 信封触发的路由复核取所携地理投影的提供方。那一类复核没有本上下文的请求可随带投影（ADR-0215 决定二），由 NR 的
// 消费侧适配器按（租户，包裹）来读；本口只交地理维、接受时刻与判断键要的几格引用，不交委托聚合。
//
// **按（租户，包裹身份）问，不要求消费方持有委托、接受基线或提交版本；不过授权查询作用域**——待遇同
// DeliveryPlaceReferenceView（NR 消费侧适配器是进程内另一个限界上下文、不是客户端）。与其余按（租户，包裹）答问的读口
// 分开、一口一问（ADR-0133 头注的先例）：按包裹找当前已接受委托用的是同一个谓词，取什么、答什么各是各的。
//
// 读的是**接受基线所指的那一版提交**——已接受委托上它就是当前提交版本，不一致即自相矛盾——不是接受之后经资料修订采用
// 的版本：基线一经固定不再改写，与判断键上的基线同版（ADR-0215 决定二对 ADR-0075 决定一的部分停用就以此为前提）。
// 受理之后的地址修订不进这里，那是 ADR-0215 越权风险点 5 的同一处取舍。
//
// 答法：
//   - found=false：没有当前已接受委托声明它，或声明它的那一份的接受基线里没有它（谱系包裹、集运单元号同落这一格），
//     按统一不可见结果，不区分不存在、他租户与仅`已提交`；
//   - found=true：domain.AcceptanceBaselineGeoProjection，投影一格都没报也照样是「带了」的一份；
//   - error 包 domain.ErrAmbiguousParcelTarget：两份已接受委托同时声明它（ADR-0060），不挑一份；
//   - error 包 domain.ErrAcceptanceBaselineInconsistent：命中的那一份委托接受基线自相矛盾、重建门会拒它（各形见该哨兵
//     头注），重投不会变；
//   - 其余 error：读不动（依赖故障、坏行），重投可能推得动。

// AcceptanceBaselineGeoProjectionView 按（租户，声明包裹身份）答当前已接受委托接受基线那一版提交的地理投影。
type AcceptanceBaselineGeoProjectionView interface {
	LoadAcceptanceBaselineGeoProjection(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.AcceptanceBaselineGeoProjection, bool, error)
}
