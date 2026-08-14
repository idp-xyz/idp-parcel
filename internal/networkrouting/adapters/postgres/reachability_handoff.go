package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// nrEventSource 是本上下文在信封 Source 位上的稳定名。
const nrEventSource = "idp-parcel/network-routing"

// reachabilityJudgmentEventType 是三值判断意图的事件类型。
const reachabilityJudgmentEventType = "network-routing.reachability-judgment.formed"

// OutboxReachabilityHandoff 把可达性判断意图写进 Outbox，实现
// ports.ReachabilityJudgmentHandoff——发布意图缝的第三个真实现，入队一步由
// platform/outboxintent 承担（rule-of-three 后的提炼点）。
type OutboxReachabilityHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxReachabilityHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxReachabilityHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("network routing postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("network routing postgres: clock is nil")
	}
	return &OutboxReachabilityHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ReachabilityJudgmentHandoff = (*OutboxReachabilityHandoff)(nil)

// reachabilityJudgmentPayload 是意图载荷的传输形状：判断引用与三值结论，不带候选
// 明细（下游要读证据去查判断库）。
type reachabilityJudgmentPayload struct {
	TenantID     string `json:"tenantId"`
	Correlation  string `json:"correlation"`
	Purpose      string `json:"purpose"`
	Value        string `json:"value"`
	JudgedAt     string `json:"judgedAt"`
	ViewRevision string `json:"viewRevision"`
}

// HandOffReachabilityJudgment 把一份意图入队。信封 ID 取请求关联——「同一判断无论
// 交几次都是同一份」正是端口注释里由请求关联认领那句的落库形态。
func (handoff *OutboxReachabilityHandoff) HandOffReachabilityJudgment(
	ctx context.Context,
	intent ports.ReachabilityJudgmentHandoffIntent,
) error {
	payload, err := json.Marshal(reachabilityJudgmentPayload{
		TenantID:     intent.Key.TenantID.String(),
		Correlation:  intent.Correlation.String(),
		Purpose:      intent.Key.ServicePurpose.String(),
		Value:        intent.Finding.Value().String(),
		JudgedAt:     intent.JudgedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		ViewRevision: intent.ViewRevision.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off reachability judgment: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(intent.Correlation.String()),
		Source:       nrEventSource,
		Type:         reachabilityJudgmentEventType,
		Version:      1,
		Scope:        intent.Key.TenantID.String(),
		Subject:      intent.Correlation.String(),
		PartitionKey: intent.Key.TenantID.String() + "/" + intent.Correlation.String(),
		OccurredAt:   intent.JudgedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off reachability judgment: %w", err)
	}
	return nil
}
