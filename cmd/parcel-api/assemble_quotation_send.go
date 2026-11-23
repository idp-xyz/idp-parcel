package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricingpostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	quotedprice "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/parcelpricing"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 报价发出的生产装配。写口一笔事务；列表、详情与再导出只读，不另开事务。
type quotationSendServices struct {
	send   transactionalQuotationSend
	query  *commercialapp.QueryQuotationSendsHandler
	export *commercialapp.ExportQuotationHandler
}

type transactionalQuotationSend struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.SendQuotationHandler
}

func (wrapper transactionalQuotationSend) Handle(
	ctx context.Context,
	command commercialapp.SendQuotationCommand,
) (commercialapp.SendQuotationResult, error) {
	return commercialInTransaction(ctx, wrapper.transactor, func(txCtx context.Context) (commercialapp.SendQuotationResult, error) {
		return wrapper.inner.Handle(txCtx, command)
	})
}

func buildQuotationSendServices(db *bentopg.DB) (quotationSendServices, error) {
	var none quotationSendServices
	drafts, err := pricingpostgres.NewPriceCardDrafts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: price card drafts for quotation send: %w", err)
	}
	cards, err := pricingpostgres.NewPriceCards(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: price cards for quotation send: %w", err)
	}
	prices, err := quotedprice.NewQuotedPrices(drafts, cards, pricecardtemplate.Reader{})
	if err != nil {
		return none, fmt.Errorf("parcel-api: quoted price reader: %w", err)
	}
	sends, err := pcpostgres.NewQuotationSends(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: quotation send store: %w", err)
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: party identities for quotation send: %w", err)
	}
	sender, err := commercialapp.NewSendQuotationHandler(prices, identities, sends)
	if err != nil {
		return none, err
	}
	query, err := commercialapp.NewQueryQuotationSendsHandler(sends)
	if err != nil {
		return none, err
	}
	export, err := commercialapp.NewExportQuotationHandler(sends, prices)
	if err != nil {
		return none, err
	}
	return quotationSendServices{
		send:   transactionalQuotationSend{transactor: db.Transactor(), inner: sender},
		query:  query,
		export: export,
	}, nil
}
