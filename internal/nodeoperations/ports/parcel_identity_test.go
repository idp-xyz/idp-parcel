package ports_test

import (
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

func candidate(t *testing.T, parcel string, compatible bool) ports.ParcelIdentityCandidate {
	t.Helper()
	reference, err := domain.NewParcelAssociationReference(parcel)
	if err != nil {
		t.Fatal(err)
	}
	return ports.ParcelIdentityCandidate{Parcel: reference, ScopeCompatible: compatible}
}

// Covers: ADR-0212 决定四「NO 的判断表」逐格，认唯一只经答复值上的这一个方法（决定四末项；同 ADR-0195 决定三）。没有可验证交付范围时
// 数全部候选：零候选待识别；多候选或恰一个带信号是冲突；恰一个、无信号、范围相容是唯一命中；恰一个、无信号、只经客户包裹件号命中
// （不范围相容）是范围未证（`AT-NO-063`）。有范围时只数范围相容的候选、信号由 PS 收到范围内：恰一个且无信号是唯一命中，范围外还有
// 几个都不管（`AT-NO-064`）；不止一个或恰一个带信号是冲突（`AT-NO-065`）；一个也没有是待识别、不记冲突（`AT-NO-066`）。
func TestTheIdentityVerdictFollowsTheScopeTable(t *testing.T) {
	a, b := "SYN-PARCEL-A", "SYN-PARCEL-B"
	for name, testCase := range map[string]struct {
		answer ports.ParcelIdentityAnswer
		want   ports.ParcelIdentityVerdict
		parcel string
	}{
		"无范围·零候选":          {ports.ParcelIdentityAnswer{}, ports.IdentityVerdictUnknown, ""},
		"无范围·零候选带信号":       {ports.ParcelIdentityAnswer{Ambiguous: true}, ports.IdentityVerdictUnknown, ""},
		"无范围·多候选":          {ports.ParcelIdentityAnswer{Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, true), candidate(t, b, true)}}, ports.IdentityVerdictMultipleCandidates, ""},
		"无范围·多候选带信号":       {ports.ParcelIdentityAnswer{Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, false), candidate(t, b, false)}, Ambiguous: true}, ports.IdentityVerdictMultipleCandidates, ""},
		"无范围·恰一个带信号":       {ports.ParcelIdentityAnswer{Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, false)}, Ambiguous: true}, ports.IdentityVerdictAmbiguityOnly, ""},
		"无范围·恰一个相容带信号":     {ports.ParcelIdentityAnswer{Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, true)}, Ambiguous: true}, ports.IdentityVerdictAmbiguityOnly, ""},
		"无范围·恰一个相容":        {ports.ParcelIdentityAnswer{Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, true)}}, ports.IdentityVerdictUnique, a},
		"无范围·恰一个只经客户包裹件号":  {ports.ParcelIdentityAnswer{Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, false)}}, ports.IdentityVerdictScopeUnverified, ""},
		"有范围·相容恰一个范围外另有一个": {ports.ParcelIdentityAnswer{Scoped: true, Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, true), candidate(t, b, false)}}, ports.IdentityVerdictUnique, a},
		"有范围·相容恰一个":        {ports.ParcelIdentityAnswer{Scoped: true, Candidates: []ports.ParcelIdentityCandidate{candidate(t, b, true)}}, ports.IdentityVerdictUnique, b},
		"有范围·相容恰一个带范围内信号":  {ports.ParcelIdentityAnswer{Scoped: true, Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, true), candidate(t, b, false)}, Ambiguous: true}, ports.IdentityVerdictAmbiguityOnly, ""},
		"有范围·相容两个":         {ports.ParcelIdentityAnswer{Scoped: true, Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, true), candidate(t, b, true)}}, ports.IdentityVerdictMultipleCandidates, ""},
		"有范围·相容零个范围外两个":    {ports.ParcelIdentityAnswer{Scoped: true, Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, false), candidate(t, b, false)}}, ports.IdentityVerdictUnknown, ""},
		"有范围·相容零个带范围内信号":   {ports.ParcelIdentityAnswer{Scoped: true, Candidates: []ports.ParcelIdentityCandidate{candidate(t, a, false)}, Ambiguous: true}, ports.IdentityVerdictUnknown, ""},
		"有范围·零候选":          {ports.ParcelIdentityAnswer{Scoped: true}, ports.IdentityVerdictUnknown, ""},
	} {
		verdict, parcel := testCase.answer.Verdict()
		if verdict != testCase.want || parcel.String() != testCase.parcel {
			t.Fatalf("%s：判 %v %q, want %v %q", name, verdict, parcel, testCase.want, testCase.parcel)
		}
	}
}

// Covers: 候选照记全部（ADR-0212 决定四「候选记全部候选，含范围外的」）：Parcels 交回每一个候选，不按范围相容筛。
func TestEveryCandidateIsKeptRegardlessOfScope(t *testing.T) {
	answer := ports.ParcelIdentityAnswer{Scoped: true, Candidates: []ports.ParcelIdentityCandidate{
		candidate(t, "SYN-PARCEL-A", true), candidate(t, "SYN-PARCEL-B", false),
	}}
	var got []string
	for _, parcel := range answer.Parcels() {
		got = append(got, parcel.String())
	}
	if !slices.Equal(got, []string{"SYN-PARCEL-A", "SYN-PARCEL-B"}) {
		t.Fatalf("Parcels = %v, want 两件都在", got)
	}
}

// Covers: 判断格的线上名封闭，零值没有名字。
func TestTheIdentityVerdictIsAClosedVocabulary(t *testing.T) {
	names := map[string]bool{}
	for _, verdict := range []ports.ParcelIdentityVerdict{
		ports.IdentityVerdictUnique, ports.IdentityVerdictUnknown, ports.IdentityVerdictScopeUnverified,
		ports.IdentityVerdictAmbiguityOnly, ports.IdentityVerdictMultipleCandidates,
	} {
		if verdict.String() == "" || names[verdict.String()] {
			t.Fatalf("判断格 %d 的名字 %q 空或重复", verdict, verdict.String())
		}
		names[verdict.String()] = true
	}
	if ports.IdentityVerdictInvalid.String() != "" {
		t.Fatal("零值有了名字")
	}
}
