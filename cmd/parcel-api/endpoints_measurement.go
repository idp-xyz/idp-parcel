package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type measurementWiring struct {
	handler nodeopshttp.ActualMeasurementHandler
}

func init() {
	registerEndpoints(endpointFeature[measurementWiring]{
		lines: map[string]endpointLine{
			"/node-operations/measurements":                     {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeNode)},
			"/node-operations/measurement-invalidations":        {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeNode)},
			"/node-operations/manual-measurements":              {method: http.MethodPost, face: decisionFace(accessidentity.DecisionManualMeasurementEntry)},
			"/node-operations/manual-measurement-invalidations": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionManualMeasurementEntry)},
		},
		build: func(sources endpointSources) (measurementWiring, error) {
			handler, err := buildActualMeasurementOrchestration(sources.db)
			return measurementWiring{handler: handler}, err
		},
		mount: func(channels endpointChannels, wiring measurementWiring) []httpapi.BusinessEndpoint {
			device := nodeopshttp.MeasurementIntake(nodeopshttp.UnconfiguredIntake{})
			if intake, ok := channels.workFactReception.(*nodeopshttp.WorkFactReceptionIntake); ok && intake != nil {
				device = intake
			}
			manual := nodeopshttp.ManualMeasurementIntake(channels.operatorDecisions.nodeOperations)
			return []httpapi.BusinessEndpoint{
				{Pattern: "/node-operations/measurements", Handler: nodeopshttp.NewRecordMeasurementEndpoint(device, wiring.handler)},
				{Pattern: "/node-operations/measurement-invalidations", Handler: nodeopshttp.NewInvalidateMeasurementEndpoint(device, wiring.handler)},
				{Pattern: "/node-operations/manual-measurements", Handler: nodeopshttp.NewRecordManualMeasurementEndpoint(manual, wiring.handler)},
				{Pattern: "/node-operations/manual-measurement-invalidations", Handler: nodeopshttp.NewInvalidateManualMeasurementEndpoint(manual, wiring.handler)},
			}
		},
	})
}
