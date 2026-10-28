package main

import (
	"context"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

func init() {
	registerUnwired(releaseGateVerificationWiring{verifier: unwiredReleaseGateVerifier{}})
}

// unwiredReleaseGateVerifier 是核对口的未接线占位：装配测试只看接入面，编排走不到。
type unwiredReleaseGateVerifier struct{}

func (unwiredReleaseGateVerifier) Handle(context.Context, customsapp.VerifyReleaseGateCommand) (customsapp.VerifyReleaseGateResult, error) {
	return customsapp.VerifyReleaseGateResult{}, errOrchestrationNotWired
}
