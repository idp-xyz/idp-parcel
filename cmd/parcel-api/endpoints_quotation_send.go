package main

import (
	"net/http"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 报价发出与答复。写面与参与方登记写面同一张面；列表、详情与再导出挂运营查阅读。
type quotationSendWiring struct {
	send    commercialhttp.QuotationSender
	query   commercialhttp.QuotationSendReader
	reading commercialhttp.QuotationReading
	reply   commercialhttp.QuotationReplyRecorder
	rule    commercialhttp.QuotationPublicationRequirementRecorder
	export  commercialhttp.QuotationExporter
}

func init() {
	registerEndpoints(endpointFeature[quotationSendWiring]{
		lines: map[string]endpointLine{
			"/commercial-quotation-send-registrations":                    {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-quotation-reply-registrations":                   {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-quotation-publication-requirement-registrations": {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-quotation-sends":                                 {method: http.MethodGet, face: operationsReadFace()},
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
			return quotationSendWiring{
				send: services.send, query: services.query, reading: services.reading,
				reply: services.reply, rule: services.rule, export: services.export,
			}, nil
		},
		mount: func(channels endpointChannels, wiring quotationSendWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/commercial-quotation-send-registrations", Handler: commercialhttp.NewSendQuotationEndpoint(channels.operatorRegistries.commercial, wiring.send, time.Now)},
				{Pattern: "/commercial-quotation-reply-registrations", Handler: commercialhttp.NewRecordQuotationReplyEndpoint(channels.operatorRegistries.commercial, wiring.reply, time.Now)},
				{Pattern: "/commercial-quotation-publication-requirement-registrations", Handler: commercialhttp.NewQuotationPublicationRequirementEndpoint(channels.operatorRegistries.commercial, wiring.rule)},
				{Pattern: "/commercial-quotation-sends", Handler: commercialhttp.NewQueryQuotationSendsEndpoint(channels.commercialCatalogue(), wiring.query)},
				{Pattern: "/commercial-quotation-sends/{quotationId}", Handler: commercialhttp.NewQueryQuotationSendEndpoint(channels.commercialCatalogue(), wiring.reading, time.Now)},
				{Pattern: "/commercial-quotation-sends/{quotationId}/workbook", Handler: commercialhttp.NewExportQuotationEndpoint(channels.commercialCatalogue(), wiring.export)},
			}
		},
	})
}
