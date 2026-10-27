package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func principalMapping(
	fact domain.ExternalFundsFact,
	target string,
	t *testing.T,
) (domain.FundsMapping, error) {
	t.Helper()
	return domain.MapFundsToTarget(
		fact,
		settlementValue(t, domain.NewMappingReference, "mapping-principal-1"),
		domain.TargetCollectedPrincipal,
		settlementValue(t, domain.NewSettlementTargetReference, target),
		settlementValue(t, domain.NewMappingBasisReference, "channel-remittance-advice-1"),
		fundsOccurredAt.Add(30*time.Minute),
	)
}

// Covers: AT-SA-216——把一条已采用的外部资金事实认作代收本金——映射的目标就是这条事实自己，整条认作，不按金额分：
// 指名别的目标、或指名的目标不是这条事实，都是映射输入不合法。
func TestACollectedPrincipalMappingNamesTheWholeReceiptFactItself(t *testing.T) {
	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 100000)

	mapping, err := principalMapping(fact, "bank-fact-1", t)
	if err != nil {
		t.Fatalf("map receipt fact to collected principal: %v", err)
	}
	if mapping.TargetKind() != domain.TargetCollectedPrincipal || !mapping.TargetKind().IsCollectedPrincipal() {
		t.Fatalf("target kind = %q", mapping.TargetKind())
	}
	if mapping.TargetKind().String() != "COLLECTED_PRINCIPAL" {
		t.Fatalf("target kind word = %q", mapping.TargetKind().String())
	}

	if _, err := principalMapping(fact, "another-fact", t); !errors.Is(err, domain.ErrInvalidFundsMapping) {
		t.Fatalf("目标不是这条事实自己 error = %v；只认作一部分、或认作别的事实的本金，都不是这条映射的意思", err)
	}
}

// Covers: AT-SA-216——只有确认到账的事实能认作代收本金：资金退回是钱离开，付款失败不是钱。
func TestOnlyAConfirmedReceiptCanBeSetAsideAsCollectedPrincipal(t *testing.T) {
	returned := adoptedFact(t, domain.FundsReturned, 2000)
	if _, err := principalMapping(returned, "bank-fact-1", t); !errors.Is(err, domain.ErrInvalidFundsMapping) {
		t.Fatalf("资金退回认作代收本金 error = %v", err)
	}
	failed := adoptedFact(t, domain.FundsPaymentFailed, 2000)
	if _, err := principalMapping(failed, "bank-fact-1", t); !errors.Is(err, domain.ErrUnfundableFact) {
		t.Fatalf("付款失败认作代收本金 error = %v", err)
	}
}

// Covers: AT-SA-216——认作代收本金的事实不核销到任何结算目标之上：代收本金不是运营企业的应收应付，它的去向在
// collection-remittance 的分户账里，结算一侧没有可核销的对象。
func TestNoApplicationAllocatesToCollectedPrincipal(t *testing.T) {
	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 100000)
	mapping, err := principalMapping(fact, "bank-fact-1", t)
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	_, err = domain.ApplySettlement(
		fact,
		[]domain.FundsMapping{mapping},
		[]domain.SettlementAllocation{{
			Mapping:     mapping.Mapping(),
			TargetKind:  domain.TargetCollectedPrincipal,
			Target:      mapping.Target(),
			Direction:   domain.AllocationDebit,
			AmountMinor: 100000,
		}},
		settlementValue(t, domain.NewApplicationReference, "application-principal-1"),
		settlementValue(t, domain.NewApplicationBasisReference, "offset-authority-1"),
		fundsAppliedAt,
	)
	if !errors.Is(err, domain.ErrInvalidApplication) {
		t.Fatalf("核销到代收本金 error = %v", err)
	}
}
