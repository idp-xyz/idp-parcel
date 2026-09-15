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

// restrictionEventType 是监管限制意图的事件类型。
const restrictionEventType = "customs-compliance.regulatory-restriction.changed"

// OutboxRestrictionHandoff 把限制的建立与解除写入 Outbox，实现
// ports.RestrictionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxRestrictionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxRestrictionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxRestrictionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxRestrictionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.RestrictionHandoff = (*OutboxRestrictionHandoff)(nil)

// restrictionPayload 是意图载荷的传输形状：只有下游 FindByID 所需的租户与限制标识，
// 不带约束动作或解除依据。
type restrictionPayload struct {
	TenantID      string `json:"tenantId"`
	RestrictionID string `json:"restrictionId"`
}

// restrictionEventIDPort 是本口在信封 ID 上的口名前缀。
const restrictionEventIDPort = "restriction"

// restrictionEventID 把限制标识折成 outboxintent.FingerprintEventID 的定长形（票 sa-cc/34 裁决 3）。它不是串接、只有
// 一维，但单一引用的长度同样归实例半边、本仓给不出上界。维度照今天的 ID 只取限制标识、不加租户——ID 认领哪一份
// 意图是 ADR-0043 的题，加维等于换认领口径，归另一次裁决。
func restrictionEventID(restrictionID string) eventing.EventID {
	return outboxintent.FingerprintEventID(restrictionEventIDPort, restrictionID)
}

// HandOffRestriction 把一份意图入队。信封 ID 由限制标识认领（ADR-0043）、折成定长指纹形。
// 租户或标识空白是装配缺陷，响亮报错不入队。
func (handoff *OutboxRestrictionHandoff) HandOffRestriction(
	ctx context.Context,
	intent ports.RestrictionHandoffIntent,
) error {
	if intent.TenantID.String() == "" || intent.Restriction.ID().String() == "" {
		return fmt.Errorf("hand off restriction: tenant and restriction id are required")
	}

	payload, err := json.Marshal(restrictionPayload{
		TenantID:      intent.TenantID.String(),
		RestrictionID: intent.Restriction.ID().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off restriction: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           restrictionEventID(intent.Restriction.ID().String()),
		Source:       ccEventSource,
		Type:         restrictionEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Restriction.ID().String(),
		PartitionKey: intent.TenantID.String() + "/" + intent.Restriction.Scope().String(),
		OccurredAt:   intent.Restriction.EffectiveAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off restriction: %w", err)
	}
	return nil
}
