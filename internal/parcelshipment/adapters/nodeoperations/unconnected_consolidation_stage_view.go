package nodeoperations

import (
	"context"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// UnconnectedConsolidationStageView 是装袋事实读口在 node-operations 还没有按正式包裹键的读面
// 之前的如实答复：一律`不知道`。
//
// 节点作业今天的 ContainmentIndex 按作业实物（HandlingUnitID）答直接父级，而正式包裹与作业实物
// 之间是版本化关联，由节点作业在识别成功后建立、不由本上下文推断（intake_source 那一路就为此拒绝
// 待识别实物）。拿包裹标识冒充作业实物去问，答出来的「没有父级」不是「没装袋」，是问错了对象。
// 本上下文按红线只走已导出的读端口，缺口归 NO 立票；在那之前答`不知道`，编排据以判不出「已制签
// 或已装袋」那一格、停在未决。读面立起来后在装配点换成真适配器，本类型随之退场。
type UnconnectedConsolidationStageView struct{}

var _ psports.ConsolidationStageView = UnconnectedConsolidationStageView{}

// LoadBaggingFact 不读询问。参数刻意匿名，理由同 UnconnectedCustomsStageView。
func (UnconnectedConsolidationStageView) LoadBaggingFact(
	context.Context,
	psdomain.TenantID,
	psdomain.DeclaredParcelID,
) (psdomain.StageFact, error) {
	return psdomain.StageFactUnknown, nil
}
