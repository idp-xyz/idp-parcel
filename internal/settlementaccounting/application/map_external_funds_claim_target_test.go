package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// claimTargetsDouble 照真库的口径作答：金额本体按（租户、格、引用）登，已核销净额从同一只核销库替身现算——未撤销的
// 核销里指向这条金额的分配，借加贷减。替身不另记一本「已核销」，否则撤销恢复未核销这一格在应用层就无从断言。
type claimTargetsDouble struct {
	amounts      map[string]claimTargetAmount
	applications *applicationStoreDouble
	findErr      error
	lockErr      error
	locked       []domain.ClaimTargetKey
}

type claimTargetAmount struct {
	currency string
	minor    int64
}

func newClaimTargets(applications *applicationStoreDouble) *claimTargetsDouble {
	return &claimTargetsDouble{amounts: map[string]claimTargetAmount{}, applications: applications}
}

func claimTargetKey(tenant domain.TenantID, key domain.ClaimTargetKey) string {
	return tenant.String() + "|" + key.Kind.String() + "|" + key.Target.String()
}

func (double *claimTargetsDouble) register(tenant string, kind domain.SettlementTargetKind, target, currency string, minor int64) {
	tenantID, _ := domain.NewTenantID(tenant)
	targetRef, _ := domain.NewSettlementTargetReference(target)
	double.amounts[claimTargetKey(tenantID, domain.ClaimTargetKey{Kind: kind, Target: targetRef})] = claimTargetAmount{currency: currency, minor: minor}
}

func (double *claimTargetsDouble) FindClaimTarget(
	_ context.Context, tenant domain.TenantID, key domain.ClaimTargetKey,
) (bool, error) {
	if double.findErr != nil {
		return false, double.findErr
	}
	_, found := double.amounts[claimTargetKey(tenant, key)]
	return found, nil
}

func (double *claimTargetsDouble) LockClaimTarget(
	_ context.Context, tenant domain.TenantID, key domain.ClaimTargetKey,
) (domain.ClaimTargetStanding, bool, error) {
	if double.lockErr != nil {
		return domain.ClaimTargetStanding{}, false, double.lockErr
	}
	double.locked = append(double.locked, key)
	amount, found := double.amounts[claimTargetKey(tenant, key)]
	if !found {
		return domain.ClaimTargetStanding{}, false, nil
	}
	applied := int64(0)
	for _, record := range double.applications.records {
		if record.Key.TenantID != tenant {
			continue
		}
		if _, _, reversed := record.Application.Reversed(); reversed {
			continue
		}
		for _, allocation := range record.Application.Allocations() {
			if allocation.TargetKind != key.Kind || allocation.Target != key.Target {
				continue
			}
			if allocation.Direction == domain.AllocationDebit {
				applied += allocation.AmountMinor
			} else {
				applied -= allocation.AmountMinor
			}
		}
	}
	currency, err := domain.NewCurrencyCode(amount.currency)
	if err != nil {
		return domain.ClaimTargetStanding{}, false, err
	}
	standing, err := domain.NewClaimTargetStanding(currency, amount.minor, applied)
	return standing, err == nil, err
}

func claimMapCommand(t *testing.T, mapping string, kind domain.SettlementTargetKind, target string) application.MapFundsCommand {
	t.Helper()
	command := mapCommand(t)
	command.Mapping = mapping
	command.TargetKind = kind
	command.Target = target
	return command
}

func claimApplyCommand(t *testing.T, application_ string, mapping string, kind domain.SettlementTargetKind, target string, amount int64) application.ApplySettlementCommand {
	t.Helper()
	command := applyCommand(t)
	command.Application = application_
	command.MappingRefs = []string{mapping}
	command.Allocations = []application.AllocationDirective{{
		Mapping: mapping, TargetKind: kind, Target: target, Direction: domain.AllocationDebit, AmountMinor: amount,
	}}
	return command
}

func mustFunds(t *testing.T, result application.FundsResult, err error, want application.FundsOutcome, what string) application.FundsResult {
	t.Helper()
	if err != nil {
		t.Fatalf("%s：%v", what, err)
	}
	if result.Outcome() != want {
		t.Fatalf("%s：outcome = %q，要 %q", what, result.Outcome(), want)
	}
	return result
}

