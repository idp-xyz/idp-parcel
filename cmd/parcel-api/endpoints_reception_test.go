package main

func init() {
	registerUnwired(receptionWiring{reception: unwiredReception{}})
	registerWorkFactProbe("/node-operations/receptions",
		`{"sourceId":"SYN-DEVICE-08/000001","deliveredBy":"SYN-COURIER-08","handlingUnit":"SYN-HU-08-0001",`+
			`"claim":"REFUSED","refusalReason":"SYN 外包装破损拒收","occurredAt":"2026-09-24T09:00:00+08:00"}`)
}
