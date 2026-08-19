package veinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// initialRouteConsumerName 是本消费者在 inbox 键上的稳定名。必须与交付、揽收、交接、
// 收寄各账分家：Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路
// 的投递当重复跳过。改名等于换消费者。
const initialRouteConsumerName = "visibility-exception/derive-projection-from-initial-route"

// InitialRouteFormedEventType 是本消费者认的事件类型：NR 的包裹级初始路由判断交接。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 一个类型走两支结论：计划已形成与无当前路由共用这一个事件类型，分支在按键取回的
// 本体上（HasPlan/HasNoRoute），不在信封上——信封只做唤醒指针。
const InitialRouteFormedEventType eventing.EventType = "network-routing.initial-route.formed"

// FormedInitialRoute 是译码后的初始路由判断键引用。权威事实留在 network-routing；
// 处理方按这六维 FindByKey 取回本体。载荷里的 correlation 故意不读：那是交接关联，
// 不是判断键维。
type FormedInitialRoute struct {
	TenantID          string
	CustomerAccountID string
	Shipment          string
	Parcel            string
	Baseline          string
	Purpose           string
}

// FormedInitialRouteHandler 是本消费者转交的处理方。真实装配接
// DeriveOnInitialRouteAdapter。
type FormedInitialRouteHandler interface {
	HandleFormedInitialRoute(ctx context.Context, formed FormedInitialRoute) error
}

// InitialRouteConsumer 把 NR 初始路由判断信封推进消费门。
type InitialRouteConsumer struct {
	gate *inboxconsume.Gate[FormedInitialRoute]
}

func NewInitialRouteConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedInitialRouteHandler,
) (*InitialRouteConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedInitialRoute]{
		Transactor:     transactor,
		Store:          store,
		Name:           initialRouteConsumerName,
		EventType:      InitialRouteFormedEventType,
		Decode:         decodeFormedInitialRoute,
		Handle:         handler.HandleFormedInitialRoute,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle formed initial route",
	})
	if err != nil {
		return nil, err
	}
	return &InitialRouteConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *InitialRouteConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedInitialRoute 译载荷。六维缺一即毒丸——处理方按含 customerAccountId 的
// 完整判断键取回本体，缺任何一维永远取不着，而重投同样内容不会长出字段来。毒丸哨兵
// 复用本包已有的 ErrPoisonEnvelope。
func decodeFormedInitialRoute(payload []byte) (FormedInitialRoute, error) {
	var body struct {
		TenantID          string `json:"tenantId"`
		CustomerAccountID string `json:"customerAccountId"`
		Shipment          string `json:"shipment"`
		Parcel            string `json:"parcel"`
		Baseline          string `json:"baseline"`
		Purpose           string `json:"purpose"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedInitialRoute{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.CustomerAccountID == "" || body.Shipment == "" ||
		body.Parcel == "" || body.Baseline == "" || body.Purpose == "" {
		return FormedInitialRoute{}, fmt.Errorf("%w: missing initial route key fields", ErrPoisonEnvelope)
	}
	return FormedInitialRoute{
		TenantID:          body.TenantID,
		CustomerAccountID: body.CustomerAccountID,
		Shipment:          body.Shipment,
		Parcel:            body.Parcel,
		Baseline:          body.Baseline,
		Purpose:           body.Purpose,
	}, nil
}
