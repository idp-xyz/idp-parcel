package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func TestAllowWeightDeltaUpToKeepsASingleDecimalSpelling(t *testing.T) {
	for _, raw := range []string{"0", "0.00", "00", "0.0"} {
		rule, err := domain.AllowWeightDeltaUpTo(raw, "KG")
		if err != nil {
			t.Fatalf("%q：%v", raw, err)
		}
		delta, unit, ok := rule.MaxDelta()
		if !ok || delta != "0" || unit != "KG" || rule.NotAPrecondition() {
			t.Fatalf("%q 收成 %q %q ok=%v", raw, delta, unit, ok)
		}
	}
	rule, err := domain.AllowWeightDeltaUpTo("0.50", "KG")
	if err != nil {
		t.Fatal(err)
	}
	delta, unit, ok := rule.MaxDelta()
	if !ok || delta != "0.5" || unit != "KG" {
		t.Fatalf("0.50 收成 %q %q", delta, unit)
	}
	same, err := domain.AllowWeightDeltaUpTo("0.500", "KG")
	if err != nil {
		t.Fatal(err)
	}
	sameDelta, _, _ := same.MaxDelta()
	if sameDelta != "0.5" {
		t.Fatalf("0.500 收成 %q", sameDelta)
	}

	waived := domain.WeightDiscrepancyNotAPrecondition()
	if !waived.NotAPrecondition() {
		t.Fatal("不构成前置条件没有读出来")
	}
	if _, _, ok := waived.MaxDelta(); ok {
		t.Fatal("不构成前置条件读出了容差")
	}
	if _, _, ok := (domain.WeightDiscrepancyGateRule{}).MaxDelta(); ok {
		t.Fatal("零值规则读出了容差")
	}
}

func TestAllowWeightDeltaUpToRejectsWhatIsNotAWeightTolerance(t *testing.T) {
	cases := []struct {
		delta, unit string
	}{
		{"-1", "KG"},
		{"+1", "KG"},
		{" 1", "KG"},
		{"1 ", "KG"},
		{"1/2", "KG"},
		{"1e-1", "KG"},
		{"", "KG"},
		{"0.", "KG"},
		{".5", "KG"},
		{"1.2.3", "KG"},
		{"0.5", ""},
		{"0.5", "BOX"},
		{"0.5", "CM"},
		{"0.5", "PCS"},
		{"0.5", "kg"},
	}
	for _, testCase := range cases {
		if _, err := domain.AllowWeightDeltaUpTo(testCase.delta, testCase.unit); !errors.Is(err, domain.ErrInvalidWeightDiscrepancyGateRule) {
			t.Fatalf("%q %q err = %v", testCase.delta, testCase.unit, err)
		}
	}
}

func TestWeightDiscrepancyJudgeComparesInTheRuleUnit(t *testing.T) {
	rule := weightRule(t, "0.50", "KG")
	within := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2.00", "KG", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "2.50", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	})
	if within.State != domain.PreconditionMet || !within.Members[0].WithinTolerance || within.MaxDelta != "0.5" || within.Unit != "KG" {
		t.Fatalf("恰好等于容差该满足：%+v", within)
	}

	over := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2.00", "KG", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "2.500001", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	})
	if over.State != domain.PreconditionUnmet || over.Members[0].WithinTolerance {
		t.Fatalf("超出容差该未满足：%+v", over)
	}

	cross := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2100", "G", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "2.60", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	})
	if cross.State != domain.PreconditionMet || cross.Members[0].DeclaredUnit != "G" || !cross.Members[0].WithinTolerance {
		t.Fatalf("2100 G 与 2.60 KG 差恰好 0.5 KG：%+v", cross)
	}

	pounds := weightRule(t, "0", "KG")
	exact := judgeWeights(t, pounds, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "1", "LB", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "453592.37", GrossWeightUnit: "MG", MeasurementSource: "SYN-WEIGH-NOW"},
	})
	if exact.State != domain.PreconditionMet {
		t.Fatalf("磅与毫克要按有理数比成相等：%+v", exact)
	}
}

