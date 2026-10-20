package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type identityDispositionWiring struct {
	identityDisposition nodeopshttp.IdentityDispositionHandler
}

func init() {
	registerEndpoints(endpointFeature[identityDispositionWiring]{
		lines: map[string]endpointLine{
			"/node-operations/identity-dispositions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionIdentityConflictDisposition)},
		},
		build: func(sources endpointSources) (identityDispositionWiring, error) {
			// 身份冲突处置编排（UC-NO-004）：同人工重核，接真不等授予——谁持有「身份冲突处置」授予是租户取值，生产装配不配任何默认。
			identityDisposition, err := buildIdentityDispositionOrchestration(sources.db)
			return identityDispositionWiring{identityDisposition: identityDisposition}, err
		},
		mount: func(channels endpointChannels, wiring identityDispositionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 身份冲突处置命令口（UC-NO-004；ADR-0210 决定六）：复核页上对一件判身份冲突的作业实物指定一件候选或确认都不是。挂操作者渠道
				// 的「运营决定」能力面，决定种类`身份冲突处置`；隔离开关换不了这一行。
				{Pattern: "/node-operations/identity-dispositions", Handler: nodeopshttp.NewDisposeIdentityConflictEndpoint(channels.operatorDecisions.nodeOperations, wiring.identityDisposition)},
			}
		},
	})
}
