package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type cancellationOrderConflictDecisionWiring struct {
	decision shipmenthttp.CancellationOrderConflictDecisionHandler
}

func init() {
	registerEndpoints(endpointFeature[cancellationOrderConflictDecisionWiring]{
		lines: map[string]endpointLine{
			"/cancellation-order-conflict-decisions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionCancellationOrderConflictResolution)},
		},
		build: func(sources endpointSources) (cancellationOrderConflictDecisionWiring, error) {
			decision, err := buildCancellationOrderConflictDecision(sources.db)
			return cancellationOrderConflictDecisionWiring{decision: decision}, err
		},
		mount: func(channels endpointChannels, wiring cancellationOrderConflictDecisionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 取消先后冲突解开（ADR-0295 决定一至四）：有权主体对冲突册上一份冲突决定「认定取消依据不成立」或「认定边界事实有误、交来源方更正」；
				// 前者同笔作废依据这份取消的终局不采用记录并交续办信封。运营侧写行，挂操作者渠道「运营决定」能力面的「取消先后冲突解开」一种，与授权处置、
				// 释放拒收处理的授予分开。
				{Pattern: "/cancellation-order-conflict-decisions", Handler: shipmenthttp.NewDecideCancellationOrderConflictEndpoint(channels.operatorDecisions.shipment, wiring.decision)},
			}
		},
	})
}
