package main

func init() {
	registerUnwired(transportCommissionWiring{
		transportCommissionSubmission: unwiredTransportCommissionSubmission{},
		bookingRequestSubmission:      unwiredBookingFaces{},
		bookingAnswer:                 unwiredBookingFaces{},
	})
}
