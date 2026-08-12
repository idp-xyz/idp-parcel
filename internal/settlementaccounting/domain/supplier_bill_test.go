package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var (
	billReceivedAt = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	billMatchedAt  = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
)

func billClaimSpec(t *testing.T) domain.SupplierBillClaimSpec {
	t.Helper()
	return domain.SupplierBillClaimSpec{
		Claim:       settlementValue(t, domain.NewBillClaimID, "bill-claim-1"),
		Version:     settlementValue(t, domain.NewBillClaimVersion, "bill-claim/v1"),
		Supplier:    settlementValue(t, domain.NewSupplierPartyReference, "partner-1"),
		LegalEntity: settlementValue(t, domain.NewLegalEntityReference, "legal-1"),
		Period:      settlementValue(t, domain.NewBillingPeriodReference, "period-2026-08"),
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		Lines: []domain.BillLine{
			{
				Line:         settlementValue(t, domain.NewBillLineReference, "line-1"),
				FeeItem:      settlementValue(t, domain.NewFeeItemReference, "fee-linehaul"),
				ClaimedMinor: 12000,
			},
			{
				Line:         settlementValue(t, domain.NewBillLineReference, "line-2"),
				FeeItem:      settlementValue(t, domain.NewFeeItemReference, "fee-fuel"),
				ClaimedMinor: 3000,
			},
		},
		ReceivedAt: billReceivedAt,
	}
}

func receivedClaim(t *testing.T) domain.SupplierBillClaim {
	t.Helper()
	claim, err := domain.ReceiveSupplierBillClaim(billClaimSpec(t))
	if err != nil {
		t.Fatalf("receive claim: %v", err)
	}
	return claim
}

func expectedCost(t *testing.T, version string, settlementMinor int64) domain.SupplierExpectedCost {
	t.Helper()
	occurrence, err := domain.NewTransportChargeOccurrence(
		settlementValue(t, domain.NewChargeOccurrenceID, "occurrence-1"),
		settlementValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		settlementValue(t, domain.NewOccurrenceVersion, "occurrence/v1"),
		billReceivedAt.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf("new occurrence: %v", err)
	}
	cost, err := domain.FormSupplierExpectedCost(domain.SupplierExpectedCostSpec{
		Version:            settlementValue(t, domain.NewSupplierCostVersionID, version),
		Occurrence:         occurrence,
		FeeItem:            settlementValue(t, domain.NewFeeItemReference, "fee-linehaul"),
		RuleVersion:        settlementValue(t, domain.NewPurchaseRuleVersionReference, "purchase-rule/v1"),
		Agreement:          settlementValue(t, domain.NewSupplierAgreementReference, "agreement-1"),
		Evaluation:         settlementValue(t, domain.NewBuyEvaluationReference, "buy-evaluation-1"),
		OriginalCurrency:   settlementValue(t, domain.NewCurrencyCode, "USD"),
		OriginalMinor:      settlementMinor,
		SettlementCurrency: settlementValue(t, domain.NewCurrencyCode, "USD"),
		SettlementMinor:    settlementMinor,
	})
	if err != nil {
		t.Fatalf("form expected cost: %v", err)
	}
	return cost
}

