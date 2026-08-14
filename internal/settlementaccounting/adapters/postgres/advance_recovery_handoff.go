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
	advanceRecoveryEventType    eventing.EventType = "settlement-accounting.advance-recovery.formed"
	recoveryAdjustmentEventType eventing.EventType = "settlement-accounting.recovery-adjustment.formed"
)

// OutboxAdvanceRecoveryHandoff 把代垫回收或回收调整写入 Outbox，实现
// ports.AdvanceRecoveryHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxAdvanceRecoveryHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxAdvanceRecoveryHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxAdvanceRecoveryHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxAdvanceRecoveryHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.AdvanceRecoveryHandoff = (*OutboxAdvanceRecoveryHandoff)(nil)

type advanceRecoveryPayload struct {
	TenantID   string `json:"tenantId"`
	Recovery   string `json:"recovery,omitempty"`
	Adjustment string `json:"adjustment,omitempty"`
}

type advanceRecoveryShape struct {
	eventID    string
	eventType  eventing.EventType
	tenant     string
	subject    string
	occurredAt time.Time
	payload    advanceRecoveryPayload
}

func advanceRecoveryIdentity(intent ports.AdvanceRecoveryIntent) (advanceRecoveryShape, error) {
	if intent.Adjustment.Key.Adjustment.String() != "" {
		tenant := intent.Adjustment.Key.TenantID.String()
		adjustment := intent.Adjustment.Key.Adjustment.String()
		if tenant == "" {
			return advanceRecoveryShape{}, fmt.Errorf("hand off advance recovery: adjustment tenant is required")
		}
		return advanceRecoveryShape{
			eventID:    tenant + "/adjustment/" + adjustment,
			eventType:  recoveryAdjustmentEventType,
			tenant:     tenant,
			subject:    adjustment,
			occurredAt: intent.Adjustment.RecordedAt.UTC(),
			payload: advanceRecoveryPayload{
				TenantID:   tenant,
				Adjustment: adjustment,
			},
		}, nil
	}
	if intent.Recovery.Key.Recovery.String() == "" || intent.Recovery.Key.TenantID.String() == "" {
		return advanceRecoveryShape{}, fmt.Errorf("hand off advance recovery: recovery or adjustment is required")
	}
	tenant := intent.Recovery.Key.TenantID.String()
	recovery := intent.Recovery.Key.Recovery.String()
	return advanceRecoveryShape{
		eventID:    tenant + "/recovery/" + recovery,
		eventType:  advanceRecoveryEventType,
		tenant:     tenant,
		subject:    recovery,
		occurredAt: intent.Recovery.RecordedAt.UTC(),
		payload: advanceRecoveryPayload{
			TenantID: tenant,
			Recovery: recovery,
		},
	}, nil
}

// HandOffAdvanceRecovery 把一份意图入队。回收由回收标识认领，调整由调整标识认领；两半
// 都缺是装配缺陷。入队走 EnqueueOnce。
func (handoff *OutboxAdvanceRecoveryHandoff) HandOffAdvanceRecovery(
	ctx context.Context,
	intent ports.AdvanceRecoveryIntent,
) error {
	shape, err := advanceRecoveryIdentity(intent)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(shape.payload)
	if err != nil {
		return fmt.Errorf("hand off advance recovery: %w", err)
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
		return fmt.Errorf("hand off advance recovery: %w", err)
	}
	return nil
}
