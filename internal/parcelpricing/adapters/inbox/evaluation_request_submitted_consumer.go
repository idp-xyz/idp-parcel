// Package ppinbox 是 parcel-pricing 收信封的那一侧（票 sa-cc/11 裁决 5）。此前 PP 只发 `parcel-pricing.evaluation.recorded`
// 不收任何信封，评价只由测试与 HTTP 回放门形成；SA 的评价请求信封是第一封。消费者只译不判：把载荷译成引用交给
// 处理方，请求内容按引用向 SA 读口取，金额 / 币种 / 换算 / 取整一个都不在这里（ADR-0107 / ADR-0013）。
package ppinbox

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

// ErrPoisonEnvelope 标记译不出引用的信封：重投同样内容不会改变结果，消费门拒收入账。毒丸只管信封结构；业务
// 缺件（请求还看不见、价卡未配置、输入不可得）走处理方哨兵，由消费门回滚重投。
var ErrPoisonEnvelope = errors.New("parcel pricing inbox: poison envelope")

// evaluationRequestSubmittedConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键由（消费者名 + 来源 + 事件 ID）
// 认领；改名等于换消费者，PP 日后再收别的信封要另起一名，共名会让一路把另一路的投递当重复跳过。
const evaluationRequestSubmittedConsumerName = "parcel-pricing/form-evaluation-from-request"

// EvaluationRequestSubmittedEventType 是本消费者认的事件类型：SA 登记一份评价请求后同事务交出的信封
// （`OutboxEvaluationRequestHandoff`，票 sa-cc/08）。消费方自己写出这个字符串——提供方那个常量未导出，也不该为了
// 消费方导出；两串是否相等由 cmd/parcel-dispatch 的真库装配用例钉，那里用提供方的真适配器入队、按本常量路由。
const EvaluationRequestSubmittedEventType eventing.EventType = "settlement-accounting.evaluation-request.submitted"

// SubmittedEvaluationRequest 是译码后的引用——只有引用。载荷恰是 SA `EvaluationRequestView.FindByID` 所需的两维
// （租户、铸造的请求 ID）；主要范围、计算目的与合格来源引用一律不在信封里，请求内容只有 SA 登记册一处权威，处理
// 方按引用取，消费者不复制第二份。
type SubmittedEvaluationRequest struct {
	TenantID            string
	EvaluationRequestID string
}

// SubmittedEvaluationRequestHandler 是本消费者转交的处理方。真实装配接 adapters/settlementaccounting 的
// FormOnEvaluationRequestSubmittedAdapter。
type SubmittedEvaluationRequestHandler interface {
	HandleSubmittedEvaluationRequest(ctx context.Context, submitted SubmittedEvaluationRequest) error
}

// EvaluationRequestSubmittedConsumer 把 SA 评价请求已提交信封推进消费门。
type EvaluationRequestSubmittedConsumer struct {
	gate *inboxconsume.Gate[SubmittedEvaluationRequest]
}

func NewEvaluationRequestSubmittedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler SubmittedEvaluationRequestHandler,
) (*EvaluationRequestSubmittedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel pricing inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel pricing inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel pricing inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[SubmittedEvaluationRequest]{
		Transactor:     transactor,
		Store:          store,
		Name:           evaluationRequestSubmittedConsumerName,
		EventType:      EvaluationRequestSubmittedEventType,
		Decode:         decodeSubmittedEvaluationRequest,
		Handle:         handler.HandleSubmittedEvaluationRequest,
		UnexpectedType: "parcel pricing inbox",
		HandleVerb:     "handle submitted evaluation request",
	})
	if err != nil {
		return nil, err
	}
	return &EvaluationRequestSubmittedConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余上下文的消费者共用同一扇门。
func (consumer *EvaluationRequestSubmittedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeSubmittedEvaluationRequest 译载荷。两维缺一即毒丸——请求 ID 指不到一份请求、租户核不了隔离（ADR-0003），
// 重投同样内容不会长出字段来。
func decodeSubmittedEvaluationRequest(payload []byte) (SubmittedEvaluationRequest, error) {
	var body struct {
		TenantID            string `json:"tenantId"`
		EvaluationRequestID string `json:"evaluationRequestId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return SubmittedEvaluationRequest{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.EvaluationRequestID == "" {
		return SubmittedEvaluationRequest{}, fmt.Errorf("%w: missing evaluation request reference fields", ErrPoisonEnvelope)
	}
	return SubmittedEvaluationRequest{TenantID: body.TenantID, EvaluationRequestID: body.EvaluationRequestID}, nil
}
