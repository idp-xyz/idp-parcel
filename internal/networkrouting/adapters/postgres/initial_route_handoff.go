package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

const initialRouteEventType = "network-routing.initial-route.formed"

// OutboxInitialRouteHandoff 把包裹级初始路由判断写入 Outbox，实现
// ports.InitialRouteHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxInitialRouteHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxInitialRouteHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxInitialRouteHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("network routing postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("network routing postgres: clock is nil")
	}
	return &OutboxInitialRouteHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.InitialRouteHandoff = (*OutboxInitialRouteHandoff)(nil)

// initialRoutePayload 带齐 InitialRouteJudgmentKey 全部六维加交接关联。客户维不可省：
// InitialRouteStore.FindByKey 按含 customer_account_id 的完整主键取回本体，载荷缺任何
// 一维，消费方就永远拼不出能命中的键（信封只带键、本体按键重取的前提）。
type initialRoutePayload struct {
	TenantID          string `json:"tenantId"`
	CustomerAccountID string `json:"customerAccountId"`
	Shipment          string `json:"shipment"`
	Parcel            string `json:"parcel"`
	Baseline          string `json:"baseline"`
	Purpose           string `json:"purpose"`
	Correlation       string `json:"correlation"`
}

func initialRouteEventID(key domain.InitialRouteJudgmentKey) string {
	return key.TenantID.String() + "/initial-route/" + key.ShipmentRequestID.String() + "/" +
		key.DeclaredParcelID.String() + "/" + key.AcceptanceBaseline.String() + "/" +
		key.ServicePurpose.String()
}

// HandOffInitialRoute 把一份意图入队。信封 ID 取判断键再加类型段——意图由判断键认领
// （ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxInitialRouteHandoff) HandOffInitialRoute(
	ctx context.Context,
	intent ports.InitialRouteHandoffIntent,
) error {
	key := intent.Record.Key
	if !key.MinimumIdentityEstablished() {
		return fmt.Errorf("hand off initial route: judgment key is required")
	}

	payload, err := json.Marshal(initialRoutePayload{
		TenantID:          key.TenantID.String(),
		CustomerAccountID: key.CustomerAccountID.String(),
		Shipment:          key.ShipmentRequestID.String(),
		Parcel:            key.DeclaredParcelID.String(),
		Baseline:          key.AcceptanceBaseline.String(),
		Purpose:           key.ServicePurpose.String(),
		Correlation:       intent.Correlation.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off initial route: %w", err)
	}

	occurredAt := handoff.clock.Now().UTC()
	if intent.Record.HasPlan {
		occurredAt = intent.Record.Plan.JudgedAt().UTC()
	} else if intent.Record.HasNoRoute {
		occurredAt = intent.Record.NoRoute.JudgedAt().UTC()
	}

	now := handoff.clock.Now().UTC()
	eventID := initialRouteEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       nrEventSource,
		Type:         initialRouteEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.DeclaredParcelID.String(),
		PartitionKey: eventID,
		OccurredAt:   occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off initial route: %w", err)
	}
	return nil
}
