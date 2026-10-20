package main

import (
	ccinbox "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/inbox"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
)

// SA 资金事实采用 → CC 入向登记（sa-cc/03）、CC 付款核对形成 → SA 结算输入采用（sa-cc/09；同一条缝的反方向，各记各的
// inbox 账）。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		fundsFacts, err := receiveExternalFundsFactConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		verifications, err := adoptDutyPaymentVerificationConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: ccinbox.ExternalFundsFactAdoptedEventType, position: positionPrimary, consumer: fundsFacts,
				sentinels: externalFundsFactUndecidedSentinels, label: "external funds fact",
			},
			{
				event: sainbox.DutyPaymentVerificationFormedEventType, position: positionPrimary, consumer: verifications,
				sentinels: dutyPaymentVerificationUndecidedSentinels, label: "duty payment verification",
			},
		}, nil
	})
}
