package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type labelTransactionsWiring struct {
	labelTransactions shipmenthttp.LabelTransactionsReader
}

func init() {
	registerEndpoints(endpointFeature[labelTransactionsWiring]{
		lines: map[string]endpointLine{
			"/label-transactions": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (labelTransactionsWiring, error) {
			// 面单交易查阅读的是自己那张表（票 admin-skeleton-closure-batch/08）：与委托查阅
			// 不同表、不同作用域维度（只有租户），因此另立适配器而不是往委托查阅那只补方法。
			labelTransactions, err := pspostgres.NewLabelTransactionViews(sources.db)
			return labelTransactionsWiring{labelTransactions: labelTransactions}, err
		},
		mount: func(channels endpointChannels, wiring labelTransactionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 面单交易查阅（票 admin-skeleton-closure-batch/08，ADR-0084 决定七）：**另立一种准入形**而不是复用委托查阅那个
				// 变量：面单交易没有账户维可分，收一个必带账户维的作用域等于在类型上声称会按账户过滤而它不会。它挂操作者查阅读
				// （票 operator-channel/35）。渠道墙未降前这一口读到的是空册，那是设计：写入方是渠道适配器，首发不进生产。
				{Pattern: "/label-transactions", Handler: shipmenthttp.NewQueryLabelTransactionsEndpoint(channels.labelTransactions(), wiring.labelTransactions)},
			}
		},
	})
}
