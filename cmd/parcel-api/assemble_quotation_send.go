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
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 报价发出的生产装配。写口一笔事务；列表、详情与再导出只读，不另开事务。
// 答复与发出各写一笔，不把两件事放进同一笔。
type quotationSendServices struct {
	send    transactionalQuotationSend
	query   *commercialapp.QueryQuotationSendsHandler
	reading *commercialapp.ReadQuotationHandler
	reply   transactionalQuotationReply
	rule    transactionalQuotationRequirement
	export  *commercialapp.ExportQuotationHandler
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
	replies, err := pcpostgres.NewQuotationReplies(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: quotation reply store: %w", err)
	}
	recorder, err := commercialapp.NewRecordQuotationReplyHandler(sends, replies, prices)
	if err != nil {
		return none, err
	}
	publication, err := pcpostgres.NewQuotationPublication(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: quotation publication view: %w", err)
	}
	bases, err := pricingpostgres.NewPublicationBases(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: publication basis for quotation reading: %w", err)
	}
	registered, err := quotedprice.NewRegisteredPrices(cards, bases)
	if err != nil {
		return none, fmt.Errorf("parcel-api: registered price reading: %w", err)
	}
	reading, err := commercialapp.NewReadQuotationHandler(sends, replies, prices, publication, registered)
	if err != nil {
		return none, err
	}
	return quotationSendServices{
		send:    transactionalQuotationSend{transactor: db.Transactor(), inner: sender},
		query:   query,
		reading: reading,
		reply:   transactionalQuotationReply{transactor: db.Transactor(), inner: recorder},
		rule:    transactionalQuotationRequirement{transactor: db.Transactor(), view: publication},
		export:  export,
	}, nil
}

type transactionalQuotationReply struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.RecordQuotationReplyHandler
}

func (wrapper transactionalQuotationReply) Handle(
	ctx context.Context,
	command commercialapp.RecordQuotationReplyCommand,
) (commercialapp.RecordQuotationReplyResult, error) {
	return commercialInTransaction(ctx, wrapper.transactor, func(txCtx context.Context) (commercialapp.RecordQuotationReplyResult, error) {
		return wrapper.inner.Handle(txCtx, command)
	})
}

type transactionalQuotationRequirement struct {
	transactor bentoapp.Transactor
	view       *pcpostgres.QuotationPublication
}

func (wrapper transactionalQuotationRequirement) Handle(ctx context.Context, tenant domain.TenantID, requires bool) (commercialapp.QuotationPublicationRequirementOutcome, error) {
	requirement, err := domain.NewQuotationPublicationRequirement(tenant, requires)
	if err != nil {
		return 0, err
	}
	return commercialInTransaction(ctx, wrapper.transactor, func(txCtx context.Context) (commercialapp.QuotationPublicationRequirementOutcome, error) {
		return commercialapp.RegisterQuotationPublicationRequirement(txCtx, wrapper.view, requirement)
	})
}
