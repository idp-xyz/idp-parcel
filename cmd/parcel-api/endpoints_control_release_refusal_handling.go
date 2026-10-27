package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type controlReleaseRefusalHandlingWiring struct {
	handling shipmenthttp.ControlReleaseRefusalHandlingHandler
}

func init() {
	registerEndpoints(endpointFeature[controlReleaseRefusalHandlingWiring]{
		lines: map[string]endpointLine{
			"/control-release-refusal-handlings": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionControlReleaseRefusalHandling)},
		},
		build: func(sources endpointSources) (controlReleaseRefusalHandlingWiring, error) {
			handling, err := buildControlReleaseRefusalHandling(sources.db)
			return controlReleaseRefusalHandlingWiring{handling: handling}, err
		},
		mount: func(channels endpointChannels, wiring controlReleaseRefusalHandlingWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 释放拒收处理（ADR-0293 决定二、三）：运维对释放未受理册上一笔被拒收的占用，核对后在结算侧释放，或标记已在别处处理，并同笔让
				// 接受链重驱那份委托的当前版本。运营侧写行，挂操作者渠道「运营决定」能力面的「释放拒收处理」一种，与授权处置的授予分开。
				{Pattern: "/control-release-refusal-handlings", Handler: shipmenthttp.NewHandleControlReleaseRefusalEndpoint(channels.operatorDecisions.shipment, wiring.handling)},
			}
		},
	})
}
