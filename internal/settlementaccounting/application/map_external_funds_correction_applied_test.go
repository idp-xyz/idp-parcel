package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 外部更正碰到已有核销的那一格（ADR-0285）：更正先锁事实身份行再读、求未撤销核销合计，照采，新金额低于合计时答复带出超核
// 金额；同一事务里重算这条事实落到的全部客户应收作用域。锁序取资金侧全序（ports.FundsFactApplicationLock 头注）的子序列：
// 事实 → 运营余额作用域（按键序）。

func indexOf(log []string, entry string) int {
	for index, logged := range log {
		if logged == entry {
			return index
		}
	}
	return len(log)
}

// correctedTo 是 correctCommand 换一个新金额。
func correctedTo(t *testing.T, amount int64) application.CorrectFundsFactCommand {
	t.Helper()
	command := correctCommand(t)
	command.AmountMinor = amount
	return command
}

func factKeyOf(t *testing.T, fact string) ports.FundsFactKey {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return ports.FundsFactKey{TenantID: tenant, Fact: fundsRef(t, domain.NewFundsFactReference, fact)}
}

// fundsHandlerWithLock 把夹具里的其余各口原样装上，只换事实锁那一口。
func fundsHandlerWithLock(t *testing.T, fixture *fundsFixture, lock ports.FundsFactApplicationLock) *application.MapExternalFundsHandler {
	t.Helper()
	handler, err := application.NewMapExternalFundsHandler(application.MapExternalFundsDeps{
		Facts:        fixture.facts,
		Mappings:     fixture.mappings,
		Applications: fixture.applications,
		Downstream:   fixture.handoff,
		FactHandoff:  fixture.factHandoff,
		Clock:        fundsClock{at: fundsNowAt},
		Books:        fixture.books,
		Balances:     fixture.books,
		Standings:    idleCreditStanding{},
		ClaimTargets: fixture.claims,
		FactLocks:    lock,
	})
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	return handler
}

// adoptMapAndApply 把 bank-receipt-1（20000）采用、映到对账单 STMT-2026-08-001（落在 account-a）并核销 applied。
func adoptMapAndApply(t *testing.T, fixture *fundsFixture, applied int64) {
	t.Helper()
	ctx := context.Background()
	fixture.books.registerStatement(t, "STMT-2026-08-001", "account-a")
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	mapped, err := fixture.handler.Map(ctx, mapCommand(t))
	mustFunds(t, mapped, err, application.FundsMapped, "映射")
	if applied == 0 {
		return
	}
	command := applyCommand(t)
	command.Allocations[0].AmountMinor = applied
	result, err := fixture.handler.Apply(ctx, command)
	mustFunds(t, result, err, application.SettlementApplied, "核销")
}

// Covers: ADR-0285 决定一——更正低于未撤销核销合计时照采、不拒：新版本落下（链头换成它、再发一封信封），答复带出超核金额
// （合计 15000、新金额 12000 → 3000）；新金额等于合计或高于合计时没有超核，字段为 0。
func TestACorrectionBelowTheAppliedTotalIsAdoptedAndSaysHowMuchIsOverApplied(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount int64
		want   int64
	}{
		{"低于合计", 12_000, 3_000},
		{"恰等于合计", 15_000, 0},
		{"高于合计", 18_000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFundsFixture(t)
			adoptMapAndApply(t, fixture, 15_000)

			result, err := fixture.handler.CorrectFact(context.Background(), correctedTo(t, tc.amount))
			mustFunds(t, result, err, application.FundsFactAdopted, "更正")
			if got := result.OverAppliedMinor(); got != tc.want {
				t.Fatalf("更正到 %d 的超核金额 = %d，要 %d", tc.amount, got, tc.want)
			}
			record, found := result.Fact()
			if !found {
				t.Fatal("更正答复该带已采用的记录")
			}
			if _, minor := record.Fact.Amount(); minor != tc.amount {
				t.Fatalf("新有效版本金额 = %d，要 %d", minor, tc.amount)
			}
			head, found, err := fixture.facts.FindByKey(context.Background(), record.Key)
			if err != nil || !found || head.Fact.Version().String() != "bank-receipt-1/v2" {
				t.Fatalf("链头 = %v found=%v err=%v，要 v2", head.Fact.Version(), found, err)
			}
		})
	}
}

