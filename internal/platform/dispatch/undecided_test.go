package dispatch_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// 消费方自己的未决哨兵（形如 NR 的 ErrRouteHandoffUndecided），派发器认不得。
var errConsumerOwnUndecided = errors.New("some context: handoff is undecided")

// Covers: 未决要落在 dispatch.consumer_undecided 而不是 dispatch.publish_failed——
// 两者要运维做的事相反：前者查消费方等的那个依赖，后者查传输。
func TestAConsumerOwnUndecidedSentinelIsTranslated(t *testing.T) {
	inner := &consumerDouble{err: errConsumerOwnUndecided}
	consumer, err := dispatch.WithUndecidedSentinels(inner, errConsumerOwnUndecided)
	if err != nil {
		t.Fatalf("包装：%v", err)
	}

	err = consumer.Consume(context.Background(), directEnvelope(directEventType))
	if !errors.Is(err, dispatch.ErrConsumerUndecided) {
		t.Fatalf("err = %v, want ErrConsumerUndecided", err)
	}
	if !errors.Is(err, errConsumerOwnUndecided) {
		t.Fatal("翻译吞掉了原错误——运维读不出消费方停在哪个依赖上")
	}
}

// Covers: 只翻声明过的那些哨兵。把所有失败都翻成未决，等于宣称下游永远只是「在等」，
// 而真正的失败就再也看不见了。
func TestAnUndeclaredFailureIsNotTranslatedIntoUndecided(t *testing.T) {
	broken := errors.New("some context: handler blew up")
	inner := &consumerDouble{err: broken}
	consumer, err := dispatch.WithUndecidedSentinels(inner, errConsumerOwnUndecided)
	if err != nil {
		t.Fatalf("包装：%v", err)
	}

	err = consumer.Consume(context.Background(), directEnvelope(directEventType))
	if !errors.Is(err, broken) {
		t.Fatalf("err = %v，原错误没有透出来", err)
	}
	if errors.Is(err, dispatch.ErrConsumerUndecided) {
		t.Fatal("一个真实失败被翻成了未决")
	}
}

func TestASuccessfulConsumeIsUntouched(t *testing.T) {
	inner := &consumerDouble{}
	consumer, err := dispatch.WithUndecidedSentinels(inner, errConsumerOwnUndecided)
	if err != nil {
		t.Fatalf("包装：%v", err)
	}

	if err := consumer.Consume(context.Background(), directEnvelope(directEventType)); err != nil {
		t.Fatalf("consume: %v", err)
	}
	if len(inner.consumed) != 1 {
		t.Fatal("包装层没有把投递转交下去")
	}
}

// Covers: 一个哨兵都不声明的包装没有意义，且会让人以为翻译已经装上了——构造期就拒。
func TestWrappingWithoutASentinelIsRefused(t *testing.T) {
	if _, err := dispatch.WithUndecidedSentinels(&consumerDouble{}); err == nil {
		t.Fatal("没有哨兵的包装被接受了")
	}
	if _, err := dispatch.WithUndecidedSentinels(nil, errConsumerOwnUndecided); err == nil {
		t.Fatal("nil 消费者被接受了")
	}
}
