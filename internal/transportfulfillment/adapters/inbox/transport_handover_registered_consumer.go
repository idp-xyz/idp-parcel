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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是本上下文消费自己交出的信封的那一扇门（ADR-0216）：权威交接登记之后，派送发起在派发进程里随这封信逐对象跑一拍。
// 没有跨上下文翻译，处理方直接是应用编排；编排答出的处置（入账 / 重投 / 缺陷）在这里落成 inbox 的三种动作。

var (
	// ErrDeliveryDispatchOnHandoverUndecided 表示这一拍停在一个读得到的依赖上（交接还读不回、段登记册或某条派送要求缝
	// 此刻读不动、任务登记册读不动）。重投可能就过；生产装配把它登进 WithUndecidedSentinels。停在哪一口留在错误正文里。
	ErrDeliveryDispatchOnHandoverUndecided = errors.New(
		"transport fulfillment inbox: delivery dispatch on transport handover is undecided")
	// ErrDeliveryDispatchOnHandoverDefect 表示本上下文自己交出的键或命令过不了自己的门，或装配缺一条派送要求缝。重投不会
	// 变，禁止进 WithUndecidedSentinels（ADR-0029）：让它落发布失败，响给人看。
	ErrDeliveryDispatchOnHandoverDefect = errors.New(
		"transport fulfillment inbox: delivery dispatch on transport handover hit a defect of its own")
)

// transportHandoverDispatchConsumerName 是本消费者在 inbox 键上的稳定名。必须与 VE 交接投影那一路分家：同一封交接信封
// 扇出给两路，inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const transportHandoverDispatchConsumerName = "transport-fulfillment/trigger-delivery-dispatch-from-transport-handover"

// TransportHandoverRegisteredEventType 是本消费者认的事件类型：本上下文的权威交接登记。串在本包写出，不去拿提供方
// postgres 适配器里那个未导出常量；它与提供方交出的、与 VE 那一路认的是同一个串，三者相等由 cmd/parcel-dispatch 用生产
// 交接口入队的真库用例钉住。
const TransportHandoverRegisteredEventType eventing.EventType = "transport-fulfillment.transport-handover.registered"

// RegisteredTransportHandover 是译码后的交接判断键，四维都是登记册的键维。版本必须进译码：更正是新版本新登记，各有自己
// 的信封，少了版本就读不回是哪一版。
type RegisteredTransportHandover struct {
	TenantID string
	Object   string
	Scope    string
	Version  string
}

// DeliveryDispatchOnHandoverTrigger 是本消费者转交的那一口，真实装配接 application.TriggerDispatchOnHandoverHandler。
type DeliveryDispatchOnHandoverTrigger interface {
	Trigger(
		ctx context.Context,
		command application.TriggerDispatchOnHandoverCommand,
	) (application.TriggerDispatchOnHandoverResult, error)
}

// TransportHandoverRegisteredConsumer 把权威交接登记信封推进消费门，交给派送发起。
type TransportHandoverRegisteredConsumer struct {
	gate *inboxconsume.Gate[RegisteredTransportHandover]
}

func NewTransportHandoverRegisteredConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	trigger DeliveryDispatchOnHandoverTrigger,
) (*TransportHandoverRegisteredConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: inbox store is nil")
	}
	if trigger == nil {
		return nil, fmt.Errorf("transport fulfillment inbox: delivery dispatch trigger is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredTransportHandover]{
		Transactor: transactor,
		Store:      store,
		Name:       transportHandoverDispatchConsumerName,
		EventType:  TransportHandoverRegisteredEventType,
		Decode:     decodeRegisteredTransportHandover,
		Handle: func(ctx context.Context, registered RegisteredTransportHandover) error {
			return triggerDispatchOn(ctx, trigger, registered)
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "trigger delivery dispatch on registered transport handover",
	})
	if err != nil {
		return nil, err
	}
	return &TransportHandoverRegisteredConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *TransportHandoverRegisteredConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// triggerDispatchOn 跑一拍并按编排答出的处置落账。处理方与消费门在同一事务里：任务口的那一行与 inbox 的已处理同笔提交，
// 重投时消费门先认出已处理就停下，执行器不会再问一遍所有者。
func triggerDispatchOn(
	ctx context.Context,
	trigger DeliveryDispatchOnHandoverTrigger,
	registered RegisteredTransportHandover,
) error {
	tenant, err := domain.NewTenantID(registered.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrDeliveryDispatchOnHandoverDefect, err)
	}
	result, err := trigger.Trigger(ctx, application.TriggerDispatchOnHandoverCommand{
		TenantID: tenant,
		Object:   registered.Object,
		Scope:    registered.Scope,
		Version:  registered.Version,
	})
	if err != nil {
		return err
	}
	switch result.Disposition() {
	case application.DispatchOnHandoverSettled:
		return nil
	case application.DispatchOnHandoverRetry:
		return fmt.Errorf("%w: %s", ErrDeliveryDispatchOnHandoverUndecided, describeDispatchOnHandover(result))
	default:
		return fmt.Errorf("%w: %s", ErrDeliveryDispatchOnHandoverDefect, describeDispatchOnHandover(result))
	}
}

// describeDispatchOnHandover 把两层答复写成一句给运维读的话：停在本编排哪一口，或执行器答了哪一格、停在哪条缝、续办引用
// 是什么。失败码只说「未决」或「发布失败」，停在哪里全靠这一句。
func describeDispatchOnHandover(result application.TriggerDispatchOnHandoverResult) string {
	if result.Outcome() != application.DispatchOnHandoverTriggered {
		return fmt.Sprintf("%s %s", result.Outcome(), result.UndecidedReason())
	}
	triggered := result.Trigger()
	return fmt.Sprintf("%s %s %s %s", result.Outcome(), triggered.Outcome(), triggered.UndecidedReason(),
		triggered.ContinuationReference())
}

// decodeRegisteredTransportHandover 译载荷。四维缺一即毒丸：处理方按（租户 + 对象 + 范围 + 版本）读回交接，缺一维永远
// 取不着，重投同样内容也不会长出字段来。
func decodeRegisteredTransportHandover(payload []byte) (RegisteredTransportHandover, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Object   string `json:"object"`
		Scope    string `json:"scope"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredTransportHandover{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Object == "" || body.Scope == "" || body.Version == "" {
		return RegisteredTransportHandover{}, fmt.Errorf("%w: missing transport handover key fields", ErrPoisonEnvelope)
	}
	return RegisteredTransportHandover{
		TenantID: body.TenantID,
		Object:   body.Object,
		Scope:    body.Scope,
		Version:  body.Version,
	}, nil
}
