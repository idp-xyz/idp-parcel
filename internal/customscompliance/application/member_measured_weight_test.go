package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var weighedAt = time.Date(2026, 11, 18, 4, 5, 6, 0, time.UTC)

type memberWeightDouble struct {
	calls        int
	err          error
	one          map[string]ports.MeasuredWeightReading
	undetermined map[string]string
}

func (double *memberWeightDouble) LoadMemberMeasuredWeight(
	_ context.Context,
	_ domain.TenantID,
	parcel domain.DeclaredParcelReference,
) (ports.MeasuredWeightReading, error) {
	double.calls++
	if double.err != nil {
		return ports.MeasuredWeightReading{}, double.err
	}
	if cause, ok := double.undetermined[parcel.String()]; ok {
		return ports.MeasuredWeightReading{Answer: ports.MeasuredWeightUndetermined, Cause: cause}, nil
	}
	if reading, ok := double.one[parcel.String()]; ok {
		reading.Answer = ports.MeasuredWeightOne
		return reading, nil
	}
	return ports.MeasuredWeightReading{Answer: ports.MeasuredWeightNone}, nil
}

func TestASubmissionCarriesOnlyTheMembersThatHaveAMeasurement(t *testing.T) {
	fixture := newDeclarationFixture(t)
	fixture.weights.one = map[string]ports.MeasuredWeightReading{
		"parcel-1": {GrossWeight: "2.1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A", MeasuredAt: weighedAt},
	}
	result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	record, present := result.Record()
	if !present || result.Outcome() != application.DeclarationSubmitted {
		t.Fatalf("outcome = %s present = %v", result.Outcome(), present)
	}
	weights := record.Version.MemberWeights()
	if len(weights) != 1 || weights[0].Parcel().String() != "parcel-1" || weights[0].GrossWeight() != "2.1" ||
		weights[0].GrossWeightUnit() != "KG" || weights[0].MeasurementSource() != "SYN-WEIGH-A" ||
		!weights[0].MeasuredAt().Equal(weighedAt) {
		t.Fatalf("毛重 = %+v", weights)
	}
}

func TestAnUndeterminedMeasurementDoesNotFixAVersion(t *testing.T) {
	fixture := newDeclarationFixture(t)
	fixture.weights.undetermined = map[string]string{"parcel-1": "SEVERAL_WITHOUT_ADOPTION"}
	result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.DeclarationUndecided || result.UndecidedReason().String() != "MEMBER_MEASUREMENT_UNDETERMINED" ||
		result.ContinuationReference() == "" {
		t.Fatalf("未决 = %s %s %q", result.Outcome(), result.UndecidedReason(), result.ContinuationReference())
	}
	if fixture.units.saves != 0 || fixture.versions.minted != 0 || len(fixture.store.records) != 0 {
		t.Fatalf("落了不该落的：units=%d minted=%d records=%d", fixture.units.saves, fixture.versions.minted, len(fixture.store.records))
	}
}

func TestAnUnreadableMeasurementDoesNotFixAVersion(t *testing.T) {
	fixture := newDeclarationFixture(t)
	fixture.weights.err = errors.New("measurement store down")
	result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.DeclarationUndecided || result.UndecidedReason().String() != "MEMBER_MEASUREMENT_UNAVAILABLE" ||
		result.ContinuationReference() == "" {
		t.Fatalf("未决 = %s %s %q", result.Outcome(), result.UndecidedReason(), result.ContinuationReference())
	}
	if fixture.units.saves != 0 || fixture.versions.minted != 0 || len(fixture.store.records) != 0 {
		t.Fatalf("落了不该落的：units=%d minted=%d records=%d", fixture.units.saves, fixture.versions.minted, len(fixture.store.records))
	}
}

func TestASubmissionDoesNotReadMeasurementsUntilBothGatesPass(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		ready  bool
		grant  bool
		revoke string
	}{
		{name: "就绪未配置", ready: false, grant: true},
		{name: "不再就绪", ready: true, grant: true, revoke: "readiness"},
		{name: "授权未配置", ready: true, grant: false},
		{name: "授权已失效", ready: true, grant: true, revoke: "authority"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newDeclarationFixture(t)
			fixture.readiness.configured = testCase.ready
			fixture.authority.granted = testCase.grant
			fixture.readiness.revoked = testCase.revoke == "readiness"
			fixture.authority.revoked = testCase.revoke == "authority"
			fixture.weights.err = errors.New("should not be asked")
			if _, err := fixture.handler.Handle(context.Background(), declarationCommand(t)); err != nil {
				t.Fatal(err)
			}
			if fixture.weights.calls != 0 {
				t.Fatalf("过门前读了实测 %d 次", fixture.weights.calls)
			}
		})
	}
}

