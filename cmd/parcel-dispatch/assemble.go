package main

import "errors"

// errDispatcherNotWired 标明组合根未完成。四个依赖里已经查明三个有真实来源，缺口
// 收敛到发布通道这一处。
//
// Claimer 与 Finalizer 不缺，而且是同一个对象：框架的 `postgres/outbox.Store` 同时
// 满足 `eventing.OutboxClaimer` 与 `eventing.OutboxFinalizer`，构造链
// `pgxpool.New` → `bentopg.NewDB(…, WithSchema(migrate.SchemaBento))` →
// `outbox.NewStore(db)` 已在 `internal/platform/dispatch` 的真库门禁与各上下文的
// `Outbox*Handoff` 适配器上走通。Clock 也不缺：一拍要的是真实时钟，`time.Now()`
// 就是它的生产实现而不是替身。
//
// 缺的是 `eventing.Publisher` 的生产实现，而框架按合同不拥有它——Bento 的 eventing
// 合同写明框架「不拥有消费者的业务事件、订阅关系、重试业务规则或消息中间件」。框架
// 里两处 `Publish` 都用不得：`eventing.PublisherFunc` 只是函数适配器（func 体仍要有
// 人写），`testkit.RecordingPublisher` 是测试替身且被框架自己的 `production-no-testkit`
// 依赖门禁挡在生产之外。本仓至今没定过发布通道形态，提案见 ADR-0049。
var errDispatcherNotWired = errors.New("parcel-dispatch: dispatcher is not wired: publish channel is undecided")

// assembleDispatcher 是派发一拍的装配点（组合根）。`dispatch.Dispatcher` 已能执行
// DispatchOnce，接它进本进程只差发布通道一个依赖。
//
// 现在交回明确未决：这不是缺口的另一种写法，而是把「三个依赖有真实来源、发布通道
// 形态未定」固化在具名缝上。红线：未接线期间不得出现任何「开发用」的空转一拍或
// 内存 Outbox——接三个真的再给 Publisher 塞个替身，正是这条要拦的东西。
func assembleDispatcher() (Beat, error) {
	return nil, errDispatcherNotWired
}
