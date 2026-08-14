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

// followUpEventType 是后续申报动作意图的事件类型。
const followUpEventType = "customs-compliance.follow-up.recorded"

// OutboxFollowUpHandoff 把后续动作目标写入 Outbox，实现 ports.FollowUpHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxFollowUpHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxFollowUpHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxFollowUpHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxFollowUpHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.FollowUpHandoff = (*OutboxFollowUpHandoff)(nil)

// followUpPayload 是意图载荷的传输形状：只有下游 FindTarget 所需的目标键四维，不带
// 替代关系或范围明细。
type followUpPayload struct {
	TenantID  string `json:"tenantId"`
	Trigger   string `json:"trigger"`
	VersionID string `json:"versionId"`
	Kind      string `json:"kind"`
}

func followUpEventID(key ports.FollowUpTargetKey) string {
	return key.TenantID.String() + "/" + key.Trigger.String() + "/" +
		key.Version.String() + "/" + key.Kind.String()
}

// HandOffFollowUp 把一份意图入队。信封 ID 取后续动作目标键——意图由目标键认领
// （ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxFollowUpHandoff) HandOffFollowUp(
	ctx context.Context,
	intent ports.FollowUpHandoffIntent,
) error {
	key := intent.Key
	if key.TenantID.String() == "" ||
		key.Trigger.String() == "" ||
		key.Version.String() == "" ||
		key.Kind.String() == "" {
		return fmt.Errorf("hand off follow-up: receive key is required")
	}

	payload, err := json.Marshal(followUpPayload{
		TenantID:  key.TenantID.String(),
		Trigger:   key.Trigger.String(),
		VersionID: key.Version.String(),
		Kind:      key.Kind.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off follow-up: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := followUpEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         followUpEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Version.String() + "/" + key.Kind.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Target.FormedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off follow-up: %w", err)
	}
	return nil
}
