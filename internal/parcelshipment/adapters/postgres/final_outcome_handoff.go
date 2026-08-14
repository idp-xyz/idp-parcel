package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

const finalOutcomeEventType = "parcel-shipment.final-outcome.formed"

// OutboxFinalOutcomeHandoff 把已提交的终局判断写入 Outbox，实现
// ports.FinalOutcomeHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxFinalOutcomeHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxFinalOutcomeHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxFinalOutcomeHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxFinalOutcomeHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.FinalOutcomeHandoff = (*OutboxFinalOutcomeHandoff)(nil)

type finalOutcomePayload struct {
	TenantID string `json:"tenantId"`
	Parcel   string `json:"parcel"`
	Kind     string `json:"kind"`
	Version  string `json:"version"`
}

func finalOutcomeEventID(key ports.FinalAdoptionKey) string {
	return key.TenantID.String() + "/final-outcome/" + key.Parcel.String() + "/" +
		key.Kind.String() + "/" + key.Version.String()
}

// HandOffFinalOutcome 把一份意图入队。信封 ID 取采用键再加类型段——意图由（租户+包裹+
// 责任结果种类+版本）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxFinalOutcomeHandoff) HandOffFinalOutcome(
	ctx context.Context,
	intent ports.FinalOutcomeHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Parcel.String() == "" ||
		key.Kind.String() == "" || key.Version.String() == "" {
		return fmt.Errorf("hand off final outcome: adoption key is required")
	}

	payload, err := json.Marshal(finalOutcomePayload{
		TenantID: key.TenantID.String(),
		Parcel:   key.Parcel.String(),
		Kind:     key.Kind.String(),
		Version:  key.Version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off final outcome: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := finalOutcomeEventID(key)
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(eventID),
		Source:      eventSource,
		Type:        finalOutcomeEventType,
		Version:     1,
		Scope:       key.TenantID.String(),
		Subject:     key.Parcel.String(),
		// 分区按租户加包裹排队，不跟着信封 ID 走：ID 含版本（重派生翻旧插新各占一个 ID，
		// 因而不丢），而顺序要的是同一包裹的先后拍在一条队里——两者跟同一个字符串时，
		// 重派生会落进另一个分区，先于原终局送达时下游最后应用的是已被取代的那一份。
		PartitionKey: key.TenantID.String() + "/" + key.Parcel.String(),
		OccurredAt:   intent.Record.AdoptedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off final outcome: %w", err)
	}
	return nil
}
