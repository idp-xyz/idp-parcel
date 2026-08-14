package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// visibilityGapEventType 是可见性缺口意图的事件类型。
const visibilityGapEventType = "visibility-exception.visibility-gap.formed"

// OutboxVisibilityGapHandoff 把已成立的缺口写入 Outbox，实现
// ports.VisibilityGapHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxVisibilityGapHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxVisibilityGapHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxVisibilityGapHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxVisibilityGapHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.VisibilityGapHandoff = (*OutboxVisibilityGapHandoff)(nil)

// visibilityGapPayload 是意图载荷的传输形状：只有下游 FindCurrent 所需的租户与缺口
// 身份三维，不带窗口截止或形成时刻。
type visibilityGapPayload struct {
	TenantID    string `json:"tenantId"`
	Parcel      string `json:"parcel"`
	Expectation string `json:"expectation"`
	WindowRule  string `json:"windowRule"`
}

func visibilityGapEventID(gap domain.VisibilityGap) string {
	return gap.Parcel().String() + "/" + gap.Expectation().String() + "/" + gap.WindowRule().String()
}

// HandOffVisibilityGap 把一份意图入队。信封 ID 取缺口身份三维——意图由三维认领
// （ADR-0043）。租户或三维空白是装配缺陷，响亮报错不入队。
func (handoff *OutboxVisibilityGapHandoff) HandOffVisibilityGap(
	ctx context.Context,
	intent ports.VisibilityGapHandoffIntent,
) error {
	if intent.TenantID.String() == "" ||
		intent.Gap.Parcel().String() == "" ||
		intent.Gap.Expectation().String() == "" ||
		intent.Gap.WindowRule().String() == "" {
		return fmt.Errorf("hand off visibility gap: tenant and gap identity are required")
	}

	payload, err := json.Marshal(visibilityGapPayload{
		TenantID:    intent.TenantID.String(),
		Parcel:      intent.Gap.Parcel().String(),
		Expectation: intent.Gap.Expectation().String(),
		WindowRule:  intent.Gap.WindowRule().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off visibility gap: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := visibilityGapEventID(intent.Gap)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       veEventSource,
		Type:         visibilityGapEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      eventID,
		PartitionKey: intent.TenantID.String() + "/" + intent.Gap.Parcel().String(),
		OccurredAt:   intent.Gap.FormedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off visibility gap: %w", err)
	}
	return nil
}
