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

const dispositionExecutionEventType = "transport-fulfillment.disposition-execution.recorded"

// OutboxDispositionExecutionHandoff 把监管来路的替代旅程写入 Outbox，实现
// ports.DispositionExecutionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxDispositionExecutionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDispositionExecutionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxDispositionExecutionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxDispositionExecutionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.DispositionExecutionHandoff = (*OutboxDispositionExecutionHandoff)(nil)

type alternateJourneyPayload struct {
	TenantID string `json:"tenantId"`
	Original string `json:"original"`
	Purpose  string `json:"purpose"`
	Basis    string `json:"basis"`
}

func alternateJourneyKeyParts(key ports.AlternateJourneyKey) string {
	return key.TenantID.String() + "/" + key.Original.String() + "/" +
		key.Purpose.String() + "/" + key.Basis.String()
}

func dispositionExecutionEventID(key ports.AlternateJourneyKey) string {
	// 类型段把本口与同键的 ExceptionJourneyHandoff 错开——监管来路两口同事务入队。
	return alternateJourneyKeyParts(key) + "/disposition-execution"
}

func alternateJourneyKeyMissing(key ports.AlternateJourneyKey) bool {
	return key.TenantID.String() == "" ||
		key.Original.String() == "" ||
		key.Purpose.String() == "" ||
		key.Basis.String() == ""
}

// HandOffDispositionExecution 把一份意图入队。信封 ID 取替代旅程幂等键再加类型段——
// 意图由（租户+原旅程+目的+处置依据）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxDispositionExecutionHandoff) HandOffDispositionExecution(
	ctx context.Context,
	intent ports.AlternateJourneyIntent,
) error {
	key := intent.Record.Key
	if alternateJourneyKeyMissing(key) {
		return fmt.Errorf("hand off disposition execution: journey key is required")
	}

	payload, err := json.Marshal(alternateJourneyPayload{
		TenantID: key.TenantID.String(),
		Original: key.Original.String(),
		Purpose:  key.Purpose.String(),
		Basis:    key.Basis.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off disposition execution: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := dispositionExecutionEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         dispositionExecutionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Original.String() + "/" + key.Purpose.String() + "/" + key.Basis.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off disposition execution: %w", err)
	}
	return nil
}
