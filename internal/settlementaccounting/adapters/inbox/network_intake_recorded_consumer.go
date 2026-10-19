package sainbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// networkIntakeRecordedConsumerName 是本消费者在 inbox 键上的稳定名。同一封收寄采用信封还投给 network-routing 的
// 路由复核（扇出），两路各记各的 inbox 账；共名会让一路把另一路的投递当重复跳过。
const networkIntakeRecordedConsumerName = "settlement-accounting/request-sell-evaluation-on-network-intake"

// NetworkIntakeRecordedEventType 是本消费者认的事件类型：parcel-shipment 落下一份有效网络收寄采用判断（采用或不采用）
// 后交出的信封。消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量；两串是否相等由 cmd/parcel-dispatch
// 的真库装配用例钉——那里用提供方的真适配器入队、经生产路由表投递。
const NetworkIntakeRecordedEventType eventing.EventType = "parcel-shipment.network-intake.recorded"

// RecordedNetworkIntake 是译码后的采用键引用，恰是提供方 FindByKey 的四维。载荷上随复核请求携带的地理投影本消费者
// 不收：计费时点只问「采用了没有、责任起点在哪一刻」，两样都按键回读采用记录，不信封上抄。
type RecordedNetworkIntake struct {
	TenantID string
	Parcel   string
	Kind     string
	Version  string
}

// RecordedNetworkIntakeHandler 是本消费者转交的处理方。真实装配接 adapters/parcelshipment 的
// RequestOnNetworkIntakeAdapter。
type RecordedNetworkIntakeHandler interface {
	HandleRecordedNetworkIntake(ctx context.Context, recorded RecordedNetworkIntake) error
}

// NetworkIntakeRecordedConsumer 把 PS 有效网络收寄采用信封推进消费门（ADR-0190 决定二「有效网络收寄已采用」）。
type NetworkIntakeRecordedConsumer struct {
	gate *inboxconsume.Gate[RecordedNetworkIntake]
}

func NewNetworkIntakeRecordedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RecordedNetworkIntakeHandler,
) (*NetworkIntakeRecordedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("settlement accounting inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RecordedNetworkIntake]{
		Transactor:     transactor,
		Store:          store,
		Name:           networkIntakeRecordedConsumerName,
		EventType:      NetworkIntakeRecordedEventType,
		Decode:         decodeRecordedNetworkIntake,
		Handle:         handler.HandleRecordedNetworkIntake,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "handle recorded network intake",
	})
	if err != nil {
		return nil, err
	}
	return &NetworkIntakeRecordedConsumer{gate: gate}, nil
}

func (consumer *NetworkIntakeRecordedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRecordedNetworkIntake 译载荷。四维缺一即毒丸：处理方按整键回读采用记录，缺一维永远取不着那一行，重投同样
// 内容也不会长出字段来。来源类型与版本尤其不给缺省——猜一个就会把计费挂在另一份收寄事实上。
func decodeRecordedNetworkIntake(payload []byte) (RecordedNetworkIntake, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Parcel   string `json:"parcel"`
		Kind     string `json:"kind"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RecordedNetworkIntake{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.Kind == "" || body.Version == "" {
		return RecordedNetworkIntake{}, fmt.Errorf("%w: missing intake adoption key fields", ErrPoisonEnvelope)
	}
	return RecordedNetworkIntake{
		TenantID: body.TenantID,
		Parcel:   body.Parcel,
		Kind:     body.Kind,
		Version:  body.Version,
	}, nil
}
