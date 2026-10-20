package main

func init() {
	registerUnwired(departureGatePreviewWiring{departureGatePreview: unwiredDepartureGatePreview{}})
	registerWorkFactProbe("/transport-fulfillment/departure-gate-previews",
		`{"schedule":"SYN-SCHEDULE/linehaul-06","segment":"SYN-SEGMENT-08-06","departsAt":"2026-09-24T14:00:00+08:00"}`)
}
