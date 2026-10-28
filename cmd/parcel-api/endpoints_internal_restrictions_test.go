package main

import (
	"context"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

func init() {
	registerUnwired(internalRestrictionsWiring{
		establisher: unwiredInternalRestrictions{},
		decider:     unwiredInternalRestrictions{},
	})
}

// unwiredInternalRestrictions 是两口的未接线占位：装配测试只看接入面，编排走不到。
type unwiredInternalRestrictions struct{}

func (unwiredInternalRestrictions) Establish(
	context.Context, customsapp.EstablishInternalRestrictionCommand,
) (customsapp.InternalRestrictionResult, error) {
	return customsapp.InternalRestrictionResult{}, errOrchestrationNotWired
}

func (unwiredInternalRestrictions) Decide(
	context.Context, customsapp.DecideInternalRestrictionCommand,
) (customsapp.InternalRestrictionResult, error) {
	return customsapp.InternalRestrictionResult{}, errOrchestrationNotWired
}
