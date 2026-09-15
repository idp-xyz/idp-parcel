package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccinbox "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/inbox"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccsettlement "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/settlementaccounting"
	ccapplication "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	ppinbox "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/inbox"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppsettlement "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/settlementaccounting"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psnodeops "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
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

// options 原样转给 wireDispatcher：多数用例不需要观察口，变参让它们一个都不必改。
func wiredBeat(t *testing.T, options ...dispatch.Option) (Beat, *bentopg.DB, *outbox.Store) {
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
	}, options...)
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

// Covers: 路由表认得出接受决定这一类，且投递走完了 FanOut 两路消费门（先 VE 客户
// 归属确立补派生，后 NR 初始路由）。
//
// 用毒丸载荷：两边消费者解不出命令时各自在自己的事务里显式拒收并交回 nil——那就是
// ADR-0049 第二条要的「消费门事务已提交」，因此这一条会被定稿。它同时证明了路由表
// 挂对了人：挂错或漏挂的话这里撞的是无订阅者，一条也发不出去。
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

// Covers: 路由表第四条——TF 对象级揽收登记投给 FanOut（先 VE 投影，后 PS 采用）。手法
// 同前三条：毒丸载荷（缺 tenantId/object/attempt）让两边消费门都显式拒收入账并交回
// nil，因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。
func TestARegisteredOffsitePickupReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "pickup-1", psinbox.OffsitePickupRegisteredEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把揽收登记投给 FanOut",
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

// Covers: 路由表第五条——TF 有效交付登记投给 FanOut（先 VE 投影，后 PS 终局）。手法
// 同前四条：毒丸载荷（缺 tenantId/object/attempt）让两边消费门都显式拒收入账并交回
// nil，因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。
func TestARegisteredEffectiveDeliveryReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "delivery-1", psinbox.EffectiveDeliveryRegisteredEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把有效交付投给 FanOut",
			published, recordedFailureCode(t, db, "delivery-1"))
	}
}

// Covers: 路由表的面单交易判断意图一条（lc/26，ADR-0134）——PS 写侧在定案 / 后续动作两拍交出的
// `label-transaction.judgment-due` 投给判断消费者。手法同前几条：毒丸载荷（缺 tenantId/transaction/parcel）
// 让消费门显式拒收入账并交回 nil，因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。
func TestALabelTransactionJudgmentDueReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "label-judgment-1", psinbox.LabelTransactionJudgmentDueEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把面单交易判断意图投给消费者",
			published, recordedFailureCode(t, db, "label-judgment-1"))
	}
}

// Covers: 路由表的关闭 / 重开决定判断意图一条（lc/27，ADR-0134）——PS 写侧在决定落册后交出的
// `continued-attempt-decision.judgment-due` 投给它自己的判断消费者。手法同前几条：毒丸载荷（缺
// tenantId/parcel/decision）让消费门显式拒收入账并交回 nil，因此这一条会被定稿。漏挂或挂错的话这里撞的是
// 无订阅者；挂到 26 那扇门上的话，26 的消费者按类型响亮拒收，这里同样不会 published = 1。
func TestAContinuedAttemptDecisionJudgmentDueReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "decision-judgment-1", psinbox.ContinuedAttemptDecisionJudgmentDueEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把关闭 / 重开决定判断意图投给消费者",
			published, recordedFailureCode(t, db, "decision-judgment-1"))
	}
}

