// Package tfinbox 是 transport-fulfillment 的入向消费门：把别的上下文交来的信封推进 inbox 舞步，译码后交给消费侧
// 适配器。本包只认信封形状，不读任何提供方的库；按键回读、翻译与判断都在处理方。
package tfinbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// ErrPoisonEnvelope 标记译不出引用的信封：重投同样内容不会改变结果，消费门拒收入账。
var ErrPoisonEnvelope = errors.New("transport fulfillment inbox: poison envelope")

// initialRouteFormedConsumerName 是本消费者在 inbox 键上的稳定名。必须与 VE 那一路分家：同一封初始路由信封扇出给两路，
// inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const initialRouteFormedConsumerName = "transport-fulfillment/prepare-opportunity-from-initial-route"

// InitialRouteFormedEventType 是本消费者认的事件类型：NR 包裹级初始路由判断的交接（ADR-0201 决定七）。字符串由消费方
// 自己写出，不 import 提供方（ADR-0025）；与 VE 那一路认的是同一个串，两串相等由 cmd/parcel-dispatch 用生产交接口入队的
// 真库用例钉住。计划已形成与无当前路由共用这一个类型，分支在按键取回的本体上。
const InitialRouteFormedEventType eventing.EventType = "network-routing.initial-route.formed"

// FormedInitialRoute 是译码后的初始路由判断键，六维都是 NR 的拼写。载荷里的 correlation 故意不读：那是交接关联，
// 不是判断键维。
type FormedInitialRoute struct {
	TenantID          string
	CustomerAccountID string
	Shipment          string
	Parcel            string
	Baseline          string
	Purpose           string
}

// FormedInitialRouteHandler 是本消费者转交的处理方。真实装配接 adapters/networkrouting 的 PrepareOnInitialRoute。
type FormedInitialRouteHandler interface {
	HandleFormedInitialRoute(ctx context.Context, formed FormedInitialRoute) error
}

// InitialRouteFormedConsumer 把初始路由判断信封推进消费门。
type InitialRouteFormedConsumer struct {
	gate *inboxconsume.Gate[FormedInitialRoute]
}

func NewInitialRouteFormedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedInitialRouteHandler,
) (*InitialRouteFormedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedInitialRoute]{
		Transactor:     transactor,
		Store:          store,
		Name:           initialRouteFormedConsumerName,
		EventType:      InitialRouteFormedEventType,
		Decode:         decodeFormedInitialRoute,
		Handle:         handler.HandleFormedInitialRoute,
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "prepare transport opportunity on formed initial route",
	})
	if err != nil {
		return nil, err
	}
	return &InitialRouteFormedConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *InitialRouteFormedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedInitialRoute 译载荷。六维缺一即毒丸：处理方按含客户账户的完整判断键回读，缺一维永远取不着，重投同样
// 内容也不会长出字段来。
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
