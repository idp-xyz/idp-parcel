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

// notificationEventType 是客户通知决定意图的事件类型。
const notificationEventType = "visibility-exception.customer-notification.formed"

// OutboxNotificationHandoff 把通知决定写入 Outbox，实现 ports.NotificationHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxNotificationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxNotificationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxNotificationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxNotificationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.NotificationHandoff = (*OutboxNotificationHandoff)(nil)

// notificationPayload 是意图载荷的传输形状：通知标识加下游 FindByDisclosure 所需
// 的披露身份三维，不带渠道节点或内容快照。
type notificationPayload struct {
	TenantID       string `json:"tenantId"`
	NotificationID string `json:"notificationId"`
	Customer       string `json:"customer"`
	EpisodeID      string `json:"episodeId"`
	DecidedAt      string `json:"decidedAt"`
}

// HandOffNotification 把一份意图入队。信封 ID 取通知标识——意图由通知标识认领
// （ADR-0043）。租户空白或通知缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxNotificationHandoff) HandOffNotification(
	ctx context.Context,
	intent ports.NotificationHandoffIntent,
) error {
	if intent.Notification == nil || intent.TenantID.String() == "" {
		return fmt.Errorf("hand off notification: tenant and notification are required")
	}

	disclosure := intent.Notification.Disclosure()
	payload, err := json.Marshal(notificationPayload{
		TenantID:       intent.TenantID.String(),
		NotificationID: intent.Notification.ID().String(),
		Customer:       disclosure.Customer().String(),
		EpisodeID:      disclosure.Episode().String(),
		DecidedAt:      disclosure.DecidedAt().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	})
	if err != nil {
		return fmt.Errorf("hand off notification: %w", err)
	}

	now := handoff.clock.Now().UTC()
	occurredAt := now
	if recorded := intent.Notification.RecordedAt(); len(recorded) > 0 {
		occurredAt = recorded[0].UTC()
	}
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.Notification.ID().String()),
		Source:       veEventSource,
		Type:         notificationEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Notification.ID().String(),
		PartitionKey: intent.TenantID.String() + "/" + disclosure.Customer().String(),
		OccurredAt:   occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off notification: %w", err)
	}
	return nil
}
