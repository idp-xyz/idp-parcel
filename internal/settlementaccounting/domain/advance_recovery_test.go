package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

var advanceJudgedAt = time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC)

func advanceSpec(t *testing.T, verdict domain.AdvanceVerdict) domain.ActualAdvanceAssessmentSpec {
	t.Helper()
	spec := domain.ActualAdvanceAssessmentSpec{
		ID:          settlementValue(t, domain.NewAdvanceAssessmentID, "advance-1"),
		Obligation:  settlementValue(t, domain.NewTaxObligationReference, "tax-obligation-1"),
		Verdict:     verdict,
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 10000,
		Version:     settlementValue(t, domain.NewAdvanceAssessmentVersion, "advance/v1"),
		JudgedAt:    advanceJudgedAt,
	}
	if verdict == domain.AdvanceEstablished {
		spec.FundsFact = settlementValue(t, domain.NewFundsFactReference, "bank-fact-1")
		spec.Payer = settlementValue(t, domain.NewAdvancePayerReference, "legal-1")
		spec.Responsibility = settlementValue(t, domain.NewAdvanceResponsibilityReference, "duty-basis-1")
	} else {
		spec.Basis = settlementValue(t, domain.NewAssessmentBasisReference, "assessment-basis-1")
	}
	return spec
}

func establishedAdvance(t *testing.T) domain.ActualAdvanceAssessment {
	t.Helper()
	assessment, err := domain.AssessActualAdvance(advanceSpec(t, domain.AdvanceEstablished))
	if err != nil {
		t.Fatalf("assess advance: %v", err)
	}
	return assessment
}

