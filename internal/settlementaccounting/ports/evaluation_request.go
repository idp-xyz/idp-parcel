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

// EvaluationRequestRecord 是一次「请求评价」越过提交边界留下的东西（UC-SA-002 步 2 的 SA 半边）。
type EvaluationRequestRecord struct {
	Key        EvaluationRequestKey
	Request    domain.EvaluationRequest
	RecordedAt time.Time
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

// EvaluationRequestRegistry 是评价请求的登记面。Save 的写入代数同 ADR-0031：撞身份键或撞自然键唯一
// 约束都答`已存在`、不覆盖先到者——自然键（租户 + 主要范围 + 计算目的 + 引用集合摘要）上的唯一约束
// 在库上，幂等由它守；FindByNaturalKey 让编排在撞上时读回先到者、交回原 ID，而不是答失败。
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
