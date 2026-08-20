package dispatch_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// 本文件对真实 PostgreSQL 16 证 FanOut 合并失败落库的失败码：仅当全部失败路都未决才记
// dispatch.consumer_undecided；出现任何非未决失败时按那一路自身分格，取最响的动作格
// （no_subscriber > publish_failed > publish_uncertain > consumer_undecided）。
//
// 出处：外部评审票 01——未决盖过硬失败时，运维按码面去查消费方等的那个依赖，而真正
// 需要人动手的是另一路的数据/装配问题；硬失败若持续，条目以 consumer_undecided 之名
// 耗尽失败预算，事后排查也从错的入口进。
//
// 测试链与真实装配同构：各路先包自己的哨兵（WithUndecidedSentinels）再 FanOut，整棵
// 挂进 DirectPublisher 路由表，经派发一拍落库读失败码。不手搓合并错误——DirectPublisher
// 的外层包装也在场，只在裸 join 上判对的实现过不了这里。

// undecidedLane 造一路「停在自己依赖上」的消费者：哨兵经 WithUndecidedSentinels 在
// 路由入口翻成 ErrConsumerUndecided，与真实装配同构（各路先包自己的哨兵再 FanOut）。
func undecidedLane(t *testing.T) dispatch.Consumer {
	t.Helper()

	sentinel := errors.New("this lane waits on its own dependency")
	lane, err := dispatch.WithUndecidedSentinels(&recordingConsumer{err: sentinel}, sentinel)
	if err != nil {
		t.Fatalf("包装未决路：%v", err)
	}
	return lane
}

func TestAJoinedFailureRecordsUndecidedOnlyWhenEveryLaneIsUndecided(t *testing.T) {
	// 重投不自愈的不变量破坏：需要人动手的那一格。
	hardErr := errors.New("lane record inconsistent, redelivery will not heal it")
	// 一路内层发布回执丢失：重投可能真的造成重复投递，要下游核对。
	uncertainErr := fmt.Errorf("lane inner ack lost: %w", eventing.ErrPublishUncertain)

	tests := []struct {
		name     string
		lanes    func(t *testing.T) []dispatch.Consumer
		wantCode string
	}{
		{
			// 票面主害：硬失败不得被另一路的未决盖码，否则运维从「查依赖」入口进，
			// 而真正要动手的是数据/装配问题。
			name: "未决路加硬失败路记硬失败",
			lanes: func(t *testing.T) []dispatch.Consumer {
				return []dispatch.Consumer{undecidedLane(t), &recordingConsumer{err: hardErr}}
			},
			wantCode: "dispatch.publish_failed",
		},
		{
			// 核对重复投递的提示不得被未决盖掉。
			name: "未决路加不确定路记不确定",
			lanes: func(t *testing.T) []dispatch.Consumer {
				return []dispatch.Consumer{undecidedLane(t), &recordingConsumer{err: uncertainErr}}
			},
			wantCode: "dispatch.publish_uncertain",
		},
		{
			// 非未决各格的相对优先级：硬失败重投不自愈、必须人动手；不确定的那一路
			// 要么其实已提交（重投被消费门跳过），要么重投得出定论。码面钉在不自愈
			// 的那格，失败预算烧尽时指向确实需要人的分支。
			name: "不确定路加硬失败路记硬失败",
			lanes: func(t *testing.T) []dispatch.Consumer {
				return []dispatch.Consumer{&recordingConsumer{err: uncertainErr}, &recordingConsumer{err: hardErr}}
			},
			wantCode: "dispatch.publish_failed",
		},
		{
			name: "全部路未决才记未决",
			lanes: func(t *testing.T) []dispatch.Consumer {
				return []dispatch.Consumer{undecidedLane(t), undecidedLane(t)}
			},
			wantCode: "dispatch.consumer_undecided",
		},
		{
			// 单路未决、另一路成功：合并错误里只有未决这一路，语义不变。
			name: "单路未决记未决",
			lanes: func(t *testing.T) []dispatch.Consumer {
				return []dispatch.Consumer{undecidedLane(t), &recordingConsumer{}}
			},
			wantCode: "dispatch.consumer_undecided",
		},
		{
			name: "单路硬失败记硬失败",
			lanes: func(t *testing.T) []dispatch.Consumer {
				return []dispatch.Consumer{&recordingConsumer{err: hardErr}, &recordingConsumer{}}
			},
			wantCode: "dispatch.publish_failed",
		},
		{
			// 无订阅者是装配错误，保持最响：哪怕只有一路撞上（消费方内层再投递时
			// 撞了自己的路由表），也先修装配。
			name: "未决路加嵌套无订阅者记无订阅者",
			lanes: func(t *testing.T) []dispatch.Consumer {
				nested := fmt.Errorf("nested route: %w", dispatch.ErrNoSubscriber)
				return []dispatch.Consumer{undecidedLane(t), &recordingConsumer{err: nested}}
			},
			wantCode: "dispatch.no_subscriber",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, store, _, clock, db := newDispatchFixture(t)

			fan, err := dispatch.FanOut(test.lanes(t)...)
			if err != nil {
				t.Fatalf("构造 FanOut：%v", err)
			}
			config := dispatch.Config{
				Limit:       10,
				LeaseFor:    time.Minute,
				MaxAttempts: 5,
				RetryAfter:  30 * time.Second,
			}
			direct, err := dispatch.NewDirectPublisher(
				map[eventing.EventType]dispatch.Consumer{"dispatch.test.event": fan},
				10*time.Second, config)
			if err != nil {
				t.Fatalf("构造直投适配器：%v", err)
			}
			fanDispatcher, err := dispatch.NewDispatcher(store, store, direct, clock, config)
			if err != nil {
				t.Fatalf("构造派发器：%v", err)
			}

			enqueueEnvelope(t, db, store, "event-1", clock.at)
			published, err := fanDispatcher.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("失败拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("published = %d，want 0——任一路失败整封不得定稿", published)
			}
			if got := recordedFailureCode(t, db, "event-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q，want %q", got, test.wantCode)
			}
		})
	}
}