// Covers: ADR-0285 决定一——重放同一次更正答`已存在`时仍带出此刻的超核金额：响应丢了、操作员重交，看到的信号与第一次一致；
// 重放首版（链头已被更正取代）说的是已经过去的金额，不带。
func TestReplayingACorrectionBelowTheAppliedTotalStillSaysHowMuchIsOverApplied(t *testing.T) {
	fixture := newFundsFixture(t)
	adoptMapAndApply(t, fixture, 15_000)
	command := correctedTo(t, 12_000)

	first, err := fixture.handler.CorrectFact(context.Background(), command)
	mustFunds(t, first, err, application.FundsFactAdopted, "更正")
	replay, err := fixture.handler.CorrectFact(context.Background(), command)
	mustFunds(t, replay, err, application.FundsFactExisting, "重放更正")
	if got := replay.OverAppliedMinor(); got != 3_000 {
		t.Fatalf("重放更正的超核金额 = %d，要 3000", got)
	}

	firstVersion, err := fixture.handler.AdoptFact(context.Background(), adoptCommand(t, domain.FundsReceiptConfirmed))
	mustFunds(t, firstVersion, err, application.FundsFactExisting, "重放首版")
	if got := firstVersion.OverAppliedMinor(); got != 0 {
		t.Fatalf("首版重放的超核金额 = %d，要 0——它只在更正答复上说", got)
	}
}

// Covers: ADR-0285 决定三、四——更正先锁事实身份行，再按键序锁事实所及的全部客户应收作用域并各自重算推进（作用域替身故意按键序
// 倒着交回，编排自己排）。不论新金额比原金额高还是低都重算：金额变，收款单作用域的封顶、退回的 −(d+r) 都随之变。
func TestACorrectionLocksTheFactFirstThenRecomputesEveryScopeTheFactReaches(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount int64
	}{
		{"改低", 18_000},
		{"改高", 25_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFundsFixture(t)
			ctx := context.Background()
			fixture.books.registerStatement(t, "STMT-A", "account-a")
			fixture.books.registerStatement(t, "STMT-B", "account-b")
			if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
				t.Fatalf("adopt: %v", err)
			}
			for _, target := range []struct{ mapping, statement string }{{"mapping-b", "STMT-B"}, {"mapping-a", "STMT-A"}} {
				mapped, err := fixture.handler.Map(ctx, claimMapCommand(t, target.mapping, domain.TargetStatement, target.statement))
				mustFunds(t, mapped, err, application.FundsMapped, "映射 "+target.statement)
			}
			*fixture.locks = (*fixture.locks)[:0]

			result, err := fixture.handler.CorrectFact(ctx, correctedTo(t, tc.amount))
			mustFunds(t, result, err, application.FundsFactAdopted, "更正")
			want := []string{"fact:bank-receipt-1", "scope:account-a", "scope:account-b"}
			if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, want) {
				t.Fatalf("更正取锁的先后 = %v，要 %v", got, want)
			}
			if !containsAll(*fixture.locks, "advance:account-a", "advance:account-b") {
				t.Fatalf("更正没在自己的事务里推进事实所及的两个作用域：%v", *fixture.locks)
			}
			// 作用域锁在落版本行与第一次推进之前取齐：随写入隐式取的行锁都落在作用域锁之后（端口头注），一边锁一边推进会让
			// 先推进的那个作用域的行锁挡在后一个作用域咨询锁之前。
			beforeFirstAdvance := (*fixture.locks)[:indexOf(*fixture.locks, "advance:account-a")]
			if !containsAll(beforeFirstAdvance, "scope:account-a", "scope:account-b") {
				t.Fatalf("第一次推进之前没把两个作用域锁取齐：%v", *fixture.locks)
			}
		})
	}
}

