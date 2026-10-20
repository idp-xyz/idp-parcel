package main

import (
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// 面单渠道终局判断的三扇门（labelFinalJudgmentCore）：面单交易判断意图（lc/26）、关闭 / 重开决定判断意图（lc/27），以及
// TF 实际承运商首次有效收寄登记 → 只投 PS 面单渠道终局判断（lc/25，ADR-0135；不 FanOut：VE 侧今天没有它的消费者，
// 登记接不住的比不登记更糟）。后者只接 `carrier-first-effective-pickup.registered`，不接
// `external-carrier-tracking.judged`——后者按 TF CONTEXT 不构成收寄，在本进程只投 VE 投影。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		labelJudgments, err := judgeLabelFinalOnLabelTransactionConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		decisionJudgments, err := judgeLabelFinalOnContinuedAttemptDecisionConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		pickupJudgments, err := judgeLabelFinalOnCarrierFirstEffectivePickupConsumer(sources.db, sources.outbox, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: psinbox.LabelTransactionJudgmentDueEventType, position: positionPrimary, consumer: labelJudgments,
				sentinels: labelTransactionJudgmentUndecidedSentinels, label: "label transaction judgment",
			},
			{
				event: psinbox.ContinuedAttemptDecisionJudgmentDueEventType, position: positionPrimary, consumer: decisionJudgments,
				sentinels: continuedAttemptDecisionJudgmentUndecidedSentinels, label: "continued attempt decision judgment",
			},
			{
				event: psinbox.CarrierFirstEffectivePickupRegisteredEventType, position: positionPrimary, consumer: pickupJudgments,
				sentinels: carrierFirstEffectivePickupJudgmentUndecidedSentinels, label: "carrier first effective pickup judgment",
			},
		}, nil
	})
}
