package nrinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
)

// networkIntakeConsumerName 是本消费者在 inbox 键上的稳定名。它与接受决定那条线的
// 名字不同，两本账因此分家——同一份投递在两条线上各处理一次不是重复处理。改名等于
// 换消费者，全部在途事件会被重新处理一遍。
const networkIntakeConsumerName = "network-routing/reassess-on-network-intake"

// AdoptedNetworkIntakeEventType 是本消费者认的事件类型。导出理由同
// AcceptedDecisionEventType：直投的路由表按 `Envelope.Type` 分派，而「本消费者认哪
// 一类」只有本包说得准；让组合根另抄一遍字符串，分家那天表现为无订阅者卡分区，不是
// 编译错误。
const AdoptedNetworkIntakeEventType eventing.EventType = "parcel-shipment.network-intake.recorded"

// AdoptedNetworkIntake 是译码后的采用键引用——只有引用，复核要读的采用记录本体由处理
// 方按引用重新取（跨上下文只传引用）。
//
// 四维恰是 PS 采用结果的幂等键（租户+包裹+来源类型+来源结果版本）：处理方拿它整行取
// 回记录。这里不带收寄明细或承诺，与 PS 侧的载荷形状一致。
type AdoptedNetworkIntake struct {
	TenantID string
	Parcel   string
	Kind     string
	Version  string
}

// NetworkIntakeHandler 是本消费者转交的处理方。真实装配接 NR 的复核编排；消费门不关心
// 处理方语义，只保证「处理成功与消费入账同一事务」。
type NetworkIntakeHandler interface {
	HandleAdoptedNetworkIntake(ctx context.Context, intake AdoptedNetworkIntake) error
}

// NetworkIntakeConsumer 把 PS 有效网络收寄采用结果的信封推进消费门（UC-PS-003 步骤 8
// → UC-NR-003）。
type NetworkIntakeConsumer struct {
	transactor bentoapp.Transactor
	store      *inbox.Store
	handler    NetworkIntakeHandler
}

func NewNetworkIntakeConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler NetworkIntakeHandler,
) (*NetworkIntakeConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("network routing inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("network routing inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("network routing inbox: handler is nil")
	}
	return &NetworkIntakeConsumer{transactor: transactor, store: store, handler: handler}, nil
}

// Consume 处理一份投递。四条与 AcceptanceConsumer 逐字相同：消费入账与处理方的业务
// 写入同一事务、重复投递幂等跳过、处理失败整体回滚可重投、毒丸显式拒收入账。
//
// 这段事务舞步与 AcceptanceConsumer 重复是有意的：本仓提炼平台件的门槛是三例同形
// （`outboxintent.EnqueueOnce` 就是那么来的），两例先各写各的。第三个消费者出现时，
// 该抽的是「消费门」而不是某一类事件。
func (consumer *NetworkIntakeConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	if envelope.Type != AdoptedNetworkIntakeEventType {
		// 认不得的类型不是毒丸——订阅面配置宽了是装配问题，拒收会把别人的事件记进
		// 自己的账。响亮报错让装配方修订阅。
		return fmt.Errorf("network routing inbox: unexpected event type %q", envelope.Type)
	}

	intake, decodeErr := decodeAdoptedNetworkIntake(envelope.Payload)

	return consumer.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		started, err := consumer.store.Start(txCtx, eventing.InboxKey{
			Consumer: networkIntakeConsumerName,
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
			if err := consumer.store.MarkRejected(
				txCtx, started.Receipt, envelope.RecordedAt, "inbox.poison_envelope"); err != nil {
				return fmt.Errorf("mark rejected: %w", err)
			}
			return nil
		}

		if err := consumer.handler.HandleAdoptedNetworkIntake(txCtx, intake); err != nil {
			// 处理失败让整个事务回滚：inbox 无痕，重投可以再试。复核的`未决`走的正是
			// 这一格——它等的是依赖恢复，重投会改变结果。
			return fmt.Errorf("handle adopted network intake: %w", err)
		}
		if err := consumer.store.MarkProcessed(txCtx, started.Receipt, envelope.RecordedAt); err != nil {
			return fmt.Errorf("mark processed: %w", err)
		}
		return nil
	})
}

// decodeAdoptedNetworkIntake 译载荷。四维缺一即毒丸——处理方按整键取回采用记录，缺任
// 一维都取不着那一行，而重投同样内容不会长出字段来。
//
// 尤其不给 kind 与 version 留缺省：来源类型决定控制依据译成哪一格（节点收寄 vs 权威
// 运输交接），来源版本进触发指纹；猜任一个都会让复核落在另一份事实上，而那种错跑得
// 通、看不出来。
func decodeAdoptedNetworkIntake(payload []byte) (AdoptedNetworkIntake, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Parcel   string `json:"parcel"`
		Kind     string `json:"kind"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return AdoptedNetworkIntake{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.Kind == "" || body.Version == "" {
		return AdoptedNetworkIntake{}, fmt.Errorf("%w: missing adoption key fields", ErrPoisonEnvelope)
	}
	return AdoptedNetworkIntake{
		TenantID: body.TenantID,
		Parcel:   body.Parcel,
		Kind:     body.Kind,
		Version:  body.Version,
	}, nil
}
