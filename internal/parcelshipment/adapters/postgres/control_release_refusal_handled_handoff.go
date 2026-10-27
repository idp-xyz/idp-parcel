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

// controlReleaseRefusalHandledEventType 是「释放拒收已处理」意图的事件类型（ADR-0293 决定四）。与消费侧（adapters/inbox）各写各的字面，
// 理由同「新提交版本已形成」那对：两边在不同包，导入对方的常量等于让消费方依赖提供方的内部形状。两串对不上由 cmd/parcel-dispatch 的
// 真链往返用例当场揪红。
const controlReleaseRefusalHandledEventType = "parcel-shipment.shipment-request.control-release-refusal-handled"

// OutboxControlReleaseRefusalHandledHandoff 把「释放拒收已处理」写入 Outbox，实现 ports.ControlReleaseRefusalHandledHandoff。入队一步由
// outboxintent.EnqueueOnce 承担。
type OutboxControlReleaseRefusalHandledHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxControlReleaseRefusalHandledHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxControlReleaseRefusalHandledHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxControlReleaseRefusalHandledHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ControlReleaseRefusalHandledHandoff = (*OutboxControlReleaseRefusalHandledHandoff)(nil)

// controlReleaseRefusalHandledPayload 的头七个字段与「复核已完成」「新提交版本已形成」同名同义：消费门要译的是同一个推进命令，几封信
// 各造一套字段名，消费侧就得维护几份译码而它们必须永远同义。提交版本那一维填委托**此刻的当前版本**——续办要的是链拿当前版本重驱，
// 被拒收的那一版多半已被取代。后两个字段只说明这一封因哪一笔占用的处理而发，消费门不读；处理人与依据不进信封（留痕在处理表里）。
type controlReleaseRefusalHandledPayload struct {
	TenantID            string   `json:"tenantId"`
	CustomerAccountID   string   `json:"customerAccountId"`
	Source              string   `json:"source"`
	SourceRequestKey    string   `json:"sourceRequestKey"`
	ShipmentRequestID   string   `json:"shipmentRequestId"`
	SubmissionVersionID string   `json:"submissionVersionId"`
	DeclaredParcelIDs   []string `json:"declaredParcelIds"`

	HandledSubmissionVersionID string `json:"handledSubmissionVersionId"`
	HandledControlResultID     string `json:"handledControlResultId"`
}

// controlReleaseRefusalHandledEventIDPort 是本口指纹 ID 的口名前缀。
const controlReleaseRefusalHandledEventIDPort = "control-release-refusal-handled"

// controlReleaseRefusalHandledEventID 由来源身份加那一笔占用的身份（委托、被拒收的版本、控制结果）认领意图（ADR-0043）：一笔占用至多
// 被处理一次，重放因此重发同一份；同一委托上另一笔占用的处理是另一封信，不被当成重放吞掉。
func controlReleaseRefusalHandledEventID(identity domain.SourceIdentity, key ports.ControlReleaseRefusalKey) string {
	return string(outboxintent.FingerprintEventID(controlReleaseRefusalHandledEventIDPort,
		identity.TenantID().String(), identity.CustomerAccountID().String(), identity.Source().String(), identity.RequestKey().String(),
		key.ShipmentRequestID.String(), key.SubmissionVersion.String(), key.ControlResultID.String()))
}

// HandOffControlReleaseRefusalHandled 把一份意图入队。身份、委托、处理留痕缺席，或留痕的租户与来源身份、委托对不上，都是装配缺陷，响亮报错
// 不入队：一封指向别的租户或别的委托的续办信封，会让接受链去重驱一份与这次处理无关的委托。
func (handoff *OutboxControlReleaseRefusalHandledHandoff) HandOffControlReleaseRefusalHandled(
	ctx context.Context,
	intent ports.ControlReleaseRefusalHandledHandoffIntent,
) error {
	identity := intent.Identity
	if identity.TenantID().String() == "" ||
		identity.CustomerAccountID().String() == "" ||
		identity.Source().String() == "" ||
		identity.RequestKey().String() == "" {
		return fmt.Errorf("hand off control release refusal handled: source identity is required")
	}
	request := intent.Request
	if request.ShipmentRequestID().String() == "" {
		return fmt.Errorf("hand off control release refusal handled: shipment request is required")
	}
	key := intent.Handling.Key
	if intent.Handling.Kind.String() == "" || key.ControlResultID.String() == "" || key.SubmissionVersion.String() == "" {
		return fmt.Errorf("hand off control release refusal handled: handling is required")
	}
	if key.TenantID != identity.TenantID() || key.ShipmentRequestID != request.ShipmentRequestID() {
		return fmt.Errorf("hand off control release refusal handled: handling %s/%s does not belong to request %s/%s",
			key.TenantID, key.ShipmentRequestID, identity.TenantID(), request.ShipmentRequestID())
	}

	version := request.CurrentSubmissionVersion()
	parcels := version.DeclaredParcelIDs()
	declared := make([]string, 0, len(parcels))
	for _, parcel := range parcels {
		declared = append(declared, parcel.String())
	}
	payload, err := json.Marshal(controlReleaseRefusalHandledPayload{
		TenantID:                   identity.TenantID().String(),
		CustomerAccountID:          identity.CustomerAccountID().String(),
		Source:                     identity.Source().String(),
		SourceRequestKey:           identity.RequestKey().String(),
		ShipmentRequestID:          request.ShipmentRequestID().String(),
		SubmissionVersionID:        version.VersionID().String(),
		DeclaredParcelIDs:          declared,
		HandledSubmissionVersionID: key.SubmissionVersion.String(),
		HandledControlResultID:     key.ControlResultID.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off control release refusal handled: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(controlReleaseRefusalHandledEventID(identity, key)),
		Source:      eventSource,
		Type:        controlReleaseRefusalHandledEventType,
		Version:     1,
		Scope:       identity.TenantID().String() + "/" + identity.CustomerAccountID().String(),
		Subject:     request.ShipmentRequestID().String(),
		// 分区与「委托已提交」「复核已完成」「新提交版本已形成」同键：同一份委托的提交与各路续办信封排同一条队，续办不会越过一封还没
		// 投出去的提交。
		PartitionKey: identity.TenantID().String() + "/" +
			identity.CustomerAccountID().String() + "/" +
			request.ShipmentRequestID().String(),
		// 领域发生时间是处理成立的时刻，不是入队时刻。
		OccurredAt:  intent.Handling.HandledAt.UTC(),
		RecordedAt:  now,
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off control release refusal handled: %w", err)
	}
	return nil
}
