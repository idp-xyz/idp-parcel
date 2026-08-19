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

// finalOutcomeConsumerName 是本消费者在 inbox 键上的稳定名。必须与交接、交付、揽收、
// 节点收寄那几本账分家：Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路
// 把另一路的投递当重复跳过。改名等于换消费者。
const finalOutcomeConsumerName = "visibility-exception/derive-projection-from-final-outcome"

// FinalOutcomeFormedEventType 是本消费者认的事件类型：PS 的包裹服务终局判断。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 这是四路里唯一一条来源为 parcel-shipment 的投影：终局是 PS 自家拥有的事实，不是
// 履约侧事实。不认 `effective-delivery.registered`——那是终局的**上游来源**，已有自己
// 的投影账；把两者都当终局会让同一次交付在追踪上出现两个终局。
const FinalOutcomeFormedEventType eventing.EventType = "parcel-shipment.final-outcome.formed"

// FormedFinalOutcome 是译码后的终局采用键引用。权威事实留在 parcel-shipment；处理方
// 按这四维 FindByKey。Kind 与 Version 都是键的一维：同一包裹可有不同责任结果种类的
// 独立终局流，同一种类的来源更正又换版本，缺任何一维都取不回唯一那条登记。
type FormedFinalOutcome struct {
	TenantID string
	Parcel   string
	Kind     string
	Version  string
}

// FormedFinalOutcomeHandler 是本消费者转交的处理方。真实装配接
// DeriveOnFinalOutcomeAdapter。
type FormedFinalOutcomeHandler interface {
	HandleFormedFinalOutcome(ctx context.Context, formed FormedFinalOutcome) error
}

// FinalOutcomeConsumer 把 PS 终局判断信封推进消费门。
type FinalOutcomeConsumer struct {
	gate *inboxconsume.Gate[FormedFinalOutcome]
}

func NewFinalOutcomeConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedFinalOutcomeHandler,
) (*FinalOutcomeConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedFinalOutcome]{
		Transactor:     transactor,
		Store:          store,
		Name:           finalOutcomeConsumerName,
		EventType:      FinalOutcomeFormedEventType,
		Decode:         decodeFormedFinalOutcome,
		Handle:         handler.HandleFormedFinalOutcome,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle formed final outcome",
	})
	if err != nil {
		return nil, err
	}
	return &FinalOutcomeConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *FinalOutcomeConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedFinalOutcome 译载荷。四维缺一即毒丸——处理方按（租户+包裹+责任结果种类
// +版本）取回登记，缺了永远取不着，而重投同样内容不会长出字段来。毒丸哨兵复用本包
// 已有的 ErrPoisonEnvelope。
func decodeFormedFinalOutcome(payload []byte) (FormedFinalOutcome, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Parcel   string `json:"parcel"`
		Kind     string `json:"kind"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedFinalOutcome{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.Kind == "" || body.Version == "" {
		return FormedFinalOutcome{}, fmt.Errorf("%w: missing final outcome key fields", ErrPoisonEnvelope)
	}
	return FormedFinalOutcome{
		TenantID: body.TenantID,
		Parcel:   body.Parcel,
		Kind:     body.Kind,
		Version:  body.Version,
	}, nil
}
