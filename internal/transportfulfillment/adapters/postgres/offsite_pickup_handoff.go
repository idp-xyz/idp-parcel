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

// offsitePickupPort 是本口在信封 ID 上的口名前缀，与同一来源名下别的口的 ID 空间错开（FingerprintEventID 头注）。
const offsitePickupPort = "offsite-pickup"

// offsitePickupEventID 按揽收尝试幂等键的两维（租户、来源）铸 ID：意图由它们认领（ADR-0043）。
func offsitePickupEventID(key ports.PickupAttemptKey) eventing.EventID {
	return outboxintent.FingerprintEventID(offsitePickupPort, key.TenantID.String(), key.SourceID)
}

// offsitePickupPartitionKey 保留可读的「租户/来源/口名」：一次揽收尝试只走本口一拍，分区仍是逐信封，取可读形只为
// 运维从分区键认得出是哪次尝试。
func offsitePickupPartitionKey(key ports.PickupAttemptKey) string {
	return key.TenantID.String() + "/" + key.SourceID + "/" + offsitePickupPort
}

// HandOffOffsitePickup 把一份意图入队。信封 ID 由（租户+来源）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
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
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           offsitePickupEventID(key),
		Source:       tfEventSource,
		Type:         offsitePickupEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.SourceID,
		PartitionKey: offsitePickupPartitionKey(key),
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
