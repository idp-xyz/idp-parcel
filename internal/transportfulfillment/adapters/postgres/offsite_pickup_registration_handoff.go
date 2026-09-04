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

// offsitePickupRegistrationPayload 带键再带结果版本。载荷仍是指针式的——只带引用，揽收本体由下游按
// 引用重新读。版本是引用维不是装饰：一次揽收在库里是一行一版本（0015），键只指到「这次尝试的揽收」，
// 指不到「哪一代」；下游今天按键读当前版，有了这一格才分得出手里这封信说的是哪一代。
type offsitePickupRegistrationPayload struct {
	TenantID      string `json:"tenantId"`
	Object        string `json:"object"`
	Attempt       string `json:"attempt"`
	PickupVersion string `json:"pickupVersion"`
}

// offsitePickupRegistrationEventID 取揽收登记键，**更正版本再加版本段**。
//
// ADR-0043 说意图由结果标识认领，而更正的结果标识是它的新版本：更正走的是同一个键，ID 少了版本两代
// 就算出同一个字符串，outboxintent.EnqueueOnce 先查后插——第二份静默不入队，编排却收到「交接成功」
// （effectiveDeliveryEventID 点名过的同一个洞）。首登 ID 不带版本段：它自 ADR-0043 起就是这个形状，
// 下游消费者与 cmd/parcel-dispatch 的对账都按它认，首登与更正在 ID 上因此分得开、首登一字不动。
// 类型段把本口与交付生效（同为租户/对象/尝试、同一 source）错开。
func offsitePickupRegistrationEventID(record ports.OffsitePickupRecord) string {
	key := record.Key
	prefix := key.TenantID.String() + "/" + key.Object.String() + "/" + key.Attempt.String()
	if _, corrected := record.Pickup.Corrects(); corrected {
		prefix += "/" + record.Pickup.Version().String()
	}
	return prefix + "/offsite-pickup-registration"
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
// 类型段自 ADR-0074 起是 TF 对象链三口的统一形状（交接登记、交付生效同款），不再是本口
// 独有的避让：TF 的排队主体是载运对象，与 visibility-exception 的（租户+包裹）分区不共队。
// 载运对象引用与申报包裹标识是同一个字符串，不加类型段，一封未决的揽收就把同一包裹已受理
// 派生的追踪投影堵在分区头，把已经成立的可见性扣到失败预算烧完。跨口保序也换不来别的：
// 投影的取代关系由来源给出（ADR-0065），本就不靠到达先后。
func offsitePickupRegistrationPartitionKey(key ports.OffsitePickupKey) string {
	return key.TenantID.String() + "/" + key.Object.String() + "/offsite-pickup-registration"
}

// HandOffOffsitePickupRegistration 把一份意图入队。信封 ID 取对象级揽收幂等键再加类型段——意图由
// （租户+对象+尝试）认领（ADR-0043），更正版本再加版本段让两代各自入队；载荷带版本让下游读得回
// 自己那一代。键或版本缺席是装配缺陷，响亮报错不入队。
func (handoff *OutboxOffsitePickupRegistrationHandoff) HandOffOffsitePickupRegistration(
	ctx context.Context,
	intent ports.OffsitePickupRegistrationIntent,
) error {
	key := intent.Record.Key
	if key.TenantID.String() == "" || key.Object.String() == "" || key.Attempt.String() == "" {
		return fmt.Errorf("hand off offsite pickup registration: pickup key is required")
	}
	version := intent.Record.Pickup.Version()
	if version.String() == "" {
		// 没有版本就分不出首登与更正：更正版本的 ID 会退化成首登那一串、第二份被静默吞掉。
		return fmt.Errorf("hand off offsite pickup registration: pickup result version is required")
	}

	payload, err := json.Marshal(offsitePickupRegistrationPayload{
		TenantID:      key.TenantID.String(),
		Object:        key.Object.String(),
		Attempt:       key.Attempt.String(),
		PickupVersion: version.String(),
	})
	if err != nil {
		return fmt.Errorf("hand off offsite pickup registration: %w", err)
	}

	now := handoff.clock.Now().UTC()
	eventID := offsitePickupRegistrationEventID(intent.Record)
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
