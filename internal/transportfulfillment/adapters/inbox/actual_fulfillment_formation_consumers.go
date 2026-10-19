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

// 本文件是本上下文消费自己交出的交接与有效交付两种信封、自动形成实际履约发生项的两扇门（ADR-0218）。没有跨上下文翻译，
// 处理方直接是应用编排；编排答出的处置（入账 / 重投 / 缺陷）在这里落成 inbox 的三种动作。两扇门共用一个编排、一对哨兵，
// 消费者名各自分家。

var (
	// ErrActualFulfillmentFormationUndecided 表示这一次停在一个读得到的依赖上（交接还读不回、段登记册、采用连线、旅程册、
	// 委托、协议核对口或发生项登记册此刻读不动）。重投可能就过；生产装配把它登进两路各自的未决名单。停在哪一口留在错误正文里。
	ErrActualFulfillmentFormationUndecided = errors.New(
		"transport fulfillment inbox: actual fulfillment occurrence formation is undecided")
	// ErrActualFulfillmentFormationDefect 表示本上下文自己交出的键或命令过不了自己的门，或登记用例答出本路不该见到的一格。
	// 重投不会变，禁止进任何未决名单（ADR-0029）：让它落发布失败，响给人看。
	ErrActualFulfillmentFormationDefect = errors.New(
		"transport fulfillment inbox: actual fulfillment occurrence formation hit a defect of its own")
)

// 两个消费者名各自分家：交接信封同时扇出给 VE 投影、派送发起与本路，有效交付信封同时扇出给 VE 投影、PS 终局与本路；inbox
// 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const (
	actualFulfillmentOnHandoverConsumerName = "transport-fulfillment/form-actual-fulfillment-occurrence-from-transport-handover"
	actualFulfillmentOnDeliveryConsumerName = "transport-fulfillment/form-actual-fulfillment-occurrence-from-effective-delivery"
)

// EffectiveDeliveryRegisteredEventType 是有效交付登记信封的类型。串在本包写出，不去拿提供方 postgres 适配器里那个未导出
// 常量；它与提供方交出的、与 VE、PS 两路认的是同一个串，相等由 cmd/parcel-dispatch 用生产交付口入队的真库用例钉住。
const EffectiveDeliveryRegisteredEventType eventing.EventType = "transport-fulfillment.effective-delivery.registered"

// ActualFulfillmentFormer 是两扇门转交的那一口，真实装配接 application.FormActualFulfillmentOnParticipationEndHandler。
type ActualFulfillmentFormer interface {
	OnHandover(ctx context.Context, command application.FormActualFulfillmentOnHandoverCommand) (application.FormActualFulfillmentResult, error)
	OnDelivery(ctx context.Context, command application.FormActualFulfillmentOnDeliveryCommand) (application.FormActualFulfillmentResult, error)
}

// ActualFulfillmentOnHandoverConsumer 把权威交接登记信封推进消费门，交给实际履约发生项的自动形成。
type ActualFulfillmentOnHandoverConsumer struct {
	gate *inboxconsume.Gate[RegisteredTransportHandover]
}

func NewActualFulfillmentOnHandoverConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	former ActualFulfillmentFormer,
) (*ActualFulfillmentOnHandoverConsumer, error) {
	if err := formationConsumerDeps(transactor, store, former); err != nil {
		return nil, err
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredTransportHandover]{
		Transactor: transactor,
		Store:      store,
		Name:       actualFulfillmentOnHandoverConsumerName,
		EventType:  TransportHandoverRegisteredEventType,
		Decode:     decodeRegisteredTransportHandover,
		Handle: func(ctx context.Context, registered RegisteredTransportHandover) error {
			tenant, err := domain.NewTenantID(registered.TenantID)
			if err != nil {
				return fmt.Errorf("%w: tenant: %v", ErrActualFulfillmentFormationDefect, err)
			}
			return settleFormation(former.OnHandover(ctx, application.FormActualFulfillmentOnHandoverCommand{
				TenantID: tenant,
				Object:   registered.Object,
				Scope:    registered.Scope,
				Version:  registered.Version,
			}))
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "form actual fulfillment occurrence on registered transport handover",
	})
	if err != nil {
		return nil, err
	}
	return &ActualFulfillmentOnHandoverConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ActualFulfillmentOnHandoverConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// RegisteredEffectiveDelivery 是译码后的有效交付判断键：登记册的三维键外加这一封所指的那一版。
type RegisteredEffectiveDelivery struct {
	TenantID string
	Object   string
	Attempt  string
	Version  string
}

// ActualFulfillmentOnDeliveryConsumer 把有效交付登记信封推进消费门，交给实际履约发生项的自动形成。
type ActualFulfillmentOnDeliveryConsumer struct {
	gate *inboxconsume.Gate[RegisteredEffectiveDelivery]
}

func NewActualFulfillmentOnDeliveryConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	former ActualFulfillmentFormer,
) (*ActualFulfillmentOnDeliveryConsumer, error) {
	if err := formationConsumerDeps(transactor, store, former); err != nil {
		return nil, err
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredEffectiveDelivery]{
		Transactor: transactor,
		Store:      store,
		Name:       actualFulfillmentOnDeliveryConsumerName,
		EventType:  EffectiveDeliveryRegisteredEventType,
		Decode:     decodeRegisteredEffectiveDelivery,
		Handle: func(ctx context.Context, registered RegisteredEffectiveDelivery) error {
			tenant, err := domain.NewTenantID(registered.TenantID)
			if err != nil {
				return fmt.Errorf("%w: tenant: %v", ErrActualFulfillmentFormationDefect, err)
			}
			return settleFormation(former.OnDelivery(ctx, application.FormActualFulfillmentOnDeliveryCommand{
				TenantID: tenant,
				Object:   registered.Object,
				Attempt:  registered.Attempt,
				Version:  registered.Version,
			}))
		},
		UnexpectedType: "transport fulfillment inbox",
		HandleVerb:     "form actual fulfillment occurrence on registered effective delivery",
	})
	if err != nil {
		return nil, err
	}
	return &ActualFulfillmentOnDeliveryConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ActualFulfillmentOnDeliveryConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

func formationConsumerDeps(transactor bentoapp.Transactor, store *inbox.Store, former ActualFulfillmentFormer) error {
	if transactor == nil {
		return fmt.Errorf("transport fulfillment inbox: transactor is nil")
	}
	if store == nil {
		return fmt.Errorf("transport fulfillment inbox: inbox store is nil")
	}
	if former == nil {
		return fmt.Errorf("transport fulfillment inbox: actual fulfillment former is nil")
	}
	return nil
}

// settleFormation 按编排答出的处置落账。处理方与消费门在同一事务里：发生项、它的已形成信封与 inbox 的已处理同笔提交；
// 未决与缺陷都交回错误，整笔回滚，不留一份落了册而 inbox 没记、或 inbox 记了而发生项没落的半成品。
func settleFormation(result application.FormActualFulfillmentResult, err error) error {
	if err != nil {
		return err
	}
	switch result.Disposition() {
	case application.FormationSettled:
		return nil
	case application.FormationRetry:
		return fmt.Errorf("%w: %s", ErrActualFulfillmentFormationUndecided, describeFormation(result))
	default:
		return fmt.Errorf("%w: %s", ErrActualFulfillmentFormationDefect, describeFormation(result))
	}
}

// describeFormation 把两层答复写成一句给运维读的话：停在本编排哪一口，或登记用例答了哪一格、细格是什么、续办引用是什么。
func describeFormation(result application.FormActualFulfillmentResult) string {
	if result.Outcome() != application.FormationRegistrationAnswered {
		return fmt.Sprintf("%s %s", result.Outcome(), result.UndecidedReason())
	}
	registered := result.Registration()
	return fmt.Sprintf("%s segment %s: %s %s %s", result.Outcome(), result.Segment(),
		registered.Outcome(), registered.Detail(), registered.Continuation())
}

// decodeRegisteredEffectiveDelivery 译载荷。四维缺一即毒丸：版本不在就说不出这一封结束了哪一条参与，重投同样内容也不会
// 长出字段来。PS 那一路故意不读版本（它按键读当前版）；本路要读，参与的终点依据记的是结束它的那一版。
func decodeRegisteredEffectiveDelivery(payload []byte) (RegisteredEffectiveDelivery, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Object   string `json:"object"`
		Attempt  string `json:"attempt"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredEffectiveDelivery{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Object == "" || body.Attempt == "" || body.Version == "" {
		return RegisteredEffectiveDelivery{}, fmt.Errorf("%w: missing effective delivery key fields", ErrPoisonEnvelope)
	}
	return RegisteredEffectiveDelivery(body), nil
}
