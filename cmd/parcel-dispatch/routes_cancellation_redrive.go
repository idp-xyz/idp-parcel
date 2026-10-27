package main

import (
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// 取消先后冲突被认定「取消依据不成立」→ 被这份取消挡住的来源重新进入原来的编排（ADR-0295 决定四）。只投 PS 一路：信封是本上下文自己发的续办信封，
// 没有别的上下文要消费它——取消在客户侧与下游已释放的东西怎么改口，属收寄后处置那一面，由它日后订阅这封信。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		redrive, err := cancellationNotUpheldRedriveConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: psinbox.CancellationNotUpheldEventType, position: positionPrimary, consumer: redrive,
				sentinels: cancellationNotUpheldRedriveUndecidedSentinels, label: "cancellation not upheld redrive",
			},
		}, nil
	})
}
