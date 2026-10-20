package main

func init() {
	registerUnwired(deliveryWiring{delivery: unwiredDelivery{}, deliveryAttempt: unwiredDeliveryAttempt{}})
	registerWorkFactProbe("/transport-fulfillment/deliveries",
		`{"attempt":"SYN-ATTEMPT-08-10","object":"SYN-PARCEL-08-10","method":"HANDED_TO_RECIPIENT",`+
			`"recipient":"SYN-RECIPIENT/consignee-10","proof":"SYN-POD/signature-10"}`)
	registerWorkFactProbe("/transport-fulfillment/delivery-proof-corrections",
		`{"attempt":"SYN-ATTEMPT-08-10","object":"SYN-PARCEL-08-10","predecessorVersion":"SYN-DRV-08-10-1",`+
			`"newProof":"SYN-POD/signature-10b","correctedAt":"2026-09-06T08:00:00Z"}`)
	registerWorkFactProbe("/transport-fulfillment/delivery-attempts",
		`{"task":"SYN-DISPATCH-08-13","attempt":"SYN-DELIVERY-ATTEMPT-08-13","executedBy":"SYN-COURIER-08",`+
			`"place":"SYN-PLACE/consignee-13","plannedFrom":"2026-09-25T09:00:00+08:00","plannedTo":"2026-09-25T12:00:00+08:00",`+
			`"arrivedAt":"2026-09-25T10:00:00+08:00","evidence":"SYN-EVIDENCE/arrival-13",`+
			`"objects":[{"object":"SYN-PARCEL-08-13","outcome":"DELIVERED","occurredAt":"2026-09-25T10:05:00+08:00"}]}`)
}
