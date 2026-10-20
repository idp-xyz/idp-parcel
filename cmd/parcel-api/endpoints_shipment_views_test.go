package main

func init() {
	registerUnwired(shipmentViewsWiring{
		requestViews:     unwiredRequestViews{},
		reviewQueue:      unwiredReviewQueue{},
		reviewJudgments:  unwiredReviewJudgments{},
		dispositionQueue: unwiredDispositionQueue{},
	})
}
