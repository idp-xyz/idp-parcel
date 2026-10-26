package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: `AT-SA-116`、`AT-SA-117` 的目标面——客户赔付与追偿认可两格是可映射目标，名字与库面 CHECK、批文词表同字；
// 账户与费用之外的既有四格不动。
func TestClaimAmountTargetsAreMappableKinds(t *testing.T) {
	for kind, name := range map[domain.SettlementTargetKind]string{
		domain.TargetCustomerCompensation: "CUSTOMER_COMPENSATION",
		domain.TargetRecoveryReceivable:   "RECOVERY_RECEIVABLE",
	} {
		if kind.String() != name {
			t.Fatalf("%d.String() = %q，要 %q", kind, kind.String(), name)
		}
		if !kind.PointsAtClaimAmount() {
			t.Fatalf("%s 没被认成指向金额本体的一格", name)
		}
		fact := adoptedFact(t, domain.FundsReceiptConfirmed, 10000)
		if _, err := domain.MapFundsToTarget(fact,
			settlementValue(t, domain.NewMappingReference, "mapping-"+name), kind,
			settlementValue(t, domain.NewSettlementTargetReference, "amount-1"),
			settlementValue(t, domain.NewMappingBasisReference, "payment-instruction-1"),
			fundsAppliedAt); err != nil {
			t.Fatalf("映射到 %s 被拒：%v", name, err)
		}
	}
	for _, kind := range []domain.SettlementTargetKind{
		domain.TargetStatement, domain.TargetPayable, domain.TargetCreditNote, domain.TargetAccount,
	} {
		if kind.PointsAtClaimAmount() {
			t.Fatalf("%s 被认成了金额本体", kind)
		}
	}
	if domain.SettlementTargetKind(domain.TargetRecoveryReceivable+1).String() != "" {
		t.Fatal("封闭集合外多出一格带名字的目标")
	}
}

// Covers: UC-SA-005「默认不做应收应付净额抵销」在两格金额本体上的落法——指向它们的分配只能是借向；贷向（拿这条金额
// 去抵别的账）在形成门与读回门都立不成。
func TestAClaimAmountTargetTakesOnlyDebitAllocations(t *testing.T) {
	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 10000)
	statement := mappingFor(t, fact, "mapping-statement", domain.TargetStatement, "STMT-1")
	compensation := mappingFor(t, fact, "mapping-compensation", domain.TargetCustomerCompensation, "compensation-1")
	_, err := domain.ApplySettlement(fact,
		[]domain.FundsMapping{statement, compensation},
		[]domain.SettlementAllocation{
			{Mapping: statement.Mapping(), TargetKind: domain.TargetStatement, Target: statement.Target(),
				Direction: domain.AllocationDebit, AmountMinor: 9000},
			{Mapping: compensation.Mapping(), TargetKind: domain.TargetCustomerCompensation, Target: compensation.Target(),
				Direction: domain.AllocationCredit, AmountMinor: 3000},
		},
		settlementValue(t, domain.NewApplicationReference, "application-net"),
		settlementValue(t, domain.NewApplicationBasisReference, "offset-instruction-1"),
		fundsAppliedAt)
	if !errors.Is(err, domain.ErrInvalidApplication) {
		t.Fatalf("赔付贷向抵应收：err = %v，要 ErrInvalidApplication", err)
	}

	_, err = domain.RehydrateSettlementApplication(domain.RehydrateSettlementApplicationSpec{
		Application:  settlementValue(t, domain.NewApplicationReference, "application-net"),
		Fact:         fact.Fact(),
		Currency:     settlementValue(t, domain.NewCurrencyCode, "USD"),
		FactMinor:    10000,
		AppliedMinor: 6000,
		Allocations: []domain.SettlementAllocation{
			{Mapping: statement.Mapping(), TargetKind: domain.TargetStatement, Target: statement.Target(),
				Direction: domain.AllocationDebit, AmountMinor: 9000},
			{Mapping: compensation.Mapping(), TargetKind: domain.TargetCustomerCompensation, Target: compensation.Target(),
				Direction: domain.AllocationCredit, AmountMinor: 3000},
		},
		Basis:     settlementValue(t, domain.NewApplicationBasisReference, "offset-instruction-1"),
		AppliedAt: fundsAppliedAt,
	})
	if !errors.Is(err, domain.ErrInvalidApplication) {
		t.Fatalf("读回门放过了指向金额本体的贷向分配：err = %v", err)
	}
}

