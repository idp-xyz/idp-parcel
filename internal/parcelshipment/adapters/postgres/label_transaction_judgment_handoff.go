package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// labelTransactionJudgmentEventType 是「这件包裹值得判一次面单服务终局」那封指针式信封的类型。
// 定案与后续动作两拍共用它（ADR-0134 决定四）：一个处理方、一扇消费门认一种类型；哪一拍在
// 载荷里只作追溯，消费者不据它分支。
const labelTransactionJudgmentEventType = "parcel-shipment.label-transaction.judgment-due"

// OutboxLabelTransactionJudgmentHandoff 把面单交易写侧交出的判断意图写入 Outbox，实现
// ports.LabelTransactionJudgmentHandoff。入队一步由 outboxintent.EnqueueOnce 承担，事务从 ctx 取
// ——与 LabelTransactions.Save 同一个，事务由组合根的事务壳开（ADR-0134 决定三）。
type OutboxLabelTransactionJudgmentHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxLabelTransactionJudgmentHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxLabelTransactionJudgmentHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxLabelTransactionJudgmentHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.LabelTransactionJudgmentHandoff = (*OutboxLabelTransactionJudgmentHandoff)(nil)

// labelTransactionJudgmentPayload 是指针式载荷：只带消费者反查与追溯要的维，不带交易本体——
// 判断读全册且读当下，信封不是一份要被读回的事实版本（ADR-0134 决定一）。
type labelTransactionJudgmentPayload struct {
	TenantID    string `json:"tenantId"`
	Transaction string `json:"transaction"`
	Parcel      string `json:"parcel"`
	Revision    int64  `json:"revision"`
	Beat        string `json:"beat"`
}

// labelTransactionJudgmentEventID 取（租户 + 交易 + 包裹 + 版本）加类型段。版本必须在里面：定案那拍
// 与作废那拍指同一笔交易、同一件包裹，少了版本两封算出同一个字符串，EnqueueOnce 先查后插，
// 第二封静默不入队（同 TF externalTrackingFactEventID 头注那条理由）。包裹也必须在里面：一封一包裹。
func labelTransactionJudgmentEventID(intent ports.LabelTransactionJudgmentIntent) string {
	return intent.Tenant.String() + "/label-transaction/" + intent.TransactionID.String() + "/" +
		intent.Parcel.String() + "/" + strconv.FormatInt(intent.Revision, 10) + "/judgment-due"
}

// HandOffLabelTransactionJudgment 把一份意图入队。意图残缺（无租户 / 交易 / 包裹、版本未持久化、
// 不是两拍之一）是编排缺陷，响亮报错不入队。
func (handoff *OutboxLabelTransactionJudgmentHandoff) HandOffLabelTransactionJudgment(
	ctx context.Context,
	intent ports.LabelTransactionJudgmentIntent,
) error {
	if intent.Tenant.String() == "" || intent.TransactionID.String() == "" || intent.Parcel.String() == "" {
		return fmt.Errorf("hand off label transaction judgment: tenant, transaction and parcel are required")
	}
	if intent.Revision < 1 {
		return fmt.Errorf("hand off label transaction judgment: revision %d is not a persisted generation", intent.Revision)
	}
	if intent.Beat.String() == "" {
		return fmt.Errorf("hand off label transaction judgment: beat is required")
	}

	payload, err := json.Marshal(labelTransactionJudgmentPayload{
		TenantID:    intent.Tenant.String(),
		Transaction: intent.TransactionID.String(),
		Parcel:      intent.Parcel.String(),
		Revision:    intent.Revision,
		Beat:        intent.Beat.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off label transaction judgment: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(labelTransactionJudgmentEventID(intent)),
		Source:      eventSource,
		Type:        labelTransactionJudgmentEventType,
		Version:     1,
		Scope:       intent.Tenant.String(),
		Subject:     intent.Parcel.String(),
		// 分区按租户加包裹排队，不跟着信封 ID 走：ID 含交易与版本（两拍、多笔交易各占一个 ID，
		// 因而不丢），而顺序要的是同一包裹的先后拍在一条队里——两笔交易先后定案、定案与作废
		// 先后到达，后一拍要读到前一拍已落的结果。与 OutboxFinalOutcomeHandoff 同一选择，让判断
		// 意图与它形成的终局排在同一条队上。
		PartitionKey: intent.Tenant.String() + "/" + intent.Parcel.String(),
		OccurredAt:   intent.OccurredAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off label transaction judgment: %w", err)
	}
	return nil
}
