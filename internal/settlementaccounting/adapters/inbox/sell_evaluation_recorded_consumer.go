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

// sellEvaluationRecordedConsumerName 是 SELL 形成那一路在 inbox 键上的稳定名（ADR-0190 决定六「各有自己的 inbox」）。
// 与 BUY 那一路同收一种信封，所以必须不同名：共名会让后到的一路把这一封当重复跳过。
const sellEvaluationRecordedConsumerName = "settlement-accounting/form-estimated-customer-charge"

// SellEvaluationRecordedEventType 是本消费者认的事件类型。提供方对 BUY、SELL 两个方向发的是同一种评价已记录信封，
// 所以它就是 BuyEvaluationRecordedEventType 那一串——引它而不再写一遍，两路认的是不是同一种信封只有一处定义。
// 方向与目的在评价本体上，是不是 SELL·CUSTOMER_CHARGE 由处理方按引用回查后分辨。
const SellEvaluationRecordedEventType = BuyEvaluationRecordedEventType

// RecordedSellEvaluation 是译码后的引用——只有（租户、评价标识）。金额、费用行与回指都在评价本体上，处理方按引用
// 回读，消费者不复制第二份。
type RecordedSellEvaluation struct {
	TenantID     string
	EvaluationID string
}

// RecordedSellEvaluationHandler 是本消费者转交的处理方。真实装配接 adapters/parcelpricing 的
// FormOnSellEvaluationRecordedAdapter。
type RecordedSellEvaluationHandler interface {
	HandleRecordedSellEvaluation(ctx context.Context, recorded RecordedSellEvaluation) error
}

// SellEvaluationRecordedConsumer 把 PP 评价已记录信封推进 SELL 形成那一扇消费门。
type SellEvaluationRecordedConsumer struct {
	gate *inboxconsume.Gate[RecordedSellEvaluation]
}

func NewSellEvaluationRecordedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RecordedSellEvaluationHandler,
) (*SellEvaluationRecordedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("settlement accounting inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RecordedSellEvaluation]{
		Transactor:     transactor,
		Store:          store,
		Name:           sellEvaluationRecordedConsumerName,
		EventType:      SellEvaluationRecordedEventType,
		Decode:         decodeRecordedSellEvaluation,
		Handle:         handler.HandleRecordedSellEvaluation,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "handle recorded sell evaluation",
	})
	if err != nil {
		return nil, err
	}
	return &SellEvaluationRecordedConsumer{gate: gate}, nil
}

// Consume 处理一份投递。
func (consumer *SellEvaluationRecordedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRecordedSellEvaluation 译载荷。载荷与 BUY 那一路同一份契约（tenantId、evaluationId），缺一即毒丸：评价标识
// 指不到一份评价、租户核不了隔离，重投同样内容不会长出字段来。
func decodeRecordedSellEvaluation(payload []byte) (RecordedSellEvaluation, error) {
	var body struct {
		TenantID     string `json:"tenantId"`
		EvaluationID string `json:"evaluationId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RecordedSellEvaluation{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.EvaluationID == "" {
		return RecordedSellEvaluation{}, fmt.Errorf("%w: missing sell evaluation reference fields", ErrPoisonEnvelope)
	}
	return RecordedSellEvaluation{TenantID: body.TenantID, EvaluationID: body.EvaluationID}, nil
}