func TestReplayingASubmissionDoesNotRereadMeasurementsOrConflictAfterAReweigh(t *testing.T) {
	fixture := newDeclarationFixture(t)
	fixture.weights.one = map[string]ports.MeasuredWeightReading{
		"parcel-1": {GrossWeight: "2.1", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-A", MeasuredAt: weighedAt},
	}
	first, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	original, _ := first.Record()
	asked := fixture.weights.calls
	fixture.weights.one["parcel-1"] = ports.MeasuredWeightReading{
		GrossWeight: "3", GrossWeightUnit: "KG", MeasurementSource: "SYN-WEIGH-B", MeasuredAt: weighedAt.Add(time.Hour),
	}
	second, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome() != application.DeclarationExistingVersion {
		t.Fatalf("重放 = %s，want EXISTING_VERSION", second.Outcome())
	}
	if fixture.weights.calls != asked {
		t.Fatalf("重放又读了实测：%d → %d", asked, fixture.weights.calls)
	}
	replayed, present := second.Record()
	if !present || len(replayed.Version.MemberWeights()) != 1 ||
		replayed.Version.MemberWeights()[0].MeasurementSource() != original.Version.MemberWeights()[0].MeasurementSource() {
		t.Fatalf("交回的毛重变了：%+v", replayed.Version.MemberWeights())
	}
}

func TestAMissingMeasurementViewIsNotTreatedAsNothingWeighed(t *testing.T) {
	_, err := application.NewSubmitDeclarationHandler(application.SubmitDeclarationDeps{})
	if !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("err = %v, want ErrNilDependency", err)
	}
}

func TestACorrectionFixesMemberWeightsByTheSameRule(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")
	asked := fixture.declaration.weights.calls
	fixture.declaration.weights.one = map[string]ports.MeasuredWeightReading{
		"parcel-2": {GrossWeight: "4", GrossWeightUnit: "G", MeasurementSource: "SYN-WEIGH-C", MeasuredAt: weighedAt},
	}
	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	record, present := result.Record()
	if !present || result.Outcome() != application.DeclarationCorrected {
		t.Fatalf("outcome = %s present = %v", result.Outcome(), present)
	}
	weights := record.Version.MemberWeights()
	if len(weights) != 1 || weights[0].Parcel().String() != "parcel-2" || weights[0].GrossWeightUnit() != "G" ||
		weights[0].MeasurementSource() != "SYN-WEIGH-C" {
		t.Fatalf("更正版毛重 = %+v", weights)
	}
	if fixture.declaration.weights.calls == asked {
		t.Fatal("更正成版没有读实测")
	}
}

func TestAnUndeterminedMeasurementDoesNotMintACorrection(t *testing.T) {
	fixture := newCorrectionFixture(t)
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")
	fixture.declaration.weights.undetermined = map[string]string{"parcel-1": "SAME_OCCURRED_AT"}
	saves, minted := fixture.declaration.store.saves, fixture.declaration.versions.minted
	result, err := fixture.handler.Handle(context.Background(), correctionCommand(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome() != application.DeclarationUndecided || result.UndecidedReason().String() != "MEMBER_MEASUREMENT_UNDETERMINED" {
		t.Fatalf("未决 = %s %s", result.Outcome(), result.UndecidedReason())
	}
	if fixture.declaration.store.saves != saves || fixture.declaration.versions.minted != minted {
		t.Fatalf("更正未决仍写了库或消耗了版本号：saves %d→%d minted %d→%d",
			saves, fixture.declaration.store.saves, minted, fixture.declaration.versions.minted)
	}
}

func TestACorrectionDoesNotReadMeasurementsWhenTheReplayOrTheGateStopsIt(t *testing.T) {
	fixture := newCorrectionFixture(t)
	asked := fixture.declaration.weights.calls
	same := correctionCommand(t)
	same.Dossier = "dossier-snapshot-1"
	replayed, err := fixture.handler.Handle(context.Background(), same)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Outcome() != application.DeclarationExistingVersion || fixture.declaration.weights.calls != asked {
		t.Fatalf("同内容重放 = %s calls %d→%d", replayed.Outcome(), asked, fixture.declaration.weights.calls)
	}
	fixture.formTargetOnCurrent(t, domain.InCaseCorrection, "regulatory-request/RR-9", "submission/v1")
	fixture.declaration.readiness.revoked = true
	if _, err := fixture.handler.Handle(context.Background(), correctionCommand(t)); err != nil {
		t.Fatal(err)
	}
	if fixture.declaration.weights.calls != asked {
		t.Fatalf("不再就绪仍读了实测：%d", fixture.declaration.weights.calls)
	}
}
