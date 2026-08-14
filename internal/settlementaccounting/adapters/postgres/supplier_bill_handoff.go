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
	return supplierBillPartitionKey(key) + "/" + key.Version.String()
}

// supplierBillPartitionKey 取「其先后状态必须保序的那个对象」——账单主张，不含版本。
//
// 版本留在信封 ID 里（同一主张的每个版本各自入队，一个都不丢），但不进分区键：同一
// 主张的 v2 重述 v1 说过的事，审核与对账消费方必须按顺序看到它们。把版本也拼进分区键
// 会让两版落进互不排队的两条队，于是 v1 可能在 v2 之后被处理，审的是已被取代的那一版。
//
// 这一处此前被判为无害，判据是「BillReceptionStore 没有 Replace」——**那个判据不充分**。
// 更正入口只是第一步筛查；真正要问的是「后一条会不会改写前一条说过的事」，而版本进键
// 本身就意味着同一主张会有多条信封。
func supplierBillPartitionKey(key ports.BillReceptionKey) string {
	return key.TenantID.String() + "/bill/" + key.Claim.String()
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
		PartitionKey: supplierBillPartitionKey(key),
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
