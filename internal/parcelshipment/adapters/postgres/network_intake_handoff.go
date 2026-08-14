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

// networkIntakeEventType 是网络收寄采用结果意图的事件类型。
const networkIntakeEventType = "parcel-shipment.network-intake.recorded"

// OutboxNetworkIntakeHandoff 把采用结果写入 Outbox，实现
// ports.NetworkIntakeHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxNetworkIntakeHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxNetworkIntakeHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxNetworkIntakeHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxNetworkIntakeHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.NetworkIntakeHandoff = (*OutboxNetworkIntakeHandoff)(nil)

// networkIntakePayload 是意图载荷的传输形状：只有下游 FindByKey 所需的采用键（含租户），
// 不带收寄明细或承诺。
type networkIntakePayload struct {
	TenantID string `json:"tenantId"`
	Parcel   string `json:"parcel"`
	Kind     string `json:"kind"`
	Version  string `json:"version"`
}

func networkIntakeEventID(key ports.IntakeAdoptionKey) string {
	// 类型段把本口与同 source 下其它采用键形状的口（如终局）错开。
	return key.TenantID.String() + "/" + key.Parcel.String() + "/" + key.Kind.String() + "/" +
		key.Version.String() + "/network-intake"
}

// HandOffNetworkIntake 把一份意图入队。信封 ID 取采用键（含租户）再加类型段——意图由
// （租户+包裹+来源类型+来源版本）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxNetworkIntakeHandoff) HandOffNetworkIntake(
	ctx context.Context,
	intent ports.NetworkIntakeHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" ||
		key.Parcel.String() == "" ||
		key.Kind.String() == "" ||
		key.Version.String() == "" {
		return fmt.Errorf("hand off network intake: adoption key is required")
	}

	payload, err := json.Marshal(networkIntakePayload{
		TenantID: key.TenantID.String(),
		Parcel:   key.Parcel.String(),
		Kind:     key.Kind.String(),
		Version:  key.Version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off network intake: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := networkIntakeEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       eventSource,
		Type:         networkIntakeEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Parcel.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.AdoptedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off network intake: %w", err)
	}
	return nil
}
