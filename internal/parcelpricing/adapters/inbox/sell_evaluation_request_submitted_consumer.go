package ppinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// sellEvaluationRequestSubmittedConsumerName 是 SELL 请求这一路在 inbox 键上的稳定名（ADR-0190 决定四）。与 BUY 那一路
// 分名：inbox 键按（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。
const sellEvaluationRequestSubmittedConsumerName = "parcel-pricing/form-sell-evaluation-from-request"

// SellEvaluationRequestSubmittedEventType 是 SA 登记一份 SELL 评价请求后同事务交出的信封（ADR-0190 决定一，
// `OutboxSellEvaluationRequestHandoff`）。消费方自己写出这个字符串，理由同 EvaluationRequestSubmittedEventType；两串
// 是否相等由 cmd/parcel-dispatch 的真库装配用例钉。
const SellEvaluationRequestSubmittedEventType eventing.EventType = "settlement-accounting.sell-evaluation-request.submitted"

// SubmittedSellEvaluationRequest 是译码后的引用——只有 SA `SellEvaluationRequestView.FindByID` 所需的两维。包裹、结算
// 账户、结算币种、计价范围与基准时点都不在信封里：请求内容只有 SA 的请求册一处权威。
type SubmittedSellEvaluationRequest struct {
	TenantID                string
	SellEvaluationRequestID string
}

// SubmittedSellEvaluationRequestHandler 是本消费者转交的处理方。真实装配接 adapters/settlementaccounting 的
// FormOnSellEvaluationRequestSubmittedAdapter。
type SubmittedSellEvaluationRequestHandler interface {
	HandleSubmittedSellEvaluationRequest(ctx context.Context, submitted SubmittedSellEvaluationRequest) error
}

// SellEvaluationRequestSubmittedConsumer 把 SA SELL 评价请求已提交信封推进消费门。
type SellEvaluationRequestSubmittedConsumer struct {
	gate *inboxconsume.Gate[SubmittedSellEvaluationRequest]
}

func NewSellEvaluationRequestSubmittedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler SubmittedSellEvaluationRequestHandler,
) (*SellEvaluationRequestSubmittedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel pricing inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel pricing inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel pricing inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[SubmittedSellEvaluationRequest]{
		Transactor:     transactor,
		Store:          store,
		Name:           sellEvaluationRequestSubmittedConsumerName,
		EventType:      SellEvaluationRequestSubmittedEventType,
		Decode:         decodeSubmittedSellEvaluationRequest,
		Handle:         handler.HandleSubmittedSellEvaluationRequest,
		UnexpectedType: "parcel pricing inbox",
		HandleVerb:     "handle submitted sell evaluation request",
	})
	if err != nil {
		return nil, err
	}
	return &SellEvaluationRequestSubmittedConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余上下文的消费者共用同一扇门。
func (consumer *SellEvaluationRequestSubmittedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeSubmittedSellEvaluationRequest 译载荷。两维缺一即毒丸，理由同 BUY 那一路：重投同样内容不会长出字段来。
func decodeSubmittedSellEvaluationRequest(payload []byte) (SubmittedSellEvaluationRequest, error) {
	var body struct {
		TenantID                string `json:"tenantId"`
		SellEvaluationRequestID string `json:"sellEvaluationRequestId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return SubmittedSellEvaluationRequest{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.SellEvaluationRequestID == "" {
		return SubmittedSellEvaluationRequest{}, fmt.Errorf("%w: missing sell evaluation request reference fields", ErrPoisonEnvelope)
	}
	return SubmittedSellEvaluationRequest{TenantID: body.TenantID, SellEvaluationRequestID: body.SellEvaluationRequestID}, nil
}
