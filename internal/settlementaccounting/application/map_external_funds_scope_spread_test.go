package application_test

import (
	"context"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: 映射的重算集合与锁序（ADR-0283）——映到对账单或账户的映射先锁事实，再按（法人、账户、币种）键序锁这一笔改了
// P 的作用域：一条收款由这一笔从一个作用域变成两个时，目标与原作用域都锁、都推进，次序与谁是目标无关（目标排在键序
// 之后，先锁目标就红；替身倒序交回作用域，漏了排序也红）；两个以上再多一个时只碰目标。映到审核应付的映射不碰客户
// 应收作用域，一把锁也不取。
func TestMappingLocksTheFactThenOnlyTheScopesItMovesInKeyOrder(t *testing.T) {
	fixture := newFundsFixture(t)
	ctx := context.Background()
	fixture.books.registerStatement(t, "STMT-A", "account-a")
	fixture.books.registerAccount(t, "account-b")
	fixture.books.registerAccount(t, "account-c")
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

	result, err = fixture.handler.Map(ctx, claimMapCommand(t, "mapping-stmt-a", domain.TargetStatement, "STMT-A"))
	mustFunds(t, result, err, application.FundsMapped, "映到 STMT-A")
	if posting, posted := result.FactScopePosting(); !posted || posting.Scopes != 1 || posting.ByApplication() {
		t.Fatalf("单作用域的记入方式 = %+v（%v）", posting, posted)
	}

	*fixture.locks = (*fixture.locks)[:0]
	result, err = fixture.handler.Map(ctx, claimMapCommand(t, "mapping-account-c", domain.TargetAccount, "account-c"))
	mustFunds(t, result, err, application.FundsMapped, "映到账户 account-c")
	want := []string{"fact:bank-receipt-1", "scope:account-a", "scope:account-c"}
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, want) {
		t.Fatalf("一个变两个的映射取锁的先后 = %v，要 %v", got, want)
	}
	if !containsAll(*fixture.locks, "advance:account-a", "advance:account-c") {
		t.Fatalf("一个变两个的映射没在自己的事务里推进原作用域与目标：%v", *fixture.locks)
	}
	posting, posted := result.FactScopePosting()
	if !posted || posting.Scopes != 2 || !posting.ByApplication() {
		t.Fatalf("记入方式 = %+v（%v），要 2 个作用域、按核销记入", posting, posted)
	}
	if unapplied, known := result.FactUnappliedMinor(); !known || unapplied != 20000 {
		t.Fatalf("映射答复的累计未分配 = %d（%v），要 20000", unapplied, known)
	}

	*fixture.locks = (*fixture.locks)[:0]
	result, err = fixture.handler.Map(ctx, claimMapCommand(t, "mapping-account-b", domain.TargetAccount, "account-b"))
	mustFunds(t, result, err, application.FundsMapped, "映到账户 account-b")
	want = []string{"fact:bank-receipt-1", "scope:account-b"}
	if got := firstLocks(*fixture.locks); !reflect.DeepEqual(got, want) {
		t.Fatalf("两个变三个的映射取锁的先后 = %v，要 %v——既有作用域的 P 不变，不碰", got, want)
	}
	if posting, _ := result.FactScopePosting(); posting.Scopes != 3 {
		t.Fatalf("记入方式 = %+v，要 3 个作用域", posting)
	}
}
