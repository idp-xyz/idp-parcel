package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

const transportHandoverRegistrationEventType = "transport-fulfillment.transport-handover.registered"

// OutboxTransportHandoverRegistrationHandoff 把交接判断登记写入 Outbox，实现
// ports.TransportHandoverRegistrationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxTransportHandoverRegistrationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxTransportHandoverRegistrationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxTransportHandoverRegistrationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxTransportHandoverRegistrationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.TransportHandoverRegistrationHandoff = (*OutboxTransportHandoverRegistrationHandoff)(nil)

type transportHandoverRegistrationPayload struct {
	TenantID string `json:"tenantId"`
	Object   string `json:"object"`
	Scope    string `json:"scope"`
	Version  string `json:"version"`
}

func transportHandoverRegistrationEventID(key ports.TransportHandoverKey) string {
	return key.TenantID.String() + "/" + key.Object.String() + "/" + key.Scope.String() + "/" + key.Version.String()
}

// transportHandoverPartitionKey 取（租户+载运对象），不取整个判断键。
//
// 分区键与信封 ID 管的不是一回事：ID 管幂等（每个判断版本一份意图，更正因而不丢），
// 分区键管顺序（同一对象的先后拍排队）。把 ID 直接当分区键会让每份信封自成一个分区，
// 框架的顺序保证于是落空——更正版本可以先于它更正的那一版送达。
//
// 主体取到对象而不取到（对象+范围）：控制转移对一个载运对象是一条链，先从节点交出、
// 再由承运方接收，两次交接分属不同范围却必须保序。取到范围就把这条链切成了互不排队的
// 两段，而 node-operations 的控制转移正是按这条链推进的。
func transportHandoverPartitionKey(key ports.TransportHandoverKey) string {
	return key.TenantID.String() + "/" + key.Object.String()
}

// HandOffTransportHandover 把一份意图入队。信封 ID 取交接判断键——意图由
// （租户+对象+范围+版本）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxTransportHandoverRegistrationHandoff) HandOffTransportHandover(
	ctx context.Context,
	intent ports.TransportHandoverRegistrationIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" ||
		key.Object.String() == "" ||
		key.Scope.String() == "" ||
		key.Version.String() == "" {
		return fmt.Errorf("hand off transport handover: handover key is required")
	}

	payload, err := json.Marshal(transportHandoverRegistrationPayload{
		TenantID: key.TenantID.String(),
		Object:   key.Object.String(),
		Scope:    key.Scope.String(),
		Version:  key.Version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off transport handover: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := transportHandoverRegistrationEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         transportHandoverRegistrationEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Object.String() + "/" + key.Scope.String() + "/" + key.Version.String(),
		PartitionKey: transportHandoverPartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off transport handover: %w", err)
	}
	return nil
}
