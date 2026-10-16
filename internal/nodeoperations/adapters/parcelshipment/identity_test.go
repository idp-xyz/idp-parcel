package parcelshipment_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证 NO 核对包裹身份的防腐适配器（ADR-0184 决定五）：号码原样交给 PS，包裹逐件译成关联引用，
// PS 读口的错误原样交回。夹具全为合成引用（S 级）。

type externalIdentifiersFake struct {
	parcels []psdomain.DeclaredParcelID
	err     error
	asked   []string
}

func (fake *externalIdentifiersFake) FindParcelsByExternalIdentifier(
	_ context.Context, tenant psdomain.TenantID, number psdomain.ExternalIdentifierNumber,
) ([]psdomain.DeclaredParcelID, error) {
	fake.asked = append(fake.asked, tenant.String()+"|"+number.String())
	return fake.parcels, fake.err
}

func resolve(t *testing.T, view *noparcel.ParcelIdentityView, mark string) ([]string, error) {
	t.Helper()
	candidates, err := view.ResolveParcelIdentity(context.Background(),
		must(t, nodomain.NewTenantID, "SYN-TENANT-01"), noports.ExternalMarkObservation{Mark: mark})
	values := make([]string, len(candidates))
	for index, candidate := range candidates {
		values[index] = candidate.String()
	}
	return values, err
}

// Covers: 号码原样交给 PS（不折算大小写与空白），PS 答的每一件包裹译成关联引用、次序不动；多件就是多个候选。
func TestTheMarkIsAskedVerbatimAndEveryParcelBecomesACandidate(t *testing.T) {
	fake := &externalIdentifiersFake{parcels: []psdomain.DeclaredParcelID{
		must(t, psdomain.NewDeclaredParcelID, "DPCL-1"), must(t, psdomain.NewDeclaredParcelID, "DPCL-2"),
	}}
	view, err := noparcel.NewParcelIdentityView(fake)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolve(t, view, " SYN-pcl-01")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !slices.Equal(got, []string{"DPCL-1", "DPCL-2"}) {
		t.Fatalf("candidates = %v", got)
	}
	if !slices.Equal(fake.asked, []string{"SYN-TENANT-01| SYN-pcl-01"}) {
		t.Fatalf("PS 被问 %v，want 原样的号码", fake.asked)
	}
}

// Covers: 观察里没有号码答零个候选、不去问 PS；PS 读不动原样交回，不译成「没有这件包裹」；构造期拒缺件。
func TestABlankMarkHasNoCandidatesAndAReadFailureIsReturned(t *testing.T) {
	fake := &externalIdentifiersFake{}
	view, err := noparcel.NewParcelIdentityView(fake)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := resolve(t, view, "   "); err != nil || len(got) != 0 || len(fake.asked) != 0 {
		t.Fatalf("空号码：candidates = %v, err = %v, PS 被问 %v", got, err, fake.asked)
	}
	down := errors.New("external identifier register unreachable")
	failing, err := noparcel.NewParcelIdentityView(&externalIdentifiersFake{err: down})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(t, failing, "SYN-PCL-01"); !errors.Is(err, down) {
		t.Fatalf("err = %v, want 原样交回", err)
	}
	if _, err := noparcel.NewParcelIdentityView(nil); err == nil {
		t.Fatal("缺读口也装得起来")
	}
}