// Covers: `AT-SA-079`「账单到达只形成接收，不自动形成应付」与 `AT-SA-100`「只形成主张、
// 匹配、争议、审核应付和贷项」——主张类型上没有审核/应付/付款字段与方法（到达≠应付的
// 结构防线）；行不空不重、金额为正；追加主张回指原主张且不得指向自己。
func TestABillClaimArrivalIsNotAPayable(t *testing.T) {
	claimType := reflect.TypeOf(domain.SupplierBillClaim{})
	for index := 0; index < claimType.NumField(); index++ {
		name := strings.ToLower(claimType.Field(index).Name)
		for _, forbidden := range []string{"payable", "audit", "payment", "approved"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("SupplierBillClaim 携带 %q——到达就能被读成应付", claimType.Field(index).Name)
			}
		}
	}
	for index := 0; index < claimType.NumMethod(); index++ {
		name := strings.ToLower(claimType.Method(index).Name)
		for _, banned := range []string{"payable", "audit", "approve"} {
			if strings.Contains(name, banned) {
				t.Fatalf("SupplierBillClaim 带方法 %q——主张自己铸出了应付", name)
			}
		}
	}

	claim := receivedClaim(t)
	// 计数锚定本夹具：两行 12000+3000（USD，账期 2026-08，收于 2026-08-12T09:00Z）。
	if claim.TotalClaimedMinor() != 15000 {
		t.Fatalf("total = %d, want 15000", claim.TotalClaimedMinor())
	}
	if _, supplements := claim.Supplements(); supplements {
		t.Fatal("首个主张凭空带上了追加前身")
	}

	t.Run("a supplementary claim points back without reopening itself", func(t *testing.T) {
		spec := billClaimSpec(t)
		spec.Claim = settlementValue(t, domain.NewBillClaimID, "bill-claim-2")
		spec.SupplementsClaim = claim.Claim()
		supplementary, err := domain.ReceiveSupplierBillClaim(spec)
		if err != nil {
			t.Fatalf("receive supplementary claim: %v", err)
		}
		original, present := supplementary.Supplements()
		if !present || original != claim.Claim() {
			t.Fatal("追加主张丢了对原主张的关联")
		}

		selfRef := billClaimSpec(t)
		selfRef.SupplementsClaim = selfRef.Claim
		if _, err := domain.ReceiveSupplierBillClaim(selfRef); !errors.Is(err, domain.ErrInvalidBillClaim) {
			t.Fatalf("error = %v; 追加指向自己就是重开原主张", err)
		}
	})

	broken := map[string]func(*domain.SupplierBillClaimSpec){
		"no lines":       func(spec *domain.SupplierBillClaimSpec) { spec.Lines = nil },
		"duplicate line": func(spec *domain.SupplierBillClaimSpec) { spec.Lines = append(spec.Lines, spec.Lines[0]) },
		"zero amount":    func(spec *domain.SupplierBillClaimSpec) { spec.Lines[0].ClaimedMinor = 0 },
		"no currency":    func(spec *domain.SupplierBillClaimSpec) { spec.Currency = domain.CurrencyCode{} },
		"no period":      func(spec *domain.SupplierBillClaimSpec) { spec.Period = domain.BillingPeriodReference{} },
		"no supplier":    func(spec *domain.SupplierBillClaimSpec) { spec.Supplier = domain.SupplierPartyReference{} },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := billClaimSpec(t)
			breakSpec(&spec)
			if _, err := domain.ReceiveSupplierBillClaim(spec); !errors.Is(err, domain.ErrInvalidBillClaim) {
				t.Fatalf("error = %v, want ErrInvalidBillClaim", err)
			}
		})
	}
}

