// Package psinbox 是 parcel-shipment 的入站事件消费适配器（ADR-0025：适配器在
// 消费方侧）。事务舞步交给 platform/inboxconsume；本包只声明消费者名、事件类型与
// 译码。生产路由登记留给 CONS-INTAKE-B（ADR-0049：接不住不登记）。
package psinbox

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

// consumerName 是本消费者在 inbox 键上的稳定名。改名等于换消费者。
const consumerName = "parcel-shipment/adopt-node-intake"

// NodeIntakeFormedEventType 是本消费者认的事件类型。消费方自己写出这个字符串，
// 不导入提供方 outbox 适配器的未导出常量——两边各写各的名字。
const NodeIntakeFormedEventType eventing.EventType = "node-operations.node-intake.formed"

// ErrPoisonEnvelope 表示信封解不出引用且重投同样内容不会改变结果。毒丸只管信封
// 结构；业务缺件（收寄还看不见、目标还没有）走处理方哨兵，由消费门回滚重投。
var ErrPoisonEnvelope = errors.New("parcel shipment inbox: poison envelope")

// FormedNodeIntake 是译码后的收寄键引用——只有引用，收寄本体由处理方按引用重新取。
type FormedNodeIntake struct {
	TenantID string
	SourceID string
}

// FormedNodeIntakeHandler 是本消费者转交的处理方。真实装配接 AdoptOnNodeIntakeAdapter。
type FormedNodeIntakeHandler interface {
	HandleFormedNodeIntake(ctx context.Context, formed FormedNodeIntake) error
}

// NodeIntakeConsumer 把 NO 节点收寄形成信封推进消费门。
type NodeIntakeConsumer struct {
	gate *inboxconsume.Gate[FormedNodeIntake]
}

func NewNodeIntakeConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedNodeIntakeHandler,
) (*NodeIntakeConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel shipment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedNodeIntake]{
		Transactor:     transactor,
		Store:          store,
		Name:           consumerName,
		EventType:      NodeIntakeFormedEventType,
		Decode:         decodeFormedNodeIntake,
		Handle:         handler.HandleFormedNodeIntake,
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "handle formed node intake",
	})
	if err != nil {
		return nil, err
	}
	return &NodeIntakeConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与 NR 两个消费者共用同一扇门。
func (consumer *NodeIntakeConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedNodeIntake 译载荷。tenantId 与 sourceId 缺一即毒丸——处理方按这两维
// 取回收寄记录，缺了永远取不着，而重投同样内容不会长出字段来。
func decodeFormedNodeIntake(payload []byte) (FormedNodeIntake, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		SourceID string `json:"sourceId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedNodeIntake{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.SourceID == "" {
		return FormedNodeIntake{}, fmt.Errorf("%w: missing reception key fields", ErrPoisonEnvelope)
	}
	return FormedNodeIntake{TenantID: body.TenantID, SourceID: body.SourceID}, nil
}
