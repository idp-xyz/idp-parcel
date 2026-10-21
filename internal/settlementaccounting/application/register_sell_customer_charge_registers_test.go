package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件钉 ADR-0190 两本租户取值册的登记用例：落库时刻取时钟、册的落点原样交回、缺租户在碰册之前拒、缺依赖构造不出来；
// `已登记`与`重放`同笔交「评价参数已登记」，`冲突`不交（ADR-0229 决定五）。重放与冲突的判定在册上（真库用例），这里不重判。

var sellRegisterAt = time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)

type sellMomentRegisterDouble struct {
	calls  int
	at     time.Time
	effect ports.CatalogueRegistrationEffect
	err    error
}

func (double *sellMomentRegisterDouble) SaveSellBillingMomentAdoption(
	_ context.Context, _ domain.TenantID, _ domain.SellBillingMomentAdoption, at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	double.calls++
	double.at = at
	return double.effect, double.err
}

type feeItemMappingRegisterDouble struct {
	calls  int
	at     time.Time
	effect ports.CatalogueRegistrationEffect
}

func (double *feeItemMappingRegisterDouble) SaveFeeItemMapping(
	_ context.Context, _ domain.TenantID, _ domain.FeeItemMapping, at time.Time,
) (ports.CatalogueRegistrationEffect, error) {
	double.calls++
	double.at = at
	return double.effect, nil
}

// parameterHandoffDouble 记下交出的「评价参数已登记」，可令它失败。
type parameterHandoffDouble struct {
	intents []ports.EvaluationParameterRegisteredIntent
	err     error
}

func (double *parameterHandoffDouble) HandOffEvaluationParameterRegistered(
	_ context.Context, intent ports.EvaluationParameterRegisteredIntent,
) error {
	double.intents = append(double.intents, intent)
	return double.err
}

func TestRegisteringASellBillingMomentAdoptionPassesTheRegisterAnswerThrough(t *testing.T) {
	if _, err := application.NewRegisterSellBillingMomentAdoptionHandler(nil, &parameterHandoffDouble{}, fixedClock{at: sellRegisterAt}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺采用册 err = %v", err)
	}
	if _, err := application.NewRegisterSellBillingMomentAdoptionHandler(&sellMomentRegisterDouble{}, nil, fixedClock{at: sellRegisterAt}); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺交接口 err = %v", err)
	}
	register := &sellMomentRegisterDouble{effect: ports.CatalogueConflict}
	handoff := &parameterHandoffDouble{}
	handler, err := application.NewRegisterSellBillingMomentAdoptionHandler(register, handoff, fixedClock{at: sellRegisterAt})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	adoption, err := domain.NewSellBillingMomentAdoption(
		mustAccountValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
		domain.BillingMomentNetworkIntakeAdopted,
		mustAccountValue(t, domain.NewEstimatePricingScope, "SYN-SCOPE-SELL-1"),
	)
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}

	if _, err := handler.Register(t.Context(), application.RegisterSellBillingMomentAdoptionCommand{Adoption: adoption}); !errors.Is(err, domain.ErrBlankValue) || register.calls != 0 {
		t.Fatalf("缺租户 err = %v、碰册 %d 次，想要碰册之前拒", err, register.calls)
	}
	effect, err := handler.Register(t.Context(), application.RegisterSellBillingMomentAdoptionCommand{
		Tenant: mustAccountValue(t, domain.NewTenantID, "SYN-T1"), Adoption: adoption,
	})
	if err != nil || effect != ports.CatalogueConflict || !register.at.Equal(sellRegisterAt) {
		t.Fatalf("effect = %d err = %v at = %v，想要册的落点原样交回、时刻取时钟", effect, err, register.at)
	}
	if len(handoff.intents) != 0 {
		t.Fatalf("冲突交了 %d 封评价参数已登记", len(handoff.intents))
	}
	register.effect = ports.CatalogueRegistered
	if _, err := handler.Register(t.Context(), application.RegisterSellBillingMomentAdoptionCommand{
		Tenant: mustAccountValue(t, domain.NewTenantID, "SYN-T1"), Adoption: adoption,
	}); err != nil || len(handoff.intents) != 1 || handoff.intents[0].Kind != domain.ParameterSellBillingMomentAdoption ||
		handoff.intents[0].Subject != "SYN-ACCT-1" || !handoff.intents[0].RegisteredAt.Equal(sellRegisterAt) {
		t.Fatalf("已登记 err = %v intents = %+v", err, handoff.intents)
	}
	handoff.err = errors.New("outbox down")
	if _, err := handler.Register(t.Context(), application.RegisterSellBillingMomentAdoptionCommand{
		Tenant: mustAccountValue(t, domain.NewTenantID, "SYN-T1"), Adoption: adoption,
	}); !errors.Is(err, handoff.err) {
		t.Fatalf("交不出 err = %v，想要上抛让登记回滚", err)
	}
	handoff.err = nil

	register.err = domain.ErrCatalogueTargetMissing
	if _, err := handler.Register(t.Context(), application.RegisterSellBillingMomentAdoptionCommand{
		Tenant: mustAccountValue(t, domain.NewTenantID, "SYN-T1"), Adoption: adoption,
	}); !errors.Is(err, domain.ErrCatalogueTargetMissing) {
		t.Fatalf("账户不在册上 err = %v，想要哨兵原样上抛供登记口归格", err)
	}
}

func TestRegisteringAFeeItemMappingPassesTheRegisterAnswerThrough(t *testing.T) {
	if _, err := application.NewRegisterFeeItemMappingHandler(&feeItemMappingRegisterDouble{}, &parameterHandoffDouble{}, nil); !errors.Is(err, application.ErrNilDependency) {
		t.Fatalf("缺时钟 err = %v", err)
	}
	register := &feeItemMappingRegisterDouble{effect: ports.CatalogueReplay}
	handoff := &parameterHandoffDouble{}
	handler, err := application.NewRegisterFeeItemMappingHandler(register, handoff, fixedClock{at: sellRegisterAt})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	mapping, err := domain.NewFeeItemMapping(
		mustAccountValue(t, domain.NewPricingChargeCode, "BASE_FREIGHT"),
		mustAccountValue(t, domain.NewFeeItemReference, "SYN-FEE-FREIGHT"),
		mustAccountValue(t, domain.NewFeeItemMappingBasisReference, "SYN-CLAUSE-1"),
	)
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}

	if _, err := handler.RegisterFeeItemMapping(t.Context(), domain.TenantID{}, mapping); !errors.Is(err, domain.ErrBlankValue) || register.calls != 0 {
		t.Fatalf("缺租户 err = %v、碰册 %d 次，想要碰册之前拒", err, register.calls)
	}
	effect, err := handler.RegisterFeeItemMapping(t.Context(), mustAccountValue(t, domain.NewTenantID, "SYN-T1"), mapping)
	if err != nil || effect != ports.CatalogueReplay || !register.at.Equal(sellRegisterAt) {
		t.Fatalf("effect = %d err = %v at = %v，想要册的落点原样交回、时刻取时钟", effect, err, register.at)
	}
	if len(handoff.intents) != 1 || handoff.intents[0].Kind != domain.ParameterFeeItemMapping || handoff.intents[0].Subject != "BASE_FREIGHT" {
		t.Fatalf("重放要重交同一封评价参数已登记：%+v", handoff.intents)
	}
}
