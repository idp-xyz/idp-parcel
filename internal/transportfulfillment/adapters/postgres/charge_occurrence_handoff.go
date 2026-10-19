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

// ChargeOccurrenceFormedEventType 是「TF 登记了一个运输收费发生项版本」那封引用式信封的类型（ADR-0207 决定五）。
// 消费方是结算的 BUY 发起（ADR-0208），它按自己的惯例自写同一串、路由表引的是它那一个，生产代码里不 import 本适配器；
// 导出只为 cmd/parcel-dispatch 那条同时装配两边的用例对照两串相等。
const ChargeOccurrenceFormedEventType = "transport-fulfillment.charge-occurrence.formed"

// OutboxChargeOccurrenceHandoff 把已登记的发生项版本写入 Outbox，实现 ports.ChargeOccurrenceHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxChargeOccurrenceHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxChargeOccurrenceHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxChargeOccurrenceHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxChargeOccurrenceHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ChargeOccurrenceHandoff = (*OutboxChargeOccurrenceHandoff)(nil)

// chargeOccurrencePayload 恰是登记册的键三维，键名与消费门认的那三个一字不差（ADR-0208 决定一）。原因、业务时间、
// 协议与成员一样不带：它们只有登记册一处权威，消费方按键回读，信封上抄一份就是第二处。
type chargeOccurrencePayload struct {
	TenantID   string `json:"tenantId"`
	Occurrence string `json:"occurrence"`
	Version    string `json:"version"`
}

// chargeOccurrencePartitionKey 取「租户/运输收费发生项」：同一发生项的首版与日后的失效、替代版本排一条队——更正先于
// 原版到达，结算就会对着一份还没见过的原项追加调整。不取到版本：取到版本就是逐信封分区，保不了这个序。
func chargeOccurrencePartitionKey(key ports.ChargeOccurrenceKey) string {
	return key.TenantID.String() + "/charge-occurrence/" + key.Occurrence.String()
}

// HandOffChargeOccurrence 把一份意图入队。信封 ID 由键三维经指纹铸出：版本在 ID 里，两代各一封，不会被 EnqueueOnce
// 当成重发吞掉；指纹让 ID 定长，不随租户或发生项引用多长而涨。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxChargeOccurrenceHandoff) HandOffChargeOccurrence(
	ctx context.Context,
	intent ports.ChargeOccurrenceIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Occurrence.String() == "" || key.Validity.String() == "" {
		return fmt.Errorf("hand off charge occurrence: charge occurrence key is required")
	}
	payload, err := json.Marshal(chargeOccurrencePayload{
		TenantID:   key.TenantID.String(),
		Occurrence: key.Occurrence.String(),
		Version:    key.Validity.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off charge occurrence: %w", err)
	}

	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID("charge-occurrence-formed",
			key.TenantID.String(), key.Occurrence.String(), key.Validity.String()),
		Source:       tfEventSource,
		Type:         ChargeOccurrenceFormedEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Occurrence.String(),
		PartitionKey: chargeOccurrencePartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off charge occurrence: %w", err)
	}
	return nil
}
