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

// exceptionJourneyConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由
// （消费者名 + 来源 + 事件 ID）认领，与其余 TF 消费账分开；改名等于换消费者。
const exceptionJourneyConsumerName = "visibility-exception/derive-projection-from-exception-journey"

// ExceptionJourneyRecordedEventType 是本消费者认的事件类型：TF 的替代/退运旅程启动。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 一封信带全体成员——载荷只有旅程幂等键四维，成员清单由处理方按键重读本体取回。
// 这条事实没有对象级替代品，按 ADR-0066 在消费侧循环拆分；同聚合的
// `disposition-execution.recorded` 是 CC 处置执行核对的口，本消费者不认。
const ExceptionJourneyRecordedEventType eventing.EventType = "transport-fulfillment.exception-journey.recorded"

// RecordedExceptionJourney 是译码后的旅程启动幂等键引用——只有引用，旅程本体（含
// 成员清单）由处理方按引用重新取（权威事实留在 transport-fulfillment）。
type RecordedExceptionJourney struct {
	TenantID string
	Original string
	Purpose  string
	Basis    string
}

// RecordedExceptionJourneyHandler 是本消费者转交的处理方。真实装配接
// DeriveOnExceptionJourneyAdapter。
type RecordedExceptionJourneyHandler interface {
	HandleRecordedExceptionJourney(ctx context.Context, recorded RecordedExceptionJourney) error
}

// ExceptionJourneyConsumer 把 TF 替代/退运旅程启动信封推进消费门。
type ExceptionJourneyConsumer struct {
	gate *inboxconsume.Gate[RecordedExceptionJourney]
}

func NewExceptionJourneyConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RecordedExceptionJourneyHandler,
) (*ExceptionJourneyConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RecordedExceptionJourney]{
		Transactor:     transactor,
		Store:          store,
		Name:           exceptionJourneyConsumerName,
		EventType:      ExceptionJourneyRecordedEventType,
		Decode:         decodeRecordedExceptionJourney,
		Handle:         handler.HandleRecordedExceptionJourney,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle recorded exception journey",
	})
	if err != nil {
		return nil, err
	}
	return &ExceptionJourneyConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ExceptionJourneyConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRecordedExceptionJourney 译载荷。tenantId / original / purpose / basis 缺一
// 即毒丸——处理方按这四维取回旅程，缺了永远取不着，而重投同样内容不会长出字段来。
func decodeRecordedExceptionJourney(payload []byte) (RecordedExceptionJourney, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Original string `json:"original"`
		Purpose  string `json:"purpose"`
		Basis    string `json:"basis"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RecordedExceptionJourney{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Original == "" || body.Purpose == "" || body.Basis == "" {
		return RecordedExceptionJourney{}, fmt.Errorf("%w: missing journey key fields", ErrPoisonEnvelope)
	}
	return RecordedExceptionJourney{
		TenantID: body.TenantID,
		Original: body.Original,
		Purpose:  body.Purpose,
		Basis:    body.Basis,
	}, nil
}
