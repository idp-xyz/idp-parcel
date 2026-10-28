package main

import (
	"context"
	"fmt"
	"net/http"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type technicalReceiptsWiring struct {
	receipts customshttp.TechnicalReceiptHandler
}

func init() {
	registerEndpoints(endpointFeature[technicalReceiptsWiring]{
		lines: map[string]endpointLine{
			// 认证复用外部结果那一种授予（ADR-0342 决定四）。
			"/customs/technical-receipts": {method: http.MethodPost, face: integrationClientFace(accessidentity.FactCustomsExternalResult)},
		},
		build: func(sources endpointSources) (technicalReceiptsWiring, error) {
			receipts, err := buildTechnicalReceiptsOrchestration(sources.db)
			return technicalReceiptsWiring{receipts: receipts}, err
		},
		mount: func(channels endpointChannels, wiring technicalReceiptsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/customs/technical-receipts", Handler: customshttp.NewReceiveTechnicalReceiptEndpoint(channels.integrationClients.customs, wiring.receipts)},
			}
		},
	})
}

func buildTechnicalReceiptsOrchestration(db *bentopg.DB) (customshttp.TechnicalReceiptHandler, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel-api: technical receipts: db is nil")
	}
	submissions, err := ccpostgres.NewDeclarationSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: declaration submissions for receipts: %w", err)
	}
	sends, err := ccpostgres.NewSubmissionSends(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: submission sends: %w", err)
	}
	handler, err := customsapp.NewReceiveTechnicalReceiptHandler(customsapp.ReceiveTechnicalReceiptDeps{
		Submissions: submissions, SendFacts: sends, Receipts: sends, Clock: systemClock{},
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: receive technical receipt: %w", err)
	}
	return transactionalReceipts{transactor: db.Transactor(), inner: handler}, nil
}

// transactionalReceipts 给回执接收一笔环境事务：回执行与读回的当前判断看同一份快照。
type transactionalReceipts struct {
	transactor bentoapp.Transactor
	inner      *customsapp.ReceiveTechnicalReceiptHandler
}

func (receipts transactionalReceipts) Handle(
	ctx context.Context, command customsapp.ReceiveTechnicalReceiptCommand,
) (customsapp.TechnicalReceiptResult, error) {
	var result customsapp.TechnicalReceiptResult
	err := receipts.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := receipts.inner.Handle(txCtx, command)
		result = handled
		return handleErr
	})
	if err != nil {
		return customsapp.TechnicalReceiptResult{}, err
	}
	return result, nil
}
