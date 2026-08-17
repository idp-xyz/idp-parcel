package main

import "errors"

// errDispatcherNotWired 标明组合根未完成。缺口已经换了位置，这里记的是现在这一处。
//
// **发布通道不再是缺口。** ADR-0049 裁定首发采用进程内直投并已 Accepted，
// `dispatch.DirectPublisher` 是它的实现：按 `Envelope.Type` 查显式路由表、对每次投递
// 施加明显小于租约的超时、消费门提交才算持久接受、无订阅者显式失败不静默丢弃。
// Claimer 与 Finalizer 是同一个对象（框架的 `postgres/outbox.Store`），Clock 要的是
// 真实时钟。四个依赖至此都有真实来源。
//
// **缺的是消费者。** 直投的路由表必须非空——空表上线会让每一类事件都撞
// `ErrNoSubscriber` 并逐个阻塞分区，因此 `NewDirectPublisher` 在构造期就拒绝空表。
// 而本仓唯一的消费者 `nrinbox.AcceptanceConsumer` 今天装不起来：它要一个
// `DecisionHandler`，真实实现是 `nrparcelshipment.RouteOnAcceptanceAdapter`，后者要
// `nrapplication.CreateInitialRouteHandler`，而那个 handler 要
// `nrports.InitialRouteEvidenceView`——**该端口至今没有任何生产实现**，且按它自己的
// 包注释不得先写一份默认实现：逐候选事实的生成、过滤与排序规则属 `PAR-NET-14`，
// 登记册状态待提供。
//
// 红线因此原样成立，只是换了对象：不得给 Publisher 塞替身，同样不得给证据视图塞
// 一份「开发用」的默认实现。前者让事件被真的定稿而消失，后者让路由按虚构的网络
// 事实形成计划——两者都是现场看不出来的假。
//
// 解开这一处要的不是接线，是那份证据视图的实例半边落地；接线本身在它到位后是几十
// 行的事。
var errDispatcherNotWired = errors.New(
	"parcel-dispatch: dispatcher is not wired: no consumer can be assembled, the route table would be empty")

// assembleDispatcher 是派发一拍的装配点（组合根）。
//
// 现在交回明确未决：这不是缺口的另一种写法，而是把「发布通道已实现、四个依赖都有
// 真实来源、唯一的消费者因证据视图缺生产实现而装不起来」固化在具名缝上。
//
// 刻意不做「先连库、再拿空路由表去撞构造期检查」：那样要先建连接池才失败，报出来的
// 是「路由表为空」而不是「为什么空」，而后者才是要修的东西。
func assembleDispatcher() (Beat, error) {
	return nil, errDispatcherNotWired
}
