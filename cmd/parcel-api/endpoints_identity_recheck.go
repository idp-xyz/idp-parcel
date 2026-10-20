package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type identityRecheckWiring struct {
	identityRecheck nodeopshttp.IdentityRecheckHandler
}

func init() {
	registerEndpoints(endpointFeature[identityRecheckWiring]{
		lines: map[string]endpointLine{
			"/node-operations/identity-rechecks": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionPendingIdentityRecheck)},
		},
		build: func(sources endpointSources) (identityRecheckWiring, error) {
			// 人工重核编排（ADR-0199 决定三）：接真不等操作者授予——运营决定 Intake 拒在编排之前，授予与准入范围是租户取值。
			identityRecheck, err := buildIdentityRecheckOrchestration(sources.db)
			return identityRecheckWiring{identityRecheck: identityRecheck}, err
		},
		mount: func(channels endpointChannels, wiring identityRecheckWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 人工重核命令口（ADR-0199 决定三；票 no-reception-source/03）：复核页上指名一件待识别实物按存下的号再问一次 PS。管理台上
				// 作判断的口，挂操作者渠道的「运营决定」能力面（ADR-0151 决定一），决定种类`待识别重核`；隔离开关换不了这一行。
				{Pattern: "/node-operations/identity-rechecks", Handler: nodeopshttp.NewRecheckPendingIdentityEndpoint(channels.operatorDecisions.nodeOperations, wiring.identityRecheck)},
			}
		},
	})
}
