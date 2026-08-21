package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证序列登记用例（票 08 件③）：受理门拦零值请求、登记册结果代数逐格译成应用
// 答案、依赖故障不吞——与价卡登记用例同一条纪律。

type seriesRegisterDouble struct {
	outcome    ports.ReferenceSeriesRegistrationOutcome
	err        error
	registered []domain.ReferenceSeriesRegistration
}

func (double *seriesRegisterDouble) Register(
	_ context.Context,
	registration domain.ReferenceSeriesRegistration,
) (ports.ReferenceSeriesRegistrationOutcome, error) {
	if double.err != nil {
		return ports.ReferenceSeriesRegistrationOutcomeInvalid, double.err
	}
	double.registered = append(double.registered, registration)
	return double.outcome, nil
}

func (double *seriesRegisterDouble) ResolveAt(
	_ context.Context,
	_ domain.TenantID,
	_ domain.VersionReference,
	_ time.Time,
) (domain.ResolvedSeriesReading, bool, error) {
	return domain.ResolvedSeriesReading{}, false, nil
}

func seriesRegistrationFixture(t *testing.T) domain.ReferenceSeriesRegistration {
	t.Helper()
	reference, err := domain.NewVersionReference(
		domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", "v1", "sha256:syn-fuel")
	if err != nil {
		t.Fatalf("构造序列引用：%v", err)
	}
	period, err := domain.NewSeriesPeriodValue(
		time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		mustValue(t, domain.ParseDecimal, "0.22"),
		"SYN-EVIDENCE/fuel-2026-W32",
	)
	if err != nil {
		t.Fatalf("构造期次：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Periods:          []domain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("构造序列登记：%v", err)
	}
	return registration
}

func seriesHandler(double *seriesRegisterDouble) *application.RegisterReferenceSeriesHandler {
	return application.NewRegisterReferenceSeriesHandler(application.RegisterReferenceSeriesDeps{Register: double})
}

// TestRegisterReferenceSeriesRecordsNewVersion 证新版本入册答`已登记`且登记原样交给
// 登记册。
func TestRegisterReferenceSeriesRecordsNewVersion(t *testing.T) {
	double := &seriesRegisterDouble{outcome: ports.ReferenceSeriesRegistered}

	outcome, err := seriesHandler(double).Handle(t.Context(),
		application.RegisterReferenceSeriesCommand{Registration: seriesRegistrationFixture(t)})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}
	if outcome != application.ReferenceSeriesRecorded {
		t.Fatalf("outcome = %s, 想要 RECORDED", outcome)
	}
	if len(double.registered) != 1 {
		t.Fatalf("登记册收到 %d 份登记, 想要 1", len(double.registered))
	}
}

// TestRegisterReferenceSeriesTranslatesRegisterAlgebra 证登记册结果代数逐格译成应用
// 答案，不吞格也不并格。
func TestRegisterReferenceSeriesTranslatesRegisterAlgebra(t *testing.T) {
	cases := map[string]struct {
		saved ports.ReferenceSeriesRegistrationOutcome
		want  application.RegisterReferenceSeriesOutcome
	}{
		"幂等重放":   {ports.ReferenceSeriesAlreadyRegistered, application.ReferenceSeriesAlreadyOnRegister},
		"版本内容冲突": {ports.ReferenceSeriesContentConflict, application.ReferenceSeriesRegistrationConflict},
		"形状不可比":  {ports.ReferenceSeriesCanonicalizationDiffers, application.ReferenceSeriesRegistrationIncomparable},
	}
	for label, tc := range cases {
		double := &seriesRegisterDouble{outcome: tc.saved}
		outcome, err := seriesHandler(double).Handle(t.Context(),
			application.RegisterReferenceSeriesCommand{Registration: seriesRegistrationFixture(t)})
		if err != nil {
			t.Fatalf("%s: err = %v", label, err)
		}
		if outcome != tc.want {
			t.Fatalf("%s: outcome = %s, 想要 %s", label, outcome, tc.want)
		}
	}
}

// TestRegisterReferenceSeriesRefusesEmptyCommand 证受理门：零值登记不受理，登记册
// 一次都不被叫到。
func TestRegisterReferenceSeriesRefusesEmptyCommand(t *testing.T) {
	double := &seriesRegisterDouble{outcome: ports.ReferenceSeriesRegistered}

	outcome, err := seriesHandler(double).Handle(t.Context(), application.RegisterReferenceSeriesCommand{})
	if err != nil {
		t.Fatalf("受理门不该报错：%v", err)
	}
	if outcome != application.ReferenceSeriesRegistrationNotAccepted {
		t.Fatalf("outcome = %s, 想要 NOT_ACCEPTED", outcome)
	}
	if len(double.registered) != 0 {
		t.Fatalf("零值请求不该到达登记册")
	}
}

// TestRegisterReferenceSeriesSurfacesRegisterFailure 证依赖故障答`未决`且原因随错误
// 交回。
func TestRegisterReferenceSeriesSurfacesRegisterFailure(t *testing.T) {
	boom := errors.New("series register is down")
	double := &seriesRegisterDouble{err: boom}

	outcome, err := seriesHandler(double).Handle(t.Context(),
		application.RegisterReferenceSeriesCommand{Registration: seriesRegistrationFixture(t)})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, 想要包住登记册失败原因", err)
	}
	if outcome != application.ReferenceSeriesRegistrationUndecided {
		t.Fatalf("outcome = %s, 想要 UNDECIDED", outcome)
	}
}
