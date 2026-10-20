package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type segmentOperationsWiring struct {
	segmentCloser      tfhttp.SegmentCloser
	dispatchTaskOpener tfhttp.DispatchTaskOpener
	loadAssigner       tfhttp.LoadAssigner
	participationEnder tfhttp.ParticipationEnder
}

func init() {
	registerEndpoints(endpointFeature[segmentOperationsWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-segment-closures":              {method: http.MethodPost, face: decisionFace(accessidentity.DecisionSegmentClosure)},
			"/transport-fulfillment-dispatch-task-registrations":   {method: http.MethodPost, face: decisionFace(accessidentity.DecisionDispatchTaskRegistration)},
			"/transport-fulfillment-load-assignment-registrations": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionLoadAssignment)},
			"/transport-fulfillment-participation-terminations":    {method: http.MethodPost, face: decisionFace(accessidentity.DecisionParticipationTermination)},
		},
		build: func(sources endpointSources) (segmentOperationsWiring, error) {
			segmentOps, err := buildSegmentOperations(sources.db)
			return segmentOperationsWiring{
				segmentCloser:      segmentOps.closer,
				dispatchTaskOpener: segmentOps.opener,
				loadAssigner:       segmentOps.assign,
				participationEnder: segmentOps.enderOf,
			}, err
		},
		mount: func(channels endpointChannels, wiring segmentOperationsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// TF 四个 admin 写面（ADR-0085，票 tf-segment-lifecycle-closure/07）：关段、建派送任务、装载分配、
				// 明确终止参与。它们是运营决定不是承运方回传口，所以路径取读面册名前缀 `transport-fulfillment-`
				// 而不是控制事实那组的 `/transport-fulfillment/...`。四口都挂操作者渠道的「运营决定」能力面（ADR-0151），每口按自己的
				// 决定种类认证；写开关换不了这几行（ADR-0150 决定三）。
				// 终止口只能铸终止那一路（tfhttp.ParticipationTermination 比应用命令窄），交付与交接两路是内部触发。
				{Pattern: "/transport-fulfillment-segment-closures", Handler: tfhttp.NewCloseFulfillmentSegmentEndpoint(channels.operatorDecisions.transport, wiring.segmentCloser)},
				{Pattern: "/transport-fulfillment-dispatch-task-registrations", Handler: tfhttp.NewOpenDispatchTaskEndpoint(channels.operatorDecisions.transport, wiring.dispatchTaskOpener)},
				{Pattern: "/transport-fulfillment-load-assignment-registrations", Handler: tfhttp.NewFormLoadAssignmentEndpoint(channels.operatorDecisions.transport, wiring.loadAssigner)},
				{Pattern: "/transport-fulfillment-participation-terminations", Handler: tfhttp.NewTerminateFulfillmentParticipationEndpoint(channels.operatorDecisions.transport, wiring.participationEnder)},
			}
		},
	})
}
