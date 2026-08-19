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
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psnodeops "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
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

// Covers: 路由表第二条——PS 有效网络收寄采用结果投给复核消费者（UC-PS-003 步骤 8 →
// UC-NR-003）。手法同上一条：毒丸载荷让消费门显式拒收并交回 nil，因此这一条会被定稿；
// 挂错人或漏挂的话这里撞的是无订阅者，一条也发不出去。
//
// 两条并存本身也被这一对用例钉住：两个消费者的 inbox 账本按消费者名分家，路由表两条
// 各投各的，不会互相顶掉。
func TestAnAdoptedNetworkIntakeReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "adoption-1", nrinbox.AdoptedNetworkIntakeEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把采用结果投给复核消费者",
			published, recordedFailureCode(t, db, "adoption-1"))
	}
}

// Covers: 路由表第三条——NO 节点收寄形成投给 FanOut（先 VE 投影，后 PS 采用）。手法
// 同前两条：毒丸载荷（缺 tenantId/sourceId）让两边消费门都显式拒收入账并交回 nil，
// 因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。方向与前两条相反——信封由
// node-operations 发出，同一 EventType 两个独立 inbox。
func TestAFormedNodeIntakeReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "node-intake-1", psinbox.NodeIntakeFormedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把节点收寄投给 FanOut",
			published, recordedFailureCode(t, db, "node-intake-1"))
	}
}

// Covers: 路由表第四条——TF 对象级揽收登记投给 PS 采用消费者（UC-PS-003 的另一个合格
// 物理来源）。手法同前三条：毒丸载荷（缺 tenantId/object/attempt）让消费门显式拒收并
// 交回 nil，因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。
func TestARegisteredOffsitePickupReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "pickup-1", psinbox.OffsitePickupRegisteredEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把揽收登记投给采用消费者",
			published, recordedFailureCode(t, db, "pickup-1"))
	}
}

// Covers: 尝试级 `offsite-pickup.formed` 不得进采用路由。一封信带一批成功对象，而采用
// 判断逐对象成立；挂上这条会让同一份揽收结果被采用两次。未登记的类型必须撞
// dispatch.no_subscriber，而不是被第四路误吃。
func TestAnOffsitePickupFormedEnvelopeIsNotRouted(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "pickup-formed-1", "transport-fulfillment.offsite-pickup.formed", `{"ok":true}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("尝试级揽收信封被当成发布成功定稿了：published = %d", published)
	}
	if got := recordedFailureCode(t, db, "pickup-formed-1"); got != "dispatch.no_subscriber" {
		t.Fatalf("failure_code = %q, want dispatch.no_subscriber", got)
	}
}

// Covers: 路由表第五条——TF 有效交付登记投给 PS 终局消费者（UC-PS-004）。手法同前四
// 条：毒丸载荷（缺 tenantId/object/attempt）让消费门显式拒收并交回 nil，因此这一条会
// 被定稿。漏挂或挂错的话这里撞的是无订阅者。
func TestARegisteredEffectiveDeliveryReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "delivery-1", psinbox.EffectiveDeliveryRegisteredEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把有效交付投给终局消费者",
			published, recordedFailureCode(t, db, "delivery-1"))
	}
}

// Covers: 控制转出交接不得进终局路由。那一封属 node-operations 的控制转移，不是
// UC-PS-004 的 TF-DELIVERY 来源行；挂上会把控制事实当成有效交付。未登记必须撞
// dispatch.no_subscriber，而不是被第五路误吃。
func TestATransportHandoverRegisteredEnvelopeIsNotRouted(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "handover-1", "transport-fulfillment.transport-handover.registered", `{"ok":true}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("交接登记信封被当成发布成功定稿了：published = %d", published)
	}
	if got := recordedFailureCode(t, db, "handover-1"); got != "dispatch.no_subscriber" {
		t.Fatalf("failure_code = %q, want dispatch.no_subscriber", got)
	}
}

