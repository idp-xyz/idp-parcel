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
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

const (
	claimAmountEventType     eventing.EventType = "settlement-accounting.claim-amount.formed"
	receivableEventType      eventing.EventType = "settlement-accounting.recovery-receivable.formed"
	acknowledgementEventType eventing.EventType = "settlement-accounting.recovery-acknowledgement.formed"
	claimAdjustmentEventType eventing.EventType = "settlement-accounting.claim-adjustment.formed"
)

// OutboxClaimSettlementHandoff 把索赔结算四类记录写入 Outbox，实现
// ports.ClaimSettlementHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxClaimSettlementHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxClaimSettlementHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxClaimSettlementHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxClaimSettlementHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ClaimSettlementHandoff = (*OutboxClaimSettlementHandoff)(nil)

type claimSettlementPayload struct {
	TenantID        string `json:"tenantId"`
	ClaimAmount     string `json:"claimAmount,omitempty"`
	Receivable      string `json:"receivable,omitempty"`
	Acknowledgement string `json:"acknowledgement,omitempty"`
	Adjustment      string `json:"adjustment,omitempty"`
}

type claimSettlementShape struct {
	eventID    string
	eventType  eventing.EventType
	tenant     string
	subject    string
	occurredAt time.Time
	payload    claimSettlementPayload
}

func claimSettlementIdentity(intent ports.ClaimSettlementIntent) (claimSettlementShape, error) {
	if intent.ClaimAmount.Key.Amount.String() != "" {
		tenant := intent.ClaimAmount.Key.TenantID.String()
		amount := intent.ClaimAmount.Key.Amount.String()
		if tenant == "" {
			return claimSettlementShape{}, fmt.Errorf("hand off claim settlement: claim amount tenant is required")
		}
		return claimSettlementShape{
			eventID:    tenant + "/claim-amount/" + amount,
			eventType:  claimAmountEventType,
			tenant:     tenant,
			subject:    amount,
			occurredAt: intent.ClaimAmount.RecordedAt.UTC(),
			payload: claimSettlementPayload{
				TenantID:    tenant,
				ClaimAmount: amount,
			},
		}, nil
	}
	if intent.Receivable.Key.Receivable.String() != "" {
		tenant := intent.Receivable.Key.TenantID.String()
		receivable := intent.Receivable.Key.Receivable.String()
		if tenant == "" {
			return claimSettlementShape{}, fmt.Errorf("hand off claim settlement: receivable tenant is required")
		}
		return claimSettlementShape{
			eventID:    tenant + "/receivable/" + receivable,
			eventType:  receivableEventType,
			tenant:     tenant,
			subject:    receivable,
			occurredAt: intent.Receivable.RecordedAt.UTC(),
			payload: claimSettlementPayload{
				TenantID:   tenant,
				Receivable: receivable,
			},
		}, nil
	}
	if intent.Acknowledgement.Key.Acknowledgement.String() != "" {
		tenant := intent.Acknowledgement.Key.TenantID.String()
		acknowledgement := intent.Acknowledgement.Key.Acknowledgement.String()
		if tenant == "" {
			return claimSettlementShape{}, fmt.Errorf("hand off claim settlement: acknowledgement tenant is required")
		}
		return claimSettlementShape{
			eventID:    tenant + "/acknowledgement/" + acknowledgement,
			eventType:  acknowledgementEventType,
			tenant:     tenant,
			subject:    acknowledgement,
			occurredAt: intent.Acknowledgement.RecordedAt.UTC(),
			payload: claimSettlementPayload{
				TenantID:        tenant,
				Acknowledgement: acknowledgement,
			},
		}, nil
	}
	if intent.Adjustment.Key.Adjustment.String() == "" || intent.Adjustment.Key.TenantID.String() == "" {
		return claimSettlementShape{}, fmt.Errorf("hand off claim settlement: claim record is required")
	}
	tenant := intent.Adjustment.Key.TenantID.String()
	adjustment := intent.Adjustment.Key.Adjustment.String()
	return claimSettlementShape{
		eventID:    tenant + "/claim-adjustment/" + adjustment,
		eventType:  claimAdjustmentEventType,
		tenant:     tenant,
		subject:    adjustment,
		occurredAt: intent.Adjustment.RecordedAt.UTC(),
		payload: claimSettlementPayload{
			TenantID:   tenant,
			Adjustment: adjustment,
		},
	}, nil
}

// HandOffClaimSettlement 把一份意图入队。四类记录各按自身幂等键认领并加类型段；都缺
// 是装配缺陷。入队走 EnqueueOnce。
func (handoff *OutboxClaimSettlementHandoff) HandOffClaimSettlement(
	ctx context.Context,
	intent ports.ClaimSettlementIntent,
) error {
	shape, err := claimSettlementIdentity(intent)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(shape.payload)
	if err != nil {
		return fmt.Errorf("hand off claim settlement: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(shape.eventID),
		Source:       saEventSource,
		Type:         shape.eventType,
		Version:      1,
		Scope:        shape.tenant,
		Subject:      shape.subject,
		PartitionKey: shape.eventID,
		OccurredAt:   shape.occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off claim settlement: %w", err)
	}
	return nil
}