// Covers: `AT-SA-082`/`AT-SA-083`「匹配与未匹配金额分开保留」、`AT-SA-086`「账单金额高于
// 协议/预期成本 → 差异进入争议，不自动接受超额」与 `AT-SA-096`「币种不同且无换算依据
// → 保持待判断」——五格分类各守完备性，金额与币种只从行与预期成本取。
func TestLineMatchingClassifiesDifferencesPerLine(t *testing.T) {
	claim := receivedClaim(t)
	line1 := settlementValue(t, domain.NewBillLineReference, "line-1")
	basis := settlementValue(t, domain.NewMatchBasisReference, "price-evidence-1")

	t.Run("equal amounts form a matched line without a variance basis", func(t *testing.T) {
		match, err := domain.MatchBillLine(claim, line1, domain.LineMatched,
			expectedCost(t, "cost/v1", 12000), domain.MatchBasisReference{}, billMatchedAt)
		if err != nil {
			t.Fatalf("match: %v", err)
		}
		if match.VarianceMinor() != 0 {
			t.Fatalf("variance = %d, want 0", match.VarianceMinor())
		}
		if _, present := match.Basis(); present {
			t.Fatal("已匹配凭空带上了差异依据")
		}

		if _, err := domain.MatchBillLine(claim, line1, domain.LineMatched,
			expectedCost(t, "cost/v1", 11000), domain.MatchBasisReference{}, billMatchedAt); !errors.Is(err, domain.ErrInvalidBillLineMatch) {
			t.Fatalf("error = %v; 金额不等还成了已匹配", err)
		}
		if _, err := domain.MatchBillLine(claim, line1, domain.LineMatched,
			expectedCost(t, "cost/v1", 12000), basis, billMatchedAt); !errors.Is(err, domain.ErrInvalidBillLineMatch) {
			t.Fatalf("error = %v; 已匹配带上了差异依据", err)
		}
	})

	t.Run("an overbilled line becomes a variance with its basis", func(t *testing.T) {
		match, err := domain.MatchBillLine(claim, line1, domain.PriceVariance,
			expectedCost(t, "cost/v1", 11000), basis, billMatchedAt)
		if err != nil {
			t.Fatalf("variance match: %v", err)
		}
		if match.VarianceMinor() != 1000 {
			t.Fatalf("variance = %d, want +1000（供应商多收进入争议，不自动接受）", match.VarianceMinor())
		}
		if _, err := domain.MatchBillLine(claim, line1, domain.PriceVariance,
			expectedCost(t, "cost/v1", 11000), domain.MatchBasisReference{}, billMatchedAt); !errors.Is(err, domain.ErrInvalidBillLineMatch) {
			t.Fatalf("error = %v; 没有依据的差异与数据丢失无从分辨", err)
		}
		if _, err := domain.MatchBillLine(claim, line1, domain.QuantityVariance,
			expectedCost(t, "cost/v1", 12000), basis, billMatchedAt); !errors.Is(err, domain.ErrInvalidBillLineMatch) {
			t.Fatalf("error = %v; 金额相等还报了量差", err)
		}
	})

	t.Run("no matching occurrence carries no expected cost", func(t *testing.T) {
		match, err := domain.MatchBillLine(claim, line1, domain.NoMatchingOccurrence,
			domain.SupplierExpectedCost{}, settlementValue(t, domain.NewMatchBasisReference, "search-scope-1"), billMatchedAt)
		if err != nil {
			t.Fatalf("no-match: %v", err)
		}
		if _, present := match.Expected(); present {
			t.Fatal("无匹配发生项却带了预期成本")
		}
		if match.ExpectedMinor() != 0 {
			t.Fatalf("expected minor = %d, want 0", match.ExpectedMinor())
		}
		if _, err := domain.MatchBillLine(claim, line1, domain.NoMatchingOccurrence,
			expectedCost(t, "cost/v1", 12000), basis, billMatchedAt); !errors.Is(err, domain.ErrInvalidBillLineMatch) {
			t.Fatalf("error = %v; 有预期成本就不是无匹配", err)
		}
	})

	t.Run("duplicate billing points at the prior billing", func(t *testing.T) {
		if _, err := domain.MatchBillLine(claim, line1, domain.DuplicateBilling,
			expectedCost(t, "cost/v1", 12000), domain.MatchBasisReference{}, billMatchedAt); !errors.Is(err, domain.ErrInvalidBillLineMatch) {
			t.Fatalf("error = %v; 重复计费必须指向已计费的原主张", err)
		}
		match, err := domain.MatchBillLine(claim, line1, domain.DuplicateBilling,
			expectedCost(t, "cost/v1", 12000),
			settlementValue(t, domain.NewMatchBasisReference, "prior-claim-0"), billMatchedAt)
		if err != nil {
			t.Fatalf("duplicate match: %v", err)
		}
		if match.Classification() != domain.DuplicateBilling {
			t.Fatalf("classification = %q", match.Classification())
		}
	})

	t.Run("a cross-currency match stays undecidable", func(t *testing.T) {
		spec := billClaimSpec(t)
		spec.Currency = settlementValue(t, domain.NewCurrencyCode, "EUR")
		euroClaim, err := domain.ReceiveSupplierBillClaim(spec)
		if err != nil {
			t.Fatalf("receive EUR claim: %v", err)
		}
		if _, err := domain.MatchBillLine(euroClaim, line1, domain.LineMatched,
			expectedCost(t, "cost/v1", 12000), domain.MatchBasisReference{}, billMatchedAt); !errors.Is(err, domain.ErrCrossCurrencyMatch) {
			t.Fatalf("error = %v, want ErrCrossCurrencyMatch（不用当前汇率猜测）", err)
		}
	})

	t.Run("a line outside the claim cannot match", func(t *testing.T) {
		if _, err := domain.MatchBillLine(claim, settlementValue(t, domain.NewBillLineReference, "line-9"),
			domain.LineMatched, expectedCost(t, "cost/v1", 12000), domain.MatchBasisReference{}, billMatchedAt); !errors.Is(err, domain.ErrInvalidBillLineMatch) {
			t.Fatalf("error = %v; 主张外的行成了匹配", err)
		}
	})

	t.Run("the classification set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, classification := range []domain.MatchClassification{
			domain.LineMatched, domain.QuantityVariance, domain.PriceVariance,
			domain.NoMatchingOccurrence, domain.DuplicateBilling,
		} {
			label := classification.String()
			if label == "" {
				t.Fatalf("classification %d has no label", classification)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 5 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.MatchClassification(len(labels)+1).String() != "" {
			t.Fatal("第六个分类取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-SA-089`「只对通过金额形成审核应付」与 `AT-SA-086`「差异不自动接受」——
// 应付只由`已匹配`经授权审核形成；四种差异格进不来；金额币种从匹配取；应付类型上
// 没有付款/净额字段（应付≠已付款）。
func TestOnlyAuditedMatchesBecomePayables(t *testing.T) {
	payableType := reflect.TypeOf(domain.AuditedPayable{})
	for index := 0; index < payableType.NumField(); index++ {
		name := strings.ToLower(payableType.Field(index).Name)
		for _, forbidden := range []string{"paid", "payment", "net", "credit"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("AuditedPayable 携带 %q——应付就能被读成已付款或净额", payableType.Field(index).Name)
			}
		}
	}

	claim := receivedClaim(t)
	line1 := settlementValue(t, domain.NewBillLineReference, "line-1")
	matched, err := domain.MatchBillLine(claim, line1, domain.LineMatched,
		expectedCost(t, "cost/v1", 12000), domain.MatchBasisReference{}, billMatchedAt)
	if err != nil {
		t.Fatalf("match: %v", err)
	}

	payable, err := domain.FormAuditedPayable(
		matched,
		settlementValue(t, domain.NewPayableID, "payable-1"),
		settlementValue(t, domain.NewLegalEntityReference, "legal-1"),
		settlementValue(t, domain.NewSettlementAccountID, "account-1"),
		settlementValue(t, domain.NewAuditorReference, "auditor-1"),
		billMatchedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("form payable: %v", err)
	}
	currency, amount := payable.Amount()
	if currency.String() != "USD" || amount != 12000 {
		t.Fatalf("amount = %s %d（金额币种只能从匹配取）", currency, amount)
	}

	t.Run("a variance cannot be audited into a payable", func(t *testing.T) {
		variance, err := domain.MatchBillLine(claim, line1, domain.PriceVariance,
			expectedCost(t, "cost/v1", 11000),
			settlementValue(t, domain.NewMatchBasisReference, "price-evidence-1"), billMatchedAt)
		if err != nil {
			t.Fatalf("variance match: %v", err)
		}
		if _, err := domain.FormAuditedPayable(
			variance,
			settlementValue(t, domain.NewPayableID, "payable-x"),
			settlementValue(t, domain.NewLegalEntityReference, "legal-1"),
			settlementValue(t, domain.NewSettlementAccountID, "account-1"),
			settlementValue(t, domain.NewAuditorReference, "auditor-1"),
			billMatchedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrNotAuditable) {
			t.Fatalf("error = %v, want ErrNotAuditable（超额差异不得被顺手接受）", err)
		}
	})

	t.Run("an audit without its auditor is refused", func(t *testing.T) {
		if _, err := domain.FormAuditedPayable(
			matched,
			settlementValue(t, domain.NewPayableID, "payable-y"),
			settlementValue(t, domain.NewLegalEntityReference, "legal-1"),
			settlementValue(t, domain.NewSettlementAccountID, "account-1"),
			domain.AuditorReference{},
			billMatchedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidAuditedPayable) {
			t.Fatalf("error = %v; 无授权不得人工接受", err)
		}
	})
}

// Covers: `AT-SA-090`「供应商贷项更正已审核金额 → 原主张/应付保留，追加具有稳定身份、
// 结算账户、借贷方向和原应付关系的贷项；不另建净额审核应付」——贷项必带原应付关系与
// 原因；应付类型上没有任何吸收贷项的方法（不静默净额）；贷项类型上没有预期成本字段
// （预期成本纠错归 UC-SA-002）。
func TestACreditNoteSupplementsWithoutRewriting(t *testing.T) {
	payableType := reflect.TypeOf(domain.AuditedPayable{})
	for index := 0; index < payableType.NumMethod(); index++ {
		name := strings.ToLower(payableType.Method(index).Name)
		for _, banned := range []string{"credit", "net", "absorb", "offset"} {
			if strings.Contains(name, banned) {
				t.Fatalf("AuditedPayable 带方法 %q——贷项就能被静默净入应付", name)
			}
		}
	}
	noteType := reflect.TypeOf(domain.SupplierCreditNote{})
	for index := 0; index < noteType.NumField(); index++ {
		if noteType.Field(index).Type == reflect.TypeOf(domain.SupplierExpectedCost{}) {
			t.Fatalf("SupplierCreditNote 持有预期成本本体 %q——贷项就有了改写 UC-SA-002 结果的把手", noteType.Field(index).Name)
		}
	}

	spec := domain.SupplierCreditNoteSpec{
		Note:        settlementValue(t, domain.NewCreditNoteID, "credit-note-1"),
		Version:     settlementValue(t, domain.NewCreditNoteVersion, "credit-note/v1"),
		Payable:     settlementValue(t, domain.NewPayableID, "payable-1"),
		Claim:       settlementValue(t, domain.NewBillClaimID, "bill-claim-1"),
		Account:     settlementValue(t, domain.NewSettlementAccountID, "account-1"),
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 1000,
		Reason:      settlementValue(t, domain.NewCreditReasonReference, "supplier-correction-1"),
		IssuedAt:    billMatchedAt.Add(48 * time.Hour),
	}
	note, err := domain.FormSupplierCreditNote(spec)
	if err != nil {
		t.Fatalf("form credit note: %v", err)
	}
	if note.Payable().String() != "payable-1" {
		t.Fatal("贷项丢了原应付关系")
	}

	broken := map[string]func(*domain.SupplierCreditNoteSpec){
		"no payable relation": func(spec *domain.SupplierCreditNoteSpec) { spec.Payable = domain.PayableID{} },
		"no reason":           func(spec *domain.SupplierCreditNoteSpec) { spec.Reason = domain.CreditReasonReference{} },
		"zero amount":         func(spec *domain.SupplierCreditNoteSpec) { spec.AmountMinor = 0 },
		"negative amount":     func(spec *domain.SupplierCreditNoteSpec) { spec.AmountMinor = -5 },
		"no account":          func(spec *domain.SupplierCreditNoteSpec) { spec.Account = domain.SettlementAccountID{} },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			broken := spec
			breakSpec(&broken)
			if _, err := domain.FormSupplierCreditNote(broken); !errors.Is(err, domain.ErrInvalidCreditNote) {
				t.Fatalf("error = %v, want ErrInvalidCreditNote", err)
			}
		})
	}
}
