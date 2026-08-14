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

// manifestEventType 是舱单引用意图的事件类型。
const manifestEventType = "customs-compliance.manifest.recorded"

// OutboxManifestHandoff 把舱单引用写入 Outbox，实现 ports.ManifestHandoff。入队一
// 步由 outboxintent.EnqueueOnce 承担。
type OutboxManifestHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxManifestHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxManifestHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxManifestHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ManifestHandoff = (*OutboxManifestHandoff)(nil)

// manifestPayload 是意图载荷的传输形状：只有下游 FindByManifest 所需的租户与舱单
// 标识，不带关联或来源版本明细。
type manifestPayload struct {
	TenantID   string `json:"tenantId"`
	ManifestID string `json:"manifestId"`
}

func manifestEventID(tenant, manifest string) string {
	return tenant + "/" + manifest
}

// HandOffManifest 把一份意图入队。信封 ID 取舱单身份（租户加舱单标识）——意图由舱
// 单身份认领（ADR-0043）。租户或舱单标识空白是装配缺陷，响亮报错不入队。
func (handoff *OutboxManifestHandoff) HandOffManifest(
	ctx context.Context,
	intent ports.ManifestHandoffIntent,
) error {
	if intent.TenantID.String() == "" || intent.Reference.Manifest().String() == "" {
		return fmt.Errorf("hand off manifest: tenant and manifest id are required")
	}

	payload, err := json.Marshal(manifestPayload{
		TenantID:   intent.TenantID.String(),
		ManifestID: intent.Reference.Manifest().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off manifest: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := manifestEventID(intent.TenantID.String(), intent.Reference.Manifest().String())
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         manifestEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Reference.Manifest().String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Reference.AcceptedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off manifest: %w", err)
	}
	return nil
}
