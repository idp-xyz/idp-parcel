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

const carrierFirstEffectivePickupEventType = "transport-fulfillment.carrier-first-effective-pickup.registered"

// OutboxCarrierFirstEffectivePickupHandoff 把一版已形成 / 替代 / 失效的收寄写入 Outbox，实现
// ports.CarrierFirstEffectivePickupHandoff（ADR-0135 决定七）。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCarrierFirstEffectivePickupHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCarrierFirstEffectivePickupHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCarrierFirstEffectivePickupHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxCarrierFirstEffectivePickupHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CarrierFirstEffectivePickupHandoff = (*OutboxCarrierFirstEffectivePickupHandoff)(nil)

// carrierFirstEffectivePickupPayload 是指针式四维——恰是 lc/25「前提」列的四件，一件不多：本体由下游按
// （租户，事实，版本）读回指名那一代，不塞快照。
type carrierFirstEffectivePickupPayload struct {
	TenantID string `json:"tenantId"`
	Fact     string `json:"fact"`
	Version  string `json:"version"`
	Object   string `json:"object"`
}

// carrierFirstEffectivePickupEventID 取（租户 + 事实 + 版本 + 类型段）。版本必须在 ID 里：一条链的替代与失效版本
// 走同一个事实身份，少了版本两代算出同一个字符串，第二份被 EnqueueOnce 静默吞掉（effectiveDeliveryEventID 的教训）。
func carrierFirstEffectivePickupEventID(key ports.CarrierFirstEffectivePickupKey) string {
	return key.TenantID.String() + "/" + key.Fact.String() + "/" + key.Version.String() + "/carrier-first-effective-pickup"
}

// carrierFirstEffectivePickupPartitionKey 取（租户 + 载运对象 + 类型段），不取整个键：ID 管幂等、分区键管顺序。一个
// 对象的收寄链在口内保序——失效先于首登送达，PS 的终局就会落在已被撤回的那一版上。类型段按 ADR-0074，理由同
// effectiveDeliveryPartitionKey：TF 的排队主体是载运对象，与 visibility-exception 的（租户+包裹）分区不共队。
func carrierFirstEffectivePickupPartitionKey(tenant domain.TenantID, object domain.CarriedObjectReference) string {
	return tenant.String() + "/" + object.String() + "/carrier-first-effective-pickup"
}

// HandOffCarrierFirstEffectivePickup 把一份意图入队。**待确认版本响亮拒绝**：它不构成收寄、不提供给 parcel-shipment
// （CONTEXT 词条），到这里说明编排装配错了，不是一格业务答案。键缺席同样是装配缺陷。
func (handoff *OutboxCarrierFirstEffectivePickupHandoff) HandOffCarrierFirstEffectivePickup(
	ctx context.Context,
	intent ports.CarrierFirstEffectivePickupHandoffIntent,
) error {
	key := intent.Record.Key
	pickup := intent.Record.Pickup
	if key.TenantID.String() == "" || key.Fact.String() == "" || key.Version.String() == "" || pickup.Object().String() == "" {
		return fmt.Errorf("hand off carrier first effective pickup: pickup key is required")
	}
	if pickup.Result() == domain.CarrierPickupPending {
		return fmt.Errorf("hand off carrier first effective pickup: a pending version does not constitute a pickup and is not handed off")
	}
	payload, err := json.Marshal(carrierFirstEffectivePickupPayload{
		TenantID: key.TenantID.String(),
		Fact:     key.Fact.String(),
		Version:  key.Version.String(),
		Object:   pickup.Object().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off carrier first effective pickup: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(carrierFirstEffectivePickupEventID(key)),
		Source:       tfEventSource,
		Type:         carrierFirstEffectivePickupEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      pickup.Object().String() + "/" + key.Fact.String(),
		PartitionKey: carrierFirstEffectivePickupPartitionKey(key.TenantID, pickup.Object()),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off carrier first effective pickup: %w", err)
	}
	return nil
}
