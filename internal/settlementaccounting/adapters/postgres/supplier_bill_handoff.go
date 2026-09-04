package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

const (
	supplierBillEventType       eventing.EventType = "settlement-accounting.supplier-bill.received"
	auditedPayableEventType     eventing.EventType = "settlement-accounting.supplier-bill.payable-audited"
	supplierCreditNoteEventType eventing.EventType = "settlement-accounting.supplier-bill.credit-note-formed"
)

// OutboxSupplierBillHandoff 把 UC-SA-004 越过提交边界的三种东西写入 Outbox，实现
// ports.SupplierBillHandoff：接收记录、审核应付引用、供应商费用贷项引用，各成一封、各由
// 自己的幂等键认领。入队一步由 outboxintent.EnqueueOnce 承担。
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

// supplierBillPayload 是三种信封共用的传输形状：只带下游按键读回所需的身份，不带金额
// 或行。接收封填 Claim/Version，应付封填 Claim/Payable，贷项封填 Claim/Payable/CreditNote
// 与其版本。
type supplierBillPayload struct {
	TenantID          string `json:"tenantId"`
	Claim             string `json:"claim"`
	Version           string `json:"version,omitempty"`
	Payable           string `json:"payable,omitempty"`
	CreditNote        string `json:"creditNote,omitempty"`
	CreditNoteVersion string `json:"creditNoteVersion,omitempty"`
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
//
// 审核应付与供应商费用贷项也排在这一条队上（claimPartitionKey）：应付审的是这份主张的一
// 行，贷项贷的是那份应付——消费方必须先看到接收、再看到应付、再看到贷项。它们各自的
// 身份进信封 ID（/payable/、/credit-note/ 段），互不吞，也不与接收封撞。
func supplierBillPartitionKey(key ports.BillReceptionKey) string {
	return claimPartitionKey(key.TenantID, key.Claim)
}

func claimPartitionKey(tenant domain.TenantID, claim domain.BillClaimID) string {
	return tenant.String() + "/bill/" + claim.String()
}

// supplierBillShape 把一封信的身份与分区分开持有：ID 管幂等，分区键管顺序（理由同
// statement_handoff.go 的 statementHandoffShape）。
type supplierBillShape struct {
	eventID      string
	partitionKey string
	eventType    eventing.EventType
	tenant       string
	subject      string
	occurredAt   time.Time
	payload      supplierBillPayload
}

// supplierBillShapeOf 按意图携带的那一格定形状。恰填一格：两格同填是装配缺陷——应付与
// 贷项必须分别发布（UC-SA-004 步 7，AT-SA-098 不把一方发布成功推定为另一方已发布），
// 一封装两份就没法各自认领各自的发布意图。
func supplierBillShapeOf(intent ports.SupplierBillHandoffIntent) (supplierBillShape, error) {
	hasRecord := intent.Record.Key.Claim.String() != ""
	hasPayable := intent.Payable.Key.Payable.String() != ""
	hasCreditNote := intent.CreditNote.Key.Note.String() != ""
	filled := 0
	for _, has := range []bool{hasRecord, hasPayable, hasCreditNote} {
		if has {
			filled++
		}
	}
	if filled != 1 {
		return supplierBillShape{}, fmt.Errorf(
			"hand off supplier bill: exactly one of reception, payable or credit note is required (got %d)", filled)
	}

	switch {
	case hasPayable:
		key := intent.Payable.Key
		payable := intent.Payable.Payable
		if key.TenantID.String() == "" || payable.Claim().String() == "" {
			return supplierBillShape{}, fmt.Errorf("hand off supplier bill: payable key is required")
		}
		partition := claimPartitionKey(key.TenantID, payable.Claim())
		return supplierBillShape{
			eventID:      partition + "/payable/" + key.Payable.String(),
			partitionKey: partition,
			eventType:    auditedPayableEventType,
			tenant:       key.TenantID.String(),
			subject:      payable.Claim().String() + "/" + key.Payable.String(),
			occurredAt:   payable.AuditedAt().UTC(),
			payload: supplierBillPayload{
				TenantID: key.TenantID.String(),
				Claim:    payable.Claim().String(),
				Payable:  key.Payable.String(),
			},
		}, nil
	case hasCreditNote:
		key := intent.CreditNote.Key
		note := intent.CreditNote.Note
		if key.TenantID.String() == "" || key.Version.String() == "" || note.Claim().String() == "" {
			return supplierBillShape{}, fmt.Errorf("hand off supplier bill: credit note key is required")
		}
		partition := claimPartitionKey(key.TenantID, note.Claim())
		return supplierBillShape{
			eventID:      partition + "/credit-note/" + key.Note.String() + "/" + key.Version.String(),
			partitionKey: partition,
			eventType:    supplierCreditNoteEventType,
			tenant:       key.TenantID.String(),
			subject:      note.Claim().String() + "/" + key.Note.String() + "/" + key.Version.String(),
			occurredAt:   note.IssuedAt().UTC(),
			payload: supplierBillPayload{
				TenantID:          key.TenantID.String(),
				Claim:             note.Claim().String(),
				Payable:           note.Payable().String(),
				CreditNote:        key.Note.String(),
				CreditNoteVersion: key.Version.String(),
			},
		}, nil
	default:
		key := intent.Record.Key
		if key.TenantID.String() == "" || key.Version.String() == "" {
			return supplierBillShape{}, fmt.Errorf("hand off supplier bill: bill reception key is required")
		}
		return supplierBillShape{
			eventID:      supplierBillEventID(key),
			partitionKey: supplierBillPartitionKey(key),
			eventType:    supplierBillEventType,
			tenant:       key.TenantID.String(),
			subject:      key.Claim.String() + "/" + key.Version.String(),
			occurredAt:   intent.Record.RecordedAt.UTC(),
			payload: supplierBillPayload{
				TenantID: key.TenantID.String(),
				Claim:    key.Claim.String(),
				Version:  key.Version.String(),
			},
		}, nil
	}
}

// HandOffSupplierBill 把一份意图入队。接收由（租户+主张+版本）认领，应付由应付身份认领，
// 贷项由（贷项身份+版本）认领，都挂在 /bill/<主张> 段之下（ADR-0043）。键缺席是装配缺陷，
// 响亮报错不入队。
func (handoff *OutboxSupplierBillHandoff) HandOffSupplierBill(
	ctx context.Context,
	intent ports.SupplierBillHandoffIntent,
) error {
	shape, err := supplierBillShapeOf(intent)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(shape.payload)
	if err != nil {
		return fmt.Errorf("hand off supplier bill: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(shape.eventID),
		Source:       saEventSource,
		Type:         shape.eventType,
		Version:      1,
		Scope:        shape.tenant,
		Subject:      shape.subject,
		PartitionKey: shape.partitionKey,
		OccurredAt:   shape.occurredAt,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off supplier bill: %w", err)
	}
	return nil
}
