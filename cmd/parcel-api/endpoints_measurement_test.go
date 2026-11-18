package main

func init() {
	registerUnwired(measurementWiring{handler: unwiredActualMeasurement{}})
	registerWorkFactProbe("/node-operations/measurements",
		`{"sourceId":"SYN-DEVICE-08/WEIGH-1","objectKind":"HANDLING_UNIT","objectId":"SYN-HU-08-0001",`+
			`"location":"bench-2","basis":"intake-1","performedBy":"scale-7","evidenceRef":"WEIGH/7",`+
			`"occurredAt":"2026-11-17T10:00:00+08:00","weightValue":"2.10","weightUnit":"KG",`+
			`"length":"20","width":"15","height":"10","dimensionUnit":"CM"}`)
	registerWorkFactProbe("/node-operations/measurement-invalidations",
		`{"sourceId":"SYN-DEVICE-08/VOID-1","targetSourceId":"SYN-DEVICE-08/WEIGH-1","reason":"称错件",`+
			`"performedBy":"scale-7","evidenceRef":"WEIGH/8","occurredAt":"2026-11-17T11:00:00+08:00"}`)
}
