// Package nrinbox 是 network-routing 的入站事件消费适配器（ADR-0025：适配器在
// 消费方侧）。它守的是消费门的事务语义：同一份投递恰好处理一次、处理失败回滚后
// 可重投、毒丸显式拒收不无限重试。
package nrinbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
)

// consumerName 是本消费者在 inbox 键上的稳定名。改名等于换消费者——已处理账本
// 与新名分家，全部在途事件会被重新处理一遍。
const consumerName = "network-routing/initial-route-on-acceptance"

// acceptedDecisionEventType 是本消费者认的事件类型（PS 侧 outbox 适配器的常量在
// 它自己包里——两边各写各的名字，消费者不导入生产方的适配器包，跨包共享这个字符
// 串反而把两个部署单元钉在一次发布里）。
const acceptedDecisionEventType = "parcel-shipment.acceptance-decision.formed"

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
	transactor bentoapp.Transactor
	store      *inbox.Store
	handler    DecisionHandler
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
	return &AcceptanceConsumer{transactor: transactor, store: store, handler: handler}, nil
}

// Consume 处理一份投递。消费入账（Start/MarkProcessed）与处理方的业务写入同一
// 事务：处理失败整体回滚，inbox 无痕，重投可以再试；已处理的重复投递幂等跳过，
// 处理方不会被调第二次；解不出命令的毒丸在自己的事务里显式拒收——拒收也是账，
// 不落账的拒收会让同一份毒丸永远重投。
func (consumer *AcceptanceConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	if envelope.Type != acceptedDecisionEventType {
		// 认不得的类型不是毒丸——订阅面配置宽了是装配问题，拒收会把别人的事件
		// 记进自己的账。响亮报错让装配方修订阅。
		return fmt.Errorf("network routing inbox: unexpected event type %q", envelope.Type)
	}

	decision, decodeErr := decodeAcceptedDecision(envelope.Payload)

	return consumer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		started, err := consumer.store.Start(txCtx, eventing.InboxKey{
			Consumer: consumerName,
			Source:   envelope.Source,
			EventID:  envelope.ID,
		}, envelope.RecordedAt)
		if err != nil {
			return fmt.Errorf("start inbox entry: %w", err)
		}
		if !started.Acquired {
			// 已处理或已拒收：同一份投递的重复，处理方不看第二眼。
			return nil
		}

		if decodeErr != nil {
			// 失败码按框架格式（小写点分，先例 envelope.invalid_on_publish）。
			if err := consumer.store.MarkRejected(
				txCtx, started.Receipt, envelope.RecordedAt, "inbox.poison_envelope"); err != nil {
				return fmt.Errorf("mark rejected: %w", err)
			}
			return nil
		}

		if err := consumer.handler.HandleAcceptedDecision(txCtx, decision); err != nil {
			// 处理失败让整个事务回滚：inbox 无痕，重投可以再试。这里不区分暂时
			// 与永久失败——那是处理方内部按 ADR-0029 分格的事，消费门只管账。
			return fmt.Errorf("handle accepted decision: %w", err)
		}
		if err := consumer.store.MarkProcessed(txCtx, started.Receipt, envelope.RecordedAt); err != nil {
			return fmt.Errorf("mark processed: %w", err)
		}
		return nil
	})
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
