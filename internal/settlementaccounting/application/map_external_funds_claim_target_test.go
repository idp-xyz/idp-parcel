package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
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
	log          *[]string
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
	if double.log != nil {
		*double.log = append(*double.log, "claim:"+key.Kind.String()+"/"+key.Target.String())
	}
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
	fixture.claims.register("tenant-1", domain.TargetRecoveryAcknowledgement, "acknowledgement-1", "USD", 6000)
	fixture.claims.register("tenant-2", domain.TargetRecoveryAcknowledgement, "acknowledgement-foreign", "USD", 6000)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	foreign, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-foreign", domain.TargetRecoveryAcknowledgement, "acknowledgement-foreign"))
	mustFunds(t, foreign, err, application.FundsNotAccepted, "映射到别的租户的认可")
	missing, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-missing", domain.TargetRecoveryAcknowledgement, "acknowledgement-never"))
	mustFunds(t, missing, err, application.FundsNotAccepted, "映射到不存在的认可")
	if foreign.ContinuationReference() != missing.ContinuationReference() || foreign.UndecidedReason() != missing.UndecidedReason() {
		t.Fatal("「不属本租户」与「不存在」答得出差别")
	}
	if len(fixture.mappings.records) != 0 {
		t.Fatalf("未受理的映射落了 %d 行", len(fixture.mappings.records))
	}

	mapped, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-recovery", domain.TargetRecoveryAcknowledgement, "acknowledgement-1"))
	mustFunds(t, mapped, err, application.FundsMapped, "映射到本租户的认可")
	partial, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-r1", "mapping-recovery",
		domain.TargetRecoveryAcknowledgement, "acknowledgement-1", 4000))
	mustFunds(t, partial, err, application.SettlementApplied, "部分核销 4000")
	over, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-r2", "mapping-recovery",
		domain.TargetRecoveryAcknowledgement, "acknowledgement-1", 2001))
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
	fixture.claims.register("tenant-1", domain.TargetRecoveryAcknowledgement, "acknowledgement-1", "USD", 5000)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	for _, target := range []struct {
		mapping string
		kind    domain.SettlementTargetKind
		ref     string
	}{
		{"mapping-ack", domain.TargetRecoveryAcknowledgement, "acknowledgement-1"},
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
		{Mapping: "mapping-ack", TargetKind: domain.TargetRecoveryAcknowledgement, Target: "acknowledgement-1", Direction: domain.AllocationDebit, AmountMinor: 1000},
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
	want := []string{"CUSTOMER_COMPENSATION/compensation-a", "CUSTOMER_COMPENSATION/compensation-b", "RECOVERY_ACKNOWLEDGEMENT/acknowledgement-1"}
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

// factLockDouble 照真库口径：事实在册才锁得到，已核销是同一只核销库替身里指向这条事实、未撤销的核销 applied 合计。
type factLockDouble struct {
	facts        *fundsFactStoreDouble
	applications *applicationStoreDouble
	err          error
	log          *[]string
}

func (double *factLockDouble) LockFundsFactApplications(ctx context.Context, key ports.FundsFactKey) (int64, bool, error) {
	if double.err != nil {
		return 0, false, double.err
	}
	*double.log = append(*double.log, "fact:"+key.Fact.String())
	if _, found, _ := double.facts.FindByKey(ctx, key); !found {
		return 0, false, nil
	}
	applied := int64(0)
	for _, record := range double.applications.records {
		if record.Key.TenantID != key.TenantID || record.Application.Fact() != key.Fact {
			continue
		}
		if _, _, reversed := record.Application.Reversed(); reversed {
			continue
		}
		applied += record.Application.AppliedMinor()
	}
	return applied, true, nil
}

// scopeBooksDouble 让登了的对账单收成一个应收作用域，记下作用域锁与推进的先后；没登的对账单不是客户收款这一支。
// 已确认应收给得够大，这里不证已确认未结的边界（那一格在 postgres 用例里对真账证）。
type scopeBooksDouble struct {
	statements map[string]domain.SettlementScope
	log        *[]string
}

func newScopeBooks(log *[]string) *scopeBooksDouble {
	return &scopeBooksDouble{statements: map[string]domain.SettlementScope{}, log: log}
}

func (double *scopeBooksDouble) registerStatement(t *testing.T, statement, account string) {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		fundsRef(t, domain.NewLegalEntityReference, "legal-entity-1"),
		fundsRef(t, domain.NewSettlementAccountID, account),
		fundsRef(t, domain.NewCurrencyCode, "USD"))
	if err != nil {
		t.Fatalf("作用域：%v", err)
	}
	double.statements[statement] = scope
}

