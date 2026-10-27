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

const dispositionExecutionEventType = "transport-fulfillment.disposition-execution.recorded"

// OutboxDispositionExecutionHandoff 把监管来路的替代旅程写入 Outbox，实现
// ports.DispositionExecutionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxDispositionExecutionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDispositionExecutionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxDispositionExecutionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxDispositionExecutionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.DispositionExecutionHandoff = (*OutboxDispositionExecutionHandoff)(nil)

type alternateJourneyPayload struct {
	TenantID string `json:"tenantId"`
	Original string `json:"original"`
	Purpose  string `json:"purpose"`
	Basis    string `json:"basis"`
}

func alternateJourneyKeyParts(key ports.AlternateJourneyKey) string {
	return key.TenantID.String() + "/" + key.Original.String() + "/" +
		key.Purpose.String() + "/" + key.Basis.String()
}

// dispositionExecutionPort 是本口在信封 ID 上的口名前缀。监管来路的两口（本口与 ExceptionJourneyHandoff）同键、同事务入队，
// 口名把两口的 ID 空间错开（FingerprintEventID 头注）。
const dispositionExecutionPort = "disposition-execution"

// dispositionExecutionEventID 按替代旅程幂等键的四维（租户、原旅程、目的、处置依据）铸 ID：意图由它们认领（ADR-0043）。
func dispositionExecutionEventID(key ports.AlternateJourneyKey) eventing.EventID {
	return outboxintent.FingerprintEventID(dispositionExecutionPort,
		key.TenantID.String(), key.Original.String(), key.Purpose.String(), key.Basis.String())
}

// dispositionExecutionPartitionKey 保留可读的「键四维/口名」：一份替代旅程只走本口一拍，分区仍是逐信封，取可读形只为
// 运维从分区键认得出是哪份旅程。
func dispositionExecutionPartitionKey(key ports.AlternateJourneyKey) string {
	return alternateJourneyKeyParts(key) + "/" + dispositionExecutionPort
}

func alternateJourneyKeyMissing(key ports.AlternateJourneyKey) bool {
	return key.TenantID.String() == "" ||
		key.Original.String() == "" ||
		key.Purpose.String() == "" ||
		key.Basis.String() == ""
}

// HandOffDispositionExecution 把一份意图入队。信封 ID 由（租户+原旅程+目的+处置依据）认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxDispositionExecutionHandoff) HandOffDispositionExecution(
	ctx context.Context,
	intent ports.AlternateJourneyIntent,
) error {
	key := intent.Record.Key
	if alternateJourneyKeyMissing(key) {
		return fmt.Errorf("hand off disposition execution: journey key is required")
	}

	payload, err := json.Marshal(alternateJourneyPayload{
		TenantID: key.TenantID.String(),
		Original: key.Original.String(),
		Purpose:  key.Purpose.String(),
		Basis:    key.Basis.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off disposition execution: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           dispositionExecutionEventID(key),
		Source:       tfEventSource,
		Type:         dispositionExecutionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Original.String() + "/" + key.Purpose.String() + "/" + key.Basis.String(),
		PartitionKey: dispositionExecutionPartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off disposition execution: %w", err)
	}
	return nil
}
