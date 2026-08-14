package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// customsCaseEventType 是关务案件建立意图的事件类型。
const customsCaseEventType = "customs-compliance.customs-case.established"

// OutboxCustomsCaseHandoff 把案件建立写入 Outbox，实现 ports.CustomsCaseHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCustomsCaseHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCustomsCaseHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCustomsCaseHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxCustomsCaseHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CustomsCaseHandoff = (*OutboxCustomsCaseHandoff)(nil)

// customsCasePayload 是意图载荷的传输形状：只有下游 FindByKey 所需的案件键，不带
// 包裹关联或角色快照。
type customsCasePayload struct {
	TenantID     string `json:"tenantId"`
	Jurisdiction string `json:"jurisdiction"`
	Direction    string `json:"direction"`
	Procedure    string `json:"procedure"`
	Obligation   string `json:"obligation"`
}

func customsCaseEventID(key ports.CustomsCaseKey) string {
	return key.TenantID.String() + "/" + key.Jurisdiction.String() + "/" +
		key.Direction.String() + "/" + key.Procedure.String() + "/" + key.Obligation.String()
}

// HandOffCase 把一份意图入队。信封 ID 取案件键——意图由案件键认领（ADR-0043）。键
// 缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxCustomsCaseHandoff) HandOffCase(
	ctx context.Context,
	intent ports.CustomsCaseHandoffIntent,
) error {
	key := intent.Key
	if key.TenantID.String() == "" ||
		key.Jurisdiction.String() == "" ||
		key.Direction.String() == "" ||
		key.Procedure.String() == "" ||
		key.Obligation.String() == "" {
		return fmt.Errorf("hand off customs case: receive key is required")
	}

	payload, err := json.Marshal(customsCasePayload{
		TenantID:     key.TenantID.String(),
		Jurisdiction: key.Jurisdiction.String(),
		Direction:    key.Direction.String(),
		Procedure:    key.Procedure.String(),
		Obligation:   key.Obligation.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off customs case: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := customsCaseEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         customsCaseEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      intent.Case.ID().String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Case.EstablishedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off customs case: %w", err)
	}
	return nil
}
