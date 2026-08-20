package dispatch

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-bento-go/eventing"
)

// WithUndecidedSentinels 把某个消费者自己的未决哨兵翻成 ErrConsumerUndecided。
//
// 翻译落在路由条目这一层，不落在消费者那一侧，也不落在派发器里：
//
//   - 派发器只认 ErrConsumerUndecided 一个哨兵（见 failureCodeFor）。让它认识每个
//     上下文的哨兵，等于平台层要知道十个上下文各自的错误学。
//   - 消费者也不该反过来 import 平台层的哨兵：那是消费方依赖派发机制，方向反了。
//
// 于是只剩组合根：它本来就同时知道「这条路由挂的是谁」与「那个人的未决长什么样」。
// 不翻的后果是具体的——消费者的未决会落成 dispatch.publish_failed，运维读到的是
// 「发布失败」，于是去查传输，而实际要查的是消费方等的那个依赖。
func WithUndecidedSentinels(consumer Consumer, sentinels ...error) (Consumer, error) {
	if consumer == nil {
		return nil, errors.New("dispatch: consumer is required")
	}
	if len(sentinels) == 0 {
		return nil, errors.New("dispatch: at least one undecided sentinel is required")
	}
	for _, sentinel := range sentinels {
		if sentinel == nil {
			return nil, errors.New("dispatch: undecided sentinel is nil")
		}
	}
	return &undecidedTranslator{
		consumer:  consumer,
		sentinels: append([]error(nil), sentinels...),
	}, nil
}

type undecidedTranslator struct {
	consumer  Consumer
	sentinels []error
}

// Consume 原样转交，只在错误回来时补一层翻译。刻意不吞掉原错误——ErrConsumerUndecided
// 决定失败码，原错误说明消费方停在哪个依赖上，运维两样都要。
func (translator *undecidedTranslator) Consume(ctx context.Context, envelope eventing.Envelope) error {
	err := translator.consumer.Consume(ctx, envelope)
	if err == nil {
		return nil
	}
	for _, sentinel := range translator.sentinels {
		if errors.Is(err, sentinel) {
			// 两个都用 %w：失败码看 ErrConsumerUndecided，排查看原错误。只包前者
			// 会把「消费方停在哪个依赖上」这句话丢掉。
			//
			// 哨兵本体留在直接子节点还是分格的结构锚点——loudestLaneKind 靠它把
			// 「翻译过的一路未决」与 FanOut 的合并区分开。改包装形状前先看那边。
			return fmt.Errorf("%w: %w", ErrConsumerUndecided, err)
		}
	}
	return err
}
