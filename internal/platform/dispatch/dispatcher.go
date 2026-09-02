// Package dispatch 拥有 outbox 到发布通道的派发一拍。框架（idp-bento-go）给的是
// Claim/MarkPublished/RecordFailure 的存储合同与租约语义，循环、批量与重试节奏是
// 消费方的装配职责——放平台层因为它对事件类型一无所知，十个上下文共用同一拍。
package dispatch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
)

// Clock 由装配方注入；派发的时间进 Claim.Now 与租约计算，可控时间是真库门禁能
// 测「租约过期重派」的前提。
type Clock interface {
	Now() time.Time
}

// ErrNoSubscriber 由发布通道交回，表示这一类信封在本进程的路由表里没有订阅者。
//
// 按 ADR-0049 无订阅者显式失败并入账，因而会阻塞该分区直到失败预算耗尽。那是有意的：
// 直投下「没有订阅者」只可能是漏装配，让它在第一份事件上就响亮地卡住，比静默吞掉一整类
// 事件好——后者要等到有人发现下游少了数据才暴露。
var ErrNoSubscriber = errors.New("dispatch: no subscriber for envelope type")

// ErrConsumerUndecided 表示消费者收到并处理了这份投递，但停在自己的未决上因而整份回滚。
//
// 它与「没送到」分开记码，因为运维要看的地方相反：未决要去查消费方等的那个依赖，没送到
// 要去查传输。包装的责任在路由表那一层——只有它知道每个消费者的未决哨兵长什么样，派发器
// 只认这一个。
var ErrConsumerUndecided = errors.New("dispatch: consumer stalled on its own dependency")

// 失败码按运维要做的动作取值，不按错误来自哪一层取值。取值形状受框架 CHECK 约束：
// 小写起首、只含 [a-z0-9._-]、不超过 128 字节。
const (
	failureNoSubscriber      eventing.FailureCode = "dispatch.no_subscriber"
	failureConsumerUndecided eventing.FailureCode = "dispatch.consumer_undecided"
	failurePublishUncertain  eventing.FailureCode = "dispatch.publish_uncertain"
	failurePublishFailed     eventing.FailureCode = "dispatch.publish_failed"
)

// failureCodeFor 把发布失败分格。合成一个码，运维读不出该改装配、该救下游，还是该去
// 下游核对重复投递——这三件的动作互不相同。
//
// FanOut 用 errors.Join 合并各路失败，合并错误可能同时带着未决与别的失败。分格取
// 最响的动作格：no_subscriber > publish_failed > publish_uncertain > consumer_undecided。
// 仅当全部失败路都未决才记 consumer_undecided——未决是「等消费方的那个依赖」，会随
// 依赖到位自愈；任何一路需要人动手或下游核对时，码面必须指向那件事，否则硬失败以
// 未决之名耗尽失败预算，事后排查从错的入口进（外部评审票 01）。
//
// publish_failed 排在 publish_uncertain 之前：硬失败重投不自愈，是几格里唯一必须
// 人动手的；不确定的那一路要么其实已提交（重投被消费门跳过而消失），要么重投得出
// 定论。码面钉在不自愈的那格，失败预算烧尽时它指向确实需要人的分支。
//
// 结果不确定单独一格：它与普通失败一样消耗失败预算并重投（框架合同如此），但重投可能
// 真的造成重复投递，处置要落在下游而不是这边。
func failureCodeFor(err error) eventing.FailureCode {
	// 无订阅者是装配错误，保持最响：哪怕只有一路撞上，先修装配。
	if errors.Is(err, ErrNoSubscriber) {
		return failureNoSubscriber
	}
	switch loudestLaneKind(err) {
	case laneUndecided:
		return failureConsumerUndecided
	case laneUncertain:
		return failurePublishUncertain
	default:
		return failurePublishFailed
	}
}

// laneKind 是一路失败的动作格。数值定序：越大越响，合并时取最大。
type laneKind int

const (
	laneUndecided laneKind = iota
	laneUncertain
	laneHard
)

