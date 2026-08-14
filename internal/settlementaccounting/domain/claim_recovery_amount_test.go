package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var claimFormedAt = time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)

func claimAmountSpec(t *testing.T, kind domain.CustomerClaimAmountKind) domain.CustomerClaimAmountSpec {
	t.Helper()
	spec := domain.CustomerClaimAmountSpec{
		ID:             settlementValue(t, domain.NewCustomerClaimAmountID, "claim-amount-1"),
		Kind:           kind,
		ClaimItem:      settlementValue(t, domain.NewClaimItemReference, "claim-item-1"),
		Responsibility: settlementValue(t, domain.NewResponsibilityConclusionReference, "responsibility/v1"),
		RuleVersion:    settlementValue(t, domain.NewAmountRuleVersionReference, "amount-rule/v1"),
		LegalEntity:    settlementValue(t, domain.NewLegalEntityReference, "legal-1"),
		Currency:       settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor:    5000,
		Period:         settlementValue(t, domain.NewBillingPeriodReference, "period-2026-09"),
		FormedAt:       claimFormedAt,
	}
	if kind == domain.ClaimChargeRefund {
		spec.OriginalCharge = settlementValue(t, domain.NewCustomerChargeID, "charge-1")
	}
	return spec
}

func formedReceivable(t *testing.T, amountMinor int64) domain.RecoveryReceivable {
	t.Helper()
	receivable, err := domain.FormRecoveryReceivable(domain.RecoveryReceivableSpec{
		ID:             settlementValue(t, domain.NewRecoveryReceivableID, "receivable-1"),
		Matter:         settlementValue(t, domain.NewRecoveryMatterReference, "recovery-matter-1"),
		Responsibility: settlementValue(t, domain.NewResponsibilityConclusionReference, "responsibility/v1"),
		Counterparty:   settlementValue(t, domain.NewRecoveryCounterpartyReference, "partner-1"),
		RuleVersion:    settlementValue(t, domain.NewAmountRuleVersionReference, "amount-rule/v1"),
		LegalEntity:    settlementValue(t, domain.NewLegalEntityReference, "legal-1"),
		Currency:       settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor:    amountMinor,
		FormedAt:       claimFormedAt,
	})
	if err != nil {
		t.Fatalf("form receivable: %v", err)
	}
	return receivable
}