// stallingConsumer 是只会交回某个既定错误的直投接收方，用来验路由条目那层的失败分格。
type stallingConsumer struct{ err error }

func (consumer *stallingConsumer) Consume(context.Context, eventing.Envelope) error {
	return consumer.err
}

// beatWithNodeIntakeConsumer 用生产的哨兵名单包一个替身，接成一拍。
//
// 名单取 nodeIntakeUndecidedSentinels 本身而不是在测试里重列一遍：重列的那份漏掉某个
// 哨兵时测试照样绿，而生产会把它塌成 dispatch.publish_failed。
func beatWithNodeIntakeConsumer(t *testing.T, inner dispatch.Consumer) (Beat, *bentopg.DB, *outbox.Store) {
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
	routed, err := dispatch.WithUndecidedSentinels(inner, nodeIntakeUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包装未决哨兵：%v", err)
	}
	config := dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{psinbox.NodeIntakeFormedEventType: routed},
		5*time.Second,
		config,
	)
	if err != nil {
		t.Fatalf("直投发布器：%v", err)
	}
	beat, err := dispatch.NewDispatcher(store, store, publisher, systemClock{}, config)
	if err != nil {
		t.Fatalf("派发器：%v", err)
	}
	return beat, db, store
}

// Covers: 采用这条链的失败分格——只有登记过的四个哨兵落 dispatch.consumer_undecided，
// 交接待发不落。两者要运维做的事相反：前者去看消费方等的那个依赖（收寄可见性、目标
// 委托、资格目录、实物识别），后者去看 outbox 下游为什么没收下那份意图。
//
// 采用记录已提交而意图没交出去时，运维照着「消费方在等」去查商业资格目录会白查一整轮
// ——那一格里资格早就过了。
func TestNodeIntakeFailuresLandInTheRightPartition(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"资格未决", psnodeops.ErrAdoptionUndecided, "dispatch.consumer_undecided"},
		{"收寄还看不见", psnodeops.ErrReceptionNotVisible, "dispatch.consumer_undecided"},
		{"目标委托还没有", psnodeops.ErrParcelTargetNotFound, "dispatch.consumer_undecided"},
		{"实物还没识别", psnodeops.ErrUnidentifiedHandlingUnit, "dispatch.consumer_undecided"},
		{"交接待发", psnodeops.ErrAdoptionHandoffPending, "dispatch.publish_failed"},
		// 反查歧义要人去看为什么两份已接受委托声明了同一个包裹，不是等依赖到位。
		{"反查歧义", psdomain.ErrAmbiguousParcelTarget, "dispatch.publish_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithNodeIntakeConsumer(t, &stallingConsumer{err: test.err})
			enqueueForBeat(t, db, store, "node-intake-1", psinbox.NodeIntakeFormedEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "node-intake-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
			}
		})
	}
}

// beatWithOffsitePickupConsumer 用生产的揽收哨兵名单包一个替身，接成一拍。
//
// 名单取 offsitePickupUndecidedSentinels 本身：测试里重列一份会让漏登记的哨兵在测试
// 里绿、在生产里塌成 dispatch.publish_failed。
func beatWithOffsitePickupConsumer(t *testing.T, inner dispatch.Consumer) (Beat, *bentopg.DB, *outbox.Store) {
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
	routed, err := dispatch.WithUndecidedSentinels(inner, offsitePickupUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包装未决哨兵：%v", err)
	}
	config := dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{psinbox.OffsitePickupRegisteredEventType: routed},
		5*time.Second,
		config,
	)
	if err != nil {
		t.Fatalf("直投发布器：%v", err)
	}
	beat, err := dispatch.NewDispatcher(store, store, publisher, systemClock{}, config)
	if err != nil {
		t.Fatalf("派发器：%v", err)
	}
	return beat, db, store
}

