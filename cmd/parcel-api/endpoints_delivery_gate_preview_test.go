package main

import (
	"context"

	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

func init() {
	registerUnwired(deliveryGatePreviewWiring{deliveryGatePreview: unwiredDeliveryGatePreview{}})
	registerWorkFactProbe("/transport-fulfillment/delivery-gate-previews",
		`{"object":"SYN-PARCEL-08-10","deliversAt":"2026-09-06T15:30:00+08:00"}`)
}

// unwiredDeliveryGatePreview 是派送出发前交付门禁问询口的占位（ADR-0238）。
type unwiredDeliveryGatePreview struct{}

func (unwiredDeliveryGatePreview) Preview(context.Context, tfapp.PreviewDeliveryGateCommand) (tfapp.PreviewDeliveryGateResult, error) {
	return tfapp.PreviewDeliveryGateResult{}, errOrchestrationNotWired
}