func (double *scopeBooksDouble) CustomerStatementScope(
	_ context.Context, _ domain.TenantID, statement string,
) (domain.SettlementScope, bool, error) {
	scope, found := double.statements[statement]
	return scope, found, nil
}

func (double *scopeBooksDouble) ReceivableAccountScope(
	context.Context, domain.TenantID, string,
) (domain.SettlementScope, bool, error) {
	return domain.SettlementScope{}, false, nil
}

func (double *scopeBooksDouble) CustomerDirectionNets(
	context.Context, domain.TenantID, domain.SettlementScope,
) (int64, int64, int64, error) {
	return 0, 0, 1_000_000, nil
}

func (double *scopeBooksDouble) ConfirmedChargeAging(
	context.Context, domain.TenantID, domain.SettlementScope, time.Time,
) (ports.ChargeDuePicture, error) {
	return ports.ChargeDuePicture{}, nil
}

func (double *scopeBooksDouble) LockOperationalBalanceScope(_ context.Context, _ domain.TenantID, scope domain.SettlementScope) error {
	*double.log = append(*double.log, "scope:"+scope.Account().String())
	return nil
}

func (double *scopeBooksDouble) AdvanceOperationalBalance(
	_ context.Context, _ domain.TenantID, scope domain.SettlementScope, _, _ int64, _ time.Time,
) (ports.OperationalBalanceAdvanceOutcome, error) {
	*double.log = append(*double.log, "advance:"+scope.Account().String())
	return ports.OperationalBalanceAdvanced, nil
}

// firstLocks 取锁记录里每把锁第一次出现的先后：同一事务里再取一次是重入，只有头一次取的先后决定会不会成环。
func firstLocks(log []string) []string {
	seen := map[string]bool{}
	var order []string
	for _, entry := range log {
		if strings.HasPrefix(entry, "advance:") || seen[entry] {
			continue
		}
		seen[entry] = true
		order = append(order, entry)
	}
	return order
}

// Covers: 锁序全序——核销先锁事实身份行，再按（格、引用）锁金额本体，再按作用域锁运营余额；撤销取的是这条全序的
// 子序列（事实、作用域）。全仓取这几把锁的路径都按这一个次序，两笔事务不会互等成环。核销与撤销都在自己的事务里推进
// 指向对账单的那几个作用域（ADR-0181 决定一）。
func TestApplyAndReverseTakeLocksInOneTotalOrderAndRecomputeTheirScopes(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-b", "USD", 5000)
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-a", "USD", 5000)
	fixture.books.registerStatement(t, "STMT-B", "account-b")
	fixture.books.registerStatement(t, "STMT-A", "account-a")
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	for _, target := range []struct {
		mapping string
		kind    domain.SettlementTargetKind
		ref     string
	}{
		{"mapping-stmt-b", domain.TargetStatement, "STMT-B"},
		{"mapping-comp-b", domain.TargetCustomerCompensation, "compensation-b"},
		{"mapping-stmt-a", domain.TargetStatement, "STMT-A"},
		{"mapping-comp-a", domain.TargetCustomerCompensation, "compensation-a"},
	} {
		result, err := fixture.handler.Map(ctx, claimMapCommand(t, target.mapping, target.kind, target.ref))
		mustFunds(t, result, err, application.FundsMapped, "映射 "+target.ref)
	}
	*fixture.locks = (*fixture.locks)[:0]

	command := applyCommand(t)
	command.Application = "application-order"
	command.MappingRefs = []string{"mapping-stmt-b", "mapping-comp-b", "mapping-stmt-a", "mapping-comp-a"}
	command.Allocations = nil
	for _, allocation := range []struct {
		mapping string
		kind    domain.SettlementTargetKind
		ref     string
	}{
		{"mapping-stmt-b", domain.TargetStatement, "STMT-B"},
		{"mapping-comp-b", domain.TargetCustomerCompensation, "compensation-b"},
		{"mapping-stmt-a", domain.TargetStatement, "STMT-A"},
		{"mapping-comp-a", domain.TargetCustomerCompensation, "compensation-a"},
	} {
		command.Allocations = append(command.Allocations, application.AllocationDirective{
			Mapping: allocation.mapping, TargetKind: allocation.kind, Target: allocation.ref,
			Direction: domain.AllocationDebit, AmountMinor: 1000,
		})
	}
	result, err := fixture.handler.Apply(ctx, command)
	mustFunds(t, result, err, application.SettlementApplied, "一笔付两条赔付两张对账单")
	want := []string{"fact:bank-receipt-1", "claim:CUSTOMER_COMPENSATION/compensation-a", "claim:CUSTOMER_COMPENSATION/compensation-b",
		"scope:account-a", "scope:account-b"}
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, want) {
		t.Fatalf("核销取锁的先后 = %v，要 %v", got, want)
	}
	if !containsAll(*fixture.locks, "advance:account-a", "advance:account-b") {
		t.Fatalf("核销没在自己的事务里推进两个作用域：%v", *fixture.locks)
	}

	*fixture.locks = (*fixture.locks)[:0]
	tenant, _ := domain.NewTenantID("tenant-1")
	reversed, err := fixture.handler.Reverse(ctx, application.ReverseApplicationCommand{
		TenantID: tenant, Application: "application-order", Basis: "payment-returned-1", ReversedAt: fundsAppliedAt.Add(time.Hour),
	})
	mustFunds(t, reversed, err, application.ApplicationReversedOutcome, "撤销")
	want = []string{"fact:bank-receipt-1", "scope:account-a", "scope:account-b"}
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, want) {
		t.Fatalf("撤销取锁的先后 = %v，要 %v", got, want)
	}
	if !containsAll(*fixture.locks, "advance:account-a", "advance:account-b") {
		t.Fatalf("撤销没在自己的事务里推进两个作用域：%v", *fixture.locks)
	}
}

