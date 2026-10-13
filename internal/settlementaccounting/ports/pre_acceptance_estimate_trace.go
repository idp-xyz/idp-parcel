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
type PreAcceptanceEstimateTraces interface {
	RecordEstimate(ctx context.Context, trace PreAcceptanceEstimateTrace) (PreAcceptanceEstimateTraceEffect, error)
}
