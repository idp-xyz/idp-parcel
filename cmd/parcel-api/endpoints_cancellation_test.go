package main

func init() {
	registerUnwired(cancellationWiring{cancellation: unwiredCancellation{}})
}
