package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

const effectiveDeliveryEventType = "transport-fulfillment.effective-delivery.registered"

// OutboxEffectiveDeliveryHandoff 把交付生效写入 Outbox，实现
// ports.EffectiveDeliveryHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxEffectiveDeliveryHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxEffectiveDeliveryHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxEffectiveDeliveryHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxEffectiveDeliveryHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.EffectiveDeliveryHandoff = (*OutboxEffectiveDeliveryHandoff)(nil)

type effectiveDeliveryPayload struct {
	TenantID string `json:"tenantId"`
	Object   string `json:"object"`
	Attempt  string `json:"attempt"`
}

// effectiveDeliveryEventID 取交付生效键**再加结果版本**。
//
// 版本必须在里面。ADR-0043 说意图由结果标识认领，而一次交付生效的结果标识是它的版本
// 不是它的键：POD 更正换出新版本走的是同一个键，ID 少了版本两代就算出同一个字符串，
// 而 outboxintent.EnqueueOnce 先查后插——第二份于是静默不入队，编排却收到「交接成功」。
func effectiveDeliveryEventID(key ports.EffectiveDeliveryKey, version domain.DeliveryResultVersion) string {
	return key.TenantID.String() + "/" + key.Object.String() + "/" + key.Attempt.String() +
		"/" + version.String() + "/effective-delivery"
}

// effectiveDeliveryPartitionKey 取（租户+载运对象），不取整个键。
//
// 与交接登记同一条理由：ID 管幂等、分区键管顺序，两者不是一回事。一个对象的交付结果
// 是一条链（首登，此后每次 POD 更正一版），下游 parcel-shipment 据它形成终局判断——
// 更正先于首登送达，终局就会落在已被取代的那一版上。
func effectiveDeliveryPartitionKey(key ports.EffectiveDeliveryKey) string {
	return key.TenantID.String() + "/" + key.Object.String()
}

// HandOffEffectiveDelivery 把一份意图入队。载荷仍只带键：下游按键读当前版本，这是有意的
// 指针式意图。版本进 ID 而不进载荷——ID 要区分两代好让两份都入队，载荷要的是「去重读」
// 而不是「这是第几版」。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxEffectiveDeliveryHandoff) HandOffEffectiveDelivery(
	ctx context.Context,
	intent ports.EffectiveDeliveryHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Object.String() == "" || key.Attempt.String() == "" {
		return fmt.Errorf("hand off effective delivery: delivery key is required")
	}
	version := intent.Record.Delivery.Version()
	if version.String() == "" {
		// 没有版本就分不出首登与更正，两代会算出同一个 ID 而第二份被静默吞掉。
		return fmt.Errorf("hand off effective delivery: delivery result version is required")
	}

	payload, err := json.Marshal(effectiveDeliveryPayload{
		TenantID: key.TenantID.String(),
		Object:   key.Object.String(),
		Attempt:  key.Attempt.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off effective delivery: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := effectiveDeliveryEventID(key, version)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         effectiveDeliveryEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Object.String() + "/" + key.Attempt.String(),
		PartitionKey: effectiveDeliveryPartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off effective delivery: %w", err)
	}
	return nil
}
