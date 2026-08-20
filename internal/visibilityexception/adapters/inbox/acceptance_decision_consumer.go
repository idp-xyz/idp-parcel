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

// acceptanceDecisionConsumerName 是本消费者在 inbox 键上的稳定名。与 NR 的
// initial-route-on-acceptance 消费同一封信封而各记各的账（FanOut 的前提），也与本
// 上下文其余消费账分家；改名等于换消费者。
const acceptanceDecisionConsumerName = "visibility-exception/derive-customer-view-from-acceptance"

// AcceptanceDecisionFormedEventType 是本消费者认的事件类型：PS 的接受决定形成。
// 消费方自己写出这个字符串，不导入生产方或其他消费方的常量——与 nrinbox 共享会把
// 两个消费面钉在一次发布里。
//
// 同一个事件类型承载接受与拒绝两种走向（State 字段），本消费面只对接受态动作——
// 客户归属确立触发客户视图形成（UC-VE-008 AT-VE-169）。载荷带 customerAccountId，
// 但它回答的是「这份委托属于谁」，账户维的权威仍是 PS 按包裹反查口（ADR-0060）；
// 信封值只作一致性校验，翻译在 adapters/parcelshipment 逐格进行。
const AcceptanceDecisionFormedEventType eventing.EventType = "parcel-shipment.acceptance-decision.formed"

// FormedAcceptanceDecision 是译码后的接受决定引用——只有引用，声明包裹清单与投影
// 本体由处理方按（租户+委托）、（租户+包裹）重新取（跨上下文只传引用）。
//
// 刻意不带 Source/SourceRequestKey/SubmissionVersion/DecisionID：本消费面按（租户+
// 委托标识）取当前清单，已接受委托撤不了也换不了代（decisionFormed 闸门），当前版
// 就是接受那一版，用不上那几维；要求在场而不消费，缺席时会把 NR 才用得着的字段错
// 判成本面的毒丸。
type FormedAcceptanceDecision struct {
	TenantID          string
	CustomerAccountID string
	ShipmentRequestID string
	State             string
}

// FormedAcceptanceDecisionHandler 是本消费者转交的处理方。真实装配接
// parcelshipment.DeriveCustomerViewOnAcceptanceAdapter。
type FormedAcceptanceDecisionHandler interface {
	HandleFormedAcceptanceDecision(ctx context.Context, formed FormedAcceptanceDecision) error
}

// AcceptanceDecisionConsumer 把 PS 接受决定信封推进消费门。
type AcceptanceDecisionConsumer struct {
	gate *inboxconsume.Gate[FormedAcceptanceDecision]
}

func NewAcceptanceDecisionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedAcceptanceDecisionHandler,
) (*AcceptanceDecisionConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedAcceptanceDecision]{
		Transactor:     transactor,
		Store:          store,
		Name:           acceptanceDecisionConsumerName,
		EventType:      AcceptanceDecisionFormedEventType,
		Decode:         decodeFormedAcceptanceDecision,
		Handle:         handler.HandleFormedAcceptanceDecision,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle formed acceptance decision",
	})
	if err != nil {
		return nil, err
	}
	return &AcceptanceDecisionConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *AcceptanceDecisionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedAcceptanceDecision 译载荷。四维缺一即毒丸：缺租户或委托取不回清单，
// 缺状态字分不出接受与拒绝，缺账户做不了一致性校验——重投同样内容不会长出字段来。
func decodeFormedAcceptanceDecision(payload []byte) (FormedAcceptanceDecision, error) {
	var body struct {
		TenantID          string `json:"tenantId"`
		CustomerAccountID string `json:"customerAccountId"`
		ShipmentRequestID string `json:"shipmentRequestId"`
		State             string `json:"state"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedAcceptanceDecision{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.CustomerAccountID == "" ||
		body.ShipmentRequestID == "" || body.State == "" {
		return FormedAcceptanceDecision{}, fmt.Errorf("%w: missing acceptance decision fields", ErrPoisonEnvelope)
	}
	return FormedAcceptanceDecision{
		TenantID:          body.TenantID,
		CustomerAccountID: body.CustomerAccountID,
		ShipmentRequestID: body.ShipmentRequestID,
		State:             body.State,
	}, nil
}
