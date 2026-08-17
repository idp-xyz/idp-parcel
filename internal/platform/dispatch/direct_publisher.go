package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
)

// Consumer 是进程内直投的接收方：一份信封推进它自己的消费门（Inbox 恰一次 + 业务
// 写入同一事务）。形状与各上下文 inbox 适配器的 Consume 一致，本包不导入它们——
// 平台层对事件类型一无所知，路由表的内容由组合根给。
type Consumer interface {
	Consume(ctx context.Context, envelope eventing.Envelope) error
}

// DirectPublisher 是 eventing.Publisher 的首发适配器：按 Envelope.Type 投给本进程
// 已注册的消费者（ADR-0049）。
//
// 它对「发布成功」取的是最严的那个含义：**只有消费者的消费门事务提交了才返回 nil**。
// 框架合同要求 nil 表示传输已持久接受，而进程内没有独立传输可以充当接受方，唯一能
// 承担这个含义的持久事实就是消费者 inbox 记录的提交。
//
// 投递施加显式超时，且构造期就要求它明显小于租约：租约若在投递返回前过期，另一个
// 派发器可以重新认领同一条，那是真正的并发重复处理——inbox 挡得住副作用，两边仍
// 白跑。这条约束是直投特有的，broker 下 Publish 只等 broker 应答，不等消费者。
type DirectPublisher struct {
	routes  map[eventing.EventType]Consumer
	timeout time.Duration
}

// NewDirectPublisher 装配路由表。
//
// 空路由表拒绝构造：直投下每一类事件都要在表里找得到订阅者，一张空表上线会让每一
// 类事件都撞 ErrNoSubscriber 并逐个阻塞分区。那是装配错误，让它在构造期立不起来，
// 比等第一份事件在生产上卡住好。
//
// timeout 必须显式给出且不超过租约的一半。不设默认与 Config 其余节奏参数同理——它
// 约束的是进程资源与重试节奏，没有哪个值对所有部署都对。取一半而不是「小于租约」：
// 认领与定稿本身也要时间，投递占满剩下的全部就等于没有余量。
func NewDirectPublisher(
	routes map[eventing.EventType]Consumer,
	timeout time.Duration,
	config Config,
) (*DirectPublisher, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, errors.New("dispatch: direct publisher needs a non-empty route table")
	}
	for eventType, consumer := range routes {
		if eventType == "" || consumer == nil {
			return nil, fmt.Errorf("dispatch: direct route %q is incomplete", eventType)
		}
	}
	if timeout <= 0 {
		return nil, errors.New("dispatch: direct delivery timeout must be given explicitly")
	}
	if timeout > config.LeaseFor/2 {
		return nil, fmt.Errorf(
			"dispatch: direct delivery timeout %s must stay well under the lease %s", timeout, config.LeaseFor)
	}

	table := make(map[eventing.EventType]Consumer, len(routes))
	for eventType, consumer := range routes {
		table[eventType] = consumer
	}
	return &DirectPublisher{routes: table, timeout: timeout}, nil
}

var _ eventing.Publisher = (*DirectPublisher)(nil)

// Publish 把一份信封投给它的订阅者。
//
// 三种不成功各自成格：没有订阅者（ErrNoSubscriber，装配漏了这一类）、结果不确定
// （ErrPublishUncertain，超时或取消——消费门事务可能已经提交，重投会真的造成重复
// 投递，处置落在消费侧 inbox）、消费者明确失败（原错误透出，下游确实没接）。
// 派发器按这三格记不同失败码，运维据此分辨该改装配、该核对重复还是该救下游。
func (publisher *DirectPublisher) Publish(ctx context.Context, envelope eventing.Envelope) error {
	consumer, subscribed := publisher.routes[envelope.Type]
	if !subscribed {
		return fmt.Errorf("%w: %q", ErrNoSubscriber, envelope.Type)
	}

	deliveryCtx, cancel := context.WithTimeout(ctx, publisher.timeout)
	defer cancel()

	err := consumer.Consume(deliveryCtx, envelope)
	if err == nil {
		return nil
	}
	// 超时或取消时消费门可能已经提交而回执没走完，判不出成没成。判成明确失败会让
	// 一次可能已生效的投递被当作没送到；判成成功则可能真的丢了。只有不确定这一格
	// 两头都不说死。
	if deliveryCtx.Err() != nil {
		return fmt.Errorf("%w: %q: %v", eventing.ErrPublishUncertain, envelope.Type, err)
	}
	return fmt.Errorf("dispatch: direct delivery of %q failed: %w", envelope.Type, err)
}
