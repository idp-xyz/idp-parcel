package main

import (
	"context"
	"time"

	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

func init() {
	registerUnwired(quotationSendWiring{
		send:    unwiredQuotationSend{},
		query:   unwiredQuotationSendQuery{},
		reading: unwiredQuotationReading{},
		reply:   unwiredQuotationReply{},
		export:  unwiredQuotationExport{},
	})
}

type unwiredQuotationSend struct{}

func (unwiredQuotationSend) Handle(context.Context, commercialapp.SendQuotationCommand) (commercialapp.SendQuotationResult, error) {
	return commercialapp.SendQuotationResult{}, errOrchestrationNotWired
}

type unwiredQuotationSendQuery struct{}

func (unwiredQuotationSendQuery) List(context.Context, domain.TenantID, int, ports.QuotationSendCursor) (commercialapp.QuotationSendList, error) {
	return commercialapp.QuotationSendList{}, errOrchestrationNotWired
}

func (unwiredQuotationSendQuery) Get(context.Context, domain.TenantID, domain.QuotationID) (commercialapp.QuotationSendDetail, error) {
	return commercialapp.QuotationSendDetail{}, errOrchestrationNotWired
}

type unwiredQuotationReply struct{}

func (unwiredQuotationReply) Handle(context.Context, commercialapp.RecordQuotationReplyCommand) (commercialapp.RecordQuotationReplyResult, error) {
	return commercialapp.RecordQuotationReplyResult{}, errOrchestrationNotWired
}

type unwiredQuotationReading struct{}

func (unwiredQuotationReading) Get(context.Context, domain.TenantID, domain.QuotationID, time.Time) (commercialapp.QuotationReadingDetail, error) {
	return commercialapp.QuotationReadingDetail{}, errOrchestrationNotWired
}

type unwiredQuotationExport struct{}

func (unwiredQuotationExport) Handle(context.Context, domain.TenantID, domain.QuotationID) (commercialapp.ExportQuotationResult, error) {
	return commercialapp.ExportQuotationResult{}, errOrchestrationNotWired
}
