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

// submissionVersionFormedEventType 是「新提交版本已形成」意图的事件类型（ADR-0106 Decision 三）。
// 与消费侧（adapters/inbox）各写各的字面，理由同复核完成那对：两边在不同包，导入对方的常量等于
// 让消费方依赖提供方的内部形状。两串对不上由 cmd/parcel-dispatch 的真链往返用例当场揪红。
const submissionVersionFormedEventType = "parcel-shipment.shipment-request.submission-version-formed"

// OutboxSubmissionVersionFormedHandoff 把「新提交版本已形成」写入 Outbox，实现
// ports.SubmissionVersionFormedHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxSubmissionVersionFormedHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSubmissionVersionFormedHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxSubmissionVersionFormedHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxSubmissionVersionFormedHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SubmissionVersionFormedHandoff = (*OutboxSubmissionVersionFormedHandoff)(nil)

// submissionVersionFormedPayload 与「复核已完成」同名同义：消费门要译的是同一个推进命令，
// 两封信各造一套字段名，消费侧就得维护两份译码而它们必须永远同义。提交版本那维填**新**版本
// ——续办要的是链拿新版本的任务重跑，旧版本的任务已随入账留在库里。
type submissionVersionFormedPayload struct {
	TenantID            string   `json:"tenantId"`
	CustomerAccountID   string   `json:"customerAccountId"`
	Source              string   `json:"source"`
	SourceRequestKey    string   `json:"sourceRequestKey"`
	ShipmentRequestID   string   `json:"shipmentRequestId"`
	SubmissionVersionID string   `json:"submissionVersionId"`
	DeclaredParcelIDs   []string `json:"declaredParcelIds"`
}

// submissionVersionFormedEventID 由来源身份加新提交版本认领意图（ADR-0043）：同一版本至多形成
// 一次（同一补充身份的重放由编排答`已处理`，走不到交接），重放不入队第二份。
func submissionVersionFormedEventID(identity domain.SourceIdentity, version domain.SubmissionVersionID) string {
	return identity.TenantID().String() + "/" +
		identity.CustomerAccountID().String() + "/" +
		identity.Source().String() + "/" +
		identity.RequestKey().String() + "/" +
		version.String() + "/submission-version-formed"
}

// HandOffSubmissionVersionFormed 把一份意图入队。身份或委托缺席、当前版本没有前版都是装配缺陷
// ——这个口只该在新版本刚落库的那一格被调（受控补充的事务边界壳），聚合的当前版本仍是首版
// 就到了这里，说明边界壳接在了首次提交上而不是补充上，响亮报错不入队。
func (handoff *OutboxSubmissionVersionFormedHandoff) HandOffSubmissionVersionFormed(
	ctx context.Context,
	intent ports.SubmissionVersionFormedHandoffIntent,
) error {
	identity := intent.Identity
	if identity.TenantID().String() == "" ||
		identity.CustomerAccountID().String() == "" ||
		identity.Source().String() == "" ||
		identity.RequestKey().String() == "" {
		return fmt.Errorf("hand off submission version formed: source identity is required")
	}
	request := intent.Request
	if request.ShipmentRequestID().String() == "" {
		return fmt.Errorf("hand off submission version formed: shipment request is required")
	}
	if len(request.PriorSubmissionVersions()) == 0 {
		return fmt.Errorf("hand off submission version formed: request carries no superseded version")
	}

	version := request.CurrentSubmissionVersion()
	parcels := version.DeclaredParcelIDs()
	declared := make([]string, 0, len(parcels))
	for _, parcel := range parcels {
		declared = append(declared, parcel.String())
	}
	payload, err := json.Marshal(submissionVersionFormedPayload{
		TenantID:            identity.TenantID().String(),
		CustomerAccountID:   identity.CustomerAccountID().String(),
		Source:              identity.Source().String(),
		SourceRequestKey:    identity.RequestKey().String(),
		ShipmentRequestID:   request.ShipmentRequestID().String(),
		SubmissionVersionID: version.VersionID().String(),
		DeclaredParcelIDs:   declared,
	})
	if err != nil {
		return fmt.Errorf("hand off submission version formed: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(submissionVersionFormedEventID(identity, version.VersionID())),
		Source:      eventSource,
		Type:        submissionVersionFormedEventType,
		Version:     1,
		Scope:       identity.TenantID().String() + "/" + identity.CustomerAccountID().String(),
		Subject:     request.ShipmentRequestID().String(),
		// 分区与「委托已提交」「复核已完成」同键：同一份委托的提交与各路续办信封排同一条队，
		// 续办不会越过一封还没投出去的提交。
		PartitionKey: identity.TenantID().String() + "/" +
			identity.CustomerAccountID().String() + "/" +
			request.ShipmentRequestID().String(),
		// 领域发生时间是新版本成立的时刻，不是入队时刻。
		OccurredAt:  version.EstablishedAt().UTC(),
		RecordedAt:  now,
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off submission version formed: %w", err)
	}
	return nil
}
