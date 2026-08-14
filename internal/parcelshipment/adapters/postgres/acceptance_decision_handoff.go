package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// acceptanceDecisionEventType 是接受决定意图的事件类型。
const acceptanceDecisionEventType = "parcel-shipment.acceptance-decision.formed"

// OutboxAcceptanceDecisionHandoff 把接受决定意图写进 Outbox，实现
// ports.AcceptanceDecisionHandoff。事务纪律与 OutboxSourceDataHandoff 同一条：
// 意图只可能与决定的落库同一事务提交，重发先查后插（同事务撞 23505 会把事务打进
// 中止态连带回滚业务写入）。
type OutboxAcceptanceDecisionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxAcceptanceDecisionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxAcceptanceDecisionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxAcceptanceDecisionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.AcceptanceDecisionHandoff = (*OutboxAcceptanceDecisionHandoff)(nil)

// acceptanceDecisionPayload 是意图载荷的传输形状：决定引用与决定后的生命周期状态，
// 不带校验明细或基线内容（下游按各自门禁重新读取与判断）。
type acceptanceDecisionPayload struct {
	TenantID          string `json:"tenantId"`
	CustomerAccountID string `json:"customerAccountId"`
	Source            string `json:"source"`
	SourceRequestKey  string `json:"sourceRequestKey"`
	ShipmentRequestID string `json:"shipmentRequestId"`
	SubmissionVersion string `json:"submissionVersion"`
	DecisionID        string `json:"decisionId"`
	State             string `json:"state"`
}

// HandOffAcceptanceDecision 把一份意图入队。信封 ID 取决定标识——意图由结果标识
// 认领（ADR-0043），重发同一份由 outboxintent.EnqueueOnce 的先查后插承担。
func (handoff *OutboxAcceptanceDecisionHandoff) HandOffAcceptanceDecision(
	ctx context.Context,
	intent ports.AcceptanceDecisionHandoffIntent,
) error {

	payload, err := json.Marshal(acceptanceDecisionPayload{
		TenantID:          intent.Identity.TenantID().String(),
		CustomerAccountID: intent.Identity.CustomerAccountID().String(),
		Source:            intent.Identity.Source().String(),
		SourceRequestKey:  intent.Identity.RequestKey().String(),
		ShipmentRequestID: intent.ShipmentRequestID.String(),
		SubmissionVersion: intent.SubmissionVersion.String(),
		DecisionID:        intent.DecisionID.String(),
		State:             intent.State.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off acceptance decision: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.DecisionID.String()),
		Source:       eventSource,
		Type:         acceptanceDecisionEventType,
		Version:      1,
		Scope:        intent.Identity.TenantID().String(),
		Subject:      intent.ShipmentRequestID.String(),
		PartitionKey: intent.Identity.TenantID().String() + "/" + intent.ShipmentRequestID.String(),
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off acceptance decision: %w", err)
	}
	return nil
}
