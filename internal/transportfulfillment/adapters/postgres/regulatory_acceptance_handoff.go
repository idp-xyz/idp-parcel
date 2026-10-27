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

const regulatoryAcceptanceEventType = "transport-fulfillment.regulatory-acceptance.recorded"

// OutboxRegulatoryAcceptanceHandoff 把承接决定回执写入 Outbox，实现
// ports.RegulatoryAcceptanceHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxRegulatoryAcceptanceHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxRegulatoryAcceptanceHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxRegulatoryAcceptanceHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxRegulatoryAcceptanceHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.RegulatoryAcceptanceHandoff = (*OutboxRegulatoryAcceptanceHandoff)(nil)

type regulatoryAcceptancePayload struct {
	TenantID string `json:"tenantId"`
	Item     string `json:"item"`
}

// regulatoryAcceptancePort 是本口在信封 ID 上的口名前缀，与同一来源名下别的口的 ID 空间错开（FingerprintEventID 头注）。
const regulatoryAcceptancePort = "regulatory-acceptance"

// regulatoryAcceptanceEventID 按承接幂等键的两维（租户、协作事项）铸 ID：意图由它们认领（ADR-0043）。
func regulatoryAcceptanceEventID(key ports.DispositionAcceptanceKey) eventing.EventID {
	return outboxintent.FingerprintEventID(regulatoryAcceptancePort, key.Tenant.String(), key.Item.String())
}

// regulatoryAcceptancePartitionKey 保留可读的「租户/协作事项/口名」：一份承接只走本口一拍，分区仍是逐信封，取可读形只为
// 运维从分区键认得出是哪份承接。
func regulatoryAcceptancePartitionKey(key ports.DispositionAcceptanceKey) string {
	return key.Tenant.String() + "/" + key.Item.String() + "/" + regulatoryAcceptancePort
}

// HandOffRegulatoryAcceptance 把一份意图入队。信封 ID 由（租户+协作事项）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxRegulatoryAcceptanceHandoff) HandOffRegulatoryAcceptance(
	ctx context.Context,
	intent ports.RegulatoryAcceptanceHandoffIntent,
) error {
	key := intent.Record.Key
	if key.Tenant.String() == "" || key.Item.String() == "" {
		return fmt.Errorf("hand off regulatory acceptance: disposition key is required")
	}

	payload, err := json.Marshal(regulatoryAcceptancePayload{
		TenantID: key.Tenant.String(),
		Item:     key.Item.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off regulatory acceptance: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           regulatoryAcceptanceEventID(key),
		Source:       tfEventSource,
		Type:         regulatoryAcceptanceEventType,
		Version:      1,
		Scope:        key.Tenant.String(),
		Subject:      key.Item.String(),
		PartitionKey: regulatoryAcceptancePartitionKey(key),
		OccurredAt:   intent.Record.Decision.DecidedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off regulatory acceptance: %w", err)
	}
	return nil
}
