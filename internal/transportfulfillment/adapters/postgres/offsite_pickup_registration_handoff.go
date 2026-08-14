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

const offsitePickupRegistrationEventType = "transport-fulfillment.offsite-pickup.registered"

// OutboxOffsitePickupRegistrationHandoff 把对象级揽收登记写入 Outbox，实现
// ports.OffsitePickupRegistrationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxOffsitePickupRegistrationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxOffsitePickupRegistrationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxOffsitePickupRegistrationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxOffsitePickupRegistrationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.OffsitePickupRegistrationHandoff = (*OutboxOffsitePickupRegistrationHandoff)(nil)

type offsitePickupRegistrationPayload struct {
	TenantID string `json:"tenantId"`
	Object   string `json:"object"`
	Attempt  string `json:"attempt"`
}

func offsitePickupRegistrationEventID(key ports.OffsitePickupKey) string {
	// 类型段把本口与已落地的交付生效（同为租户/对象/尝试、同一 source）错开，避免
	// EnqueueOnce 把另一口的已入队当成「同一份」。
	return key.TenantID.String() + "/" + key.Object.String() + "/" + key.Attempt.String() +
		"/offsite-pickup-registration"
}

// HandOffOffsitePickupRegistration 把一份意图入队。信封 ID 取对象级揽收幂等键再加类型
// 段——意图由（租户+对象+尝试）认领（ADR-0043），类型段与交付生效错开。键缺席是装配
// 缺陷，响亮报错不入队。
func (handoff *OutboxOffsitePickupRegistrationHandoff) HandOffOffsitePickupRegistration(
	ctx context.Context,
	intent ports.OffsitePickupRegistrationIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Object.String() == "" || key.Attempt.String() == "" {
		return fmt.Errorf("hand off offsite pickup registration: pickup key is required")
	}

	payload, err := json.Marshal(offsitePickupRegistrationPayload{
		TenantID: key.TenantID.String(),
		Object:   key.Object.String(),
		Attempt:  key.Attempt.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off offsite pickup registration: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := offsitePickupRegistrationEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         offsitePickupRegistrationEventType,
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
		return fmt.Errorf("hand off offsite pickup registration: %w", err)
	}
	return nil
}
