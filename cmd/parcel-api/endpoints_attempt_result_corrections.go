package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type attemptResultCorrectionsWiring struct {
	pickup   tfhttp.PickupAttemptResultCorrector
	delivery tfhttp.DeliveryAttemptResultCorrector
}

func init() {
	registerEndpoints(endpointFeature[attemptResultCorrectionsWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment/offsite-pickup-attempt-result-corrections": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite),
			},
			"/transport-fulfillment/delivery-attempt-result-corrections": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite),
			},
			"/transport-fulfillment-pickup-attempt-result-confirmations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionPickupAttemptResultConfirmation),
			},
			"/transport-fulfillment-delivery-attempt-result-confirmations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionDeliveryAttemptResultConfirmation),
			},
		},
		build: func(sources endpointSources) (attemptResultCorrectionsWiring, error) {
			pickup, err := buildPickupAttemptResultCorrection(sources.db)
			if err != nil {
				return attemptResultCorrectionsWiring{}, err
			}
			delivery, err := buildDeliveryAttemptResultCorrection(sources.db)
			return attemptResultCorrectionsWiring{pickup: pickup, delivery: delivery}, err
		},
		mount: func(channels endpointChannels, wiring attemptResultCorrectionsWiring) []httpapi.BusinessEndpoint {
			pickupFromDevice := tfhttp.PickupAttemptResultCorrectionIntake(tfhttp.UnconfiguredIntake{})
			deliveryFromDevice := tfhttp.DeliveryAttemptResultCorrectionIntake(tfhttp.UnconfiguredIntake{})
			if channels.workFactTransport != nil {
				pickupFromDevice = channels.workFactTransport
				deliveryFromDevice = channels.workFactTransport
			}
			return []httpapi.BusinessEndpoint{
				// 待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果（票 docs-consistency/code/90，ADR-0297）：揽收一口、派送一口，
				// 每口两个接入面。执行方后补挂作业事实面（更正随原事实的族，ADR-0151 决定四）；复核人核对来源后认定挂运营决定面，揽收与
				// 派送各一种决定，与别的决定分开授予。两面同一份应用编排、同一个端点实现，只有 Intake 不同；写开关换不了这几行
				// （ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment/offsite-pickup-attempt-result-corrections", Handler: tfhttp.NewCorrectPickupAttemptResultEndpoint(pickupFromDevice, wiring.pickup)},
				{Pattern: "/transport-fulfillment/delivery-attempt-result-corrections", Handler: tfhttp.NewCorrectDeliveryAttemptResultEndpoint(deliveryFromDevice, wiring.delivery)},
				{Pattern: "/transport-fulfillment-pickup-attempt-result-confirmations", Handler: tfhttp.NewCorrectPickupAttemptResultEndpoint(channels.operatorDecisions.transport, wiring.pickup)},
				{Pattern: "/transport-fulfillment-delivery-attempt-result-confirmations", Handler: tfhttp.NewCorrectDeliveryAttemptResultEndpoint(channels.operatorDecisions.transport, wiring.delivery)},
			}
		},
	})
}
