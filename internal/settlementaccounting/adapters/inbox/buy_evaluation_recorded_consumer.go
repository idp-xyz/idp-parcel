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

// buyEvaluationRecordedConsumerName 是本消费者在 inbox 键上的稳定名。与付款核对那一路不同名：两路各记各的
// inbox 账，共名会让一路把另一路的投递当重复跳过。
const buyEvaluationRecordedConsumerName = "settlement-accounting/form-supplier-expected-cost"

// BuyEvaluationRecordedEventType 是本消费者认的事件类型：parcel-pricing 记下一份评价后交出的信封
// （`OutboxEvaluationHandoff`）。消费方自己写出这个字符串——提供方那个常量未导出，也不该为了消费方导出：
// 两串是否相等由 cmd/parcel-dispatch 的真库装配用例钉，那里用提供方的真适配器入队、按本常量路由。
//
// 提供方对 BUY 与 SELL 两个方向发的是同一种信封：信封只带评价引用，方向 / 目的在评价本体上。本消费者
// 因此收下每一封，是不是 BUY·SUPPLIER_COST 由处理方按引用回查后分辨——不是本消费者的评价，处理方答
// 不处理、消费门入账不重投（票 sa-cc/01 做法 2）。
const BuyEvaluationRecordedEventType eventing.EventType = "parcel-pricing.evaluation.recorded"

// RecordedBuyEvaluation 是译码后的引用——只有引用。载荷恰是提供方 FindByID 所需的两维（租户、评价标识），
// 金额、币种、规则版本一律不在信封里：它们整组出自评价，处理方按引用向提供方读口取回（ADR-0107），
// 消费者不复制第二份。
type RecordedBuyEvaluation struct {
	TenantID     string
	EvaluationID string
}

// RecordedBuyEvaluationHandler 是本消费者转交的处理方。真实装配接
// adapters/parcelpricing 的 FormOnBuyEvaluationRecordedAdapter。
type RecordedBuyEvaluationHandler interface {
	HandleRecordedBuyEvaluation(ctx context.Context, recorded RecordedBuyEvaluation) error
}

// BuyEvaluationRecordedConsumer 把 PP 评价已记录信封推进消费门。
type BuyEvaluationRecordedConsumer struct {
	gate *inboxconsume.Gate[RecordedBuyEvaluation]
}

func NewBuyEvaluationRecordedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RecordedBuyEvaluationHandler,
) (*BuyEvaluationRecordedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("settlement accounting inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RecordedBuyEvaluation]{
		Transactor:     transactor,
		Store:          store,
		Name:           buyEvaluationRecordedConsumerName,
		EventType:      BuyEvaluationRecordedEventType,
		Decode:         decodeRecordedBuyEvaluation,
		Handle:         handler.HandleRecordedBuyEvaluation,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "handle recorded buy evaluation",
	})
	if err != nil {
		return nil, err
	}
	return &BuyEvaluationRecordedConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余上下文的消费者共用同一扇门。
func (consumer *BuyEvaluationRecordedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRecordedBuyEvaluation 译载荷。两维缺一即毒丸——评价标识指不到一份评价、租户核不了隔离
// （ADR-0003），重投同样内容不会长出字段来。
func decodeRecordedBuyEvaluation(payload []byte) (RecordedBuyEvaluation, error) {
	var body struct {
		TenantID     string `json:"tenantId"`
		EvaluationID string `json:"evaluationId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RecordedBuyEvaluation{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.EvaluationID == "" {
		return RecordedBuyEvaluation{}, fmt.Errorf("%w: missing buy evaluation reference fields", ErrPoisonEnvelope)
	}
	return RecordedBuyEvaluation{TenantID: body.TenantID, EvaluationID: body.EvaluationID}, nil
}
