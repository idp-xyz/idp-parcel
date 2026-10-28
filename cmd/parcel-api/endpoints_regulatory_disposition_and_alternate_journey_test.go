package main

import (
	"context"

	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

func init() {
	registerUnwired(regulatoryDispositionAndJourneyWiring{
		acceptance: unwiredRegulatoryDispositionAcceptor{},
		journey:    unwiredAlternateJourneyStarter{},
	})
}

// unwiredRegulatoryDispositionAcceptor 与 unwiredAlternateJourneyStarter 是两口的未接线占位：装配测试只看接入面，编排走不到。
type unwiredRegulatoryDispositionAcceptor struct{}

func (unwiredRegulatoryDispositionAcceptor) Handle(
	context.Context, tfapp.AcceptRegulatoryDispositionCommand,
) (tfapp.AcceptDispositionResult, error) {
	return tfapp.AcceptDispositionResult{}, errOrchestrationNotWired
}

type unwiredAlternateJourneyStarter struct{}

func (unwiredAlternateJourneyStarter) Handle(
	context.Context, tfapp.StartAlternateJourneyCommand,
) (tfapp.StartAlternateJourneyResult, error) {
	return tfapp.StartAlternateJourneyResult{}, errOrchestrationNotWired
}
