package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// PreAcceptanceEstimateTrace 是一次受理前估价随控制结果留下的痕（ADR-0179 决定六）：哪一次控制请求、按哪份提交版本与判断时点要的
// 试算，连同估价自己带着的形态、计价范围、价卡、逐件评价与版本清单、合计。试算评价不进计价的评价册，版本清单只在这里。
type PreAcceptanceEstimateTrace struct {
	Tenant            domain.TenantID
	Scope             domain.SettlementScope
	Request           domain.ControlRequestID
	ShipmentRequest   string
	SubmissionVersion string
	At                time.Time
	Estimate          domain.PreAcceptanceEstimate
	RecordedAt        time.Time
}

// PreAcceptanceEstimateTraceEffect 是写一份留痕的三种落点。重放是同一次控制请求带同一份估价再来；冲突是同一次控制请求带了另一份估价
// ——与两本账「同一请求身份携带不同内容」同一条判据。两种都不改已落的行。
type PreAcceptanceEstimateTraceEffect uint8

const (
	PreAcceptanceEstimateTraceRecorded PreAcceptanceEstimateTraceEffect = iota + 1
	PreAcceptanceEstimateTraceReplay
	PreAcceptanceEstimateTraceConflict
)

// PreAcceptanceEstimateTraces 是留痕的写口。写在环境事务里，与冻结、信用暴露两本账同一事务：控制没落库，留痕也不该落。
// 比内容不比落库时刻。
//
// 判断时点 At 算内容。同一次控制请求跨重驱再写是重放，前提是判断时点不随重驱移动：「提交接收」（PS partycommercial 的
// SubmissionReceiptAsOf）折成该提交版本已记录的系统接收时间、不读本地时钟，满足这个前提。前一轮已提交了留痕（组合策略下账期那一项
// 停在`未配置`时事务照样提交）而换成会随重驱移动的时点语义时，同一请求第二轮会在这里答冲突，施加编排停在`请求冲突`；两本账对同额
// 却是重放。要换那种语义，先改这里的判据。
type PreAcceptanceEstimateTraces interface {
	RecordEstimate(ctx context.Context, trace PreAcceptanceEstimateTrace) (PreAcceptanceEstimateTraceEffect, error)
}
