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

// parcelCancellationEventType 是包裹取消决定意图的事件类型。
const parcelCancellationEventType = "parcel-shipment.parcel-cancellation.recorded"

// OutboxParcelCancellationHandoff 把取消决定写入 Outbox，实现
// ports.ParcelCancellationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxParcelCancellationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxParcelCancellationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxParcelCancellationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxParcelCancellationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ParcelCancellationHandoff = (*OutboxParcelCancellationHandoff)(nil)

// parcelCancellationPayload 是意图载荷的传输形状：只有下游 FindByKey 所需的取消键
// （含租户），不带拒绝依据或收寄版本。
type parcelCancellationPayload struct {
	TenantID   string `json:"tenantId"`
	RequestKey string `json:"requestKey"`
	Parcel     string `json:"parcel"`
}

func parcelCancellationEventID(key ports.CancellationRequestKey) string {
	return key.TenantID.String() + "/" + key.RequestKey.String() + "/" + key.Parcel.String()
}

// HandOffParcelCancellation 把一份意图入队。信封 ID 取取消键（含租户）——意图由
// （租户+请求身份+包裹）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxParcelCancellationHandoff) HandOffParcelCancellation(
	ctx context.Context,
	intent ports.ParcelCancellationHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.RequestKey.String() == "" || key.Parcel.String() == "" {
		return fmt.Errorf("hand off parcel cancellation: cancellation key is required")
	}

	payload, err := json.Marshal(parcelCancellationPayload{
		TenantID:   key.TenantID.String(),
		RequestKey: key.RequestKey.String(),
		Parcel:     key.Parcel.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off parcel cancellation: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := parcelCancellationEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       eventSource,
		Type:         parcelCancellationEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Parcel.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.DecidedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off parcel cancellation: %w", err)
	}
	return nil
}
