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

// verificationEventType 是处置执行核对意图的事件类型。
const verificationEventType = "customs-compliance.disposition-verification.recorded"

// OutboxVerificationHandoff 把核对结论写入 Outbox，实现 ports.VerificationHandoff。
// 入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxVerificationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxVerificationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxVerificationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxVerificationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.VerificationHandoff = (*OutboxVerificationHandoff)(nil)

// verificationPayload 是意图载荷的传输形状：只有下游 FindByKey 所需的核对幂等键，
// 不带执行事实或结论明细。
type verificationPayload struct {
	TenantID   string `json:"tenantId"`
	DecisionID string `json:"decisionId"`
	Digest     string `json:"digest"`
}

func verificationEventID(key ports.VerificationKey) string {
	return key.TenantID.String() + "/" + key.Decision.String() + "/" + key.Digest
}

// HandOffVerification 把一份意图入队。信封 ID 取核对幂等键——意图由核对键认领
// （ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxVerificationHandoff) HandOffVerification(
	ctx context.Context,
	intent ports.VerificationHandoffIntent,
) error {
	key := intent.Key
	if key.TenantID.String() == "" || key.Decision.String() == "" || key.Digest == "" {
		return fmt.Errorf("hand off verification: receive key is required")
	}

	payload, err := json.Marshal(verificationPayload{
		TenantID:   key.TenantID.String(),
		DecisionID: key.Decision.String(),
		Digest:     key.Digest,
	})
	if err != nil {
		return fmt.Errorf("hand off verification: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := verificationEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         verificationEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Decision.String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Verification.VerifiedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off verification: %w", err)
	}
	return nil
}
