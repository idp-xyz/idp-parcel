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
	fundsOccurredAt = time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)
	fundsAppliedAt  = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
)

func adoptedFact(t *testing.T, kind domain.FundsFactKind, amountMinor int64) domain.ExternalFundsFact {
	t.Helper()
	fact, err := domain.AdoptExternalFundsFact(domain.ExternalFundsFactSpec{
		Fact:        settlementValue(t, domain.NewFundsFactReference, "bank-fact-1"),
		Source:      settlementValue(t, domain.NewFundsSourceRegistrationReference, "source-bank-feed-1"),
		Kind:        kind,
		Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
		AmountMinor: amountMinor,
		Version:     settlementValue(t, domain.NewFundsFactVersion, "bank-fact/v1"),
		OccurredAt:  fundsOccurredAt,
	})
	if err != nil {
		t.Fatalf("adopt funds fact: %v", err)
	}
	return fact
}

func mappingFor(t *testing.T, fact domain.ExternalFundsFact, id string, kind domain.SettlementTargetKind, target string) domain.FundsMapping {
	t.Helper()
	mapping, err := domain.MapFundsToTarget(
		fact,
		settlementValue(t, domain.NewMappingReference, id),
		kind,
		settlementValue(t, domain.NewSettlementTargetReference, target),
		settlementValue(t, domain.NewMappingBasisReference, "payment-instruction-1"),
		fundsOccurredAt.Add(30*time.Minute),
	)
	if err != nil {
		t.Fatalf("map funds: %v", err)
	}
	return mapping
}

