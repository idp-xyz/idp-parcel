package commercialhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 客户时刻空着（字段不在或空串）交给应用层，答未受理，不落行。
// 写了但不是 RFC3339 的仍是坏报文。

func TestMissingCustomerReplyTimeIsNotAccepted(t *testing.T) {
	intake := publicationOperatorIntake(t, envelopeOperators{})
	book := &quotationReplyHTTPBook{}
	recorder, err := application.NewRecordQuotationReplyHandler(book, book, book)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := commercialhttp.NewRecordQuotationReplyEndpoint(intake, recorder, func() time.Time {
		return time.Date(2026, 11, 23, 5, 0, 0, 0, time.UTC)
	})
	base := `{"quotationId":"QTS-1","decision":"ACCEPTED","evidenceIndex":"SYN-EVIDENCE-01","contentDigest":"digest-1"`
	for _, item := range []struct {
		name string
		body string
	}{
		{name: "字段不在", body: base + `}`},
		{name: "空字符串", body: base + `,"repliedAt":""}`},
	} {
		t.Run(item.name, func(t *testing.T) {
			response := serveOperator(endpoint, http.MethodPost, "/", "SYN-OPERATOR-01", item.body)
			var answer struct {
				Outcome    string `json:"outcome"`
				Conclusion string `json:"conclusion"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
				t.Fatalf("应答 %d 不是 JSON：%s", response.Code, response.Body.Bytes())
			}
			if response.Code != http.StatusOK || answer.Outcome != application.QuotationReplyNotAccepted.String() {
				t.Fatalf("应答 = %d %s %s，想要 200 NOT_ACCEPTED", response.Code, answer.Outcome, answer.Conclusion)
			}
		})
	}
	if book.appends != 0 {
		t.Fatalf("空着的客户时刻落了 %d 行", book.appends)
	}

	malformed := serveOperator(endpoint, http.MethodPost, "/", "SYN-OPERATOR-01", base+`,"repliedAt":"昨天"}`)
	var problem struct {
		Error struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	if err := json.Unmarshal(malformed.Body.Bytes(), &problem); err != nil {
		t.Fatalf("坏报文应答不是 JSON：%s", malformed.Body.Bytes())
	}
	if malformed.Code != http.StatusBadRequest || problem.Error.Code != "MALFORMED_REQUEST" {
		t.Fatalf("非 RFC3339 = %d %s，想要 400 坏报文", malformed.Code, problem.Error.Code)
	}
	if book.appends != 0 {
		t.Fatalf("坏报文落了 %d 行", book.appends)
	}

	parsed := serveOperator(endpoint, http.MethodPost, "/", "SYN-OPERATOR-01", base+`,"repliedAt":"2026-11-23T04:00:00Z"}`)
	var handed struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(parsed.Body.Bytes(), &handed); err != nil {
		t.Fatal(err)
	}
	if parsed.Code != http.StatusOK || handed.Outcome != application.QuotationReplyNotFound.String() || book.appends != 0 {
		t.Fatalf("合法时刻 = %d %s，行 %d，想要交给应用层且不落行", parsed.Code, handed.Outcome, book.appends)
	}
}

type quotationReplyHTTPBook struct {
	appends int
}

func (book *quotationReplyHTTPBook) AppendQuotationReply(context.Context, domain.QuotationReply) (ports.QuotationReplyAppendOutcome, error) {
	book.appends++
	return ports.QuotationReplyAppended, nil
}

func (book *quotationReplyHTTPBook) GetQuotationReply(context.Context, domain.TenantID, domain.QuotationID) (domain.QuotationReply, bool, error) {
	return domain.QuotationReply{}, false, nil
}

func (book *quotationReplyHTTPBook) AppendQuotationSend(context.Context, domain.QuotationSend) (ports.QuotationAppendOutcome, error) {
	return ports.QuotationAppended, nil
}

func (book *quotationReplyHTTPBook) GetQuotationSend(context.Context, domain.TenantID, domain.QuotationID) (domain.QuotationSend, bool, error) {
	return domain.QuotationSend{}, false, nil
}

func (book *quotationReplyHTTPBook) ListQuotationSends(context.Context, domain.TenantID, int, ports.QuotationSendCursor) (ports.QuotationSendPage, error) {
	return ports.QuotationSendPage{}, nil
}

func (book *quotationReplyHTTPBook) ReadQuotedPrice(context.Context, ports.QuotedPriceQuery) (ports.QuotedPriceFact, error) {
	return ports.QuotedPriceFact{}, nil
}
