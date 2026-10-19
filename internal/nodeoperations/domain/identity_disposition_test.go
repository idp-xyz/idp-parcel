package domain_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证身份冲突处置文档（ADR-0210 决定二至四、六）：两种结果的形状、连带实物、NID-1 往返与比对格摘要。夹具全为合成（S 级）。

var dispositionAt = time.Date(2026, 10, 19, 9, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))

func designation(t *testing.T) domain.IdentityDisposition {
	t.Helper()
	return domain.IdentityDisposition{
		Disposed:   mustBuild(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-NRI06-LATE"),
		Unit:       mustBuild(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-B"),
		Result:     domain.DispositionDesignatesParcel,
		Parcel:     mustBuild(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-X"),
		CoDisposed: []domain.CoDisposedUnit{{Unit: mustBuild(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-A"), Version: mustBuild(t, domain.NewIntakeResultVersion, "SYN-INTAKEV-NRI06-A-CONFLICT")}},
		Basis:      "  SYN-DISPOSITION-BASIS-NRI06 \t",
		Operator:   "OPERATOR/https://id.syn.example/dex#SYN-OPERATOR-01",
		DisposedAt: dispositionAt,
	}
}

func mustBuild[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// Covers: 指定一件的处置文档——依据引用去两端空白、时刻按 UTC；ExcludedFor 对主实物什么也不排除、对连带实物只排除指定的那件
// （ADR-0210 决定四「排除的那一组只有 X」、决定九）；NID-1 往返逐格不变。
func TestADesignationExcludesTheParcelOnlyForTheCoDisposedUnits(t *testing.T) {
	formed, err := domain.FormIdentityDisposition(designation(t))
	if err != nil {
		t.Fatalf("立指定一件的处置：%v", err)
	}
	if formed.Basis != "SYN-DISPOSITION-BASIS-NRI06" || formed.DisposedAt.Location() != time.UTC || !formed.DisposedAt.Equal(dispositionAt) {
		t.Fatalf("依据 %q、时刻 %s", formed.Basis, formed.DisposedAt)
	}
	if excluded := formed.ExcludedFor(formed.Unit); len(excluded) != 0 {
		t.Fatalf("主实物排除了 %v", excluded)
	}
	coUnit := mustBuild(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-A")
	if excluded := formed.ExcludedFor(coUnit); len(excluded) != 1 || excluded[0] != formed.Parcel {
		t.Fatalf("连带实物排除 %v, want 只有 %s", excluded, formed.Parcel)
	}
	document, err := domain.CanonicalizeIdentityDisposition(formed)
	if err != nil {
		t.Fatalf("写成 NID-1：%v", err)
	}
	if !strings.Contains(string(document), `"shape":"NID-1"`) {
		t.Fatalf("文档没带形状版本：%s", document)
	}
	parsed, err := domain.ParseIdentityDisposition(document)
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	if !parsed.DisposedAt.Equal(formed.DisposedAt) || parsed.Disposed != formed.Disposed || parsed.Unit != formed.Unit ||
		parsed.Result != formed.Result || parsed.Parcel != formed.Parcel || !slices.Equal(parsed.CoDisposed, formed.CoDisposed) ||
		parsed.Basis != formed.Basis || parsed.Operator != formed.Operator || len(parsed.Excluded) != 0 {
		t.Fatalf("往返不等：%+v vs %+v", parsed, formed)
	}
}

// Covers: 确认都不是的处置文档——排除的那一组就是处置候选，按标识定序；主实物那一版据它排除。
func TestNoneOfCandidatesExcludesTheCandidates(t *testing.T) {
	none := designation(t)
	none.Result = domain.DispositionNoneOfCandidates
	none.Parcel = domain.ParcelAssociationReference{}
	none.CoDisposed = nil
	none.Excluded = []domain.ParcelAssociationReference{
		mustBuild(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Z"),
		mustBuild(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Y"),
	}
	formed, err := domain.FormIdentityDisposition(none)
	if err != nil {
		t.Fatalf("立都不是：%v", err)
	}
	excluded := formed.ExcludedFor(formed.Unit)
	if len(excluded) != 2 || excluded[0].String() != "SYN-PARCEL-NRI06-Y" || excluded[1].String() != "SYN-PARCEL-NRI06-Z" {
		t.Fatalf("排除 %v, want 两件按标识定序", excluded)
	}
	document, err := domain.CanonicalizeIdentityDisposition(formed)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := domain.ParseIdentityDisposition(document)
	if err != nil || !slices.Equal(parsed.Excluded, formed.Excluded) || parsed.Recorded() != true {
		t.Fatalf("读回 %+v err %v", parsed, err)
	}
	if (domain.IdentityDisposition{}).Recorded() {
		t.Fatal("零值读成了处置版")
	}
}

// Covers: ADR-0210 决定六坏载荷的领域半边——甲不带 X、乙带 X 或连带实物、连带实物重复或就是主实物、依据空白或带 NUL、缺操作者，
// 都立不起来；形状版本认不出的文档读不回。
func TestAMalformedDispositionIsRefused(t *testing.T) {
	cases := map[string]func(*domain.IdentityDisposition){
		"甲不带 X": func(d *domain.IdentityDisposition) { d.Parcel = domain.ParcelAssociationReference{} },
		"乙带 X": func(d *domain.IdentityDisposition) {
			d.Result, d.CoDisposed, d.Excluded = domain.DispositionNoneOfCandidates, nil, []domain.ParcelAssociationReference{d.Parcel}
		},
		"乙带连带实物": func(d *domain.IdentityDisposition) {
			d.Result, d.Excluded = domain.DispositionNoneOfCandidates, []domain.ParcelAssociationReference{d.Parcel}
			d.Parcel = domain.ParcelAssociationReference{}
		},
		"乙不排除任何一件": func(d *domain.IdentityDisposition) {
			d.Result, d.CoDisposed, d.Parcel = domain.DispositionNoneOfCandidates, nil, domain.ParcelAssociationReference{}
		},
		"甲带排除":     func(d *domain.IdentityDisposition) { d.Excluded = []domain.ParcelAssociationReference{d.Parcel} },
		"连带实物重复":   func(d *domain.IdentityDisposition) { d.CoDisposed = append(d.CoDisposed, d.CoDisposed[0]) },
		"连带实物是主实物": func(d *domain.IdentityDisposition) { d.CoDisposed[0].Unit = d.Unit },
		"连带实物缺版本":  func(d *domain.IdentityDisposition) { d.CoDisposed[0].Version = domain.IntakeResultVersion{} },
		"依据空白":     func(d *domain.IdentityDisposition) { d.Basis = " \t " },
		"依据带 NUL":  func(d *domain.IdentityDisposition) { d.Basis = "SYN\x00BASIS" },
		"缺操作者":     func(d *domain.IdentityDisposition) { d.Operator = "" },
		"缺结果":      func(d *domain.IdentityDisposition) { d.Result = domain.IdentityDispositionResultInvalid },
	}
	for name, mutate := range cases {
		disposition := designation(t)
		disposition.CoDisposed = slices.Clone(disposition.CoDisposed)
		mutate(&disposition)
		if _, err := domain.FormIdentityDisposition(disposition); !errors.Is(err, domain.ErrInvalidIdentityDisposition) {
			t.Fatalf("%s：err = %v, want ErrInvalidIdentityDisposition", name, err)
		}
	}
	if _, err := domain.ParseIdentityDisposition([]byte(`{"shape":"NID-2"}`)); !errors.Is(err, domain.ErrInvalidIdentityDisposition) {
		t.Fatalf("认不出的形状版本：err = %v", err)
	}
	if _, err := domain.ParseIdentityDispositionResult("designated"); !errors.Is(err, domain.ErrInvalidIdentityDisposition) {
		t.Fatalf("大小写不折叠：err = %v", err)
	}
}

// Covers: 比对格摘要（ADR-0210 决定六）：只看结果、指定的包裹与连带实物的集合——连带实物的提交次序不是内容；换结果、换指定的
// 包裹、换连带实物都换摘要。摘要带 NOC-1 前缀。
func TestTheComparisonDigestCoversResultParcelAndCoDisposedUnitsOnly(t *testing.T) {
	x := mustBuild(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-X")
	y := mustBuild(t, domain.NewParcelAssociationReference, "SYN-PARCEL-NRI06-Y")
	a := mustBuild(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-A")
	c := mustBuild(t, domain.NewHandlingUnitID, "SYN-UNIT-NRI06-C")
	digest := func(result domain.IdentityDispositionResult, parcel domain.ParcelAssociationReference, units ...domain.HandlingUnitID) string {
		t.Helper()
		_, value, err := domain.CanonicalizeIdentityDispositionComparison(result, parcel, units)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	base := digest(domain.DispositionDesignatesParcel, x, a, c)
	if !strings.HasPrefix(base, "NOC-1:") {
		t.Fatalf("摘要 %q 没带形状前缀", base)
	}
	if digest(domain.DispositionDesignatesParcel, x, c, a) != base {
		t.Fatal("连带实物的次序进了摘要")
	}
	for name, other := range map[string]string{
		"换指定的包裹": digest(domain.DispositionDesignatesParcel, y, a, c),
		"换连带实物":  digest(domain.DispositionDesignatesParcel, x, a),
		"换结果":    digest(domain.DispositionNoneOfCandidates, domain.ParcelAssociationReference{}),
	} {
		if other == base {
			t.Fatalf("%s 摘要没变", name)
		}
	}
}

// Covers: JudgmentChainHeads 交回各链没有后继的那几行，与 JudgeIdentityBasis 同一个读法：被组里另一行指名为前版的不是链头。
func TestJudgmentChainHeadsAreTheUnsucceededObservations(t *testing.T) {
	version := func(raw string) domain.IntakeResultVersion { return mustBuild(t, domain.NewIntakeResultVersion, raw) }
	observations := []domain.IntakeIdentityObservation{
		{SourceID: "SYN-A", Version: version("SYN-V-1"), ReceivedAt: dispositionAt},
		{SourceID: "SYN-B", Version: version("SYN-V-2"), Predecessor: version("SYN-V-1"), IdentityConflict: true, ReceivedAt: dispositionAt},
		{SourceID: "SYN-C", Version: version("SYN-V-3"), ReceivedAt: dispositionAt},
	}
	heads, err := domain.JudgmentChainHeads(observations)
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, head := range heads {
		sources = append(sources, head.SourceID)
	}
	if !slices.Equal(sources, []string{"SYN-B", "SYN-C"}) {
		t.Fatalf("链头 %v, want SYN-B 与 SYN-C", sources)
	}
	observations = append(observations, domain.IntakeIdentityObservation{SourceID: "SYN-D", Version: version("SYN-V-4"), Predecessor: version("SYN-V-1"), ReceivedAt: dispositionAt})
	if _, err := domain.JudgmentChainHeads(observations); !errors.Is(err, domain.ErrInvalidDirectionalBasis) {
		t.Fatalf("一版两个后继：err = %v", err)
	}
}
