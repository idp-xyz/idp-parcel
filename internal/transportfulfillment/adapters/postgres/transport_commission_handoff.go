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

const transportCommissionEventType = "transport-fulfillment.transport-commission.submitted"

// OutboxTransportCommissionHandoff 把已提交的运输委托写入 Outbox，实现
// ports.TransportCommissionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxTransportCommissionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxTransportCommissionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxTransportCommissionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxTransportCommissionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.TransportCommissionHandoff = (*OutboxTransportCommissionHandoff)(nil)

type transportCommissionPayload struct {
	TenantID   string `json:"tenantId"`
	Commission string `json:"commission"`
}

func transportCommissionEventID(key ports.TransportCommissionKey) string {
	return key.TenantID.String() + "/" + key.Commission.String()
}

// HandOffTransportCommission 把一份意图入队。信封 ID 取委托幂等键——意图由
// （租户+委托）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxTransportCommissionHandoff) HandOffTransportCommission(
	ctx context.Context,
	intent ports.TransportCommissionIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Commission.String() == "" {
		return fmt.Errorf("hand off transport commission: commission key is required")
	}

	payload, err := json.Marshal(transportCommissionPayload{
		TenantID:   key.TenantID.String(),
		Commission: key.Commission.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off transport commission: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := transportCommissionEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         transportCommissionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Commission.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off transport commission: %w", err)
	}
	return nil
}
