package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type continuedAttemptDecisionWiring struct {
	continuedAttemptDecisions shipmenthttp.ContinuedAttemptDecisionHandler
}

func init() {
	registerEndpoints(endpointFeature[continuedAttemptDecisionWiring]{
		lines: map[string]endpointLine{
			"/shipment-requests/continued-attempt-closures":   {method: http.MethodPost, face: decisionFace(accessidentity.DecisionControlledClosure)},
			"/shipment-requests/continued-attempt-reopenings": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionControlledReopening)},
		},
		build: func(sources endpointSources) (continuedAttemptDecisionWiring, error) {
			decisions, err := buildContinuedAttemptDecisionOrchestration(sources.db)
			return continuedAttemptDecisionWiring{continuedAttemptDecisions: decisions}, err
		},
		mount: func(channels endpointChannels, wiring continuedAttemptDecisionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 受控关闭 / 重开决定两个命令口（票 label-channel/30）：同一只编排两条命令，运营侧写行，挂操作者渠道的「运营决定」
				// 能力面（ADR-0151；票 operator-channel/15）。按包裹寻址、请求方与货主账户取包裹所属已接受委托的客户账户（ADR-0183）；
				// 提交操作者只作操作证据（ADR-0187），决定方与授权角色由授权答复给出。授权请求映射未配置（`PAR-COM-13` / `PAR-COM-14`）时越过
				// Intake 答`未决 · 授权口不可用`。
				{Pattern: "/shipment-requests/continued-attempt-closures", Handler: shipmenthttp.NewFormControlledClosureEndpoint(channels.operatorDecisions.shipment, wiring.continuedAttemptDecisions)},
				{Pattern: "/shipment-requests/continued-attempt-reopenings", Handler: shipmenthttp.NewFormReopeningEndpoint(channels.operatorDecisions.shipment, wiring.continuedAttemptDecisions)},
			}
		},
	})
}
