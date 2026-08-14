package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

const supplierBillEventType = "settlement-accounting.supplier-bill.received"

// OutboxSupplierBillHandoff 把已提交的供应商账单接收写入 Outbox，实现
// ports.SupplierBillHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxSupplierBillHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSupplierBillHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSupplierBillHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("settlement accounting postgres: clock is nil")
	}
	return &OutboxSupplierBillHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SupplierBillHandoff = (*OutboxSupplierBillHandoff)(nil)

type supplierBillPayload struct {
	TenantID string `json:"tenantId"`
	Claim    string `json:"claim"`
	Version  string `json:"version"`
}

func supplierBillEventID(key ports.BillReceptionKey) string {
	return key.TenantID.String() + "/bill/" + key.Claim.String() + "/" + key.Version.String()
}

// HandOffSupplierBill 把一份意图入队。信封 ID 取（租户+主张+版本）并加 /bill/ 段——
// 意图由账单接收幂等键认领（ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxSupplierBillHandoff) HandOffSupplierBill(
	ctx context.Context,
	intent ports.SupplierBillHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Claim.String() == "" || key.Version.String() == "" {
		return fmt.Errorf("hand off supplier bill: bill reception key is required")
	}

	payload, err := json.Marshal(supplierBillPayload{
		TenantID: key.TenantID.String(),
		Claim:    key.Claim.String(),
		Version:  key.Version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off supplier bill: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := supplierBillEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       saEventSource,
		Type:         supplierBillEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Claim.String() + "/" + key.Version.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off supplier bill: %w", err)
	}
	return nil
}
