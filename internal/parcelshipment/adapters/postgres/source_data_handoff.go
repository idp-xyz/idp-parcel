package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// eventSource 是本上下文在信封 Source 位上的稳定名。消费方按它认来源，改名等于换
// 了一个来源，在途意图会与新名分家。
const eventSource = "idp-parcel/parcel-shipment"

// bentoSchemaOutboxTable 是意图查重读的框架技术表。表名耦合是先查后插的代价，由
// 真库测试守着（表名变化时查询与测试一起红，不会静默漂移）。
const bentoSchemaOutboxTable = migrate.SchemaBento + ".outbox"

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
// 信封 ID 取版本标识——意图由结果标识认领（ADR-0043）。重发同一份（重放路径）先按
// 标识查已入队：查到即成功返回——AT-PS-031 要的「仅重试同一发布意图」正是这一格。
// 先查后插而不是撞唯一约束翻译：同一事务里撞 23505 会把事务打进中止态，随后的提交
// 一律失败，调用方在同事务里的业务写入会被连带回滚（真库实跑抓出，与聚合库 Insert
// 用 ON CONFLICT 是同一个病的两种解法）。并发首发的竞态窗口仍由唯一约束兜底——
// 那一格撞上时本次事务确实该重试，语义无损。
func (handoff *OutboxSourceDataHandoff) HandOffSourceDataVersion(
	ctx context.Context,
	intent ports.SourceDataVersionHandoffIntent,
) error {
	enqueued, err := handoff.alreadyEnqueued(ctx, intent.Version.String())
	if err != nil {
		return fmt.Errorf("hand off source data version: %w", err)
	}
	if enqueued {
		return nil
	}
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

	if err := handoff.store.Enqueue(ctx, envelope); err != nil {
		return fmt.Errorf("hand off source data version: %w", err)
	}
	return nil
}

// alreadyEnqueued 在同一事务里按（来源+事件标识）查该意图是否已入队。
//
// 它读的是框架技术表——表名耦合是本方法的代价，由真库测试守着（表名变化时这里
// 与测试一起红，不会静默漂移）；框架 Store 今天没有查询口，开了再换。
func (handoff *OutboxSourceDataHandoff) alreadyEnqueued(
	ctx context.Context,
	eventID string,
) (bool, error) {
	executor, err := handoff.db.RequireExecutor(ctx)
	if err != nil {
		return false, err
	}
	var exists bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM `+bentoSchemaOutboxTable+` WHERE source = $1 AND event_id = $2)`,
		eventSource, eventID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
