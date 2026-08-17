package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证组合根本身：整张依赖图接得起来、一拍跑得通、路由表
// 认得出该认的那一类、认不得的那一类不被静默丢弃。
//
// 组合根值得有自己的测试，因为它是唯一「只能靠进程起停验证」的地方——而那等于没有
// 验证。wireDispatcher 与读环境分开正是为了让这一层验得动。

// beatInstant 取一个刚过去的真实时刻。组合根装的是 systemClock（一拍要真实时钟，
// 那是它的生产实现），所以入队时刻必须与真实时间对齐——用一个写死的未来时刻，认领
// 会一条都取不到，而每个「published = 0」的断言都会假绿。
func beatInstant() time.Time { return time.Now().UTC().Add(-time.Minute) }

func wiredBeat(t *testing.T) (Beat, *bentopg.DB, *outbox.Store) {
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

	purpose, err := nrdomain.NewServicePurpose("NETWORK_SERVICE")
	if err != nil {
		t.Fatalf("服务目的：%v", err)
	}
	beat, err := wireDispatcher(db, dispatchSettings{
		purpose:         purpose,
		deliveryTimeout: 5 * time.Second,
		config: dispatch.Config{
			Limit:       10,
			LeaseFor:    time.Minute,
			MaxAttempts: 5,
			RetryAfter:  30 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("装配派发器：%v", err)
	}
	return beat, db, store
}

func enqueueForBeat(t *testing.T, db *bentopg.DB, store *outbox.Store, id string, eventType eventing.EventType, payload string) {
	t.Helper()

	at := beatInstant()
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(id),
		Source:       "idp-parcel/assemble-test",
		Type:         eventType,
		Version:      1,
		Scope:        "tenant-a",
		Subject:      "subject-1",
		PartitionKey: "tenant-a/" + id,
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload:      []byte(payload),
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return store.Enqueue(txCtx, envelope)
	}); err != nil {
		t.Fatalf("入队 %s：%v", id, err)
	}
}

// Covers: 整张依赖图接得起来且一拍跑得通。空 Outbox 交回零条不是废话——它证的是
// 认领这一步真的打到了库上，而不是装配在某个 nil 依赖上悄悄成立。
func TestTheComposedDispatcherRunsABeatAgainstARealDatabase(t *testing.T) {
	beat, _, _ := wiredBeat(t)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("空 Outbox 却发布了 %d 条", published)
	}
}

// Covers: 路由表认得出接受决定这一类，且投递真的走完了消费门。
//
// 用毒丸载荷：消费者解不出命令时在自己的事务里显式拒收并交回 nil——那就是 ADR-0049
// 第二条要的「消费门事务已提交」，因此这一条会被定稿。它同时证明了路由表挂对了人：
// 挂错的话这里撞的是无订阅者，一条也发不出去。
//
// 载荷是结构合法但要件皆空的对象：框架在入队处就要求载荷是 JSON 对象，所以毒丸不能
// 用一段坏字节来造，得让它坏在本上下文的要件上。
func TestAnAcceptanceEnvelopeReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "accepted-1", nrinbox.AcceptedDecisionEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把接受决定投给消费者",
			published, recordedFailureCode(t, db, "accepted-1"))
	}
}

func recordedFailureCode(t *testing.T, db *bentopg.DB, eventID string) string {
	t.Helper()

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var code *string
	query := `SELECT failure_code FROM ` + migrate.SchemaBento + `.outbox WHERE event_id = $1`
	if err := querier.QueryRow(t.Context(), query, eventID).Scan(&code); err != nil {
		t.Fatalf("读回失败码：%v", err)
	}
	if code == nil {
		return ""
	}
	return *code
}

// Covers: ADR-0049 第三条——没有订阅者的类型显式失败并入账，不静默丢弃。它此刻会
// 阻塞自己那个分区直到失败预算耗尽，那是记录里认下的代价，不是缺陷。
func TestAnUnroutedEnvelopeIsNotSilentlyDropped(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "unrouted-1", "some.context.unsubscribed.event", `{"ok":true}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("没有订阅者的信封被当成发布成功定稿了：published = %d", published)
	}
	// 光看「没发布」不够——一条根本没被认领的信封也是零。失败码证明它确实走到了
	// 发布这一步并被显式记账。
	if got := recordedFailureCode(t, db, "unrouted-1"); got != "dispatch.no_subscriber" {
		t.Fatalf("failure_code = %q, want dispatch.no_subscriber", got)
	}
}