// Covers: 路由表的 TF 实际承运商首次有效收寄登记一条（lc/25，ADR-0135）与它在生产依赖图上的未决翻译——
// 正：毒丸载荷（缺 tenantId/fact/version）让消费门显式拒收入账并交回 nil，这一条定稿；漏挂或挂错的话这里撞的
// 是无订阅者。反：三维齐全但 TF 登记册里没有那一代 → 可见性滞后是未决，不定稿、不毒丸，失败码落
// dispatch.consumer_undecided——这一格证的是 carrierFirstEffectivePickupJudgmentUndecidedSentinels 真接在路由条目上，
// 而不是错落成 publish_failed。
func TestACarrierFirstEffectivePickupRegisteredReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "carrier-pickup-1", psinbox.CarrierFirstEffectivePickupRegisteredEventType, `{}`)
	enqueueForBeat(t, db, store, "carrier-pickup-2", psinbox.CarrierFirstEffectivePickupRegisteredEventType,
		`{"tenantId":"tenant-a","fact":"CFEP-1","version":"CFEV-1","object":"parcel-1"}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1（正：毒丸定稿；反：读不回的那一代未决）；失败码 毒丸 = %q，未决 = %q",
			published, recordedFailureCode(t, db, "carrier-pickup-1"), recordedFailureCode(t, db, "carrier-pickup-2"))
	}
	if got := recordedFailureCode(t, db, "carrier-pickup-2"); got != "dispatch.consumer_undecided" {
		t.Fatalf("TF 登记册里没有的那一代：failure_code = %q, want dispatch.consumer_undecided", got)
	}
}

// Covers: PS 消费者自写的收寄登记类型串与 TF outbox 适配器写出的那一个相等（lc/27「取舍两处」①同一取舍：
// 消费者不 import 提供方，两串各写一份，由一条对照用例钉住）。对照落在本包而不是 PS `adapters/inbox` 的测试里，
// 因为只有组合根本来就同时装配两边——PS 的 inbox 包不为一条断言去 import 另一个上下文的持久化适配器。
// 任一侧改一字这里就红；路由表引的是 psinbox 那一个，这里一红，路由表认的类型就与 TF 写出的对不上。
func TestTheCarrierPickupConsumerAndTheTFHandoffAgreeOnTheEventType(t *testing.T) {
	if string(psinbox.CarrierFirstEffectivePickupRegisteredEventType) != tfpostgres.CarrierFirstEffectivePickupRegisteredEventType {
		t.Fatalf("PS 消费者认 %q，TF 写出 %q",
			psinbox.CarrierFirstEffectivePickupRegisteredEventType, tfpostgres.CarrierFirstEffectivePickupRegisteredEventType)
	}
}

// Covers: 路由表的 SA 资金事实采用一条（sa-cc/03）与完成判据 3「真库装配用例一正一反」——在生产依赖图上：
// 正：SA 采用过的事实（带来源提供的付款人）经 `external-funds-fact.adopted` 引用式信封到 CC 消费者，按
// （租户 + 事实 + 版本）回查 SA 只读视图、译成入向登记，CC 入向登记册按引用读回信封那一版且来源 / 付款人 / 币种 /
// 金额 / 版本照 SA 转述（`customs_compliance/0021` 起落的是身份行 + 版本子表各一行）；正例第二格（更正版本 v2
// 落第二行、回指 v1）与第三格（v2 到达 → 既往核对谱系形成一版 (a′) 并经交接到 SA 采用，票 sa-cc/19）见函数内注释；
// 反：信封所指的版本 SA 还没有 → 可见性滞后是未决，不定稿、不毒丸，失败码落 dispatch.consumer_undecided。
func TestAnAdoptedExternalFundsFactReachesTheCustomsRegisterThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	facts, err := sapostgres.NewExternalFundsFacts(db)
	if err != nil {
		t.Fatalf("SA 资金事实库：%v", err)
	}
	adopted, err := sadomain.AdoptExternalFundsFact(sadomain.ExternalFundsFactSpec{
		Fact:        saTestValue(t, sadomain.NewFundsFactReference, "bank-fact-1"),
		Source:      saTestValue(t, sadomain.NewFundsSourceRegistrationReference, "source-bank-feed-1"),
		Payer:       saTestValue(t, sadomain.NewFundsPayerReference, "payer-customer-7"),
		Kind:        sadomain.FundsReceiptConfirmed,
		Currency:    saTestValue(t, sadomain.NewCurrencyCode, "USD"),
		AmountMinor: 8000,
		Version:     saTestValue(t, sadomain.NewFundsFactVersion, "bank-fact/v1"),
		OccurredAt:  beatInstant().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("SA 采用事实：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := facts.Save(txCtx, saports.FundsFactRecord{
			Key:           saports.FundsFactKey{TenantID: saTestValue(t, sadomain.NewTenantID, "tenant-a"), Fact: adopted.Fact()},
			ContentDigest: "digest-bank-fact-1",
			Fact:          adopted,
			RecordedAt:    beatInstant(),
		})
		return err
	}); err != nil {
		t.Fatalf("写 SA 事实：%v", err)
	}

	enqueueForBeat(t, db, store, "funds-fact-1", ccinbox.ExternalFundsFactAdoptedEventType,
		`{"tenantId":"tenant-a","fact":"bank-fact-1","version":"bank-fact/v1"}`)
	enqueueForBeat(t, db, store, "funds-fact-2", ccinbox.ExternalFundsFactAdoptedEventType,
		`{"tenantId":"tenant-a","fact":"bank-fact-1","version":"bank-fact/v9"}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1（正：v1 定稿；反：v9 未决）；失败码 v1 = %q，v9 = %q",
			published, recordedFailureCode(t, db, "funds-fact-1"), recordedFailureCode(t, db, "funds-fact-2"))
	}
	if got := recordedFailureCode(t, db, "funds-fact-2"); got != "dispatch.consumer_undecided" {
		t.Fatalf("SA 还没有的版本：failure_code = %q, want dispatch.consumer_undecided", got)
	}

	register, err := ccpostgres.NewDutyPaymentReconciliation(db)
	if err != nil {
		t.Fatalf("CC 登记册：%v", err)
	}
	registration, found, err := register.LoadFundsFactVersion(t.Context(),
		saTestValue(t, ccdomain.NewTenantID, "tenant-a"),
		saTestValue(t, ccdomain.NewExternalFundsFactReference, "bank-fact-1"),
		saTestValue(t, ccdomain.NewFundsFactVersion, "bank-fact/v1"))
	if err != nil || !found {
		t.Fatalf("CC 入向登记：found = %v err = %v——信封到了消费者却没落登记", found, err)
	}
	if registration.Source != "source-bank-feed-1" || !registration.Payer.Provided() || registration.Payer.Reference() != "payer-customer-7" ||
		registration.Currency != "USD" || registration.AmountMinor != 8000 ||
		registration.Version != saTestValue(t, ccdomain.NewFundsFactVersion, "bank-fact/v1") {
		t.Fatalf("登记 = %+v，want 来源 / 付款人 / 币种 / 金额 / 版本照 SA 那一版转述", registration)
	}

	// 正例再扩一格（票 sa-cc/13 完成判据 2；sa-cc/20 完成判据 3 把它改回同一事实两版都在 SA）：同一事实的 v1 与
	// 更正版本 v2（回指 v1、金额变）都落在 SA（settlement_accounting 0021 起身份行一行 + 版本子表两行），各发一封同一
	// 事件类型的信封、先后经消费者落 CC 入向登记册两行——v2 回指 v1，v1 一字不动。两版走的都是生产依赖图（SA 版本行
	// → 信封 → 消费者 → SA 只读视图 → 入向登记），CC 那头不再预铺任何一版。两封分两拍投：同一拍里两个分区谁先
	// 落定没有保证，而 CC 按接收先后列版本。
	tenant := saTestValue(t, ccdomain.NewTenantID, "tenant-a")
	correctedFact := saTestValue(t, ccdomain.NewExternalFundsFactReference, "bank-fact-2")
	original, err := sadomain.AdoptExternalFundsFact(sadomain.ExternalFundsFactSpec{
		Fact:        saTestValue(t, sadomain.NewFundsFactReference, "bank-fact-2"),
		Source:      saTestValue(t, sadomain.NewFundsSourceRegistrationReference, "source-bank-feed-1"),
		Payer:       saTestValue(t, sadomain.NewFundsPayerReference, "payer-customer-7"),
		Kind:        sadomain.FundsReceiptConfirmed,
		Currency:    saTestValue(t, sadomain.NewCurrencyCode, "USD"),
		AmountMinor: 8000,
		Version:     saTestValue(t, sadomain.NewFundsFactVersion, "bank-fact-2/v1"),
		OccurredAt:  beatInstant().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("SA 原版事实：%v", err)
	}
	corrected, err := original.CorrectAmount(9000, saTestValue(t, sadomain.NewFundsFactVersion, "bank-fact-2/v2"), beatInstant())
	if err != nil {
		t.Fatalf("SA 更正版本：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		key := saports.FundsFactKey{TenantID: saTestValue(t, sadomain.NewTenantID, "tenant-a"), Fact: original.Fact()}
		if _, err := facts.Save(txCtx, saports.FundsFactRecord{
			Key:           key,
			ContentDigest: "digest-bank-fact-2-v1",
			Fact:          original,
			RecordedAt:    beatInstant().Add(-time.Hour),
		}); err != nil {
			return err
		}
		_, err := facts.Save(txCtx, saports.FundsFactRecord{
			Key:           key,
			ContentDigest: "digest-bank-fact-2-v2",
			Fact:          corrected,
			RecordedAt:    beatInstant(),
		})
		return err
	}); err != nil {
		t.Fatalf("写 SA 的 v1 与更正版本 v2：%v", err)
	}
	enqueueForBeat(t, db, store, "funds-fact-3", ccinbox.ExternalFundsFactAdoptedEventType,
		`{"tenantId":"tenant-a","fact":"bank-fact-2","version":"bank-fact-2/v1"}`)

	published, err = beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("第二拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("第二拍 published = %d, want 1（bank-fact-2 v1 定稿；v9 仍未决）；失败码 v1 = %q", published, recordedFailureCode(t, db, "funds-fact-3"))
	}

	// 正例第三格的前置（票 sa-cc/19 完成判据 (2)）：v1 已在 CC、v2 未到之际，按 v1 形成一版核对——经 CC 自己的编排
	// `VerifyPayment`（协作事项与付款人规则先经真登记册铺好），走的是生产那条形成路（真核对册 + 真 Outbox 交接）。
	// 税费与范围两个合成引用取本文件的常规长度：这一对在 05 的旧串接形 ID 下曾超过 eventing 的信封 ID 上限、让交接
	// 折成续办引用（票 sa-cc/19 判断项 ④），自票 sa-cc/29 起 ID 是定长指纹形，引用多长都装得下；下面对
	// 「交接成功」的断言因此是正路的证据，不再是绕开。
	verificationHandoff, err := ccpostgres.NewOutboxDutyPaymentVerificationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("CC 核对交接：%v", err)
	}
	reconciliation, err := ccapplication.NewDutyPaymentReconciliationHandler(ccapplication.DutyPaymentReconciliationDeps{
		Collaborations: register, Funds: register, Verifications: register, PayerRules: register,
		Handoff: verificationHandoff, Clock: systemClock{},
	})
	if err != nil {
		t.Fatalf("CC 核对编排：%v", err)
	}
	lineageDuty := saTestValue(t, ccdomain.NewAssessedDutyReference, "SYN-DUTY-RD/v1")
	lineageScope := saTestValue(t, ccdomain.NewDecisionScopeReference, "SYN-UNIT-RD")
	lineageProcedure := saTestValue(t, ccdomain.NewCustomsProcedureReference, "SYN-PROC-RD")
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		collaboration, err := ccdomain.FormDutyCollaboration(ccdomain.DutyCollaborationSpec{
			Kind:        ccdomain.ObligationFromAssessedDuty,
			Duty:        lineageDuty,
			Scope:       lineageScope,
			Obligor:     saTestValue(t, ccdomain.NewLegalObligorReference, "SYN-OBLIGOR-RD"),
			Requirement: saTestValue(t, ccdomain.NewPaymentRequirementSource, "SYN-ASSESSMENT-RD"),
			Target:      saTestValue(t, ccdomain.NewResponsibilityTargetReference, "SYN-DUTY-DESK"),
			FormedAt:    beatInstant(),
		})
		if err != nil {
			return err
		}
		if _, err := register.SaveCollaboration(txCtx, tenant, collaboration); err != nil {
			return err
		}
		if _, err := register.RegisterPayerRequirement(txCtx, tenant, lineageProcedure, ccdomain.PayerRequired); err != nil {
			return err
		}
		result, err := reconciliation.VerifyPayment(txCtx, ccapplication.VerifyDutyPaymentCommand{
			TenantID:     tenant,
			Duty:         lineageDuty,
			Funds:        correctedFact,
			FundsVersion: saTestValue(t, ccdomain.NewFundsFactVersion, "bank-fact-2/v1"),
			Scope:        lineageScope,
			Procedure:    lineageProcedure,
			Coverage:     ccdomain.CoverageFull,
			Delta:        ccdomain.DeltaNone,
			Validity:     ccdomain.FundsFactValid,
			Basis:        "SYN-RULE-RD: assessment reference quoted on the remittance",
		})
		if err != nil {
			return err
		}
		if result.Outcome() != ccapplication.DutyVerificationFormed || result.HandoffReference() != "" {
			return fmt.Errorf("按 v1 核对 outcome = %s（reason %s，续办 %q），want DUTY_VERIFICATION_FORMED 且交接成功",
				result.Outcome(), result.UndecidedReason(), result.HandoffReference())
		}
		return nil
	}); err != nil {
		t.Fatalf("CC 侧按 v1 形成核对：%v", err)
	}
	// 按 v1 形成的那一版自己交了一封（05）；先把它发出去，第三拍的计数才只剩 v2 那一封。
	if published, err = beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("发出按 v1 形成的核对信封：published = %d err = %v, want 1", published, err)
	}

	enqueueForBeat(t, db, store, "funds-fact-4", ccinbox.ExternalFundsFactAdoptedEventType,
		`{"tenantId":"tenant-a","fact":"bank-fact-2","version":"bank-fact-2/v2","corrects":"bank-fact-2/v1"}`)

	published, err = beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("第三拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("第三拍 published = %d, want 1（v2 定稿；v9 仍未决）；失败码 v2 = %q", published, recordedFailureCode(t, db, "funds-fact-4"))
	}
	versions, err := register.ListFundsFactVersions(t.Context(), tenant, correctedFact)
	if err != nil || len(versions) != 2 {
		t.Fatalf("更正版本到达后该列两版：err=%v n=%d", err, len(versions))
	}
	if versions[0].Version != saTestValue(t, ccdomain.NewFundsFactVersion, "bank-fact-2/v1") || versions[0].AmountMinor != 8000 {
		t.Fatalf("v1 该一字不动：%+v", versions[0])
	}
	if versions[1].Version != saTestValue(t, ccdomain.NewFundsFactVersion, "bank-fact-2/v2") ||
		versions[1].Corrects != versions[0].Version || versions[1].AmountMinor != 9000 {
		t.Fatalf("v2 该回指 v1 且带更正后金额：%+v", versions[1])
	}

	// 正例第三格（票 sa-cc/19 完成判据 (2)）：v2 到达 → 同一拍、同一事务里编排对 bank-fact-2 既往那条核对谱系形成
	// 一版 (a′)——覆盖承前（COVERED）、差额 / 有效性 PENDING、依据 / 程序承前、资金版本 = v2；前版一字不动；新版本
	// 经 05 的交接一版一封，下一拍发出去、SA 按引用回读并采用它（sa-cc/09 那族照收，SA 零改动）。
	lineage, err := register.ListVerificationsByFundsFact(t.Context(), tenant, correctedFact)
	if err != nil || len(lineage) != 2 {
		t.Fatalf("v2 到达后该谱系该有两版核对：err=%v n=%d", err, len(lineage))
	}
	previous, rederived := lineage[0], lineage[1]
	if previous.Verification.FundsVersion() != versions[0].Version || previous.Verification.Delta() != ccdomain.DeltaNone ||
		previous.Verification.Validity() != ccdomain.FundsFactValid {
		t.Fatalf("按 v1 那一版该一字不动：%+v", previous.Verification)
	}
	if rederived.Verification.FundsVersion() != versions[1].Version ||
		rederived.Verification.Coverage() != ccdomain.CoverageFull ||
		rederived.Verification.Delta() != ccdomain.DeltaPending ||
		rederived.Verification.Validity() != ccdomain.FundsFactPending ||
		rederived.Verification.Procedure() != lineageProcedure ||
		rederived.Basis != previous.Basis ||
		rederived.Key.Duty != lineageDuty || rederived.Key.Scope != lineageScope || rederived.Key.Digest == previous.Key.Digest {
		t.Fatalf("新版本该是 (a′)：%+v", rederived)
	}

	if published, err = beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("第四拍该发出新核对版本那一封：published = %d err = %v, want 1", published, err)
	}
	adoptions, err := sapostgres.NewDutyPaymentVerificationAdoptions(db)
	if err != nil {
		t.Fatalf("SA 采用册：%v", err)
	}
	rederivedReference, err := sadomain.NewDutyPaymentVerificationReference(
		saTestValue(t, sadomain.NewDeclarationScopeReference, lineageScope.String()),
		saTestValue(t, sadomain.NewTaxObligationReference, lineageDuty.String()),
		saTestValue(t, sadomain.NewFundsFactReference, correctedFact.String()),
		saTestValue(t, sadomain.NewDutyVerificationVersion, rederived.Key.Digest),
	)
	if err != nil {
		t.Fatalf("SA 引用：%v", err)
	}
	if _, found, err := adoptions.FindByKey(t.Context(), saports.DutyPaymentVerificationAdoptionKey{
		TenantID: saTestValue(t, sadomain.NewTenantID, "tenant-a"), Verification: rederivedReference,
	}); err != nil || !found {
		t.Fatalf("新核对版本的信封该到 SA 并被采用：found = %v err = %v", found, err)
	}
}

