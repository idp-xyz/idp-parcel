package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type movementFactWiring struct {
	movementFact tfhttp.MovementFactHandler
}

func init() {
	registerEndpoints(endpointFeature[movementFactWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment/movement-facts": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
		},
		build: func(sources endpointSources) (movementFactWiring, error) {
			movementFact, err := buildMovementFactOrchestration(sources.db)
			return movementFactWiring{movementFact: movementFact}, err
		},
		mount: func(channels endpointChannels, wiring movementFactWiring) []httpapi.BusinessEndpoint {
			movementFactIntake := tfhttp.MovementFactIntake(tfhttp.UnconfiguredIntake{})
			if channels.workFactTransport != nil {
				movementFactIntake = channels.workFactTransport
			}
			return []httpapi.BusinessEndpoint{
				// 移动事实口（票 tf-segment-lifecycle-closure/05）：只收自营执行方的出发 / 移动 / 到达；外部承运
				// 轨迹**不从这里进**，走 TrackingSource 入站口的采纳执行器（label-channel/16 已落）。谁是自营
				// 执行方由 Intake 的认证结果说：作业事实渠道由这一面说自营（frontlineMovementSource，票 operator-channel/10）。
				{Pattern: "/transport-fulfillment/movement-facts", Handler: tfhttp.NewRecordMovementFactEndpoint(movementFactIntake, wiring.movementFact)},
			}
		},
	})
}
