package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type postIntakeDispositionWiring struct {
	decision shipmenthttp.PostIntakeDispositionDecisionHandler
	requests shipmenthttp.PostIntakeDispositionRequestReader
}

func init() {
	registerEndpoints(endpointFeature[postIntakeDispositionWiring]{
		lines: map[string]endpointLine{
			"/post-intake-disposition-decisions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionPostIntakeServiceDisposition)},
			"/post-intake-disposition-requests":  {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (postIntakeDispositionWiring, error) {
			decision, err := buildPostIntakeDisposition(sources.db)
			if err != nil {
				return postIntakeDispositionWiring{}, err
			}
			requests, err := pspostgres.NewPostIntakeDispositionRequests(sources.db)
			return postIntakeDispositionWiring{decision: decision, requests: requests}, err
		},
		mount: func(channels endpointChannels, wiring postIntakeDispositionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 收寄后服务处置决定（ADR-0332）：有权主体对一件越过取消边界的包裹形成继续、拦截、改送、退运或服务终止的决定，只追加、同笔向各对象所有者
				// 交范围化请求信封。挂操作者渠道「运营决定」能力面的「收寄后服务处置」一种，与授权处置、取消先后冲突解开的授予分开。
				{Pattern: "/post-intake-disposition-decisions", Handler: shipmenthttp.NewDecidePostIntakeDispositionEndpoint(channels.operatorDecisions.shipment, wiring.decision)},
				// 等回应的停止请求查阅：运营查阅，授权边界是租户，与冲突册等口共用那一只操作者查阅 Intake；查阅面里没有动作。
				{Pattern: "/post-intake-disposition-requests", Handler: shipmenthttp.NewQueryPostIntakeDispositionRequestsEndpoint(channels.postIntakeDispositionRequests(), wiring.requests)},
			}
		},
	})
}

func (channels endpointChannels) postIntakeDispositionRequests() shipmenthttp.PostIntakeDispositionRequestQueryIntake {
	if channels.operatorRegistries.shipmentOperationsRead != nil {
		return channels.operatorRegistries.shipmentOperationsRead
	}
	return shipmenthttp.UnconfiguredIntake{}
}
