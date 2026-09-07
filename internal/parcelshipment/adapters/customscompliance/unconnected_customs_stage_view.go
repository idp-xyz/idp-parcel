// Package customscompliance 是 parcel-shipment 消费 customs-compliance 事实的适配器（ADR-0025
// 消费方侧；缝的登记见 ADR-0118）。它只翻译不判断：关务出「形成了没有、提交了没有、关闭了
// 没有」，阶段由 PS 的资料修订编排判。
package customscompliance

import (
	"context"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// UnconnectedCustomsStageView 是关务阶段事实读口在 customs-compliance 还没有按包裹键的读面之前
// 的如实答复：三格一律`不知道`。
//
// 它不是「关务对这个包裹没有案件」——那是三格`不在`，得由关务读面自己答。今天关务的案件键是
// 管辖 × 方向 × 程序 × 义务范围，申报单元记着成员却没有「按包裹反查」的口（DeclarationUnitStore
// 头注写明今天没有消费方、端口不预设方法）；本上下文按红线只走已导出的读端口，缺口归 CC 立票，
// 不在这里绕过它去读表。三格`不知道`让编排判不出阶段、停在未决（不默认最早阶段），这正是
// 「读面不存在或未接 → 判不出阶段 → 未决」在装配点上的落点。读面立起来后在装配点换成真适配器，
// 本类型随之退场。
type UnconnectedCustomsStageView struct{}

var _ psports.CustomsStageView = UnconnectedCustomsStageView{}

// LoadCustomsStageFacts 不读询问。参数刻意匿名：连签名都不给「看一眼包裹再决定」留位置——
// 看了再答同一个值，会让人以为它判过什么。
func (UnconnectedCustomsStageView) LoadCustomsStageFacts(
	context.Context,
	psdomain.TenantID,
	psdomain.DeclaredParcelID,
) (psports.CustomsStageFacts, error) {
	return psports.CustomsStageFacts{
		DataForming: psdomain.StageFactUnknown,
		Submitted:   psdomain.StageFactUnknown,
		CaseClosed:  psdomain.StageFactUnknown,
	}, nil
}
