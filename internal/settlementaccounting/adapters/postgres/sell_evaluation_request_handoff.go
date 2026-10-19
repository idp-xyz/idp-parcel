package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// sellEvaluationRequestEventType 是 SELL 评价请求提交意图的事件类型（ADR-0190 决定一）。与 BUY 的
// `settlement-accounting.evaluation-request.submitted` 分开：计价侧按类型分派到两个消费者，各按各的请求册回读。
const sellEvaluationRequestEventType eventing.EventType = "settlement-accounting.sell-evaluation-request.submitted"

// sellEvaluationRequestEventIDPort 是信封 ID 指纹的口名前缀，只求本来源名下几只口的 ID 空间互不相交。
const sellEvaluationRequestEventIDPort = "sell-evaluation-request"

// OutboxSellEvaluationRequestHandoff 把 SELL 评价请求提交写入 Outbox，实现 ports.SellEvaluationRequestHandoff。入队
// 一步由 outboxintent.EnqueueOnce 承担。
type OutboxSellEvaluationRequestHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSellEvaluationRequestHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSellEvaluationRequestHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxSellEvaluationRequestHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SellEvaluationRequestHandoff = (*OutboxSellEvaluationRequestHandoff)(nil)

// sellEvaluationRequestPayload 只带下游按键读回所需的引用：租户与铸造的请求 ID。包裹、账户、币种、计价范围、时点与
// 来源事实一个都不带——请求内容只有登记册一处权威，信封里再抄一份就是第二处权威。
type sellEvaluationRequestPayload struct {
	TenantID                string `json:"tenantId"`
	SellEvaluationRequestID string `json:"sellEvaluationRequestId"`
}

// sellEvaluationRequestPartitionKey 取「租户/包裹」——排队主体是包裹，不是请求：同一件包裹在两种计费时点各发一份
// 请求时，两拍排在同一条队里，先到的先被计价接住。键带 /sell-evaluation-request/ 段，与 parcel-shipment、visibility-exception
// 按包裹排的队不是同一个字符串：两边之间没有消费方依赖跨口到达序，并进一条队只会互相堵头。主体名登在
// internal/architecture 的分区主体登记表（ADR-0074 决定五）。
func sellEvaluationRequestPartitionKey(record ports.SellEvaluationRequestRecord) string {
	return record.Key.TenantID.String() + "/sell-evaluation-request/" + record.Request.Parcel().String()
}

// sellEvaluationRequestEventID 取（租户 / 请求 / submitted）的定长指纹：一份请求一封，重放同一份由 EnqueueOnce 按 ID 吞掉
// （ADR-0043）；状态段让日后同一份请求的第二拍另得一个 ID。租户与请求标识多长归实例半边，指纹形的理由在
// outboxintent.FingerprintEventID 头注。
func sellEvaluationRequestEventID(key ports.SellEvaluationRequestKey) eventing.EventID {
	return outboxintent.FingerprintEventID(sellEvaluationRequestEventIDPort,
		key.TenantID.String(), key.Request.String(), "submitted")
}

// HandOffSellEvaluationRequest 把一份意图入队。键、包裹或登记时刻缺席是装配缺陷，响亮报错不入队——这个口只该在请求行
// 刚落库（或撞上已在册的同一份）的那一格被调。
func (handoff *OutboxSellEvaluationRequestHandoff) HandOffSellEvaluationRequest(
	ctx context.Context,
	intent ports.SellEvaluationRequestIntent,
) error {
	record := intent.Record
	if record.Key.TenantID.String() == "" || record.Key.Request.String() == "" || record.Request.Parcel().String() == "" {
		return fmt.Errorf("hand off sell evaluation request: tenant, request and parcel are required")
	}
	if record.RecordedAt.IsZero() {
		return fmt.Errorf("hand off sell evaluation request: registration moment is required")
	}

	payload, err := json.Marshal(sellEvaluationRequestPayload{
		TenantID:                record.Key.TenantID.String(),
		SellEvaluationRequestID: record.Key.Request.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off sell evaluation request: %w", err)
	}

	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           sellEvaluationRequestEventID(record.Key),
		Source:       saEventSource,
		Type:         sellEvaluationRequestEventType,
		Version:      1,
		Scope:        record.Key.TenantID.String(),
		Subject:      record.Key.Request.String(),
		PartitionKey: sellEvaluationRequestPartitionKey(record),
		OccurredAt:   record.RecordedAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off sell evaluation request: %w: %w", ports.ErrSellEvaluationRequestHandoffRejected, err)
		}
		return fmt.Errorf("hand off sell evaluation request: %w", err)
	}
	return nil
}
