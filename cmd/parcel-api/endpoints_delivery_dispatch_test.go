package main

func init() {
	registerUnwired(deliveryDispatchWiring{deliveryDispatchTrigger: unwiredDeliveryDispatchTrigger{}})
	registerWorkFactProbe("/transport-fulfillment-delivery-dispatch-triggers",
		`{"segment":"SYN-SEGMENT-08-09","object":"SYN-PARCEL-08-09","occurredAt":"2026-09-25T07:00:00+08:00"}`)
}
