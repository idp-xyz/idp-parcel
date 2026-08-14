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

const capacityConsumptionEventType = "transport-fulfillment.capacity-consumption.recorded"

// OutboxCapacityConsumptionHandoff 把容量消耗写入 Outbox，实现
// ports.CapacityConsumptionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCapacityConsumptionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCapacityConsumptionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCapacityConsumptionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxCapacityConsumptionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CapacityConsumptionHandoff = (*OutboxCapacityConsumptionHandoff)(nil)

type capacityConsumptionPayload struct {
	TenantID    string `json:"tenantId"`
	Pool        string `json:"pool"`
	Reservation string `json:"reservation"`
	Assignment  string `json:"assignment"`
	Quantity    int64  `json:"quantity"`
}

func capacityConsumptionEventID(key ports.CapacityPoolKey, reservation string) string {
	// 类型段把本口与已落地交付生效（同为租户/甲/乙 三元组、同一 source）错开。
	return key.TenantID.String() + "/" + key.Pool.String() + "/" + reservation + "/capacity-consumption"
}

// HandOffCapacityConsumption 把一份意图入队。信封 ID 取容量池/预占键再加类型段——意图
// 由（租户+池+预占）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxCapacityConsumptionHandoff) HandOffCapacityConsumption(
	ctx context.Context,
	intent ports.CapacityConsumptionIntent,
) error {
	key := intent.Pool.Key
	if key.TenantID.String() == "" || key.Pool.String() == "" || intent.Reservation.String() == "" {
		return fmt.Errorf("hand off capacity consumption: pool and reservation are required")
	}

	payload, err := json.Marshal(capacityConsumptionPayload{
		TenantID:    key.TenantID.String(),
		Pool:        key.Pool.String(),
		Reservation: intent.Reservation.String(),
		Assignment:  intent.Assignment.String(),
		Quantity:    intent.Quantity,
	})
	if err != nil {
		return fmt.Errorf("hand off capacity consumption: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := capacityConsumptionEventID(key, intent.Reservation.String())
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         capacityConsumptionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Pool.String() + "/" + intent.Reservation.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Pool.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off capacity consumption: %w", err)
	}
	return nil
}
