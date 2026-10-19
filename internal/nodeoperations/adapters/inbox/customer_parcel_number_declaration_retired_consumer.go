package noinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// CustomerParcelNumberDeclarationRetiredEventType 是 PS 在一份委托由`已提交`转为`已拒绝`或`已撤回`时，按它声明的每个客户包裹件号
// 交出的门铃（ADR-0211 决定三）。消费方自己写出这个字符串，不导入提供方 outbox 适配器的常量，同关联已形成那一个；两串由组合根
// 的一条对照用例钉住。
const CustomerParcelNumberDeclarationRetiredEventType eventing.EventType = "parcel-shipment.customer-parcel-number-declaration.retired"

// RetiredCustomerParcelNumberDeclaration 是译码后的门铃：只有租户与号码。门铃不是身份答案，不说哪份委托、为什么撤下；号码原样，
// 不去空白——重核逐字比对来源文档里存下的号。
type RetiredCustomerParcelNumberDeclaration struct {
	TenantID string
	Number   string
}

// RetiredCustomerParcelNumberDeclarationHandler 是本消费门转交的处理方。真实装配接按号重核。
type RetiredCustomerParcelNumberDeclarationHandler interface {
	HandleRetiredCustomerParcelNumberDeclaration(ctx context.Context, retired RetiredCustomerParcelNumberDeclaration) error
}

// CustomerParcelNumberDeclarationRetiredConsumer 是重核消费者的又一扇门，与关联已形成那一扇同用一个消费者名（ADR-0211 决定三「NO
// 用已有的重核消费者接」）。同名不会互相顶替：inbox 键由（消费者名，来源，事件 ID）认领，PS 两只口的信封 ID 各带口名前缀。
// 处理方交回错误（身份核对读不动等）整笔回滚，inbox 无账，信封退避重投。
type CustomerParcelNumberDeclarationRetiredConsumer struct {
	gate *inboxconsume.Gate[RetiredCustomerParcelNumberDeclaration]
}

func NewCustomerParcelNumberDeclarationRetiredConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RetiredCustomerParcelNumberDeclarationHandler,
) (*CustomerParcelNumberDeclarationRetiredConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("node operations inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("node operations inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RetiredCustomerParcelNumberDeclaration]{
		Transactor:     transactor,
		Store:          store,
		Name:           recheckPendingIdentitiesConsumerName,
		EventType:      CustomerParcelNumberDeclarationRetiredEventType,
		Decode:         decodeRetiredCustomerParcelNumberDeclaration,
		Handle:         handler.HandleRetiredCustomerParcelNumberDeclaration,
		UnexpectedType: "node operations inbox",
		HandleVerb:     "recheck pending identities by the retired declaration",
	})
	if err != nil {
		return nil, err
	}
	return &CustomerParcelNumberDeclarationRetiredConsumer{gate: gate}, nil
}

func (consumer *CustomerParcelNumberDeclarationRetiredConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

func decodeRetiredCustomerParcelNumberDeclaration(payload []byte) (RetiredCustomerParcelNumberDeclaration, error) {
	tenant, number, err := decodeTenantAndNumber(payload)
	if err != nil {
		return RetiredCustomerParcelNumberDeclaration{}, err
	}
	return RetiredCustomerParcelNumberDeclaration{TenantID: tenant, Number: number}, nil
}

// decodeTenantAndNumber 译 PS 两种门铃共同的载荷：tenantId 与 number 缺一即毒丸——重核按这两维找待识别，缺了永远找不着。
func decodeTenantAndNumber(payload []byte) (string, string, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Number   string `json:"number"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Number == "" {
		return "", "", fmt.Errorf("%w: missing tenantId or number", ErrPoisonEnvelope)
	}
	return body.TenantID, body.Number, nil
}
