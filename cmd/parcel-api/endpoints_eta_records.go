package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

// ETA 预测查阅：运营看各预测线当前登记的是哪一版、是人录的还是预测来源送来的。与追踪投影查阅同一个 Intake——都是
// 可视与异常上下文的操作者查阅读；预测对客户可不可见不在这一口答。

type etaRecordsWiring struct {
	predictions visibilityhttp.ETAPredictionReviewReader
}

func init() {
	registerEndpoints(endpointFeature[etaRecordsWiring]{
		lines: map[string]endpointLine{
			"/eta-prediction-records": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (etaRecordsWiring, error) {
			predictions, err := vepostgres.NewETAs(sources.db)
			return etaRecordsWiring{predictions: predictions}, err
		},
		mount: func(channels endpointChannels, wiring etaRecordsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/eta-prediction-records", Handler: visibilityhttp.NewQueryETAPredictionsEndpoint(channels.trackingProjections(), wiring.predictions)},
			}
		},
	})
}
