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

const transportCommissionEventType = "transport-fulfillment.transport-commission.submitted"

// OutboxTransportCommissionHandoff 把已提交的运输委托写入 Outbox，实现
// ports.TransportCommissionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxTransportCommissionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxTransportCommissionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxTransportCommissionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxTransportCommissionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.TransportCommissionHandoff = (*OutboxTransportCommissionHandoff)(nil)

type transportCommissionPayload struct {
	TenantID   string `json:"tenantId"`
	Commission string `json:"commission"`
}

// transportCommissionPort 是本口在信封 ID 上的口名前缀，与同一来源名下别的口的 ID 空间错开（FingerprintEventID 头注）。
const transportCommissionPort = "transport-commission"

// transportCommissionEventID 按委托幂等键（租户+委托）铸 ID：意图由它认领（ADR-0043），一份委托只发提交一拍。租户与
// 委托标识多长归实例半边，原样串接会把信封上限押在别人的长度上，所以取指纹。
func transportCommissionEventID(key ports.TransportCommissionKey) eventing.EventID {
	return outboxintent.FingerprintEventID(transportCommissionPort, key.TenantID.String(), key.Commission.String())
}

// transportCommissionPartitionKey 保留可读的「租户/委托/口名」：一份委托只走本口一拍，分区仍是逐信封，取可读形只为
// 运维从分区键认得出是哪份委托。
func transportCommissionPartitionKey(key ports.TransportCommissionKey) string {
	return key.TenantID.String() + "/" + key.Commission.String() + "/" + transportCommissionPort
}

// HandOffTransportCommission 把一份意图入队。信封 ID 由委托幂等键认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxTransportCommissionHandoff) HandOffTransportCommission(
	ctx context.Context,
	intent ports.TransportCommissionIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Commission.String() == "" {
		return fmt.Errorf("hand off transport commission: commission key is required")
	}

	payload, err := json.Marshal(transportCommissionPayload{
		TenantID:   key.TenantID.String(),
		Commission: key.Commission.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off transport commission: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           transportCommissionEventID(key),
		Source:       tfEventSource,
		Type:         transportCommissionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Commission.String(),
		PartitionKey: transportCommissionPartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off transport commission: %w", err)
	}
	return nil
}