// Covers: `AT-SA-116`「客户赔付义务已形成，匹配的真实付款事实到达 → 形成对应应付的付款映射和核销；不改写赔付金额」与
// UC-SA-007 `AT-SA-154` 的编排面——赔付付款映射到那一条客户赔付；未核销按它自己的金额本体减去已核销净额算，分几次付清，
// 超出那一笔失衡、什么都不落；撤销恢复未核销且原分配原样留着。编排手里没有任何改金额本体的口。
func TestACompensationPaymentMapsAndAppliesAgainstThatCompensationOnly(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-1", "USD", 5000)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	mapping := claimMapCommand(t, "mapping-compensation", domain.TargetCustomerCompensation, "compensation-1")
	mapped, err := fixture.handler.Map(ctx, mapping)
	mustFunds(t, mapped, err, application.FundsMapped, "映射到客户赔付")
	if record, _ := mapped.Mapping(); record.Mapping.TargetKind() != domain.TargetCustomerCompensation ||
		record.Mapping.Target().String() != "compensation-1" {
		t.Fatalf("映射落下的目标 = %s/%s", record.Mapping.TargetKind(), record.Mapping.Target())
	}

	first, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-c1", "mapping-compensation",
		domain.TargetCustomerCompensation, "compensation-1", 3000))
	mustFunds(t, first, err, application.SettlementApplied, "第一次核销 3000")
	original, _ := first.Application()
	if original.Application.RemainderMinor() != 17000 {
		t.Fatalf("事实未分配 = %d，要 17000（20000 − 3000）", original.Application.RemainderMinor())
	}

	second, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-c2", "mapping-compensation",
		domain.TargetCustomerCompensation, "compensation-1", 2000))
	mustFunds(t, second, err, application.SettlementApplied, "第二次核销 2000，恰好付清")

	over, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-c3", "mapping-compensation",
		domain.TargetCustomerCompensation, "compensation-1", 1))
	mustFunds(t, over, err, application.ApplicationImbalanceOutcome, "付清之后再核 1")
	if _, found, _ := fixture.applications.FindByKey(ctx, ports.SettlementApplicationKey{
		TenantID: mapping.TenantID, Application: fundsRef(t, domain.NewApplicationReference, "application-c3"),
	}); found {
		t.Fatal("失衡的那一笔落了核销")
	}

	reversed, err := fixture.handler.Reverse(ctx, application.ReverseApplicationCommand{
		TenantID: mapping.TenantID, Application: "application-c1", Basis: "payment-returned-1",
		ReversedAt: fundsAppliedAt.Add(time.Hour),
	})
	mustFunds(t, reversed, err, application.ApplicationReversedOutcome, "撤销第一次")
	kept, _ := reversed.Application()
	if !reflect.DeepEqual(kept.Application.Allocations(), original.Application.Allocations()) {
		t.Fatalf("撤销后分配 = %+v，要原样留着 %+v", kept.Application.Allocations(), original.Application.Allocations())
	}

	again, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-c4", "mapping-compensation",
		domain.TargetCustomerCompensation, "compensation-1", 3000))
	mustFunds(t, again, err, application.SettlementApplied, "撤销恢复的 3000 再核")
}

