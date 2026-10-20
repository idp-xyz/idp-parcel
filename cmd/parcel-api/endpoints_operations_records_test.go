package main

func init() {
	registerUnwired(operationsRecordsWiring{
		nodeOperationsRecords:       unwiredNodeOperationsRecords{},
		transportFulfillmentRecords: unwiredTransportFulfillmentRecords{},
		handoverScopeSummary:        unwiredHandoverScopeSummary{},
	})
}
