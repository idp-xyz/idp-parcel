package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
)

// manualReviewCompletedEventType 是「复核已完成」意图的事件类型（ADR-0086 Decision 二）。
// 与消费侧（adapters/inbox）各写各的字面：两边在不同包，导入对方的常量等于让消费方依赖
// 提供方的内部形状。两串对不上由 cmd/parcel-api 的铸封译码用例当场揪红。
const manualReviewCompletedEventType = "parcel-shipment.shipment-request.manual-review-completed"

// OutboxManualReviewCompletedHandoff 把「复核已完成」写入 Outbox，实现
// ports.ManualReviewCompletedHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxManualReviewCompletedHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxManualReviewCompletedHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxManualReviewCompletedHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxManualReviewCompletedHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ManualReviewCompletedHandoff = (*OutboxManualReviewCompletedHandoff)(nil)

// manualReviewCompletedPayload 是意图载荷的传输形状。字段与「委托已提交」同名同义（批次
// 除外——续办一拍不重放批次语义）：消费门要译的是同一个推进命令，两封信各造一套字段名，
// 消费侧就得维护两份译码而它们必须永远同义。复核证据引用不进信封：续办要的是「完成了」，
// 完成的凭据留在聚合上由读面交代。
type manualReviewCompletedPayload struct {
	TenantID            string   `json:"tenantId"`
	CustomerAccountID   string   `json:"customerAccountId"`
	Source              string   `json:"source"`
	SourceRequestKey    string   `json:"sourceRequestKey"`
	ShipmentRequestID   string   `json:"shipmentRequestId"`
	SubmissionVersionID string   `json:"submissionVersionId"`
	DeclaredParcelIDs   []string `json:"declaredParcelIds"`
}

// manualReviewCompletedEventID 由来源身份加提交版本认领意图（ADR-0043）：完成挂在版本的
// 判断任务上，同一版本至多一次`已记录`（领域对重复完成答`已有完成`，走不到交接），新提交
// 版本换代后的再复核是另一份完成、另一个 ID。
func manualReviewCompletedEventID(identity domain.SourceIdentity, version domain.SubmissionVersionID) string {
	return identity.TenantID().String() + "/" +
		identity.CustomerAccountID().String() + "/" +
		identity.Source().String() + "/" +
		identity.RequestKey().String() + "/" +
		version.String() + "/manual-review-completed"
}

// HandOffManualReviewCompleted 把一份意图入队。身份、委托或完成留痕缺席是装配缺陷——
// 这个口只该在完成刚落库的那一格被调（reviewCompletionBoundary），聚合上没有完成就到了
// 这里，说明边界壳接错了地方，响亮报错不入队。
func (handoff *OutboxManualReviewCompletedHandoff) HandOffManualReviewCompleted(
	ctx context.Context,
	intent ports.ManualReviewCompletedHandoffIntent,
) error {
	identity := intent.Identity
	if identity.TenantID().String() == "" ||
		identity.CustomerAccountID().String() == "" ||
		identity.Source().String() == "" ||
		identity.RequestKey().String() == "" {
		return fmt.Errorf("hand off manual review completed: source identity is required")
	}
	request := intent.Request
	if request.ShipmentRequestID().String() == "" {
		return fmt.Errorf("hand off manual review completed: shipment request is required")
	}
	completion, done := request.AcceptanceDecisionTask().ManualReviewCompletion()
	if !done {
		return fmt.Errorf("hand off manual review completed: request carries no review completion")
	}

	version := request.CurrentSubmissionVersion()
	parcels := version.DeclaredParcelIDs()
	declared := make([]string, 0, len(parcels))
	for _, parcel := range parcels {
		declared = append(declared, parcel.String())
	}
	payload, err := json.Marshal(manualReviewCompletedPayload{
		TenantID:            identity.TenantID().String(),
		CustomerAccountID:   identity.CustomerAccountID().String(),
		Source:              identity.Source().String(),
		SourceRequestKey:    identity.RequestKey().String(),
		ShipmentRequestID:   request.ShipmentRequestID().String(),
		SubmissionVersionID: version.VersionID().String(),
		DeclaredParcelIDs:   declared,
	})
	if err != nil {
		return fmt.Errorf("hand off manual review completed: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(manualReviewCompletedEventID(identity, version.VersionID())),
		Source:      eventSource,
		Type:        manualReviewCompletedEventType,
		Version:     1,
		Scope:       identity.TenantID().String() + "/" + identity.CustomerAccountID().String(),
		Subject:     request.ShipmentRequestID().String(),
		// 分区与「委托已提交」同键（租户/客户账户/委托）：同一份委托的提交与续办信封
		// 排同一条队，续办不会越过一封还没投出去的提交。
		PartitionKey: identity.TenantID().String() + "/" +
			identity.CustomerAccountID().String() + "/" +
			request.ShipmentRequestID().String(),
		// 领域发生时间是复核完成时刻，不是入队时刻——两者分开与「委托已提交」同一条纪律。
		OccurredAt:  completion.CompletedAt().UTC(),
		RecordedAt:  now,
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off manual review completed: %w", err)
	}
	return nil
}
