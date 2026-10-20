package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type controlFactsWiring struct {
	handover           tfhttp.HandoverHandler
	pickupRegistration tfhttp.PickupRegistrationHandler
	pickupCorrection   tfhttp.PickupCorrectionHandler
	pickupAttempt      tfhttp.PickupAttemptHandler
}

func init() {
	registerEndpoints(endpointFeature[controlFactsWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment/handovers":                  {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
			"/transport-fulfillment/handover-corrections":       {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
			"/transport-fulfillment/offsite-pickups":            {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
			"/transport-fulfillment/offsite-pickup-corrections": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
			"/transport-fulfillment/offsite-pickup-attempts":    {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeOffsite)},
		},
		build: func(sources endpointSources) (controlFactsWiring, error) {
			controlFacts, err := buildControlFactOrchestrations(sources.db)
			if err != nil {
				return controlFactsWiring{}, err
			}
			pickupCorrection, err := buildOffsitePickupCorrectionOrchestration(sources.db)
			return controlFactsWiring{
				handover:           controlFacts.handover,
				pickupRegistration: controlFacts.pickupRegistration,
				pickupCorrection:   pickupCorrection,
				pickupAttempt:      controlFacts.pickupAttempt,
			}, err
		},
		mount: func(channels endpointChannels, wiring controlFactsWiring) []httpapi.BusinessEndpoint {
			pickupRegistrationIntake := tfhttp.PickupRegistrationIntake(tfhttp.UnconfiguredIntake{})
			pickupAttemptIntake := tfhttp.PickupAttemptIntake(tfhttp.UnconfiguredIntake{})
			pickupCorrectionIntake := tfhttp.PickupCorrectionIntake(tfhttp.UnconfiguredIntake{})
			handoverRegistrationIntake := tfhttp.HandoverRegistrationIntake(tfhttp.UnconfiguredIntake{})
			handoverCorrectionIntake := tfhttp.HandoverCorrectionIntake(tfhttp.UnconfiguredIntake{})
			if channels.workFactTransport != nil {
				pickupRegistrationIntake = channels.workFactTransport
				pickupAttemptIntake = channels.workFactTransport
				pickupCorrectionIntake = channels.workFactTransport
				handoverRegistrationIntake = channels.workFactTransport
				handoverCorrectionIntake = channels.workFactTransport
			}
			return []httpapi.BusinessEndpoint{
				// 控制事实入口（票 tf-segment-lifecycle-closure/04）：交接一组（登记 + 更正）、揽收一组
				// （单对象登记 + 更正 + 多对象执行），按事实分组而不按 UC 分。它们是 CONTEXT 成立边界的来源事实，
				// 进段那道门（enterFulfillmentSegment）在生产上只从登记那几行走得到——接上之前它没有任何路。
				// 命令面，命令里带着段引用，这几行比交付更不能让隔离读开关换值：一条穿过去的请求会在段登记册
				// 上立出一个来源不明的实际履约段。登记、执行与更正各行都挂作业事实渠道（首登 ADR-0149 决定二；更正随原事实，
				// ADR-0151 决定四），写开关换不了这几行（ADR-0150 决定三）。
				// 揽收更正口（票 tf-segment-lifecycle-closure/08）落新版本回指前版、重交 PS 采认；它接的是自己那一格装配
				// （buildOffsitePickupCorrectionOrchestration），与首登共用一册。
				{Pattern: "/transport-fulfillment/handovers", Handler: tfhttp.NewRegisterTransportHandoverEndpoint(handoverRegistrationIntake, wiring.handover)},
				{Pattern: "/transport-fulfillment/handover-corrections", Handler: tfhttp.NewCorrectTransportHandoverEndpoint(handoverCorrectionIntake, wiring.handover)},
				{Pattern: "/transport-fulfillment/offsite-pickups", Handler: tfhttp.NewRegisterOffsitePickupEndpoint(pickupRegistrationIntake, wiring.pickupRegistration)},
				{Pattern: "/transport-fulfillment/offsite-pickup-corrections", Handler: tfhttp.NewCorrectOffsitePickupEndpoint(pickupCorrectionIntake, wiring.pickupCorrection)},
				{Pattern: "/transport-fulfillment/offsite-pickup-attempts", Handler: tfhttp.NewPerformOffsitePickupEndpoint(pickupAttemptIntake, wiring.pickupAttempt)},
			}
		},
	})
}