// Covers: `AT-SA-117`「追偿应收已形成……到账后由本用例形成收款映射和部分/完整核销，不改写原追偿金额」与 UC-SA-007
// `AT-SA-155` 的编排面——追偿到账映射到那一条追偿认可；指向不存在的与指向别的租户的同答`未受理`、什么都不落（ADR-0029：
// 两者恢复动作相同，分开答就是回答了调用方无权问的那件事）；部分核销留下剩余未结。
func TestARecoveryReceiptMapsOnlyToAnAcknowledgementInItsOwnTenant(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.claims.register("tenant-1", domain.TargetRecoveryReceivable, "acknowledgement-1", "USD", 6000)
	fixture.claims.register("tenant-2", domain.TargetRecoveryReceivable, "acknowledgement-foreign", "USD", 6000)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	foreign, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-foreign", domain.TargetRecoveryReceivable, "acknowledgement-foreign"))
	mustFunds(t, foreign, err, application.FundsNotAccepted, "映射到别的租户的认可")
	missing, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-missing", domain.TargetRecoveryReceivable, "acknowledgement-never"))
	mustFunds(t, missing, err, application.FundsNotAccepted, "映射到不存在的认可")
	if foreign.ContinuationReference() != missing.ContinuationReference() || foreign.UndecidedReason() != missing.UndecidedReason() {
		t.Fatal("「不属本租户」与「不存在」答得出差别")
	}
	if len(fixture.mappings.records) != 0 {
		t.Fatalf("未受理的映射落了 %d 行", len(fixture.mappings.records))
	}

	mapped, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-recovery", domain.TargetRecoveryReceivable, "acknowledgement-1"))
	mustFunds(t, mapped, err, application.FundsMapped, "映射到本租户的认可")
	partial, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-r1", "mapping-recovery",
		domain.TargetRecoveryReceivable, "acknowledgement-1", 4000))
	mustFunds(t, partial, err, application.SettlementApplied, "部分核销 4000")
	over, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-r2", "mapping-recovery",
		domain.TargetRecoveryReceivable, "acknowledgement-1", 2001))
	mustFunds(t, over, err, application.ApplicationImbalanceOutcome, "剩 2000 却核 2001")
}

// Covers: `AT-SA-110` 落在金额本体上——币种以被指向的那一条为准，不以命令自报的目标币种为准：付款 USD、赔付 EUR，命令
// 写 USD 也停在待判断，什么都不落。
func TestAClaimTargetInAnotherCurrencyStaysUndecidedWhateverTheCommandSays(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-eur", "EUR", 5000)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	mapped, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-eur", domain.TargetCustomerCompensation, "compensation-eur"))
	mustFunds(t, mapped, err, application.FundsMapped, "映射本身不看币种")
	cross, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-eur", "mapping-eur",
		domain.TargetCustomerCompensation, "compensation-eur", 1000))
	mustFunds(t, cross, err, application.CrossCurrencyOutcome, "付款与赔付币种不同")
	if len(fixture.applications.records) != 0 {
		t.Fatal("跨币种的核销落了行")
	}
}

// Covers: 锁序——一次核销同时付两条赔付、收一条认可，按（格、引用）的顺序逐条锁；对账单那一片不进锁。
func TestClaimTargetsAreLockedInOneOrder(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-b", "USD", 5000)
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-a", "USD", 5000)
	fixture.claims.register("tenant-1", domain.TargetRecoveryReceivable, "acknowledgement-1", "USD", 5000)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	for _, target := range []struct {
		mapping string
		kind    domain.SettlementTargetKind
		ref     string
	}{
		{"mapping-ack", domain.TargetRecoveryReceivable, "acknowledgement-1"},
		{"mapping-b", domain.TargetCustomerCompensation, "compensation-b"},
		{"mapping-a", domain.TargetCustomerCompensation, "compensation-a"},
		{"mapping-1", domain.TargetStatement, "STMT-2026-08-001"},
	} {
		result, err := fixture.handler.Map(ctx, claimMapCommand(t, target.mapping, target.kind, target.ref))
		mustFunds(t, result, err, application.FundsMapped, "映射 "+target.ref)
	}
	command := applyCommand(t)
	command.Application = "application-many"
	command.MappingRefs = []string{"mapping-ack", "mapping-b", "mapping-a", "mapping-1"}
	command.Allocations = []application.AllocationDirective{
		{Mapping: "mapping-ack", TargetKind: domain.TargetRecoveryReceivable, Target: "acknowledgement-1", Direction: domain.AllocationDebit, AmountMinor: 1000},
		{Mapping: "mapping-b", TargetKind: domain.TargetCustomerCompensation, Target: "compensation-b", Direction: domain.AllocationDebit, AmountMinor: 1000},
		{Mapping: "mapping-1", TargetKind: domain.TargetStatement, Target: "STMT-2026-08-001", Direction: domain.AllocationDebit, AmountMinor: 1000},
		{Mapping: "mapping-a", TargetKind: domain.TargetCustomerCompensation, Target: "compensation-a", Direction: domain.AllocationDebit, AmountMinor: 1000},
	}
	result, err := fixture.handler.Apply(ctx, command)
	mustFunds(t, result, err, application.SettlementApplied, "一笔付三条金额加一张对账单")
	var order []string
	for _, key := range fixture.claims.locked {
		order = append(order, key.Kind.String()+"/"+key.Target.String())
	}
	want := []string{"CUSTOMER_COMPENSATION/compensation-a", "CUSTOMER_COMPENSATION/compensation-b", "RECOVERY_RECEIVABLE/acknowledgement-1"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("锁序 = %v，要 %v", order, want)
	}
}

