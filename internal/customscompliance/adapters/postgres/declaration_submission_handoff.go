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

// declarationSubmissionEventType 是申报提交版本意图的事件类型。
const declarationSubmissionEventType = "customs-compliance.declaration-submission.formed"

// OutboxDeclarationSubmissionHandoff 把提交版本写入 Outbox，实现
// ports.DeclarationSubmissionHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxDeclarationSubmissionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxDeclarationSubmissionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxDeclarationSubmissionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("customs compliance postgres: clock is nil")
	}
	return &OutboxDeclarationSubmissionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.DeclarationSubmissionHandoff = (*OutboxDeclarationSubmissionHandoff)(nil)

// declarationSubmissionPayload 是意图载荷的传输形状：只有下游 FindByKey 所需的幂等
// 键三维，不带组成快照或发送尝试明细。
type declarationSubmissionPayload struct {
	TenantID  string `json:"tenantId"`
	UnitID    string `json:"unitId"`
	Procedure string `json:"procedure"`
	VersionID string `json:"versionId"`
}

func declarationSubmissionEventID(key ports.DeclarationSubmissionKey) string {
	return key.TenantID.String() + "/" + key.Unit.String() + "/" + key.Procedure.String()
}

// HandOffDeclarationSubmission 把一份意图入队。信封 ID 取幂等键——意图由幂等键认领
// （ADR-0043）。键缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxDeclarationSubmissionHandoff) HandOffDeclarationSubmission(
	ctx context.Context,
	intent ports.DeclarationSubmissionHandoffIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Unit.String() == "" || key.Procedure.String() == "" {
		return fmt.Errorf("hand off declaration submission: receive key is required")
	}

	payload, err := json.Marshal(declarationSubmissionPayload{
		TenantID:  key.TenantID.String(),
		UnitID:    key.Unit.String(),
		Procedure: key.Procedure.String(),
		VersionID: intent.Record.Version.ID().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off declaration submission: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := declarationSubmissionEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       ccEventSource,
		Type:         declarationSubmissionEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      intent.Record.Version.ID().String(),
		PartitionKey: eventID,
		OccurredAt:   intent.Record.Version.FixedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off declaration submission: %w", err)
	}
	return nil
}
