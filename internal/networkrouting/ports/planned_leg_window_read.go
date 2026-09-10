package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// PlannedLegWindowRead 是按计划履约段引用取那一段计划时间窗口的窄读口（ADR-0131 决定一），供
// transport-fulfillment 的派送时间窗口缝在进程内消费——它不是列表口，也不拓宽既有判断口：
// InitialRouteStore / ReassessmentStore 按完整判断键取单行，服务编排的写前查；TF 不持有那把键，
// 它手里只有登记方在参与关系上声明的这一个引用。
//
// 交内容不交适用性（ADR-0131 决定二）：答的是引用所钉那一版的那一段，无论该版本此刻当前有效、
// 已被替代、已失效还是已结束——适用性四态由本上下文别的口回答，不折进窗口答案。实现按各判断口
// 现有的装载纪律把计划本体重建后取段（无论那一版躺在初始路由判断行的 plan 列还是复核判断行的
// new_plan 列），不从 jsonb 里抠字段冒充窗口。
//
// 三格：found=true 带窗口（含形成依据）；found=false 是**业务答案不是错误**——版本不在本租户下、
// 或序位越出该版本的段链，一条指错的引用重跑也不会长出那一段来；error 只留给读不回。租户在方法
// 签名上，作用域经判断行的租户条件承担（ADR-0003），跨租户零行答 found=false，不区分「不存在」
// 与「属于另一个租户」。
type PlannedLegWindowRead interface {
	LoadPlannedLegWindow(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.PlannedLegReference,
	) (domain.PlannedTimeWindow, bool, error)
}
