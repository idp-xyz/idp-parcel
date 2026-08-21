package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

const offsitePickupRegistrationEventType = "transport-fulfillment.offsite-pickup.registered"

// OutboxOffsitePickupRegistrationHandoff 把对象级揽收登记写入 Outbox，实现
// ports.OffsitePickupRegistrationHandoff。入队一步由 outboxintent.EnqueueOnce 承担。
type OutboxOffsitePickupRegistrationHandoff struct {
	db    *bentopg.DB
	store *outbox.Store
	clock ports.Clock
}

func NewOutboxOffsitePickupRegistrationHandoff(
	db *bentopg.DB,
	store *outbox.Store,
	clock ports.Clock,
) (*OutboxOffsitePickupRegistrationHandoff, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: outbox store is nil")
	}
	if clock == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: clock is nil")
	}
	return &OutboxOffsitePickupRegistrationHandoff{db: db, store: store, clock: clock}, nil
}

var _ ports.OffsitePickupRegistrationHandoff = (*OutboxOffsitePickupRegistrationHandoff)(nil)

type offsitePickupRegistrationPayload struct {
	TenantID string `json:"tenantId"`
	Object   string `json:"object"`
	Attempt  string `json:"attempt"`
}

func offsitePickupRegistrationEventID(key ports.OffsitePickupKey) string {
	// 类型段把本口与已落地的交付生效（同为租户/对象/尝试、同一 source）错开，避免
	// EnqueueOnce 把另一口的已入队当成「同一份」。
	return key.TenantID.String() + "/" + key.Object.String() + "/" + key.Attempt.String() +
		"/offsite-pickup-registration"
}

// offsitePickupRegistrationPartitionKey 取（租户+载运对象+类型段），不取整个信封 ID。
//
// ID 管幂等（每次尝试一份意图，第二次成功因而不丢），分区键管顺序（同一对象的先后拍排队）。
// 把 ID 直接当分区键会让每次尝试自成一个分区，框架的顺序保证于是落空。
//
// 主体取到对象而不取到（对象+尝试）：同一载运对象在前一段履约参与结束后可以由新的尝试再次
// 形成揽收成功（退运再出、召回后再揽收是常态，TF CONTEXT 的跨段接续句），两次成功是同一条
// 对象控制链的先后两段。取到尝试就把链切成互不排队的两段，而下游 parcel-shipment 的来源采用
// 正是逐对象判断的。
//
// 类型段留着，不与 transportHandoverPartitionKey、effectiveDeliveryPartitionKey 合流。那两口
// 取的是光秃秃的（租户+对象），而 visibility-exception 的投影、triage、gap、eta 四口取的是
// （租户+包裹）——载运对象引用与申报包裹标识是同一个字符串，两边因此落进同一分区。揽收登记
// 一旦并进去，一封未决的揽收就把同一包裹的追踪投影堵在分区头，而那份投影 VE 已经受理并派生，
// 堵它只是把已经成立的可见性扣到失败预算烧完。跨口保序也换不来别的：投影的取代关系由来源给出
// （ADR-0065），本就不靠到达先后。TF 对象分区与 VE 包裹分区共键那一类另有票，不在本口就地解决。
func offsitePickupRegistrationPartitionKey(key ports.OffsitePickupKey) string {
	return key.TenantID.String() + "/" + key.Object.String() + "/offsite-pickup-registration"
}

// HandOffOffsitePickupRegistration 把一份意图入队。信封 ID 取对象级揽收幂等键再加类型
// 段——意图由（租户+对象+尝试）认领（ADR-0043），类型段与交付生效错开。键缺席是装配
// 缺陷，响亮报错不入队。
func (handoff *OutboxOffsitePickupRegistrationHandoff) HandOffOffsitePickupRegistration(
	ctx context.Context,
	intent ports.OffsitePickupRegistrationIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Object.String() == "" || key.Attempt.String() == "" {
		return fmt.Errorf("hand off offsite pickup registration: pickup key is required")
	}

	payload, err := json.Marshal(offsitePickupRegistrationPayload{
		TenantID: key.TenantID.String(),
		Object:   key.Object.String(),
		Attempt:  key.Attempt.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off offsite pickup registration: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := offsitePickupRegistrationEventID(key)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       tfEventSource,
		Type:         offsitePickupRegistrationEventType,
		Version:      1,
		Scope:        key.TenantID.String(),
		Subject:      key.Object.String() + "/" + key.Attempt.String(),
		PartitionKey: offsitePickupRegistrationPartitionKey(key),
		OccurredAt:   intent.Record.RecordedAt.UTC(),
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := outboxintent.EnqueueOnce(ctx, handoff.db, handoff.store, envelope); err != nil {
		return fmt.Errorf("hand off offsite pickup registration: %w", err)
	}
	return nil
}
