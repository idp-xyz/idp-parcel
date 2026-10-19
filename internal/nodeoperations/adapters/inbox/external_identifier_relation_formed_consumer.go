// Package noinbox 是 node-operations 的入站事件消费适配器（ADR-0025：适配器在消费方侧）。事务舞步交给
// platform/inboxconsume；本包只声明消费者名、事件类型与译码。
package noinbox

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

// recheckPendingIdentitiesConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，同包再起
// 一个消费者要另取名字，否则两路互相把对方的投递当重复跳过。改名等于换消费者。自投的重核门铃那一扇门（UncheckedPendingIdentityRecordedConsumer）
// 用的就是这个名字：它是同一个重核消费者的另一扇门，信封来源是本上下文，与 PS 门铃的键不相交。
const recheckPendingIdentitiesConsumerName = "node-operations/recheck-pending-identities"

// ExternalIdentifierRelationFormedEventType 是 PS 外部标识关联册新形成一条标识包裹的关联时交出的门铃（ADR-0199 决定三）。消费方
// 自己写出这个字符串，不导入提供方 outbox 适配器的常量——两边各写各的名字，由组合根的一条对照用例钉住。
const ExternalIdentifierRelationFormedEventType eventing.EventType = "parcel-shipment.external-identifier-relation.formed"

// ErrPoisonEnvelope 表示门铃解不出租户或号码，重投同样内容不会改变结果。
var ErrPoisonEnvelope = errors.New("node operations inbox: poison envelope")

// FormedExternalIdentifierRelation 是译码后的门铃：只有租户与号码。门铃不是身份答案，不带包裹、分配方、标识类型或委托；
// 号码原样，不去空白——重核逐字比对来源文档里存下的号。
type FormedExternalIdentifierRelation struct {
	TenantID string
	Number   string
}

// FormedExternalIdentifierRelationHandler 是本消费者转交的处理方。真实装配接按号重核。
type FormedExternalIdentifierRelationHandler interface {
	HandleFormedExternalIdentifierRelation(ctx context.Context, formed FormedExternalIdentifierRelation) error
}

// ExternalIdentifierRelationFormedConsumer 把 PS 的门铃推进消费门。处理方交回错误（身份核对读不动等）整笔回滚，inbox 无账，
// 信封退避重投。
type ExternalIdentifierRelationFormedConsumer struct {
	gate *inboxconsume.Gate[FormedExternalIdentifierRelation]
}

func NewExternalIdentifierRelationFormedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedExternalIdentifierRelationHandler,
) (*ExternalIdentifierRelationFormedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("node operations inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("node operations inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedExternalIdentifierRelation]{
		Transactor:     transactor,
		Store:          store,
		Name:           recheckPendingIdentitiesConsumerName,
		EventType:      ExternalIdentifierRelationFormedEventType,
		Decode:         decodeFormedExternalIdentifierRelation,
		Handle:         handler.HandleFormedExternalIdentifierRelation,
		UnexpectedType: "node operations inbox",
		HandleVerb:     "recheck pending identities by the formed relation",
	})
	if err != nil {
		return nil, err
	}
	return &ExternalIdentifierRelationFormedConsumer{gate: gate}, nil
}

func (consumer *ExternalIdentifierRelationFormedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedExternalIdentifierRelation 译载荷。tenantId 与 number 缺一即毒丸：重核按这两维找待识别，缺了永远找不着。
func decodeFormedExternalIdentifierRelation(payload []byte) (FormedExternalIdentifierRelation, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Number   string `json:"number"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedExternalIdentifierRelation{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Number == "" {
		return FormedExternalIdentifierRelation{}, fmt.Errorf("%w: missing tenantId or number", ErrPoisonEnvelope)
	}
	return FormedExternalIdentifierRelation{TenantID: body.TenantID, Number: body.Number}, nil
}
