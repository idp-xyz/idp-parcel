package parcelshipment_test

import (
	"context"
	"errors"
	"slices"
	"strings"
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

// FindParcelsByExternalIdentifier 把预置的包裹逐件答成范围相容的候选（照渠道包裹件号答）：本文件证号码与信号的翻译，范围的翻译另有
// identity_scope_test.go。
func (fake *externalIdentifiersFake) FindParcelsByExternalIdentifier(
	_ context.Context, tenant psdomain.TenantID, number psdomain.ExternalIdentifierLookupNumber, _ []psdomain.CustomerAccountID,
) (psports.ExternalIdentifierParcels, error) {
	fake.asked = append(fake.asked, tenant.String()+"|"+number.String())
	answer := psports.ExternalIdentifierParcels{DeclaredBySubmittedRequest: fake.declared}
	for _, parcel := range fake.parcels {
		answer.Candidates = append(answer.Candidates, psports.ExternalIdentifierCandidate{Parcel: parcel, ScopeCompatible: true})
	}
	return answer, fake.err
}

func resolve(t *testing.T, view *noparcel.ParcelIdentityView, mark string) ([]string, error) {
	t.Helper()
	answer, err := resolveAnswer(t, view, mark)
	values := make([]string, len(answer.Candidates))
	for index, candidate := range answer.Candidates {
		values[index] = candidate.Parcel.String()
	}
	return values, err
}

func resolveAnswer(t *testing.T, view *noparcel.ParcelIdentityView, mark string) (noports.ParcelIdentityAnswer, error) {
	t.Helper()
	return view.ResolveParcelIdentity(context.Background(),
		must(t, nodomain.NewTenantID, "SYN-TENANT-01"), noports.ExternalMarkObservation{Mark: mark}, nil)
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
	if !answer.Ambiguous || len(answer.Candidates) != 1 || answer.Candidates[0].Parcel.String() != "DPCL-1" {
		t.Fatalf("answer = %+v, want 带歧义信号、候选只是 DPCL-1", answer)
	}
	if verdict, _ := answer.Verdict(); verdict != noports.IdentityVerdictAmbiguityOnly {
		t.Fatalf("带歧义信号的一个候选判成 %v, want 只因歧义信号", verdict)
	}

	fake.declared = false
	clear, err := resolveAnswer(t, view, "SYN-PCL-01")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if verdict, parcel := clear.Verdict(); clear.Ambiguous || verdict != noports.IdentityVerdictUnique || parcel.String() != "DPCL-1" {
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

// Covers: 票 ps-external-mark-relations/06 裁定三与裁定补——带 NUL 的号 Postgres 的 text 存不下，确定不在册：答零个候选、不带
// 歧义信号、不报错，也不去问 PS（问了反倒在库面报错，收寄会读成身份读不动、落进永远续不上的重核）。只是超长的号照常原样
// 去问：入册门只管写，门立起来之前进册的超长号要核得到，PS 答什么就译什么。
func TestANulMarkIsNotAskedAndAnOverlongMarkIsStillAsked(t *testing.T) {
	fake := &externalIdentifiersFake{
		parcels:  []psdomain.DeclaredParcelID{must(t, psdomain.NewDeclaredParcelID, "DPCL-1")},
		declared: true,
	}
	view, err := noparcel.NewParcelIdentityView(fake)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := resolveAnswer(t, view, "SYN-PCL\x00-01")
	if err != nil || len(answer.Candidates) != 0 || answer.Ambiguous || len(fake.asked) != 0 {
		t.Fatalf("带 NUL：answer = %+v err = %v、PS 被问 %d 次, want 零候选、无信号、不报错、不去问", answer, err, len(fake.asked))
	}

	overlong := strings.Repeat("M", 513)
	answer, err = resolveAnswer(t, view, overlong)
	if err != nil || !answer.Ambiguous || len(answer.Candidates) != 1 || answer.Candidates[0].Parcel.String() != "DPCL-1" {
		t.Fatalf("513 字节：answer = %+v err = %v, want PS 答的候选与信号照转", answer, err)
	}
	if !slices.Equal(fake.asked, []string{"SYN-TENANT-01|" + overlong}) {
		t.Fatalf("PS 被问 %d 次, want 原样的超长号恰一次", len(fake.asked))
	}
}
