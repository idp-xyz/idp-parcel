package main

import (
	"net/http"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 报价发出。写面与参与方登记写面同一张面；列表、详情与再导出挂运营查阅读。
// 四口放在同一个特性里：列表与发出共用路径、只是方法不同，不能拆进两个特性各声明一次。
type quotationSendWiring struct {
	send   commercialhttp.QuotationSender
	query  commercialhttp.QuotationSendReader
	export commercialhttp.QuotationExporter
}

func init() {
	registerEndpoints(endpointFeature[quotationSendWiring]{
		lines: map[string]endpointLine{
			"/commercial-quotation-send-registrations": {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-quotation-sends":              {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-quotation-sends/{quotationId}": {
				method: http.MethodGet, face: operationsReadFace(),
				probe: "/commercial-quotation-sends/QTS-PROBE",
			},
			"/commercial-quotation-sends/{quotationId}/workbook": {
				method: http.MethodGet, face: operationsReadFace(),
				probe: "/commercial-quotation-sends/QTS-PROBE/workbook",
			},
		},
		build: func(sources endpointSources) (quotationSendWiring, error) {
			services, err := buildQuotationSendServices(sources.db)
			if err != nil {
				return quotationSendWiring{}, err
			}
			return quotationSendWiring{send: services.send, query: services.query, export: services.export}, nil
		},
		mount: func(channels endpointChannels, wiring quotationSendWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/commercial-quotation-send-registrations", Handler: commercialhttp.NewSendQuotationEndpoint(channels.operatorRegistries.commercial, wiring.send, time.Now)},
				{Pattern: "/commercial-quotation-sends", Handler: commercialhttp.NewQueryQuotationSendsEndpoint(channels.commercialCatalogue(), wiring.query)},
				{Pattern: "/commercial-quotation-sends/{quotationId}", Handler: commercialhttp.NewQueryQuotationSendEndpoint(channels.commercialCatalogue(), wiring.query)},
				{Pattern: "/commercial-quotation-sends/{quotationId}/workbook", Handler: commercialhttp.NewExportQuotationEndpoint(channels.commercialCatalogue(), wiring.export)},
			}
		},
	})
}
