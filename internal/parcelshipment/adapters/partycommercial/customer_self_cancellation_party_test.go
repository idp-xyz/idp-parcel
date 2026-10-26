package partycommercial_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: AT-PS-146——直客取消的请求方格映射（ADR-0313）：请求方引用正是委托所属客户账户本人才折成「客户」格；别的账户、运营角色或
// 任意写法都「折不出来」，不猜它是不是运营。
func TestCustomerSelfCancellationPartyOnlyFormsTheCustomersOwnAccount(t *testing.T) {
	identity, err := psdomain.NewSourceIdentity(
		mustID(t, psdomain.NewTenantID, "SYN-TENANT-01"), mustID(t, psdomain.NewCustomerAccountID, "SYN-ACCOUNT-01"),
		mustID(t, psdomain.NewSource, "SYN-SOURCE-A"), mustID(t, psdomain.NewSourceRequestKey, "SYN-KEY-1"))
	if err != nil {
		t.Fatal(err)
	}
	own, err := psdomain.NewCustomerAccountCancellationRequester(identity.CustomerAccountID())
	if err != nil {
		t.Fatal(err)
	}
	party, formed, err := adapter.CustomerSelfCancellationParty{}.FormCancellationParty(t.Context(), identity, own)
	if err != nil || !formed || party != pcdomain.DeclaredCustomerCancellation {
		t.Fatalf("本人：party=%v formed=%v err=%v, want 客户格", party, formed, err)
	}
	foreign, _ := psdomain.NewCustomerAccountCancellationRequester(mustID(t, psdomain.NewCustomerAccountID, "SYN-ACCOUNT-02"))
	for name, requester := range map[string]psdomain.CancellationRequesterReference{
		"别的账户": foreign,
		"运营角色": mustID(t, psdomain.NewCancellationRequesterReference, "OPERATOR:SYN-ROLE-1"),
		"裸账户号": mustID(t, psdomain.NewCancellationRequesterReference, "SYN-ACCOUNT-01"),
	} {
		party, formed, err := adapter.CustomerSelfCancellationParty{}.FormCancellationParty(t.Context(), identity, requester)
		if err != nil || formed || party != pcdomain.DeclaredCancellationPartyInvalid {
			t.Errorf("%s：party=%v formed=%v err=%v, want 折不出来", name, party, formed, err)
		}
	}
}
