package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// StatementCycleRegister 是对账周期册的写口：一户一份，同内容答重放，异内容答冲突、不覆盖（ADR-0339 决定三）。
type StatementCycleRegister interface {
	SaveStatementCycle(ctx context.Context, tenant domain.TenantID, cycle domain.StatementCycle, at time.Time) (CatalogueRegistrationEffect, error)
}

// StatementCycleRecord 是册上一户的周期。
type StatementCycleRecord struct {
	Tenant domain.TenantID
	Cycle  domain.StatementCycle
}

// StatementCycleCut 是到点截单对一个截止日记下的结果。
type StatementCycleCut struct {
	Closing   domain.AttributionDate
	Outcome   string
	Statement string
	CutAt     time.Time
}

// StatementCycleLedger 是到点截单读写的那几样：册上各户的周期、每户最后截到哪个截止日、记一个截止日。
type StatementCycleLedger interface {
	ListStatementCycles(ctx context.Context) ([]StatementCycleRecord, error)
	LastCycleCut(ctx context.Context, tenant domain.TenantID, account domain.SettlementAccountID) (domain.AttributionDate, bool, error)
	RecordCycleCut(ctx context.Context, tenant domain.TenantID, account domain.SettlementAccountID, cut StatementCycleCut) error
}

// CycleCutCandidate 是一期能收的一笔费用：标识与结算金额（最小币单位）。
type CycleCutCandidate struct {
	Charge domain.CustomerChargeID
	Minor  int64
}

// CycleCutCandidateView 收拢一户本期能入单的费用：已确认、归属日不晚于截止日、结算币种是这一户的币种、不在任何未作废的已发布
// 单上。没有下限：起算日之后的第一期把此前没出账的都收进来。归属日没形成的不在这里，先经 ListUnattributedConfirmedCharges 补判。
// 截单编排照旧逐件判一遍，这里只是替执行器列清单。
type CycleCutCandidateView interface {
	ListCycleCutCandidates(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
		currency domain.CurrencyCode,
		through domain.AttributionDate,
	) ([]CycleCutCandidate, error)
	// ListUnattributedConfirmedCharges 交回这一户已确认、还没有归属日的费用：确认时对账安排或归属日形态没登，判不出。
	ListUnattributedConfirmedCharges(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccountID,
	) ([]domain.CustomerChargeID, error)
}
