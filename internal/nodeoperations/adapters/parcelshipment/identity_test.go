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
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证 NO 核对包裹身份的防腐适配器（ADR-0184 决定五、ADR-0195）：号码原样交给 PS，包裹逐件译成关联引用，PS 答出的
// 歧义信号照转，PS 读口的错误原样交回。夹具全为合成引用（S 级）。

type externalIdentifiersFake struct {
	parcels  []psdomain.DeclaredParcelID
	declared bool
	err      error
	asked    []string
}

func (fake *externalIdentifiersFake) FindParcelsByExternalIdentifier(
	_ context.Context, tenant psdomain.TenantID, number psdomain.ExternalIdentifierNumber,
) (psports.ExternalIdentifierParcels, error) {
	fake.asked = append(fake.asked, tenant.String()+"|"+number.String())
	return psports.ExternalIdentifierParcels{Parcels: fake.parcels, DeclaredBySubmittedRequest: fake.declared}, fake.err
}

func resolve(t *testing.T, view *noparcel.ParcelIdentityView, mark string) ([]string, error) {
	t.Helper()
	answer, err := resolveAnswer(t, view, mark)
	values := make([]string, len(answer.Candidates))
	for index, candidate := range answer.Candidates {
		values[index] = candidate.String()
	}
	return values, err
}

func resolveAnswer(t *testing.T, view *noparcel.ParcelIdentityView, mark string) (noports.ParcelIdentityAnswer, error) {
	t.Helper()
	return view.ResolveParcelIdentity(context.Background(),
		must(t, nodomain.NewTenantID, "SYN-TENANT-01"), noports.ExternalMarkObservation{Mark: mark})
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

// Covers: ADR-0195——PS 答出号码同时被一份`已提交`委托声明时，适配器把它照转成歧义信号，候选仍只是在册的包裹；唯一
// 那一件因此不再是唯一命中。PS 没答出时没有信号，唯一那一件照旧唯一。
func TestADeclarationByASubmittedRequestBecomesTheAmbiguitySignal(t *testing.T) {
	fake := &externalIdentifiersFake{parcels: []psdomain.DeclaredParcelID{must(t, psdomain.NewDeclaredParcelID, "DPCL-1")}, declared: true}
	view, err := noparcel.NewParcelIdentityView(fake)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := resolveAnswer(t, view, "SYN-PCL-01")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !answer.Ambiguous || len(answer.Candidates) != 1 || answer.Candidates[0].String() != "DPCL-1" {
		t.Fatalf("answer = %+v, want 带歧义信号、候选只是 DPCL-1", answer)
	}
	if _, unique := answer.Unique(); unique {
		t.Fatal("带歧义信号的一个候选被认成了唯一命中")
	}

	fake.declared = false
	clear, err := resolveAnswer(t, view, "SYN-PCL-01")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if parcel, unique := clear.Unique(); clear.Ambiguous || !unique || parcel.String() != "DPCL-1" {
		t.Fatalf("answer = %+v, want 不带信号、唯一命中 DPCL-1", clear)
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
