package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// ccEventSource 是本上下文在信封 Source 位上的稳定名。
const ccEventSource = "idp-parcel/customs-compliance"

// externalResultEventType 是外部结果接收意图的事件类型。
const externalResultEventType = "customs-compliance.external-result.received"

// OutboxExternalResultHandoff 把外部结果接收意图写进 Outbox，实现
// ports.ExternalResultHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxExternalResultHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxExternalResultHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxExternalResultHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxExternalResultHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ExternalResultHandoff = (*OutboxExternalResultHandoff)(nil)

// externalResultPayload 是意图载荷的传输形状：只有下游按幂等键查库所需的两维，
// 不带层事实或原始语义。
type externalResultPayload struct {
	TenantID string `json:"tenantId"`
	SourceID string `json:"sourceId"`
}

// externalResultEventIDPort 是本口在信封 ID 上的口名前缀。
const externalResultEventIDPort = "external-result"

// externalResultEventID 把接收幂等键两维折成 outboxintent.FingerprintEventID 的定长形（票 sa-cc/34 裁决 3）。SourceID
// 是来源给的裸字符串、连构造门都没有，长度完全归实例半边——正是不能拼进 ID 的那一类。
func externalResultEventID(key ports.ExternalResultKey) eventing.EventID {
	return outboxintent.FingerprintEventID(externalResultEventIDPort, key.TenantID.String(), key.SourceID)
}

// externalResultPartitionKey 取「租户 / 来源标识」可读串接——这正是 ID 换成指纹形之前信封 ID 兼作分区键的那一串，
// 一字不变（票 sa-cc/34 裁决 3）。可读形留给运维，超 eventing.MaxPartitionKeyLength 由框架校验拒收、本口分格交出。
func externalResultPartitionKey(key ports.ExternalResultKey) string {
	return key.TenantID.String() + "/" + key.SourceID
}

// HandOffExternalResult 把一份意图入队。信封 ID 由接收幂等键（租户+来源标识）认领
// （ADR-0043）、折成定长指纹形；分区键取该键的可读串接。归属不上的留存没有可供判断
// 消费的监管事实，响亮报错不入队，与编排不交那条对齐。
func (handoff *OutboxExternalResultHandoff) HandOffExternalResult(
	ctx context.Context,
	intent ports.ExternalResultHandoffIntent,
) error {
	if intent.Record.Key.TenantID.String() == "" || intent.Record.Key.SourceID == "" {
		return fmt.Errorf("hand off external result: receive key is required")
	}
	if intent.Record.Unattributable {
		return fmt.Errorf("hand off external result: unattributable record has no judgment fact")
	}

	payload, err := json.Marshal(externalResultPayload{
		TenantID: intent.Record.Key.TenantID.String(),
		SourceID: intent.Record.Key.SourceID,
	})
	if err != nil {
		return fmt.Errorf("hand off external result: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           externalResultEventID(intent.Record.Key),
		Source:       ccEventSource,
		Type:         externalResultEventType,
		Version:      1,
		Scope:        intent.Record.Key.TenantID.String(),
		Subject:      intent.Record.Key.SourceID,
		PartitionKey: externalResultPartitionKey(intent.Record.Key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off external result: %w", err)
	}
	return nil
}
