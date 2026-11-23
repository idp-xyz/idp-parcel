package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const (
	outboundConfirmationEventSource = "idp-parcel/warehouse-fulfillment"
	outboundConfirmationEventType   = "warehouse-fulfillment.outbound-confirmed"
)

// OutboundConfirmationEventType 是交给全程追踪的出库确认。消费方按这串认，不引用本常量。
const OutboundConfirmationEventType = outboundConfirmationEventType

// OutboxOutboundConfirmationHandoff 把本产品网络、已指名包裹的出库确认写进 Outbox。
type OutboxOutboundConfirmationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxOutboundConfirmationHandoff(db *bentopg.DB, store *outbox.Store, clock ports.Clock) (*OutboxOutboundConfirmationHandoff, error) {
	if db == nil || store == nil || clock == nil {
		return nil, fmt.Errorf("warehouse fulfillment postgres: outbound confirmation handoff needs a db, an outbox store and a clock")
	}
	return &OutboxOutboundConfirmationHandoff{db: db, store: store, clock: clock}, nil
}

func (handoff *OutboxOutboundConfirmationHandoff) HandOffOutboundConfirmation(ctx context.Context, tenant, factID, parcelID string, occurredAt time.Time, version, supersedes string) error {
	if version == "" {
		version = "1"
	}
	body, err := json.Marshal(outboundConfirmationPayload{
		TenantID: tenant, FactID: factID, DeclaredParcelID: parcelID, OccurredAt: occurredAt.UTC(),
		Version: version, Supersedes: supersedes,
	})
	if err != nil {
		return fmt.Errorf("hand off outbound confirmation: %w", err)
	}
	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          outboxintent.FingerprintEventID("outbound-confirmation", tenant, factID, parcelID, version),
		Source:      outboundConfirmationEventSource, Type: outboundConfirmationEventType, Version: 1,
		Scope: tenant, Subject: parcelID,
		PartitionKey: tenant + "/outbound-confirmation/" + parcelID,
		OccurredAt:   occurredAt.UTC(), RecordedAt: now, ContentType: eventing.JSONContentType, Payload: body,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off outbound confirmation: %w", err)
	}
	return nil
}

type outboundConfirmationPayload struct {
	TenantID         string    `json:"tenantId"`
	FactID           string    `json:"factId"`
	DeclaredParcelID string    `json:"declaredParcelId"`
	OccurredAt       time.Time `json:"occurredAt"`
	Version          string    `json:"version,omitempty"`
	Supersedes       string    `json:"supersedes,omitempty"`
}
