package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 事件类型与包裹那一路相同：计价按类型分派到同一只消费者，再按标识决定去哪本册。分区键带另一段，不跟包裹那一路挤在一条队里。
const shipmentSellEvaluationEventIDPort = "shipment-sell-evaluation"

// OutboxShipmentSellEvaluationRequestHandoff 把委托级请求提交写入 Outbox。
type OutboxShipmentSellEvaluationRequestHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxShipmentSellEvaluationRequestHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxShipmentSellEvaluationRequestHandoff, error) {
	if db == nil || store == nil || clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: shipment sell evaluation request handoff is missing a dependency")
	}
	return &OutboxShipmentSellEvaluationRequestHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ShipmentSellEvaluationRequestHandoff = (*OutboxShipmentSellEvaluationRequestHandoff)(nil)

func (handoff *OutboxShipmentSellEvaluationRequestHandoff) HandOffShipmentSellEvaluationRequest(
	ctx context.Context,
	record ports.ShipmentSellEvaluationRequestRecord,
) error {
	if record.Key.TenantID.String() == "" || record.Key.Request.String() == "" || record.Request.Shipment().String() == "" {
		return fmt.Errorf("hand off shipment sell evaluation request: tenant, request and shipment are required")
	}
	if record.RecordedAt.IsZero() {
		return fmt.Errorf("hand off shipment sell evaluation request: registration moment is required")
	}
	payload, err := json.Marshal(sellEvaluationRequestPayload{
		TenantID:                record.Key.TenantID.String(),
		SellEvaluationRequestID: record.Key.Request.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off shipment sell evaluation request: %w", err)
	}
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID: outboxintent.FingerprintEventID(shipmentSellEvaluationEventIDPort,
			record.Key.TenantID.String(), record.Key.Request.String(), "submitted"),
		Source:       saEventSource,
		Type:         sellEvaluationRequestEventType,
		Version:      1,
		Scope:        record.Key.TenantID.String(),
		Subject:      record.Key.Request.String(),
		PartitionKey: record.Key.TenantID.String() + "/shipment-sell-evaluation/" + record.Request.Shipment().String(),
		OccurredAt:   record.RecordedAt.UTC(),
		RecordedAt:   handoff.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		if errors.Is(err, eventing.ErrInvalidEnvelope) {
			return fmt.Errorf("hand off shipment sell evaluation request: %w: %w", ports.ErrSellEvaluationRequestHandoffRejected, err)
		}
		return fmt.Errorf("hand off shipment sell evaluation request: %w", err)
	}
	return nil
}
