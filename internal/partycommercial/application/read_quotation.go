package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 发出记录的读面只报发出事实。送达不在这里记，也不派生待答复、已过期或已接受。
// 再导出按记录上的摘要重新投影，不插行。

const quotationDeliveryNote = "送达不在这里记。"

// QueryQuotationSendsHandler 列出或读取一条发出记录。
type QueryQuotationSendsHandler struct {
	store ports.QuotationSendStore
}

func NewQueryQuotationSendsHandler(store ports.QuotationSendStore) (*QueryQuotationSendsHandler, error) {
	if store == nil {
		return nil, fmt.Errorf("party commercial: quotation read needs the send store")
	}
	return &QueryQuotationSendsHandler{store: store}, nil
}

// QuotationSendList 是一页发出事实。
type QuotationSendList struct {
	Sends        []domain.QuotationSend
	Next         ports.QuotationSendCursor
	HasNext      bool
	DeliveryNote string
}

func (handler *QueryQuotationSendsHandler) List(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	cursor ports.QuotationSendCursor,
) (QuotationSendList, error) {
	if tenant.String() == "" || limit <= 0 || limit > 100 {
		return QuotationSendList{}, fmt.Errorf("party commercial: quotation list needs a tenant and a limit from 1 to 100")
	}
	page, err := handler.store.ListQuotationSends(ctx, tenant, limit, cursor)
	if err != nil {
		return QuotationSendList{}, err
	}
	return QuotationSendList{Sends: page.Sends, Next: page.Next, HasNext: page.HasNext, DeliveryNote: quotationDeliveryNote}, nil
}

// QuotationSendDetail 是一条发出事实。Found 为假时没有 Send。
type QuotationSendDetail struct {
	Found        bool
	Send         domain.QuotationSend
	DeliveryNote string
}

func (handler *QueryQuotationSendsHandler) Get(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.QuotationID,
) (QuotationSendDetail, error) {
	if tenant.String() == "" || id.String() == "" {
		return QuotationSendDetail{}, fmt.Errorf("party commercial: quotation detail needs a tenant and an id")
	}
	send, found, err := handler.store.GetQuotationSend(ctx, tenant, id)
	if err != nil {
		return QuotationSendDetail{}, err
	}
	if !found {
		return QuotationSendDetail{DeliveryNote: quotationDeliveryNote}, nil
	}
	return QuotationSendDetail{Found: true, Send: send, DeliveryNote: quotationDeliveryNote}, nil
}

// ExportQuotationOutcome 是再导出的结论。READY 才有文件。任何一格都不插行。
type ExportQuotationOutcome uint8

const (
	ExportQuotationOutcomeInvalid ExportQuotationOutcome = iota
	QuotationExportReady
	QuotationExportNotFound
	QuotationExportContentChanged
	QuotationExportNotRepresentable
)

func (outcome ExportQuotationOutcome) String() string {
	switch outcome {
	case QuotationExportReady:
		return "READY"
	case QuotationExportNotFound:
		return "NOT_FOUND"
	case QuotationExportContentChanged:
		return "CONTENT_CHANGED"
	case QuotationExportNotRepresentable:
		return "NOT_REPRESENTABLE"
	default:
		return ""
	}
}

// ExportQuotationResult 是再导出的文件或业务结论。
type ExportQuotationResult struct {
	Outcome    ExportQuotationOutcome
	Conclusion string
	Body       []byte
	MediaType  string
	FileName   string
}

// ExportQuotationHandler 按发出记录上的摘要重新投影对客价格表。
type ExportQuotationHandler struct {
	store  ports.QuotationSendStore
	prices ports.QuotedPriceReader
}

func NewExportQuotationHandler(store ports.QuotationSendStore, prices ports.QuotedPriceReader) (*ExportQuotationHandler, error) {
	if store == nil || prices == nil {
		return nil, fmt.Errorf("party commercial: quotation export needs the send store and the price reader")
	}
	return &ExportQuotationHandler{store: store, prices: prices}, nil
}

func (handler *ExportQuotationHandler) Handle(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.QuotationID,
) (ExportQuotationResult, error) {
	var none ExportQuotationResult
	if tenant.String() == "" || id.String() == "" {
		return none, fmt.Errorf("party commercial: quotation export needs a tenant and an id")
	}
	send, found, err := handler.store.GetQuotationSend(ctx, tenant, id)
	if err != nil {
		return none, err
	}
	if !found {
		return ExportQuotationResult{Outcome: QuotationExportNotFound, Conclusion: "没有这一条发出记录"}, nil
	}
	fact, err := handler.prices.ReadQuotedPrice(ctx, ports.QuotedPriceQuery{
		Tenant: tenant, Lane: send.Lane(), PlanID: send.PlanID(), PlanVersion: send.PlanVersion(),
	})
	if err != nil {
		return none, err
	}
	if !fact.Found || fact.ContentDigest != send.ContentDigest() || fact.Canonicalization != send.Canonicalization() {
		return ExportQuotationResult{Outcome: QuotationExportContentChanged, Conclusion: "所引价格内容已变，不能按发出记录上的摘要再导出"}, nil
	}
	if fact.GapWhere != "" || len(fact.Workbook) == 0 {
		conclusion := fact.GapConclusion
		if conclusion == "" {
			conclusion = "这份价格内容表达不成对客价格表"
		}
		return ExportQuotationResult{Outcome: QuotationExportNotRepresentable, Conclusion: conclusion}, nil
	}
	file, fileName, err := customerRateSheet(send, fact.Workbook)
	if err != nil {
		return none, err
	}
	media := fact.MediaType
	if media == "" {
		media = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	return ExportQuotationResult{
		Outcome: QuotationExportReady, Conclusion: quotationDeliveryNote,
		Body: file, MediaType: media, FileName: fileName,
	}, nil
}
