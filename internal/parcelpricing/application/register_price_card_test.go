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

// 本文件证价卡登记用例（票 07 件③）：受理门拦零值请求、仓储结果代数逐格译成应用
// 答案、依赖故障不吞——登记是治理动作，操作者必须拿到失败原因，不能拿一个没有成因
// 的`未决`。

type priceCardCatalogDouble struct {
	outcome    ports.PriceCardRegistrationOutcome
	err        error
	registered []domain.PriceCardRegistration
}

func (double *priceCardCatalogDouble) Register(
	_ context.Context,
	registration domain.PriceCardRegistration,
) (ports.PriceCardRegistrationOutcome, error) {
	if double.err != nil {
		return ports.PriceCardRegistrationOutcomeInvalid, double.err
	}
	double.registered = append(double.registered, registration)
	return double.outcome, nil
}

func (double *priceCardCatalogDouble) LoadApplicable(
	_ context.Context,
	_ domain.TenantID,
	_ domain.PricingDirection,
	_ domain.PricingScopeID,
	_ time.Time,
) ([]domain.PricingPlanVersion, error) {
	return nil, nil
}

func registrationFixture(t *testing.T) domain.PriceCardRegistration {
	t.Helper()
	source, err := domain.NewSourceFileIdentity(
		"SYN-PRC-CARD-260820.xlsx",
		"9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d")
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	grant, err := domain.NewVersionReference(
		domain.ArtifactCommercialAuthorization, "SYN-PRC-GRANT", "v1", "sha256:syn-grant")
	if err != nil {
		t.Fatalf("构造授权引用：%v", err)
	}
	registration, err := domain.NewPriceCardRegistration(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		minimalPlan(t),
		source,
		grant,
		"SYN-PRC-GOVERNANCE",
	)
	if err != nil {
		t.Fatalf("构造价卡登记：%v", err)
	}
	return registration
}

func registerHandler(double *priceCardCatalogDouble) *application.RegisterPriceCardHandler {
	return application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{Catalog: double})
}

// TestRegisterPriceCardRecordsNewVersion 证新版本入册答`已登记`且登记原样交给仓储。
func TestRegisterPriceCardRecordsNewVersion(t *testing.T) {
	double := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}

	outcome, err := registerHandler(double).Handle(t.Context(),
		application.RegisterPriceCardCommand{Registration: registrationFixture(t)})
	if err != nil {
		t.Fatalf("登记失败：%v", err)
	}
	if outcome != application.PriceCardRecorded {
		t.Fatalf("outcome = %s, 想要 RECORDED", outcome)
	}
	if len(double.registered) != 1 {
		t.Fatalf("仓储收到 %d 份登记, 想要 1", len(double.registered))
	}
}

// TestRegisterPriceCardTranslatesRegisterAlgebra 证仓储结果代数逐格译成应用答案，
// 不吞格也不并格。
func TestRegisterPriceCardTranslatesRegisterAlgebra(t *testing.T) {
	cases := map[string]struct {
		saved ports.PriceCardRegistrationOutcome
		want  application.RegisterPriceCardOutcome
	}{
		"幂等重放":   {ports.PriceCardAlreadyRegistered, application.PriceCardAlreadyOnRegister},
		"版本内容冲突": {ports.PriceCardContentConflict, application.PriceCardRegistrationConflict},
		"规范化不可比": {ports.PriceCardCanonicalizationDiffers, application.PriceCardRegistrationIncomparable},
	}
	for label, tc := range cases {
		double := &priceCardCatalogDouble{outcome: tc.saved}
		outcome, err := registerHandler(double).Handle(t.Context(),
			application.RegisterPriceCardCommand{Registration: registrationFixture(t)})
		if err != nil {
			t.Fatalf("%s: err = %v", label, err)
		}
		if outcome != tc.want {
			t.Fatalf("%s: outcome = %s, 想要 %s", label, outcome, tc.want)
		}
	}
}

// TestRegisterPriceCardRefusesEmptyCommand 证受理门：零值登记不受理，仓储一次都
// 不被叫到。
func TestRegisterPriceCardRefusesEmptyCommand(t *testing.T) {
	double := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}

	outcome, err := registerHandler(double).Handle(t.Context(), application.RegisterPriceCardCommand{})
	if err != nil {
		t.Fatalf("受理门不该报错：%v", err)
	}
	if outcome != application.PriceCardRegistrationNotAccepted {
		t.Fatalf("outcome = %s, 想要 NOT_ACCEPTED", outcome)
	}
	if len(double.registered) != 0 {
		t.Fatalf("零值请求不该到达仓储")
	}
}

// TestRegisterPriceCardSurfacesCatalogFailure 证依赖故障答`未决`且原因随错误交回：
// 登记是治理动作，没有成因的未决没法续办。
func TestRegisterPriceCardSurfacesCatalogFailure(t *testing.T) {
	boom := errors.New("register store is down")
	double := &priceCardCatalogDouble{err: boom}

	outcome, err := registerHandler(double).Handle(t.Context(),
		application.RegisterPriceCardCommand{Registration: registrationFixture(t)})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, 想要包住仓储失败原因", err)
	}
	if outcome != application.PriceCardRegistrationUndecided {
		t.Fatalf("outcome = %s, 想要 UNDECIDED", outcome)
	}
}
