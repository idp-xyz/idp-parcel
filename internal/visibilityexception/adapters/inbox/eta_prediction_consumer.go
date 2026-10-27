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

// etaPredictionConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由（消费者名 + 来源 + 事件 ID）
// 认领，与投影派生那本账分家；改名等于换消费者。
const etaPredictionConsumerName = "visibility-exception/judge-eta-visibility-from-prediction"

// ETAPredictionFormedEventType 是本消费者认的事件类型：VE 自己的内部 ETA 预测版本形成。消费方自己写出
// 这个字符串，不导入提供方 outbox 适配器的未导出常量——同一上下文的两个适配器也不例外。
//
// 载荷只有（租户+包裹+里程碑+预测版本）引用，预测本体由处理方按（租户+包裹+里程碑）读回当前版。
const ETAPredictionFormedEventType eventing.EventType = "visibility-exception.eta-prediction.formed"

// FormedETAPrediction 是译码后的预测版本引用。
type FormedETAPrediction struct {
	TenantID  string
	Parcel    string
	Milestone string
	VersionID string
}

// FormedETAPredictionHandler 是本消费者转交的处理方。真实装配接
// parcelshipment.JudgeETAVisibilityOnPredictionAdapter。
type FormedETAPredictionHandler interface {
	HandleFormedETAPrediction(ctx context.Context, formed FormedETAPrediction) error
}

// ETAPredictionConsumer 把 ETA 预测版本信封推进消费门。
type ETAPredictionConsumer struct {
	gate *inboxconsume.Gate[FormedETAPrediction]
}

func NewETAPredictionConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedETAPredictionHandler,
) (*ETAPredictionConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedETAPrediction]{
		Transactor:     transactor,
		Store:          store,
		Name:           etaPredictionConsumerName,
		EventType:      ETAPredictionFormedEventType,
		Decode:         decodeFormedETAPrediction,
		Handle:         handler.HandleFormedETAPrediction,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle formed eta prediction",
	})
	if err != nil {
		return nil, err
	}
	return &ETAPredictionConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ETAPredictionConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedETAPrediction 译载荷。四维缺一即毒丸——处理方按（租户+包裹+里程碑）取回当前预测，缺了永远
// 取不着，而重投同样内容不会长出字段来。
func decodeFormedETAPrediction(payload []byte) (FormedETAPrediction, error) {
	var body struct {
		TenantID  string `json:"tenantId"`
		Parcel    string `json:"parcel"`
		Milestone string `json:"milestone"`
		VersionID string `json:"versionId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedETAPrediction{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.Milestone == "" || body.VersionID == "" {
		return FormedETAPrediction{}, fmt.Errorf("%w: missing eta prediction reference fields", ErrPoisonEnvelope)
	}
	return FormedETAPrediction{
		TenantID:  body.TenantID,
		Parcel:    body.Parcel,
		Milestone: body.Milestone,
		VersionID: body.VersionID,
	}, nil
}
