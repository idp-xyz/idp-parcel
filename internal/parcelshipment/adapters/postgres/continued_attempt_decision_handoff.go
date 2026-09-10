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

// ContinuedAttemptDecisionEventType 是「一条关闭 / 重开决定已落册，这件包裹值得判一次面单服务终局」那封
// 指针式信封的类型（票 label-channel/30 写、票 label-channel/27 消费）。两种决定共用它：一个处理方、一扇
// 消费门认一种类型；哪一种在载荷里只作追溯，消费者不据它分支（ADR-0134 决定五）。导出是给 27 的消费者
// 与 cmd/parcel-dispatch 路由表引同一个串，不在两处各抄一遍。
const ContinuedAttemptDecisionEventType = "parcel-shipment.continued-attempt-decision.judgment-due"

// OutboxContinuedAttemptDecisionHandoff 把`面单继续尝试决定`写侧交出的判断意图写入 Outbox，实现
// ports.ContinuedAttemptDecisionHandoff。入队一步由 outboxintent.EnqueueOnce 承担，事务从 ctx 取——与
// ContinuedAttemptRegisters.Insert / Save 同一个，事务由组合根的事务壳开（ADR-0134 决定三）。
type OutboxContinuedAttemptDecisionHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxContinuedAttemptDecisionHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxContinuedAttemptDecisionHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxContinuedAttemptDecisionHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ContinuedAttemptDecisionHandoff = (*OutboxContinuedAttemptDecisionHandoff)(nil)

// continuedAttemptDecisionPayload 是指针式载荷：只带消费者反查与追溯要的维，不带决定本体——判断读全册
// 且读当下，信封不是一份要被读回的事实版本（ADR-0134 决定一）。
type continuedAttemptDecisionPayload struct {
	TenantID string `json:"tenantId"`
	Parcel   string `json:"parcel"`
	Decision string `json:"decision"`
	Kind     string `json:"kind"`
}

// continuedAttemptDecisionEventID 取（租户 + 包裹 + 决定标识）加类型段。决定标识必须在里面：关过—重开—再关
// 三条决定指同一件包裹，少了它三封算出同一个字符串，EnqueueOnce 先查后插，后两封静默不入队。登记册键就是
// 租户 + 包裹，一封一决定即一封一包裹。
func continuedAttemptDecisionEventID(intent ports.ContinuedAttemptDecisionHandoffIntent) string {
	return intent.Tenant.String() + "/continued-attempt-decision/" + intent.Parcel.String() + "/" +
		intent.Decision.String() + "/judgment-due"
}

// HandOffContinuedAttemptDecision 把一份意图入队。意图残缺（无租户 / 包裹 / 决定标识、种类不是两格之一、
// 生效时间为零）是编排缺陷，响亮报错不入队。
func (handoff *OutboxContinuedAttemptDecisionHandoff) HandOffContinuedAttemptDecision(
	ctx context.Context,
	intent ports.ContinuedAttemptDecisionHandoffIntent,
) error {
	if intent.Tenant.String() == "" || intent.Parcel.String() == "" || intent.Decision.String() == "" {
		return fmt.Errorf("hand off continued attempt decision: tenant, parcel and decision are required")
	}
	if intent.Kind.String() == "" {
		return fmt.Errorf("hand off continued attempt decision: decision kind is required")
	}
	if intent.OccurredAt.IsZero() {
		return fmt.Errorf("hand off continued attempt decision: effective time is required")
	}

	payload, err := json.Marshal(continuedAttemptDecisionPayload{
		TenantID: intent.Tenant.String(),
		Parcel:   intent.Parcel.String(),
		Decision: intent.Decision.String(),
		Kind:     intent.Kind.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off continued attempt decision: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(continuedAttemptDecisionEventID(intent)),
		Source:      eventSource,
		Type:        ContinuedAttemptDecisionEventType,
		Version:     1,
		Scope:       intent.Tenant.String(),
		Subject:     intent.Parcel.String(),
		// 分区按租户加包裹排队：同一包裹的关闭、重开与面单交易的多拍要在一条队里先后消费（后一拍要读到
		// 前一拍已落的结果），与 OutboxLabelTransactionJudgmentHandoff / OutboxFinalOutcomeHandoff 同一选择。
		PartitionKey: intent.Tenant.String() + "/" + intent.Parcel.String(),
		OccurredAt:   intent.OccurredAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off continued attempt decision: %w", err)
	}
	return nil
}
