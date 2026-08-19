package dispatch

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
)

// FanOut 把一份信封交给多个独立消费者。平台仍然对事件类型一无所知——谁订阅哪一类
// 由组合根把本函数的返回值放进 DirectPublisher 路由表。
//
// 每一路都要调到，即使前面已经失败。两本 inbox 互不隶属，不能因为 A 停在实例墙
// 就把 B 的入账也卡住；已提交的那路靠 inbox 在重投时跳过。任一失败都让 Publish
// 失败，派发器会重试。
//
// 调用顺序由调用方给定。FanOut 不翻错误码——未决哨兵仍由 WithUndecidedSentinels
// 在各路由入口自己翻。
func FanOut(consumers ...Consumer) (Consumer, error) {
	if len(consumers) == 0 {
		return nil, errors.New("dispatch: fan-out needs at least one consumer")
	}
	copies := make([]Consumer, len(consumers))
	for i, consumer := range consumers {
		if consumer == nil {
			return nil, fmt.Errorf("dispatch: fan-out consumer %d is nil", i)
		}
		copies[i] = consumer
	}
	return &fanOut{consumers: copies}, nil
}

type fanOut struct {
	consumers []Consumer
}

func (fan *fanOut) Consume(ctx context.Context, envelope eventing.Envelope) error {
	var joined error
	for _, consumer := range fan.consumers {
		if err := consumer.Consume(ctx, envelope); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}
