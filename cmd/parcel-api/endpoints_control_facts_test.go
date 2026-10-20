package main

func init() {
	registerUnwired(controlFactsWiring{
		handover:           unwiredHandover{},
		pickupRegistration: unwiredPickupRegistration{},
		pickupCorrection:   unwiredPickupCorrection{},
		pickupAttempt:      unwiredPickupAttempt{},
	})
	registerWorkFactProbe("/transport-fulfillment/offsite-pickups",
		`{"object":"SYN-PARCEL-08-01","task":"SYN-TASK-08-01","attempt":"SYN-ATTEMPT-08-01","place":"SYN-PLACE/shipper-dock-01",`+
			`"control":"SYN-CONTROL/signed-pickup-01","executedBy":"SYN-COURIER-08","occurredAt":"2026-09-24T08:30:00+08:00"}`)
	registerWorkFactProbe("/transport-fulfillment/offsite-pickup-attempts",
		`{"sourceId":"SYN-DEVICE-08/attempt-01","task":"SYN-TASK-08-02","attempt":"SYN-ATTEMPT-08-02","executedBy":"SYN-COURIER-08",`+
			`"place":"SYN-PLACE/shipper-dock-02","plannedFrom":"2026-09-24T08:00:00+08:00","plannedTo":"2026-09-24T10:00:00+08:00",`+
			`"arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-02","objects":[{"object":"SYN-PARCEL-08-02",`+
			`"outcome":"PICKED_UP","control":"SYN-CONTROL/signed-02","occurredAt":"2026-09-24T08:45:00+08:00"}]}`)
	registerWorkFactProbe("/transport-fulfillment/offsite-pickup-corrections",
		`{"object":"parcel-1","attempt":"attempt-1","predecessorVersion":"pickup-result/v1","place":"door-2",`+
			`"control":"control-2","executedBy":"courier-2","occurredAt":"2026-09-05T08:00:00Z","correctedAt":"2026-09-06T09:00:00Z"}`)
	registerWorkFactProbe("/transport-fulfillment/handovers",
		`{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","releasedBy":"SYN-PARTY/hub-08","receivedBy":"SYN-PARTY/linehaul-08",`+
			`"verdict":"HANDED_OVER","releasingEvidence":"SYN-EVIDENCE/release-05","receivingEvidence":"SYN-EVIDENCE/receive-05",`+
			`"rule":"SYN-RULE/handover@v1","version":"v1","judgedAt":"2026-09-24T13:00:00+08:00"}`)
	registerWorkFactProbe("/transport-fulfillment/handover-corrections",
		`{"object":"SYN-PARCEL-08-05","scope":"SYN-SCOPE/hub-dock-05","predecessorVersion":"v1","verdict":"HANDED_OVER",`+
			`"releasingEvidence":"SYN-EVIDENCE/release-05","receivingEvidence":"SYN-EVIDENCE/receive-05",`+
			`"rule":"SYN-RULE/handover@v1","newVersion":"v2","correctedAt":"2026-09-24T14:00:00+08:00"}`)
}
