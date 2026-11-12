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

// —— 按委托读留痕（ADR-0379 决定二）——
//
// 读口与写口分立，判据同 catalogue_read.go 头注（伴生读端口另立，ADR-0077 决定五）：施加编排只写不读，读口只给运营读面。上列的是
// 留痕与两本账的照实转写，不重建估价、不复算、不判占用该不该在。

// PreAcceptanceEstimateEntry 是一份受理前估价连同它的占用结果。PriceCard 是留痕字面拆回的几段；ServiceProduct 空即没申报，或这一行
// 写在补记之前——两者由 DetailRecorded 分开：为假即补记之前，那时产品、各件的计价重量与费用行都没记。
type PreAcceptanceEstimateEntry struct {
	SubmissionVersion string
	Form              string
	PricingScope      string
	PriceCard         domain.VersionReferenceParts
	ServiceProduct    string
	DetailRecorded    bool
	TotalMinor        int64
	Currency          string
	TrialAt           time.Time
	RecordedAt        time.Time
	Parcels           []PreAcceptanceEstimateParcel
	// Occupations 是这份估价占的冻结与信用暴露，冻结在前。组合策略两本账各一条；受限的那一项不入账，所以受限而没有占用时为空。
	Occupations []PreAcceptanceOccupation
}

// PreAcceptanceEstimateParcel 是估价里的一件。PricingWeight 为 nil、ChargeLines 为 nil 即这一件没记那一格。
type PreAcceptanceEstimateParcel struct {
	Parcel        string
	Evaluation    string
	TotalMinor    int64
	Currency      string
	PricingWeight *PreAcceptanceParcelPricingWeight
	ChargeLines   []PreAcceptanceParcelChargeLine
}

type PreAcceptanceParcelPricingWeight struct {
	Value string
	Unit  string
}

// PreAcceptanceParcelChargeLine 的金额是评价的十进制原文、币种是卡本币（domain.PreAcceptanceTrialChargeLine）。
type PreAcceptanceParcelChargeLine struct {
	Code     string
	Effect   string
	Amount   string
	Currency string
}

// PreAcceptanceOccupationKind 指名占用落在哪本账上。
type PreAcceptanceOccupationKind string

const (
	OccupationFundsFreeze    PreAcceptanceOccupationKind = "FUNDS_FREEZE"
	OccupationCreditExposure PreAcceptanceOccupationKind = "CREDIT_EXPOSURE"
)

// PreAcceptanceOccupation 是一条冻结或信用暴露。Status 用各自账本的词（domain.FreezeStatus、domain.ExposureStatus）：两本账的「占着」
// 本就是两个词，并成一个就得替其中一本改名。部分转出时状态仍是占着，ConvertedMinor 大于零、ConvertedAt 是最近一次转出。
type PreAcceptanceOccupation struct {
	Kind           PreAcceptanceOccupationKind
	ID             string
	Status         string
	AmountMinor    int64
	Currency       string
	FormedAt       time.Time
	ReleasedAt     *time.Time
	ConvertedMinor int64
	ConvertedAt    *time.Time
}

// PreAcceptanceEstimateRead 按（租户，委托）列出各次受理前估价，按试算时点排；没有留痕答空列表。委托标识是 parcel-shipment 的原样，
// 本上下文不解释。
type PreAcceptanceEstimateRead interface {
	ListPreAcceptanceEstimates(ctx context.Context, tenant domain.TenantID, shipmentRequest string) ([]PreAcceptanceEstimateEntry, error)
}