// Covers: `AT-SA-144`「没有责任结论 → 不形成赔付或追偿金额」、`AT-SA-147`「保存采用
// 规则版本」与 `AT-SA-153`「索赔费用退款与计价纠错分族」——赔付/退款二格各守完备性；
// 类型上没有任何追偿或净额字段（分层，AT-SA-148）。
func TestClaimAmountsFormOnlyFromResponsibilityConclusions(t *testing.T) {
	amountType := reflect.TypeOf(domain.CustomerClaimAmount{})
	for index := 0; index < amountType.NumField(); index++ {
		name := strings.ToLower(amountType.Field(index).Name)
		for _, forbidden := range []string{"recovery", "acknowledg", "net", "received"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("CustomerClaimAmount 携带 %q——赔付就能被追偿状态牵住或净额抵销", amountType.Field(index).Name)
			}
		}
	}

	compensation, err := domain.FormCustomerClaimAmount(claimAmountSpec(t, domain.CustomerCompensationPayable))
	if err != nil {
		t.Fatalf("form compensation: %v", err)
	}
	if _, present := compensation.OriginalCharge(); present {
		t.Fatal("赔付义务凭空挂上了原费用——混进了费用调整族")
	}

	refund, err := domain.FormCustomerClaimAmount(claimAmountSpec(t, domain.ClaimChargeRefund))
	if err != nil {
		t.Fatalf("form refund: %v", err)
	}
	if _, present := refund.OriginalCharge(); !present {
		t.Fatal("索赔退款丢了被贷记的原费用")
	}

	t.Run("without a responsibility conclusion nothing forms", func(t *testing.T) {
		spec := claimAmountSpec(t, domain.CustomerCompensationPayable)
		spec.Responsibility = domain.ResponsibilityConclusionReference{}
		if _, err := domain.FormCustomerClaimAmount(spec); !errors.Is(err, domain.ErrInvalidClaimAmount) {
			t.Fatalf("error = %v, want ErrInvalidClaimAmount（只有证据没有结论保持待判断）", err)
		}
	})

	t.Run("a refund without its original charge is refused", func(t *testing.T) {
		spec := claimAmountSpec(t, domain.ClaimChargeRefund)
		spec.OriginalCharge = domain.CustomerChargeID{}
		if _, err := domain.FormCustomerClaimAmount(spec); !errors.Is(err, domain.ErrInvalidClaimAmount) {
			t.Fatalf("error = %v; 退款不知贷记哪笔费用", err)
		}
	})

	t.Run("a compensation carrying a charge is refused", func(t *testing.T) {
		spec := claimAmountSpec(t, domain.CustomerCompensationPayable)
		spec.OriginalCharge = settlementValue(t, domain.NewCustomerChargeID, "charge-1")
		if _, err := domain.FormCustomerClaimAmount(spec); !errors.Is(err, domain.ErrInvalidClaimAmount) {
			t.Fatalf("error = %v; 赔付混成了费用调整", err)
		}
	})

	t.Run("the claim amount kind set is closed", func(t *testing.T) {
		if domain.CustomerClaimAmountKind(3).String() != "" {
			t.Fatal("第三个客户金额取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-SA-148`/`AT-SA-149`「客户赔付先行、应追偿独立形成，不等待对方认可或到账」
// ——应追偿类型上没有已认可/已到账/已核销字段（分层结构防线）。
func TestRecoveryReceivableIsIndependentOfAcknowledgement(t *testing.T) {
	receivableType := reflect.TypeOf(domain.RecoveryReceivable{})
	for index := 0; index < receivableType.NumField(); index++ {
		name := strings.ToLower(receivableType.Field(index).Name)
		for _, forbidden := range []string{"acknowledg", "received", "settled", "applied", "compensat"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("RecoveryReceivable 携带 %q——应追偿就能被读成已认可或已到账", receivableType.Field(index).Name)
			}
		}
	}

	receivable := formedReceivable(t, 10000)
	if _, amount := receivable.Amount(); amount != 10000 {
		t.Fatalf("amount = %d", amount)
	}

	compensation, err := domain.FormCustomerClaimAmount(claimAmountSpec(t, domain.CustomerCompensationPayable))
	if err != nil {
		t.Fatalf("form compensation alongside: %v（赔付不等追偿）", err)
	}
	if compensation.Responsibility() != receivable.Responsibility() {
		t.Fatal("夹具应共用同一责任结论版本")
	}
}

// Covers: `AT-SA-150`「对方只认可部分 → 保留完整应追偿，另行形成部分认可金额和未认可
// 范围」与结果契约「不把要求补充、审核中、拒绝或无响应当成认可，不把认可解释为到账」
// ——立场封闭二值（拒绝/审核中无格），全部认可须等额、部分认可须少于应追偿。
func TestAcknowledgementCoversOnlyTheAcceptedRange(t *testing.T) {
	acknowledgementType := reflect.TypeOf(domain.RecoveryAcknowledgement{})
	for index := 0; index < acknowledgementType.NumField(); index++ {
		name := strings.ToLower(acknowledgementType.Field(index).Name)
		for _, forbidden := range []string{"received", "settled", "applied", "funds"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("RecoveryAcknowledgement 携带 %q——认可就能被读成到账", acknowledgementType.Field(index).Name)
			}
		}
	}

	receivable := formedReceivable(t, 10000)
	response := settlementValue(t, domain.NewCounterpartyResponseReference, "response/v1")

	partial, err := domain.AcknowledgeRecovery(
		receivable,
		settlementValue(t, domain.NewAcknowledgementID, "acknowledgement-1"),
		response,
		domain.ResponsePartiallyAccepted,
		6000,
		claimFormedAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("acknowledge partially: %v", err)
	}
	// 计数锚定本夹具：应追偿 10000，认可 6000 → 未认可 4000。
	if partial.UnacknowledgedMinor() != 4000 {
		t.Fatalf("unacknowledged = %d, want 4000", partial.UnacknowledgedMinor())
	}

	t.Run("full acceptance must equal the receivable", func(t *testing.T) {
		if _, err := domain.AcknowledgeRecovery(
			receivable,
			settlementValue(t, domain.NewAcknowledgementID, "acknowledgement-x"),
			response,
			domain.ResponseAccepted,
			6000,
			claimFormedAt.Add(24*time.Hour),
		); !errors.Is(err, domain.ErrInvalidAcknowledgement) {
			t.Fatalf("error = %v; 全部接受却不等额", err)
		}
		full, err := domain.AcknowledgeRecovery(
			receivable,
			settlementValue(t, domain.NewAcknowledgementID, "acknowledgement-2"),
			response,
			domain.ResponseAccepted,
			10000,
			claimFormedAt.Add(24*time.Hour),
		)
		if err != nil {
			t.Fatalf("acknowledge fully: %v", err)
		}
		if full.UnacknowledgedMinor() != 0 {
			t.Fatalf("unacknowledged = %d, want 0", full.UnacknowledgedMinor())
		}
	})

	t.Run("partial acceptance cannot claim the full amount", func(t *testing.T) {
		if _, err := domain.AcknowledgeRecovery(
			receivable,
			settlementValue(t, domain.NewAcknowledgementID, "acknowledgement-y"),
			response,
			domain.ResponsePartiallyAccepted,
			10000,
			claimFormedAt.Add(24*time.Hour),
		); !errors.Is(err, domain.ErrInvalidAcknowledgement) {
			t.Fatalf("error = %v; 部分接受占满了全额", err)
		}
	})

	t.Run("acknowledgement beyond the receivable is refused", func(t *testing.T) {
		if _, err := domain.AcknowledgeRecovery(
			receivable,
			settlementValue(t, domain.NewAcknowledgementID, "acknowledgement-z"),
			response,
			domain.ResponsePartiallyAccepted,
			10001,
			claimFormedAt.Add(24*time.Hour),
		); !errors.Is(err, domain.ErrInvalidAcknowledgement) {
			t.Fatalf("error = %v; 认可超过了应追偿", err)
		}
	})

	t.Run("a rejection has no standing to acknowledge", func(t *testing.T) {
		// 立场是封闭二值：拒绝、审核中、无响应在类型上没有格——第三个取值没有标签。
		if domain.ResponseStanding(3).String() != "" {
			t.Fatal("第三个立场取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-SA-152`「新证据改变责任 → 原责任和原金额保留，只追加金额差额」与
// `AT-SA-155`「真实资金变化不改写金额」——调整封闭三因（资金变化无格）、依据必备、
// 目标三层各可指。
func TestAdjustmentsAppendWithoutRewritingAmounts(t *testing.T) {
	spec := domain.ClaimAmountAdjustmentSpec{
		ID:          settlementValue(t, domain.NewClaimAmountAdjustmentID, "claim-adjustment-1"),
		TargetKind:  domain.AdjustsRecoveryReceivable,
		Target:      settlementValue(t, domain.NewAdjustedAmountReference, "receivable-1"),
		Reason:      domain.ResponsibilityRevised,
		Basis:       settlementValue(t, domain.NewResponsibilityConclusionReference, "responsibility/v2"),
		Direction:   domain.AdjustmentCredit,
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 2000,
		Period:      settlementValue(t, domain.NewBillingPeriodReference, "period-2026-10"),
		FormedAt:    claimFormedAt.Add(48 * time.Hour),
	}
	adjustment, err := domain.FormClaimAmountAdjustment(spec)
	if err != nil {
		t.Fatalf("form adjustment: %v", err)
	}
	if adjustment.Reason() != domain.ResponsibilityRevised {
		t.Fatalf("reason = %q", adjustment.Reason())
	}

	t.Run("an adjustment without its new basis is refused", func(t *testing.T) {
		broken := spec
		broken.Basis = domain.ResponsibilityConclusionReference{}
		if _, err := domain.FormClaimAmountAdjustment(broken); !errors.Is(err, domain.ErrInvalidAmountAdjustment) {
			t.Fatalf("error = %v; 没有新依据的差额与数据丢失无从分辨", err)
		}
	})

	t.Run("the reason and target sets are closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []domain.ClaimAdjustmentReason{
			domain.ResponsibilityRevised, domain.AmountRuleCorrected, domain.AcknowledgementChanged,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("reason labels collapsed into %d", len(labels))
		}
		if domain.ClaimAdjustmentReason(4).String() != "" {
			t.Fatal("第四个调整原因带了标签——资金变化溜进了金额调整")
		}
		if domain.AdjustedAmountKind(4).String() != "" {
			t.Fatal("第四个目标层带了标签——封闭集合被悄悄放开")
		}
	})
}

func TestRehydrateAcknowledgementDoesNotNeedTheReceivable(t *testing.T) {
	receivable := formedReceivable(t, 10000)
	formed, err := domain.AcknowledgeRecovery(
		receivable,
		settlementValue(t, domain.NewAcknowledgementID, "acknowledgement-1"),
		settlementValue(t, domain.NewCounterpartyResponseReference, "response/v1"),
		domain.ResponsePartiallyAccepted,
		6000,
		claimFormedAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("acknowledge: %v", err)
	}

	restored, err := domain.RehydrateRecoveryAcknowledgement(domain.RehydrateRecoveryAcknowledgementSpec{
		ID:                formed.ID(),
		Receivable:        formed.Receivable(),
		Response:          formed.Response(),
		Standing:          formed.Standing(),
		Currency:          settlementValue(t, domain.NewCurrencyCode, "USD"),
		AcknowledgedMinor: 6000,
		ReceivableMinor:   10000,
		AcknowledgedAt:    formed.AcknowledgedAt(),
	})
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if restored.UnacknowledgedMinor() != 4000 {
		t.Fatalf("unacknowledged = %d", restored.UnacknowledgedMinor())
	}

	if _, err := domain.RehydrateRecoveryAcknowledgement(domain.RehydrateRecoveryAcknowledgementSpec{
		ID:                formed.ID(),
		Receivable:        formed.Receivable(),
		Response:          formed.Response(),
		Standing:          domain.ResponseAccepted,
		Currency:          settlementValue(t, domain.NewCurrencyCode, "USD"),
		AcknowledgedMinor: 6000,
		ReceivableMinor:   10000,
		AcknowledgedAt:    formed.AcknowledgedAt(),
	}); !errors.Is(err, domain.ErrInvalidAcknowledgement) {
		t.Fatalf("error = %v; 全部接受却不等额从重建门溜过", err)
	}
}