// Covers: ADR-0285 决定三——事实还没映射到任何客户应收作用域时更正无账可算：只取事实锁，不取任何作用域锁、不推进。
func TestACorrectionOfAFactMappedToNoCustomerScopeRecomputesNothing(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	*fixture.locks = (*fixture.locks)[:0]

	result, err := fixture.handler.CorrectFact(ctx, correctedTo(t, 12_000))
	mustFunds(t, result, err, application.FundsFactAdopted, "更正")
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, []string{"fact:bank-receipt-1"}) {
		t.Fatalf("没有客户应收作用域时更正取的锁 = %v，要只有事实锁", got)
	}
	if containsAll(*fixture.locks, "advance:account-a") {
		t.Fatalf("没有作用域也推进了余额：%v", *fixture.locks)
	}
}

// Covers: ADR-0285 决定二——更正没被受理（回指非链头）时什么都不落、不锁作用域：作用域锁只为落下的那一版取，空取一把就是
// 白白挡住别的事务。事实锁在读之前取，所以它在册。
func TestACorrectionThatIsNotAcceptedLocksNoScopeAndRecomputesNothing(t *testing.T) {
	fixture := newFundsFixture(t)
	adoptMapAndApply(t, fixture, 15_000)
	*fixture.locks = (*fixture.locks)[:0]

	command := correctedTo(t, 12_000)
	command.Corrects = "bank-receipt-1/v0"
	result, err := fixture.handler.CorrectFact(context.Background(), command)
	mustFunds(t, result, err, application.FundsNotAccepted, "回指非链头")
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, []string{"fact:bank-receipt-1"}) {
		t.Fatalf("未受理的更正取的锁 = %v，要只有事实锁", got)
	}
	if containsAll(*fixture.locks, "advance:account-a") {
		t.Fatalf("未受理的更正推进了余额：%v", *fixture.locks)
	}
}

// Covers: ADR-0029——事实锁读不动才是未决（资金事实库不可用）；事实不在册（锁不到）是确定性的`未受理`。
func TestACorrectionStopsOnTheFactLockAccordingToWhyItCouldNotTakeIt(t *testing.T) {
	fixture := newFundsFixture(t)
	adoptMapAndApply(t, fixture, 0)

	unavailable := &factLockDouble{facts: fixture.facts, applications: fixture.applications, log: fixture.locks,
		err: errors.New("lock unavailable")}
	handler := fundsHandlerWithLock(t, fixture, unavailable)
	result, err := handler.CorrectFact(context.Background(), correctedTo(t, 12_000))
	if err != nil {
		t.Fatalf("锁读不动该折成未决而不是 error：%v", err)
	}
	if result.Outcome() != application.FundsUndecided || result.UndecidedReason() != application.FundsFactStoreUnavailable {
		t.Fatalf("锁读不动：%s/%s，要未决/资金事实库不可用", result.Outcome(), result.UndecidedReason())
	}
	if head, _, _ := fixture.facts.FindByKey(context.Background(), factKeyOf(t, "bank-receipt-1")); head.Fact.Version().String() != "bank-receipt-1/v1" {
		t.Fatalf("锁读不动时版本行还是落了：链头 %s", head.Fact.Version())
	}

	unadopted := correctedTo(t, 12_000)
	unadopted.Fact = "bank-receipt-unknown"
	unadopted.Corrects = "bank-receipt-unknown/v1"
	unadopted.Version = "bank-receipt-unknown/v2"
	missing, err := fixture.handler.CorrectFact(context.Background(), unadopted)
	mustFunds(t, missing, err, application.FundsNotAccepted, "更正没采用的事实")
}
