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

const exceptionJourneyEventType = "transport-fulfillment.exception-journey.recorded"

// OutboxExceptionJourneyHandoff 把替代旅程写入 Outbox 交异常链，实现
// ports.ExceptionJourneyHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxExceptionJourneyHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxExceptionJourneyHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxExceptionJourneyHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxExceptionJourneyHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ExceptionJourneyHandoff = (*OutboxExceptionJourneyHandoff)(nil)

// exceptionJourneyPort 是本口在信封 ID 上的口名前缀。监管来路的两口（本口与 DispositionExecutionHandoff）同键、同事务入队，
// 口名把两口的 ID 空间错开（FingerprintEventID 头注）。
const exceptionJourneyPort = "exception-journey"

// exceptionJourneyEventID 按替代旅程幂等键的四维（租户、原旅程、目的、处置依据）铸 ID：意图由它们认领（ADR-0043）。
func exceptionJourneyEventID(key ports.AlternateJourneyKey) eventing.EventID {
	return outboxintent.FingerprintEventID(exceptionJourneyPort,
		key.TenantID.String(), key.Original.String(), key.Purpose.String(), key.Basis.String())
}

// exceptionJourneyPartitionKey 保留可读的「键四维/口名」：一份替代旅程只走本口一拍，分区仍是逐信封，取可读形只为
// 运维从分区键认得出是哪份旅程。
func exceptionJourneyPartitionKey(key ports.AlternateJourneyKey) string {
	return alternateJourneyKeyParts(key) + "/" + exceptionJourneyPort
}

// HandOffExceptionJourney 把一份意图入队。信封 ID 由（租户+原旅程+目的+处置依据）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxExceptionJourneyHandoff) HandOffExceptionJourney(
	ctx context.Context,
	intent ports.AlternateJourneyIntent,
) error {
	key := intent.Record.Key
	if alternateJourneyKeyMissing(key) {
		return fmt.Errorf("hand off exception journey: journey key is required")
	}

	payload, err := json.Marshal(alternateJourneyPayload{
		TenantID: key.TenantID.String(),
		Original: key.Original.String(),
		Purpose:  key.Purpose.String(),
		Basis:    key.Basis.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off exception journey: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           exceptionJourneyEventID(key),
		Source:       tfEventSource,
		Type:         exceptionJourneyEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Original.String() + "/" + key.Purpose.String() + "/" + key.Basis.String(),
		PartitionKey: exceptionJourneyPartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off exception journey: %w", err)
	}
	return nil
}
