package partycommercial

import (
	"context"
	"errors"
	"testing"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	wfdomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type accountStub struct {
	found   bool
	err     error
	tenant  string
	account string
}

func (stub *accountStub) LoadLatestCustomerAccount(
	_ context.Context,
	tenant pcdomain.TenantID,
	account pcdomain.CustomerAccountID,
) (pcdomain.CustomerAccountRegistration, bool, error) {
	stub.tenant = tenant.String()
	stub.account = account.String()
	return pcdomain.CustomerAccountRegistration{}, stub.found, stub.err
}

func TestOwnerAccountLookupReportsPresenceWithoutFoldingAReadFailure(t *testing.T) {
	tenant, err := wfdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := wfdomain.NewOwnerAccountID("SYN-OWNER-01")
	if err != nil {
		t.Fatal(err)
	}

	present := &accountStub{found: true}
	lookup, err := NewOwnerAccountLookup(present)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := lookup.OwnerAccountRegistered(t.Context(), tenant, owner)
	if err != nil || !registered || present.tenant != "SYN-TENANT-01" || present.account != "SYN-OWNER-01" {
		t.Fatalf("在册 = %v（%v），问的是 %q / %q", registered, err, present.tenant, present.account)
	}

	absent := &accountStub{}
	lookup, err = NewOwnerAccountLookup(absent)
	if err != nil {
		t.Fatal(err)
	}
	registered, err = lookup.OwnerAccountRegistered(t.Context(), tenant, owner)
	if err != nil || registered {
		t.Fatalf("不在册 = %v（%v）", registered, err)
	}

	unreadable := &accountStub{err: errors.New("party commercial down")}
	lookup, err = NewOwnerAccountLookup(unreadable)
	if err != nil {
		t.Fatal(err)
	}
	registered, err = lookup.OwnerAccountRegistered(t.Context(), tenant, owner)
	if err == nil || registered {
		t.Fatalf("读不动 = %v（%v），want error", registered, err)
	}

	if _, err := NewOwnerAccountLookup(nil); err == nil {
		t.Fatal("没接上客户账户册也建成了核对口")
	}
}
