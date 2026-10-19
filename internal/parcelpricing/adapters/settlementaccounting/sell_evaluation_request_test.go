package settlementaccounting_test

import (
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/settlementaccounting"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件证 SA SELL 评价请求记录 → PP「按评价请求形成评价」命令的翻译（ADR-0190 决定四）：方向与目的固定为 SELL·客户
// 费用，计价范围、计价基准时点、包裹与结算币种取自请求，回指是 SELL 请求标识。来源事实引用与结算账户不进命令——计价
// 不拿它们键任何事实。

var sellPricingBasisAt = time.Date(2026, 8, 9, 6, 30, 0, 0, time.UTC)

// synSellRecord 是一份 SYN- 合成的 SA SELL 评价请求记录：包裹 SYN-PARCEL-1 在`有效网络收寄已采用`那一拍，结算币种 CNY。
func synSellRecord(t *testing.T, currency string) saports.SellEvaluationRequestRecord {
	t.Helper()
	request, err := sadomain.SubmitSellEvaluationRequest(sadomain.SellEvaluationRequestSpec{
		ID:                 mustSA(t, sadomain.NewSellEvaluationRequestID, "SELLREQ-SYN-1"),
		Parcel:             mustSA(t, sadomain.NewParcelReference, "SYN-PARCEL-1"),
		Account:            mustSA(t, sadomain.NewSettlementAccountID, "SYN-SA-ACCOUNT-1"),
		SettlementCurrency: mustSA(t, sadomain.NewCurrencyCode, currency),
		PricingScope:       mustSA(t, sadomain.NewEstimatePricingScope, "scope-1"),
		Moment:             sadomain.BillingMomentNetworkIntakeAdopted,
		Source:             mustSA(t, sadomain.NewSourceFactReference, "SYN-INTAKE-ADOPTION-1"),
		PricingBasisAt:     sellPricingBasisAt,
	})
	if err != nil {
		t.Fatalf("submit sell evaluation request: %v", err)
	}
	return saports.SellEvaluationRequestRecord{
		Key: saports.SellEvaluationRequestKey{
			TenantID: mustSA(t, sadomain.NewTenantID, "tenant-1"),
			Request:  request.ID(),
		},
		Request:    request,
		RecordedAt: sellPricingBasisAt.Add(time.Hour),
		State:      sadomain.SellRequestSubmitted,
	}
}

// Covers: 命令逐格译——租户 / 回指取键，SELL + CUSTOMER_CHARGE 一对，计价范围按字面，计价基准时点取请求上那一格（不是
// RecordedAt），来源引用只带包裹、不带发生项，结算币种取请求上快照的那一格，证据层级照装配方声明。
func TestASellEvaluationRequestRecordTranslatesIntoACustomerChargeCommand(t *testing.T) {
	command, err := adapter.TranslateSellEvaluationRequest(synSellRecord(t, "CNY"), ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if command.Tenant.String() != "tenant-1" || command.Request.String() != "SELLREQ-SYN-1" || command.Scope.String() != "scope-1" {
		t.Fatalf("keys = tenant %q request %q scope %q", command.Tenant, command.Request, command.Scope)
	}
	if command.Direction != ppdomain.PricingDirectionSell || command.Purpose != ppdomain.PricingPurposeCustomerCharge {
		t.Fatalf("direction / purpose = %s / %s, want SELL / CUSTOMER_CHARGE", command.Direction, command.Purpose)
	}
	if !command.BasisAt.Equal(sellPricingBasisAt) {
		t.Fatalf("basis at = %v, want the request's pricing basis time %v", command.BasisAt, sellPricingBasisAt)
	}
	if command.Sources.Parcel != "SYN-PARCEL-1" || command.Sources.OccurrenceReferenced() ||
		command.Sources.FeeItem != "" || command.Sources.SupplierAgreement != "" {
		t.Fatalf("sources = %+v, want the parcel alone", command.Sources)
	}
	if command.Settlement.String() != "CNY" {
		t.Fatalf("settlement currency = %q, want the snapshotted CNY", command.Settlement)
	}
	if command.Evidence != ppdomain.EvidenceSynthetic {
		t.Fatalf("evidence = %q, want the declared S", command.Evidence)
	}
}

// Covers: 结算币种译不成计价的币种就拒，不拿卡币种顶；证据层级不给默认；零值记录译不出。三格都是 ErrUntranslatableAnswer：
// 两侧词汇表分歧，重投不会变好。
func TestSellTranslationRefusesWhatItCannotCarryOver(t *testing.T) {
	cases := map[string]struct {
		record   saports.SellEvaluationRequestRecord
		evidence ppdomain.EvidenceKind
	}{
		"币种不是计价词表里的形": {record: synSellRecord(t, "cny"), evidence: ppdomain.EvidenceSynthetic},
		"证据层级空":       {record: synSellRecord(t, "CNY"), evidence: ""},
		"证据层级词表外":     {record: synSellRecord(t, "CNY"), evidence: "X"},
		"零值记录":        {record: saports.SellEvaluationRequestRecord{}, evidence: ppdomain.EvidenceSynthetic},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := adapter.TranslateSellEvaluationRequest(tc.record, tc.evidence)
			if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
			}
		})
	}
}
