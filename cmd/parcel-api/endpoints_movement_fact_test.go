package main

func init() {
	registerUnwired(movementFactWiring{movementFact: unwiredMovementFact{}})
	registerWorkFactProbe("/transport-fulfillment/movement-facts",
		`{"fact":"SYN-MOVE-08-06","schedule":"SYN-SCHEDULE/linehaul-06","kind":"ARRIVAL","location":"SYN-NODE-SIN-HUB",`+
			`"version":"v1","occurredAt":"2026-09-24T18:00:00+08:00"}`)
}
