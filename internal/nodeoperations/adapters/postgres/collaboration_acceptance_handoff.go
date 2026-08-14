package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// collaborationAcceptanceEventType 是承接决定意图的事件类型。
const collaborationAcceptanceEventType = "node-operations.collaboration-acceptance.decided"

// OutboxCollaborationAcceptanceHandoff 把承接决定写入 Outbox，实现
// ports.CollaborationAcceptanceHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCollaborationAcceptanceHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCollaborationAcceptanceHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCollaborationAcceptanceHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("node operations postgres: clock is nil")
	}
	return &OutboxCollaborationAcceptanceHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CollaborationAcceptanceHandoff = (*OutboxCollaborationAcceptanceHandoff)(nil)

// collaborationAcceptancePayload 是意图载荷的传输形状：只有下游 FindByKey 所需的
// 承接幂等键，不带决定内容或范围明细。
type collaborationAcceptancePayload struct {
	TenantID string `json:"tenantId"`
	ItemRef  string `json:"itemRef"`
}

func collaborationAcceptanceEventID(key ports.CollaborationAcceptanceKey) string {
	return key.TenantID.String() + "/" + key.Item.String()
}

// HandOffCollaborationAcceptance 把一份意图入队。信封 ID 取承接幂等键——意图由
// （租户+事项）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxCollaborationAcceptanceHandoff) HandOffCollaborationAcceptance(
	ctx context.Context,
	intent ports.CollaborationAcceptanceHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Item.String() == "" {
		return fmt.Errorf("hand off collaboration acceptance: acceptance key is required")
	}

	payload, err := json.Marshal(collaborationAcceptancePayload{
		TenantID: key.TenantID.String(),
		ItemRef:  key.Item.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off collaboration acceptance: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := collaborationAcceptanceEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       noEventSource,
		Type:         collaborationAcceptanceEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Item.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off collaboration acceptance: %w", err)
	}
	return nil
}
