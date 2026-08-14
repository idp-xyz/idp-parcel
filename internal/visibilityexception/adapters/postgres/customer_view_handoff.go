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

// customerViewEventType 是客户视图发布意图的事件类型。
const customerViewEventType = "visibility-exception.customer-view.published"

// OutboxCustomerViewHandoff 把客户视图写入 Outbox，实现 ports.CustomerViewHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxCustomerViewHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxCustomerViewHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxCustomerViewHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxCustomerViewHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.CustomerViewHandoff = (*OutboxCustomerViewHandoff)(nil)

// customerViewPayload 是意图载荷的传输形状：只有下游 FindCurrent 所需的租户/客户/
// 包裹，外加版本标识认领，不带四维内容。
type customerViewPayload struct {
	TenantID  string `json:"tenantId"`
	Customer  string `json:"customer"`
	Parcel    string `json:"parcel"`
	VersionID string `json:"versionId"`
}

// HandOffCustomerView 把一份意图入队。信封 ID 取视图版本——意图由视图版本认领
// （ADR-0043）。租户或版本空白是装配缺陷，响亮报错不入队。
func (handoff *OutboxCustomerViewHandoff) HandOffCustomerView(
	ctx context.Context,
	intent ports.CustomerViewHandoffIntent,
) error {
	if intent.TenantID.String() == "" || intent.View.Version().String() == "" {
		return fmt.Errorf("hand off customer view: tenant and view version are required")
	}

	payload, err := json.Marshal(customerViewPayload{
		TenantID:  intent.TenantID.String(),
		Customer:  intent.View.Customer().String(),
		Parcel:    intent.View.Parcel().String(),
		VersionID: intent.View.Version().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off customer view: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.View.Version().String()),
		Source:       veEventSource,
		Type:         customerViewEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.View.Version().String(),
		PartitionKey: intent.TenantID.String() + "/" + intent.View.Customer().String(),
		OccurredAt:   intent.View.PublishedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off customer view: %w", err)
	}
	return nil
}
