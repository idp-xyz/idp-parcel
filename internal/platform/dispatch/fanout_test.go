package dispatch_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// 本文件证 FanOut：一类信封投给多个独立消费者。平台仍对事件类型一无所知；每个
// 消费者都要调到，即使前面已经失败——两本 inbox 互不隶属。

type recordingConsumer struct {
	name  string
	err   error
	calls int
	order *[]string
}

func (double *recordingConsumer) Consume(_ context.Context, _ eventing.Envelope) error {
	double.calls++
	if double.order != nil {
		*double.order = append(*double.order, double.name)
	}
	return double.err
}

func TestFanOutCallsEveryConsumerInCallerOrder(t *testing.T) {
	var order []string
	first := &recordingConsumer{name: "first", order: &order}
	second := &recordingConsumer{name: "second", order: &order}
	fan, err := dispatch.FanOut(first, second)
	if err != nil {
		t.Fatalf("构造 FanOut：%v", err)
	}

	if err := fan.Consume(t.Context(), directEnvelope(directEventType)); err != nil {
		t.Fatalf("全部成功应收 nil：%v", err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("调用次数 first=%d second=%d, want 1/1", first.calls, second.calls)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("顺序 = %v, want [first second]", order)
	}
}

// Covers: 先失败后成功仍两者都调。CONS-PROJ-B 会先 VE 后 PS；反过来这份顺序也必须
// 把后面那路送到，已入账的靠 inbox 跳过，不能因为前面失败就停。
func TestFanOutStillCallsTheRestAfterAnEarlierFailure(t *testing.T) {
	firstErr := errors.New("first consumer refused")
	var order []string
	first := &recordingConsumer{name: "first", err: firstErr, order: &order}
	second := &recordingConsumer{name: "second", order: &order}
	fan, err := dispatch.FanOut(first, second)
	if err != nil {
		t.Fatalf("构造 FanOut：%v", err)
	}

	err = fan.Consume(t.Context(), directEnvelope(directEventType))
	if !errors.Is(err, firstErr) {
		t.Fatalf("err = %v, want first consumer's error", err)
	}
	if errors.Is(err, dispatch.ErrConsumerUndecided) {
		t.Fatal("FanOut 不得把失败翻成未决哨兵")
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("调用次数 first=%d second=%d, want 1/1——前面失败也要投后面", first.calls, second.calls)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("顺序 = %v, want [first second]", order)
	}
}

func TestFanOutReturnsALaterFailureAfterAnEarlierSuccess(t *testing.T) {
	secondErr := errors.New("second consumer refused")
	first := &recordingConsumer{name: "first"}
	second := &recordingConsumer{name: "second", err: secondErr}
	fan, err := dispatch.FanOut(first, second)
	if err != nil {
		t.Fatalf("构造 FanOut：%v", err)
	}

	err = fan.Consume(t.Context(), directEnvelope(directEventType))
	if !errors.Is(err, secondErr) {
		t.Fatalf("err = %v, want second consumer's error", err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("调用次数 first=%d second=%d, want 1/1", first.calls, second.calls)
	}
}

func TestFanOutJoinsFailuresWithoutTranslatingThem(t *testing.T) {
	firstErr := errors.New("first refused")
	secondErr := errors.New("second refused")
	first := &recordingConsumer{err: firstErr}
	second := &recordingConsumer{err: secondErr}
	fan, err := dispatch.FanOut(first, second)
	if err != nil {
		t.Fatalf("构造 FanOut：%v", err)
	}

	err = fan.Consume(t.Context(), directEnvelope(directEventType))
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("err = %v, 两路失败都要能 errors.Is 到，且不得翻码", err)
	}
	if errors.Is(err, dispatch.ErrConsumerUndecided) || errors.Is(err, dispatch.ErrNoSubscriber) {
		t.Fatal("FanOut 不得把失败翻成平台失败码")
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("调用次数 first=%d second=%d, want 1/1", first.calls, second.calls)
	}
}

func TestFanOutPassesThroughAnAlreadyTranslatedUndecided(t *testing.T) {
	sentinel := errors.New("stage rules unconfigured")
	inner, err := dispatch.WithUndecidedSentinels(
		&recordingConsumer{err: sentinel},
		sentinel,
	)
	if err != nil {
		t.Fatalf("包装未决：%v", err)
	}
	ok := &recordingConsumer{}
	fan, err := dispatch.FanOut(inner, ok)
	if err != nil {
		t.Fatalf("构造 FanOut：%v", err)
	}

	err = fan.Consume(t.Context(), directEnvelope(directEventType))
	if !errors.Is(err, dispatch.ErrConsumerUndecided) {
		t.Fatalf("err = %v, 路由层已翻过的未决必须原样透出", err)
	}
	if ok.calls != 1 {
		t.Fatalf("后面那路调用 = %d, want 1", ok.calls)
	}
}

func TestFanOutRejectsEmptyOrNilConsumers(t *testing.T) {
	if _, err := dispatch.FanOut(); err == nil {
		t.Fatal("空 FanOut 被接受了")
	}
	var missing dispatch.Consumer
	if _, err := dispatch.FanOut(missing); err == nil {
		t.Fatal("含 nil 的 FanOut 被接受了")
	}
	if _, err := dispatch.FanOut(&recordingConsumer{}, nil); err == nil {
		t.Fatal("第二个消费者为 nil 的 FanOut 被接受了")
	}
}
