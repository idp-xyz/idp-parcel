// Package sainbox 是 settlement-accounting 的 inbox 消费者：把别的上下文经 Outbox 发出的信封推进本上下文
// 的消费门（Inbox 恰一次 + 业务写入同一事务，舞步在 inboxconsume）。消费者只译引用、不判业务：处理方
// 按引用回查提供方读口再交编排（票 sa-cc/09 做法 1）。
package sainbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// ErrPoisonEnvelope 标记译不出引用的信封：重投同样内容不会改变结果，消费门拒收入账。
var ErrPoisonEnvelope = errors.New("settlement accounting inbox: poison envelope")

// dutyPaymentVerificationConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由（消费者名 + 来源 +
// 事件 ID）认领，与别的消费者共名会让一路把另一路的投递当重复跳过；改名等于换消费者。
const dutyPaymentVerificationConsumerName = "settlement-accounting/adopt-duty-payment-verification"

// DutyPaymentVerificationFormedEventType 是本消费者认的事件类型：customs-compliance 形成一版税费付款核对后
// 交出的信封（票 sa-cc/05）。消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量；两串是否
// 相等由 cmd/parcel-dispatch 的真库装配用例钉——那里用提供方的真适配器入队、按本常量路由。
//
// 同一范围的下一版核对（迟到事实换指纹）在提供方是同一事件类型再发一封——本消费者对首版与后续版一视
// 同仁地交给处理方，各版各成一次采用；哪一版是当前不是消费者的事。
const DutyPaymentVerificationFormedEventType eventing.EventType = "customs-compliance.duty-payment-verification.formed"

// FormedDutyPaymentVerification 是译码后的引用——只有引用。载荷恰是提供方核对幂等键的五维（租户、申报范围、
// 税费引用、资金事实引用、版本指纹），三态与关联依据不在信封里（票 sa-cc/05 红线），处理方按这五维向
// 提供方读口回查、再交采用编排。
type FormedDutyPaymentVerification struct {
	TenantID string
	Scope    string
	Duty     string
	Funds    string
	Digest   string
}

// FormedDutyPaymentVerificationHandler 是本消费者转交的处理方。真实装配接
// adapters/customscompliance 的 AdoptOnDutyPaymentVerificationAdapter。
type FormedDutyPaymentVerificationHandler interface {
	HandleFormedDutyPaymentVerification(ctx context.Context, formed FormedDutyPaymentVerification) error
}

// DutyPaymentVerificationConsumer 把 CC 付款核对形成信封推进消费门。
type DutyPaymentVerificationConsumer struct {
	gate *inboxconsume.Gate[FormedDutyPaymentVerification]
}

func NewDutyPaymentVerificationConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedDutyPaymentVerificationHandler,
) (*DutyPaymentVerificationConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("settlement accounting inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedDutyPaymentVerification]{
		Transactor:     transactor,
		Store:          store,
		Name:           dutyPaymentVerificationConsumerName,
		EventType:      DutyPaymentVerificationFormedEventType,
		Decode:         decodeFormedDutyPaymentVerification,
		Handle:         handler.HandleFormedDutyPaymentVerification,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "handle formed duty payment verification",
	})
	if err != nil {
		return nil, err
	}
	return &DutyPaymentVerificationConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余上下文的消费者共用同一扇门。
func (consumer *DutyPaymentVerificationConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedDutyPaymentVerification 译载荷。五维缺一即毒丸——处理方按完整引用向提供方回查、按完整引用
// 采用，缺一维永远指不到一版，而重投同样内容不会长出字段来。
func decodeFormedDutyPaymentVerification(payload []byte) (FormedDutyPaymentVerification, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Scope    string `json:"scope"`
		Duty     string `json:"duty"`
		Funds    string `json:"funds"`
		Digest   string `json:"digest"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedDutyPaymentVerification{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Scope == "" || body.Duty == "" || body.Funds == "" || body.Digest == "" {
		return FormedDutyPaymentVerification{}, fmt.Errorf("%w: missing duty payment verification reference fields", ErrPoisonEnvelope)
	}
	return FormedDutyPaymentVerification{
		TenantID: body.TenantID,
		Scope:    body.Scope,
		Duty:     body.Duty,
		Funds:    body.Funds,
		Digest:   body.Digest,
	}, nil
}