// Covers: ADR-0029 在两格目标上的恢复面——金额册读不回是依赖故障，答未决并指名等的是金额册；不折成`未受理`，也不
// 落行。
func TestAClaimTargetStoreFailureIsUndecidedNamingTheTargetStore(t *testing.T) {
	ctx := context.Background()

	mapping := newFundsFixture(t)
	if _, err := mapping.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	mapping.claims.findErr = errors.New("claim books down")
	result, err := mapping.handler.Map(ctx, claimMapCommand(t, "mapping-compensation", domain.TargetCustomerCompensation, "compensation-1"))
	mustFunds(t, result, err, application.FundsUndecided, "映射时金额册不可用")
	if result.UndecidedReason() != application.ClaimTargetStoreUnavailable || result.ContinuationReference() == "" {
		t.Fatalf("reason = %q continuation = %q", result.UndecidedReason(), result.ContinuationReference())
	}

	apply := newFundsFixture(t)
	apply.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-1", "USD", 5000)
	if _, err := apply.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := apply.handler.Map(ctx, claimMapCommand(t, "mapping-compensation", domain.TargetCustomerCompensation, "compensation-1")); err != nil {
		t.Fatalf("map: %v", err)
	}
	apply.claims.lockErr = errors.New("claim books down")
	result, err = apply.handler.Apply(ctx, claimApplyCommand(t, "application-c1", "mapping-compensation",
		domain.TargetCustomerCompensation, "compensation-1", 1000))
	mustFunds(t, result, err, application.FundsUndecided, "核销时金额册不可用")
	if result.UndecidedReason() != application.ClaimTargetStoreUnavailable {
		t.Fatalf("reason = %q", result.UndecidedReason())
	}
	if len(apply.applications.records) != 0 {
		t.Fatal("未决的核销落了行")
	}
}

// Covers: ADR-0029 在核销交接口上的那一格——核销或撤销信封被框架确定性拒收，编排不答`已核销`/`已撤销`也不折成续办引用，
// 原样交回错误让事务壳回滚：同一份重投永远同一个结果，续办引用会让「重跑补发」恒假。依赖故障那一格照旧留续办引用。
func TestARejectedApplicationEnvelopeIsReturnedAsAnErrorNotAContinuation(t *testing.T) {
	ctx := context.Background()
	rejected := fmt.Errorf("enqueue: %w", ports.ErrSettlementApplicationHandoffRejected)

	fixture := newFundsFixture(t)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := fixture.handler.Map(ctx, mapCommand(t)); err != nil {
		t.Fatalf("map: %v", err)
	}
	fixture.handoff.err = rejected
	if _, err := fixture.handler.Apply(ctx, applyCommand(t)); !errors.Is(err, ports.ErrSettlementApplicationHandoffRejected) {
		t.Fatalf("核销信封被拒：err = %v，要 errors.Is ErrSettlementApplicationHandoffRejected", err)
	}
	if _, err := fixture.handler.Apply(ctx, applyCommand(t)); !errors.Is(err, ports.ErrSettlementApplicationHandoffRejected) {
		t.Fatalf("重放同一份：err = %v，不该答已核销", err)
	}

	fixture.handoff.err = nil
	if _, err := fixture.handler.Apply(ctx, applyCommand(t)); err != nil {
		t.Fatalf("交接恢复后重放：%v", err)
	}
	fixture.handoff.err = rejected
	tenant, _ := domain.NewTenantID("tenant-1")
	if _, err := fixture.handler.Reverse(ctx, application.ReverseApplicationCommand{
		TenantID: tenant, Application: "application-1", Basis: "payment-returned-1", ReversedAt: fundsAppliedAt.Add(time.Hour),
	}); !errors.Is(err, ports.ErrSettlementApplicationHandoffRejected) {
		t.Fatalf("撤销信封被拒：err = %v", err)
	}
}

func fundsRef[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}