func containsAll(log []string, entries ...string) bool {
	for _, entry := range entries {
		found := false
		for _, logged := range log {
			if logged == entry {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Covers: 资金退回事实映射或核销到客户赔付、追偿认可两格一律`未受理`：退回走核销撤销，不能记成一笔付款；
// 对账单等既有四格照旧收资金退回。
func TestAFundsReturnedFactNeitherMapsNorAppliesToAClaimAmount(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-1", "USD", 5000)
	returned := adoptCommand(t, domain.FundsReturned)
	if _, err := fixture.handler.AdoptFact(ctx, returned); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	refused, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-compensation", domain.TargetCustomerCompensation, "compensation-1"))
	mustFunds(t, refused, err, application.FundsNotAccepted, "资金退回映射到客户赔付")
	statement, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-statement", domain.TargetStatement, "STMT-2026-08-001"))
	mustFunds(t, statement, err, application.FundsMapped, "资金退回映射到对账单")

	// 库里若已有一条指向赔付的映射（这条规则之前落下的那种），核销也立不成。
	tenant, _ := domain.NewTenantID("tenant-1")
	planted, err := domain.RehydrateFundsMapping(domain.RehydrateFundsMappingSpec{
		Mapping:    fundsRef(t, domain.NewMappingReference, "mapping-legacy"),
		Fact:       fundsRef(t, domain.NewFundsFactReference, "bank-receipt-1"),
		TargetKind: domain.TargetCustomerCompensation,
		Target:     fundsRef(t, domain.NewSettlementTargetReference, "compensation-1"),
		Basis:      fundsRef(t, domain.NewMappingBasisReference, "payment-instruction-1"),
		MappedAt:   fundsMappedAt,
	})
	if err != nil {
		t.Fatalf("预置映射：%v", err)
	}
	legacyKey := ports.FundsMappingKey{TenantID: tenant, Mapping: planted.Mapping()}
	fixture.mappings.records[fundsMappingKey(legacyKey)] = ports.FundsMappingRecord{Key: legacyKey, ContentDigest: "legacy", Mapping: planted}
	applied, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-returned", "mapping-legacy",
		domain.TargetCustomerCompensation, "compensation-1", 1000))
	mustFunds(t, applied, err, application.FundsNotAccepted, "资金退回核销到客户赔付")
	if len(fixture.applications.records) != 0 {
		t.Fatal("资金退回核销到赔付落了行")
	}
}

// Covers: ADR-0029——被指向那一条的已核销已越过它的金额本体（账不平）是确定性的：重交同一份永远同一个结果，续办与作用域
// 已确认未结为负那一格相同，所以编排交回包着 ErrInvalidClaimTargetStanding 的 error 整笔回滚，不折成金额册不可用的未决。
func TestABrokenClaimTargetStandingIsADeterministicBooksError(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.claims.register("tenant-1", domain.TargetCustomerCompensation, "compensation-1", "USD", 5000)
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-compensation", domain.TargetCustomerCompensation, "compensation-1")); err != nil {
		t.Fatalf("map: %v", err)
	}
	fixture.claims.lockErr = fmt.Errorf("lock claim target: %w", domain.ErrInvalidClaimTargetStanding)
	result, err := fixture.handler.Apply(ctx, claimApplyCommand(t, "application-c1", "mapping-compensation",
		domain.TargetCustomerCompensation, "compensation-1", 1000))
	if !errors.Is(err, domain.ErrInvalidClaimTargetStanding) {
		t.Fatalf("账不平的赔付：outcome = %s、err = %v，要包着 ErrInvalidClaimTargetStanding 的 error", result.Outcome(), err)
	}
	if len(fixture.applications.records) != 0 {
		t.Fatal("账不平时落了核销")
	}
}

// Covers: 锁后查过同键之后、落行之前另一位写入方落了同键（同一核销引用、不同事实的两份并发交，各锁各的事实）——落行答
// 「已有」时照锁后那段比内容：异内容答冲突，同内容答已存在；赢家可能在另一条事实上，不报累计未分配。
func TestALostSaveRaceComparesTheWinnerLikeTheLockedCheck(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []struct {
		name    string
		winner  func(application.ApplySettlementCommand) application.ApplySettlementCommand
		outcome application.FundsOutcome
	}{
		{"异内容的赢家", func(command application.ApplySettlementCommand) application.ApplySettlementCommand {
			command.Allocations[0].AmountMinor = 1000
			return command
		}, application.ApplicationConflict},
		{"同内容的赢家", func(command application.ApplySettlementCommand) application.ApplySettlementCommand {
			return command
		}, application.ApplicationExisting},
	} {
		fixture := newFundsFixture(t)
		if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
			t.Fatalf("adopt: %v", err)
		}
		if _, err := fixture.handler.Map(ctx, mapCommand(t)); err != nil {
			t.Fatalf("map: %v", err)
		}
		// 落行前那一刻用同一只编排把赢家先落下：它的内容摘要与这一份同一套算法，比得出同内容还是异内容。
		planted := scenario.winner(applyCommand(t))
		fixture.applications.beforeSave = func() {
			if _, err := fixture.handler.Apply(ctx, planted); err != nil {
				t.Fatalf("落赢家：%v", err)
			}
		}
		result, err := fixture.handler.Apply(ctx, applyCommand(t))
		mustFunds(t, result, err, scenario.outcome, scenario.name)
		if _, reported := result.FactUnappliedMinor(); reported {
			t.Fatalf("%s：输掉落行竞态的那一份报了累计未分配", scenario.name)
		}
	}
}

