// Package ccinbox 是 customs-compliance 的 inbox 消费者：把别的上下文经 Outbox 发出的信封推进本
// 上下文的消费门（Inbox 恰一次 + 业务写入同一事务，舞步在 inboxconsume）。消费者只译引用、不判
// 业务：处理方按引用回查提供方读口再交编排（票 sa-cc/03 做法 1）。
package ccinbox

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
var ErrPoisonEnvelope = errors.New("customs compliance inbox: poison envelope")

// externalFundsFactConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由（消费者名 + 来源 +
// 事件 ID）认领，与别的消费者共名会让一路把另一路的投递当重复跳过；改名等于换消费者。
const externalFundsFactConsumerName = "customs-compliance/receive-external-funds-fact"

// ExternalFundsFactAdoptedEventType 是本消费者认的事件类型：settlement-accounting 采用一条外部资金
// 事实后交出的信封（票 sa-cc/02）。消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 更正 / 撤销在提供方是回指原事实的新版本、同一事件类型再发一封（票 sa-cc/02 裁决 2）——本消费者
// 对首版与更正版一视同仁地交给处理方；处理方按（引用 + 版本）把更正版本登成入向登记册的新一行、回指前版
// （票 sa-cc/13，`ExternalFundsFactRegister.ListFundsFactVersions` 列得出全部版本），新版本落册后接着让该事实的
// 每条既往核对谱系各形成一版待人重判的新核对版本（票 sa-cc/19，`RederiveDutyVerificationsOnFundsFactVersion`）
// ——都在处理方与编排里，本消费者仍只译引用。
const ExternalFundsFactAdoptedEventType eventing.EventType = "settlement-accounting.external-funds-fact.adopted"

// AdoptedExternalFundsFact 是译码后的引用——只有引用，事实内容由处理方按（租户 + 事实 + 版本）
// 向提供方读口回查（权威留在 settlement-accounting，载荷不带金额）。Version 是信封所指的那一版：
// 处理方按它读、不读 latest（票 sa-cc/03 裁决）。载荷里可缺席的 corrects 不进译码：更正回指是事实
// 本体的一维，读事实时自会看到。
type AdoptedExternalFundsFact struct {
	TenantID string
	Fact     string
	Version  string
}

// AdoptedExternalFundsFactHandler 是本消费者转交的处理方。真实装配接
// adapters/settlementaccounting 的 ReceiveOnAdoptedFundsFactAdapter。
type AdoptedExternalFundsFactHandler interface {
	HandleAdoptedExternalFundsFact(ctx context.Context, adopted AdoptedExternalFundsFact) error
}

// ExternalFundsFactConsumer 把 SA 资金事实采用信封推进消费门。
type ExternalFundsFactConsumer struct {
	gate *inboxconsume.Gate[AdoptedExternalFundsFact]
}

func NewExternalFundsFactConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler AdoptedExternalFundsFactHandler,
) (*ExternalFundsFactConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("customs compliance inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("customs compliance inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("customs compliance inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[AdoptedExternalFundsFact]{
		Transactor:     transactor,
		Store:          store,
		Name:           externalFundsFactConsumerName,
		EventType:      ExternalFundsFactAdoptedEventType,
		Decode:         decodeAdoptedExternalFundsFact,
		Handle:         handler.HandleAdoptedExternalFundsFact,
		UnexpectedType: "customs compliance inbox",
		HandleVerb:     "handle adopted external funds fact",
	})
	if err != nil {
		return nil, err
	}
	return &ExternalFundsFactConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余上下文的消费者共用同一扇门。
func (consumer *ExternalFundsFactConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeAdoptedExternalFundsFact 译载荷。三维缺一即毒丸——处理方按（租户 + 事实 + 版本）向提供方
// 回查，缺了永远查不着，而重投同样内容不会长出字段来。
func decodeAdoptedExternalFundsFact(payload []byte) (AdoptedExternalFundsFact, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Fact     string `json:"fact"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return AdoptedExternalFundsFact{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Fact == "" || body.Version == "" {
		return AdoptedExternalFundsFact{}, fmt.Errorf("%w: missing funds fact reference fields", ErrPoisonEnvelope)
	}
	return AdoptedExternalFundsFact{TenantID: body.TenantID, Fact: body.Fact, Version: body.Version}, nil
}