// loudestLaneKind 走合并错误的展开树，取各失败路里最响的动作格。
//
// 树里有两种多路节点，形状相同、语义相反，靠「直接子节点是不是 ErrConsumerUndecided
// 本体」区分：
//
//   - WithUndecidedSentinels 的包装（哨兵与原错误都用 %w）：第一路是哨兵本体，第二路
//     是消费方的原错误。整棵是翻译过的**一路**未决，不再往下拆——拆开会把原错误错当
//     成另一路硬失败，而它只是「停在哪个依赖」的说明。
//   - fanOut.Consume 的 errors.Join：各路互不隶属，逐路归格取最响。
//
// 这个区分是稳的：消费方按方向约束不 import 平台哨兵（见 WithUndecidedSentinels 的
// 注释），join 的直接子节点因此不可能是哨兵本体，只有翻译层会把本体放进子节点。
// 判不出形状时一律落最响的 laneHard——宁可把未决记成失败让人来看，不可反向盖码。
func loudestLaneKind(err error) laneKind {
	for err != nil {
		if err == ErrConsumerUndecided {
			return laneUndecided
		}
		if err == eventing.ErrPublishUncertain {
			return laneUncertain
		}
		switch node := err.(type) {
		case interface{ Unwrap() []error }:
			children := node.Unwrap()
			if len(children) == 0 {
				return laneHard
			}
			for _, child := range children {
				if child == ErrConsumerUndecided {
					return laneUndecided
				}
			}
			loudest := laneUndecided
			for _, child := range children {
				if kind := loudestLaneKind(child); kind > loudest {
					loudest = kind
				}
			}
			return loudest
		case interface{ Unwrap() error }:
			err = node.Unwrap()
		default:
			// 叶子：identity 之外还留 errors.Is 兜自定义 Is 的实现。
			switch {
			case errors.Is(err, ErrConsumerUndecided):
				return laneUndecided
			case errors.Is(err, eventing.ErrPublishUncertain):
				return laneUncertain
			default:
				return laneHard
			}
		}
	}
	return laneHard
}

// Config 是一拍的节奏参数。零值不可用——批量上限与租约时长没有合理默认，装配方
// 必须显式给出（这不是业务阈值：它约束的是进程资源与重试节奏，属部署形态）。
type Config struct {
	// Limit 一拍最多认领多少条。
	Limit int
	// LeaseFor 租约时长：认领后这么久没定稿，别的派发器可以重新认领。
	LeaseFor time.Duration
	// MaxAttempts 交给框架的认领预算：失败次数达到它的条目不再被认领。
	MaxAttempts uint32
	// RetryAfter 发布失败后多久允许重试。
	RetryAfter time.Duration
}

func (config Config) validate() error {
	if config.Limit <= 0 || config.LeaseFor <= 0 || config.MaxAttempts == 0 || config.RetryAfter <= 0 {
		return errors.New("dispatch: config fields must all be positive")
	}
	return nil
}

// DeliveryFailureObserver 由装配方注入，用来看见逐条投递失败（ADR-0095 Decision 二）。
//
// 它交出三样：这一条投递、已分格的失败码、以及**原始错误**。第三样是要害——失败码按运维要做
// 的动作取值（见 failureCodeFor 的分格判据），本就不负责答「停在哪一站」，而消费方恰恰把
// 停在哪一站与未决原因都写在原始错误的正文里。丢掉它，那些字就没有任何人读得到。
//
// 平台层不决定怎么出声、也不决定出声到哪里：本包对事件类型一无所知，十个上下文共用同一拍，
// 循环与节奏本就是装配职责（见包注释）。这个口只把它手上已经有的那个 err 交出去。
type DeliveryFailureObserver func(delivery eventing.Delivery, code eventing.FailureCode, err error)

// Option 是 NewDispatcher 的可选装配项。用变参而不是多加一个位置参数，是为了让既有装配点
// 一个都不必改——ADR-0095 Decision 四要求「回调为 nil 时行为与今天逐字相同」。
type Option func(*Dispatcher)

// WithDeliveryFailureObserver 注入失败观察口。传 nil 等同于不注入。
func WithDeliveryFailureObserver(observe DeliveryFailureObserver) Option {
	return func(dispatcher *Dispatcher) { dispatcher.observeFailure = observe }
}

