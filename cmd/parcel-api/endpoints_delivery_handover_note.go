package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type deliveryHandoverNoteWiring struct {
	deliveryHandoverNotes deliveryHandoverNoteFaces
}

func init() {
	registerEndpoints(endpointFeature[deliveryHandoverNoteWiring]{
		lines: map[string]endpointLine{
			"/node-operations/delivery-handover-notes":       {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeNode), ownIntake: true},
			"/node-operations/delivery-handover-assignments": {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeNode), ownIntake: true},
			"/node-operations/delivery-handover-closures":    {method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeNode), ownIntake: true},
			"/node-operations/delivery-handover-confirmations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionDeliveryHandoverConfirmation),
			},
			"/node-operations/delivery-handover-conclusions": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityDeliveryHandoverConfirmation), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (deliveryHandoverNoteWiring, error) {
			// 送站交接单五口的 Intake 与编排（ADR-0212 决定三；票 ps-external-mark-relations/12）：作业事实三口与收寄口同一组核验方、操作者册、
			// 设备册与准入范围，客户结论口用提交口那一只首方铸造器。
			faces, err := buildDeliveryHandoverNoteFaces(sources.db, sources.operatorVerifier, sources.operatorRegistry, sources.deviceRegistry,
				sources.admission, sources.customerClientMinter)
			return deliveryHandoverNoteWiring{deliveryHandoverNotes: faces}, err
		},
		mount: func(channels endpointChannels, wiring deliveryHandoverNoteWiring) []httpapi.BusinessEndpoint {
			deliveryHandoverNotes := wiring.deliveryHandoverNotes
			return []httpapi.BusinessEndpoint{
				// 送站交接单五口（ADR-0212 决定三；UC-NO-006）：开立、归入、关闭是节点当场的作业事实，挂「作业事实登记」能力面、各判自己那一种
				// 事实的准入范围；确认挂「运营决定」能力面的送站交接确认一种；客户的确认或否认挂首方客户渠道族，一口一项客户面能力。写开关换
				// 不了它们（ADR-0150 决定三）。
				{Pattern: "/node-operations/delivery-handover-notes", Handler: nodeopshttp.NewOpenDeliveryHandoverNoteEndpoint(deliveryHandoverNotes.openingIntake(), deliveryHandoverNotes.handler)},
				{Pattern: "/node-operations/delivery-handover-assignments", Handler: nodeopshttp.NewAssignToDeliveryHandoverNoteEndpoint(deliveryHandoverNotes.assignmentIntake(), deliveryHandoverNotes.handler)},
				{Pattern: "/node-operations/delivery-handover-closures", Handler: nodeopshttp.NewCloseDeliveryHandoverNoteEndpoint(deliveryHandoverNotes.closureIntake(), deliveryHandoverNotes.handler)},
				{Pattern: "/node-operations/delivery-handover-confirmations", Handler: nodeopshttp.NewConfirmDeliveryHandoverEndpoint(channels.operatorDecisions.nodeOperations, deliveryHandoverNotes.handler)},
				{Pattern: "/node-operations/delivery-handover-conclusions", Handler: nodeopshttp.NewConcludeDeliveryHandoverEndpoint(deliveryHandoverNotes.conclusionIntake(), deliveryHandoverNotes.handler)},
			}
		},
	})
}
