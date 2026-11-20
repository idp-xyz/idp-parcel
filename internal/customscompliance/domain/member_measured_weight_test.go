package domain

import (
	"testing"
	"time"
)

func TestAVersionFixesTheMeasuredGrossWeightOfItsMembers(t *testing.T) {
	weighedAt := time.Date(2026, 11, 18, 3, 4, 5, 0, time.UTC)
	version := versionWithWeights(t, []MemberMeasuredWeight{
		mustWeight(t, "parcel-b", "3", "KG", "SYN-WEIGH-B", weighedAt),
		mustWeight(t, "parcel-a", "2.1", "KG", "SYN-WEIGH-A", weighedAt.Add(time.Minute)),
	})
	weights := version.MemberWeights()
	if len(weights) != 2 || weights[0].Parcel().String() != "parcel-a" || weights[1].Parcel().String() != "parcel-b" {
		t.Fatalf("顺序 = %s %s，want 按成员引用", weights[0].Parcel(), weights[1].Parcel())
	}
	if weights[0].GrossWeight() != "2.1" || weights[0].GrossWeightUnit() != "KG" || weights[0].MeasurementSource() != "SYN-WEIGH-A" ||
		!weights[0].MeasuredAt().Equal(weighedAt.Add(time.Minute)) {
		t.Fatalf("毛重 = %+v", weights[0])
	}
	weights[0].gross = "9"
	if again := version.MemberWeights(); again[0].GrossWeight() != "2.1" {
		t.Fatalf("取值被改写了：%s", again[0].GrossWeight())
	}
	snapshot := version.Snapshot()
	if snapshot.MemberWeights[0].Parcel().String() != "parcel-a" || snapshot.MemberWeights[1].Parcel().String() != "parcel-b" {
		t.Fatal("快照顺序与取值不一致")
	}
	rebuilt, err := RehydrateSubmissionVersion(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.MemberWeights()[0].MeasurementSource() != "SYN-WEIGH-A" {
		t.Fatal("读回丢了来源身份")
	}
}

func TestMemberWeightsRejectWhatCannotBeFixed(t *testing.T) {
	weighedAt := time.Date(2026, 11, 18, 3, 4, 5, 0, time.UTC)
	valid := mustWeight(t, "parcel-a", "2.1", "KG", "SYN-WEIGH-A", weighedAt)
	outsider := mustWeight(t, "parcel-z", "2.1", "KG", "SYN-WEIGH-Z", weighedAt)
	blankGross := valid
	blankGross.gross = " "
	blankUnit := valid
	blankUnit.unit = " "
	blankSource := valid
	blankSource.source = " "
	timeless := valid
	timeless.measuredAt = time.Time{}
	for _, testCase := range []struct {
		name    string
		weights []MemberMeasuredWeight
	}{
		{name: "非成员", weights: []MemberMeasuredWeight{outsider}},
		{name: "同一成员两条", weights: []MemberMeasuredWeight{valid, valid}},
		{name: "空白数值", weights: []MemberMeasuredWeight{blankGross}},
		{name: "空白单位", weights: []MemberMeasuredWeight{blankUnit}},
		{name: "空白来源", weights: []MemberMeasuredWeight{blankSource}},
		{name: "零时刻", weights: []MemberMeasuredWeight{timeless}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := fixWeights(t, testCase.weights); err != ErrInvalidSubmissionVersion {
				t.Fatalf("成版 err = %v, want ErrInvalidSubmissionVersion", err)
			}
			snapshot := versionWithWeights(t, nil).Snapshot()
			snapshot.MemberWeights = testCase.weights
			if _, err := RehydrateSubmissionVersion(snapshot); err != ErrInvalidSubmissionVersion {
				t.Fatalf("重建 err = %v, want ErrInvalidSubmissionVersion", err)
			}
		})
	}
	reversed := versionWithWeights(t, []MemberMeasuredWeight{
		mustWeight(t, "parcel-b", "3", "KG", "SYN-WEIGH-B", weighedAt),
		mustWeight(t, "parcel-a", "2.1", "KG", "SYN-WEIGH-A", weighedAt),
	}).Snapshot()
	reversed.MemberWeights = []MemberMeasuredWeight{reversed.MemberWeights[1], reversed.MemberWeights[0]}
	rebuilt, err := RehydrateSubmissionVersion(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.MemberWeights()[0].Parcel().String() != "parcel-a" {
		t.Fatal("重建没有把顺序收稳")
	}
}

func TestCompareMemberMeasurementNowKeepsTheFiveStatesApart(t *testing.T) {
	same, listed := CompareMemberMeasurementNow(MemberMeasurementNowFacts{Fixed: true, FixedSource: "A", Found: true, Source: "A"})
	changedSource, _ := CompareMemberMeasurementNow(MemberMeasurementNowFacts{Fixed: true, FixedSource: "A", Found: true, Source: "B"})
	changedNew, _ := CompareMemberMeasurementNow(MemberMeasurementNowFacts{Found: true, Source: "B"})
	noneNow, _ := CompareMemberMeasurementNow(MemberMeasurementNowFacts{Fixed: true, FixedSource: "A"})
	undetermined, _ := CompareMemberMeasurementNow(MemberMeasurementNowFacts{Undetermined: true, Found: true, Source: "B"})
	unavailable, _ := CompareMemberMeasurementNow(MemberMeasurementNowFacts{Failed: true, Fixed: true, FixedSource: "A"})
	_, bothNone := CompareMemberMeasurementNow(MemberMeasurementNowFacts{})
	if !listed || same != MemberMeasurementSame || same.String() != "SAME" ||
		changedSource != MemberMeasurementChanged || changedNew != MemberMeasurementChanged ||
		noneNow != MemberMeasurementNoneNow || noneNow.String() != "NONE_NOW" ||
		undetermined != MemberMeasurementUndetermined || undetermined.String() != "UNDETERMINED" ||
		unavailable != MemberMeasurementUnavailable || unavailable.String() != "UNAVAILABLE" || bothNone {
		t.Fatalf("same=%s changed=%s %s none=%s undetermined=%s unavailable=%s bothNone=%v",
			same, changedSource, changedNew, noneNow, undetermined, unavailable, bothNone)
	}
}

func versionWithWeights(t *testing.T, weights []MemberMeasuredWeight) CustomsSubmissionVersion {
	t.Helper()
	version, err := fixWeights(t, weights)
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func fixWeights(t *testing.T, weights []MemberMeasuredWeight) (CustomsSubmissionVersion, error) {
	t.Helper()
	unit, err := FormDeclarationUnit(
		mustID(t, NewDeclarationUnitID, "declaration-unit-1"),
		mustID(t, NewCustomsCaseID, "case-1"),
		mustID(t, NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		[]DeclaredParcelReference{mustParcel(t, "parcel-a"), mustParcel(t, "parcel-b")},
	)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)
	readiness, err := JudgeReady(unit.ID(), mustID(t, NewReadinessBasisReference, "readiness-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := GrantSubmissionAuthority(unit.ID(), mustID(t, NewSubmissionAuthorityReference, "authority-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	return FixSubmissionVersion(CustomsSubmissionVersionSpec{
		ID:            mustID(t, NewSubmissionVersionID, "submission-1"),
		Unit:          unit,
		Dossier:       mustID(t, NewDossierSnapshotReference, "dossier-1"),
		Roles:         mustID(t, NewRoleSnapshotReference, "roles-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       at.Add(time.Hour),
		MemberWeights: weights,
	})
}

func mustWeight(t *testing.T, parcel, gross, unit, source string, at time.Time) MemberMeasuredWeight {
	t.Helper()
	weight, err := NewMemberMeasuredWeight(mustParcel(t, parcel), gross, unit, source, at)
	if err != nil {
		t.Fatal(err)
	}
	return weight
}

func mustParcel(t *testing.T, raw string) DeclaredParcelReference {
	t.Helper()
	return mustID(t, NewDeclaredParcelReference, raw)
}

func mustID[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
