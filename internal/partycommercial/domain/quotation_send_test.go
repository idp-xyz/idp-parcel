package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 发出记录就是报价。它只记下发出那一刻的事实：所引价格、相对方、责任法人、收口的有效期、
// 条款、发出人、时刻与方式。没有状态列，也不派生待答复、已过期或已接受。

func TestQuotationValidityIsClosedAndHalfOpen(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	interval, err := domain.NewQuotationValidity(start, end)
	if err != nil {
		t.Fatal(err)
	}
	if !interval.Contains(start) || interval.Contains(end) || !interval.Contains(end.Add(-time.Nanosecond)) {
		t.Fatal("有效期应起点算在内、终点算在外")
	}
	if _, err := domain.NewQuotationValidity(start, time.Time{}); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("缺终点 = %v，想要未受理", err)
	}
	if _, err := domain.NewQuotationValidity(time.Time{}, end); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("缺起点 = %v，想要未受理", err)
	}
	if _, err := domain.NewQuotationValidity(end, start); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("终点不晚于起点 = %v，想要未受理", err)
	}
	if _, err := domain.NewQuotationValidity(start, start); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("终点等于起点 = %v，想要未受理", err)
	}
}

func TestQuotationTermsTrimToEmptyAndRejectControls(t *testing.T) {
	empty, err := domain.NewQuotationTerms("  \n\t  ")
	if err != nil || empty.Text() != "" {
		t.Fatalf("空白条款 = %q, %v，想要空串", empty.Text(), err)
	}
	kept, err := domain.NewQuotationTerms("  运费不含燃油  ")
	if err != nil || kept.Text() != "运费不含燃油" {
		t.Fatalf("条款 = %q, %v", kept.Text(), err)
	}
	if _, err := domain.NewQuotationTerms("运费\u0000不含"); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("控制字符 = %v，想要未受理", err)
	}
}

func TestAQuotationSendRecordsTheFactAndNotDelivery(t *testing.T) {
	send := quotationSend(t, "  另加条款  ")
	if send.DeliveryRecorded() {
		t.Fatal("发出记录不记送达")
	}
	if send.Method() != domain.QuotationSendByWorkbookExport || send.Method().String() != "WORKBOOK_EXPORT" {
		t.Fatalf("方式 = %s", send.Method())
	}
	if send.Lane() != domain.QuotationPriceFromApprovedDraft || send.Lane().String() != "APPROVED_DRAFT" {
		t.Fatalf("来源 = %s", send.Lane())
	}
	if send.Terms().Text() != "另加条款" || send.ContentDigest() != "digest-1" {
		t.Fatalf("条款或摘要不对：%q %q", send.Terms().Text(), send.ContentDigest())
	}
	if send.Counterparty().String() == "" || send.LegalEntity().String() == "" || send.Sender().String() == "" {
		t.Fatal("相对方、责任法人或发出人缺了")
	}
}

func TestQuotationSendStringNamesEveryNonZeroConstant(t *testing.T) {
	if domain.QuotationPriceFromRegisteredCard.String() != "REGISTERED_PRICE_CARD" {
		t.Fatalf("价卡路 = %s", domain.QuotationPriceFromRegisteredCard)
	}
	if domain.QuotationPriceLaneInvalid.String() != "" || domain.QuotationSendMethodInvalid.String() != "" {
		t.Fatal("零值不该有名字")
	}
}

func TestQuotationSendRejectsAnOpenValidityBorrowedFromElsewhere(t *testing.T) {
	// 商业版本的适用区间允许不写终点。报价的有效期不走那一条。
	open, err := domain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil || !open.Contains(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("对照用的开放区间应当成立")
	}
	if _, err := domain.NewQuotationValidity(open.StartsAt(), time.Time{}); !errors.Is(err, domain.ErrQuotationNotAccepted) {
		t.Fatalf("把开放终点交进报价 = %v", err)
	}
}

func quotationSend(t *testing.T, terms string) domain.QuotationSend {
	t.Helper()
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	send, err := domain.NewQuotationSend(domain.QuotationSendSpec{
		ID:               commercialValue(t, domain.NewQuotationID, "QTS-1"),
		Tenant:           commercialValue(t, domain.NewTenantID, "tenant-1"),
		Lane:             domain.QuotationPriceFromApprovedDraft,
		PlanID:           "PLAN-1",
		PlanVersion:      "v1",
		Canonicalization: "PPC-5",
		ContentDigest:    "digest-1",
		Counterparty:     commercialValue(t, domain.NewPartyID, "party-1"),
		CounterpartyName: commercialValue(t, domain.NewPartyName, "相对方"),
		LegalEntity:      commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		LegalEntityName:  commercialValue(t, domain.NewPartyName, "责任法人"),
		StartsAt:         start,
		EndsAt:           start.Add(24 * time.Hour),
		Terms:            terms,
		Sender:           commercialValue(t, domain.NewOperatorSubjectReference, "op-1"),
		SentAt:           start.Add(time.Hour),
		Method:           domain.QuotationSendByWorkbookExport,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.IndexFunc(send.Terms().Text(), unicode.IsControl) >= 0 {
		t.Fatal("落下来的条款不该再含控制字符")
	}
	return send
}
