package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// liabilityEventType 是责任结论意图的事件类型。
const liabilityEventType = "visibility-exception.claim-liability.concluded"

// OutboxLiabilityHandoff 把责任结论写入 Outbox，实现 ports.LiabilityHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxLiabilityHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxLiabilityHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxLiabilityHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxLiabilityHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.LiabilityHandoff = (*OutboxLiabilityHandoff)(nil)

// liabilityPayload 是意图载荷的传输形状：只有下游 FindByBatchItem 所需的租户/批次/
// 项，不带资格依据或结论明细。
type liabilityPayload struct {
	TenantID string `json:"tenantId"`
	Batch    string `json:"batch"`
	ItemID   string `json:"itemId"`
}

// HandOffLiability 把一份意图入队。信封 ID 取索赔项标识——意图由索赔项认领
// （ADR-0043）。租户空白或索赔缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxLiabilityHandoff) HandOffLiability(
	ctx context.Context,
	intent ports.LiabilityHandoffIntent,
) error {
	if intent.Claim == nil || intent.TenantID.String() == "" {
		return fmt.Errorf("hand off liability: tenant and claim are required")
	}

	payload, err := json.Marshal(liabilityPayload{
		TenantID: intent.TenantID.String(),
		Batch:    intent.Claim.Batch().String(),
		ItemID:   intent.Claim.ID().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off liability: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.Claim.ID().String()),
		Source:       veEventSource,
		Type:         liabilityEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Claim.ID().String(),
		PartitionKey: intent.TenantID.String() + "/" + intent.Claim.Batch().String(),
		OccurredAt:   intent.Claim.SubmittedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off liability: %w", err)
	}
	return nil
}
