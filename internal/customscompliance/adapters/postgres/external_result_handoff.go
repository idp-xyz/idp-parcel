package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// ccEventSource 是本上下文在信封 Source 位上的稳定名。
const ccEventSource = "idp-parcel/customs-compliance"

// fingerprintEventID 铸定长的信封 ID：口名前缀 + "/" + 各维以 \x00 拼接后的 sha256 十六进制（票 sa-cc/29 裁决 1）。
//
// 信封 ID 的上限是 eventing.MaxEventIDLength，而租户、范围、税费这类引用多长归实例半边、本仓给不出上界；把它们原样
// 拼进 ID 就是把上限押在别人的长度上——旧串接形（钉 `a0cb6fef`，票 sa-cc/19 作者 tip）下，`tenant-a` / `SYN-UNIT-RD` /
// `SYN-DUTY-RD/v1` / `bank-fact-2` 四个短合成引用加一段六十四位指纹就拼出 138 字节，被 Envelope.Validate 确定性
// 拒收。哈希把长度钉死在「前缀 + 六十四」，与任何一维多长无关；口名前缀让几只口在同一个
// ccEventSource 下的 ID 空间互不相交（outboxintent.EnqueueOnce 按（source, event_id）查重）。代价是 ID 不再可读，
// 运维从 ID 反查走载荷——载荷照旧全量带引用。
func fingerprintEventID(portName string, dimensions ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(dimensions, "\x00")))
	return portName + "/" + hex.EncodeToString(digest[:])
}

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

func externalResultEventID(key ports.ExternalResultKey) string {
	return key.TenantID.String() + "/" + key.SourceID
}

// HandOffExternalResult 把一份意图入队。信封 ID 取接收幂等键（租户+来源标识）——
// 意图由幂等键认领（ADR-0043）。归属不上的留存没有可供判断消费的监管事实，响亮
// 报错不入队，与编排不交那条对齐。
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
	eventID := externalResultEventID(intent.Record.Key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         externalResultEventType,
		Version:      1,
		Scope:        intent.Record.Key.TenantID.String(),
		Subject:      intent.Record.Key.SourceID,
		PartitionKey: eventID,
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
