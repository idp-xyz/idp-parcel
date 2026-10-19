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

// initialRouteEventIDPort 是本口指纹 ID 的口名前缀。
const initialRouteEventIDPort = "initial-route"

// initialRouteEventID 把判断键六维折成 outboxintent.FingerprintEventID 的定长形，理由见那份头注：各维多长归实例半边，
// 原样串接就把信封 ID 上限押在别人的长度上。越过上限的入队被确定性拒收，而交接失败只化成一条续办引用，重发同一份
// 照样被拒。
func initialRouteEventID(key domain.InitialRouteJudgmentKey) eventing.EventID {
	return outboxintent.FingerprintEventID(initialRouteEventIDPort,
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.DeclaredParcelID.String(), key.AcceptanceBaseline.String(), key.ServicePurpose.String())
}

// initialRoutePartitionKey 是换成指纹 ID 之前兼作分区键的那一串，一字不变：分区键是顺序语义，改它会让换形前后
// 同一判断键落进两个分区（先例 customsCasePartitionKey）。它仍随各维长度涨，上限是 eventing.MaxPartitionKeyLength；
// 越过时框架照样拒收，在本口同样只化成续办引用。
func initialRoutePartitionKey(key domain.InitialRouteJudgmentKey) string {
	return key.TenantID.String() + "/initial-route/" + key.ShipmentRequestID.String() + "/" +
		key.DeclaredParcelID.String() + "/" + key.AcceptanceBaseline.String() + "/" +
		key.ServicePurpose.String()
}

// HandOffInitialRoute 把一份意图入队。信封 ID 是判断键的指纹加口名前缀——意图由判断键认领
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
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           initialRouteEventID(key),
		Source:       nrEventSource,
		Type:         initialRouteEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.DeclaredParcelID.String(),
		PartitionKey: initialRoutePartitionKey(key),
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