// Covers: `AT-SA-002`「只有核定没有资金事实 → 不成立/待判断」、`AT-SA-003`「只有付款
// 不能关联义务 → 不把付款自动记为代垫」、`AT-SA-004`「预付不证明代垫」与结果契约
// 「不得用资料缺失伪装为不成立」——成立须资金事实+付款方+责任依据三件，负向须依据；
// 类型上没有重算税费或制造付款的入口。
func TestAdvanceAssessmentJudgesWithoutRecomputingOrFabricating(t *testing.T) {
	assessmentType := reflect.TypeOf(domain.ActualAdvanceAssessment{})
	for index := 0; index < assessmentType.NumMethod(); index++ {
		name := strings.ToLower(assessmentType.Method(index).Name)
		for _, banned := range []string{"recalculate", "recompute", "pay", "settle"} {
			if strings.Contains(name, banned) {
				t.Fatalf("ActualAdvanceAssessment 带方法 %q——判断就有了重算税费或制造付款的入口", name)
			}
		}
	}

	assessment := establishedAdvance(t)
	if _, present := assessment.FundsFact(); !present {
		t.Fatal("成立的代垫丢了资金事实引用")
	}
	if _, present := assessment.Basis(); present {
		t.Fatal("成立凭空带上了负向依据")
	}

	missing := map[string]func(*domain.ActualAdvanceAssessmentSpec){
		"funds fact": func(spec *domain.ActualAdvanceAssessmentSpec) { spec.FundsFact = domain.FundsFactReference{} },
		"payer":      func(spec *domain.ActualAdvanceAssessmentSpec) { spec.Payer = domain.AdvancePayerReference{} },
		"responsibility": func(spec *domain.ActualAdvanceAssessmentSpec) {
			spec.Responsibility = domain.AdvanceResponsibilityReference{}
		},
	}
	for name, drop := range missing {
		t.Run("established without "+name+" is refused", func(t *testing.T) {
			spec := advanceSpec(t, domain.AdvanceEstablished)
			drop(&spec)
			if _, err := domain.AssessActualAdvance(spec); !errors.Is(err, domain.ErrInvalidAdvanceAssessment) {
				t.Fatalf("error = %v; 缺 %s 的「成立」说的不是实际替他方付款", err, name)
			}
		})
	}

	t.Run("not-established needs its negative basis", func(t *testing.T) {
		spec := advanceSpec(t, domain.AdvanceNotEstablishedVerdict)
		spec.Basis = domain.AssessmentBasisReference{}
		if _, err := domain.AssessActualAdvance(spec); !errors.Is(err, domain.ErrInvalidAdvanceAssessment) {
			t.Fatalf("error = %v; 资料缺失伪装成了不成立", err)
		}
	})

	t.Run("undecided and conflicting need their gap basis", func(t *testing.T) {
		for _, verdict := range []domain.AdvanceVerdict{domain.AdvanceUndecided, domain.AdvanceConflicting} {
			spec := advanceSpec(t, verdict)
			spec.Basis = domain.AssessmentBasisReference{}
			if _, err := domain.AssessActualAdvance(spec); !errors.Is(err, domain.ErrInvalidAdvanceAssessment) {
				t.Fatalf("error = %v; 没有依据的 %s", err, verdict)
			}
		}
	})

	t.Run("the verdict set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, verdict := range []domain.AdvanceVerdict{
			domain.AdvanceEstablished, domain.AdvanceNotEstablishedVerdict,
			domain.AdvanceUndecided, domain.AdvanceConflicting,
		} {
			label := verdict.String()
			if label == "" {
				t.Fatalf("verdict %d has no label", verdict)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.AdvanceVerdict(len(labels)+1).String() != "" {
			t.Fatal("第五个判断取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-SA-005`「运营法人实际付款且合同明确客户承担 → 形成代垫与回收」、`AT-SA-006`
// 「客户直接付款 → 代垫不成立不形成回收」与明句「超出范围的金额不得为对平自动转为
// 回收」——回收只立在已成立代垫上、合同依据必备、限额内；无收款/核销字段。
func TestRecoveryNeedsAnEstablishedAdvanceAndContract(t *testing.T) {
	recoveryType := reflect.TypeOf(domain.CustomerAdvanceRecovery{})
	for index := 0; index < recoveryType.NumField(); index++ {
		name := strings.ToLower(recoveryType.Field(index).Name)
		for _, forbidden := range []string{"paid", "applied", "settled", "invoiced"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("CustomerAdvanceRecovery 携带 %q——回收就能被读成已收款", recoveryType.Field(index).Name)
			}
		}
	}

	assessment := establishedAdvance(t)
	recovery, err := domain.FormCustomerAdvanceRecovery(
		assessment,
		settlementValue(t, domain.NewAdvanceRecoveryID, "recovery-1"),
		settlementValue(t, domain.NewRecoveryCustomerReference, "customer-1"),
		settlementValue(t, domain.NewContractResponsibilityReference, "contract-duty-1"),
		settlementValue(t, domain.NewSettlementAccountID, "account-1"),
		10000,
		advanceJudgedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("form recovery: %v", err)
	}
	if recovery.Assessment() != assessment.ID() {
		t.Fatal("回收丢了对代垫判断的锚")
	}

	t.Run("an undecided advance forms no recovery", func(t *testing.T) {
		undecided, err := domain.AssessActualAdvance(advanceSpec(t, domain.AdvanceUndecided))
		if err != nil {
			t.Fatalf("assess undecided: %v", err)
		}
		if _, err := domain.FormCustomerAdvanceRecovery(
			undecided,
			settlementValue(t, domain.NewAdvanceRecoveryID, "recovery-x"),
			settlementValue(t, domain.NewRecoveryCustomerReference, "customer-1"),
			settlementValue(t, domain.NewContractResponsibilityReference, "contract-duty-1"),
			settlementValue(t, domain.NewSettlementAccountID, "account-1"),
			5000,
			advanceJudgedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrAdvanceNotEstablished) {
			t.Fatalf("error = %v, want ErrAdvanceNotEstablished（分层）", err)
		}
	})

	t.Run("a recovery without its contract basis is refused", func(t *testing.T) {
		if _, err := domain.FormCustomerAdvanceRecovery(
			assessment,
			settlementValue(t, domain.NewAdvanceRecoveryID, "recovery-y"),
			settlementValue(t, domain.NewRecoveryCustomerReference, "customer-1"),
			domain.ContractResponsibilityReference{},
			settlementValue(t, domain.NewSettlementAccountID, "account-1"),
			5000,
			advanceJudgedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidAdvanceRecovery) {
			t.Fatalf("error = %v; 回收不是从代垫自动长出来的", err)
		}
	})

	t.Run("a recovery beyond the advance amount is refused", func(t *testing.T) {
		if _, err := domain.FormCustomerAdvanceRecovery(
			assessment,
			settlementValue(t, domain.NewAdvanceRecoveryID, "recovery-z"),
			settlementValue(t, domain.NewRecoveryCustomerReference, "customer-1"),
			settlementValue(t, domain.NewContractResponsibilityReference, "contract-duty-1"),
			settlementValue(t, domain.NewSettlementAccountID, "account-1"),
			10001,
			advanceJudgedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidAdvanceRecovery) {
			t.Fatalf("error = %v; 超出范围的金额为对平转成了回收", err)
		}
	})
}

// Covers: 结果契约「客户代垫回收调整已形成：原回收、调整原因、新旧依据、差额和适用
// 账期；不得覆盖原回收或已发布对账单」——调整封闭三因带新依据；真实收款不在其中。
func TestRecoveryAdjustmentsCarryReasonsWithoutRewriting(t *testing.T) {
	spec := domain.RecoveryAdjustmentSpec{
		ID:          settlementValue(t, domain.NewRecoveryAdjustmentID, "recovery-adjustment-1"),
		Recovery:    settlementValue(t, domain.NewAdvanceRecoveryID, "recovery-1"),
		Reason:      domain.TaxAssessmentCorrected,
		NewBasis:    settlementValue(t, domain.NewAssessmentBasisReference, "tax-correction-1"),
		Direction:   domain.AdjustmentCredit,
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: 1500,
		Period:      settlementValue(t, domain.NewBillingPeriodReference, "period-2026-09"),
		FormedAt:    advanceJudgedAt.Add(48 * time.Hour),
	}
	adjustment, err := domain.FormRecoveryAdjustment(spec)
	if err != nil {
		t.Fatalf("form adjustment: %v", err)
	}
	if adjustment.Reason() != domain.TaxAssessmentCorrected {
		t.Fatalf("reason = %q", adjustment.Reason())
	}

	t.Run("an adjustment without its new basis is refused", func(t *testing.T) {
		broken := spec
		broken.NewBasis = domain.AssessmentBasisReference{}
		if _, err := domain.FormRecoveryAdjustment(broken); !errors.Is(err, domain.ErrInvalidRecoveryAdjustment) {
			t.Fatalf("error = %v; 没有新依据的差额与数据丢失无从分辨", err)
		}
	})

	t.Run("the reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []domain.RecoveryAdjustmentReason{
			domain.TaxAssessmentCorrected, domain.FundsFactRevised, domain.CustomerResponsibilityChanged,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.RecoveryAdjustmentReason(4).String() != "" {
			t.Fatal("第四个调整原因带了标签——真实收款溜进了回收调整")
		}
	})
}