// Covers: 路由表的 CC 付款核对形成一条（sa-cc/09）与完成判据 3「真库装配用例一正一反」——在生产依赖图上：
// 正：CC 落过一版核对（连同它外键前置的入向资金事实）→ 用 CC 的真 Outbox 适配器把信封入队（事件类型由
// 提供方写，路由表按消费方自写的常量认——两串相等在这里被真库钉住）→ SA 消费者按五维回查 CC 只读半边、
// 译成采用命令 → SA 的 duty_payment_verification_adoption 落一行，advance_assessment 零行（采用不是判断）；
// 反：信封所指的版本 CC 还没有 → 可见性滞后是未决，不定稿、不毒丸，失败码落 dispatch.consumer_undecided。
func TestAFormedDutyPaymentVerificationReachesTheSettlementInputThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	register, err := ccpostgres.NewDutyPaymentReconciliation(db)
	if err != nil {
		t.Fatalf("CC 核对册：%v", err)
	}
	verificationAt := beatInstant().Add(-time.Hour)
	tenant := saTestValue(t, ccdomain.NewTenantID, "tenant-a")
	duty := saTestValue(t, ccdomain.NewAssessedDutyReference, "SYN-DUTY-01/v1")
	funds := saTestValue(t, ccdomain.NewExternalFundsFactReference, "SYN-FUNDS-01")
	scope := saTestValue(t, ccdomain.NewDecisionScopeReference, "SYN-UNIT-01")
	verification, err := ccdomain.VerifyDutyPayment(duty, funds,
		saTestValue(t, ccdomain.NewFundsFactVersion, "SYN-FUNDS-01/v1"), scope,
		saTestValue(t, ccdomain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		ccdomain.CoverageFull, ccdomain.DeltaNone, ccdomain.FundsFactValid, verificationAt)
	if err != nil {
		t.Fatalf("CC 核对：%v", err)
	}
	record := ccports.DutyVerificationRecord{
		Key:          ccports.DutyVerificationKey{TenantID: tenant, Duty: duty, Funds: funds, Scope: scope, Digest: "digest-v1"},
		Verification: verification,
		Basis:        "SYN-RULE-01: assessment reference quoted on the remittance",
	}
	handoff, err := ccpostgres.NewOutboxDutyPaymentVerificationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("CC 核对交接：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := register.RegisterFundsFact(txCtx, tenant, ccports.ExternalFundsFactRegistration{
			Fact: funds, Version: saTestValue(t, ccdomain.NewFundsFactVersion, "SYN-FUNDS-01/v1"),
			Source: "SYN-BANK-01", Payer: saTestValue(t, ccdomain.ProvidedFundsPayer, "SYN-PAYER-01"), Currency: "XTS", AmountMinor: 12500,
			OccurredAt: verificationAt.Add(-time.Hour),
		}); err != nil {
			return err
		}
		if _, err := register.SaveVerification(txCtx, record); err != nil {
			return err
		}
		return handoff.HandOffDutyPaymentVerification(txCtx, ccports.DutyPaymentVerificationHandoffIntent{
			Key: record.Key, Verification: verification,
		})
	}); err != nil {
		t.Fatalf("CC 侧落核对并入队：%v", err)
	}
	// 反例：同一范围、CC 还没有的指纹。
	enqueueForBeat(t, db, store, "duty-verification-missing", sainbox.DutyPaymentVerificationFormedEventType,
		`{"tenantId":"tenant-a","scope":"SYN-UNIT-01","duty":"SYN-DUTY-01/v1","funds":"SYN-FUNDS-01","digest":"digest-v9"}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1（正：digest-v1 定稿；反：digest-v9 未决）；失败码 v9 = %q",
			published, recordedFailureCode(t, db, "duty-verification-missing"))
	}
	if got := recordedFailureCode(t, db, "duty-verification-missing"); got != "dispatch.consumer_undecided" {
		t.Fatalf("CC 还没有的版本：failure_code = %q, want dispatch.consumer_undecided", got)
	}

	adoptions, err := sapostgres.NewDutyPaymentVerificationAdoptions(db)
	if err != nil {
		t.Fatalf("SA 采用册：%v", err)
	}
	reference, err := sadomain.NewDutyPaymentVerificationReference(
		saTestValue(t, sadomain.NewDeclarationScopeReference, "SYN-UNIT-01"),
		saTestValue(t, sadomain.NewTaxObligationReference, "SYN-DUTY-01/v1"),
		saTestValue(t, sadomain.NewFundsFactReference, "SYN-FUNDS-01"),
		saTestValue(t, sadomain.NewDutyVerificationVersion, "digest-v1"),
	)
	if err != nil {
		t.Fatalf("SA 引用：%v", err)
	}
	adopted, found, err := adoptions.FindByKey(t.Context(), saports.DutyPaymentVerificationAdoptionKey{
		TenantID: saTestValue(t, sadomain.NewTenantID, "tenant-a"), Verification: reference,
	})
	if err != nil || !found {
		t.Fatalf("SA 采用：found = %v err = %v——信封到了消费者却没落采用", found, err)
	}
	if adopted.Adoption.AdoptedAt().IsZero() {
		t.Fatal("采用时刻为零")
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var assessmentRows int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.advance_assessment WHERE tenant_id = $1`, "tenant-a",
	).Scan(&assessmentRows); err != nil {
		t.Fatalf("数评估行：%v", err)
	}
	if assessmentRows != 0 {
		t.Fatalf("采用付款核对不得形成实际代垫判断：advance_assessment 有 %d 行", assessmentRows)
	}

	// 票 sa-cc/29 裁决 4 / 完成判据 (4)：SA 侧的幂等靠采用登记册照抄 CC 核对键的五维主键，不靠信封 ID。同一版核对以
	// **另一个**信封 ID 再投一封（CC 换 ID 形之后重投、或任何两封同内容不同 ID 的信封）：Inbox 门只挡同一 ID，这一封
	// 放行到采用编排，编排撞五维主键答幂等——采用册仍一行、消费入账不报错、不形成第二次采用。
	enqueueForBeat(t, db, store, "duty-verification-second-id-for-digest-v1", sainbox.DutyPaymentVerificationFormedEventType,
		`{"tenantId":"tenant-a","scope":"SYN-UNIT-01","duty":"SYN-DUTY-01/v1","funds":"SYN-FUNDS-01","digest":"digest-v1"}`)
	if published, err := beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("同一核对换 ID 再投：published = %d err = %v, want 1；失败码 = %q",
			published, err, recordedFailureCode(t, db, "duty-verification-second-id-for-digest-v1"))
	}
	if got := recordedFailureCode(t, db, "duty-verification-second-id-for-digest-v1"); got != "" {
		t.Fatalf("同一核对换 ID 再投不该失败：failure_code = %q", got)
	}
	var adoptionRows int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.duty_payment_verification_adoption
		  WHERE tenant_id = $1 AND scope_ref = $2 AND duty_ref = $3 AND funds_ref = $4 AND version_digest = $5`,
		"tenant-a", "SYN-UNIT-01", "SYN-DUTY-01/v1", "SYN-FUNDS-01", "digest-v1",
	).Scan(&adoptionRows); err != nil {
		t.Fatalf("数采用行：%v", err)
	}
	if adoptionRows != 1 {
		t.Fatalf("同一版核对两个信封 ID 各投一封后采用册该仍是一行，实得 %d 行", adoptionRows)
	}
}

// Covers: 票 sa-cc/29 裁决 2 (b) 在生产依赖图上：重派形成的新核对版本交结算意图时信封被框架确定性拒收 → 整笔硬失败、
// 整笔回滚——CC 既不留 v2 的版本行也不留 (a′) 核对行、信封没发、失败码落 dispatch.publish_failed 人动手；没有
// 「核对行提交、信封没发、无人知道」那个半截（sa-cc/19 评审 Spec ② 点名的缺口）。触发用的是 Subject 超
// eventing.MaxSubjectLength：ID 已是定长指纹形，Subject / PartitionKey 仍取可读形、引用多长归实例半边，本格同时
// 证这一格今天确实还到得了、且到了是响亮的。同一条超长范围在人重核路上仍按 05 的形折成续办引用（裁决 3 不动），
// 前置那一步顺手把它断出来。
func TestARederivedVerificationWhoseEnvelopeIsRejectedRollsBackLoudlyInsteadOfCommittingHalf(t *testing.T) {
	beat, db, store := wiredBeat(t)
	facts, err := sapostgres.NewExternalFundsFacts(db)
	if err != nil {
		t.Fatalf("SA 资金事实库：%v", err)
	}
	original, err := sadomain.AdoptExternalFundsFact(sadomain.ExternalFundsFactSpec{
		Fact:        saTestValue(t, sadomain.NewFundsFactReference, "bank-fact-3"),
		Source:      saTestValue(t, sadomain.NewFundsSourceRegistrationReference, "source-bank-feed-1"),
		Payer:       saTestValue(t, sadomain.NewFundsPayerReference, "payer-customer-7"),
		Kind:        sadomain.FundsReceiptConfirmed,
		Currency:    saTestValue(t, sadomain.NewCurrencyCode, "USD"),
		AmountMinor: 8000,
		Version:     saTestValue(t, sadomain.NewFundsFactVersion, "bank-fact-3/v1"),
		OccurredAt:  beatInstant().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("SA 原版事实：%v", err)
	}
	corrected, err := original.CorrectAmount(9000, saTestValue(t, sadomain.NewFundsFactVersion, "bank-fact-3/v2"), beatInstant())
	if err != nil {
		t.Fatalf("SA 更正版本：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		key := saports.FundsFactKey{TenantID: saTestValue(t, sadomain.NewTenantID, "tenant-a"), Fact: original.Fact()}
		if _, err := facts.Save(txCtx, saports.FundsFactRecord{
			Key: key, ContentDigest: "digest-bank-fact-3-v1", Fact: original, RecordedAt: beatInstant().Add(-time.Hour),
		}); err != nil {
			return err
		}
		_, err := facts.Save(txCtx, saports.FundsFactRecord{
			Key: key, ContentDigest: "digest-bank-fact-3-v2", Fact: corrected, RecordedAt: beatInstant(),
		})
		return err
	}); err != nil {
		t.Fatalf("写 SA 的 v1 与更正版本 v2：%v", err)
	}
	enqueueForBeat(t, db, store, "funds-fact-long-1", ccinbox.ExternalFundsFactAdoptedEventType,
		`{"tenantId":"tenant-a","fact":"bank-fact-3","version":"bank-fact-3/v1"}`)
	if published, err := beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("v1 到 CC：published = %d err = %v, want 1", published, err)
	}

	register, err := ccpostgres.NewDutyPaymentReconciliation(db)
	if err != nil {
		t.Fatalf("CC 登记册：%v", err)
	}
	verificationHandoff, err := ccpostgres.NewOutboxDutyPaymentVerificationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("CC 核对交接：%v", err)
	}
	reconciliation, err := ccapplication.NewDutyPaymentReconciliationHandler(ccapplication.DutyPaymentReconciliationDeps{
		Collaborations: register, Funds: register, Verifications: register, PayerRules: register,
		Handoff: verificationHandoff, Clock: systemClock{},
	})
	if err != nil {
		t.Fatalf("CC 核对编排：%v", err)
	}
	tenant := saTestValue(t, ccdomain.NewTenantID, "tenant-a")
	fact := saTestValue(t, ccdomain.NewExternalFundsFactReference, "bank-fact-3")
	lineageDuty := saTestValue(t, ccdomain.NewAssessedDutyReference, "SYN-DUTY-LONG/v1")
	// 范围引用长到 Subject（范围 / 税费）必然超过 eventing.MaxSubjectLength——合成串，不假定任何租户的引用多长。
	lineageScope := saTestValue(t, ccdomain.NewDecisionScopeReference, "SYN-UNIT-LONG-"+strings.Repeat("x", eventing.MaxSubjectLength))
	lineageProcedure := saTestValue(t, ccdomain.NewCustomsProcedureReference, "SYN-PROC-RD")
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		collaboration, err := ccdomain.FormDutyCollaboration(ccdomain.DutyCollaborationSpec{
			Kind:        ccdomain.ObligationFromAssessedDuty,
			Duty:        lineageDuty,
			Scope:       lineageScope,
			Obligor:     saTestValue(t, ccdomain.NewLegalObligorReference, "SYN-OBLIGOR-RD"),
			Requirement: saTestValue(t, ccdomain.NewPaymentRequirementSource, "SYN-ASSESSMENT-RD"),
			Target:      saTestValue(t, ccdomain.NewResponsibilityTargetReference, "SYN-DUTY-DESK"),
			FormedAt:    beatInstant(),
		})
		if err != nil {
			return err
		}
		if _, err := register.SaveCollaboration(txCtx, tenant, collaboration); err != nil {
			return err
		}
		if _, err := register.RegisterPayerRequirement(txCtx, tenant, lineageProcedure, ccdomain.PayerRequired); err != nil {
			return err
		}
		result, err := reconciliation.VerifyPayment(txCtx, ccapplication.VerifyDutyPaymentCommand{
			TenantID:     tenant,
			Duty:         lineageDuty,
			Funds:        fact,
			FundsVersion: saTestValue(t, ccdomain.NewFundsFactVersion, "bank-fact-3/v1"),
			Scope:        lineageScope,
			Procedure:    lineageProcedure,
			Coverage:     ccdomain.CoverageFull,
			Delta:        ccdomain.DeltaNone,
			Validity:     ccdomain.FundsFactValid,
			Basis:        "SYN-RULE-RD: assessment reference quoted on the remittance",
		})
		if err != nil {
			return err
		}
		// 人重核路：05 的兜底原样——核对形成、信封被拒折成续办引用、原始错误随结果可见（裁决 3 不动这一格）。
		if result.Outcome() != ccapplication.DutyVerificationFormed || result.HandoffReference() == "" ||
			!errors.Is(result.HandoffError(), ccports.ErrHandoffEnvelopeRejected) {
			return fmt.Errorf("人重核路按 v1 核对 outcome = %s 续办 %q err = %v，want 形成 + 续办引用 + 信封被拒",
				result.Outcome(), result.HandoffReference(), result.HandoffError())
		}
		return nil
	}); err != nil {
		t.Fatalf("CC 侧按 v1 形成核对：%v", err)
	}

	enqueueForBeat(t, db, store, "funds-fact-long-2", ccinbox.ExternalFundsFactAdoptedEventType,
		`{"tenantId":"tenant-a","fact":"bank-fact-3","version":"bank-fact-3/v2","corrects":"bank-fact-3/v1"}`)
	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("v2 那一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("v2 到达该整笔硬失败：published = %d, want 0", published)
	}
	if got := recordedFailureCode(t, db, "funds-fact-long-2"); got != "dispatch.publish_failed" {
		t.Fatalf("信封被确定性拒收该落硬失败码：failure_code = %q, want dispatch.publish_failed", got)
	}
	versions, err := register.ListFundsFactVersions(t.Context(), tenant, fact)
	if err != nil || len(versions) != 1 {
		t.Fatalf("整笔回滚后 CC 只该有 v1 的版本行：err=%v n=%d", err, len(versions))
	}
	lineage, err := register.ListVerificationsByFundsFact(t.Context(), tenant, fact)
	if err != nil || len(lineage) != 1 || lineage[0].Verification.Delta() != ccdomain.DeltaNone {
		t.Fatalf("整笔回滚后不得留下 (a′) 核对行：err=%v n=%d", err, len(lineage))
	}
}

// Covers: 票 sa-cc/29 完成判据 (2) 的集合半边——重派路上信封被确定性拒收的硬失败哨兵**不在**这条线的未决名单里，
// 与 externalFundsFactUndecidedSentinels 名单里每一只互不 errors.Is。名单取该切片本身：测试里重列一份会让误登记的哨兵在
// 测试里也一起消失。
func TestARejectedDutyVerificationHandoffIsNotRegisteredAsUndecided(t *testing.T) {
	for _, sentinel := range externalFundsFactUndecidedSentinels {
		if errors.Is(sentinel, ccsettlement.ErrDutyVerificationHandoffRejected) ||
			errors.Is(ccsettlement.ErrDutyVerificationHandoffRejected, sentinel) {
			t.Fatalf("硬失败哨兵被登成了未决：%v——重投同一份永远同一个结果，登进名单只会以未决之名耗尽失败预算", sentinel)
		}
	}
	undecided := fmt.Errorf("%w: %w", ccsettlement.ErrDutyVerificationRederivationUndecided, errors.New("outbox down"))
	registered := false
	for _, sentinel := range externalFundsFactUndecidedSentinels {
		registered = registered || errors.Is(undecided, sentinel)
	}
	if !registered {
		t.Fatal("重派未决哨兵该仍在名单里——本格只把硬失败拦在外面，不动未决那一半")
	}
}

// saTestValue 构造一个值对象，失败即用例失败。只给上面那条用例用，别处各有自己的同形助手。
func saTestValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// Covers: 路由表的 PP 评价已记录一条（sa-cc/01，裁决 (c)「信封到未决」）与完成判据 3「真库装配用例一正一反」——
// 在生产依赖图上：
// 正：PP 落一份 BUY·SUPPLIER_COST 评价 → 用 PP 的真 Outbox 适配器把信封入队（事件类型由提供方写，路由表按消费方
// 自写的常量认——两串相等在这里被真库钉住）→ SA 消费者按引用回查 PP 评价库 → 形成命令还缺三件来源引用 → 停在
// dispatch.consumer_undecided：inbox 无账、supplier_expected_cost 零行，等评价请求记录接上后重投；
// 反：同一种信封指着一份 SELL·CUSTOMER_CHARGE 评价 → 不是本消费者的信封，入账定稿、SA 零写入；另一封指着 PP
// 还没有的评价 → 可见性滞后同样是未决，不毒丸、不定稿。
func TestARecordedBuyEvaluationStopsUndecidedAtTheExpectedCostSeamThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	evaluations, err := pppostgres.NewEvaluations(db)
	if err != nil {
		t.Fatalf("PP 评价库：%v", err)
	}
	handoff, err := pppostgres.NewOutboxEvaluationHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("PP 评价交接：%v", err)
	}
	buy := syntheticPricingEvaluation(t, "SYN-EVAL-BUY-01", ppdomain.PricingDirectionBuy, ppdomain.PricingPurposeSupplierCost)
	sell := syntheticPricingEvaluation(t, "SYN-EVAL-SELL-01", ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge)
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		for _, evaluation := range []ppdomain.PricingEvaluation{buy, sell} {
			if _, err := evaluations.Save(txCtx, evaluation); err != nil {
				return err
			}
			if err := handoff.HandOffEvaluation(txCtx, ppports.EvaluationHandoffIntent{Evaluation: evaluation}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("PP 侧落评价并入队：%v", err)
	}
	// 反例二：信封指着 PP 还没有的评价。
	enqueueForBeat(t, db, store, "SYN-EVAL-MISSING", sainbox.BuyEvaluationRecordedEventType,
		`{"tenantId":"tenant-a","evaluationId":"SYN-EVAL-MISSING"}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1（正：BUY 未决；反：SELL 入账定稿、缺席评价未决）；失败码 BUY = %q",
			published, recordedFailureCode(t, db, "SYN-EVAL-BUY-01"))
	}
	if got := recordedFailureCode(t, db, "SYN-EVAL-BUY-01"); got != "dispatch.consumer_undecided" {
		t.Fatalf("BUY 评价：failure_code = %q, want dispatch.consumer_undecided（等评价请求记录）", got)
	}
	if got := recordedFailureCode(t, db, "SYN-EVAL-MISSING"); got != "dispatch.consumer_undecided" {
		t.Fatalf("PP 还没有的评价：failure_code = %q, want dispatch.consumer_undecided", got)
	}

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var costRows int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1`, "tenant-a",
	).Scan(&costRows); err != nil {
		t.Fatalf("数预期成本行：%v", err)
	}
	if costRows != 0 {
		t.Fatalf("命令没凑齐却形成了预期成本：supplier_expected_cost 有 %d 行", costRows)
	}
	inboxRows := func(eventID string) int {
		var count int
		if err := querier.QueryRow(t.Context(),
			`SELECT count(*) FROM `+migrate.SchemaBento+`.inbox WHERE consumer = $1 AND event_id = $2`,
			"settlement-accounting/form-supplier-expected-cost", eventID,
		).Scan(&count); err != nil {
			t.Fatalf("数 inbox：%v", err)
		}
		return count
	}
	if n := inboxRows("SYN-EVAL-BUY-01"); n != 0 {
		t.Fatalf("BUY 未决必须回滚：inbox 行数 = %d, want 0", n)
	}
	if n := inboxRows("SYN-EVAL-SELL-01"); n != 1 {
		t.Fatalf("SELL 不是本消费者的信封，要入账不重投：inbox 行数 = %d, want 1", n)
	}
}

// syntheticPricingEvaluation 造一份 PP 评价：一张 SYN 卡（USD 价表 12.5，合计 HALF_UP 到 0.01——声明了取整策略，
// SA 读口才不会以 AMOUNT_PRECISION_UNDECLARED 拒）对一份 5 kg / Z1 的包裹输入评价。方向与目的由调用方给：
// 同一张卡的形状换个方向就是 SELL 评价，正是提供方对两个方向发同一种信封的那个事实。
// 构造走 PP 的测试专属夹具 pptest，值全部在这里显式给出（票 sa-cc/17）。
func syntheticPricingEvaluation(
	t *testing.T, id string, direction ppdomain.PricingDirection, purpose ppdomain.PricingPurpose,
) ppdomain.PricingEvaluation {
	t.Helper()
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-CARD-"+string(direction), "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT", "v1"),
		Scope:                 "SYN-SCOPE-01",
		Direction:             direction,
		Purpose:               purpose,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "entry",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "12.5",
		WeightRounding:      ppdomain.RoundingNone,
		WeightStepKilograms: "1",
		AmountRounding: &pptest.AmountRoundingSpec{
			Mode:      ppdomain.RoundingHalfUp,
			Increment: "0.01",
			Points:    []ppdomain.AmountRoundingPoint{ppdomain.AmountRoundingTotal},
		},
	})
	input := pptest.Input(t, pptest.InputSpec{
		Tenant:     "tenant-a",
		Scope:      "SYN-SCOPE-01",
		PackageID:  "SYN-PKG-01",
		Zone:       "Z1",
		Kilograms:  "5",
		BusinessAt: time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	})
	evaluation := pptest.Evaluate(t, id, plan, input)
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("夹具评价没完成：%s %#v", evaluation.Status(), evaluation.Issues())
	}
	return evaluation
}

// Covers: 路由表的 SA 评价请求已提交一条（sa-cc/11 裁决 5）与完成判据 3「真库装配用例一正一反」——在生产依赖图上：
// 正：SA 用真登记册落一份 BUY 评价请求、用真 Outbox 适配器把信封入队（事件类型由提供方写，路由表按消费方自写的常量
// 认——两串相等在这里被真库钉住）；PP 册上有一张适用的 SYN BUY 卡 → PP 消费者按引用回查 SA 登记册 → 入口解析到卡 →
// 造快照停在「输入不可得」并点名三只读口（裁决 4）→ dispatch.consumer_undecided：inbox 无账、parcel_pricing.evaluation
// 零行，等读口接上后重投。今天生产图上形成不了任何一份评价，这条正例证的就是「诚实地停在那一格」。
// 反：另一份请求的范围下没有卡 → 未配置同样未决、零写入，观察口交出的原文说的是缺价卡不是缺输入；信封指着 SA 还
// 没有的请求 → 可见性滞后未决；载荷缺请求 ID → 毒丸入账定稿。
//
// 「重投不翻倍」不在这里假装：形成路今天到不了入册，重投同一封在应用层按回指命中`已存在`
// （application 用例 TestFormEvaluationFromRequestAnswersExistingByBackReference），库上由迁移 0010 的部分唯一索引守
// （pppostgres 用例 evaluation_by_request_test.go），inbox 认领由 ppinbox 的真库用例守。
func TestASubmittedEvaluationRequestStopsHonestlyAtThePricingInputSeamThroughTheRouteTable(t *testing.T) {
	observed := map[string]error{}
	beat, db, store := wiredBeat(t, dispatch.WithDeliveryFailureObserver(
		func(delivery eventing.Delivery, _ eventing.FailureCode, err error) {
			observed[string(delivery.Envelope.ID)] = err
		}))

	// PP 册上一张适用的 SYN BUY 卡：范围 SYN-SCOPE-11，期内覆盖发生项业务时点。
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatalf("PP 价卡登记册：%v", err)
	}
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-BUY-CARD-11", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-BUY-TABLE-11", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-BUY-WEIGHT-11", "v1"),
		Scope:                 "SYN-SCOPE-11",
		Direction:             ppdomain.PricingDirectionBuy,
		Purpose:               ppdomain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "USD",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "entry",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "12",
		WeightRounding:      ppdomain.RoundingCeiling,
		WeightStepKilograms: "0.5",
	})
	source, err := ppdomain.NewSourceFileIdentity("SYN-PRC-CARD-11.xlsx",
		"0000000000000000000000000000000000000000000000000000000000000011")
	if err != nil {
		t.Fatalf("源文件身份：%v", err)
	}
	grant, err := ppdomain.NewVersionReference(ppdomain.ArtifactCommercialAuthorization, "SYN-PRC-GRANT-11", "v1", "sha256:syn-grant-11")
	if err != nil {
		t.Fatalf("授权引用：%v", err)
	}
	tenant, err := ppdomain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	registration, err := ppdomain.NewPriceCardRegistration(tenant, plan, source, grant, "SYN-PRC-GOVERNANCE")
	if err != nil {
		t.Fatalf("价卡登记：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := cards.Register(txCtx, registration)
		return err
	}); err != nil {
		t.Fatalf("登记 SYN BUY 卡：%v", err)
	}

	// SA 侧：两份 BUY 评价请求经真登记册 + 真 Outbox 适配器落库入队——有卡的范围一份、没卡的范围一份。
	requests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		t.Fatalf("SA 评价请求登记册：%v", err)
	}
	requestHandoff, err := sapostgres.NewOutboxEvaluationRequestHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatalf("SA 评价请求交接：%v", err)
	}
	occurredAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	submit := func(requestID, scope string) saports.EvaluationRequestRecord {
		occurrence, err := sadomain.NewTransportChargeOccurrence(
			saValue(t, sadomain.NewChargeOccurrenceID, "SYN-OCC-"+requestID),
			saValue(t, sadomain.NewOccurrenceReasonReference, "SYN-REASON-BOOKING"),
			saValue(t, sadomain.NewOccurrenceVersion, "v1"),
			occurredAt,
		)
		if err != nil {
			t.Fatalf("发生项引用：%v", err)
		}
		request, err := sadomain.SubmitEvaluationRequest(sadomain.EvaluationRequestSpec{
			ID:      saValue(t, sadomain.NewEvaluationRequestID, requestID),
			Scope:   saValue(t, sadomain.NewPrimaryScopeReference, scope),
			Purpose: sadomain.BuySupplierCost,
			Sources: sadomain.EligibleSourceReferences{
				Occurrence: occurrence,
				FeeItem:    saValue(t, sadomain.NewFeeItemReference, "SYN-FEE-11"),
				Agreement:  saValue(t, sadomain.NewSupplierAgreementReference, "SYN-AGR-11@v1"),
			},
			RequestedAt: occurredAt.Add(time.Hour),
			RequestedBy: saValue(t, sadomain.NewRequesterReference, "SYN-SETTLEMENT-JOB"),
		})
		if err != nil {
			t.Fatalf("SA 评价请求：%v", err)
		}
		return saports.EvaluationRequestRecord{
			Key:        saports.EvaluationRequestKey{TenantID: saValue(t, sadomain.NewTenantID, "tenant-a"), Request: request.ID()},
			Request:    request,
			RecordedAt: beatInstant(),
		}
	}
	carded := submit("EVREQ-SYN-CARDED", "SYN-SCOPE-11")
	uncarded := submit("EVREQ-SYN-NOCARD", "SYN-SCOPE-NOCARD")
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		for _, record := range []saports.EvaluationRequestRecord{carded, uncarded} {
			if _, err := requests.Save(txCtx, record); err != nil {
				return err
			}
			if err := requestHandoff.HandOffEvaluationRequest(txCtx, saports.EvaluationRequestIntent{Record: record}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("SA 侧落请求并入队：%v", err)
	}
	// 反例二：信封指着 SA 还没有的请求。反例三：载荷缺请求 ID——毒丸。
	enqueueForBeat(t, db, store, "EVREQ-SYN-MISSING", ppinbox.EvaluationRequestSubmittedEventType,
		`{"tenantId":"tenant-a","evaluationRequestId":"EVREQ-SYN-MISSING"}`)
	enqueueForBeat(t, db, store, "EVREQ-SYN-POISON", ppinbox.EvaluationRequestSubmittedEventType,
		`{"tenantId":"tenant-a"}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	cardedID := "tenant-a/evaluation-request/EVREQ-SYN-CARDED/submitted"
	uncardedID := "tenant-a/evaluation-request/EVREQ-SYN-NOCARD/submitted"
	if published != 1 {
		t.Fatalf("published = %d, want 1（正：有卡的停在输入不可得；反：没卡的未配置、缺席请求未决、毒丸入账）；有卡的失败码 = %q 原文 = %v",
			published, recordedFailureCode(t, db, cardedID), observed[cardedID])
	}
	for _, eventID := range []string{cardedID, uncardedID, "EVREQ-SYN-MISSING"} {
		if got := recordedFailureCode(t, db, eventID); got != "dispatch.consumer_undecided" {
			t.Fatalf("%s：failure_code = %q, want dispatch.consumer_undecided；原文 = %v", eventID, got, observed[eventID])
		}
	}
	// 三封未决各停在哪一格，观察口交出的原文要分得开——失败码按恢复动作取值，本来答不出这一层。
	if err := observed[cardedID]; !errors.Is(err, ppsettlement.ErrPricingInputUnavailable) || !strings.Contains(err.Error(), "transport-fulfillment") {
		t.Fatalf("有卡的请求没停在「输入不可得」或没点名读口：%v", err)
	}
	if err := observed[uncardedID]; !errors.Is(err, ppsettlement.ErrPriceCardNotConfigured) {
		t.Fatalf("没卡的请求没停在「未配置」：%v", err)
	}
	if err := observed["EVREQ-SYN-MISSING"]; !errors.Is(err, ppsettlement.ErrEvaluationRequestNotVisible) {
		t.Fatalf("SA 还没有的请求没停在「还看不见」：%v", err)
	}

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var evaluationRows int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_pricing.evaluation WHERE tenant_id = $1`, "tenant-a",
	).Scan(&evaluationRows); err != nil {
		t.Fatalf("数评价行：%v", err)
	}
	if evaluationRows != 0 {
		t.Fatalf("输入不可得却形成了评价：parcel_pricing.evaluation 有 %d 行", evaluationRows)
	}
	inboxRows := func(eventID string) int {
		var count int
		if err := querier.QueryRow(t.Context(),
			`SELECT count(*) FROM `+migrate.SchemaBento+`.inbox WHERE consumer = $1 AND event_id = $2`,
			"parcel-pricing/form-evaluation-from-request", eventID,
		).Scan(&count); err != nil {
			t.Fatalf("数 inbox：%v", err)
		}
		return count
	}
	for _, eventID := range []string{cardedID, uncardedID, "EVREQ-SYN-MISSING"} {
		if n := inboxRows(eventID); n != 0 {
			t.Fatalf("%s 未决必须回滚：inbox 行数 = %d, want 0", eventID, n)
		}
	}
	if n := inboxRows("EVREQ-SYN-POISON"); n != 1 {
		t.Fatalf("毒丸要拒收入账不重投：inbox 行数 = %d, want 1", n)
	}
}

// saValue 是 SA 值对象构造器的测试速记：构造失败即用例失败。
func saValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// Covers: 路由表第六条——TF 权威交接登记只投 VE 投影，不 FanOut 给 PS。手法同前五条：
// 毒丸载荷（缺 tenantId/object/scope/version 四维之一）让消费门显式拒收入账并交回
// nil，因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。
func TestARegisteredTransportHandoverReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "handover-1", veinbox.TransportHandoverRegisteredEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把交接登记投给 VE",
			published, recordedFailureCode(t, db, "handover-1"))
	}
}

// Covers: 路由表的外部承运轨迹一条（label-channel/16）——TF 判断过有效时间的外部承运轨迹事实
// 只投 VE 投影，不 FanOut 给 PS：它是来源事实，不是有效交付，不构成终局。手法同前几条：毒丸
// 载荷（缺 tenantId/fact/version 三维之一）让消费门显式拒收入账并交回 nil，因此这一条会被定稿。
// 漏挂或挂错的话这里撞的是无订阅者。
func TestAJudgedExternalCarrierTrackingReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "external-tracking-1", veinbox.ExternalCarrierTrackingJudgedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把外部承运轨迹事实投给 VE",
			published, recordedFailureCode(t, db, "external-tracking-1"))
	}
}

// Covers: 路由表第八条——NR 包裹级初始路由判断只投 VE 投影，不 FanOut（应消费方还有
// NO/TF，但两侧消费者今天不存在，登记接不住的比不登记更糟）。手法同前几条：毒丸载荷
// （缺六维之一）让消费门显式拒收入账并交回 nil，因此这一条会被定稿。漏挂或挂错的话
// 这里撞的是无订阅者。
func TestAFormedInitialRouteReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "initial-route-1", veinbox.InitialRouteFormedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把初始路由判断投给 VE",
			published, recordedFailureCode(t, db, "initial-route-1"))
	}
}

// submittedForChain 是发布侧 OutboxShipmentRequestSubmittedHandoff 写出的载荷形状，
// 各维都填得出领域标识。它用来证「整条接受判断链在真依赖图上跑得动」——毒丸载荷在译码
// 处就被拦下，证不到编排那一层。
const submittedForChain = `{
	"tenantId": "tenant-a",
	"customerAccountId": "customer-1",
	"source": "source-a",
	"sourceRequestKey": "key-1",
	"shipmentRequestId": "request-1",
	"submissionBatchId": "batch-1",
	"submissionVersionId": "version-1",
	"declaredParcelIds": ["parcel-1"]
}`

// Covers: 路由表新增的这一条——PS「委托已提交」投给接受判断链消费者（UC-PS-001 步骤 8
// 「自动接受」）。手法同前几条：毒丸载荷（各维皆空）让消费门显式拒收入账并交回 nil，
// 因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。
//
// 方向与前几条都不同：发布侧与消费侧同属 parcel-shipment，本进程里唯一一条自发自收的
// 链。它照样得走路由表——提交事务只负责把意图落进 outbox，推进判断是下一拍的事。
func TestASubmittedShipmentRequestReachesTheAcceptanceChainThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "submitted-1", psinbox.ShipmentRequestSubmittedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把已提交委托投给接受判断链",
			published, recordedFailureCode(t, db, "submitted-1"))
	}
}

// Covers: 路由表续办这一条——PS「复核已完成」投给接受判断链的第二扇门（ADR-0086
// Decision 二）。手法同前几条：毒丸载荷（各维皆空）让消费门显式拒收入账并交回 nil，
// 因此这一条会被定稿。漏挂的话，复核完成事务铸出的每一封续办信封都撞
// dispatch.no_subscriber——暂停的委托从此没人续办。
func TestACompletedManualReviewReachesTheResumeGateThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "review-completed-1", psinbox.ManualReviewCompletedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把复核完成投给续办门",
			published, recordedFailureCode(t, db, "review-completed-1"))
	}
}

// Covers: 路由表第三扇门——PC「参数已登记」投给接受判断链的登记续办门（ADR-0094
// Decision 四）。手法同前几条：毒丸载荷（两键皆空）让消费门显式拒收入账并交回 nil，因此
// 这一条会被定稿。漏挂的话，时点策略登记事务铸出的每一封都撞 dispatch.no_subscriber——
// 停在`等待运营登记`的委托从此没人续办，正是 D4 说的那个更安静的永久停滞。
func TestACompletedOperatorRegistrationReachesTheResumeGateThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "operator-registration-1", psinbox.OperatorRegistrationCompletedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把参数已登记投给登记续办门",
			published, recordedFailureCode(t, db, "operator-registration-1"))
	}
}

// Covers: 登记续办门在**生产依赖图**上真的接得起来：一份两键齐全的载荷穿过译码去问真队列
// 读口，该租户没有委托在等，本封按处理完毕入账并定稿。它比上一条重一格——上一条毒丸在
// 译码处就被拦下，队列读口一步没走过；这里证的是 ShipmentRequests 那口按 0013 的谓词真能
// 在生产图上答出「没有人等」。
func TestACompletedOperatorRegistrationWithNobodyWaitingSettlesOnTheProductionGraph(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "operator-registration-2", psinbox.OperatorRegistrationCompletedEventType,
		`{"tenantId":"SYN-TENANT-01","registrationKind":"AS_OF_POLICY"}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——空队列的登记信封该入账定稿",
			published, recordedFailureCode(t, db, "operator-registration-2"))
	}
}

