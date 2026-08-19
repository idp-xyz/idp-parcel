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

// customsCaseConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由
// （消费者名 + 来源 + 事件 ID）认领，与 TF 各消费账分开；改名等于换消费者。
const customsCaseConsumerName = "visibility-exception/derive-projection-from-customs-case"

// CustomsCaseEstablishedEventType 是本消费者认的事件类型：CC 的关务案件建立。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 一封信带全体成员关联——载荷只有案件身份键五维，成员由处理方按键重读本体取回。
// 这条事实没有对象级替代品，按 ADR-0066 在消费侧循环拆分。
const CustomsCaseEstablishedEventType eventing.EventType = "customs-compliance.customs-case.established"

// EstablishedCustomsCase 是译码后的案件身份键引用——只有引用，案件本体（含成员
// 关联与客户归属）由处理方按引用重新取（权威事实留在 customs-compliance）。
type EstablishedCustomsCase struct {
	TenantID     string
	Jurisdiction string
	Direction    string
	Procedure    string
	Obligation   string
}

// EstablishedCustomsCaseHandler 是本消费者转交的处理方。真实装配接
// DeriveOnCustomsCaseAdapter。
type EstablishedCustomsCaseHandler interface {
	HandleEstablishedCustomsCase(ctx context.Context, established EstablishedCustomsCase) error
}

// CustomsCaseConsumer 把 CC 关务案件建立信封推进消费门。
type CustomsCaseConsumer struct {
	gate *inboxconsume.Gate[EstablishedCustomsCase]
}

func NewCustomsCaseConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler EstablishedCustomsCaseHandler,
) (*CustomsCaseConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[EstablishedCustomsCase]{
		Transactor:     transactor,
		Store:          store,
		Name:           customsCaseConsumerName,
		EventType:      CustomsCaseEstablishedEventType,
		Decode:         decodeEstablishedCustomsCase,
		Handle:         handler.HandleEstablishedCustomsCase,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle established customs case",
	})
	if err != nil {
		return nil, err
	}
	return &CustomsCaseConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *CustomsCaseConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeEstablishedCustomsCase 译载荷。五维键缺一即毒丸——处理方按（租户+辖区+方向+
// 程序+义务范围）取回案件，缺了永远取不着，而重投同样内容不会长出字段来。
func decodeEstablishedCustomsCase(payload []byte) (EstablishedCustomsCase, error) {
	var body struct {
		TenantID     string `json:"tenantId"`
		Jurisdiction string `json:"jurisdiction"`
		Direction    string `json:"direction"`
		Procedure    string `json:"procedure"`
		Obligation   string `json:"obligation"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return EstablishedCustomsCase{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Jurisdiction == "" || body.Direction == "" ||
		body.Procedure == "" || body.Obligation == "" {
		return EstablishedCustomsCase{}, fmt.Errorf("%w: missing customs case key fields", ErrPoisonEnvelope)
	}
	return EstablishedCustomsCase{
		TenantID:     body.TenantID,
		Jurisdiction: body.Jurisdiction,
		Direction:    body.Direction,
		Procedure:    body.Procedure,
		Obligation:   body.Obligation,
	}, nil
}