// Covers: 「核销余额按目标各自算」——一次核销分给同一条金额本体的多片按目标合计，按（格、引用）排序交回：锁按这个
// 顺序取，两笔并发核销不会互等成环；对账单与应付这几格不在其中。
func TestClaimTargetDemandsAreSummedPerTargetInLockOrder(t *testing.T) {
	fact := adoptedFact(t, domain.FundsReceiptConfirmed, 10000)
	recovery := mappingFor(t, fact, "mapping-recovery", domain.TargetRecoveryReceivable, "acknowledgement-1")
	compensationB := mappingFor(t, fact, "mapping-compensation-b", domain.TargetCustomerCompensation, "compensation-b")
	compensationA := mappingFor(t, fact, "mapping-compensation-a", domain.TargetCustomerCompensation, "compensation-a")
	payable := mappingFor(t, fact, "mapping-payable", domain.TargetPayable, "payable-1")
	debit := func(mapping domain.FundsMapping, amount int64) domain.SettlementAllocation {
		return domain.SettlementAllocation{Mapping: mapping.Mapping(), TargetKind: mapping.TargetKind(),
			Target: mapping.Target(), Direction: domain.AllocationDebit, AmountMinor: amount}
	}
	application, err := domain.ApplySettlement(fact,
		[]domain.FundsMapping{recovery, compensationB, compensationA, payable},
		[]domain.SettlementAllocation{
			debit(recovery, 1000), debit(compensationB, 2000), debit(payable, 500),
			debit(compensationA, 1500), debit(compensationB, 700),
		},
		settlementValue(t, domain.NewApplicationReference, "application-1"),
		settlementValue(t, domain.NewApplicationBasisReference, "payment-instruction-1"),
		fundsAppliedAt)
	if err != nil {
		t.Fatalf("核销：%v", err)
	}

	demands := application.ClaimTargetDemands()
	want := []struct {
		kind   domain.SettlementTargetKind
		target string
		amount int64
	}{
		{domain.TargetCustomerCompensation, "compensation-a", 1500},
		{domain.TargetCustomerCompensation, "compensation-b", 2700},
		{domain.TargetRecoveryReceivable, "acknowledgement-1", 1000},
	}
	if len(demands) != len(want) {
		t.Fatalf("demands = %+v，要 %d 条", demands, len(want))
	}
	for index, expected := range want {
		got := demands[index]
		if got.Key.Kind != expected.kind || got.Key.Target.String() != expected.target || got.AmountMinor != expected.amount {
			t.Fatalf("demands[%d] = %s/%s %d，要 %s/%s %d", index,
				got.Key.Kind, got.Key.Target, got.AmountMinor, expected.kind, expected.target, expected.amount)
		}
	}
}

// Covers: `AT-SA-108`、`AT-SA-109`、`AT-SA-110` 落在金额本体目标上——未核销是这条金额本体减去指向它、尚未撤销的
// 核销分配净额；本次分配不超过它即可落，超出即失衡（超额保持未分配，不吞进金额）；币种与付款不同停在待判断。
func TestAClaimTargetStandingAdmitsOnlyWithinItsOwnRemainder(t *testing.T) {
	usd := settlementValue(t, domain.NewCurrencyCode, "USD")
	eur := settlementValue(t, domain.NewCurrencyCode, "EUR")
	standing, err := domain.NewClaimTargetStanding(usd, 5000, 3000)
	if err != nil {
		t.Fatalf("构造未核销面：%v", err)
	}
	if standing.RemainingMinor() != 2000 {
		t.Fatalf("remaining = %d，要 2000", standing.RemainingMinor())
	}
	if err := standing.Admit(usd, 2000); err != nil {
		t.Fatalf("恰好核完被拒：%v", err)
	}
	if err := standing.Admit(usd, 2001); !errors.Is(err, domain.ErrApplicationImbalance) {
		t.Fatalf("超出未核销：err = %v，要 ErrApplicationImbalance", err)
	}
	if err := standing.Admit(eur, 100); !errors.Is(err, domain.ErrCrossCurrencyApplication) {
		t.Fatalf("币种不同：err = %v，要 ErrCrossCurrencyApplication", err)
	}

	for name, build := range map[string]func() error{
		"金额本体非正":    func() error { _, err := domain.NewClaimTargetStanding(usd, 0, 0); return err },
		"已核销为负":     func() error { _, err := domain.NewClaimTargetStanding(usd, 5000, -1); return err },
		"已核销超过金额本体": func() error { _, err := domain.NewClaimTargetStanding(usd, 5000, 5001); return err },
		"币种缺席":      func() error { _, err := domain.NewClaimTargetStanding(domain.CurrencyCode{}, 5000, 0); return err },
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidClaimTargetStanding) {
			t.Fatalf("%s：err = %v，要 ErrInvalidClaimTargetStanding", name, err)
		}
	}
}
