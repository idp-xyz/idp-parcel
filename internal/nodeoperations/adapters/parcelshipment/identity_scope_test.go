package parcelshipment_test

import (
	"context"
	"slices"
	"testing"

	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// scopedIdentifiersFake 记下 PS 每次被问时带的客户账户，答预置的候选与信号。
type scopedIdentifiersFake struct {
	candidates []psports.ExternalIdentifierCandidate
	declared   bool
	scopes     [][]string
}

func (fake *scopedIdentifiersFake) FindParcelsByExternalIdentifier(
	_ context.Context, _ psdomain.TenantID, _ psdomain.ExternalIdentifierLookupNumber, scope []psdomain.CustomerAccountID,
) (psports.ExternalIdentifierParcels, error) {
	asked := make([]string, 0, len(scope))
	for _, account := range scope {
		asked = append(asked, account.String())
	}
	fake.scopes = append(fake.scopes, asked)
	return psports.ExternalIdentifierParcels{Candidates: fake.candidates, DeclaredBySubmittedRequest: fake.declared}, nil
}

// Covers: ADR-0212 决定四「PS 读口的输入多一格：可验证交付范围给出的客户账户集合」与「不交分配方、客户账户或委托」的读法——NO 给的
// 客户账户逐字交给 PS；PS 逐候选答的范围相容照转，答复记下这次是不是带着范围问的（判断表按它分两张），候选一件不筛。没给范围时
// 不带客户账户去问，答复不带范围。
func TestTheScopeAccountsAreAskedVerbatimAndCompatibilityIsTranslated(t *testing.T) {
	fake := &scopedIdentifiersFake{candidates: []psports.ExternalIdentifierCandidate{
		{Parcel: must(t, psdomain.NewDeclaredParcelID, "DPCL-A"), ScopeCompatible: true},
		{Parcel: must(t, psdomain.NewDeclaredParcelID, "DPCL-B"), ScopeCompatible: false},
	}, declared: true}
	view, err := noparcel.NewParcelIdentityView(fake)
	if err != nil {
		t.Fatal(err)
	}
	tenant := must(t, nodomain.NewTenantID, "SYN-TENANT-01")
	scope := []nodomain.CustomerAccountReference{
		must(t, nodomain.NewCustomerAccountReference, "SYN-CUSTOMER-A"),
		must(t, nodomain.NewCustomerAccountReference, "SYN-CUSTOMER-B"),
	}

	scoped, err := view.ResolveParcelIdentity(t.Context(), tenant, noports.ExternalMarkObservation{Mark: "SYN-PCL-01"}, scope)
	if err != nil {
		t.Fatalf("带范围核对：%v", err)
	}
	if !scoped.Scoped || !scoped.Ambiguous || len(scoped.Candidates) != 2 ||
		scoped.Candidates[0].Parcel.String() != "DPCL-A" || !scoped.Candidates[0].ScopeCompatible ||
		scoped.Candidates[1].Parcel.String() != "DPCL-B" || scoped.Candidates[1].ScopeCompatible {
		t.Fatalf("带范围的答复 = %+v, want 两件照转、A 相容 B 不相容、带信号、记带过范围", scoped)
	}

	unscoped, err := view.ResolveParcelIdentity(t.Context(), tenant, noports.ExternalMarkObservation{Mark: "SYN-PCL-01"}, nil)
	if err != nil {
		t.Fatalf("不带范围核对：%v", err)
	}
	if unscoped.Scoped || len(unscoped.Candidates) != 2 {
		t.Fatalf("不带范围的答复 = %+v, want 没带范围、候选两件", unscoped)
	}
	if len(fake.scopes) != 2 || !slices.Equal(fake.scopes[0], []string{"SYN-CUSTOMER-A", "SYN-CUSTOMER-B"}) || len(fake.scopes[1]) != 0 {
		t.Fatalf("PS 被问时带的客户账户 = %v, want 第一次两个原样、第二次一个不带", fake.scopes)
	}
}
