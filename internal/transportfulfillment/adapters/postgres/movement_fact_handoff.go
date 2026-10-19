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

// MovementFactRecordedEventType 是「TF 登记了一个实际移动事实版本」那封引用式信封的类型（ADR-0205 决定五）。消费方是
// 全程追踪的投影派生，它按自己的惯例自写同一串、路由表引的是它那一个，生产代码里不 import 本适配器；导出只为
// cmd/parcel-dispatch 那条同时装配两边的用例对照两串相等。
const MovementFactRecordedEventType = "transport-fulfillment.movement-fact.recorded"

// OutboxMovementFactHandoff 把已登记的移动事实版本写入 Outbox，实现 ports.MovementFactHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxMovementFactHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxMovementFactHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxMovementFactHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxMovementFactHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.MovementFactHandoff = (*OutboxMovementFactHandoff)(nil)

// movementFactPayload 恰是登记册的键三维，键名与消费门认的那三个一字不差。种类、段、业务时间与门禁读数一样不带：它们
// 只有登记册一处权威，消费方按键回读，信封上抄一份就是第二处——读数尤其不能抄，抄了就有两份「当时关务怎么答」。
type movementFactPayload struct {
	TenantID string `json:"tenantId"`
	Fact     string `json:"fact"`
	Version  string `json:"version"`
}

// movementFactPartitionKey 取「租户/移动事实」：同一事实的首版与日后的更正版排一条队——更正先于原版到达，全程追踪就会
// 对着一份还没见过的前身登替代关系。不取到版本：取到版本就是逐信封分区，保不了这个序。不取到对象：一条事实的对象要
// 消费方按段回读才知道，信封这一侧说不出来。
func movementFactPartitionKey(key ports.MovementFactKey) string {
	return key.TenantID.String() + "/movement-fact/" + key.Fact.String()
}

// HandOffMovementFact 把一份意图入队。信封 ID 由键三维经指纹铸出：版本在 ID 里，两代各一封，不会被 EnqueueOnce 当成
// 重发吞掉；指纹让 ID 定长，不随租户或事实引用多长而涨。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxMovementFactHandoff) HandOffMovementFact(
	ctx context.Context,
	intent ports.MovementFactIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Fact.String() == "" || key.Version.String() == "" {
		return fmt.Errorf("hand off movement fact: movement fact key is required")
	}
	payload, err := json.Marshal(movementFactPayload{
		TenantID: key.TenantID.String(),
		Fact:     key.Fact.String(),
		Version:  key.Version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off movement fact: %w", err)
	}

	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID("movement-fact-recorded",
			key.TenantID.String(), key.Fact.String(), key.Version.String()),
		Source:       tfEventSource,
		Type:         MovementFactRecordedEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Fact.String(),
		PartitionKey: movementFactPartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off movement fact: %w", err)
	}
	return nil
}
