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

// triageEventType 是分诊结论意图的事件类型。
const triageEventType = "visibility-exception.signal-triage.concluded"

// OutboxTriageHandoff 把分诊结论写入 Outbox，实现 ports.TriageHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxTriageHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxTriageHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxTriageHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxTriageHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.TriageHandoff = (*OutboxTriageHandoff)(nil)

// triagePayload 是意图载荷的传输形状：只有下游 FindLatest 所需的租户/包裹/类型，
// 外加发作期标识，不带分诊走向明细。
type triagePayload struct {
	TenantID  string `json:"tenantId"`
	Parcel    string `json:"parcel"`
	Kind      string `json:"kind"`
	EpisodeID string `json:"episodeId"`
}

func triageEventID(tenant, parcel, kind string) string {
	return tenant + "/" + parcel + "/" + kind
}

// HandOffTriage 把一份意图入队。信封 ID 取租户加对象加类型——与 FindLatest 键一致
// （ADR-0003 / ADR-0043）。缺租户维时两租户同包裹+类型会合成一份。租户、对象、类型
// 或结论缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxTriageHandoff) HandOffTriage(
	ctx context.Context,
	intent ports.TriageHandoffIntent,
) error {
	if intent.TenantID.String() == "" ||
		intent.Parcel.String() == "" ||
		intent.Kind.String() == "" ||
		intent.Conclusion.Episode().String() == "" {
		return fmt.Errorf("hand off triage: tenant, parcel, kind and conclusion are required")
	}

	payload, err := json.Marshal(triagePayload{
		TenantID:  intent.TenantID.String(),
		Parcel:    intent.Parcel.String(),
		Kind:      intent.Kind.String(),
		EpisodeID: intent.Conclusion.Episode().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off triage: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := triageEventID(intent.TenantID.String(), intent.Parcel.String(), intent.Kind.String())
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       veEventSource,
		Type:         triageEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Conclusion.Episode().String(),
		PartitionKey: intent.TenantID.String() + "/" + intent.Parcel.String(),
		OccurredAt:   intent.Conclusion.TriagedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off triage: %w", err)
	}
	return nil
}
