package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「渠道择优决定」登记册的伴生运营查阅读端口（票 `label-channel/23`，读面形状照 ADR-0077
// 通例）：管理台「渠道择优决定」页的供数面。不拓宽写口 ChannelSelectionDecisionRegistry——扩写侧
// 接口会拆全部写侧测试替身，伴生读端口另立（理由与 parcelpricing 的 EvaluationCatalogueRead 同句）。
//
// 读面只列不判：并列冲突的人工裁决是另一种形状的决定（裁决人与裁决规则属 `PAR-NET-16` 待提供的实例
// 半边，机制侧怎么落要先过 /domain-modeling），本口不给任何动作。也不按时间隐式截断：留痕要求待提供，
// 「近期」之类窄口由调用方显式给，读口只按租户与对象过滤、按 limit 截页。
//
// 两口读回的都是整条决定（头行 + 逐候选结果），逐字段过领域构造函数再进重建门——列面就是权威内容，
// 与 ChannelSelectionDecisionRegistry.ListBySubject 同一条装载纪律，不另造一套「只读列面」的行类型。

// TiedChannelSelectionFilter 是并列冲突列表的可选收窄：零值不收窄，带对象即只列该（商业范围 + 产品—渠道
// 映射）下的并列冲突。收窄维只有对象一维——租户在方法签名上，结论已被这一口钉成 TIED。
type TiedChannelSelectionFilter struct {
	subject    domain.ChannelSelectionSubject
	hasSubject bool
}

// EveryTiedChannelSelection 不收窄：租户下全部并列冲突。
func EveryTiedChannelSelection() TiedChannelSelectionFilter { return TiedChannelSelectionFilter{} }

// TiedChannelSelectionsOf 只列该对象下的并列冲突。
func TiedChannelSelectionsOf(subject domain.ChannelSelectionSubject) TiedChannelSelectionFilter {
	return TiedChannelSelectionFilter{subject: subject, hasSubject: true}
}

// Subject 交出收窄对象，第二个返回值为 false 即不收窄。
func (filter TiedChannelSelectionFilter) Subject() (domain.ChannelSelectionSubject, bool) {
	return filter.subject, filter.hasSubject
}

// ChannelSelectionDecisionRead 是「渠道择优决定」登记册的伴生运营查阅读端口。
type ChannelSelectionDecisionRead interface {
	// ListTiedChannelSelectionDecisions 按租户列结论为并列冲突（TIED）的决定——`PAR-NET-16`「并列且无法
	// 选出唯一一条时为冲突，交人工裁决」那一格的待办面。按决定时刻倒序、同刻按标识倒序：最近一次冲突在
	// 最上面。limit 必须为正；从未择优过或没有冲突如实答空切片，不是错误。
	ListTiedChannelSelectionDecisions(
		ctx context.Context,
		tenant domain.TenantID,
		filter TiedChannelSelectionFilter,
		limit int,
	) ([]domain.ChannelSelectionDecision, error)
	// FindChannelSelectionDecision 按（租户 + 决定标识）取一条决定连逐候选结果。第二个返回值为 false 即
	// 该租户下没有这条记录——不区分「不存在」与「属别的租户」（ADR-0029 探针纪律）。
	FindChannelSelectionDecision(
		ctx context.Context,
		tenant domain.TenantID,
		id domain.ChannelSelectionDecisionID,
	) (domain.ChannelSelectionDecision, bool, error)
}
