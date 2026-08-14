package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// etaEventType 是 ETA 预测版本意图的事件类型。
const etaEventType = "visibility-exception.eta-prediction.formed"

// OutboxETAHandoff 把预测版本写入 Outbox，实现 ports.ETAHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxETAHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxETAHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxETAHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("visibility exception postgres: clock is nil")
	}
	return &OutboxETAHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ETAHandoff = (*OutboxETAHandoff)(nil)

// etaPayload 是意图载荷的传输形状：只有下游 FindCurrent 所需的租户/包裹/里程碑，
// 外加版本标识认领，不带区间或模型明细。
type etaPayload struct {
	TenantID  string `json:"tenantId"`
	Parcel    string `json:"parcel"`
	Milestone string `json:"milestone"`
	VersionID string `json:"versionId"`
}

// HandOffETA 把一份意图入队。信封 ID 取预测版本——意图由预测版本认领（ADR-0043）。
// 租户空白是装配缺陷，响亮报错不入队。
func (handoff *OutboxETAHandoff) HandOffETA(
	ctx context.Context,
	intent ports.ETAHandoffIntent,
) error {
	if intent.TenantID.String() == "" || intent.Prediction.Version().String() == "" {
		return fmt.Errorf("hand off eta: tenant and prediction version are required")
	}

	payload, err := json.Marshal(etaPayload{
		TenantID:  intent.TenantID.String(),
		Parcel:    intent.Prediction.Parcel().String(),
		Milestone: intent.Prediction.Milestone().String(),
		VersionID: intent.Prediction.Version().String(),
	})
	if err != nil {
		return fmt.Errorf("hand off eta: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.Prediction.Version().String()),
		Source:       veEventSource,
		Type:         etaEventType,
		Version:      1,
		Scope:        intent.TenantID.String(),
		Subject:      intent.Prediction.Version().String(),
		PartitionKey: intent.TenantID.String() + "/" + intent.Prediction.Parcel().String(),
		OccurredAt:   intent.Prediction.PredictedAt().UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off eta: %w", err)
	}
	return nil
}
