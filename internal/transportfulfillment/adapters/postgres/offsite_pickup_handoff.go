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

// tfEventSource 是本上下文在信封 Source 位上的稳定名。
const tfEventSource = "idp-parcel/transport-fulfillment"

const offsitePickupEventType = "transport-fulfillment.offsite-pickup.formed"

// OutboxOffsitePickupHandoff 把已提交的对象级揽收写入 Outbox，实现
// ports.OffsitePickupHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxOffsitePickupHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxOffsitePickupHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxOffsitePickupHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxOffsitePickupHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.OffsitePickupHandoff = (*OutboxOffsitePickupHandoff)(nil)

type offsitePickupPayload struct {
	TenantID string `json:"tenantId"`
	SourceID string `json:"sourceId"`
}

func offsitePickupEventID(key ports.PickupAttemptKey) string {
	return key.TenantID.String() + "/" + key.SourceID + "/offsite-pickup"
}

// HandOffOffsitePickup 把一份意图入队。信封 ID 取揽收尝试幂等键再加类型段——意图由
// （租户+来源）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxOffsitePickupHandoff) HandOffOffsitePickup(
	ctx context.Context,
	intent ports.OffsitePickupHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.SourceID == "" {
		return fmt.Errorf("hand off offsite pickup: pickup attempt key is required")
	}

	payload, err := json.Marshal(offsitePickupPayload{
		TenantID: key.TenantID.String(),
		SourceID: key.SourceID,
	})
	if err != nil {
		return fmt.Errorf("hand off offsite pickup: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := offsitePickupEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         offsitePickupEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.SourceID,
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off offsite pickup: %w", err)
	}
	return nil
}
