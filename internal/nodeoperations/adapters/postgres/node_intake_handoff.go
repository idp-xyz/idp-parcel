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

// noEventSource 是本上下文在信封 Source 位上的稳定名。
const noEventSource = "idp-parcel/node-operations"

// nodeIntakeEventType 是节点收寄判断意图的事件类型。
const nodeIntakeEventType = "node-operations.node-intake.formed"

// OutboxNodeIntakeHandoff 把已提交的收寄判断写入 Outbox，实现
// ports.NodeIntakeHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxNodeIntakeHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxNodeIntakeHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxNodeIntakeHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("node operations postgres: clock is nil")
	}
	return &OutboxNodeIntakeHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.NodeIntakeHandoff = (*OutboxNodeIntakeHandoff)(nil)

// nodeIntakePayload 是意图载荷的传输形状：只有下游 FindByKey 所需的收寄幂等键，不带
// 收寄内容或控制明细。
type nodeIntakePayload struct {
	TenantID string `json:"tenantId"`
	SourceID string `json:"sourceId"`
}

func nodeIntakeEventID(key ports.ReceptionKey) string {
	return key.TenantID.String() + "/" + key.SourceID
}

// HandOffNodeIntake 把一份意图入队。信封 ID 取收寄幂等键——意图由收寄键认领
// （ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxNodeIntakeHandoff) HandOffNodeIntake(
	ctx context.Context,
	intent ports.NodeIntakeHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.SourceID == "" {
		return fmt.Errorf("hand off node intake: receive key is required")
	}

	payload, err := json.Marshal(nodeIntakePayload{
		TenantID: key.TenantID.String(),
		SourceID: key.SourceID,
	})
	if err != nil {
		return fmt.Errorf("hand off node intake: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := nodeIntakeEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       noEventSource,
		Type:         nodeIntakeEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.SourceID,
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off node intake: %w", err)
	}
	return nil
}
