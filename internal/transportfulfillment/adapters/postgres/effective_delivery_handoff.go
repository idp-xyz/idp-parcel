package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

const effectiveDeliveryEventType = "transport-fulfillment.effective-delivery.registered"

// OutboxEffectiveDeliveryHandoff 把交付生效写入 Outbox，实现
// ports.EffectiveDeliveryHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxEffectiveDeliveryHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxEffectiveDeliveryHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxEffectiveDeliveryHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxEffectiveDeliveryHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.EffectiveDeliveryHandoff = (*OutboxEffectiveDeliveryHandoff)(nil)

type effectiveDeliveryPayload struct {
	TenantID string `json:"tenantId"`
	Object   string `json:"object"`
	Attempt  string `json:"attempt"`
}

func effectiveDeliveryEventID(key ports.EffectiveDeliveryKey) string {
	return key.TenantID.String() + "/" + key.Object.String() + "/" + key.Attempt.String()
}

// HandOffEffectiveDelivery 把一份意图入队。信封 ID 取交付生效键——意图由
// （租户+对象+尝试）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxEffectiveDeliveryHandoff) HandOffEffectiveDelivery(
	ctx context.Context,
	intent ports.EffectiveDeliveryHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Object.String() == "" || key.Attempt.String() == "" {
		return fmt.Errorf("hand off effective delivery: delivery key is required")
	}

	payload, err := json.Marshal(effectiveDeliveryPayload{
		TenantID: key.TenantID.String(),
		Object:   key.Object.String(),
		Attempt:  key.Attempt.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off effective delivery: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := effectiveDeliveryEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         effectiveDeliveryEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Object.String() + "/" + key.Attempt.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off effective delivery: %w", err)
	}
	return nil
}
