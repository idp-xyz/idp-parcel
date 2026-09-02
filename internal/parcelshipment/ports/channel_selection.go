package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是渠道择优编排要用的两个出向口（票 `label-channel/12` 接线那一步）。
//
// 两个而不是一个：装配回答「有哪些渠道可选」，取成本回答「各自要花多少」，两问的失败方式
// 完全不同——前者答不上来时一个候选都没有，后者答不上来只坏那一个候选（`parcel-pricing`
// 的四种非完成结果各自成格）。并成一个口会逼调用方从一份混合结果里再把两类分开。

// ChannelSelectionQuery 是一次渠道择优的输入：在哪个租户的哪个商业范围下、按哪笔产品—渠道
// 映射、对准哪个时点。
//
// 时点由调用方给而不是由实现取当下时钟：同一份委托重算两次必须得到同一批候选，读时钟会让
// 它随调用时刻漂移，而漂移出来的差别在结果上看不出来。
type ChannelSelectionQuery struct {
	Tenant  domain.TenantID
	Scope   domain.CommercialScopeReference
	Mapping domain.ProductChannelMappingReference
	At      time.Time
}

// ChannelCandidateAssembly 交回该时点可参与择优的渠道候选。
//
// 空列表与错误是两回事：空列表是「映射如实作过答，此刻没有可用渠道」（到期、或被客户约束
// 收窄到空），错误是「装配没能进行」。两者续办不同，实现不得互相顶替。
type ChannelCandidateAssembly interface {
	AssembleChannelCandidates(
		ctx context.Context,
		query ChannelSelectionQuery,
	) ([]domain.ChannelCandidateID, error)
}

// ChannelCandidateCostSource 为一批候选逐个取回成本单维取值。
//
// 交回的是 ChannelCandidateCost 而不是金额：算不出的候选必须带着它的出局格回来，而不是
// 从结果里消失——消失与「成本为零」在下游都表现为「它没赢」，而它俩一个是没资格参选、
// 一个是参选了没赢。
//
// **交回的条数必须与入参候选数相等且一一对应**，逐格作答。少一条即调用方无从知道是哪个
// 候选被漏掉了。
type ChannelCandidateCostSource interface {
	ChannelCandidateCosts(
		ctx context.Context,
		query ChannelSelectionQuery,
		candidates []domain.ChannelCandidateID,
	) ([]domain.ChannelCandidateCost, error)
}