// Covers: 路由表第四扇门——PS「新提交版本已形成」投给接受判断链的受控补充续办门（ADR-0106
// Decision 三）。手法同前几条：毒丸载荷（各维皆空）让消费门显式拒收入账并交回 nil，因此这一条
// 会被定稿。漏挂的话，受控补充事务铸出的每一封都撞 dispatch.no_subscriber——停在`等待受控补充`
// 并已入账的委托从此没人续办，正是决定三禁止的「只翻转不给触发」那种更安静的永久停滞。
func TestAFormedSubmissionVersionReachesTheResumeGateThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "submission-version-formed-1", psinbox.SubmissionVersionFormedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把新提交版本已形成投给受控补充续办门",
			published, recordedFailureCode(t, db, "submission-version-formed-1"))
	}
}

// Covers: 接受判断链在**生产依赖图**上真的接得起来，且实例半边空着时停成未决而不是报错。
//
// 这一条比前一条重得多。前一条只证路由表挂对了人：毒丸在译码处就被拦下，编排那一层
// 一步都没走过。这里给一份各维齐全的载荷，链会一路走到第一步的商业依据——本库没有登记
// 过任何解析键，于是它按「显式未配置」答解析未决，编排收成`本轮没形成决定`，消费门挂
// 未决哨兵，路由条目翻成 dispatch.consumer_undecided。
//
// 三件事同时被钉住：
//   - acceptanceChainConsumer 那张依赖图（商业依据、可达性、财务控制、形成决定四组构造）
//     全部构造得出来，没有哪一个 nil 依赖让装配悄悄成立；
//   - 空实例半边的答复是「等参数」而不是「炸开」——首发就该停在这里；
//   - 未决哨兵登记生效，失败码不是 dispatch.publish_failed。
//
// 落成 publish_failed 的话运维会去查一个并不存在的故障，而实情是这个租户还没登记解析键。
func TestAnUnconfiguredAcceptanceChainStallsAsUndecidedOnTheRealGraph(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "submitted-live-1", psinbox.ShipmentRequestSubmittedEventType, submittedForChain)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("未决的一封被当成发布成功定稿了：published = %d", published)
	}
	if got := recordedFailureCode(t, db, "submitted-live-1"); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided", got)
	}
}

