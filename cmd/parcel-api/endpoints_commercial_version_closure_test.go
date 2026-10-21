package main

import (
	"context"

	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

func init() {
	registerUnwired(commercialVersionClosureWiring{versionClosure: unwiredCommercialVersionClosure{}})
}

// unwiredCommercialVersionClosure 只给装配测试用，答法同 unwired_orchestration.go 里的各占位。
type unwiredCommercialVersionClosure struct{}

func (unwiredCommercialVersionClosure) Cut(
	context.Context,
	commercialapp.CutCommercialVersionAtSuccessorCommand,
) (commercialapp.VersionClosureResult, error) {
	return commercialapp.VersionClosureResult{}, errOrchestrationNotWired
}

func (unwiredCommercialVersionClosure) RetireAtSameStart(
	context.Context,
	commercialapp.RetireCommercialVersionAtSameStartCommand,
) (commercialapp.VersionClosureResult, error) {
	return commercialapp.VersionClosureResult{}, errOrchestrationNotWired
}
