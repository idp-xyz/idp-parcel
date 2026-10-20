package main

import (
	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
)

// 待识别实物的重核（recheckPendingIdentitiesConsumer）：交回的一个 Consumer 按事件类型把信封分给各扇门，路由表上每一类各挂
// 一行、挂的是同一个值。身份核对读不动在那几扇门里已译成未决，路由条目处不再翻。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		identityRechecks, err := recheckPendingIdentitiesConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: noinbox.ExternalIdentifierRelationFormedEventType, position: positionPrimary, consumer: identityRechecks,
				untranslated: true, label: "identity recheck",
			},
			{
				event: noinbox.UncheckedPendingIdentityRecordedEventType, position: positionPrimary, consumer: identityRechecks,
				untranslated: true, label: "identity recheck",
			},
			{
				event: noinbox.CustomerParcelNumberDeclarationRetiredEventType, position: positionPrimary, consumer: identityRechecks,
				untranslated: true, label: "identity recheck",
			},
			{
				event: noinbox.DeliveryScopeEstablishedEventType, position: positionPrimary, consumer: identityRechecks,
				untranslated: true, label: "identity recheck",
			},
		}, nil
	})
}
