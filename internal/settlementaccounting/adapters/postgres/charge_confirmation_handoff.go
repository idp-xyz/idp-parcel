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

// saEventSource 是本上下文在信封 Source 位上的稳定名。
const saEventSource = "idp-parcel/settlement-accounting"

// chargeConfirmationEventType 是费用确认意图的事件类型。
const chargeConfirmationEventType = "settlement-accounting.charge-confirmation.formed"

// OutboxChargeConfirmationHandoff 把已确认费用意图写进 Outbox，实现
// ports.ChargeConfirmationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxChargeConfirmationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxChargeConfirmationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxChargeConfirmationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxChargeConfirmationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ChargeConfirmationHandoff = (*OutboxChargeConfirmationHandoff)(nil)

type chargeConfirmationPayload struct {
	TenantID string `json:"tenantId"`
	ChargeID string `json:"chargeId"`
}

func chargeConfirmationEventID(tenant, charge string) string {
	return tenant + "/charge/" + charge
}

// HandOffChargeConfirmation 把一份意图入队。信封 ID 取（租户+费用）——意图由费用
// 标识认领（ADR-0043）。租户或费用空白是装配缺陷，响亮报错不入队。
func (handoff *OutboxChargeConfirmationHandoff) HandOffChargeConfirmation(
	ctx context.Context,
	intent ports.ChargeConfirmationHandoffIntent,
) error {
	if intent.TenantID.String() == "" || intent.Charge.ID().String() == "" {
		return fmt.Errorf("hand off charge confirmation: tenant and charge are required")
	}

	payload, err := json.Marshal(chargeConfirmationPayload{
		TenantID: intent.TenantID.String(),
		ChargeID: intent.Charge.ID().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off charge confirmation: %w", err)
	}

	now := handoff.clock.Now().UTC()
	confirmedAt, ok := intent.Charge.ConfirmedAt()
	if !ok {
		return fmt.Errorf("hand off charge confirmation: charge is not confirmed")
	}
	eventID := chargeConfirmationEventID(intent.TenantID.String(), intent.Charge.ID().String())
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       saEventSource,
		Type:         chargeConfirmationEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Charge.ID().String(),
		PartitionKey: eventID,
		OccurredAt:   confirmedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off charge confirmation: %w", err)
	}
	return nil
}
