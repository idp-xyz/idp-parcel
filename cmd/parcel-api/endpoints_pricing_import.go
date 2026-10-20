package main

import (
	"net/http"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type pricingImportWiring struct {
	priceCardPreview          pricinghttp.PriceCardImportPreviewer
	priceCardDraftSubmission  pricinghttp.PriceCardDraftSubmitter
	priceCardDrafts           pricinghttp.PriceCardDraftReader
	priceCardDraftApproval    pricinghttp.PriceCardDraftApprover
	priceCardDraftPublication pricinghttp.PriceCardDraftPublisher
}

func init() {
	registerEndpoints(endpointFeature[pricingImportWiring]{
		lines: map[string]endpointLine{
			"/pricing-price-card-previews":           {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-price-card-drafts":             {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-price-card-draft-views":        {method: http.MethodGet, face: registryWriteFace()},
			"/pricing-price-card-draft-approvals":    {method: http.MethodPost, face: registryWriteFace()},
			"/pricing-price-card-draft-publications": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (pricingImportWiring, error) {
			none := pricingImportWiring{}
			// 价卡导入预览编排（ADR-0101）：不碰库，接真不等操作者 Intake，判据同试算。
			priceCardPreview, err := buildPriceCardImportPreview()
			if err != nil {
				return none, err
			}
			// 价卡录入编排与草稿册读口（ADR-0101 决定三）：两口挂操作者渠道的登记册 Intake，判据同预览。
			priceCardDraftSubmission, err := buildPriceCardDraftSubmission(sources.db)
			if err != nil {
				return none, err
			}
			priceCardDrafts, err := pppostgres.NewPriceCardDrafts(sources.db)
			if err != nil {
				return none, err
			}
			// 价卡草稿批准与发布编排（ADR-0101 决定五、六）：两口同挂操作者渠道的登记册 Intake；审批职责规则未登记时批准答`未配置`。
			priceCardDraftApproval, err := buildPriceCardDraftApproval(sources.db)
			if err != nil {
				return none, err
			}
			priceCardDraftPublication, err := buildPriceCardDraftPublication(sources.db)
			if err != nil {
				return none, err
			}
			return pricingImportWiring{
				priceCardPreview:          priceCardPreview,
				priceCardDraftSubmission:  priceCardDraftSubmission,
				priceCardDrafts:           priceCardDrafts,
				priceCardDraftApproval:    priceCardDraftApproval,
				priceCardDraftPublication: priceCardDraftPublication,
			}, nil
		},
		mount: func(channels endpointChannels, wiring pricingImportWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 价卡导入预览（ADR-0101 决定四，票 price-card-import/02）：命令行，挂操作者渠道的登记册 Intake——租户来自认证出的
				// 身份，与序列预览同一个 Intake；不加隔离放行（ADR-0150）。编排只读上传字节，依赖结构上没有仓储。
				{Pattern: "/pricing-price-card-previews", Handler: pricinghttp.NewPreviewPriceCardImportEndpoint(channels.operatorRegistries.pricing, wiring.priceCardPreview)},
				// 价卡草稿录入口与草稿查阅读口（ADR-0101 决定三与 Consequences，票 price-card-import/03）：与预览口同挂操作者渠道的
				// 登记册 Intake——Consequences 写明草稿查阅读口也挂操作者 Intake，所以读口不走查阅行的 Intake 变量，隔离读放行换不了
				// 它。读口另立 `-draft-views` 路径：`/pricing-price-card-drafts` 已是录入口，一个路径只挂一个方法。
				{Pattern: "/pricing-price-card-drafts", Handler: pricinghttp.NewSubmitPriceCardDraftEndpoint(channels.operatorRegistries.pricing, wiring.priceCardDraftSubmission)},
				{Pattern: "/pricing-price-card-draft-views", Handler: pricinghttp.NewQueryPriceCardDraftsEndpoint(channels.operatorRegistries.pricing, wiring.priceCardDrafts)},
				// 价卡草稿批准口与发布口（ADR-0101 决定五，票 price-card-import/04）：同挂操作者渠道的登记册 Intake——批准者主体与
				// 授予集要从信封来（spec 自决第 4 格 2026-09-25 更正：本批各口直接挂这个 Intake）。发布在用例层交既有价卡登记，
				// 不经 `/pricing-price-card-registrations` 那一口。
				{Pattern: "/pricing-price-card-draft-approvals", Handler: pricinghttp.NewApprovePriceCardDraftEndpoint(channels.operatorRegistries.pricing, wiring.priceCardDraftApproval)},
				{Pattern: "/pricing-price-card-draft-publications", Handler: pricinghttp.NewPublishPriceCardDraftEndpoint(channels.operatorRegistries.pricing, wiring.priceCardDraftPublication)},
			}
		},
	})
}
