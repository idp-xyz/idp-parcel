package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 核销与它的撤销是同一笔核销的两拍，各认领各的信封、各有各的事件类型。状态段照
// statement_handoff 的 /voided 现成形状：首拍裸键，撤销拍带后缀。
const (
	settlementApplicationEventType         = "settlement-accounting.settlement-application.applied"
	settlementApplicationReversedEventType = "settlement-accounting.settlement-application.reversed"
)

// OutboxSettlementApplicationHandoff 把核销写入 Outbox，实现
// ports.SettlementApplicationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxSettlementApplicationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSettlementApplicationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSettlementApplicationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxSettlementApplicationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SettlementApplicationHandoff = (*OutboxSettlementApplicationHandoff)(nil)

type settlementApplicationPayload struct {
	TenantID    string `json:"tenantId"`
	Application string `json:"application"`
}

// settlementApplicationPartitionKey 取（租户+核销），不取状态段。
//
// ID 管幂等、分区键管顺序，两者不是一回事。核销与撤销是同一笔核销的先后两拍，状态段
// 进分区键两拍就各自成区，撤销可能先于它撤销的那笔送达——下游据核销形成的判断从此
// 没有先后可言。
func settlementApplicationPartitionKey(key ports.SettlementApplicationKey) string {
	return key.TenantID.String() + "/application/" + key.Application.String()
}

// HandOffSettlementApplication 把一份意图入队。信封 ID 取（租户+核销）并按状态加
// /reversed 段——意图由「核销的这一拍」认领（ADR-0043）：撤销走的是同一个核销键
// （MapExternalFundsHandler.Reverse），ID 少了状态段两拍就算出同一个字符串，而
// outboxintent.EnqueueOnce 先查后插——撤销静默不入队，编排却收到「交接成功」。
// 键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxSettlementApplicationHandoff) HandOffSettlementApplication(
	ctx context.Context,
	intent ports.SettlementApplicationIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Application.String() == "" {
		return fmt.Errorf("hand off settlement application: application key is required")
	}

	payload, err := json.Marshal(settlementApplicationPayload{
		TenantID:    key.TenantID.String(),
		Application: key.Application.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off settlement application: %w", err)
	}

	now := handoff.clock.Now().UTC()
	partitionKey := settlementApplicationPartitionKey(key)
	eventID := partitionKey
	eventType := eventing.EventType(settlementApplicationEventType)
	occurredAt := intent.Record.RecordedAt.UTC()
	if _, reversedAt, reversed := intent.Record.Application.Reversed(); reversed {
		eventID += "/reversed"
		eventType = settlementApplicationReversedEventType
		occurredAt = reversedAt.UTC()
	}
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       saEventSource,
		Type:         eventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Application.String(),
		PartitionKey: partitionKey,
		OccurredAt:   occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off settlement application: %w", err)
	}
	return nil
}
