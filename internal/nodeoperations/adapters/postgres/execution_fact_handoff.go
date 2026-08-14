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

// executionFactEventType 是执行事实意图的事件类型。
const executionFactEventType = "node-operations.execution-fact.recorded"

// OutboxExecutionFactHandoff 把执行事实写入 Outbox，实现
// ports.ExecutionFactHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxExecutionFactHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxExecutionFactHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxExecutionFactHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("node operations postgres: clock is nil")
	}
	return &OutboxExecutionFactHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ExecutionFactHandoff = (*OutboxExecutionFactHandoff)(nil)

// executionFactPayload 是意图载荷的传输形状：只有下游 FindByKey 所需的执行事实
// 幂等键，不带证据快照。
type executionFactPayload struct {
	TenantID string `json:"tenantId"`
	ItemRef  string `json:"itemRef"`
	UnitID   string `json:"unitId"`
	Action   string `json:"action"`
}

func executionFactEventID(key ports.ExecutionFactKey) string {
	return key.TenantID.String() + "/" + key.Item.String() + "/" + key.Unit.String() + "/" + key.Action.String()
}

// HandOffExecutionFact 把一份意图入队。信封 ID 取执行事实幂等键——意图由
// （租户+事项+实物+动作）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxExecutionFactHandoff) HandOffExecutionFact(
	ctx context.Context,
	intent ports.ExecutionFactHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" ||
		key.Item.String() == "" ||
		key.Unit.String() == "" ||
		key.Action.String() == "" {
		return fmt.Errorf("hand off execution fact: execution fact key is required")
	}

	payload, err := json.Marshal(executionFactPayload{
		TenantID: key.TenantID.String(),
		ItemRef:  key.Item.String(),
		UnitID:   key.Unit.String(),
		Action:   key.Action.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off execution fact: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := executionFactEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       noEventSource,
		Type:         executionFactEventType,
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
		return fmt.Errorf("hand off execution fact: %w", err)
	}
	return nil
}
