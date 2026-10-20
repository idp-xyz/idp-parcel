package main

import (
	"net/http"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type pricingRegistrationWiring struct {
	priceCardRegistration          pricinghttp.PriceCardRegistrar
	referenceSeriesRegistration    pricinghttp.ReferenceSeriesRegistrar
	referenceSeriesReview          pricinghttp.ReferenceSeriesReviewer
	referenceSeriesPreview         pricinghttp.ReferenceSeriesPreviewer
	referenceCatalogueRegistration pricinghttp.ReferenceCatalogueRegistrar
}

func init() {
	registerEndpoints(endpointFeature[pricingRegistrationWiring]{
		lines: map[string]endpointLine{
			"/pricing-price-card-registrations":          {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-reference-series-registrations":    {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-reference-series-reviews":          {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-reference-series-previews":         {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-reference-catalogue-registrations": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (pricingRegistrationWiring, error) {
			none := pricingRegistrationWiring{}
			// 价卡与序列登记写面编排（ADR-0085，票 admin-write-faces/01）：接真不等墙降——
			// 未配置 Intake 拒在编排之前，墙降那笔工作在装配点换的只是 Intake。
			priceCardRegistration, err := buildPriceCardRegistrationOrchestration(sources.db)
			if err != nil {
				return none, err
			}
			referenceSeriesRegistration, err := buildReferenceSeriesRegistrationOrchestration(sources.db)
			if err != nil {
				return none, err
			}
			referenceSeriesReview, err := buildReferenceSeriesReviewOrchestration(sources.db)
			if err != nil {
				return none, err
			}
			referenceSeriesPreview, err := buildReferenceSeriesPreviewOrchestration(sources.db)
			if err != nil {
				return none, err
			}
			referenceCatalogueRegistration, err := buildReferenceCatalogueRegistrationOrchestration(sources.db)
			if err != nil {
				return none, err
			}
			return pricingRegistrationWiring{
				priceCardRegistration:          priceCardRegistration,
				referenceSeriesRegistration:    referenceSeriesRegistration,
				referenceSeriesReview:          referenceSeriesReview,
				referenceSeriesPreview:         referenceSeriesPreview,
				referenceCatalogueRegistration: referenceCatalogueRegistration,
			}, nil
		},
		mount: func(channels endpointChannels, wiring pricingRegistrationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 价卡与序列登记写面（ADR-0085，票 admin-write-faces/01）：登记是命令行，挂操作者
				// 渠道的登记册 Intake（ADR-0100 决定四，票 operator-channel/04）——写准入不另立形，查阅行的 Intake 变量换不了
				// 写行。两类登记各立端点，
				// 与其 CLI 命令一一对应（裁决在端点构造函数注释）；登记 CLI 保留为受控批量口，
				// 两口消费同一登记用例，答案代数一致。
				{Pattern: "/pricing-price-card-registrations", Handler: pricinghttp.NewRegisterPriceCardEndpoint(channels.operatorRegistries.pricing, wiring.priceCardRegistration)},
				{Pattern: "/pricing-reference-series-registrations", Handler: pricinghttp.NewRegisterReferenceSeriesEndpoint(channels.operatorRegistries.pricing, wiring.referenceSeriesRegistration)},
				// 序列版本复核（ADR-0099 决定二，票 pricing-reference-series-operations/04）：治理
				// 动作也是命令行，挂操作者渠道的登记册 Intake。**这一口比两个登记口更不能松**
				// ——复核责任方是四眼门的一半，任何采信自报身份的 Intake 都等于把那道门拆了。
				{Pattern: "/pricing-reference-series-reviews", Handler: pricinghttp.NewReviewReferenceSeriesEndpoint(channels.operatorRegistries.pricing, wiring.referenceSeriesReview)},
				// 序列登记前预览（票 pricing-reference-series-operations/08，ADR-0101 决定四）：不写库，
				// 但拟登本体要信封里的租户与登记责任方才立得住，等的与登记口是同一样东西，所以同挂
				// 操作者渠道的登记册 Intake，不走查阅行的 Intake 变量。
				{Pattern: "/pricing-reference-series-previews", Handler: pricinghttp.NewPreviewReferenceSeriesEndpoint(channels.operatorRegistries.pricing, wiring.referenceSeriesPreview)},
				// 计价参考目录登记写面（ADR-0109 Decision 二，票 price-card-shape-gaps/01）：模板导入的命令行，
				// 挂操作者渠道的登记册 Intake，判据同两个登记口。
				{Pattern: "/pricing-reference-catalogue-registrations", Handler: pricinghttp.NewRegisterReferenceCatalogueEndpoint(channels.operatorRegistries.pricing, wiring.referenceCatalogueRegistration)},
			}
		},
	})
}
