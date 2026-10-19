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

// 本文件是落下未核对行时给本上下文自投的重核门铃（ADR-0199 决定三第三个触发、越权风险点「未核对行由自投门铃续上」）：未核对的
// 待识别与门铃同一笔事务写下，NO 的重核消费者接住后只重核那一件。

// UncheckedPendingIdentityRecordedEventType 是自投门铃的事件类型。导出给组合根的对照用例：消费方（noinbox）自己写一份同字面的串，
// 两串由那条用例钉住。
const UncheckedPendingIdentityRecordedEventType = "node-operations.unchecked-pending-identity.recorded"

const (
	// uncheckedPendingIdentityEventIDPort 是门铃信封 ID 的口名：ID 由那一版判断认领。
	uncheckedPendingIdentityEventIDPort = "unchecked-pending-identity"
	// uncheckedPendingIdentityPartitionPort 是门铃分区键的口名：同一件作业实物的门铃排一条队。
	uncheckedPendingIdentityPartitionPort = "unchecked-pending-identity-unit"
)

// OutboxIdentityRecheckDoorbell 实现 ports.IdentityRecheckDoorbell。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxIdentityRecheckDoorbell struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxIdentityRecheckDoorbell(db *bentopg.DB, store *outbox.Store, clock ports.Clock) (*OutboxIdentityRecheckDoorbell, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("node operations postgres: clock is nil")
	}
	return &OutboxIdentityRecheckDoorbell{db: db, store: store, clock: clock}, nil
}

var _ ports.IdentityRecheckDoorbell = (*OutboxIdentityRecheckDoorbell)(nil)

// uncheckedPendingIdentityPayload 是门铃载荷的传输形状：只有租户与判断版本。门铃不是身份答案，不带号码、实物或候选——重核按判断
// 版本从收寄册读回那一行，号码取它的来源文档。
type uncheckedPendingIdentityPayload struct {
	TenantID        string `json:"tenantId"`
	JudgmentVersion string `json:"judgmentVersion"`
}

// RingIdentityRecheck 给一条未核对的待识别入队一封门铃。
//
//   - ID 是（租户，判断版本）的定长指纹：一版一封，重放撞同一个 ID，EnqueueOnce 不出第二封。
//   - 分区键是（租户，作业实物）的定长指纹：同一件实物的门铃排一条队。重核只看那一件，不同实物之间没有先后可言，按实物分区
//     不让一件在等 PS 的实物堵住别的实物。
//   - Subject 是判断版本：由本上下文签发、长度有界，不必像 PS 门铃那样为号码换摘要。
//
// 走 RequireExecutor：门铃与未核对行同生共死，没有事务时那一行本就写不进来，在事务外入队只会入一封指名不存在的版本的门铃。
func (doorbell *OutboxIdentityRecheckDoorbell) RingIdentityRecheck(
	ctx context.Context,
	intent ports.IdentityRecheckDoorbellIntent,
) error {
	if _, err := doorbell.db.RequireExecutor(ctx); err != nil {
		return fmt.Errorf("ring identity recheck doorbell: %w", err)
	}
	record := intent.Record
	tenant, version := record.Key.TenantID.String(), record.JudgmentVersion.String()
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(version) == "" {
		return fmt.Errorf("ring identity recheck doorbell: tenant and judgment version are required")
	}
	payload, err := json.Marshal(uncheckedPendingIdentityPayload{TenantID: tenant, JudgmentVersion: version})
	if err != nil {
		return fmt.Errorf("ring identity recheck doorbell: %w", err)
	}
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           outboxintent.FingerprintEventID(uncheckedPendingIdentityEventIDPort, tenant, version),
		Source:       noEventSource,
		Type:         UncheckedPendingIdentityRecordedEventType,
		Version:      1,
		Scope:        tenant,
		Subject:      version,
		PartitionKey: string(outboxintent.FingerprintEventID(uncheckedPendingIdentityPartitionPort, tenant, record.Intake.Unit().String())),
		OccurredAt:   record.RecordedAt.UTC(),
		RecordedAt:   doorbell.clock.Now().UTC(),
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}
	if err := outboxintent.EnqueueOnce(ctx, doorbell.db, doorbell.store, envelope); err != nil {
		return fmt.Errorf("ring identity recheck doorbell: %w", err)
	}
	return nil
}
