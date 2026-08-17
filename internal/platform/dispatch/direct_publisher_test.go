package dispatch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// 本文件证进程内直投适配器（ADR-0049）的四条：只有消费者持久接受才算发布成功、
// 无订阅者显式失败不静默丢弃、投递超时按结果不确定交回、空路由表与过长超时在
// 构造期就立不起来。不需要真库——直投的语义在路由与超时上，不在存储上。

const directEventType eventing.EventType = "dispatch.direct.test.event"

type consumerDouble struct {
	consumed []eventing.Envelope
	err      error
	block    time.Duration
	sawDead  bool
}

func (double *consumerDouble) Consume(ctx context.Context, envelope eventing.Envelope) error {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		double.sawDead = true
	}
	if double.block > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(double.block):
		}
	}
	if double.err != nil {
		return double.err
	}
	double.consumed = append(double.consumed, envelope)
	return nil
}

func directEnvelope(eventType eventing.EventType) eventing.Envelope {
	return eventing.Envelope{
		SpecVersion: eventing.SpecVersion,
		ID:          eventing.EventID("event-1"),
		Source:      "idp-parcel/direct-test",
		Type:        eventType,
		Version:     1,
		RecordedAt:  time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
		Payload:     []byte(`{}`),
	}
}

func directConfig() dispatch.Config {
	return dispatch.Config{
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
		RetryAfter:  30 * time.Second,
	}
}

// Covers: ADR-0049 第二条——Publish 返回 nil 只在消费者的 inbox 事务已提交后才成立。
// 进程内没有别的持久接受方，消费者返回 nil 就是那个事实。
func TestDirectDeliverySucceedsOnlyAfterTheConsumerAccepts(t *testing.T) {
	consumer := &consumerDouble{}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{directEventType: consumer}, time.Second, directConfig())
	if err != nil {
		t.Fatalf("构造直投适配器：%v", err)
	}

	if err := publisher.Publish(context.Background(), directEnvelope(directEventType)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(consumer.consumed) != 1 {
		t.Fatalf("消费者被调 %d 次，want 1", len(consumer.consumed))
	}
	if !consumer.sawDead {
		t.Fatal("投递没有带显式超时——租约会在投递返回前过期（ADR-0049 第四条）")
	}
}

// Covers: ADR-0049 第三条——路由表是显式清单，没有订阅者的类型显式失败并入账，
// 不静默丢弃也不「未知类型即放行」。
func TestAnEnvelopeWithNoSubscriberFailsLoudly(t *testing.T) {
	consumer := &consumerDouble{}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{directEventType: consumer}, time.Second, directConfig())
	if err != nil {
		t.Fatalf("构造直投适配器：%v", err)
	}

	err = publisher.Publish(context.Background(), directEnvelope("dispatch.direct.test.unrouted"))
	if !errors.Is(err, dispatch.ErrNoSubscriber) {
		t.Fatalf("err = %v, want ErrNoSubscriber", err)
	}
	if len(consumer.consumed) != 0 {
		t.Fatal("没订阅这一类的消费者被投了别人的事件")
	}
}

// Covers: ADR-0049 第二条后半——提交结果不确定时按 ErrPublishUncertain 交回，由
// At-Least-Once 重投、消费侧 inbox 抑制重复副作用。超时正是「不知道它提交没有」。
func TestATimedOutDeliveryIsUncertainNotAPlainFailure(t *testing.T) {
	consumer := &consumerDouble{block: time.Second}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{directEventType: consumer}, 10*time.Millisecond, directConfig())
	if err != nil {
		t.Fatalf("构造直投适配器：%v", err)
	}

	err = publisher.Publish(context.Background(), directEnvelope(directEventType))
	if !errors.Is(err, eventing.ErrPublishUncertain) {
		t.Fatalf("err = %v, want ErrPublishUncertain", err)
	}
}

// Covers: 消费者明确失败与结果不确定分开——前者下游确实没接，重投是本来的账；
// 后者可能已经提交，重投会造成重复投递，处置落在消费侧 inbox。混成一格，运维
// 读不出该救下游还是该去核对重复。
func TestAConsumerFailureIsDefiniteNotUncertain(t *testing.T) {
	refused := errors.New("consumer refused")
	consumer := &consumerDouble{err: refused}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{directEventType: consumer}, time.Second, directConfig())
	if err != nil {
		t.Fatalf("构造直投适配器：%v", err)
	}

	err = publisher.Publish(context.Background(), directEnvelope(directEventType))
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v，消费者的失败原因没有透出来", err)
	}
	if errors.Is(err, eventing.ErrPublishUncertain) {
		t.Fatal("一次明确的消费者拒绝被记成了结果不确定")
	}
}

// Covers: 基线重盘那条硬约束与 ADR-0049 第三条合起来的推论——空路由表上线会让
// 每一类事件都撞无订阅者、逐个阻塞分区。它是装配错误，构造期就该立不起来，而不是
// 等第一份事件在生产上卡住才发现。
func TestAnEmptyRouteTableCannotBeAssembled(t *testing.T) {
	if _, err := dispatch.NewDirectPublisher(nil, time.Second, directConfig()); err == nil {
		t.Fatal("空路由表被接受了")
	}
	if _, err := dispatch.NewDirectPublisher(map[eventing.EventType]dispatch.Consumer{}, time.Second, directConfig()); err == nil {
		t.Fatal("零条目的路由表被接受了")
	}
}

// Covers: ADR-0049 第四条——投递超时必须显式且明显小于 LeaseFor，否则租约会在投递
// 返回之前过期，另一个派发器重新认领同一条，出现真正的并发重复处理。
func TestADeliveryTimeoutThatOutrunsTheLeaseCannotBeAssembled(t *testing.T) {
	routes := map[eventing.EventType]dispatch.Consumer{directEventType: &consumerDouble{}}
	config := directConfig()

	if _, err := dispatch.NewDirectPublisher(routes, 0, config); err == nil {
		t.Fatal("没有超时的直投被接受了——ADR-0049 要求显式给出，不设默认")
	}
	if _, err := dispatch.NewDirectPublisher(routes, config.LeaseFor, config); err == nil {
		t.Fatal("超时等于租约时长被接受了")
	}
	if _, err := dispatch.NewDirectPublisher(routes, config.LeaseFor/2+time.Millisecond, config); err == nil {
		t.Fatal("超时超过租约一半被接受了——余量不够认领与定稿本身")
	}
	if _, err := dispatch.NewDirectPublisher(routes, config.LeaseFor/2, config); err != nil {
		t.Fatalf("留足余量的超时反被拒：%v", err)
	}
}
