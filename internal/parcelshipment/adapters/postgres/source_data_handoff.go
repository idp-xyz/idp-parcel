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

// eventSource 是本上下文在信封 Source 位上的稳定名。消费方按它认来源，改名等于换
// 了一个来源，在途意图会与新名分家。
const eventSource = "idp-parcel/parcel-shipment"

// sourceDataVersionEventType 是资料版本意图的事件类型。类型携带语义版本位（信封
// Version 字段），载荷形状变化时升版本位而不是换类型名。
const sourceDataVersionEventType = "parcel-shipment.source-data-version.formed"

// OutboxSourceDataHandoff 把资料版本意图写进 Outbox，实现 ports.SourceDataVersionHandoff。
//
// 它是「发布意图与业务结果同一提交」的第一个真实实现：Enqueue 走 RequireExecutor，
// 无事务时直接报错而不是悄悄用连接池——意图只可能与调用方同一事务落库，事务回滚时
// 意图一并消失，不存在「版本没保住而下游已被通知」的分岔。投递到消费方的时点由
// Outbox 的租约派发承担，不在本适配器的责任里。
type OutboxSourceDataHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxSourceDataHandoff(db *bentopg.DB, store *outbox.Store, clock ports.Clock) (*OutboxSourceDataHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("parcel shipment postgres: clock is nil")
	}
	return &OutboxSourceDataHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.SourceDataVersionHandoff = (*OutboxSourceDataHandoff)(nil)

// sourceDataVersionPayload 是意图载荷的传输形状。只有引用与范围，没有资料内容——
// 跨上下文只传引用（UC-PS-002 步骤 9），下游按各自门禁重新读取。范围三件展开而不是
// 拼一个字符串：拼串是发明第二套范围编码，读方还得再解析一遍。
type sourceDataVersionPayload struct {
	TenantID          string `json:"tenantId"`
	CustomerAccountID string `json:"customerAccountId"`
	Source            string `json:"source"`
	SourceRequestKey  string `json:"sourceRequestKey"`
	Version           string `json:"version"`
	ShipmentRequestID string `json:"shipmentRequestId"`
	DeclaredParcelID  string `json:"declaredParcelId,omitempty"`
	DataGroup         string `json:"dataGroup"`
	AdoptionOutcome   string `json:"adoptionOutcome"`
	AdoptedVersion    string `json:"adoptedVersion,omitempty"`
}

// HandOffSourceDataVersion 把一份意图入队。
//
// 信封 ID 取版本标识——意图由结果标识认领（ADR-0043），AT-PS-031 要的「仅重试同一
// 发布意图」由 outboxintent.EnqueueOnce 的先查后插承担（三个适配器同形后提炼的
// 平台件，取舍见其包注释）。
func (handoff *OutboxSourceDataHandoff) HandOffSourceDataVersion(
	ctx context.Context,
	intent ports.SourceDataVersionHandoffIntent,
) error {
	body := sourceDataVersionPayload{
		TenantID:          intent.Identity.TenantID().String(),
		CustomerAccountID: intent.Identity.CustomerAccountID().String(),
		Source:            intent.Identity.Source().String(),
		SourceRequestKey:  intent.Identity.RequestKey().String(),
		Version:           intent.Version.String(),
		ShipmentRequestID: intent.Scope.ShipmentRequestID().String(),
		DataGroup:         intent.Scope.DataGroup().String(),
		AdoptionOutcome:   intent.Adoption.Outcome().String(),
	}
	if parcel, scoped := intent.Scope.DeclaredParcelID(); scoped {
		body.DeclaredParcelID = parcel.String()
	}
	if adopted, present := intent.Adoption.AdoptedVersion(); present {
		body.AdoptedVersion = adopted.String()
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("hand off source data version: %w", err)
	}

	now := handoff.clock.Now().UTC()
	envelope := eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID(intent.Version.String()),
		Source:      eventSource,
		Type:        sourceDataVersionEventType,
		Version:     1,
		Scope:       intent.Identity.TenantID().String(),
		Subject:     intent.Identity.RequestKey().String(),
		// 分区按租户加来源请求排队：同一份委托的意图保持发生序，不同委托互不阻塞。
		PartitionKey: intent.Identity.TenantID().String() + "/" + intent.Identity.RequestKey().String(),
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off source data version: %w", err)
	}
	return nil
}
