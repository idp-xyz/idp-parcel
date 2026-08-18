// Package nrinbox 是 network-routing 的入站事件消费适配器（ADR-0025：适配器在
// 消费方侧）。事务舞步交给 platform/inboxconsume；本包只声明消费者名、事件类型与
// 译码，以及转交给处理方的引用形状。
package nrinbox

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

// consumerName 是本消费者在 inbox 键上的稳定名。改名等于换消费者——已处理账本
// 与新名分家，全部在途事件会被重新处理一遍。
const consumerName = "network-routing/initial-route-on-acceptance"

// AcceptedDecisionEventType 是本消费者认的事件类型（PS 侧 outbox 适配器的常量在
// 它自己包里——两边各写各的名字，消费者不导入生产方的适配器包，跨包共享这个字符
// 串反而把两个部署单元钉在一次发布里）。
//
// 导出是给组合根登记路由用的：直投的路由表按 `Envelope.Type` 分派，而「本消费者
// 认哪一类」只有本包说得准。让组合根自己再抄一遍字符串，两处迟早分家，且分家那天
// 表现为无订阅者卡分区，不是编译错误。
const AcceptedDecisionEventType eventing.EventType = "parcel-shipment.acceptance-decision.formed"

// ErrPoisonEnvelope 表示信封解不出命令且重投同样内容不会改变结果——拒收而不是
// 无限重试。
var ErrPoisonEnvelope = errors.New("network routing inbox: poison envelope")

// AcceptedDecision 是译码后的接受决定引用——只有引用，路由创建要读的基线与包裹
// 清单由编排按引用重新取（跨上下文只传引用）。
//
// Source 与 SourceRequestKey 带上是因为「按引用重新取」得取得到：PS 的委托聚合按
// 完整来源身份（租户+客户+来源+来源请求键）取回，少这两维就查不着那份基线，而
// UC-NR-001 启动条件明写只有状态字符串而没有基线引用时不得继续。两者本来就在 PS
// 发的载荷里，此前只是被这个译码器丢掉了。
type AcceptedDecision struct {
	TenantID          string
	CustomerAccountID string
	Source            string
	SourceRequestKey  string
	ShipmentRequestID string
	SubmissionVersion string
	DecisionID        string
	State             string
}

// DecisionHandler 是本消费者转交的处理方。真实装配接 NR 的初始路由编排；消费门
// 不关心处理方语义，只保证「处理成功与消费入账同一事务」。
type DecisionHandler interface {
	HandleAcceptedDecision(ctx context.Context, decision AcceptedDecision) error
}

// AcceptanceConsumer 把 PS 接受决定信封推进消费门。
type AcceptanceConsumer struct {
	gate *inboxconsume.Gate[AcceptedDecision]
}

func NewAcceptanceConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler DecisionHandler,
) (*AcceptanceConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("network routing inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("network routing inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("network routing inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[AcceptedDecision]{
		Transactor:     transactor,
		Store:          store,
		Name:           consumerName,
		EventType:      AcceptedDecisionEventType,
		Decode:         decodeAcceptedDecision,
		Handle:         handler.HandleAcceptedDecision,
		UnexpectedType: "network routing inbox",
		HandleVerb:     "handle accepted decision",
	})
	if err != nil {
		return nil, err
	}
	return &AcceptanceConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume：消费入账与处理方同一事务、重复跳过、
// 失败回滚、毒丸拒收。
func (consumer *AcceptanceConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeAcceptedDecision 译载荷。缺字段即毒丸——重投同样内容不会长出字段来。
func decodeAcceptedDecision(payload []byte) (AcceptedDecision, error) {
	var body struct {
		TenantID          string `json:"tenantId"`
		CustomerAccountID string `json:"customerAccountId"`
		Source            string `json:"source"`
		SourceRequestKey  string `json:"sourceRequestKey"`
		ShipmentRequestID string `json:"shipmentRequestId"`
		SubmissionVersion string `json:"submissionVersion"`
		DecisionID        string `json:"decisionId"`
		State             string `json:"state"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return AcceptedDecision{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	// 来源身份四维与决定标识一并必备：缺其中任何一个，处理方都取不回基线，因而
	// 这份投递重投多少次都路由不出东西——那正是毒丸的定义，不是可重试的失败。
	if body.TenantID == "" || body.CustomerAccountID == "" ||
		body.Source == "" || body.SourceRequestKey == "" ||
		body.ShipmentRequestID == "" || body.DecisionID == "" {
		return AcceptedDecision{}, fmt.Errorf("%w: missing identity fields", ErrPoisonEnvelope)
	}
	// 状态字与提交版本同属必备，理由与上一组一样但落点不同：处理方按状态字分接受与
	// 拒绝两条走向，按提交版本认这份信封是不是换代前那一版的。两者缺席时没有安全的
	// 缺省——按「非接受即拒绝」往下走，会让一份说不清自己是什么的信封被静默入账；
	// 少了提交版本，一次陈旧的接受决定就能挂到新基线上。本消费者不认这两个字段的取值
	// （那是 PS 的话语，由 adapters/parcelshipment 逐格翻译），只要求它们在场。
	if body.State == "" || body.SubmissionVersion == "" {
		return AcceptedDecision{}, fmt.Errorf("%w: missing decision facts", ErrPoisonEnvelope)
	}
	return AcceptedDecision{
		TenantID:          body.TenantID,
		CustomerAccountID: body.CustomerAccountID,
		Source:            body.Source,
		SourceRequestKey:  body.SourceRequestKey,
		ShipmentRequestID: body.ShipmentRequestID,
		SubmissionVersion: body.SubmissionVersion,
		DecisionID:        body.DecisionID,
		State:             body.State,
	}, nil
}
