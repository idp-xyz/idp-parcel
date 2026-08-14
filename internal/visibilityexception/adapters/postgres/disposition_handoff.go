package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// veEventSource 是本上下文在信封 Source 位上的稳定名。
const veEventSource = "idp-parcel/visibility-exception"

// dispositionRequestEventType 是处置请求发送意图的事件类型。
const dispositionRequestEventType = "visibility-exception.disposition-request.sent"

// OutboxDispositionHandoff 把处置请求发送意图写进 Outbox，实现
// ports.DispositionHandoff。入队一步由 outboxintent.EnqueueOnce 承担，不在这里
// 再写一遍先查后插。
type OutboxDispositionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDispositionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxDispositionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxDispositionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.DispositionHandoff = (*OutboxDispositionHandoff)(nil)

// dispositionRequestPayload 是意图载荷的传输形状：只有下游查库所需的（租户+请求
// 标识），不带动作、范围或证据明细。
type dispositionRequestPayload struct {
	TenantID  string `json:"tenantId"`
	RequestID string `json:"requestId"`
	CaseID    string `json:"caseId"`
}

// HandOffDispositionRequest 把一份意图入队。信封 ID 取请求标识——意图由请求标识
// 认领（ADR-0043）。租户空白或请求缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxDispositionHandoff) HandOffDispositionRequest(
	ctx context.Context,
	intent ports.DispositionHandoffIntent,
) error {
	if intent.Request == nil || intent.TenantID.String() == "" {
		return fmt.Errorf("hand off disposition request: tenant and request are required")
	}

	payload, err := json.Marshal(dispositionRequestPayload{
		TenantID:  intent.TenantID.String(),
		RequestID: intent.Request.ID().String(),
		CaseID:    intent.Request.Case().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off disposition request: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.Request.ID().String()),
		Source:       veEventSource,
		Type:         dispositionRequestEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Request.ID().String(),
		PartitionKey: intent.TenantID.String() + "/" + intent.Request.Case().String(),
		OccurredAt:   intent.Request.Snapshot().SentAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off disposition request: %w", err)
	}
	return nil
}