func TestWeightDiscrepancyJudgeFailsTheWholeGateWhenAnyMemberIsOut(t *testing.T) {
	rule := weightRule(t, "0.5", "KG")
	mixed := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-b": weightAt(t, "parcel-b", "1.00", "KG", "SYN-WEIGH-B"),
		"parcel-a": weightAt(t, "parcel-a", "2.00", "KG", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-b": {GrossWeight: "3.00", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-B2"},
		"parcel-a": {GrossWeight: "2.10", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A2"},
	})
	if mixed.State != domain.PreconditionUnmet || len(mixed.Members) != 2 ||
		mixed.Members[0].Parcel.String() != "parcel-a" || !mixed.Members[0].WithinTolerance ||
		mixed.Members[1].Parcel.String() != "parcel-b" || mixed.Members[1].WithinTolerance {
		t.Fatalf("一件超差整门未满足，且按成员引用排：%+v", mixed)
	}

	unfixed := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2.00", "KG", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "2.10", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A2"},
		"parcel-b": {GrossWeight: "1.00", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-B2"},
	})
	if unfixed.State != domain.PreconditionUnmet || unfixed.Members[1].WithinTolerance ||
		unfixed.Members[1].DeclaredWeight != "" || unfixed.Members[1].DeclaredUnit != "" || unfixed.Members[1].DeclaredSource != "" {
		t.Fatalf("成版时没固定毛重的成员不在容差内：%+v", unfixed)
	}
}

// 容差比的是差的绝对值。实测比固定毛重轻，和偏重走同一条门；差得恰好等于容差仍算满足。
func TestWeightDiscrepancyJudgeUsesAbsoluteDeltaWhenTheScaleReadsLight(t *testing.T) {
	rule := weightRule(t, "0.50", "KG")
	light := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2.60", "KG", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "1.80", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	})
	if light.State != domain.PreconditionUnmet || len(light.Members) != 1 || light.Members[0].WithinTolerance {
		t.Fatalf("比固定轻 0.80 KG、容差 0.50 KG 该整门未满足：%+v", light)
	}

	edge := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2.60", "KG", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "2.10", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	})
	if edge.State != domain.PreconditionMet || !edge.Members[0].WithinTolerance {
		t.Fatalf("轻恰好 0.50 KG 该满足：%+v", edge)
	}
}

