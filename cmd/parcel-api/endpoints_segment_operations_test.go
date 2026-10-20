package main

func init() {
	registerUnwired(segmentOperationsWiring{
		segmentCloser:      unwiredSegmentCloser{},
		dispatchTaskOpener: unwiredDispatchTaskOpener{},
		loadAssigner:       unwiredLoadAssigner{},
		participationEnder: unwiredParticipationEnder{},
	})
}
