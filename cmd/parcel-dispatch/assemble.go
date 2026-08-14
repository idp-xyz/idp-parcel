package main

import "errors"

// errDispatcherNotWired 标明组合根未完成：Claimer / Finalizer / Publisher / Clock
// 来自真实 Outbox 与发布通道，本票不接线真实 DB，也不用开发替身冒充生产默认。
var errDispatcherNotWired = errors.New("parcel-dispatch: dispatcher is not wired")

// assembleDispatcher 是派发一拍的装配点（组合根）。`dispatch.Dispatcher` 已能执行
// DispatchOnce；把它接到本进程需要存储与发布通道，那些接线是后续票。
//
// 现在交回明确未决：这不是缺口的另一种写法，而是把「循环骨架已开、一拍尚未装配」
// 固化在具名缝上。红线：未接线期间不得出现任何「开发用」的空转一拍或内存 Outbox。
func assembleDispatcher() (Beat, error) {
	return nil, errDispatcherNotWired
}
