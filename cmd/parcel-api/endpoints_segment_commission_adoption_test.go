package main

func init() {
	registerUnwired(segmentCommissionAdoptionWiring{
		segmentCommissionAdoption:       unwiredSegmentCommissionAdoption{},
		transportCommissionCancellation: unwiredCancellationFaces{},
		bookingRequestCancellation:      unwiredCancellationFaces{},
	})
}
