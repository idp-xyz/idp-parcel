package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是「渠道择优决定」记录的登记册端口（票 `label-channel/14` 裁决）：只追加，按被择优对象
// 列历史。没有 Update、没有 Delete——记录落库后就是历史，重跑择优铸新记录（裁决原句「整条记录
// 只追加：重跑择优形成新记录，不改旧记录」）。
//
// 也没有清理口与保留期：`PAR-NET-16` 的留痕要求（留多久、给谁看）在登记册里待提供，属实例半边，
// 机制不设默认值。运营查阅面另立读口与票（ADR-0077 读面通例），本口只管登记。

// ChannelSelectionDecisionAppendOutcome 是追加的封闭答案（ADR-0031：写口的否定结果是答案不是
// 错误）。`已登记`是同一条记录重放到位——决定标识由签发口铸，撞键只可能是重放，不是两次择优。
type ChannelSelectionDecisionAppendOutcome uint8

const (
	ChannelSelectionDecisionAppendOutcomeInvalid ChannelSelectionDecisionAppendOutcome = iota
	ChannelSelectionDecisionAppended
	ChannelSelectionDecisionAlreadyRecorded
)

// ChannelSelectionDecisionRegistry 是登记册的写口与按对象列历史的读口。
//
// Append 必须在事务内调用（无事务被 RequireExecutor 拒绝）：择优编排在同一事务里落定选中者与
// 决定记录，两者要么一起成立、要么一起消失——只留一半的话，读面上就有一次没人记得的择优。
type ChannelSelectionDecisionRegistry interface {
	Append(
		ctx context.Context,
		decision domain.ChannelSelectionDecision,
	) (ChannelSelectionDecisionAppendOutcome, error)
	// ListBySubject 按（租户 + 被择优对象）取回全部决定，按决定时刻升序、同刻按标识升序——
	// 同一对象多条记录按决定时刻构成择优历史。空切片是「从未择优过」，不是错误。
	ListBySubject(
		ctx context.Context,
		tenant domain.TenantID,
		subject domain.ChannelSelectionSubject,
	) ([]domain.ChannelSelectionDecision, error)
}

// ChannelSelectionDecisionIdentity 签发决定记录标识。由编排铸、不从内容派生：同一票、同一批
// 候选重跑两次是两条记录（ChannelSelectionDecisionID 的注释）。
type ChannelSelectionDecisionIdentity interface {
	NextChannelSelectionDecisionID(ctx context.Context) (domain.ChannelSelectionDecisionID, error)
}
