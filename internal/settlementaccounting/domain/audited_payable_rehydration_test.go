package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var payableRehydratedAt = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)

func payableRehydrationSpec(t *testing.T) domain.RehydrateAuditedPayableSpec {
	t.Helper()
	return domain.RehydrateAuditedPayableSpec{
		Payable:     settlementValue(t, domain.NewPayableID, "payable-1"),
		Claim:       settlementValue(t, domain.NewBillClaimID, "claim-1"),
		Line:        settlementValue(t, domain.NewBillLineReference, "line-1"),
		Expected:    settlementValue(t, domain.NewSupplierCostVersionID, "cost-v1"),
		LegalEntity: settlementValue(t, domain.NewLegalEntityReference, "legal-1"),
		Account:     settlementValue(t, domain.NewSettlementAccountID, "account-1"),
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 12000,
		Auditor:     settlementValue(t, domain.NewAuditorReference, "auditor-1"),
		AuditedAt:   payableRehydratedAt,
	}
}

// Covers: ADR-0028/0030 在审核应付上的重建门——行数据回到领域前复验结果自证的那几件：
// 身份与依据齐备、预期成本在场（`已匹配`行必有它）、金额恒正、审核时点在场；合法的一行
// 读回后与形成期的访问器逐格一致。
func TestARehydratedPayableRevalidatesItsOwnEvidence(t *testing.T) {
	payable, err := domain.RehydrateAuditedPayable(payableRehydrationSpec(t))
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if payable.Payable().String() != "payable-1" || payable.Claim().String() != "claim-1" ||
		payable.Line().String() != "line-1" || payable.Expected().String() != "cost-v1" ||
		payable.LegalEntity().String() != "legal-1" || payable.Account().String() != "account-1" ||
		payable.Auditor().String() != "auditor-1" || !payable.AuditedAt().Equal(payableRehydratedAt) {
		t.Fatalf("重建后的应付与行不一致：%+v", payable)
	}
	if currency, amount := payable.Amount(); currency.String() != "USD" || amount != 12000 {
		t.Fatalf("amount = %s %d", currency, amount)
	}

	rejections := map[string]func(*domain.RehydrateAuditedPayableSpec){
		"a zero amount":            func(spec *domain.RehydrateAuditedPayableSpec) { spec.AmountMinor = 0 },
		"a missing expected cost":  func(spec *domain.RehydrateAuditedPayableSpec) { spec.Expected = domain.SupplierCostVersionID{} },
		"a missing auditor":        func(spec *domain.RehydrateAuditedPayableSpec) { spec.Auditor = domain.AuditorReference{} },
		"a missing account":        func(spec *domain.RehydrateAuditedPayableSpec) { spec.Account = domain.SettlementAccountID{} },
		"a missing audited moment": func(spec *domain.RehydrateAuditedPayableSpec) { spec.AuditedAt = time.Time{} },
	}
	for name, corrupt := range rejections {
		t.Run(name+" is refused", func(t *testing.T) {
			spec := payableRehydrationSpec(t)
			corrupt(&spec)
			if _, err := domain.RehydrateAuditedPayable(spec); !errors.Is(err, domain.ErrInvalidRehydratedPayable) {
				t.Fatalf("error = %v, want ErrInvalidRehydratedPayable", err)
			}
		})
	}
}
