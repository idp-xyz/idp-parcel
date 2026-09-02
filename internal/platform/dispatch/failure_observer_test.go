package dispatch_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件钉住 ADR-0095 Decision 二：逐条投递失败要把**原始错误**交给装配方，而不是记完失败码
// 就把它丢掉。
//
// 缺了它的后果实测过：消费门把停在哪一站与未决原因都写进错误正文，其注释说「原错误原样留在
// 链上供运维读」，而生产接线里没有任何东西在读——dispatch 连跑 11 分钟、库里累计 24 次失败、
// 进程输出零行。

var errObservedPublish = errors.New("观察口探针：发布失败")

func newObservedDispatcher(t *testing.T, observe dispatch.DeliveryFailureObserver) (
	*dispatch.Dispatcher, *outbox.Store, *publisherDouble, *movableClock, *bentopg.DB,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	publisher := &publisherDouble{}
	clock := &movableClock{at: time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)}
	dispatcher, err := dispatch.NewDispatcher(store, store, publisher, clock, dispatch.Config{
		Limit:       10,
		LeaseFor:    time.Minute,
		MaxAttempts: 5,
		RetryAfter:  30 * time.Second,
	}, dispatch.WithDeliveryFailureObserver(observe))
	if err != nil {
		t.Fatalf("构造派发器：%v", err)
	}
	return dispatcher, store, publisher, clock, db
}

// TestAFailedDeliveryHandsTheOriginalErrorToTheAssembler 是本记录的要害：交出去的必须是
// **原始错误**，不是那个已经分过格的失败码。
//
// 失败码按运维要做的动作取值，本就不负责答「停在哪一站」；两者都要，而只有原始错误里有后者。
func TestAFailedDeliveryHandsTheOriginalErrorToTheAssembler(t *testing.T) {
	type observation struct {
		code eventing.FailureCode
		err  error
	}
	var seen []observation

	dispatcher, store, publisher, clock, db := newObservedDispatcher(t,
		func(_ eventing.Delivery, code eventing.FailureCode, err error) {
			seen = append(seen, observation{code: code, err: err})
		})
	publisher.err = errObservedPublish
	enqueueEnvelope(t, db, store, "observed-1", clock.at)

	if _, err := dispatcher.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍不该整拍失败：%v", err)
	}

	if len(seen) != 1 {
		t.Fatalf("观察口应被调用一次，实际 %d 次", len(seen))
	}
	if !errors.Is(seen[0].err, errObservedPublish) {
		t.Fatalf("交出去的应是原始错误，实际 %v", seen[0].err)
	}
	if seen[0].code == "" {
		t.Fatal("已分格的失败码也该一并交出——运维要的两样缺一不可")
	}
}

// TestASuccessfulDeliveryDoesNotCallTheObserver 守住「只观察失败」这一格：把成功也报出去
// 会让日志里那条失败淹掉，而它本来就是唯一值得看的东西。
func TestASuccessfulDeliveryDoesNotCallTheObserver(t *testing.T) {
	called := 0

	dispatcher, store, _, clock, db := newObservedDispatcher(t,
		func(_ eventing.Delivery, _ eventing.FailureCode, _ error) { called++ })
	enqueueEnvelope(t, db, store, "observed-ok", clock.at)

	if _, err := dispatcher.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍不该失败：%v", err)
	}

	if called != 0 {
		t.Fatalf("成功投递不该惊动观察口，实际被调用 %d 次", called)
	}
}

// TestANilObserverKeepsTodaysBehaviour 钉住 ADR-0095 Decision 四那半：回调为 nil 时行为与
// 今天逐字相同。既有装配点一个都不必改，靠的就是这一条。
func TestANilObserverKeepsTodaysBehaviour(t *testing.T) {
	dispatcher, store, publisher, clock, db := newObservedDispatcher(t, nil)
	publisher.err = errObservedPublish
	enqueueEnvelope(t, db, store, "observed-nil", clock.at)

	if _, err := dispatcher.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("没有观察口时一拍仍不该整拍失败：%v", err)
	}
}