// Covers: 同一场未决，停在哪一站、原因是什么，在 dispatch 这一侧读得到（ADR-0095 Decision 二）。
//
// 上一条只证失败码落对了格，而失败码按恢复动作取值，本来就答不出「停在哪一站」——那句话
// 消费门写在错误正文里（`stage …, reason …`）。这里在**生产依赖图**上接一个替身观察口，
// 钉的是那句原文穿过 WithUndecidedSentinels 的 `%w: %w` 包装之后仍然在。平台包里那条
// 「交出的是原始 err」用例钉的是替身发布器上的 errors.Is，证不到这一层：包装形状归路由
// 条目，只有真图上才看得见它有没有把原文吞掉。
//
// 断言只认 "stage " 与 "reason " 两个词，不认具体取值：本库空着时停在哪一站是实例半边
// 的事实，这条用例钉的是「说了」，不是「说了什么」。
func TestAnUndecidedStallSurfacesStageAndReasonToTheObserver(t *testing.T) {
	var observed []error
	beat, db, store := wiredBeat(t, dispatch.WithDeliveryFailureObserver(
		func(_ eventing.Delivery, _ eventing.FailureCode, err error) {
			observed = append(observed, err)
		}))
	enqueueForBeat(t, db, store, "submitted-live-2", psinbox.ShipmentRequestSubmittedEventType, submittedForChain)

	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if len(observed) != 1 {
		t.Fatalf("观察口收到 %d 条失败，want 1", len(observed))
	}
	if !errors.Is(observed[0], psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("观察口交出的不是消费门的原始未决：%v", observed[0])
	}
	text := observed[0].Error()
	for _, want := range []string{"stage ", "reason "} {
		if !strings.Contains(text, want) {
			t.Fatalf("错误正文不含 %q，运维读不出停在哪一站：%q", want, text)
		}
	}
}