// Covers: 揽收这条链的失败分格——三个可续办哨兵落 dispatch.consumer_undecided，另外三
// 格保持 dispatch.publish_failed。后三格要运维做的事不是「等依赖」：
//   - 键/本体不符是仓储不变量已破，重投不自愈；
//   - 交接待发要查 outbox 下游；
//   - 反查歧义要人去看为什么两份已接受委托声明了同一个包裹。
func TestOffsitePickupFailuresLandInTheRightPartition(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"资格未决", pstf.ErrAdoptionUndecided, "dispatch.consumer_undecided"},
		{"揽收还看不见", pstf.ErrPickupNotVisible, "dispatch.consumer_undecided"},
		{"目标委托还没有", pstf.ErrParcelTargetNotFound, "dispatch.consumer_undecided"},
		{"键与本体不符", pstf.ErrPickupRecordInconsistent, "dispatch.publish_failed"},
		{"交接待发", pstf.ErrAdoptionHandoffPending, "dispatch.publish_failed"},
		{"反查歧义", psdomain.ErrAmbiguousParcelTarget, "dispatch.publish_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithOffsitePickupConsumer(t, &stallingConsumer{err: test.err})
			enqueueForBeat(t, db, store, "pickup-1", psinbox.OffsitePickupRegisteredEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "pickup-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
			}
		})
	}
}

// beatWithEffectiveDeliveryConsumer 用生产的终局哨兵名单包一个替身，接成一拍。
//
// 名单取 effectiveDeliveryUndecidedSentinels 本身：测试里重列一份会让漏登记的哨兵在
// 测试里绿、在生产里塌成 dispatch.publish_failed。
func beatWithEffectiveDeliveryConsumer(t *testing.T, inner dispatch.Consumer) (Beat, *bentopg.DB, *outbox.Store) {
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
	routed, err := dispatch.WithUndecidedSentinels(inner, effectiveDeliveryUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包装未决哨兵：%v", err)
	}
	config := dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{psinbox.EffectiveDeliveryRegisteredEventType: routed},
		5*time.Second,
		config,
	)
	if err != nil {
		t.Fatalf("直投发布器：%v", err)
	}
	beat, err := dispatch.NewDispatcher(store, store, publisher, systemClock{}, config)
	if err != nil {
		t.Fatalf("派发器：%v", err)
	}
	return beat, db, store
}

// Covers: 终局这条链的失败分格——三个可续办哨兵落 dispatch.consumer_undecided，另外
// 几格保持 dispatch.publish_failed。后几格要运维做的事不是「等依赖」：
//   - 键/本体不符是仓储不变量已破，重投不自愈；
//   - 交接待发要查 outbox 下游；
//   - 反查歧义要人去看为什么两份已接受委托声明了同一个包裹；
//   - 词汇表外与封闭集合外都是编程错误，响亮而不是等。
func TestEffectiveDeliveryFailuresLandInTheRightPartition(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"终局未决", pstf.ErrFinalUndecided, "dispatch.consumer_undecided"},
		{"交付还看不见", pstf.ErrDeliveryNotVisible, "dispatch.consumer_undecided"},
		{"目标委托还没有", pstf.ErrParcelTargetNotFound, "dispatch.consumer_undecided"},
		{"键与本体不符", pstf.ErrDeliveryRecordInconsistent, "dispatch.publish_failed"},
		{"交接待发", pstf.ErrFinalHandoffPending, "dispatch.publish_failed"},
		{"反查歧义", psdomain.ErrAmbiguousParcelTarget, "dispatch.publish_failed"},
		{"词汇表外", pstf.ErrUntranslatableAnswer, "dispatch.publish_failed"},
		{"封闭集合外", finalconsume.ErrUnexpectedFinalOutcome, "dispatch.publish_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithEffectiveDeliveryConsumer(t, &stallingConsumer{err: test.err})
			enqueueForBeat(t, db, store, "delivery-1", psinbox.EffectiveDeliveryRegisteredEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "delivery-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
			}
		})
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
