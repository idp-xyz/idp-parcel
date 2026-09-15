package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// gateVerificationEventType 是放行门禁核对意图的事件类型。
const gateVerificationEventType = "customs-compliance.gate-verification.recorded"

// OutboxGateVerificationHandoff 把门禁核对写入 Outbox，实现
// ports.GateVerificationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxGateVerificationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxGateVerificationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxGateVerificationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxGateVerificationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.GateVerificationHandoff = (*OutboxGateVerificationHandoff)(nil)

// gateVerificationPayload 是意图载荷的传输形状：只有下游 FindByKey 所需的幂等键，
// 不带前置条件清单或结论明细。
type gateVerificationPayload struct {
	TenantID string `json:"tenantId"`
	Scope    string `json:"scope"`
	Action   string `json:"action"`
	Boundary string `json:"boundary"`
	Digest   string `json:"digest"`
}

// gateVerificationEventIDPort 是本口在信封 ID 上的口名前缀。
const gateVerificationEventIDPort = "gate-verification"

// gateVerificationEventID 把门禁幂等键五维折成 fingerprintEventID 的定长形（票 sa-cc/29 裁决 1：三只核对口同改）。
func gateVerificationEventID(key ports.GateVerificationKey) string {
	return fingerprintEventID(gateVerificationEventIDPort,
		key.TenantID.String(), key.Scope.String(), key.Action.String(), key.Boundary.String(), key.Digest)
}

// gateVerificationPartitionKey 取（租户+范围+动作+边界），不取整个幂等键。
//
// ID 管幂等、分区键管顺序，两者不是一回事。同一道门禁的核对随条件状态变化换指纹换版
// （受阻 → 放行），指纹进分区键每版就自成一区，后一版可能先于前一版送达，下游读到的
// 放行结论从此没有先后可言。
func gateVerificationPartitionKey(key ports.GateVerificationKey) string {
	return key.TenantID.String() + "/" + key.Scope.String() + "/" +
		key.Action.String() + "/" + key.Boundary.String()
}

// HandOffGate 把一份意图入队。信封 ID 取门禁幂等键——意图由幂等键认领（ADR-0043）。
// 键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxGateVerificationHandoff) HandOffGate(
	ctx context.Context,
	intent ports.GateVerificationHandoffIntent,
) error {
	key := intent.Key
	if key.TenantID.String() == "" ||
		key.Scope.String() == "" ||
		key.Action.String() == "" ||
		key.Boundary.String() == "" ||
		key.Digest == "" {
		return fmt.Errorf("hand off gate verification: receive key is required")
	}

	payload, err := json.Marshal(gateVerificationPayload{
		TenantID: key.TenantID.String(),
		Scope:    key.Scope.String(),
		Action:   key.Action.String(),
		Boundary: key.Boundary.String(),
		Digest:   key.Digest,
	})
	if err != nil {
		return fmt.Errorf("hand off gate verification: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := gateVerificationEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         gateVerificationEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Scope.String() + "/" + key.Action.String(),
		PartitionKey: gateVerificationPartitionKey(key),
		OccurredAt:   intent.Gate.VerifiedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off gate verification: %w", err)
	}
	return nil
}
