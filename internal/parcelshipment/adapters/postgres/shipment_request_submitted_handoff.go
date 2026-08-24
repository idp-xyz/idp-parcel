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

// shipmentRequestSubmittedEventType 是「委托已提交」意图的事件类型——简报「事件信封基线」
// 定义的首个 Parcel 自有事件。类型字面取本上下文事件家族的既有拍法
// `parcel-shipment.<主体>.<语义>`，不取简报表当年草拟的 `idp.parcel.*`：一个上下文只有
// 一套类型命名，Source 同理由 eventSource 单点承载，简报表已随实现更新。
const shipmentRequestSubmittedEventType = "parcel-shipment.shipment-request.submitted"

// OutboxShipmentRequestSubmittedHandoff 把「委托已提交」写入 Outbox，实现
// ports.ShipmentRequestSubmittedHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxShipmentRequestSubmittedHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxShipmentRequestSubmittedHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxShipmentRequestSubmittedHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxShipmentRequestSubmittedHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.ShipmentRequestSubmittedHandoff = (*OutboxShipmentRequestSubmittedHandoff)(nil)

// shipmentRequestSubmittedPayload 是意图载荷的传输形状：只有标识与关联——委托、批次、
// 当前提交版本、成员声明包裹，以及据以重放查询的来源身份四维。地址、联系人、货物与
// 申报明文不进信封（简报「事件信封基线」）；它们本就不在聚合里，这个形状让它们此后
// 也进不来。
type shipmentRequestSubmittedPayload struct {
	TenantID            string   `json:"tenantId"`
	CustomerAccountID   string   `json:"customerAccountId"`
	Source              string   `json:"source"`
	SourceRequestKey    string   `json:"sourceRequestKey"`
	ShipmentRequestID   string   `json:"shipmentRequestId"`
	SubmissionBatchID   string   `json:"submissionBatchId"`
	SubmissionVersionID string   `json:"submissionVersionId"`
	DeclaredParcelIDs   []string `json:"declaredParcelIds"`
}

// shipmentRequestSubmittedEventID 由来源身份（含租户）加类型段认领意图（ADR-0043）。
// 同一来源身份至多建立一份委托，重复请求因此复用原 EventID，不造语义相同的第二个事件
// ——「EventID 在首次命令处理中生成并与业务结果一起保存」由身份即键结构性兑现：身份
// 就存在业务行上，重放按同一身份重导出同一个 ID。
func shipmentRequestSubmittedEventID(identity domain.SourceIdentity) string {
	return identity.TenantID().String() + "/" +
		identity.CustomerAccountID().String() + "/" +
		identity.Source().String() + "/" +
		identity.RequestKey().String() + "/shipment-request-submitted"
}

// HandOffShipmentRequestSubmitted 把一份意图入队。身份或委托缺席是装配缺陷，响亮报错
// 不入队。
func (handoff *OutboxShipmentRequestSubmittedHandoff) HandOffShipmentRequestSubmitted(
	ctx context.Context,
	intent ports.ShipmentRequestSubmittedHandoffIntent,
) error {
	identity := intent.Identity
	if identity.TenantID().String() == "" ||
		identity.CustomerAccountID().String() == "" ||
		identity.Source().String() == "" ||
		identity.RequestKey().String() == "" {
		return fmt.Errorf("hand off shipment request submitted: source identity is required")
	}
	request := intent.Request
	if request.ShipmentRequestID().String() == "" {
		return fmt.Errorf("hand off shipment request submitted: shipment request is required")
	}

	version := request.CurrentSubmissionVersion()
	parcels := version.DeclaredParcelIDs()
	declared := make([]string, 0, len(parcels))
	for _, parcel := range parcels {
		declared = append(declared, parcel.String())
	}
	payload, err := json.Marshal(shipmentRequestSubmittedPayload{
		TenantID:            identity.TenantID().String(),
		CustomerAccountID:   identity.CustomerAccountID().String(),
		Source:              identity.Source().String(),
		SourceRequestKey:    identity.RequestKey().String(),
		ShipmentRequestID:   request.ShipmentRequestID().String(),
		SubmissionBatchID:   request.BatchID().String(),
		SubmissionVersionID: version.VersionID().String(),
		DeclaredParcelIDs:   declared,
	})
	if err != nil {
		return fmt.Errorf("hand off shipment request submitted: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(shipmentRequestSubmittedEventID(identity)),
		Source:      eventSource,
		Type:        shipmentRequestSubmittedEventType,
		Version:     1,
		// Scope 带租户与客户账户两维（简报「事件信封基线」）：委托是货主客户的委托，
		// 消费方的可见性边界在客户账户，不止租户。
		Scope:   identity.TenantID().String() + "/" + identity.CustomerAccountID().String(),
		Subject: request.ShipmentRequestID().String(),
		// 分区按（租户/客户账户/委托）排队：同一份委托的先后事件在一条队里，不同委托
		// 互不阻塞——「每份委托独立提交」（简报「事务边界」）在投递侧的对应物。
		PartitionKey: identity.TenantID().String() + "/" +
			identity.CustomerAccountID().String() + "/" +
			request.ShipmentRequestID().String(),
		// 领域发生时间与记录时间分别填写（简报「事件信封基线」）：提交时刻是领域事实，
		// 入队时刻是本机读数，不得用后者覆盖前者。
		OccurredAt:  request.SubmittedAt().UTC(),
		RecordedAt:  now,
		ContentType: eventing.JSONContentType,
		Payload:     payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off shipment request submitted: %w", err)
	}
	return nil
}
