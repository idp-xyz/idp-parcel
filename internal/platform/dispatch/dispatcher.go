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

// Dispatcher 把已入队的信封逐条交给发布通道并定稿。
type Dispatcher struct {
	claimer   eventing.OutboxClaimer
	finalizer eventing.OutboxFinalizer
	publisher eventing.Publisher
	clock     Clock
	config    Config
}

func NewDispatcher(
	claimer eventing.OutboxClaimer,
	finalizer eventing.OutboxFinalizer,
	publisher eventing.Publisher,
	clock Clock,
	config Config,
) (*Dispatcher, error) {
	if claimer == nil || finalizer == nil || publisher == nil || clock == nil {
		return nil, errors.New("dispatch: all dependencies are required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Dispatcher{
		claimer:   claimer,
		finalizer: finalizer,
		publisher: publisher,
		clock:     clock,
		config:    config,
	}, nil
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
			if recordErr := dispatcher.finalizer.RecordFailure(ctx, delivery.Ref, eventing.DeliveryFailure{
				FailedAt:    failedAt,
				RetryAt:     failedAt.Add(dispatcher.config.RetryAfter),
				Code:        "dispatch.publish_failed",
				MaxFailures: dispatcher.config.MaxAttempts,
			}); recordErr != nil {
				// 失败没记上：租约还在，条目会在租约过期后被重新认领——比静默
				// 丢失强，但要让装配方看见这一拍没走完。
				return published, fmt.Errorf("dispatch: record failure: %w", errors.Join(recordErr, err))
			}
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
