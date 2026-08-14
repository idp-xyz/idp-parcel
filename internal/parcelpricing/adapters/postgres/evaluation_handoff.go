package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// ppEventSource 是本上下文在信封 Source 位上的稳定名。
const ppEventSource = "idp-parcel/parcel-pricing"

// evaluationEventType 是评价结果意图的事件类型。
const evaluationEventType = "parcel-pricing.evaluation.recorded"

// OutboxEvaluationHandoff 把评价结果写入 Outbox，实现 ports.EvaluationHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxEvaluationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxEvaluationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxEvaluationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel pricing postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel pricing postgres: clock is nil")
	}
	return &OutboxEvaluationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.EvaluationHandoff = (*OutboxEvaluationHandoff)(nil)

// evaluationPayload 是意图载荷的传输形状：只有下游 FindByID 所需的评价标识与租户，
// 不带费用行或价卡快照。
type evaluationPayload struct {
	TenantID     string `json:"tenantId"`
	EvaluationID string `json:"evaluationId"`
}

// HandOffEvaluation 把一份意图入队。信封 ID 取评价标识——意图由评价标识认领
// （ADR-0043）。评价标识或租户缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxEvaluationHandoff) HandOffEvaluation(
	ctx context.Context,
	intent ports.EvaluationHandoffIntent,
) error {
	evaluation := intent.Evaluation
	tenant := evaluation.Input().TenantID().String()
	if evaluation.ID().String() == "" || tenant == "" {
		return fmt.Errorf("hand off evaluation: evaluation id and tenant are required")
	}

	payload, err := json.Marshal(evaluationPayload{
		TenantID:     tenant,
		EvaluationID: evaluation.ID().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off evaluation: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := evaluation.ID().String()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ppEventSource,
		Type:         evaluationEventType,
		Version:      1,
		Scope:        tenant,
		Subject:      eventID,
		PartitionKey: tenant + "/" + eventID,
		OccurredAt:   evaluation.Input().BusinessAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off evaluation: %w", err)
	}
	return nil
}
