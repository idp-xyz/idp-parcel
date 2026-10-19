package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// 本文件是「交付范围成立」时给本上下文自投的重核门铃（ADR-0212 决定六）：交接单最后一个绑定得到确认，或实物归入一张范围已成立的
// 交接单，与结论或成员同一笔事务写下；NO 的重核消费者接住后按交接单重核名下的重核对象，或只重核归入的那一版。

// DeliveryScopeEstablishedEventType 是这一种门铃的事件类型。导出给组合根的对照用例：消费方（noinbox）自己写一份同字面的串，两串由那
// 条用例钉住。
const DeliveryScopeEstablishedEventType = "node-operations.delivery-scope.established"

const (
	// deliveryScopeEstablishedNotePort 是最后一个绑定确认那一封的信封 ID 口名：一张交接单的范围只成立一次（确认与否认都是终态），
	// ID 由交接单认领。
	deliveryScopeEstablishedNotePort = "delivery-scope-established"
	// deliveryScopeEstablishedUnitPort 是归入那一封的信封 ID 口名：ID 由（交接单，判断版本）认领。与上一种分口名，两种 ID 空间不相交。
	deliveryScopeEstablishedUnitPort = "delivery-scope-established-unit"
	// deliveryScopeEstablishedPartitionPort 是分区键的口名：同一张交接单的两种门铃排一条队。
	deliveryScopeEstablishedPartitionPort = "delivery-scope-established-note"
)

// OutboxDeliveryScopeEstablishedDoorbell 实现 ports.DeliveryScopeEstablishedDoorbell。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxDeliveryScopeEstablishedDoorbell struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDeliveryScopeEstablishedDoorbell(db *bentopg.DB, store *outbox.Store, clock ports.Clock) (*OutboxDeliveryScopeEstablishedDoorbell, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("node operations postgres: clock is nil")
	}
	return &OutboxDeliveryScopeEstablishedDoorbell{db: db, store: store, clock: clock}, nil
}

var _ ports.DeliveryScopeEstablishedDoorbell = (*OutboxDeliveryScopeEstablishedDoorbell)(nil)

// deliveryScopeEstablishedPayload 是门铃载荷的传输形状：租户、交接单，归入那一种另带判断版本。门铃不是身份答案，不带号码、实物、
// 客户账户或候选——重核按交接单读成员、按判断版本读那一行，范围按存下的依据与此刻的证据重算。
type deliveryScopeEstablishedPayload struct {
	TenantID        string `json:"tenantId"`
	HandoverNote    string `json:"handoverNote"`
	JudgmentVersion string `json:"judgmentVersion,omitempty"`
}

// RingDeliveryScopeEstablished 入队一封门铃。
//
//   - ID：最后一个绑定确认那一封是（租户，交接单）的定长指纹，归入那一封是（租户，交接单，判断版本）的；重放撞同一个 ID，EnqueueOnce
//     不出第二封。
//   - 分区键是（租户，交接单）的定长指纹：同一张交接单的门铃排一条队，不同交接单之间没有先后可言。
//   - Subject 是交接单标识：由设备签发、在本上下文的册上有界。
//
// 走 RequireExecutor：门铃与触发它的结论或成员同生共死，在事务外入队只会入一封指名一份并不存在的结论的门铃。
func (doorbell *OutboxDeliveryScopeEstablishedDoorbell) RingDeliveryScopeEstablished(
	ctx context.Context,
	intent ports.DeliveryScopeEstablishedIntent,
) error {
	if _, err := doorbell.db.RequireExecutor(ctx); err != nil {
		return fmt.Errorf("ring delivery scope established doorbell: %w", err)
	}
	tenant, note, version := intent.Tenant.String(), intent.Note.String(), intent.Version.String()
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(note) == "" {
		return fmt.Errorf("ring delivery scope established doorbell: tenant and handover note are required")
	}
	payload, err := json.Marshal(deliveryScopeEstablishedPayload{TenantID: tenant, HandoverNote: note, JudgmentVersion: version})
	if err != nil {
		return fmt.Errorf("ring delivery scope established doorbell: %w", err)
	}
	id := outboxintent.FingerprintEventID(deliveryScopeEstablishedNotePort, tenant, note)
	if version != "" {
		id = outboxintent.FingerprintEventID(deliveryScopeEstablishedUnitPort, tenant, note, version)
	}
	occurredAt := intent.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = doorbell.clock.Now()
	}
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           id,
		Source:       noEventSource,
		Type:         DeliveryScopeEstablishedEventType,
		Version:      1,
		Scope:        tenant,
		Subject:      note,
		PartitionKey: string(outboxintent.FingerprintEventID(deliveryScopeEstablishedPartitionPort, tenant, note)),
		OccurredAt:   occurredAt.UTC(),
		RecordedAt:   doorbell.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, doorbell.db, doorbell.store, envelope); err != nil {
		return fmt.Errorf("ring delivery scope established doorbell: %w", err)
	}
	return nil
}
