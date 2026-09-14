package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// evaluationRequestEventType 是评价请求提交意图的事件类型（票 sa-cc/08 做法 2）。SA→PP 这一向与 PP→SA 的
// `parcel-pricing.evaluation.recorded` 同为信封（裁决 1：一条缝两个方向一种机制）。
const evaluationRequestEventType eventing.EventType = "settlement-accounting.evaluation-request.submitted"

// OutboxEvaluationRequestHandoff 把评价请求提交写入 Outbox，实现 ports.EvaluationRequestHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxEvaluationRequestHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxEvaluationRequestHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxEvaluationRequestHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxEvaluationRequestHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.EvaluationRequestHandoff = (*OutboxEvaluationRequestHandoff)(nil)

// evaluationRequestPayload 只带下游按键读回所需的引用：租户与铸造的请求 ID。主要范围、计算目的与来源引用
// （发生项 / 费用项目 / 供应商协议）一个都不带——请求内容只有登记册一处权威，消费方按 ID 读 ports.EvaluationRequestView；信封里再抄
// 一份就是第二处权威（票面做法 2）。
type evaluationRequestPayload struct {
	TenantID            string `json:"tenantId"`
	EvaluationRequestID string `json:"evaluationRequestId"`
}

// evaluationRequestPartitionKey 取「租户/评价请求」——主体是这一份请求。
//
// ID 管幂等、分区键管顺序（ADR-0069 决定二）。首发只有「已提交」一拍，一份请求一封信；分区仍取到请求
// 而不是取到信封，是为了让这份请求日后若有第二拍（撤回、失效）排在提交之后，而不是各自成区乱序。
// 两份不同的请求之间没有先后可言（PP 各自形成评价），所以主体不取到主要范围或租户。主体名「租户/评价
// 请求」登在 internal/architecture 的分区主体登记表（ADR-0074 决定五）。
func evaluationRequestPartitionKey(key ports.EvaluationRequestKey) string {
	return key.TenantID.String() + "/evaluation-request/" + key.Request.String()
}

// HandOffEvaluationRequest 把一份意图入队。信封 ID 取分区键加状态段 /submitted：请求没有版本维，状态段是
// 让 ID 与分区键不同源的那一维（信封分区键门禁的形状——无版本用状态段，不为此在领域里加假版本）；重放
// 同一份算出同一个 ID，被 EnqueueOnce 认领吞掉（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxEvaluationRequestHandoff) HandOffEvaluationRequest(
	ctx context.Context,
	intent ports.EvaluationRequestIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Request.String() == "" {
		return fmt.Errorf("hand off evaluation request: evaluation request key is required")
	}

	payload, err := json.Marshal(evaluationRequestPayload{
		TenantID:            key.TenantID.String(),
		EvaluationRequestID: key.Request.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off evaluation request: %w", err)
	}

	partitionKey := evaluationRequestPartitionKey(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(partitionKey + "/submitted"),
		Source:       saEventSource,
		Type:         evaluationRequestEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Request.String(),
		PartitionKey: partitionKey,
		OccurredAt:   intent.Record.Request.RequestedAt().UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off evaluation request: %w", err)
	}
	return nil
}
