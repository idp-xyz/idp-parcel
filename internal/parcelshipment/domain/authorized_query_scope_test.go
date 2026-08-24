package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func queryScope(t *testing.T, reference, tenant string, accounts ...string) domain.AuthorizedQueryScope {
	t.Helper()
	ids := make([]domain.CustomerAccountID, len(accounts))
	for index, account := range accounts {
		ids[index] = mustValue(t, domain.NewCustomerAccountID, account)
	}
	scope, err := domain.NewAuthorizedQueryScope(
		mustValue(t, domain.NewQueryScopeReference, reference),
		mustValue(t, domain.NewTenantID, tenant),
		ids,
	)
	if err != nil {
		t.Fatalf("new authorized query scope: %v", err)
	}
	return scope
}

// Covers: CONTEXT「授权查询作用域」——由共享身份/授权能力已经判断并传入的可见范围引用，
// 本上下文只消费：引用、租户与可见客户账户三样都要能读回，过滤才有键可用。
func TestAnAuthorizedQueryScopeCarriesItsReferenceTenantAndAccounts(t *testing.T) {
	scope := queryScope(t, "SCOPE-GRANT-1", "TENANT-1", "CUST-1", "CUST-2")

	if scope.Reference().String() != "SCOPE-GRANT-1" {
		t.Fatalf("reference = %q, want SCOPE-GRANT-1", scope.Reference())
	}
	if scope.TenantID().String() != "TENANT-1" {
		t.Fatalf("tenant = %q, want TENANT-1", scope.TenantID())
	}
	accounts := scope.CustomerAccountIDs()
	if len(accounts) != 2 || accounts[0].String() != "CUST-1" || accounts[1].String() != "CUST-2" {
		t.Fatalf("accounts = %v, want [CUST-1 CUST-2]", accounts)
	}
}

// Covers: 作用域是共享授权能力交来的既成判断，本上下文原样消费——读方拿到的切片被改动
// 不得写回作用域本体，否则一次展示层的排序就替授权能力扩了权。
func TestAQueryScopeHandsOutCopiesOfItsAccounts(t *testing.T) {
	scope := queryScope(t, "SCOPE-GRANT-1", "TENANT-1", "CUST-1", "CUST-2")

	leaked := scope.CustomerAccountIDs()
	leaked[0] = mustValue(t, domain.NewCustomerAccountID, "CUST-INTRUDER")

	if scope.CustomerAccountIDs()[0].String() != "CUST-1" {
		t.Fatal("改动读出的切片改写了作用域本体")
	}
}

// Covers: CONTEXT 规则「查询只消费共享身份/授权能力传入的授权查询作用域」——缺任何一段的
// 作用域立不起来：没有引用无从留痕，没有租户越过最高隔离边界（ADR-0003），没有客户账户则
// 「可见范围」为空——授权能力答「什么都看不见」时该在接入处拒绝，不该造出一个查询作用域。
func TestAQueryScopeRequiresItsParts(t *testing.T) {
	reference := mustValue(t, domain.NewQueryScopeReference, "SCOPE-GRANT-1")
	tenant := mustValue(t, domain.NewTenantID, "TENANT-1")
	account := mustValue(t, domain.NewCustomerAccountID, "CUST-1")

	if _, err := domain.NewQueryScopeReference(" "); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空白作用域引用 err = %v, want ErrBlankValue", err)
	}
	if _, err := domain.NewAuthorizedQueryScope(domain.QueryScopeReference{}, tenant,
		[]domain.CustomerAccountID{account}); err == nil {
		t.Fatal("零值作用域引用不该立得起来")
	}
	if _, err := domain.NewAuthorizedQueryScope(reference, domain.TenantID{},
		[]domain.CustomerAccountID{account}); err == nil {
		t.Fatal("零值租户不该立得起来")
	}
	if _, err := domain.NewAuthorizedQueryScope(reference, tenant, nil); err == nil {
		t.Fatal("没有任何可见客户账户的作用域不该立得起来")
	}
	if _, err := domain.NewAuthorizedQueryScope(reference, tenant,
		[]domain.CustomerAccountID{account, {}}); err == nil {
		t.Fatal("含零值客户账户的作用域不该立得起来")
	}
	if _, err := domain.NewAuthorizedQueryScope(reference, tenant,
		[]domain.CustomerAccountID{account, account}); err == nil {
		t.Fatal("重复客户账户不该静默通过——去重是授权能力的事，重复说明它交来的判断坏了")
	}
}
