package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricingpostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	quotedprice "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/parcelpricing"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 演示种子登记一条发出记录。在线口不收报价号、发出人与时刻；这一入口给合成租户灌已经取定的那一条，
// 文件里带着这三格，重放才对得上同一行。

type quotationSendFile struct {
	QuotationID         string `json:"quotationId"`
	TenantID            string `json:"tenantId"`
	Sender              string `json:"sender"`
	SentAt              string `json:"sentAt"`
	Lane                string `json:"lane"`
	PlanID              string `json:"planId"`
	PlanVersion         string `json:"planVersion"`
	ContentDigest       string `json:"contentDigest"`
	CounterpartyPartyID string `json:"counterpartyPartyId"`
	LegalEntityID       string `json:"legalEntityId"`
	ValidFrom           string `json:"validFrom"`
	ValidUntil          string `json:"validUntil"`
	Terms               string `json:"terms"`
}

func runRegisterQuotationSend(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-quotation-send", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "报价发出 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-quotation-send 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读报价发出文件：%v\n", err)
		return exitTechnical
	}
	command, err := quotationSendCommand(raw)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	db, cleanup, err := openDatabase(ctx, getenv)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	defer cleanup()
	handler, err := newQuotationSendHandler(db)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	var result application.SendQuotationResult
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = handler.Handle(txCtx, command)
		return handleErr
	}); err != nil {
		fmt.Fprintf(errOut, "报价发出：%v\n", err)
		return exitTechnical
	}
	fmt.Fprintf(out, "报价发出 %s：%s %s\n", command.ID, result.Outcome, result.Conclusion)
	switch result.Outcome {
	case application.QuotationSent, application.QuotationSendReplay:
		return exitLanded
	default:
		return exitAttention
	}
}

func quotationSendCommand(raw []byte) (application.SendQuotationCommand, error) {
	var none application.SendQuotationCommand
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document quotationSendFile
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("报价发出文件：%w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return none, fmt.Errorf("报价发出文件：只收一个对象")
	}
	id, err := domain.NewQuotationID(document.QuotationID)
	if err != nil {
		return none, err
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	sender, err := domain.NewOperatorSubjectReference(document.Sender)
	if err != nil {
		return none, err
	}
	sentAt, err := time.Parse(time.RFC3339, document.SentAt)
	if err != nil {
		return none, fmt.Errorf("sentAt 要 RFC3339")
	}
	lane, known := domain.QuotationPriceLaneNamed(document.Lane)
	if !known {
		return none, fmt.Errorf("lane 要 APPROVED_DRAFT 或 REGISTERED_PRICE_CARD")
	}
	counterparty, err := domain.NewPartyID(document.CounterpartyPartyID)
	if err != nil {
		return none, err
	}
	legalEntity, err := domain.NewLegalEntityReference(document.LegalEntityID)
	if err != nil {
		return none, err
	}
	startsAt, err := time.Parse(time.RFC3339, document.ValidFrom)
	if err != nil {
		return none, fmt.Errorf("validFrom 要 RFC3339")
	}
	endsAt, err := time.Parse(time.RFC3339, document.ValidUntil)
	if err != nil {
		return none, fmt.Errorf("validUntil 要 RFC3339")
	}
	return application.SendQuotationCommand{
		ID: id, Tenant: tenant, Sender: sender, SentAt: sentAt.UTC(), Lane: lane,
		PlanID: document.PlanID, PlanVersion: document.PlanVersion, ContentDigest: document.ContentDigest,
		Counterparty: counterparty, LegalEntity: legalEntity,
		StartsAt: startsAt.UTC(), EndsAt: endsAt.UTC(), Terms: document.Terms,
	}, nil
}

func newQuotationSendHandler(db *bentopg.DB) (*application.SendQuotationHandler, error) {
	drafts, err := pricingpostgres.NewPriceCardDrafts(db)
	if err != nil {
		return nil, err
	}
	cards, err := pricingpostgres.NewPriceCards(db)
	if err != nil {
		return nil, err
	}
	prices, err := quotedprice.NewQuotedPrices(drafts, cards, pricecardtemplate.Reader{})
	if err != nil {
		return nil, err
	}
	sends, err := pcpostgres.NewQuotationSends(db)
	if err != nil {
		return nil, err
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return nil, err
	}
	return application.NewSendQuotationHandler(prices, identities, sends)
}
