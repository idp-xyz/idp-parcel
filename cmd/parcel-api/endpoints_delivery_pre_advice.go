package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type deliveryPreAdviceWiring struct {
	// customerPreAdvice 缺席即两行字面量未配置（customerPreAdviceIntakes 判 nil）。
	customerPreAdvice  *nodeopshttp.CustomerPreAdviceIntake
	deliveryPreAdvices nodeopshttp.DeliveryPreAdviceHandler
}

func init() {
	registerEndpoints(endpointFeature[deliveryPreAdviceWiring]{
		lines: map[string]endpointLine{
			"/node-operations/delivery-pre-advices": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission), ownIntake: true,
			},
			"/node-operations/delivery-pre-advice-withdrawals": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (deliveryPreAdviceWiring, error) {
			// 送站预报两口的首方 Intake 与编排（ADR-0212 决定二；票 ps-external-mark-relations/09）：铸造器与提交口同一只。
			customerPreAdvice, err := buildCustomerPreAdviceIntake(sources.customerClientMinter)
			if err != nil {
				return deliveryPreAdviceWiring{}, err
			}
			deliveryPreAdvices, err := buildDeliveryPreAdviceOrchestration(sources.db)
			return deliveryPreAdviceWiring{customerPreAdvice: customerPreAdvice, deliveryPreAdvices: deliveryPreAdvices}, err
		},
		mount: func(_ endpointChannels, wiring deliveryPreAdviceWiring) []httpapi.BusinessEndpoint {
			// 送站预报两口挂首方客户渠道的 Intake（ADR-0212 决定二；票 ps-external-mark-relations/09），理由同 endpoints_submission.go 里
			// submissionIntake 那段注释：单独的变量，两个隔离开关都换不了它们。customerPreAdvice 缺席时两行是字面量未配置即拒。
			preAdviceSubmissionIntake, preAdviceWithdrawalIntake := customerPreAdviceIntakes(wiring.customerPreAdvice)
			return []httpapi.BusinessEndpoint{
				// 送站预报的提交口与撤回口（ADR-0212 决定二；UC-NO-006）：节点作业在客户面开的写口，挂首方客户渠道族，一口一项客户面能力。
				// 路径随本上下文命令口的 /node-operations/ 前缀；撤回另立子资源，理由同 /shipment-requests/withdrawals：命令形状与恢复动作
				// 都不同。
				{Pattern: "/node-operations/delivery-pre-advices", Handler: nodeopshttp.NewSubmitDeliveryPreAdviceEndpoint(preAdviceSubmissionIntake, wiring.deliveryPreAdvices)},
				{Pattern: "/node-operations/delivery-pre-advice-withdrawals", Handler: nodeopshttp.NewWithdrawDeliveryPreAdviceEndpoint(preAdviceWithdrawalIntake, wiring.deliveryPreAdvices)},
			}
		},
	})
}
