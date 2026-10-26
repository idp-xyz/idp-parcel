package application_test

import (
	"context"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: 锁序全序在多作用域映射上的落法（ADR-0283）——映到对账单或账户的映射先锁事实，再按（法人、账户、币种）键序锁
// 这条事实落到的全部客户应收作用域，与映射目标是哪一个、读口按什么次序交回都无关；每个作用域都在这一笔里推进。替身
// 故意倒序交回作用域，第三次映射的目标又不在键序之首，漏了排序或先锁目标都会红。映到审核应付的映射不碰客户应收作用域，
// 一把锁也不取。
func TestMappingIntoSeveralScopesLocksTheFactThenEveryScopeInKeyOrder(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.books.registerStatement(t, "STMT-C", "account-c")
	fixture.books.registerStatement(t, "STMT-A", "account-a")
	fixture.books.registerAccount(t, "account-b")
	if _, err := fixture.handler.AdoptFact(ctx, adoptCommand(t, domain.FundsReceiptConfirmed)); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	*fixture.locks = (*fixture.locks)[:0]
	result, err := fixture.handler.Map(ctx, claimMapCommand(t, "mapping-payable", domain.TargetPayable, "PAYABLE-1"))
	mustFunds(t, result, err, application.FundsMapped, "映到审核应付")
	if len(*fixture.locks) != 0 {
		t.Fatalf("映到审核应付取了锁：%v", *fixture.locks)
	}
	if _, posted := result.FactScopePosting(); posted {
		t.Fatal("映到审核应付的答复带了客户应收作用域的记入方式")
	}

	for _, target := range []struct {
		mapping string
		kind    domain.SettlementTargetKind
		ref     string
	}{
		{"mapping-stmt-c", domain.TargetStatement, "STMT-C"},
		{"mapping-stmt-a", domain.TargetStatement, "STMT-A"},
	} {
		result, err := fixture.handler.Map(ctx, claimMapCommand(t, target.mapping, target.kind, target.ref))
		mustFunds(t, result, err, application.FundsMapped, "映射 "+target.ref)
	}

	*fixture.locks = (*fixture.locks)[:0]
	result, err = fixture.handler.Map(ctx, claimMapCommand(t, "mapping-account-b", domain.TargetAccount, "account-b"))
	mustFunds(t, result, err, application.FundsMapped, "映到账户 account-b")
	want := []string{"fact:bank-receipt-1", "scope:account-a", "scope:account-b", "scope:account-c"}
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, want) {
		t.Fatalf("映射取锁的先后 = %v，要 %v", got, want)
	}
	if !containsAll(*fixture.locks, "advance:account-a", "advance:account-b", "advance:account-c") {
		t.Fatalf("映射没在自己的事务里推进这条事实落到的全部作用域：%v", *fixture.locks)
	}
	posting, posted := result.FactScopePosting()
	if !posted || posting.Scopes != 3 || !posting.ByApplication() {
		t.Fatalf("记入方式 = %+v（%v），要 3 个作用域、按核销记入", posting, posted)
	}
	if unapplied, known := result.FactUnappliedMinor(); !known || unapplied != 20000 {
		t.Fatalf("映射答复的累计未分配 = %d（%v），要 20000", unapplied, known)
	}
}