// Covers: `AT-SA-101`「外部收款事实到达 → 只形成外部事实采用和待匹配入口，不直接成为
// 已核销」、`AT-SA-102`「来源未经登记 → 不建立本地资金事实」与 `AT-SA-114`「金额被更正
// → 保留原版本，按新有效版本重算」——引用类型上没有余额或已结清字段。
func TestAnAdoptedFundsFactIsAReferenceNotABalance(t *testing.T) {
	factType := reflect.TypeOf(domain.ExternalFundsFact{})
	for index := 0; index < factType.NumField(); index++ {
		name := strings.ToLower(factType.Field(index).Name)
		for _, forbidden := range []string{"balance", "settled", "applied", "cleared"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("ExternalFundsFact 携带 %q——外部事实引用就能被读成已核销或余额", factType.Field(index).Name)
			}
		}
	}

	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 8000)
	if currency, amount := fact.Amount(); currency.String() != "USD" || amount != 8000 {
		t.Fatalf("amount = %s %d", currency, amount)
	}

	t.Run("an unregistered source cannot be adopted", func(t *testing.T) {
		if _, err := domain.AdoptExternalFundsFact(domain.ExternalFundsFactSpec{
			Fact:        settlementValue(t, domain.NewFundsFactReference, "bank-fact-2"),
			Kind:        domain.FundsReceiptConfirmed,
			Currency:    settlementValue(t, domain.NewCurrencyCode, "USD"),
			AmountMinor: 100,
			Version:     settlementValue(t, domain.NewFundsFactVersion, "bank-fact/v1"),
			OccurredAt:  fundsOccurredAt,
		}); !errors.Is(err, domain.ErrInvalidFundsFact) {
			t.Fatalf("error = %v, want ErrInvalidFundsFact", err)
		}
	})

	t.Run("an amount correction forms a new version keeping the original", func(t *testing.T) {
		corrected, err := fact.CorrectAmount(7500,
			settlementValue(t, domain.NewFundsFactVersion, "bank-fact/v2"),
			fundsOccurredAt.Add(time.Hour))
		if err != nil {
			t.Fatalf("correct amount: %v", err)
		}
		predecessor, present := corrected.Corrects()
		if !present || predecessor.String() != "bank-fact/v1" {
			t.Fatalf("corrects = %q present=%v", predecessor, present)
		}
		if _, amount := fact.Amount(); amount != 8000 {
			t.Fatal("更正改写了原版本金额")
		}
		if _, err := fact.CorrectAmount(7500, fact.Version(), fundsOccurredAt.Add(time.Hour)); !errors.Is(err, domain.ErrInvalidFundsFact) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("the fact kind set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, kind := range []domain.FundsFactKind{
			domain.FundsReceiptConfirmed, domain.FundsPaymentFailed, domain.FundsReturned,
		} {
			label := kind.String()
			if label == "" {
				t.Fatalf("kind %d has no label", kind)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.FundsFactKind(len(labels)+1).String() != "" {
			t.Fatal("第四个事实取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: UC-SA-005 匹配纪律「金额相同、同一客户或同一时间不单独证明映射」（依据必填
// 的结构防线，AT-SA-104/105）与 `AT-SA-111`「付款失败事实不形成收款或核销」。
func TestMappingNeedsExplicitBasisBeyondCoincidence(t *testing.T) {
	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 8000)

	mapping := mappingFor(t, fact, "mapping-1", domain.TargetPayable, "payable-1")
	if mapping.TargetKind() != domain.TargetPayable {
		t.Fatalf("target kind = %q", mapping.TargetKind())
	}

	t.Run("a mapping without an explicit basis is refused", func(t *testing.T) {
		if _, err := domain.MapFundsToTarget(
			fact,
			settlementValue(t, domain.NewMappingReference, "mapping-x"),
			domain.TargetPayable,
			settlementValue(t, domain.NewSettlementTargetReference, "payable-1"),
			domain.MappingBasisReference{},
			fundsOccurredAt.Add(30*time.Minute),
		); !errors.Is(err, domain.ErrInvalidFundsMapping) {
			t.Fatalf("error = %v; 金额巧合顶替了显式依据", err)
		}
	})

	t.Run("a failed payment cannot fund a mapping", func(t *testing.T) {
		failed := adoptedFact(t, domain.FundsPaymentFailed, 8000)
		if _, err := domain.MapFundsToTarget(
			failed,
			settlementValue(t, domain.NewMappingReference, "mapping-y"),
			domain.TargetPayable,
			settlementValue(t, domain.NewSettlementTargetReference, "payable-1"),
			settlementValue(t, domain.NewMappingBasisReference, "payment-instruction-1"),
			fundsOccurredAt.Add(30*time.Minute),
		); !errors.Is(err, domain.ErrUnfundableFact) {
			t.Fatalf("error = %v, want ErrUnfundableFact", err)
		}
	})

	t.Run("a returned fact may map to a credit note", func(t *testing.T) {
		returned := adoptedFact(t, domain.FundsReturned, 2000)
		if _, err := domain.MapFundsToTarget(
			returned,
			settlementValue(t, domain.NewMappingReference, "mapping-z"),
			domain.TargetCreditNote,
			settlementValue(t, domain.NewSettlementTargetReference, "credit-note-1"),
			settlementValue(t, domain.NewMappingBasisReference, "return-reference-1"),
			fundsOccurredAt.Add(30*time.Minute),
		); err != nil {
			t.Fatalf("map returned funds: %v（返款映射核销到贷项是 AT-SA-170 的正路）", err)
		}
	})
}

// Covers: `AT-SA-167`「付款 80 分别形成指向应付 100 和贷项 -20 的带方向核销分配，分配
// 合计等于真实付款，不改写为净应付」、`AT-SA-106`「金额严格守恒」、`AT-SA-107/108`
// 「部分核销保留剩余未结」与 `AT-SA-168`「依据缺失只核销唯一可证明范围」。
func TestApplicationConservesAmountsPerAllocation(t *testing.T) {
	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 8000)
	payableMapping := mappingFor(t, fact, "mapping-1", domain.TargetPayable, "payable-1")
	creditMapping := mappingFor(t, fact, "mapping-2", domain.TargetCreditNote, "credit-note-1")
	basis := settlementValue(t, domain.NewApplicationBasisReference, "offset-authority-1")

	application, err := domain.ApplySettlement(
		fact,
		[]domain.FundsMapping{payableMapping, creditMapping},
		[]domain.SettlementAllocation{
			{
				Mapping:     payableMapping.Mapping(),
				TargetKind:  domain.TargetPayable,
				Target:      payableMapping.Target(),
				Direction:   domain.AllocationDebit,
				AmountMinor: 10000,
			},
			{
				Mapping:     creditMapping.Mapping(),
				TargetKind:  domain.TargetCreditNote,
				Target:      creditMapping.Target(),
				Direction:   domain.AllocationCredit,
				AmountMinor: 2000,
			},
		},
		settlementValue(t, domain.NewApplicationReference, "application-1"),
		basis,
		fundsAppliedAt,
	)
	if err != nil {
		t.Fatalf("apply settlement: %v", err)
	}
	// 计数锚定 AT-SA-167 夹具：借 10000 − 贷 2000 = 净 8000 == 付款 8000，剩余 0。
	if application.AppliedMinor() != 8000 || application.RemainderMinor() != 0 {
		t.Fatalf("applied = %d remainder = %d, want 8000/0", application.AppliedMinor(), application.RemainderMinor())
	}
	if len(application.Allocations()) != 2 {
		t.Fatalf("allocations = %d, want 2（两个金额身份保留，不改写为净应付）", len(application.Allocations()))
	}

	t.Run("a partial application keeps the remainder open", func(t *testing.T) {
		partial, err := domain.ApplySettlement(
			fact,
			[]domain.FundsMapping{payableMapping},
			[]domain.SettlementAllocation{{
				Mapping:     payableMapping.Mapping(),
				TargetKind:  domain.TargetPayable,
				Target:      payableMapping.Target(),
				Direction:   domain.AllocationDebit,
				AmountMinor: 5000,
			}},
			settlementValue(t, domain.NewApplicationReference, "application-2"),
			basis,
			fundsAppliedAt,
		)
		if err != nil {
			t.Fatalf("partial application: %v", err)
		}
		if partial.AppliedMinor() != 5000 || partial.RemainderMinor() != 3000 {
			t.Fatalf("applied = %d remainder = %d, want 5000/3000", partial.AppliedMinor(), partial.RemainderMinor())
		}
	})

	t.Run("an allocation without a backing mapping is refused", func(t *testing.T) {
		if _, err := domain.ApplySettlement(
			fact,
			[]domain.FundsMapping{payableMapping},
			[]domain.SettlementAllocation{{
				Mapping:     settlementValue(t, domain.NewMappingReference, "mapping-ghost"),
				TargetKind:  domain.TargetPayable,
				Target:      payableMapping.Target(),
				Direction:   domain.AllocationDebit,
				AmountMinor: 5000,
			}},
			settlementValue(t, domain.NewApplicationReference, "application-x"),
			basis,
			fundsAppliedAt,
		); !errors.Is(err, domain.ErrInvalidApplication) {
			t.Fatalf("error = %v; 没有映射背书的分配就是凭巧合分钱（AT-SA-168）", err)
		}
	})

	t.Run("a net exceeding the fact amount is refused", func(t *testing.T) {
		if _, err := domain.ApplySettlement(
			fact,
			[]domain.FundsMapping{payableMapping},
			[]domain.SettlementAllocation{{
				Mapping:     payableMapping.Mapping(),
				TargetKind:  domain.TargetPayable,
				Target:      payableMapping.Target(),
				Direction:   domain.AllocationDebit,
				AmountMinor: 9000,
			}},
			settlementValue(t, domain.NewApplicationReference, "application-y"),
			basis,
			fundsAppliedAt,
		); !errors.Is(err, domain.ErrApplicationImbalance) {
			t.Fatalf("error = %v, want ErrApplicationImbalance（超额保持未分配）", err)
		}
	})

	t.Run("a cross-currency application stays undecided", func(t *testing.T) {
		if _, err := domain.ApplySettlementInCurrency(
			fact,
			settlementValue(t, domain.NewCurrencyCode, "EUR"),
			[]domain.FundsMapping{payableMapping},
			[]domain.SettlementAllocation{{
				Mapping:     payableMapping.Mapping(),
				TargetKind:  domain.TargetPayable,
				Target:      payableMapping.Target(),
				Direction:   domain.AllocationDebit,
				AmountMinor: 5000,
			}},
			settlementValue(t, domain.NewApplicationReference, "application-z"),
			basis,
			fundsAppliedAt,
		); !errors.Is(err, domain.ErrCrossCurrencyApplication) {
			t.Fatalf("error = %v, want ErrCrossCurrencyApplication（不用当前汇率）", err)
		}
	})
}

// Covers: `AT-SA-112`/`AT-SA-113`「资金退回/付款撤销 → 追加核销撤销，恢复未结金额，
// 不删除原收款引用」——撤销一次、依据必备、原核销与原分配原样保留。
func TestReversalAppendsWithoutDeletingTheOriginal(t *testing.T) {
	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 8000)
	payableMapping := mappingFor(t, fact, "mapping-1", domain.TargetPayable, "payable-1")
	application, err := domain.ApplySettlement(
		fact,
		[]domain.FundsMapping{payableMapping},
		[]domain.SettlementAllocation{{
			Mapping:     payableMapping.Mapping(),
			TargetKind:  domain.TargetPayable,
			Target:      payableMapping.Target(),
			Direction:   domain.AllocationDebit,
			AmountMinor: 8000,
		}},
		settlementValue(t, domain.NewApplicationReference, "application-1"),
		settlementValue(t, domain.NewApplicationBasisReference, "payment-instruction-1"),
		fundsAppliedAt,
	)
	if err != nil {
		t.Fatalf("apply settlement: %v", err)
	}

	reversed, err := application.Reverse(
		settlementValue(t, domain.NewApplicationBasisReference, "funds-returned-1"),
		fundsAppliedAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if _, _, ok := reversed.Reversed(); !ok {
		t.Fatal("撤销没有登记")
	}
	if reversed.Fact() != application.Fact() || len(reversed.Allocations()) != 1 {
		t.Fatal("撤销删掉了原收款引用或原分配")
	}
	if _, _, ok := application.Reversed(); ok {
		t.Fatal("撤销改写了原核销值——值语义破了")
	}

	t.Run("a second reversal is refused", func(t *testing.T) {
		if _, err := reversed.Reverse(
			settlementValue(t, domain.NewApplicationBasisReference, "funds-returned-2"),
			fundsAppliedAt.Add(48*time.Hour),
		); !errors.Is(err, domain.ErrApplicationReversed) {
			t.Fatalf("error = %v, want ErrApplicationReversed", err)
		}
	})

	t.Run("a reversal without a basis is refused", func(t *testing.T) {
		if _, err := application.Reverse(domain.ApplicationBasisReference{}, fundsAppliedAt.Add(24*time.Hour)); !errors.Is(err, domain.ErrInvalidApplication) {
			t.Fatalf("error = %v; 没有依据的撤销与数据丢失无从分辨", err)
		}
	})
}
