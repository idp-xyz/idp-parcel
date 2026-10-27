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

// capacityConsumptionPort 是本口在信封 ID 上的口名前缀，与同一来源名下别的口（如交付生效）的 ID 空间错开（FingerprintEventID 头注）。
const capacityConsumptionPort = "capacity-consumption"

// capacityConsumptionEventID 按（租户、池、预占）三维铸 ID：意图由它们认领（ADR-0043）。三维逐个分开传、由
// FingerprintEventID 分隔编码：池身份写作「班次身份/维度词」，自带「/」，班次身份本身也可能带「/」，用「/」拼起来
// 的两组不同的键会拼成同一个串，被 EnqueueOnce 静默吞掉第二份；分隔编码下维度边界移位算不出同一个 ID。
func capacityConsumptionEventID(key ports.CapacityPoolKey, reservation string) eventing.EventID {
	return outboxintent.FingerprintEventID(capacityConsumptionPort, key.TenantID.String(), key.Pool.String(), reservation)
}

// capacityConsumptionPartitionKey 保留可读的「租户/池/预占/口名」：一份预占只消耗一次，分区仍是逐信封，取可读形只为
// 运维从分区键认得出是哪份预占。
func capacityConsumptionPartitionKey(key ports.CapacityPoolKey, reservation string) string {
	return key.TenantID.String() + "/" + key.Pool.String() + "/" + reservation + "/" + capacityConsumptionPort
}

// HandOffCapacityConsumption 把一份意图入队。信封 ID 由（租户+池+预占）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
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
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           capacityConsumptionEventID(key, intent.Reservation.String()),
		Source:       tfEventSource,
		Type:         capacityConsumptionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Pool.String() + "/" + intent.Reservation.String(),
		PartitionKey: capacityConsumptionPartitionKey(key, intent.Reservation.String()),
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
