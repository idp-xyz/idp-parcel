package main

func init() {
	registerUnwired(reviewWiring{manualReview: unwiredManualReview{}, rejection: unwiredRejection{}})
}