// Covers: UC-SA-005 金额守恒与 `AT-SA-109` 在事实这一级——一笔事实的未撤销核销合计不超过它当前那一版的金额；答案带
// 这一步之后该事实的累计未分配；撤销把额度还回来。
func TestApplicationsOfOneFactStayWithinItsAmount(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := fixture.handler.Map(ctx, mapCommand(t)); err != nil {
		t.Fatalf("map: %v", err)
	}
	first, err := fixture.handler.Apply(ctx, applyCommand(t))
	mustFunds(t, first, err, application.SettlementApplied, "核销 15000")
	if unapplied, ok := first.FactUnappliedMinor(); !ok || unapplied != 5000 {
		t.Fatalf("核销后事实累计未分配 = %d（%v），要 5000", unapplied, ok)
	}
	second := applyCommand(t)
	second.Application = "application-2"
	second.Allocations[0].AmountMinor = 6000
	over, err := fixture.handler.Apply(ctx, second)
	mustFunds(t, over, err, application.ApplicationImbalanceOutcome, "已核 15000 再核 6000")

	tenant, _ := domain.NewTenantID("tenant-1")
	reversed, err := fixture.handler.Reverse(ctx, application.ReverseApplicationCommand{
		TenantID: tenant, Application: "application-1", Basis: "payment-returned-1", ReversedAt: fundsAppliedAt.Add(time.Hour),
	})
	mustFunds(t, reversed, err, application.ApplicationReversedOutcome, "撤销")
	if unapplied, ok := reversed.FactUnappliedMinor(); !ok || unapplied != 20000 {
		t.Fatalf("撤销后事实累计未分配 = %d（%v），要 20000", unapplied, ok)
	}
	again, err := fixture.handler.Apply(ctx, second)
	mustFunds(t, again, err, application.SettlementApplied, "撤销后再核 6000")
}
