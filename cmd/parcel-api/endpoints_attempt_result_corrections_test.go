package main

import (
	"context"

	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

func init() {
	registerUnwired(attemptResultCorrectionsWiring{
		pickup:   unwiredPickupAttemptResultCorrection{},
		delivery: unwiredDeliveryAttemptResultCorrection{},
	})
	registerWorkFactProbe("/transport-fulfillment/offsite-pickup-attempt-result-corrections",
		`{"sourceId":"SYN-DEVICE-OFFSITE/attempt-90","object":"SYN-PARCEL-90-01","outcome":"CUSTOMER_ABSENT",`+
			`"basis":"SYN-REASON/partner-confirms-absent","source":"SYN-SOURCE/partner-late-status",`+
			`"occurredAt":"2026-09-24T08:42:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`)
	registerWorkFactProbe("/transport-fulfillment/delivery-attempt-result-corrections",
		`{"attempt":"SYN-DELIVERY-ATTEMPT-90","object":"SYN-PARCEL-90-02","outcome":"DELIVERED",`+
			`"source":"SYN-SOURCE/address-review-confirmed","occurredAt":"2026-09-25T10:07:00+08:00","correctedAt":"2026-09-25T15:00:00+08:00"}`)
}

// unwiredPickupAttemptResultCorrection 与 unwiredDeliveryAttemptResultCorrection 是更正两口的未接线占位：装配测试只看接入面，编排走不到。
type unwiredPickupAttemptResultCorrection struct{}

func (unwiredPickupAttemptResultCorrection) Correct(
	context.Context, tfapp.CorrectPickupAttemptResultCommand,
) (tfapp.CorrectPickupAttemptResultResult, error) {
	return tfapp.CorrectPickupAttemptResultResult{}, errOrchestrationNotWired
}

type unwiredDeliveryAttemptResultCorrection struct{}

func (unwiredDeliveryAttemptResultCorrection) Correct(
	context.Context, tfapp.CorrectDeliveryAttemptResultCommand,
) (tfapp.CorrectDeliveryAttemptResultResult, error) {
	return tfapp.CorrectDeliveryAttemptResultResult{}, errOrchestrationNotWired
}