// 成员按包裹引用的字典序入行。排在前面的一件超差之后，后面一件在容差内不能把整门盖回满足。
func TestWeightDiscrepancyJudgeStaysUnmetWhenAnEarlierMemberIsOutsideTolerance(t *testing.T) {
	rule := weightRule(t, "0.50", "KG")
	reading, err := rule.Judge(weightVersion(t, []string{"PARCEL-A", "PARCEL-B"}, map[string]domain.MemberMeasuredWeight{
		"PARCEL-A": weightAt(t, "PARCEL-A", "2.00", "KG", "SYN-WEIGH-A"),
		"PARCEL-B": weightAt(t, "PARCEL-B", "2.00", "KG", "SYN-WEIGH-B"),
	}), weightActuals(t, map[string]domain.WeightDiscrepancyMemberActual{
		"PARCEL-A": {GrossWeight: "2.60", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A2"},
		"PARCEL-B": {GrossWeight: "2.10", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-B2"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.Members) != 2 ||
		reading.Members[0].Parcel.String() != "PARCEL-A" || reading.Members[0].WithinTolerance ||
		reading.Members[1].Parcel.String() != "PARCEL-B" || !reading.Members[1].WithinTolerance ||
		reading.State != domain.PreconditionUnmet {
		t.Fatalf("排在前面的一件超差，整门仍该未满足：%+v", reading)
	}
}

func TestWeightDiscrepancyJudgeRefusesUnconvertibleWeightsAndARuleThatCannotJudge(t *testing.T) {
	rule := weightRule(t, "0.5", "KG")
	fixed := map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2.00", "KG", "SYN-WEIGH-A"),
	}
	for _, actual := range []domain.WeightDiscrepancyMemberActual{
		{GrossWeight: "2.10", GrossWeightUnit: "CM", MeasurementSource: "SYN-WEIGH-NOW"},
		{GrossWeight: "2.10", GrossWeightUnit: "PCS", MeasurementSource: "SYN-WEIGH-NOW"},
		{GrossWeight: "1e-1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
		{GrossWeight: "1/2", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
		{GrossWeight: "-1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	} {
		_, err := rule.Judge(weightVersion(t, []string{"parcel-a"}, fixed), weightActuals(t, map[string]domain.WeightDiscrepancyMemberActual{
			"parcel-a": actual,
		}))
		if !errors.Is(err, domain.ErrWeightDiscrepancyUnitUnconvertible) {
			t.Fatalf("%+v err = %v", actual, err)
		}
	}
	_, err := rule.Judge(weightVersion(t, []string{"parcel-a"}, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2", "CM", "SYN-WEIGH-A"),
	}), weightActuals(t, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "2.10", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	}))
	if !errors.Is(err, domain.ErrWeightDiscrepancyUnitUnconvertible) {
		t.Fatalf("固定毛重单位换不出：%v", err)
	}

	_, err = domain.WeightDiscrepancyNotAPrecondition().Judge(weightVersion(t, []string{"parcel-a"}, fixed), nil)
	if !errors.Is(err, domain.ErrInvalidWeightDiscrepancyGateRule) {
		t.Fatalf("不构成前置条件拿来判：%v", err)
	}
	_, err = (domain.WeightDiscrepancyGateRule{}).Judge(weightVersion(t, []string{"parcel-a"}, fixed), nil)
	if !errors.Is(err, domain.ErrInvalidWeightDiscrepancyGateRule) {
		t.Fatalf("零值规则拿来判：%v", err)
	}
	_, err = rule.Judge(domain.CustomsSubmissionVersion{}, nil)
	if !errors.Is(err, domain.ErrInvalidWeightDiscrepancyGateRule) {
		t.Fatalf("空版本：%v", err)
	}
	_, err = rule.Judge(weightVersion(t, []string{"parcel-a"}, fixed), nil)
	if !errors.Is(err, domain.ErrInvalidWeightDiscrepancyGateRule) {
		t.Fatalf("缺实测：%v", err)
	}
	_, err = rule.Judge(weightVersion(t, []string{"parcel-a"}, fixed), weightActuals(t, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: " ", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	}))
	if !errors.Is(err, domain.ErrInvalidWeightDiscrepancyGateRule) {
		t.Fatalf("空白实测：%v", err)
	}
}

func weightActuals(t *testing.T, actuals map[string]domain.WeightDiscrepancyMemberActual) map[domain.DeclaredParcelReference]domain.WeightDiscrepancyMemberActual {
	t.Helper()
	byParcel := make(map[domain.DeclaredParcelReference]domain.WeightDiscrepancyMemberActual, len(actuals))
	for parcel, actual := range actuals {
		byParcel[mustValue(t, domain.NewDeclaredParcelReference, parcel)] = actual
	}
	return byParcel
}

func TestWeightDiscrepancyReadingStaysOnTheGateThatListsIt(t *testing.T) {
	rule := weightRule(t, "0.5", "KG")
	reading := judgeWeights(t, rule, map[string]domain.MemberMeasuredWeight{
		"parcel-a": weightAt(t, "parcel-a", "2.00", "KG", "SYN-WEIGH-A"),
	}, map[string]domain.WeightDiscrepancyMemberActual{
		"parcel-a": {GrossWeight: "2.10", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-NOW"},
	})
	gate, err := domain.VerifyReleaseGate(
		mustValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		domain.OutboundRelease,
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		[]domain.PreconditionReference{domain.WeightDiscrepancyPrecondition},
		domain.GateMet, time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	attached, err := gate.WithWeightDiscrepancy(reading)
	if err != nil {
		t.Fatal(err)
	}
	got, has := attached.WeightDiscrepancy()
	if !has || got.State != reading.State || got.Members[0].MeasuredWeight != "2.10" {
		t.Fatalf("读数走样：%+v", got)
	}
	got.Members[0].MeasuredWeight = "9"
	again, _ := attached.WeightDiscrepancy()
	if again.Members[0].MeasuredWeight != "2.10" {
		t.Fatal("读口交出的切片能改回判断")
	}
	if _, has := gate.WeightDiscrepancy(); has {
		t.Fatal("挂读数改了原判断")
	}

	plain, err := domain.VerifyReleaseGate(
		mustValue(t, domain.NewDecisionScopeReference, "SYN-UNIT-01"),
		domain.OutboundRelease,
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		[]domain.PreconditionReference{mustValue(t, domain.NewPreconditionReference, "RESTRICTION/released")},
		domain.GateMet, time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.WithWeightDiscrepancy(reading); !errors.Is(err, domain.ErrInvalidGateVerification) {
		t.Fatalf("清单里没有这一道却挂上了：%v", err)
	}
}

func weightRule(t *testing.T, delta, unit string) domain.WeightDiscrepancyGateRule {
	t.Helper()
	rule, err := domain.AllowWeightDeltaUpTo(delta, unit)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func judgeWeights(
	t *testing.T,
	rule domain.WeightDiscrepancyGateRule,
	fixed map[string]domain.MemberMeasuredWeight,
	actuals map[string]domain.WeightDiscrepancyMemberActual,
) domain.WeightDiscrepancyGateReading {
	t.Helper()
	parcels := []string{"parcel-a", "parcel-b"}
	if _, onlyA := fixed["parcel-a"]; onlyA && len(fixed) == 1 && len(actuals) == 1 {
		parcels = []string{"parcel-a"}
	}
	reading, err := rule.Judge(weightVersion(t, parcels, fixed), weightActuals(t, actuals))
	if err != nil {
		t.Fatal(err)
	}
	return reading
}

func weightVersion(t *testing.T, parcels []string, fixed map[string]domain.MemberMeasuredWeight) domain.CustomsSubmissionVersion {
	t.Helper()
	members := make([]domain.DeclaredParcelReference, 0, len(parcels))
	for _, parcel := range parcels {
		members = append(members, mustValue(t, domain.NewDeclaredParcelReference, parcel))
	}
	unit, err := domain.FormDeclarationUnit(
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		mustValue(t, domain.NewCustomsCaseID, "case-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-IMPORT"),
		members)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 18, 1, 0, 0, 0, time.UTC)
	readiness, err := domain.JudgeReady(unit.ID(), mustValue(t, domain.NewReadinessBasisReference, "readiness-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := domain.GrantSubmissionAuthority(unit.ID(), mustValue(t, domain.NewSubmissionAuthorityReference, "authority-1"), at)
	if err != nil {
		t.Fatal(err)
	}
	weights := make([]domain.MemberMeasuredWeight, 0, len(fixed))
	for _, weight := range fixed {
		weights = append(weights, weight)
	}
	version, err := domain.FixSubmissionVersion(domain.CustomsSubmissionVersionSpec{
		ID:            mustValue(t, domain.NewSubmissionVersionID, "submission-1"),
		Unit:          unit,
		Dossier:       mustValue(t, domain.NewDossierSnapshotReference, "dossier-1"),
		Roles:         mustValue(t, domain.NewRoleSnapshotReference, "roles-1"),
		Readiness:     readiness,
		Authorization: authorization,
		FixedAt:       at.Add(time.Hour),
		MemberWeights: weights,
	})
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func weightAt(t *testing.T, parcel, gross, unit, source string) domain.MemberMeasuredWeight {
	t.Helper()
	weight, err := domain.NewMemberMeasuredWeight(
		mustValue(t, domain.NewDeclaredParcelReference, parcel), gross, unit, source,
		time.Date(2026, 11, 18, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return weight
}
