package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// EvaluationRequestKey 是评价请求登记册的身份键：租户 + 铸造 ID（票 sa-cc/08 裁决 2）。
type EvaluationRequestKey struct {
	TenantID domain.TenantID
	Request  domain.EvaluationRequestID
}

// EvaluationRequestRecord 是一次「请求评价」越过提交边界留下的东西（UC-SA-002 步 2 的 SA 半边）。StopCause 是册上的
// 停因格：登记时没有，请求停下之后由记停因的那一口写下（EvaluationRequestStopRecorder）。ZeroTotal 是`负向结束（零额）`
// 那一格：评价已完成而合计为零，由零额写口写下（EvaluationRequestZeroTotalRecorder，ADR-0306 决定三）；与停因互斥，
// 登记时同样没有。
type EvaluationRequestRecord struct {
	Key        EvaluationRequestKey
	Request    domain.EvaluationRequest
	RecordedAt time.Time
	StopCause  domain.EvaluationStopCause
	ZeroTotal  bool
}

type EvaluationRequestSaveOutcome uint8

const (
	EvaluationRequestSaveOutcomeInvalid EvaluationRequestSaveOutcome = iota
	EvaluationRequestSaved
	EvaluationRequestAlreadyRequested
)

func (outcome EvaluationRequestSaveOutcome) String() string {
	switch outcome {
	case EvaluationRequestSaved:
		return "SAVED"
	case EvaluationRequestAlreadyRequested:
		return "ALREADY_REQUESTED"
	default:
		return ""
	}
}

// EvaluationRequestRegistry 是评价请求的登记面。Save 的写入代数同 ADR-0031：撞身份键、撞（自然键、代次）或撞前一代
// 唯一约束都答`已存在`、不覆盖先到者——自然键（租户 + 主要范围 + 计算目的 + 引用集合摘要）加代次上的唯一约束在库上，
// 幂等由它守，一份请求至多一个下一代也由它守（ADR-0229 决定一）；FindByNaturalKey 交回这条代次链上最新的那一代，
// 让编排在撞上时读回先到者、交回原 ID，而不是答失败。Save 收到带停因或带零额的记录即拒：两格各只由自己那一口写。
type EvaluationRequestRegistry interface {
	Save(ctx context.Context, record EvaluationRequestRecord) (EvaluationRequestSaveOutcome, error)
	FindByNaturalKey(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.EvaluationRequestNaturalKey,
	) (EvaluationRequestRecord, bool, error)
}

// EvaluationRequestView 是登记册的只读半边：按铸造 ID 取回一份请求。评价形成后回指 evaluationRequestId，
// 采用评价的消费者按它取发生项 / 费用项目 / 供应商协议引用去凑 FormSupplierExpectedCostCommand——走这里，拿不到 Save。
// 与登记面同一个适配器实现，行模型只有一份。
type EvaluationRequestView interface {
	FindByID(ctx context.Context, key EvaluationRequestKey) (EvaluationRequestRecord, bool, error)
}

// EvaluationRequestIdentityFactory 为新请求签发铸造 ID（裁决 2）。
type EvaluationRequestIdentityFactory interface {
	MintEvaluationRequestID(ctx context.Context) (domain.EvaluationRequestID, error)
}

// EvaluationRequestIntent 把一份已登记的评价请求交给 parcel-pricing（裁决 1：信封，不同步调用）。载荷
// 只带引用 {tenantId, evaluationRequestId}，来源引用由消费方按 ID 读 EvaluationRequestView——请求
// 内容只有登记册一处权威。重放重发同一份（ADR-0043）。
type EvaluationRequestIntent struct {
	Record EvaluationRequestRecord
}

// EvaluationRequestHandoff 把评价请求写入 Outbox（`OutboxEvaluationRequestHandoff`）。
type EvaluationRequestHandoff interface {
	HandOffEvaluationRequest(ctx context.Context, intent EvaluationRequestIntent) error
}

// EvaluationRequestZeroTotalEffect 是在 BUY 请求册上记`负向结束（零额）`的落点。
type EvaluationRequestZeroTotalEffect uint8

const (
	EvaluationRequestZeroTotalEffectInvalid EvaluationRequestZeroTotalEffect = iota
	// EvaluationRequestZeroTotalRecorded：请求原本既没有停因也没有零额，这一格写下了。
	EvaluationRequestZeroTotalRecorded
	// EvaluationRequestZeroTotalReplayed：册上已是零额，没有再写。
	EvaluationRequestZeroTotalReplayed
	// EvaluationRequestAlreadyStopped：册上已有停因，不覆盖——同一份评价只折得出一种去向，撞上另一种是两侧说法矛盾。
	EvaluationRequestAlreadyStopped
)

func (effect EvaluationRequestZeroTotalEffect) String() string {
	switch effect {
	case EvaluationRequestZeroTotalRecorded:
		return "ZERO_TOTAL_RECORDED"
	case EvaluationRequestZeroTotalReplayed:
		return "REPLAYED"
	case EvaluationRequestAlreadyStopped:
		return "ALREADY_STOPPED"
	default:
		return ""
	}
}

// EvaluationRequestZeroTotalRecorder 是 BUY 请求册上`负向结束（零额）`的写口（ADR-0306 决定三）。BUY 请求册没有状态格：
// 形成了的由派生的预期成本版本可查，停下的记停因，评价已完成而合计为零的记这一格——不记它，这份请求在册上与「评价还没
// 回来」分不出。它不是停因：不进续办队列，运营续办也不推。只从「既无停因也无零额」往外走，册上已是零额答重放，已有
// 停因答另有去向，都不覆盖。请求不在册上是错误。要求环境事务。
type EvaluationRequestZeroTotalRecorder interface {
	RecordZeroTotal(ctx context.Context, key EvaluationRequestKey) (EvaluationRequestZeroTotalEffect, error)
}
