package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

const settlementApplicationEventType = "settlement-accounting.settlement-application.applied"

// OutboxSettlementApplicationHandoff 把核销写入 Outbox，实现
// ports.SettlementApplicationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxSettlementApplicationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSettlementApplicationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSettlementApplicationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxSettlementApplicationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SettlementApplicationHandoff = (*OutboxSettlementApplicationHandoff)(nil)

type settlementApplicationPayload struct {
	TenantID    string `json:"tenantId"`
	Application string `json:"application"`
}

func settlementApplicationEventID(key ports.SettlementApplicationKey) string {
	return key.TenantID.String() + "/application/" + key.Application.String()
}

// HandOffSettlementApplication 把一份意图入队。信封 ID 取（租户+核销）并加 /application/
// 段——意图由核销幂等键认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxSettlementApplicationHandoff) HandOffSettlementApplication(
	ctx context.Context,
	intent ports.SettlementApplicationIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Application.String() == "" {
		return fmt.Errorf("hand off settlement application: application key is required")
	}

	payload, err := json.Marshal(settlementApplicationPayload{
		TenantID:    key.TenantID.String(),
		Application: key.Application.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off settlement application: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := settlementApplicationEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       saEventSource,
		Type:         settlementApplicationEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Application.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off settlement application: %w", err)
	}
	return nil
}
