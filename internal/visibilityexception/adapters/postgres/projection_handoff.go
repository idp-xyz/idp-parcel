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

// projectionEventType 是追踪投影版本意图的事件类型。
const projectionEventType = "visibility-exception.tracking-projection.derived"

// OutboxProjectionHandoff 把投影版本写入 Outbox，实现 ports.ProjectionHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxProjectionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxProjectionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxProjectionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxProjectionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ProjectionHandoff = (*OutboxProjectionHandoff)(nil)

// projectionPayload 是意图载荷的传输形状：只有下游 FindCurrent 所需的租户与包裹，
// 外加版本标识认领，不带里程碑条目。
type projectionPayload struct {
	TenantID  string `json:"tenantId"`
	Parcel    string `json:"parcel"`
	VersionID string `json:"versionId"`
}

// HandOffProjection 把一份意图入队。信封 ID 取投影版本——意图由投影版本认领
// （ADR-0043）。租户或版本空白是装配缺陷，响亮报错不入队。
func (handoff *OutboxProjectionHandoff) HandOffProjection(
	ctx context.Context,
	intent ports.ProjectionHandoffIntent,
) error {
	if intent.TenantID.String() == "" || intent.Projection.Version().String() == "" {
		return fmt.Errorf("hand off projection: tenant and projection version are required")
	}

	payload, err := json.Marshal(projectionPayload{
		TenantID:  intent.TenantID.String(),
		Parcel:    intent.Projection.Parcel().String(),
		VersionID: intent.Projection.Version().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off projection: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.Projection.Version().String()),
		Source:       veEventSource,
		Type:         projectionEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Projection.Version().String(),
		PartitionKey: intent.TenantID.String() + "/" + intent.Projection.Parcel().String(),
		OccurredAt:   intent.Projection.DerivedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off projection: %w", err)
	}
	return nil
}