// Dispatcher 把已入队的信封逐条交给发布通道并定稿。
type Dispatcher struct {
	claimer        eventing.OutboxClaimer
	finalizer      eventing.OutboxFinalizer
	publisher      eventing.Publisher
	clock          Clock
	config         Config
	observeFailure DeliveryFailureObserver
}

func NewDispatcher(
	claimer eventing.OutboxClaimer,
	finalizer eventing.OutboxFinalizer,
	publisher eventing.Publisher,
	clock Clock,
	config Config,
	options ...Option,
) (*Dispatcher, error) {
	if claimer == nil || finalizer == nil || publisher == nil || clock == nil {
		return nil, errors.New("dispatch: all dependencies are required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	dispatcher := &Dispatcher{
		claimer:   claimer,
		finalizer: finalizer,
		publisher: publisher,
		clock:     clock,
		config:    config,
	}
	for _, option := range options {
		option(dispatcher)
	}
	return dispatcher, nil
}

// DispatchOnce 执行一拍：认领一批、逐条发布、逐条定稿。交回本拍成功发布的条数。
//
// 刻意只有一拍没有循环——循环（间隔、退避、停机信号）是进程装配的事，一拍是可以
// 在真库门禁里从头验到尾的最大单元。逐条定稿而不是攒批：MarkPublished 是幂等锚点，
// 攒批会让一条失败拖住同批已发布条目的定稿，重派时它们就被发布第二次。
//
// 发布失败记 RecordFailure（带重试时刻）不 Abandon：弃单意味着下游永远收不到这份
// 意图，那是运维在看过失败原因后的显式决定，不是重试预算耗尽的自动结果——预算
// 耗尽的条目由 Claim 的 MaxAttempts 拦在认领之外，留在库里可查。
func (dispatcher *Dispatcher) DispatchOnce(ctx context.Context) (int, error) {
	now := dispatcher.clock.Now().UTC()
	deliveries, err := dispatcher.claimer.Claim(ctx, eventing.OutboxClaim{
		Now:         now,
		Limit:       dispatcher.config.Limit,
		LeaseFor:    dispatcher.config.LeaseFor,
		MaxAttempts: dispatcher.config.MaxAttempts,
	})
	if err != nil {
		return 0, fmt.Errorf("dispatch: claim: %w", err)
	}

	published := 0
	for _, delivery := range deliveries {
		if err := dispatcher.publisher.Publish(ctx, delivery.Envelope); err != nil {
			failedAt := dispatcher.clock.Now().UTC()
			code := failureCodeFor(err)
			if recordErr := dispatcher.finalizer.RecordFailure(ctx, delivery.Ref, eventing.DeliveryFailure{
				FailedAt:    failedAt,
				RetryAt:     failedAt.Add(dispatcher.config.RetryAfter),
				Code:        code,
				MaxFailures: dispatcher.config.MaxAttempts,
			}); recordErr != nil {
				// 失败没记上：租约还在，条目会在租约过期后被重新认领——比静默
				// 丢失强，但要让装配方看见这一拍没走完。
				return published, fmt.Errorf("dispatch: record failure: %w", errors.Join(recordErr, err))
			}
			// 记上之后才观察：观察口报的是一条**已经入账**的失败，而不是一次可能还会
			// 回滚的尝试。它只观察，不改变派发——返回值不看，因此一条日志写不出去不会
			// 变成一次投递失败（ADR-0095 Decision 四）。
			dispatcher.observe(delivery, code, err)
			continue
		}
		if err := dispatcher.finalizer.MarkPublished(ctx, delivery.Ref, dispatcher.clock.Now().UTC()); err != nil {
			// 已发布没定稿：重派会发布第二次，消费侧的 inbox 恰一次门为此存在。
			// 报错让装配方知道出现了「至少一次」窗口。
			return published, fmt.Errorf("dispatch: mark published: %w", err)
		}
		published++
	}
	return published, nil
}

// observe 把一条已入账的失败交给装配方。没注入观察口时什么都不做——那是既有装配点的形状，
// ADR-0095 Decision 四要求它与今天逐字相同。
func (dispatcher *Dispatcher) observe(
	delivery eventing.Delivery,
	code eventing.FailureCode,
	err error,
) {
	if dispatcher.observeFailure == nil {
		return
	}
	dispatcher.observeFailure(delivery, code, err)
}