// beatWithAcceptanceChainConsumer 用生产的接受链哨兵名单包一个替身，接成一拍。
//
// 名单取 acceptanceChainUndecidedSentinels 本身：测试里重列一份会让漏登记的哨兵在测试
// 里绿、在生产里塌成 dispatch.publish_failed。
func beatWithAcceptanceChainConsumer(t *testing.T, inner dispatch.Consumer) (Beat, *bentopg.DB, *outbox.Store) {
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
	routed, err := dispatch.WithUndecidedSentinels(inner, acceptanceChainUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包装未决哨兵：%v", err)
	}
	config := dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{psinbox.ShipmentRequestSubmittedEventType: routed},
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

// Covers: 接受判断链这条链的失败分格——名单只有一格落 dispatch.consumer_undecided，
// 其余保持 dispatch.publish_failed。
//
// 三步各自的未决原因（时点未配置、可达性权威答不出、控制策略未配置、人工复核待办……）
// 在编排里已经收成同一个「本轮没形成决定」，因此这里只有一个哨兵，而不是每步一个。
// 另外三格要运维做的事与「等依赖」相反：
//   - 封闭集合外是编程错误，重投改不了它；
//   - 装配缺件是本进程漏接了一步，等多久也不会长出一个没接上的编排；
//   - 空成员清单是发布侧发错了，`已提交`委托必有声明成员。
//
// 三者若落成未决，这一封会一路重投到失败预算耗尽，而日志上看起来像「一直在等某个依赖」。
func TestAcceptanceChainFailuresLandInTheRightPartition(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"本轮没形成决定", psinbox.ErrAcceptanceChainUndecided, "dispatch.consumer_undecided"},
		{"封闭集合外", psinbox.ErrUnexpectedAcceptanceChainOutcome, "dispatch.publish_failed"},
		{"装配缺件", psapplication.ErrAcceptanceChainNotAssembled, "dispatch.publish_failed"},
		{"成员清单为空", psapplication.ErrAcceptanceChainHasNoMembers, "dispatch.publish_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithAcceptanceChainConsumer(t, &stallingConsumer{err: test.err})
			enqueueForBeat(t, db, store, "submitted-1", psinbox.ShipmentRequestSubmittedEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "submitted-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
			}
		})
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

