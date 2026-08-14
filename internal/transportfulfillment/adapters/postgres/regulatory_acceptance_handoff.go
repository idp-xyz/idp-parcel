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

func regulatoryAcceptanceEventID(key ports.DispositionAcceptanceKey) string {
	return key.Tenant.String() + "/" + key.Item.String() + "/regulatory-acceptance"
}

// HandOffRegulatoryAcceptance 把一份意图入队。信封 ID 取承接幂等键再加类型段——意图
// 由（租户+协作事项）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
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
	eventID := regulatoryAcceptanceEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         regulatoryAcceptanceEventType,
		Version:      1,
		Scope:        key.Tenant.String(),
		Subject:      key.Item.String(),
		PartitionKey: eventID,
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
