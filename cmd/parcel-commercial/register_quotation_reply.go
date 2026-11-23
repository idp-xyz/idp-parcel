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

// 演示种子登记一条答复。在线口不收代录人与登记时刻；这一入口给合成租户灌已经取定的那一条。

type quotationReplyFile struct {
	TenantID        string `json:"tenantId"`
	QuotationID     string `json:"quotationId"`
	Decision        string `json:"decision"`
	RepliedAt       string `json:"repliedAt"`
	EvidenceIndex   string `json:"evidenceIndex"`
	RejectionReason string `json:"rejectionReason"`
	ContentDigest   string `json:"contentDigest"`
	Recorder        string `json:"recorder"`
	RecordedAt      string `json:"recordedAt"`
}

func runRegisterQuotationReply(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-quotation-reply", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "报价答复 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-quotation-reply 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读报价答复文件：%v\n", err)
		return exitTechnical
	}
	command, err := quotationReplyCommand(raw)
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
	handler, err := newQuotationReplyHandler(db)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	var result application.RecordQuotationReplyResult
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = handler.Handle(txCtx, command)
		return handleErr
	}); err != nil {
		fmt.Fprintf(errOut, "报价答复：%v\n", err)
		return exitTechnical
	}
	fmt.Fprintf(out, "报价答复 %s：%s %s\n", command.QuotationID, result.Outcome, result.Conclusion)
	switch result.Outcome {
	case application.QuotationReplyRecorded, application.QuotationReplyReplay:
		return exitLanded
	default:
		return exitAttention
	}
}

func quotationReplyCommand(raw []byte) (application.RecordQuotationReplyCommand, error) {
	var none application.RecordQuotationReplyCommand
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document quotationReplyFile
	if err := decoder.Decode(&document); err != nil {
		return none, fmt.Errorf("报价答复文件：%w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return none, fmt.Errorf("报价答复文件：只收一个对象")
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, err
	}
	quotationID, err := domain.NewQuotationID(document.QuotationID)
	if err != nil {
		return none, err
	}
	decision, known := domain.QuotationReplyDecisionNamed(document.Decision)
	if !known {
		return none, fmt.Errorf("decision 要 ACCEPTED 或 REJECTED")
	}
	repliedAt, err := time.Parse(time.RFC3339, document.RepliedAt)
	if err != nil {
		return none, fmt.Errorf("repliedAt 要 RFC3339")
	}
	recorder, err := domain.NewOperatorSubjectReference(document.Recorder)
	if err != nil {
		return none, err
	}
	recordedAt, err := time.Parse(time.RFC3339, document.RecordedAt)
	if err != nil {
		return none, fmt.Errorf("recordedAt 要 RFC3339")
	}
	return application.RecordQuotationReplyCommand{
		Tenant: tenant, QuotationID: quotationID, Decision: decision,
		RepliedAt: repliedAt.UTC(), EvidenceIndex: document.EvidenceIndex, RejectionReason: document.RejectionReason,
		ContentDigest: document.ContentDigest, Recorder: recorder, RecordedAt: recordedAt.UTC(),
	}, nil
}

func newQuotationReplyHandler(db *bentopg.DB) (*application.RecordQuotationReplyHandler, error) {
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
	replies, err := pcpostgres.NewQuotationReplies(db)
	if err != nil {
		return nil, err
	}
	return application.NewRecordQuotationReplyHandler(sends, replies, prices)
}