// beatWithAcceptanceRederiveConsumer 用生产的补派生哨兵名单包一个替身，接成一拍。
//
// 名单取 veAcceptanceRederiveUndecidedSentinels 本身：测试里重列一份会让漏登记的
// 哨兵在测试里绿、在生产里塌成 dispatch.publish_failed。
func beatWithAcceptanceRederiveConsumer(t *testing.T, inner dispatch.Consumer) (Beat, *bentopg.DB, *outbox.Store) {
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
	routed, err := dispatch.WithUndecidedSentinels(inner, veAcceptanceRederiveUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包装未决哨兵：%v", err)
	}
	config := dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{veinbox.AcceptanceDecisionFormedEventType: routed},
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

// Covers: 补派生这条链的失败分格——五个可续办哨兵落 dispatch.consumer_undecided，
// 其余保持 dispatch.publish_failed。注意歧义在本路是未决（AT-VE-152 机制拒绝自动
// 采认，运维去 PS 解开歧义，解开前信封如实卡着），与 PS 采用三路的 publish_failed
// 相反——那三路的歧义卡的是采用判断本身，这里卡的是账户维，恢复动作同是「人去看」，
// 但本路解开后重投即自愈，不需要人动信封。其余硬失败格：
//   - 信封与委托行/反查零行自相矛盾是仓储不变量已破，重投不自愈；
//   - 信封账户与权威反查不符是本票立的硬闸（基线上结构不可达，到达即不变量已破）；
//   - 集合外状态字是编程错误；
//   - 交接待发要查 outbox 下游；
//   - 封闭集合外不留兜底。
func TestAcceptanceRederiveFailuresLandInTheRightPartition(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"清单读口调不通", veps.ErrDeclaredParcelsUnavailable, "dispatch.consumer_undecided"},
		{"投影库调不通", veps.ErrDerivedProjectionUnreadable, "dispatch.consumer_undecided"},
		{"反查读口调不通", veps.ErrCustomerAccountUnavailable, "dispatch.consumer_undecided"},
		{"反查歧义", veps.ErrAmbiguousCustomerAccount, "dispatch.consumer_undecided"},
		{"派生编排未决", veconsume.ErrCustomerViewUndecided, "dispatch.consumer_undecided"},
		{"信封与委托记录不符", veps.ErrAcceptanceRecordInconsistent, "dispatch.publish_failed"},
		{"账户不匹配", veps.ErrCustomerAccountMismatch, "dispatch.publish_failed"},
		{"集合外状态字", veps.ErrAcceptanceDecisionUntranslatable, "dispatch.publish_failed"},
		{"交接待发", veconsume.ErrCustomerViewHandoffPending, "dispatch.publish_failed"},
		{"封闭集合外", veconsume.ErrUnexpectedCustomerViewOutcome, "dispatch.publish_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithAcceptanceRederiveConsumer(t, &stallingConsumer{err: test.err})
			enqueueForBeat(t, db, store, "accepted-1", veinbox.AcceptanceDecisionFormedEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "accepted-1"); got != test.wantCode {
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

// Covers: ADR-0049 决定三「没有订阅者的事件类型显式失败并入账，不静默丢弃」。它此刻会
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
